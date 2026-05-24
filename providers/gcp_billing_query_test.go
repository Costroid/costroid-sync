package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------- validateGCPTableID ----------

func TestValidateGCPTableID_Rejects(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"semicolon_injection", "my-proj.ds.tbl; DROP TABLE x"},
		{"backtick", "my-proj.ds.`tbl`"},
		{"whitespace", "my-proj. ds.tbl"},
		{"line_comment", "my-proj.ds.tbl --comment"},
		{"block_comment_open", "my-proj.ds./*x*/tbl"},
		{"too_few_segments", "my-proj.ds"},
		{"too_many_segments", "my-proj.ds.tbl.extra"},
		{"newline", "my-proj.ds.tbl\n"},
		{"backslash", "my-proj.ds.tbl\\x"},
		{"uppercase_project", "MyProj.ds.tbl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateGCPTableID(tc.input); err == nil {
				t.Fatalf("validateGCPTableID(%q) returned nil, want error", tc.input)
			}
		})
	}
}

func TestValidateGCPTableID_Accepts(t *testing.T) {
	cases := []string{
		"my-proj.billing_export_data.gcp_billing_export_v1_0123ABCDEF",
		"abcdef.dataset.table",
		"long-project-name.ds_with_underscore.t",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if err := validateGCPTableID(in); err != nil {
				t.Fatalf("validateGCPTableID(%q) returned %v, want nil", in, err)
			}
		})
	}
}

// ---------- buildGCPQuerySQL ----------

func TestBuildGCPQuerySQL_OnlySafeColumns(t *testing.T) {
	sql := buildGCPQuerySQL(gcpTestTable, false)

	// Reject any reference to the forbidden columns. This is the primary
	// metadata-only enforcement at the SQL layer.
	for _, bad := range []string{"labels", "system_labels", "tags", "credits", "adjustment_info", "SELECT *", "select *"} {
		if strings.Contains(sql, bad) {
			t.Errorf("SQL contains forbidden token %q: %s", bad, sql)
		}
	}

	// Sanity: every expected safe column is present.
	for _, want := range []string{
		"usage_start_time", "usage_end_time",
		"service.id", "service.description",
		"sku.id", "sku.description",
		"project.id", "project.name",
		"location.location",
		"cost", "currency",
		"usage.amount", "usage.unit",
		"invoice.month", "cost_type",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing safe column %q: %s", want, sql)
		}
	}

	// Filters and parameter placeholders.
	for _, want := range []string{
		"currency = @currency",
		"DATE(usage_start_time)",
		"DATE_SUB(CURRENT_DATE(), INTERVAL @days DAY)",
		"FROM `" + gcpTestTable + "`",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q: %s", want, sql)
		}
	}

	// No project_filter clause when withProjectFilter=false.
	if strings.Contains(sql, "@project_filter") {
		t.Errorf("SQL has @project_filter when filter disabled: %s", sql)
	}
}

func TestBuildGCPQuerySQL_WithProjectFilter(t *testing.T) {
	sql := buildGCPQuerySQL(gcpTestTable, true)
	if !strings.Contains(sql, "AND project.id = @project_filter") {
		t.Errorf("SQL missing project filter clause: %s", sql)
	}
}

// ---------- buildGCPQueryBody ----------

func TestBuildGCPQueryBody_Parameters(t *testing.T) {
	body := buildGCPQueryBody("SELECT 1", "USD", 30, "myproj")
	if body["useLegacySql"] != false {
		t.Errorf("useLegacySql = %v, want false", body["useLegacySql"])
	}
	if body["parameterMode"] != "NAMED" {
		t.Errorf("parameterMode = %v, want NAMED", body["parameterMode"])
	}

	params, ok := body["queryParameters"].([]map[string]any)
	if !ok {
		t.Fatalf("queryParameters wrong type: %T", body["queryParameters"])
	}
	if len(params) != 3 {
		t.Fatalf("len(params) = %d, want 3 (currency, days, project_filter)", len(params))
	}
	names := []string{}
	for _, p := range params {
		names = append(names, p["name"].(string))
	}
	for _, want := range []string{"currency", "days", "project_filter"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("param %q missing; got %v", want, names)
		}
	}
}

func TestBuildGCPQueryBody_NoProjectFilter(t *testing.T) {
	body := buildGCPQueryBody("SELECT 1", "USD", 7, "")
	params := body["queryParameters"].([]map[string]any)
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2 (currency, days only)", len(params))
	}
}

