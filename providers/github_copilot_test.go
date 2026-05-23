package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	copilotTestPAT = "ghp_test_FAKE_xxxxxxxxxxxxxxxx"
	copilotTestOrg = "test-org"
)

func newCopilotTestProvider(t *testing.T, h http.HandlerFunc) *GitHubCopilotProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &GitHubCopilotProvider{
		Token:      copilotTestPAT,
		Org:        copilotTestOrg,
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		UserAgent:  githubCopilotUserAgentDev,
	}
}

const copilotHappyResponse = `{
  "usageItems": [
    {
      "product": "copilot",
      "sku": "copilot_premium_request_user",
      "model": "gpt-4-copilot",
      "unitType": "premium_requests",
      "pricePerUnit": 0.01,
      "grossQuantity": 300,
      "grossAmount": 3.00,
      "discountQuantity": 50,
      "discountAmount": 0.50,
      "netQuantity": 250,
      "netAmount": 2.50
    },
    {
      "product": "copilot",
      "sku": "copilot_chat_premium_request",
      "model": "claude-sonnet-4-copilot",
      "unitType": "premium_requests",
      "pricePerUnit": 0.02,
      "grossQuantity": 40,
      "grossAmount": 0.80,
      "netQuantity": 40,
      "netAmount": 0.80
    }
  ]
}`

func TestGitHubCopilot_FetchHappyPath(t *testing.T) {
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(copilotHappyResponse))
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	// Sort is by RecordedAt asc then Product, SKU, Model.
	for _, r := range records {
		if r.Provider != "github-copilot" {
			t.Errorf("Provider = %q", r.Provider)
		}
		if r.ProjectID != copilotTestOrg {
			t.Errorf("ProjectID = %q", r.ProjectID)
		}
		if r.APIKeyID != "" {
			t.Errorf("APIKeyID should be empty, got %q", r.APIKeyID)
		}
		if r.PromptTokens != 0 || r.CompletionTokens != 0 || r.TotalTokens != 0 {
			t.Errorf("non-token unitType should leave token counts at 0: %+v", r)
		}
		if r.SourceHash == "" {
			t.Errorf("SourceHash empty: %+v", r)
		}
	}
	// Find the gpt-4-copilot row.
	var gpt NormalizedCostRecord
	for _, r := range records {
		if r.Model == "gpt-4-copilot" {
			gpt = r
		}
	}
	if gpt.Product != "copilot" || gpt.SKU != "copilot_premium_request_user" {
		t.Errorf("gpt row product/sku wrong: %+v", gpt)
	}
	if gpt.UnitType != "premium_requests" {
		t.Errorf("gpt UnitType = %q", gpt.UnitType)
	}
	if gpt.UnitPriceUSD != 0.01 {
		t.Errorf("gpt UnitPriceUSD = %v", gpt.UnitPriceUSD)
	}
	if gpt.UsageQuantity != 250 { // prefers netQuantity
		t.Errorf("gpt UsageQuantity = %v, want 250 (netQuantity)", gpt.UsageQuantity)
	}
	if gpt.GrossAmountUSD != 3.00 || gpt.DiscountAmountUSD != 0.50 {
		t.Errorf("gpt gross/discount wrong: %+v", gpt)
	}
	if gpt.CostUSD != 2.50 {
		t.Errorf("gpt CostUSD = %v, want 2.50 (netAmount)", gpt.CostUSD)
	}
}

func TestGitHubCopilot_QueryConstruction(t *testing.T) {
	type captured struct {
		Path  string
		Query url.Values
	}
	var got captured
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		got.Query = r.URL.Query()
		_, _ = w.Write([]byte(`{"usageItems":[]}`))
	})
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	wantPath := "/organizations/" + copilotTestOrg + "/settings/billing/premium_request/usage"
	if got.Path != wantPath {
		t.Errorf("path = %q, want %q", got.Path, wantPath)
	}
	for _, k := range []string{"year", "month", "day"} {
		if v := got.Query.Get(k); v == "" {
			t.Errorf("query %q missing", k)
		}
	}
	now := time.Now().UTC()
	if got.Query.Get("year") != fmt.Sprint(now.Year()) {
		t.Errorf("year = %q, want %d", got.Query.Get("year"), now.Year())
	}
}

