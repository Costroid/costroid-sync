package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const DefaultAnthropicBaseURL = "https://api.anthropic.com"

const (
	anthropicUsagePath     = "/v1/organizations/usage_report/messages"
	anthropicCostPath      = "/v1/organizations/cost_report"
	anthropicAPIVersion    = "2023-06-01"
	anthropicUserAgentDev  = "costroid/dev"
	anthropicProviderName  = "anthropic"
	anthropicBucketWidth1d = "1d"
)

var errInvalidAnthropicAmount = errors.New("invalid cost amount")

type AnthropicProvider struct {
	AdminKey   string
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

var _ Provider = (*AnthropicProvider)(nil)

func init() {
	Register(Registration{
		Name:   anthropicProviderName,
		EnvVar: "ANTHROPIC_ADMIN_KEY",
		MissingEnvHelp: "ANTHROPIC_ADMIN_KEY is not set.\n" +
			"Create an Anthropic Admin API key in the Claude Console, then:\n" +
			"  export ANTHROPIC_ADMIN_KEY=sk-ant-admin-...",
		New: func(adminKey string) Provider {
			return NewAnthropicProvider(adminKey)
		},
	})
}

func NewAnthropicProvider(adminKey string) *AnthropicProvider {
	return &AnthropicProvider{
		AdminKey:   adminKey,
		BaseURL:    DefaultAnthropicBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  anthropicUserAgentDev,
	}
}

func (p *AnthropicProvider) Name() string { return anthropicProviderName }

func (p *AnthropicProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	now := time.Now().UTC().Truncate(time.Second)
	lookbackDays := days
	if lookbackDays < 1 {
		lookbackDays = 1
	}
	start := now.Add(-time.Duration(lookbackDays) * 24 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)
	limit := days
	if limit > 31 {
		limit = 31
	}
	if limit < 1 {
		limit = 1
	}

	usage, err := p.fetchUsagePages(ctx, start, end, limit)
	if err != nil {
		return nil, err
	}
	costs, err := p.fetchCostPages(ctx, start, end, limit)
	if err != nil {
		return nil, err
	}
	return joinAnthropicProportional(usage, costs)
}

type anthropicUsagePage struct {
	Data     []anthropicUsageBucket `json:"data"`
	HasMore  bool                   `json:"has_more"`
	NextPage string                 `json:"next_page"`
}

type anthropicUsageBucket struct {
	StartingAt string                 `json:"starting_at"`
	EndingAt   string                 `json:"ending_at"`
	Results    []anthropicUsageResult `json:"results"`
}

type anthropicUsageResult struct {
	UncachedInputTokens int                    `json:"uncached_input_tokens"`
	CacheCreation       anthropicCacheCreation `json:"cache_creation"`
	CacheReadTokens     int                    `json:"cache_read_input_tokens"`
	OutputTokens        int                    `json:"output_tokens"`
	APIKeyID            string                 `json:"api_key_id"`
	WorkspaceID         string                 `json:"workspace_id"`
	Model               string                 `json:"model"`
}

type anthropicCacheCreation struct {
	Ephemeral1hTokens int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mTokens int `json:"ephemeral_5m_input_tokens"`
}

func (r anthropicUsageResult) inputTokens() int {
	return r.UncachedInputTokens + r.CacheCreation.Ephemeral1hTokens +
		r.CacheCreation.Ephemeral5mTokens + r.CacheReadTokens
}

type anthropicCostPage struct {
	Data     []anthropicCostBucket `json:"data"`
	HasMore  bool                  `json:"has_more"`
	NextPage string                `json:"next_page"`
}

type anthropicCostBucket struct {
	StartingAt string                `json:"starting_at"`
	EndingAt   string                `json:"ending_at"`
	Results    []anthropicCostResult `json:"results"`
}

type anthropicCostResult struct {
	Currency    string `json:"currency"`
	Amount      string `json:"amount"`
	WorkspaceID string `json:"workspace_id"`
	Description string `json:"description"`
	Model       string `json:"model"`
}

type anthropicHTTPError struct {
	StatusCode int
	Endpoint   string
}

func (e *anthropicHTTPError) Error() string {
	return fmt.Sprintf("anthropic %s: HTTP %d", e.Endpoint, e.StatusCode)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, path string, query url.Values, out any) error {
	u, err := url.Parse(p.BaseURL + path)
	if err != nil {
		return fmt.Errorf("anthropic %s: build url: %w", path, err)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("anthropic %s: build request: %w", path, err)
	}
	req.Header.Set("x-api-key", p.AdminKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &anthropicHTTPError{StatusCode: resp.StatusCode, Endpoint: path}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("anthropic %s: decode response: %w", path, err)
	}
	return nil
}

func (p *AnthropicProvider) userAgent() string {
	if p.UserAgent == "" {
		return anthropicUserAgentDev
	}
	return p.UserAgent
}

func (p *AnthropicProvider) fetchUsagePages(ctx context.Context, start, end string, limit int) ([]anthropicUsageBucket, error) {
	var all []anthropicUsageBucket
	nextPage := ""
	for {
		q := anthropicBaseQuery(start, end, limit, nextPage)
		q.Add("group_by[]", "model")
		q.Add("group_by[]", "workspace_id")
		q.Add("group_by[]", "api_key_id")

		var page anthropicUsagePage
		if err := p.doRequest(ctx, anthropicUsagePath, q, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if !page.HasMore || page.NextPage == "" {
			break
		}
		nextPage = page.NextPage
	}
	return all, nil
}

func (p *AnthropicProvider) fetchCostPages(ctx context.Context, start, end string, limit int) ([]anthropicCostBucket, error) {
	var all []anthropicCostBucket
	nextPage := ""
	for {
		q := anthropicBaseQuery(start, end, limit, nextPage)
		q.Add("group_by[]", "workspace_id")
		q.Add("group_by[]", "description")

		var page anthropicCostPage
		if err := p.doRequest(ctx, anthropicCostPath, q, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if !page.HasMore || page.NextPage == "" {
			break
		}
		nextPage = page.NextPage
	}
	return all, nil
}

func anthropicBaseQuery(start, end string, limit int, nextPage string) url.Values {
	q := url.Values{}
	q.Set("starting_at", start)
	q.Set("ending_at", end)
	q.Set("bucket_width", anthropicBucketWidth1d)
	q.Set("limit", strconv.Itoa(limit))
	if nextPage != "" {
		q.Set("page", nextPage)
	}
	return q
}
