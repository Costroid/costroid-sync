package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testAdminKey = "sk-admin-SECRET-12345"

func newTestProvider(t *testing.T, h http.HandlerFunc) *OpenAIProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &OpenAIProvider{
		AdminKey:   testAdminKey,
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ---------- pure-helper tests ----------

func TestComputeSourceHash_Deterministic(t *testing.T) {
	a := ComputeSourceHash("openai", "2026-05-22T00:00:00Z", "gpt-4o", "proj_1", "key_a")
	b := ComputeSourceHash("openai", "2026-05-22T00:00:00Z", "gpt-4o", "proj_1", "key_a")
	if a != b {
		t.Fatalf("same inputs -> different hashes: %s vs %s", a, b)
	}
	if a == ComputeSourceHash("openai", "2026-05-22T00:00:00Z", "gpt-4o-mini", "proj_1", "key_a") {
		t.Error("different model collided")
	}
	if a == ComputeSourceHash("openai", "2026-05-22T00:00:00Z", "gpt-4o", "proj_1", "key_b") {
		t.Error("different api_key collided")
	}
	if a == ComputeSourceHash("openai", "2026-05-23T00:00:00Z", "gpt-4o", "proj_1", "key_a") {
		t.Error("different recorded_at collided")
	}
}

func TestExtractModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"gpt-4o, input", "gpt-4o"},
		{"claude-3-opus, cached", "claude-3-opus"},
		{"", ""},
		{"weird", "weird"},
		{"gpt-4o", "gpt-4o"},
		{"  spaced , output", "spaced"},
	}
	for _, c := range cases {
		if got := extractModel(c.in); got != c.want {
			t.Errorf("extractModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJoinProportional_HappyPath(t *testing.T) {
	usage := []openaiUsageBucket{{
		StartTime: 1715000000,
		Results: []openaiUsageResult{
			{InputTokens: 80, OutputTokens: 20, ProjectID: "proj_1", APIKeyID: "key_a", Model: "gpt-4o"},
			{InputTokens: 160, OutputTokens: 40, ProjectID: "proj_1", APIKeyID: "key_b", Model: "gpt-4o"},
		},
	}}
	costs := []openaiCostBucket{{
		StartTime: 1715000000,
		Results: []openaiCostResult{
			{Amount: openaiAmount{Value: 0.30, Currency: "usd"}, LineItem: "gpt-4o, input", ProjectID: "proj_1"},
		},
	}}
	got := joinProportional(usage, costs)
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}
	total := got[0].CostUSD + got[1].CostUSD
	if abs(total-0.30) > 1e-9 {
		t.Errorf("cost sum %v != 0.30", total)
	}
	for _, r := range got {
		switch r.APIKeyID {
		case "key_a":
			if abs(r.CostUSD-0.10) > 1e-9 {
				t.Errorf("key_a cost = %v, want 0.10", r.CostUSD)
			}
		case "key_b":
			if abs(r.CostUSD-0.20) > 1e-9 {
				t.Errorf("key_b cost = %v, want 0.20", r.CostUSD)
			}
		default:
			t.Errorf("unexpected APIKeyID %q", r.APIKeyID)
		}
		if r.SourceHash == "" {
			t.Error("SourceHash empty")
		}
	}
}

func TestJoinProportional_NoCostMatch(t *testing.T) {
	usage := []openaiUsageBucket{{
		StartTime: 1715000000,
		Results: []openaiUsageResult{
			{InputTokens: 10, OutputTokens: 5, ProjectID: "p", APIKeyID: "k", Model: "gpt-4o"},
		},
	}}
	got := joinProportional(usage, nil)
	if len(got) != 1 || got[0].CostUSD != 0 {
		t.Errorf("want 1 record with cost=0, got %+v", got)
	}
}

func TestJoinProportional_ZeroTokens(t *testing.T) {
	usage := []openaiUsageBucket{{
		StartTime: 1715000000,
		Results: []openaiUsageResult{
			{InputTokens: 0, OutputTokens: 0, ProjectID: "p", APIKeyID: "k", Model: "gpt-4o"},
		},
	}}
	costs := []openaiCostBucket{{
		StartTime: 1715000000,
		Results: []openaiCostResult{
			{Amount: openaiAmount{Value: 0.10, Currency: "usd"}, LineItem: "gpt-4o, input", ProjectID: "p"},
		},
	}}
	got := joinProportional(usage, costs)
	if len(got) != 1 || got[0].CostUSD != 0 {
		t.Errorf("want zero cost for zero-token group, got %+v", got)
	}
}

// ---------- HTTP flow tests ----------

const usageHappy = `{
  "object": "page",
  "data": [{
    "object": "bucket",
    "start_time": 1715000000,
    "end_time": 1715086400,
    "results": [
      {"input_tokens": 100, "output_tokens": 50, "project_id": "proj_1", "api_key_id": "key_a", "model": "gpt-4o"}
    ]
  }],
  "has_more": false,
  "next_page": ""
}`

const costHappy = `{
  "object": "page",
  "data": [{
    "object": "bucket",
    "start_time": 1715000000,
    "end_time": 1715086400,
    "results": [
      {"amount": {"value": 0.50, "currency": "usd"}, "line_item": "gpt-4o, input", "project_id": "proj_1"}
    ]
  }],
  "has_more": false,
  "next_page": ""
}`

func TestFetch_HappyPath(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, openaiUsagePath):
			_, _ = w.Write([]byte(usageHappy))
		case strings.HasPrefix(r.URL.Path, openaiCostsPath):
			_, _ = w.Write([]byte(costHappy))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Provider != "openai" || r.Model != "gpt-4o" ||
		r.PromptTokens != 100 || r.CompletionTokens != 50 || r.TotalTokens != 150 {
		t.Errorf("unexpected record: %+v", r)
	}
	if abs(r.CostUSD-0.50) > 1e-9 {
		t.Errorf("cost = %v, want 0.50", r.CostUSD)
	}
	if r.SourceHash == "" || r.ProjectID != "proj_1" || r.APIKeyID != "key_a" {
		t.Errorf("identity fields wrong: %+v", r)
	}
}

