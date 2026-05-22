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

const anthropicTestAdminKey = "sk-ant-admin-SECRET-12345"

func newAnthropicTestProvider(t *testing.T, h http.HandlerFunc) *AnthropicProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &AnthropicProvider{
		AdminKey:   anthropicTestAdminKey,
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		UserAgent:  anthropicUserAgentDev,
	}
}

const anthropicUsageHappy = `{
  "data": [{
    "starting_at": "2026-05-21T00:00:00Z",
    "ending_at": "2026-05-22T00:00:00Z",
    "results": [{
      "uncached_input_tokens": 100,
      "cache_creation": {"ephemeral_1h_input_tokens": 20, "ephemeral_5m_input_tokens": 30},
      "cache_read_input_tokens": 50,
      "output_tokens": 80,
      "api_key_id": "key_1",
      "workspace_id": "wrk_1",
      "model": "claude-sonnet-4-20250514"
    }]
  }],
  "has_more": false,
  "next_page": null
}`

const anthropicCostHappy = `{
  "data": [{
    "starting_at": "2026-05-21T00:00:00Z",
    "ending_at": "2026-05-22T00:00:00Z",
    "results": [{
      "currency": "USD",
      "amount": "280",
      "workspace_id": "wrk_1",
      "description": "Claude Sonnet 4 Usage - Input Tokens",
      "model": "claude-sonnet-4-20250514"
    }]
  }],
  "has_more": false,
  "next_page": null
}`

func TestAnthropicFetch_HappyPath(t *testing.T) {
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicFixture(t, w, r, anthropicUsageHappy, anthropicCostHappy)
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}

	r := records[0]
	if r.Provider != "anthropic" || r.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("identity mismatch: %+v", r)
	}
	if r.PromptTokens != 200 || r.CompletionTokens != 80 || r.TotalTokens != 280 {
		t.Errorf("token counts mismatch: %+v", r)
	}
	if abs(r.CostUSD-2.80) > 1e-9 {
		t.Errorf("cost = %v, want 2.80", r.CostUSD)
	}
	wantHash := ComputeSourceHash("anthropic", "2026-05-21T00:00:00Z", r.Model, "wrk_1", "key_1")
	if r.SourceHash != wantHash || wantHash != ComputeSourceHash("anthropic", "2026-05-21T00:00:00Z", r.Model, "wrk_1", "key_1") {
		t.Errorf("source hash not deterministic: got %q want %q", r.SourceHash, wantHash)
	}
}

const anthropicUsagePoison = `{
  "data": [{
    "starting_at": "2026-05-21T00:00:00Z",
    "results": [{
      "uncached_input_tokens": 1,
      "cache_creation": {"ephemeral_1h_input_tokens": 2, "ephemeral_5m_input_tokens": 3},
      "cache_read_input_tokens": 4,
      "output_tokens": 5,
      "api_key_id": "key_1",
      "workspace_id": "wrk_1",
      "model": "claude-3-5-haiku-20241022",
      "prompt": "POISON_PROMPT_TEXT",
      "completion": "POISON_COMPLETION_TEXT",
      "messages": [{"role": "user", "content": "POISON_MESSAGE_CONTENT"}],
      "content": "POISON_CONTENT",
      "tool_calls": [{"name": "POISON_TOOL"}],
      "raw_response": "POISON_RAW_RESPONSE",
      "request_body": "POISON_REQUEST_BODY",
      "response_body": "POISON_RESPONSE_BODY",
      "raw_payload": "POISON_RAW_PAYLOAD"
    }]
  }],
  "has_more": false
}`

const anthropicCostPoison = `{
  "data": [{
    "starting_at": "2026-05-21T00:00:00Z",
    "results": [{
      "currency": "USD",
      "amount": "11",
      "workspace_id": "wrk_1",
      "description": "claude-3-5-haiku-20241022 input",
      "prompt": "POISON_COST_PROMPT",
      "raw_response": "POISON_COST_RAW"
    }]
  }],
  "has_more": false
}`

func TestAnthropicFetch_StripsProhibitedFields(t *testing.T) {
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicFixture(t, w, r, anthropicUsagePoison, anthropicCostPoison)
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
		`"prompt"`, `"completion"`, `"messages"`, `"content"`, `"tool_calls"`,
		`"raw_response"`, `"request_body"`, `"response_body"`, `"raw_payload"`,
		"POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGE_CONTENT",
		"POISON_CONTENT", "POISON_TOOL", "POISON_RAW_RESPONSE", "POISON_REQUEST_BODY",
		"POISON_RESPONSE_BODY", "POISON_RAW_PAYLOAD", "POISON_COST_PROMPT", "POISON_COST_RAW",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}
}

