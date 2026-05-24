package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Azure Monitor enrichment for Azure OpenAI cost rows.
//
// JOIN LIMITATION (see README and plan): a single Azure OpenAI resource
// can host multiple model deployments. Cost Management aggregates at the
// meter (model) level; Monitor reports per-resource and optionally per
// deployment. When the cost row's meter-derived model does not uniquely
// identify a Monitor entry for that (date, resourceId), we leave token
// counts at 0 rather than guess. Cost Management cost is always
// authoritative; Monitor only writes into the token fields, never into
// CostUSD.
//
// METADATA ONLY. We request three metric names (ProcessedPromptTokens,
// GeneratedTokens, TotalTokens) and decode only their numeric `total`
// aggregation. No diagnostic logs, no request/response bodies, no chat
// content.

// ---------- Monitor response (narrow decode) ----------

type azureMonitorResponse struct {
	Value []azureMonitorMetric `json:"value"`
}

type azureMonitorMetric struct {
	Name       azureLocalizedName     `json:"name"`
	Timeseries []azureMonitorTimeries `json:"timeseries"`
}

type azureMonitorTimeries struct {
	MetadataValues []azureMonitorMetadata `json:"metadatavalues"`
	Data           []azureMonitorDatum    `json:"data"`
}

type azureMonitorMetadata struct {
	Name  azureLocalizedName `json:"name"`
	Value string             `json:"value"`
}

type azureMonitorDatum struct {
	Timestamp string  `json:"timeStamp"`
	Total     float64 `json:"total"`
}

type azureLocalizedName struct {
	Value string `json:"value"`
}

// ---------- accumulator and key ----------

type azureMonitorKey struct {
	Date       string
	ResourceID string
	Model      string // canonical lowercase; "" when Monitor returned no model dimension
}

type azureTokenTotals struct {
	Prompt        int
	Completion    int
	Total         int
	HasPrompt     bool
	HasCompletion bool
	HasTotal      bool
}

// enrichWithMonitor mutates `records` in place by writing token counts
// onto rows where a Monitor entry can be safely joined. Per-resource
// fetch failures are non-fatal: they're returned in the joined error
// (wrapped) but enrichment proceeds for the other resources. The cost
// records themselves are never dropped.
func enrichWithMonitor(ctx context.Context, p *AzureOpenAIProvider, records []NormalizedCostRecord, start, end time.Time) error {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return err
	}

	byResource := map[azureResourceDayKey][]azureModelTotals{}
	var fetchErrors []error
	for _, resourceID := range p.ResourceIDs {
		entries, err := p.fetchMonitorMetricsForResource(ctx, resourceID, token, start, end)
		if err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("azure-openai monitor %s: %w", resourceID, err))
			continue
		}
		for k, totals := range entries {
			daykey := azureResourceDayKey{Date: k.Date, ResourceID: k.ResourceID}
			byResource[daykey] = append(byResource[daykey], azureModelTotals{Model: k.Model, Totals: totals})
		}
	}

	for i := range records {
		applyMonitorTokens(&records[i], byResource)
	}

	if len(fetchErrors) > 0 {
		return joinAzureErrors(fetchErrors)
	}
	return nil
}

type azureResourceDayKey struct {
	Date       string
	ResourceID string
}

type azureModelTotals struct {
	Model  string
	Totals azureTokenTotals
}

// applyMonitorTokens implements the safe-join rules from the plan.
//
//	exact (date+resourceId+model match) → populate
//	resource-level (date+resourceId, exactly one Monitor candidate) → populate
//	ambiguous (multiple candidates, no exact model match) → leave at 0
func applyMonitorTokens(r *NormalizedCostRecord, byResource map[azureResourceDayKey][]azureModelTotals) {
	candidates := byResource[azureResourceDayKey{Date: r.RecordedAt, ResourceID: r.ProjectID}]
	if len(candidates) == 0 {
		return
	}

	var match *azureModelTotals
	if r.Model != "" {
		for j := range candidates {
			if strings.EqualFold(candidates[j].Model, r.Model) {
				match = &candidates[j]
				break
			}
		}
	}
	if match == nil && len(candidates) == 1 {
		match = &candidates[0]
	}
	if match == nil {
		return
	}

	t := match.Totals
	if t.HasPrompt {
		r.PromptTokens = t.Prompt
	}
	if t.HasCompletion {
		r.CompletionTokens = t.Completion
	}
	switch {
	case t.HasTotal:
		r.TotalTokens = t.Total
	case t.HasPrompt || t.HasCompletion:
		r.TotalTokens = r.PromptTokens + r.CompletionTokens
	}
}