// ---------- Pagination ----------

func TestGCPBillingQuery_Pagination(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
		  "jobReference": {"projectId":"test-project","jobId":"job_paged","location":"US"},
		  "jobComplete": true,
		  "pageToken": "PAGE_2",
		  "schema": {"fields": [
		    {"name":"usage_start_time","type":"STRING"},
		    {"name":"cost","type":"STRING"},
		    {"name":"currency","type":"STRING"}
		  ]},
		  "rows": [
		    {"f":[{"v":"2026-05-20 12:00:00 UTC"},{"v":"1.00"},{"v":"USD"}]}
		  ]
		}`))
	}
	ts.getResHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
		  "jobComplete": true,
		  "rows": [
		    {"f":[{"v":"2026-05-20 13:00:00 UTC"},{"v":"2.00"},{"v":"USD"}]}
		  ]
		}`))
	}

	p := newGCPBillingTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := len(records); got != 2 {
		t.Fatalf("want 2 records across two pages, got %d", got)
	}

	if got := atomic.LoadInt32(&ts.getResCalls); got != 1 {
		t.Errorf("getResults called %d times, want 1", got)
	}

	// The getResults call must carry the pageToken from page 1.
	if got := ts.capturedGetRes[0].Query; !strings.Contains(got, "pageToken=PAGE_2") {
		t.Errorf("getResults query = %q, missing pageToken", got)
	}
	if !strings.Contains(ts.capturedGetRes[0].Path, "/job_paged") {
		t.Errorf("getResults path = %q, missing jobId", ts.capturedGetRes[0].Path)
	}
}

// ---------- Request body composition ----------

func TestGCPBillingQuery_RequestBodyComposition(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	p.ProjectFilter = "filter-me"
	p.Currency = "USD"

	if _, err := p.Fetch(context.Background(), 14); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ts.capturedQuery) != 1 {
		t.Fatalf("want 1 query call, got %d", len(ts.capturedQuery))
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(ts.capturedQuery[0].Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	sql, _ := body["query"].(string)
	for _, bad := range []string{"labels", "system_labels"} {
		if strings.Contains(sql, bad) {
			t.Errorf("query SQL contains forbidden column %q", bad)
		}
	}
	if !strings.Contains(sql, "@project_filter") {
		t.Errorf("query SQL missing project filter: %s", sql)
	}

	// Endpoint path must include the GCP_BILLING_PROJECT, NOT the table.
	if !strings.HasSuffix(ts.capturedQuery[0].Path, "/projects/"+gcpTestProject+"/queries") {
		t.Errorf("query path = %q", ts.capturedQuery[0].Path)
	}
}

// ---------- Missing optional fields ----------

func TestGCPBillingQuery_MissingOptionalFieldsNoPanic(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
		// Schema with the minimum columns only; project_id, sku_*, etc. absent.
		_, _ = w.Write([]byte(`{
		  "jobComplete": true,
		  "schema": {"fields": [
		    {"name":"usage_start_time","type":"STRING"},
		    {"name":"cost","type":"STRING"},
		    {"name":"currency","type":"STRING"}
		  ]},
		  "rows": [
		    {"f":[{"v":"2026-05-20 12:00:00 UTC"},{"v":"0.50"},{"v":"USD"}]}
		  ]
		}`))
	}
	p := newGCPBillingTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	if records[0].ProjectID != "" {
		t.Errorf("ProjectID should be empty when missing: %q", records[0].ProjectID)
	}
	if records[0].CostUSD != 0.5 {
		t.Errorf("CostUSD = %v", records[0].CostUSD)
	}
}

// ---------- Query permission hint wrapping ----------

func TestGCPBillingQuery_PermissionHint(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ts := newGCPBillingTestServer(t)
			ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"POISON_QUERY_BODY"}`))
			}
			p := newGCPBillingTestProvider(t, ts)
			_, err := p.Fetch(context.Background(), 7)
			if err == nil {
				t.Fatal("want error")
			}
			msg := err.Error()
			if strings.Contains(msg, "POISON_QUERY_BODY") {
				t.Errorf("error leaked response body: %s", msg)
			}
			hasHint := strings.Contains(msg, "BigQuery REST request failed")
			if status == 500 && hasHint {
				t.Errorf("500 should not include permission hint: %s", msg)
			}
			if status != 500 && !hasHint {
				t.Errorf("status %d should include permission hint: %s", status, msg)
			}
		})
	}
}
