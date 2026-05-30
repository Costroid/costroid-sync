package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultGitHubAPIBaseURL     = "https://api.github.com"
	githubCopilotProviderName   = "github-copilot"
	githubCopilotAPIVersion     = "2026-03-10"
	githubCopilotUserAgentDev   = "costroid/dev"
	githubCopilotMaxDays        = 31
	githubCopilotPathPrefix     = "/organizations/"
	githubCopilotPathSuffix     = "/settings/billing/premium_request/usage"
	githubCopilotDateLayout     = "2006-01-02"
	githubCopilotRecordedLayout = "2006-01-02T15:04:05Z"
)

// GitHubCopilotProvider fetches metadata-only premium-request billing usage
// from the GitHub organization billing API. Billing metadata only — no
// repository, issue, PR, source-code, Copilot chat, or completion data is
// ever requested or surfaced.
type GitHubCopilotProvider struct {
	Token      string
	Org        string
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

var _ Provider = (*GitHubCopilotProvider)(nil)

func NewGitHubCopilotProvider(token, org string) *GitHubCopilotProvider {
	return &GitHubCopilotProvider{
		Token:      token,
		Org:        org,
		BaseURL:    DefaultGitHubAPIBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  githubCopilotUserAgentDev,
	}
}

func (p *GitHubCopilotProvider) Name() string { return githubCopilotProviderName }

func (p *GitHubCopilotProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	clamped := clampCopilotDays(days)
	now := time.Now().UTC()
	var out []NormalizedCostRecord
	for i := 0; i < clamped; i++ {
		day := now.AddDate(0, 0, -i)
		records, err := p.fetchDay(ctx, day)
		if err != nil {
			return nil, wrapCopilotPermissionHint(err)
		}
		out = append(out, records...)
	}
	sortCopilotRecords(out)
	return out, nil
}

func clampCopilotDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > githubCopilotMaxDays {
		return githubCopilotMaxDays
	}
	return days
}

// ---------- narrow decode types (metadata-only by construction) ----------

// githubCopilotUsageResponse models the official premium_request/usage
// response shape. Only fields enumerated here are decoded. Any extra
// fields in the response (whether GitHub adds new ones or a poisoned test
// fixture injects forbidden ones) are silently discarded by encoding/json.
// We never read the raw body — no json.RawMessage, no map[string]any.
type githubCopilotUsageResponse struct {
	UsageItems []githubCopilotUsageItem `json:"usageItems"`
}

type githubCopilotUsageItem struct {
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	Model            string  `json:"model"`
	UnitType         string  `json:"unitType"`
	PricePerUnit     float64 `json:"pricePerUnit"`
	GrossQuantity    float64 `json:"grossQuantity"`
	GrossAmount      float64 `json:"grossAmount"`
	DiscountQuantity float64 `json:"discountQuantity"`
	DiscountAmount   float64 `json:"discountAmount"`
	NetQuantity      float64 `json:"netQuantity"`
	NetAmount        float64 `json:"netAmount"`
}

// ---------- safe error (no body, no token) ----------

type ghCopilotHTTPError struct {
	StatusCode int
	Endpoint   string
}

func (e *ghCopilotHTTPError) Error() string {
	return fmt.Sprintf("github-copilot %s: HTTP %d", e.Endpoint, e.StatusCode)
}

// wrapCopilotPermissionHint adds a friendly help message for the four
// "likely permission/availability" statuses while preserving the underlying
// ghCopilotHTTPError via %w. 500-class errors stay raw.
func wrapCopilotPermissionHint(err error) error {
	var he *ghCopilotHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 400, 401, 403, 404:
			return fmt.Errorf("%w: GitHub Copilot billing usage is unavailable. "+
				"Check GITHUB_PAT permissions, GITHUB_ORG, organization admin access, "+
				"and whether premium request billing data is available for this account", err)
		}
	}
	return err
}

// ---------- HTTP plumbing ----------

func (p *GitHubCopilotProvider) fetchDay(ctx context.Context, day time.Time) ([]NormalizedCostRecord, error) {
	day = day.UTC()
	path := githubCopilotPathPrefix + p.Org + githubCopilotPathSuffix
	q := url.Values{}
	q.Set("year", strconv.Itoa(day.Year()))
	q.Set("month", strconv.Itoa(int(day.Month())))
	q.Set("day", strconv.Itoa(day.Day()))

	var resp githubCopilotUsageResponse
	if err := p.doRequest(ctx, path, q, &resp); err != nil {
		return nil, err
	}

	recordedAt := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Format(githubCopilotRecordedLayout)
	out := make([]NormalizedCostRecord, 0, len(resp.UsageItems))
	for _, item := range resp.UsageItems {
		out = append(out, mapCopilotItem(item, p.Org, recordedAt))
	}
	return out, nil
}

func mapCopilotItem(item githubCopilotUsageItem, org, recordedAt string) NormalizedCostRecord {
	quantity := item.NetQuantity
	if quantity == 0 {
		quantity = item.GrossQuantity
	}
	totalTokens := 0
	if strings.EqualFold(item.UnitType, "tokens") {
		totalTokens = int(quantity)
	}
	model := item.Model
	if model == "" {
		if item.SKU != "" {
			model = item.SKU
		} else {
			model = item.Product
		}
	}
	return NormalizedCostRecord{
		Provider:          githubCopilotProviderName,
		Model:             model,
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       totalTokens,
		CostUSD:           item.NetAmount,
		RecordedAt:        recordedAt,
		APIKeyID:          "",
		ProjectID:         org,
		Product:           item.Product,
		SKU:               item.SKU,
		UnitType:          item.UnitType,
		UsageQuantity:     quantity,
		UnitPriceUSD:      item.PricePerUnit,
		GrossAmountUSD:    item.GrossAmount,
		DiscountAmountUSD: item.DiscountAmount,
		SourceHash:        copilotSourceHash(recordedAt, org, item.Product, item.SKU, item.Model, item.UnitType),
	}
}

func (p *GitHubCopilotProvider) doRequest(ctx context.Context, path string, query url.Values, out any) error {
	u, err := url.Parse(p.BaseURL + path)
	if err != nil {
		return fmt.Errorf("github-copilot %s: build url: %w", path, err)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("github-copilot %s: build request: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("X-GitHub-Api-Version", githubCopilotAPIVersion)
	req.Header.Set("User-Agent", p.userAgent())

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("github-copilot %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never read the body — return only status + path.
		return &ghCopilotHTTPError{StatusCode: resp.StatusCode, Endpoint: path}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("github-copilot %s: decode response: %w", path, err)
	}
	return nil
}

func (p *GitHubCopilotProvider) userAgent() string {
	if p.UserAgent == "" {
		return githubCopilotUserAgentDev
	}
	return p.UserAgent
}

// copilotSourceHash is the GitHub-specific identity hash. Inputs include
// the billing dimensions GitHub groups on (product, sku, model, unitType)
// in addition to org and day. Provider name is the prefix, ensuring no
// cross-provider hash collisions.
func copilotSourceHash(recordedAt, org, product, sku, model, unitType string) string {
	h := sha256.New()
	fmt.Fprintf(h, "github-copilot|%s|%s|%s|%s|%s|%s",
		recordedAt, org, product, sku, model, unitType)
	return hex.EncodeToString(h.Sum(nil))
}

func sortCopilotRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].Product != records[j].Product {
			return records[i].Product < records[j].Product
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