// fetchMonitorMetricsForResource issues one GET to the Azure Monitor
// metrics endpoint scoped to the resource and returns a map of
// (date, resourceId, model) → totals. The model field is empty when
// Monitor didn't return a ModelDeploymentName / ModelName dimension.
func (p *AzureOpenAIProvider) fetchMonitorMetricsForResource(ctx context.Context, resourceID, token string, start, end time.Time) (map[azureMonitorKey]azureTokenTotals, error) {
	cleanResource := "/" + strings.TrimPrefix(resourceID, "/")
	path := cleanResource + "/providers/Microsoft.Insights/metrics"
	endpoint := strings.TrimRight(p.mgmtBaseURL(), "/") + path

	q := url.Values{}
	q.Set("api-version", azureMonitorAPIVersion)
	q.Set("metricnames", "ProcessedPromptTokens,GeneratedTokens,TotalTokens")
	q.Set("aggregation", "Total")
	q.Set("interval", "P1D")
	q.Set("timespan", start.UTC().Format("2006-01-02T15:04:05Z")+"/"+end.UTC().Format("2006-01-02T15:04:05Z"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, wrapAzureManagementPermissionHint(&azureHTTPError{
			StatusCode: resp.StatusCode,
			Endpoint:   "/providers/Microsoft.Insights/metrics",
		})
	}

	var body azureMonitorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return aggregateMonitorBody(body, resourceID), nil
}

// aggregateMonitorBody walks a Monitor response and produces a
// (date, resourceId, model) → totals map. Multiple deployments with the
// same model name on the same day SUM together (so the cost row sees
// the full attribution).
func aggregateMonitorBody(body azureMonitorResponse, resourceID string) map[azureMonitorKey]azureTokenTotals {
	out := map[azureMonitorKey]azureTokenTotals{}
	for _, metric := range body.Value {
		metricName := metric.Name.Value
		for _, ts := range metric.Timeseries {
			model := strings.ToLower(extractMonitorModel(ts.MetadataValues))
			for _, datum := range ts.Data {
				date := canonicalMonitorDate(datum.Timestamp)
				if date == "" {
					continue
				}
				key := azureMonitorKey{Date: date, ResourceID: resourceID, Model: model}
				totals := out[key]
				addMetricValue(&totals, metricName, datum.Total)
				out[key] = totals
			}
		}
	}
	return out
}

func extractMonitorModel(meta []azureMonitorMetadata) string {
	var model string
	for _, m := range meta {
		switch strings.ToLower(m.Name.Value) {
		case "modelname":
			if m.Value != "" {
				return m.Value
			}
		case "modeldeploymentname":
			if model == "" {
				model = m.Value
			}
		}
	}
	return model
}

func addMetricValue(t *azureTokenTotals, metricName string, value float64) {
	v := int(value)
	switch metricName {
	case "ProcessedPromptTokens":
		t.Prompt += v
		t.HasPrompt = true
	case "GeneratedTokens":
		t.Completion += v
		t.HasCompletion = true
	case "TotalTokens":
		t.Total += v
		t.HasTotal = true
	}
}

func canonicalMonitorDate(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ""
	}
	return time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC).Format(azureRecordedLayout)
}

// joinAzureErrors wraps a non-empty list of errors into a single error
// whose message lists each underlying message on its own line. errors.As
// against the first child still works (others are accessible via
// errors.Unwrap iteration).
func joinAzureErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return fmt.Errorf("azure-openai monitor: %s", strings.Join(parts, "; "))
}
