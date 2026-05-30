package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	azureProviderName       = "azure-openai"
	envAzureTenantID        = "AZURE_TENANT_ID"
	envAzureClientID        = "AZURE_CLIENT_ID"
	envAzureClientSecret    = "AZURE_CLIENT_SECRET"
	envAzureSubscriptionID  = "AZURE_SUBSCRIPTION_ID"
	envAzureCostScope       = "AZURE_COST_SCOPE"
	envAzureOpenAIResources = "AZURE_OPENAI_RESOURCE_IDS"

	azureMaxDays           = 366
	azureMaxPages          = 50
	azureMgmtScope         = "https://management.azure.com/.default"
	azureCostAPIVersion    = "2025-03-01"
	azureMonitorAPIVersion = "2023-10-01"
	azureUserAgentDev      = "costroid/dev"

	azureDefaultTokenBaseURL = "https://login.microsoftonline.com"
	azureDefaultMgmtBaseURL  = "https://management.azure.com"

	azureRecordedLayout = "2006-01-02T15:04:05Z"
)

// azureModelRegex matches common Azure OpenAI model slug forms inside
// meter / sku description strings. Case-insensitive. Returns the matched
// substring lowercased; callers normalize from there.
var azureModelRegex = regexp.MustCompile(`(?i)(gpt-[0-9]+(?:\.[0-9]+)?o?(?:-mini|-turbo|-nano)?|text-embedding-[a-z0-9][a-z0-9-]*|whisper(?:-[0-9a-z]+)?|dall-e-[0-9]+|o[0-9]+(?:-mini)?)`)

// azureServiceFilters is the conservative default for client-side
// defense-in-depth filtering. Server-side filtering uses the same list
// (see buildCostQueryBody). Lowercased for case-insensitive compare.
var azureServiceFilters = []string{"azure openai", "cognitive services"}

// AzureOpenAIConfig is the keyword-style constructor input for the
// provider. Tests construct this directly; the registry factory
// populates it from environment variables.
type AzureOpenAIConfig struct {
	TenantID       string
	ClientID       string
	ClientSecret   string
	SubscriptionID string
	Scope          string   // resolved cost-management scope, e.g. "subscriptions/<id>"
	ResourceIDs    []string // empty => Monitor enrichment skipped
}

// AzureOpenAIProvider fetches Azure OpenAI cost metadata from the Azure
// Cost Management Query API. When ResourceIDs are configured, it
// additionally fetches token-count metrics from Azure Monitor and joins
// them onto cost rows by (date, resourceId, model) when uniquely
// resolvable. Cost is always authoritative from Cost Management; Monitor
// only writes into token fields, never into CostUSD.
//
// METADATA ONLY. No prompts, completions, messages, chat content,
// function arguments, tool calls, source code, repository contents,
// diagnostic logs, request bodies, or response bodies are read.
type AzureOpenAIProvider struct {
	TenantID       string
	ClientID       string
	ClientSecret   string
	SubscriptionID string
	Scope          string
	ResourceIDs    []string

	// Overridable for tests.
	TokenBaseURL      string
	ManagementBaseURL string
	HTTPClient        *http.Client
	UserAgent         string
	Now               func() time.Time

	// Token cache (no disk persistence).
	tokenMu     sync.Mutex
	accessToken string
	tokenExp    time.Time
}

var _ Provider = (*AzureOpenAIProvider)(nil)

// NewAzureOpenAIProvider constructs a provider from a config struct.
// HTTPClient defaults to a 30s-timeout client; Now defaults to time.Now.
func NewAzureOpenAIProvider(cfg AzureOpenAIConfig) *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		TenantID:          cfg.TenantID,
		ClientID:          cfg.ClientID,
		ClientSecret:      cfg.ClientSecret,
		SubscriptionID:    cfg.SubscriptionID,
		Scope:             cfg.Scope,
		ResourceIDs:       cfg.ResourceIDs,
		TokenBaseURL:      azureDefaultTokenBaseURL,
		ManagementBaseURL: azureDefaultMgmtBaseURL,
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
		UserAgent:         azureUserAgentDev,
	}
}

func (p *AzureOpenAIProvider) Name() string { return azureProviderName }

// Fetch returns normalized Azure OpenAI cost records for the last `days`
// calendar days (UTC). When p.ResourceIDs is non-empty, token-count
// fields are enriched from Azure Monitor metrics; otherwise they stay 0.
func (p *AzureOpenAIProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	days = clampAzureDays(days)
	end := p.now().UTC()
	start := end.AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	endOfDay := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, time.UTC)

	rows, err := p.fetchCostRows(ctx, start, endOfDay)
	if err != nil {
		return nil, err
	}

	records := make([]NormalizedCostRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, mapAzureCostRow(row, p.Scope))
	}

	if len(p.ResourceIDs) > 0 {
		_ = enrichWithMonitor(ctx, p, records, start, endOfDay)
	}

	sortAzureRecords(records)
	return records, nil
}

