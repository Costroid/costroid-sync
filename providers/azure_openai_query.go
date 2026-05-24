package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OAuth token exchange lives in azure_openai_auth.go.

// ---------- Cost Management query ----------

func (p *AzureOpenAIProvider) fetchCostRows(ctx context.Context, start, end time.Time) ([]azureCostRow, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	scope := normalizeAzureScope(p.Scope)
	path := "/" + scope + "/providers/Microsoft.CostManagement/query"
	endpoint := strings.TrimRight(p.mgmtBaseURL(), "/") + path
	q := url.Values{}
	q.Set("api-version", azureCostAPIVersion)
	endpointWithQuery := endpoint + "?" + q.Encode()

	body := buildAzureCostQueryBody(start, end)

	var (
		out     []azureCostRow
		nextURL = endpointWithQuery
		pages   = 0
		queryB  []byte
	)
	queryB, err = json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("azure-openai cost: marshal body: %w", err)
	}

	for nextURL != "" && pages < azureMaxPages {
		pages++
		page, link, err := p.doCostQuery(ctx, nextURL, queryB, token)
		if err != nil {
			return nil, wrapAzureManagementPermissionHint(err)
		}
		out = append(out, page...)
		// Cost Management nextLink is a fully-qualified URL with its own
		// api-version and continuation token; POST body for subsequent
		// pages remains the same (per Azure docs).
		nextURL = link
	}
	return out, nil
}

func (p *AzureOpenAIProvider) doCostQuery(ctx context.Context, rawURL string, body []byte, token string) ([]azureCostRow, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("azure-openai cost: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("azure-openai cost: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, "", &azureHTTPError{
			StatusCode: resp.StatusCode,
			Endpoint:   "/providers/Microsoft.CostManagement/query",
		}
	}

	var decoded azureCostQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, "", fmt.Errorf("azure-openai cost: decode response: %w", err)
	}

	rows := decodeAzureCostRows(decoded)
	nextLink := decoded.Properties.NextLink
	if nextLink == "" {
		nextLink = decoded.NextLink
	}
	return rows, nextLink, nil
}

// buildAzureCostQueryBody returns the JSON body for the Cost Management
// Query API. Filtered server-side to MeterCategory IN
// {"Azure OpenAI", "Cognitive Services"} so the request doesn't return
// rows we'd just discard locally.
func buildAzureCostQueryBody(start, end time.Time) map[string]any {
	return map[string]any{
		"type":      "ActualCost",
		"timeframe": "Custom",
		"timePeriod": map[string]string{
			"from": start.UTC().Format("2006-01-02T15:04:05Z"),
			"to":   end.UTC().Format("2006-01-02T15:04:05Z"),
		},
		"dataset": map[string]any{
			"granularity": "Daily",
			"aggregation": map[string]any{
				"totalCost":  map[string]string{"name": "Cost", "function": "Sum"},
				"totalUsage": map[string]string{"name": "UsageQuantity", "function": "Sum"},
			},
			"grouping": []map[string]string{
				{"type": "Dimension", "name": "ResourceId"},
				{"type": "Dimension", "name": "ResourceGroup"},
				{"type": "Dimension", "name": "ServiceName"},
				{"type": "Dimension", "name": "Meter"},
				{"type": "Dimension", "name": "MeterCategory"},
				{"type": "Dimension", "name": "MeterSubCategory"},
				{"type": "Dimension", "name": "UnitOfMeasure"},
			},
			"filter": map[string]any{
				"dimensions": map[string]any{
					"name":     "MeterCategory",
					"operator": "In",
					"values":   []string{"Azure OpenAI", "Cognitive Services"},
				},
			},
		},
	}
}

// decodeAzureCostRows pulls typed values out of the columns/rows envelope
// returned by Cost Management. Column lookup is case-insensitive (the
// API has been seen to vary between camelCase and PascalCase across
// versions). USD-only and conservative-service-name filters are applied
// at this layer.
func decodeAzureCostRows(resp azureCostQueryResponse) []azureCostRow {
	col := buildAzureColumnIndex(resp.Properties.Columns)
	out := make([]azureCostRow, 0, len(resp.Properties.Rows))
	for _, raw := range resp.Properties.Rows {
		row := decodeAzureSingleRow(raw, col)
		if row.Date == "" {
			continue
		}
		if row.Currency != "" && !strings.EqualFold(row.Currency, "USD") {
			continue
		}
		if !isAzureServiceAllowed(row.ServiceName, row.MeterCategory) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func buildAzureColumnIndex(columns []struct {
	Name string `json:"name"`
	Type string `json:"type"`
}) map[string]int {
	idx := map[string]int{}
	for i, c := range columns {
		idx[strings.ToLower(c.Name)] = i
	}
	return idx
}

func decodeAzureSingleRow(raw []json.RawMessage, col map[string]int) azureCostRow {
	getStr := func(name string) string { return rowString(raw, col, name) }
	getNum := func(name string) float64 { return rowFloat(raw, col, name) }

	dateInt := int(getNum("usagedate"))
	if dateInt == 0 {
		// Some API versions name it "UsageDateTime" as a string.
		if s := getStr("usagedatetime"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				dateInt, _ = strconv.Atoi(t.UTC().Format("20060102"))
			}
		}
	}
	return azureCostRow{
		Date:             formatAzureDate(dateInt),
		Cost:             firstNonZero(getNum("totalcost"), getNum("cost"), getNum("pretaxcost")),
		Currency:         getStr("currency"),
		UsageQuantity:    firstNonZero(getNum("totalusage"), getNum("usagequantity")),
		ResourceID:       getStr("resourceid"),
		ResourceGroup:    getStr("resourcegroup"),
		ServiceName:      getStr("servicename"),
		Meter:            getStr("meter"),
		MeterCategory:    getStr("metercategory"),
		MeterSubCategory: getStr("metersubcategory"),
		UnitOfMeasure:    getStr("unitofmeasure"),
	}
}

func rowString(raw []json.RawMessage, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok || i < 0 || i >= len(raw) {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw[i], &s); err == nil {
		return s
	}
	return ""
}

func rowFloat(raw []json.RawMessage, col map[string]int, name string) float64 {
	i, ok := col[name]
	if !ok || i < 0 || i >= len(raw) {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw[i], &f); err == nil {
		return f
	}
	return 0
}

func firstNonZero(values ...float64) float64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// isAzureServiceAllowed is the defense-in-depth check that runs even
// after the server-side filter narrows MeterCategory. Accepts a row
// whose ServiceName OR MeterCategory case-insensitively matches one of
// the allowed filters.
func isAzureServiceAllowed(serviceName, meterCategory string) bool {
	candidates := []string{strings.ToLower(serviceName), strings.ToLower(meterCategory)}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		for _, f := range azureServiceFilters {
			if strings.Contains(c, f) {
				return true
			}
		}
	}
	return false
}

// ---------- common HTTP helpers ----------

func (p *AzureOpenAIProvider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (p *AzureOpenAIProvider) userAgent() string {
	if p.UserAgent != "" {
		return p.UserAgent
	}
	return azureUserAgentDev
}

func (p *AzureOpenAIProvider) tokenBaseURL() string {
	if p.TokenBaseURL != "" {
		return p.TokenBaseURL
	}
	return azureDefaultTokenBaseURL
}

func (p *AzureOpenAIProvider) mgmtBaseURL() string {
	if p.ManagementBaseURL != "" {
		return p.ManagementBaseURL
	}
	return azureDefaultMgmtBaseURL
}