func TestGitHubCopilot_Headers(t *testing.T) {
	var got http.Header
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"usageItems":[]}`))
	})
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	cases := map[string]string{
		"Accept":               "application/vnd.github+json",
		"Authorization":        "Bearer " + copilotTestPAT,
		"X-Github-Api-Version": githubCopilotAPIVersion,
		"User-Agent":           githubCopilotUserAgentDev,
	}
	for h, want := range cases {
		if v := got.Get(h); v != want {
			t.Errorf("header %s = %q, want %q", h, v, want)
		}
	}
}

const copilotPoisonResponse = `{
  "usageItems": [{
    "product": "copilot",
    "sku": "copilot_premium_request_user",
    "model": "gpt-4-copilot",
    "unitType": "premium_requests",
    "pricePerUnit": 0.01,
    "netQuantity": 100,
    "netAmount": 1.00,
    "prompt": "POISON_PROMPT_TEXT",
    "completion": "POISON_COMPLETION_TEXT",
    "messages": [{"role":"user","content":"POISON_MESSAGE_CONTENT"}],
    "content": "POISON_CONTENT",
    "tool_calls": [{"name":"POISON_TOOL"}],
    "raw_response": "POISON_RAW_RESPONSE",
    "raw_payload": "POISON_RAW_PAYLOAD",
    "request_body": "POISON_REQUEST_BODY",
    "response_body": "POISON_RESPONSE_BODY",
    "system_prompt": "POISON_SYSTEM_PROMPT",
    "function_args": "POISON_FN_ARGS",
    "source_code": "POISON_SOURCE_CODE",
    "repository_content": "POISON_REPO_CONTENT"
  }]
}`

func TestGitHubCopilot_StripsProhibitedFields(t *testing.T) {
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(copilotPoisonResponse))
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
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
		`"raw_response"`, `"raw_payload"`, `"request_body"`, `"response_body"`,
		`"system_prompt"`, `"function_args"`, `"source_code"`, `"repository_content"`,
		"POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGE_CONTENT",
		"POISON_CONTENT", "POISON_TOOL", "POISON_RAW_RESPONSE", "POISON_RAW_PAYLOAD",
		"POISON_REQUEST_BODY", "POISON_RESPONSE_BODY", "POISON_SYSTEM_PROMPT",
		"POISON_FN_ARGS", "POISON_SOURCE_CODE", "POISON_REPO_CONTENT",
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

func runCopilotErrorTest(t *testing.T, status int, body string, wantHint bool) {
	t.Helper()
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatalf("status %d: expected error", status)
	}
	msg := err.Error()
	if strings.Contains(msg, copilotTestPAT) {
		t.Errorf("status %d: PAT leaked: %s", status, msg)
	}
	if strings.Contains(msg, "POISON_BODY") {
		t.Errorf("status %d: body bytes leaked: %s", status, msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("HTTP %d", status)) {
		t.Errorf("status %d: want HTTP %d in error, got: %s", status, status, msg)
	}
	hasHint := strings.Contains(msg, "GitHub Copilot billing usage is unavailable")
	if wantHint && !hasHint {
		t.Errorf("status %d: want permission hint, got: %s", status, msg)
	}
	if !wantHint && hasHint {
		t.Errorf("status %d: did not want permission hint, got: %s", status, msg)
	}
	var he *ghCopilotHTTPError
	if !errors.As(err, &he) {
		t.Errorf("status %d: error chain missing ghCopilotHTTPError", status)
	}
}

func TestGitHubCopilot_HTTP400_PermissionHint(t *testing.T) {
	runCopilotErrorTest(t, 400, `{"message":"POISON_BODY","token":"`+copilotTestPAT+`"}`, true)
}

func TestGitHubCopilot_HTTP401_NoLeak(t *testing.T) {
	runCopilotErrorTest(t, 401, `{"message":"Bad credentials POISON_BODY token=`+copilotTestPAT+`"}`, true)
}

func TestGitHubCopilot_HTTP403_NoLeak(t *testing.T) {
	runCopilotErrorTest(t, 403, `{"message":"Forbidden POISON_BODY for `+copilotTestPAT+`"}`, true)
}

func TestGitHubCopilot_HTTP404_NoLeak(t *testing.T) {
	runCopilotErrorTest(t, 404, `{"message":"Not Found POISON_BODY"}`, true)
}

func TestGitHubCopilot_HTTP500_NoLeakNoHint(t *testing.T) {
	runCopilotErrorTest(t, 500, `{"message":"Server error POISON_BODY `+copilotTestPAT+`"}`, false)
}

func TestCopilotSourceHash_Deterministic(t *testing.T) {
	a := copilotSourceHash("2026-05-22T00:00:00Z", "org", "copilot", "sku1", "gpt-4-copilot", "premium_requests")
	b := copilotSourceHash("2026-05-22T00:00:00Z", "org", "copilot", "sku1", "gpt-4-copilot", "premium_requests")
	if a != b {
		t.Fatalf("same inputs -> different hashes: %s vs %s", a, b)
	}
	cases := []struct {
		name                                           string
		recordedAt, org, product, sku, model, unitType string
	}{
		{"different date", "2026-05-23T00:00:00Z", "org", "copilot", "sku1", "gpt-4-copilot", "premium_requests"},
		{"different org", "2026-05-22T00:00:00Z", "other", "copilot", "sku1", "gpt-4-copilot", "premium_requests"},
		{"different product", "2026-05-22T00:00:00Z", "org", "actions", "sku1", "gpt-4-copilot", "premium_requests"},
		{"different sku", "2026-05-22T00:00:00Z", "org", "copilot", "sku2", "gpt-4-copilot", "premium_requests"},
		{"different model", "2026-05-22T00:00:00Z", "org", "copilot", "sku1", "claude-sonnet", "premium_requests"},
		{"different unitType", "2026-05-22T00:00:00Z", "org", "copilot", "sku1", "gpt-4-copilot", "tokens"},
	}
	for _, c := range cases {
		other := copilotSourceHash(c.recordedAt, c.org, c.product, c.sku, c.model, c.unitType)
		if other == a {
			t.Errorf("%s should differ but matched", c.name)
		}
	}
}

func TestGitHubCopilot_MissingOptionalFields(t *testing.T) {
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// Minimal item — only product + netAmount.
		_, _ = w.Write([]byte(`{"usageItems":[{"product":"copilot","netAmount":1.23}]}`))
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Product != "copilot" {
		t.Errorf("Product = %q", r.Product)
	}
	if r.Model != "copilot" { // fallback chain: model -> sku -> product
		t.Errorf("Model = %q, want fallback to product %q", r.Model, "copilot")
	}
	if r.CostUSD != 1.23 {
		t.Errorf("CostUSD = %v", r.CostUSD)
	}
	if r.SKU != "" || r.UnitType != "" || r.UnitPriceUSD != 0 {
		t.Errorf("optional fields not zero: %+v", r)
	}
}

func TestGitHubCopilot_TokensOnlyWhenUnitTypeMatches(t *testing.T) {
	// unitType "tokens" → TotalTokens populated.
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"usageItems":[{"product":"copilot","model":"x","unitType":"tokens","netQuantity":12345,"netAmount":0.5}]}`))
	})
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch tokens: %v", err)
	}
	if records[0].TotalTokens != 12345 {
		t.Errorf("TotalTokens = %d, want 12345", records[0].TotalTokens)
	}
	if records[0].UsageQuantity != 12345 {
		t.Errorf("UsageQuantity = %v, want 12345", records[0].UsageQuantity)
	}

	// unitType "premium_requests" → TotalTokens stays 0.
	p2 := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"usageItems":[{"product":"copilot","model":"x","unitType":"premium_requests","netQuantity":250,"netAmount":2.5}]}`))
	})
	records2, err := p2.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch non-tokens: %v", err)
	}
	if records2[0].TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 for non-tokens unitType", records2[0].TotalTokens)
	}
	if records2[0].UsageQuantity != 250 {
		t.Errorf("UsageQuantity = %v, want 250", records2[0].UsageQuantity)
	}
}

func TestGitHubCopilot_DaysClampedTo31(t *testing.T) {
	count := 0
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		count++
		_, _ = w.Write([]byte(`{"usageItems":[]}`))
	})
	if _, err := p.Fetch(context.Background(), 90); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if count != 31 {
		t.Errorf("days=90 → %d requests, want 31 (clamped)", count)
	}
}

func TestGitHubCopilot_PerDayLoop(t *testing.T) {
	seenDays := map[string]int{}
	p := newCopilotTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		key := q.Get("year") + "-" + q.Get("month") + "-" + q.Get("day")
		seenDays[key]++
		_, _ = w.Write([]byte(`{"usageItems":[]}`))
	})
	if _, err := p.Fetch(context.Background(), 3); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(seenDays) != 3 {
		t.Errorf("want 3 distinct days, got %d: %v", len(seenDays), seenDays)
	}
	for k, n := range seenDays {
		if n != 1 {
			t.Errorf("day %s queried %d times, want 1", k, n)
		}
	}
}
