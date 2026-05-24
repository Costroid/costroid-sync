package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---------- HTTP plumbing ----------

func (p *GCPBillingProvider) fetchBillingRows(ctx context.Context, days int) ([]gcpBillingRow, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	sql := buildGCPQuerySQL(p.BillingTable, p.ProjectFilter != "")
	body := buildGCPQueryBody(sql, p.Currency, days, p.ProjectFilter)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("gcp-billing: marshal query body failed")
	}

	first, err := p.doGCPQuery(ctx, token, payload)
	if err != nil {
		return nil, wrapGCPBillingQueryPermissionHint(err)
	}

	rows := decodeGCPBillingRows(first)
	pageToken := first.PageToken
	jobRef := first.JobReference

	for page := 1; page < gcpBQMaxPages && pageToken != "" && jobRef.JobID != ""; page++ {
		next, err := p.doGCPGetResults(ctx, token, jobRef, pageToken)
		if err != nil {
			return nil, wrapGCPBillingQueryPermissionHint(err)
		}
		// Subsequent pages may not re-send the schema; pass through the
		// schema from the first page so decoding stays positional.
		if len(next.Schema.Fields) == 0 {
			next.Schema = first.Schema
		}
		rows = append(rows, decodeGCPBillingRows(next)...)
		pageToken = next.PageToken
	}
	return rows, nil
}

func (p *GCPBillingProvider) doGCPQuery(ctx context.Context, token string, payload []byte) (bqQueryResponse, error) {
	var decoded bqQueryResponse
	endpoint := p.bigQueryBaseURL() + "/bigquery/" + gcpBQAPIVersion + "/projects/" + url.PathEscape(p.BillingProject) + "/queries"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return decoded, errors.New("gcp-billing query: build request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())
	return decoded, p.doGCPHTTP(req, &decoded, "/bigquery/v2/projects/{project}/queries")
}

func (p *GCPBillingProvider) doGCPGetResults(ctx context.Context, token string, jobRef bqJobReference, pageToken string) (bqQueryResponse, error) {
	var decoded bqQueryResponse
	endpoint := p.bigQueryBaseURL() + "/bigquery/" + gcpBQAPIVersion + "/projects/" + url.PathEscape(jobRef.ProjectID) + "/queries/" + url.PathEscape(jobRef.JobID)
	q := url.Values{}
	if jobRef.Location != "" {
		q.Set("location", jobRef.Location)
	}
	q.Set("pageToken", pageToken)
	q.Set("maxResults", strconv.Itoa(gcpBQMaxResults))
	endpoint += "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return decoded, errors.New("gcp-billing query: build request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())
	return decoded, p.doGCPHTTP(req, &decoded, "/bigquery/v2/projects/{project}/queries/{jobId}")
}

func (p *GCPBillingProvider) doGCPHTTP(req *http.Request, out any, endpointForError string) error {
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return errors.New("gcp-billing query: HTTP request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &gcpBillingHTTPError{StatusCode: resp.StatusCode, Endpoint: endpointForError}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errors.New("gcp-billing query: decode response failed")
	}
	return nil
}

// ---------- Row decoding ----------

func decodeGCPBillingRows(resp bqQueryResponse) []gcpBillingRow {
	idx := buildGCPColumnIndex(resp.Schema.Fields)
	out := make([]gcpBillingRow, 0, len(resp.Rows))
	for _, raw := range resp.Rows {
		row, ok := decodeGCPBillingRow(raw, idx)
		if !ok {
			continue
		}
		out = append(out, row)
	}
	return out
}

func buildGCPColumnIndex(fields []bqField) map[string]int {
	idx := map[string]int{}
	for i, f := range fields {
		idx[strings.ToLower(f.Name)] = i
	}
	return idx
}

func decodeGCPBillingRow(row bqRow, idx map[string]int) (gcpBillingRow, bool) {
	getStr := func(name string) string { return bqValueString(row, idx, name) }
	usageAmountRaw := getStr("usage_amount")
	costRaw := getStr("cost")
	usageAmount, _ := strconv.ParseFloat(usageAmountRaw, 64)
	cost, _ := strconv.ParseFloat(costRaw, 64)

	out := gcpBillingRow{
		UsageStartTime:     normalizeBQTimestamp(getStr("usage_start_time")),
		UsageEndTime:       normalizeBQTimestamp(getStr("usage_end_time")),
		ServiceID:          getStr("service_id"),
		ServiceDescription: getStr("service_description"),
		SKUID:              getStr("sku_id"),
		SKUDescription:     getStr("sku_description"),
		ProjectID:          getStr("project_id"),
		ProjectName:        getStr("project_name"),
		Location:           getStr("location_location"),
		Cost:               cost,
		Currency:           getStr("currency"),
		UsageAmount:        usageAmount,
		UsageUnit:          getStr("usage_unit"),
		InvoiceMonth:       getStr("invoice_month"),
		CostType:           getStr("cost_type"),
		RawUsageAmount:     usageAmountRaw,
		RawCost:            costRaw,
	}
	// A row is only useful if it has at least a timestamp; everything else
	// can be optional and still produce a valid (degraded) record.
	if out.UsageStartTime == "" {
		return gcpBillingRow{}, false
	}
	return out, true
}

func bqValueString(row bqRow, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i < 0 || i >= len(row.F) {
		return ""
	}
	var s string
	if err := json.Unmarshal(row.F[i].V, &s); err == nil {
		return s
	}
	return ""
}

// normalizeBQTimestamp accepts the timestamp representations BigQuery is
// known to emit for a CAST(... AS STRING) column and returns an RFC3339
// UTC string, or "" when nothing matches. Conservative — when in doubt,
// the row is dropped upstream rather than guessing.
func normalizeBQTimestamp(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// BigQuery often emits TIMESTAMP as "YYYY-MM-DD HH:MM:SS[.ffffff] UTC".
	if t, ok := parseUsageStart(s); ok {
		return t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return ""
}
