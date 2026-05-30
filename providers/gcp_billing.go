package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	gcpBillingProviderName = "gcp-billing"

	envGCPServiceAccountJSON = "GCP_SERVICE_ACCOUNT_JSON"
	envGCPBillingProject     = "GCP_BILLING_PROJECT"
	envGCPBillingTable       = "GCP_BILLING_TABLE"
	envGCPBillingProjectFlt  = "GCP_BILLING_PROJECT_FILTER"
	envGCPBillingServiceFlt  = "GCP_BILLING_SERVICE_FILTER"
	envGCPBillingCurrency    = "GCP_BILLING_CURRENCY"

	gcpBillingMaxDays      = 366
	gcpBillingUserAgentDev = "costroid/dev"

	gcpBillingDefaultTokenURL    = "https://oauth2.googleapis.com/token"
	gcpBillingDefaultBigQueryURL = "https://bigquery.googleapis.com"
	gcpBillingScope              = "https://www.googleapis.com/auth/bigquery.readonly"
	gcpBillingDefaultCurrency    = "USD"
)

// GCPBillingConfig is the keyword-style constructor input for the
// provider. The registry factory populates it from environment variables;
// tests construct it directly.
type GCPBillingConfig struct {
	ServiceAccountJSONPath string
	BillingProject         string   // GCP project that runs the BigQuery query
	BillingTable           string   // `project.dataset.table`
	ProjectFilter          string   // optional project.id filter
	ServiceFilters         []string // optional lowercased substrings (client-side)
	Currency               string   // default USD
}

// GCPBillingProvider queries Google Cloud Billing detailed-export data
// from BigQuery via the REST API. Service-account JSON key + JWT-bearer
// OAuth flow. Metadata only: never selects labels, system_labels, tags,
// credits, adjustment_info, or `*` projections. Never calls Cloud
// Logging, Audit Logs, Vertex runtime, request/prompt/completion APIs.
type GCPBillingProvider struct {
	ServiceAccountJSONPath string
	BillingProject         string
	BillingTable           string
	ProjectFilter          string
	ServiceFilters         []string
	Currency               string

	// Overridable for tests.
	TokenURL    string // exact URL of the OAuth token endpoint
	BigQueryURL string // base URL of the BigQuery REST endpoint
	HTTPClient  *http.Client
	UserAgent   string
	Now         func() time.Time

	// Cached service account, loaded lazily on first Fetch.
	saMu sync.Mutex
	sa   *gcpServiceAccount

	// Token cache (in memory only, mirrors azure_openai_auth.go).
	tokenMu     sync.Mutex
	accessToken string
	tokenExp    time.Time
}

var _ Provider = (*GCPBillingProvider)(nil)

// NewGCPBillingProvider constructs a provider from a config struct.
// HTTPClient defaults to a 30s-timeout client; Now defaults to time.Now.
func NewGCPBillingProvider(cfg GCPBillingConfig) *GCPBillingProvider {
	currency := strings.ToUpper(strings.TrimSpace(cfg.Currency))
	if currency == "" {
		currency = gcpBillingDefaultCurrency
	}
	return &GCPBillingProvider{
		ServiceAccountJSONPath: cfg.ServiceAccountJSONPath,
		BillingProject:         cfg.BillingProject,
		BillingTable:           cfg.BillingTable,
		ProjectFilter:          cfg.ProjectFilter,
		ServiceFilters:         cfg.ServiceFilters,
		Currency:               currency,
		HTTPClient:             &http.Client{Timeout: 30 * time.Second},
		UserAgent:              gcpBillingUserAgentDev,
	}
}

func (p *GCPBillingProvider) Name() string { return gcpBillingProviderName }

// Fetch returns normalized GCP billing rows for the last `days` calendar
// days (UTC) filtered to the configured currency. days is clamped to
// [1, 366].
func (p *GCPBillingProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	days = clampGCPBillingDays(days)

	if err := validateGCPTableID(p.BillingTable); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.BillingProject) == "" {
		return nil, fmt.Errorf("gcp-billing: %s not set", envGCPBillingProject)
	}

	rows, err := p.fetchBillingRows(ctx, days)
	if err != nil {
		return nil, err
	}

	filters := lowerCopy(p.ServiceFilters)
	records := make([]NormalizedCostRecord, 0, len(rows))
	for _, row := range rows {
		if !rowMatchesGCPServiceFilters(row, filters) {
			continue
		}
		records = append(records, mapGCPBillingRow(row, p.BillingTable))
	}
	sortGCPBillingRecords(records)
	return records, nil
}

func (p *GCPBillingProvider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *GCPBillingProvider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (p *GCPBillingProvider) userAgent() string {
	if p.UserAgent != "" {
		return p.UserAgent
	}
	return gcpBillingUserAgentDev
}

func (p *GCPBillingProvider) bigQueryBaseURL() string {
	if p.BigQueryURL != "" {
		return strings.TrimRight(p.BigQueryURL, "/")
	}
	return gcpBillingDefaultBigQueryURL
}

func clampGCPBillingDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > gcpBillingMaxDays {
		return gcpBillingMaxDays
	}
	return days
}

// gcpBillingRow, mapping, hash, sort, and filter helpers live in
// gcp_billing_map.go to keep this file under the 300-line limit.