const usageWithPoison = `{
  "object": "page",
  "data": [{
    "object": "bucket",
    "start_time": 1715000000,
    "end_time": 1715086400,
    "results": [{
      "input_tokens": 100,
      "output_tokens": 50,
      "project_id": "proj_1",
      "api_key_id": "key_a",
      "model": "gpt-4o",
      "prompt": "POISON_PROMPT_TEXT",
      "completion": "POISON_COMPLETION_TEXT",
      "messages": [{"role": "user", "content": "POISON_MESSAGE_CONTENT"}],
      "content": "POISON_CONTENT",
      "tool_calls": [{"name": "POISON_TOOL"}],
      "raw_response": "POISON_RAW_RESPONSE",
      "system_prompt": "POISON_SYSTEM_PROMPT",
      "function_args": "POISON_FN_ARGS",
      "request_body": "POISON_REQUEST_BODY",
      "response_body": "POISON_RESPONSE_BODY",
      "raw_payload": "POISON_RAW_PAYLOAD"
    }]
  }],
  "has_more": false
}`

func TestFetch_StripsProhibitedFields(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, openaiUsagePath):
			_, _ = w.Write([]byte(usageWithPoison))
		case strings.HasPrefix(r.URL.Path, openaiCostsPath):
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		}
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}

	js, err := json.Marshal(records[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(js)

	forbidden := []string{
		`"messages"`,
		`"content"`,
		`"tool_calls"`,
		`"raw_response"`,
		`"raw_payload"`,
		`"request_body"`,
		`"response_body"`,
		`"system_prompt"`,
		`"function_args"`,
		"POISON_PROMPT_TEXT",
		"POISON_COMPLETION_TEXT",
		"POISON_MESSAGE_CONTENT",
		"POISON_CONTENT",
		"POISON_TOOL",
		"POISON_RAW_RESPONSE",
		"POISON_SYSTEM_PROMPT",
		"POISON_FN_ARGS",
		"POISON_REQUEST_BODY",
		"POISON_RESPONSE_BODY",
		"POISON_RAW_PAYLOAD",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}

	var back map[string]any
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	expected := map[string]bool{
		"provider": true, "model": true,
		"prompt_tokens": true, "completion_tokens": true, "total_tokens": true,
		"cost_usd": true, "recorded_at": true,
		"api_key_id": true, "project_id": true, "source_hash": true,
		"product": true, "sku": true, "unit_type": true,
		"usage_quantity": true, "unit_price_usd": true,
		"gross_amount_usd": true, "discount_amount_usd": true,
	}
	for k := range back {
		if !expected[k] {
			t.Errorf("unexpected JSON key %q in %s", k, s)
		}
	}
	if len(back) != len(expected) {
		t.Errorf("got %d keys, want %d: %v", len(back), len(expected), back)
	}
}

func TestFetch_HTTP401_NoLeak(t *testing.T) {
	body := `{"error":{"message":"Invalid Authorization header: ` + testAdminKey +
		` is not a valid admin key, please verify your secret with the dashboard. ` +
		strings.Repeat("X", 100) + `"}}`
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(body))
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, testAdminKey) {
		t.Errorf("admin key leaked: %s", msg)
	}
	if strings.Contains(msg, "verify your secret") {
		t.Errorf("response body text leaked: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 401") {
		t.Errorf("want HTTP 401 in error, got: %s", msg)
	}
	if !strings.Contains(msg, openaiUsagePath) {
		t.Errorf("want endpoint path in error, got: %s", msg)
	}
}