func (p *AzureOpenAIProvider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func clampAzureDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > azureMaxDays {
		return azureMaxDays
	}
	return days
}

// ---------- narrow decode types (metadata-only by construction) ----------

// azureCostQueryResponse models only the structural envelope of the
// Cost Management Query API response. The actual row values are typed
// per-column at extraction time, never via map[string]any forwarding.
type azureCostQueryResponse struct {
	Properties struct {
		NextLink string `json:"nextLink"`
		Columns  []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Rows [][]json.RawMessage `json:"rows"`
	} `json:"properties"`
	NextLink string `json:"nextLink"` // some response shapes put nextLink at the top level
}

// azureCostRow is the typed per-row projection. Only the fields enumerated
// here are read from the response — extra columns are discarded.
type azureCostRow struct {
	Date             string  // canonical RFC3339 UTC at 00:00:00
	Cost             float64 // USD only (non-USD rows are filtered upstream)
	Currency         string
	UsageQuantity    float64
	ResourceID       string
	ResourceGroup    string
	ServiceName      string
	Meter            string
	MeterCategory    string
	MeterSubCategory string
	UnitOfMeasure    string
}

// ---------- mapping ----------

func mapAzureCostRow(row azureCostRow, scope string) NormalizedCostRecord {
	model := extractAzureModel(row.MeterSubCategory)
	if model == "" {
		model = extractAzureModel(row.Meter)
	}
	projectID := row.ResourceID
	if projectID == "" {
		projectID = row.ResourceGroup
	}
	if projectID == "" {
		projectID = scope
	}
	sku := firstNonEmpty(row.MeterSubCategory, row.Meter)
	return NormalizedCostRecord{
		Provider:          azureProviderName,
		Model:             model,
		PromptTokens:      0, // populated by enrichWithMonitor when safely joinable
		CompletionTokens:  0,
		TotalTokens:       0,
		CostUSD:           row.Cost,
		RecordedAt:        row.Date,
		APIKeyID:          "",
		ProjectID:         projectID,
		Product:           row.ServiceName,
		SKU:               sku,
		UnitType:          row.UnitOfMeasure,
		UsageQuantity:     row.UsageQuantity,
		UnitPriceUSD:      0,
		GrossAmountUSD:    row.Cost,
		DiscountAmountUSD: 0,
		SourceHash:        azureSourceHash(row.Date, scope, row.ResourceID, row.Meter, sku, row.UnitOfMeasure),
	}
}

// extractAzureModel returns the first matched model slug from s,
// lowercased. Returns "" when no match — never returns garbage.
func extractAzureModel(s string) string {
	if s == "" {
		return ""
	}
	match := azureModelRegex.FindString(s)
	if match == "" {
		return ""
	}
	return strings.ToLower(match)
}

// formatAzureDate accepts a YYYYMMDD integer (e.g., 20260520) and returns
// "2026-05-20T00:00:00Z". Returns "" for unparseable input — caller must
// filter such rows out before mapping.
func formatAzureDate(yyyymmdd int) string {
	if yyyymmdd < 19000101 || yyyymmdd > 99991231 {
		return ""
	}
	s := strconv.Itoa(yyyymmdd)
	if len(s) != 8 {
		return ""
	}
	t, err := time.ParseInLocation("20060102", s, time.UTC)
	if err != nil {
		return ""
	}
	return t.Format(azureRecordedLayout)
}

// ---------- source hash (identity-only) ----------

// azureSourceHash is identity-only; volatile fields (cost, quantity) are
// intentionally excluded so re-syncs UPSERT in place. Provider prefix
// prevents cross-provider collisions. Mirrors the C9.1 copilotSourceHash
// pattern.
func azureSourceHash(date, scope, resourceID, meter, sku, unitOfMeasure string) string {
	h := sha256.New()
	fmt.Fprintf(h, "azure-openai|%s|%s|%s|%s|%s|%s",
		date, scope, resourceID, meter, sku, unitOfMeasure)
	return hex.EncodeToString(h.Sum(nil))
}

func sortAzureRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].ProjectID != records[j].ProjectID {
			return records[i].ProjectID < records[j].ProjectID
		}
		if records[i].SKU != records[j].SKU {
			return records[i].SKU < records[j].SKU
		}
		if records[i].Model != records[j].Model {
			return records[i].Model < records[j].Model
		}
		return records[i].UnitType < records[j].UnitType
	})
}

// Safe-error type, permission-hint wrappers, and scope normalization
// live in azure_openai_errors.go to keep this file under the 300-line
// rule and the security-relevant code in one place.