func TestAnthropicFetch_RequestHeadersAndQuery(t *testing.T) {
	var usageQuery, costQuery url.Values
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != anthropicTestAdminKey {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicAPIVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != anthropicUserAgentDev {
			t.Errorf("User-Agent = %q", got)
		}
		switch r.URL.Path {
		case anthropicUsagePath:
			usageQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		case anthropicCostPath:
			costQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})
	if _, err := p.Fetch(context.Background(), 90); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	assertAnthropicBaseQuery(t, "usage", usageQuery)
	assertAnthropicBaseQuery(t, "cost", costQuery)
	assertGroups(t, "usage", usageQuery, []string{"model", "workspace_id", "api_key_id"})
	assertGroups(t, "cost", costQuery, []string{"workspace_id", "description"})
}

func TestAnthropicFetch_Pagination(t *testing.T) {
	calls := map[string]int{}
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case anthropicUsagePath:
			calls["usage"]++
			if r.URL.Query().Get("page") == "" {
				_, _ = w.Write([]byte(`{"data":[{"starting_at":"2026-05-20T00:00:00Z","results":[{"uncached_input_tokens":1}]}],"has_more":true,"next_page":"U2"}`))
			} else {
				if got := r.URL.Query().Get("page"); got != "U2" {
					t.Errorf("usage page = %q", got)
				}
				_, _ = w.Write([]byte(`{"data":[{"starting_at":"2026-05-21T00:00:00Z","results":[{"uncached_input_tokens":1}]}],"has_more":false}`))
			}
		case anthropicCostPath:
			calls["cost"]++
			if r.URL.Query().Get("page") == "" {
				_, _ = w.Write([]byte(`{"data":[],"has_more":true,"next_page":"C2"}`))
			} else {
				if got := r.URL.Query().Get("page"); got != "C2" {
					t.Errorf("cost page = %q", got)
				}
				_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
			}
		}
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if calls["usage"] != 2 || calls["cost"] != 2 || len(records) != 2 {
		t.Fatalf("calls=%v records=%d", calls, len(records))
	}
}

func TestAnthropicFetch_NullDimensions(t *testing.T) {
	usage := `{"data":[{"starting_at":"2026-05-21T00:00:00Z","results":[{"uncached_input_tokens":1,"output_tokens":2,"api_key_id":null,"workspace_id":null,"model":null}]}],"has_more":false}`
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeAnthropicFixture(t, w, r, usage, `{"data":[],"has_more":false}`)
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.APIKeyID != "" || r.ProjectID != "" || r.Model != "" {
		t.Errorf("null dimensions not normalized: %+v", r)
	}
	wantHash := ComputeSourceHash("anthropic", "2026-05-21T00:00:00Z", "", "", "")
	if r.SourceHash != wantHash {
		t.Errorf("hash = %q, want %q", r.SourceHash, wantHash)
	}
}

func TestAnthropicFetch_HTTPErrorNoLeak(t *testing.T) {
	p := newAnthropicTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"` + anthropicTestAdminKey + ` POISON_RAW_BODY"}`))
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, anthropicTestAdminKey) || strings.Contains(msg, "POISON_RAW_BODY") {
		t.Errorf("unsafe error: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 401") || !strings.Contains(msg, anthropicUsagePath) {
		t.Errorf("missing safe status/path: %s", msg)
	}
}

func TestAnthropicCostDescriptionSlugFallback(t *testing.T) {
	model := "claude-3-5-haiku-20241022"
	usage := []anthropicUsageBucket{{StartingAt: "2026-05-21T00:00:00Z", Results: []anthropicUsageResult{{UncachedInputTokens: 10, Model: model}}}}
	costs := []anthropicCostBucket{{StartingAt: "2026-05-21T00:00:00Z", Results: []anthropicCostResult{{Currency: "USD", Amount: "50", Description: model + " input"}}}}
	records, err := joinAnthropicProportional(usage, costs)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if len(records) != 1 || abs(records[0].CostUSD-0.50) > 1e-9 {
		t.Fatalf("fallback allocation failed: %+v", records)
	}
}
