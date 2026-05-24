package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const azureCostPoisonResponse = `{
  "properties": {
    "columns": [
      {"name":"totalCost","type":"Number"},
      {"name":"UsageDate","type":"Number"},
      {"name":"ResourceId","type":"String"},
      {"name":"ServiceName","type":"String"},
      {"name":"MeterCategory","type":"String"},
      {"name":"Meter","type":"String"},
      {"name":"MeterSubCategory","type":"String"},
      {"name":"Currency","type":"String"},
      {"name":"prompt","type":"String"},
      {"name":"completion","type":"String"},
      {"name":"messages","type":"String"},
      {"name":"content","type":"String"},
      {"name":"raw_response","type":"String"},
      {"name":"request_body","type":"String"},
      {"name":"response_body","type":"String"},
      {"name":"source_code","type":"String"},
      {"name":"repository_content","type":"String"},
      {"name":"diagnostic_log","type":"String"},
      {"name":"user_text","type":"String"},
      {"name":"system_prompt","type":"String"},
      {"name":"tool_calls","type":"String"}
    ],
    "rows": [
      [1.25, 20260520, "/subscriptions/sub/rg/myacct", "Azure OpenAI", "Azure OpenAI", "GPT-4o Input Tokens", "Azure OpenAI - GPT-4o", "USD",
       "POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGES", "POISON_CONTENT",
       "POISON_RAW_RESPONSE", "POISON_REQUEST_BODY", "POISON_RESPONSE_BODY", "POISON_SOURCE_CODE",
       "POISON_REPOSITORY_CONTENT", "POISON_DIAGNOSTIC_LOG", "POISON_USER_TEXT",
       "POISON_SYSTEM_PROMPT", "POISON_TOOL_CALLS"]
    ]
  }
}`

func TestAzure_StripsProhibitedFields(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(azureCostPoisonResponse))
	}
	p := newAzureTestProvider(t, ts)
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
	assertAzureNoForbiddenSubstrings(t, string(js))
	assertAzureExpectedJSONKeys(t, js)
}

func TestAzure_TokenEndpoint401NoLeak(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.tokenHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","client_secret":"POISON_BODY_SECRET","client_id":"` + azureTestClientID + `"}`))
	}
	p := newAzureTestProvider(t, ts)
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if strings.Contains(msg, azureTestClientSecret) {
		t.Errorf("client secret leaked: %s", msg)
	}
	if strings.Contains(msg, "POISON_BODY_SECRET") {
		t.Errorf("body leaked: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 401") {
		t.Errorf("want HTTP 401 in error, got: %s", msg)
	}
	if !strings.Contains(msg, "AZURE_") {
		t.Errorf("want env-var permission hint, got: %s", msg)
	}
	var he *azureHTTPError
	if !errors.As(err, &he) {
		t.Errorf("error chain missing azureHTTPError")
	}
}

func TestAzure_Management403NoLeak(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"AuthorizationFailed","message":"POISON_BODY ` + azureTestBearer + `"}}`))
	}
	p := newAzureTestProvider(t, ts)
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if strings.Contains(msg, azureTestBearer) {
		t.Errorf("bearer token leaked: %s", msg)
	}
	if strings.Contains(msg, "POISON_BODY") {
		t.Errorf("body leaked: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 403") {
		t.Errorf("want HTTP 403, got: %s", msg)
	}
	if !strings.Contains(msg, "Cost Management Reader") {
		t.Errorf("want Cost Management Reader hint, got: %s", msg)
	}
}

func TestAzure_Management500NoHint(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error POISON_BODY`))
	}
	p := newAzureTestProvider(t, ts)
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if strings.Contains(msg, "POISON_BODY") {
		t.Errorf("body leaked: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 500") {
		t.Errorf("want HTTP 500, got: %s", msg)
	}
	if strings.Contains(msg, "Cost Management Reader") {
		t.Errorf("500 should not include permission hint: %s", msg)
	}
}

func assertAzureNoForbiddenSubstrings(t *testing.T, s string) {
	t.Helper()
	forbidden := []string{
		`"prompt"`, `"completion"`, `"messages"`, `"content"`,
		`"raw_response"`, `"request_body"`, `"response_body"`,
		`"source_code"`, `"repository_content"`, `"diagnostic_log"`,
		`"user_text"`, `"system_prompt"`, `"tool_calls"`,
		"POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGES",
		"POISON_CONTENT", "POISON_RAW_RESPONSE", "POISON_REQUEST_BODY",
		"POISON_RESPONSE_BODY", "POISON_SOURCE_CODE", "POISON_REPOSITORY_CONTENT",
		"POISON_DIAGNOSTIC_LOG", "POISON_USER_TEXT", "POISON_SYSTEM_PROMPT",
		"POISON_TOOL_CALLS",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}
}

func assertAzureExpectedJSONKeys(t *testing.T, js []byte) {
	t.Helper()
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
			t.Errorf("unexpected JSON key %q", k)
		}
	}
	if len(back) != len(expected) {
		t.Errorf("got %d keys, want %d: %v", len(back), len(expected), back)
	}
}
