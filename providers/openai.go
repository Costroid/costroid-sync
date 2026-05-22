package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultOpenAIBaseURL = "https://api.openai.com"

const (
	openaiUsagePath = "/v1/organization/usage/completions"
	openaiCostsPath = "/v1/organization/costs"
)

// OpenAIProvider fetches metadata-only usage from the OpenAI Usage and Cost
// APIs. It NEVER reads or stores prompt, completion, message, or any other
// user-generated text — only counts, identifiers, model names, and cost
// amounts. See providers/types.go for the metadata-only rule.
type OpenAIProvider struct {
	AdminKey   string
	BaseURL    string
	HTTPClient *http.Client
}

var _ Provider = (*OpenAIProvider)(nil)

func NewOpenAIProvider(adminKey string) *OpenAIProvider {
	return &OpenAIProvider{
		AdminKey:   adminKey,
		BaseURL:    DefaultOpenAIBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	now := time.Now().UTC()
	end := now.Unix()
	start := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()

	// OpenAI caps page size at 31 buckets when bucket_width=1d.
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
	return joinProportional(usage, costs), nil
}

// ---------- narrow decode types (metadata-only by construction) ----------

type openaiUsagePage struct {
	Data     []openaiUsageBucket `json:"data"`
	HasMore  bool                `json:"has_more"`
	NextPage string              `json:"next_page"`
}

type openaiUsageBucket struct {
	StartTime int64               `json:"start_time"`
	EndTime   int64               `json:"end_time"`
	Results   []openaiUsageResult `json:"results"`
}

// openaiUsageResult deliberately omits any field that could carry prompt,
// completion, message, content, tool_call, raw_response, system_prompt,
// function_args, request_body, response_body, or raw_payload data.
// Unmapped JSON fields are discarded by encoding/json.
type openaiUsageResult struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	ProjectID    string `json:"project_id"`
	APIKeyID     string `json:"api_key_id"`
	Model        string `json:"model"`
}

type openaiCostPage struct {
	Data     []openaiCostBucket `json:"data"`
	HasMore  bool               `json:"has_more"`
	NextPage string             `json:"next_page"`
}

type openaiCostBucket struct {
	StartTime int64              `json:"start_time"`
	EndTime   int64              `json:"end_time"`
	Results   []openaiCostResult `json:"results"`
}

type openaiCostResult struct {
	Amount    openaiAmount `json:"amount"`
	LineItem  string       `json:"line_item"`
	ProjectID string       `json:"project_id"`
}

type openaiAmount struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// ---------- safe error (no body, no key) ----------

type httpError struct {
	StatusCode int
	Endpoint   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("openai %s: HTTP %d", e.Endpoint, e.StatusCode)
}

// ---------- HTTP plumbing ----------