func TestFetch_HTTP500_NoLeak(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"server error: ` + testAdminKey + `"}`))
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testAdminKey) {
		t.Errorf("admin key leaked: %s", err.Error())
	}
}

func TestFetch_EmptyResponse(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("want 0 records, got %d", len(records))
	}
}

func TestFetch_Pagination(t *testing.T) {
	calls := map[string]int{}
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, openaiUsagePath) {
			calls["usage"]++
			if r.URL.Query().Get("page") == "" {
				_, _ = w.Write([]byte(`{"data":[{"start_time":1,"results":[{"input_tokens":1,"output_tokens":0,"project_id":"p","api_key_id":"k","model":"m"}]}],"has_more":true,"next_page":"PAGE2"}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"start_time":2,"results":[{"input_tokens":1,"output_tokens":0,"project_id":"p","api_key_id":"k","model":"m"}]}],"has_more":false,"next_page":""}`))
			}
		} else {
			calls["cost"]++
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		}
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if calls["usage"] != 2 {
		t.Errorf("want 2 usage calls, got %d", calls["usage"])
	}
	if len(records) != 2 {
		t.Errorf("want 2 records from 2 pages, got %d", len(records))
	}
}

func TestFetch_RequestParameters(t *testing.T) {
	var (
		usageQuery url.Values
		costQuery  url.Values
	)
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, openaiUsagePath):
			usageQuery = r.URL.Query()
		case strings.HasPrefix(r.URL.Path, openaiCostsPath):
			costQuery = r.URL.Query()
		}
		_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
	})
	if _, err := p.Fetch(context.Background(), 7); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	for name, q := range map[string]url.Values{"usage": usageQuery, "costs": costQuery} {
		if q.Get("start_time") == "" {
			t.Errorf("%s: start_time missing", name)
		}
		if got := q.Get("bucket_width"); got != "1d" {
			t.Errorf("%s: bucket_width = %q, want 1d", name, got)
		}
		if q.Get("limit") == "" {
			t.Errorf("%s: limit missing", name)
		}
	}

	wantGroupBy := map[string]bool{"model": true, "project_id": true, "api_key_id": true}
	got := map[string]bool{}
	for _, g := range usageQuery["group_by"] {
		got[g] = true
	}
	for k := range wantGroupBy {
		if !got[k] {
			t.Errorf("usage: missing group_by=%s; got=%v", k, usageQuery["group_by"])
		}
	}
}

func TestFetch_LimitClampedTo31(t *testing.T) {
	var captured url.Values
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, openaiUsagePath) && captured == nil {
			captured = r.URL.Query()
		}
		_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
	})
	if _, err := p.Fetch(context.Background(), 90); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := captured.Get("limit"); got != "31" {
		t.Errorf("limit with days=90: got %q, want 31", got)
	}
}