func (p *OpenAIProvider) doRequest(ctx context.Context, path string, query url.Values, out any) error {
	u, err := url.Parse(p.BaseURL + path)
	if err != nil {
		return fmt.Errorf("openai %s: build url: %w", path, err)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("openai %s: build request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+p.AdminKey)
	req.Header.Set("Accept", "application/json")

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("openai %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpError{StatusCode: resp.StatusCode, Endpoint: path}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("openai %s: decode response: %w", path, err)
	}
	return nil
}

func (p *OpenAIProvider) fetchUsagePages(ctx context.Context, startTime, endTime int64, limit int) ([]openaiUsageBucket, error) {
	var all []openaiUsageBucket
	nextPage := ""
	for {
		q := url.Values{}
		q.Set("start_time", strconv.FormatInt(startTime, 10))
		q.Set("end_time", strconv.FormatInt(endTime, 10))
		q.Set("bucket_width", "1d")
		q.Set("limit", strconv.Itoa(limit))
		q.Add("group_by", "project_id")
		q.Add("group_by", "api_key_id")
		q.Add("group_by", "model")
		if nextPage != "" {
			q.Set("page", nextPage)
		}

		var page openaiUsagePage
		if err := p.doRequest(ctx, openaiUsagePath, q, &page); err != nil {
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

func (p *OpenAIProvider) fetchCostPages(ctx context.Context, startTime, endTime int64, limit int) ([]openaiCostBucket, error) {
	var all []openaiCostBucket
	nextPage := ""
	for {
		q := url.Values{}
		q.Set("start_time", strconv.FormatInt(startTime, 10))
		q.Set("end_time", strconv.FormatInt(endTime, 10))
		q.Set("bucket_width", "1d")
		q.Set("limit", strconv.Itoa(limit))
		q.Add("group_by", "project_id")
		q.Add("group_by", "line_item")
		if nextPage != "" {
			q.Set("page", nextPage)
		}

		var page openaiCostPage
		if err := p.doRequest(ctx, openaiCostsPath, q, &page); err != nil {
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

// ---------- join logic ----------

// extractModel returns the model name from a cost line_item like
// "gpt-4o, input". Returns the trimmed prefix before the first comma.
func extractModel(lineItem string) string {
	if i := strings.Index(lineItem, ","); i >= 0 {
		return strings.TrimSpace(lineItem[:i])
	}
	return strings.TrimSpace(lineItem)
}

type groupKey struct {
	StartTime int64
	ProjectID string
	Model     string
}

// joinProportional merges usage + cost buckets into NormalizedCostRecords
// using token-share proportional allocation:
//   - Group usage by (bucket, project, api_key, model).
//   - Group costs by (bucket, project, model) — costs API has no api_key.
//   - Each cost line item is distributed across matching api_keys in
//     proportion to that key's token share within the (bucket, project,
//     model) group.
//
// Limitations:
//   - Cost line items with no matching usage are silently dropped.
//   - Groups with totalTokens=0 cannot allocate cost (CostUSD=0).
//   - Non-USD line items skipped.
func joinProportional(usage []openaiUsageBucket, costs []openaiCostBucket) []NormalizedCostRecord {
	costsByGroup := map[groupKey]float64{}
	for _, b := range costs {
		for _, r := range b.Results {
			if !strings.EqualFold(r.Amount.Currency, "usd") {
				continue
			}
			model := extractModel(r.LineItem)
			if model == "" {
				continue
			}
			k := groupKey{StartTime: b.StartTime, ProjectID: r.ProjectID, Model: model}
			costsByGroup[k] += r.Amount.Value
		}
	}

	type entry struct {
		bucket int64
		result openaiUsageResult
	}
	usageByGroup := map[groupKey][]entry{}
	for _, b := range usage {
		for _, r := range b.Results {
			k := groupKey{StartTime: b.StartTime, ProjectID: r.ProjectID, Model: r.Model}
			usageByGroup[k] = append(usageByGroup[k], entry{bucket: b.StartTime, result: r})
		}
	}

	var out []NormalizedCostRecord
	for k, entries := range usageByGroup {
		var totalTokens int
		for _, e := range entries {
			totalTokens += e.result.InputTokens + e.result.OutputTokens
		}
		groupCost := costsByGroup[k]

		for _, e := range entries {
			rTokens := e.result.InputTokens + e.result.OutputTokens
			cost := 0.0
			if totalTokens > 0 {
				cost = groupCost * float64(rTokens) / float64(totalTokens)
			}
			recordedAt := time.Unix(e.bucket, 0).UTC().Format(time.RFC3339)
			out = append(out, NormalizedCostRecord{
				Provider:         "openai",
				Model:            e.result.Model,
				PromptTokens:     e.result.InputTokens,
				CompletionTokens: e.result.OutputTokens,
				TotalTokens:      rTokens,
				CostUSD:          cost,
				RecordedAt:       recordedAt,
				APIKeyID:         e.result.APIKeyID,
				ProjectID:        e.result.ProjectID,
				SourceHash:       ComputeSourceHash("openai", recordedAt, e.result.Model, e.result.ProjectID, e.result.APIKeyID),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].RecordedAt != out[j].RecordedAt {
			return out[i].RecordedAt < out[j].RecordedAt
		}
		if out[i].ProjectID != out[j].ProjectID {
			return out[i].ProjectID < out[j].ProjectID
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].APIKeyID < out[j].APIKeyID
	})
	return out
}
