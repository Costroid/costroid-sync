package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// gcpPoisonQueryResponse embeds every forbidden field shape we can think
// of inside a BigQuery REST response — both as top-level keys and as
// extra row "f" entries — to verify decoding silently drops them. The
// strict per-field positional decode (via the declared schema) plus the
// narrow gcpBillingRow struct must guarantee none of these substrings
// make it into a NormalizedCostRecord's JSON output.
const gcpPoisonQueryResponse = `{
  "jobReference": {"projectId":"test-project","jobId":"job_poison","location":"US"},
  "jobComplete": true,
  "prompt": "POISON_PROMPT_TEXT",
  "completion": "POISON_COMPLETION_TEXT",
  "messages": "POISON_MESSAGES",
  "content": "POISON_CONTENT",
  "raw_response": "POISON_RAW_RESPONSE",
  "request_body": "POISON_REQUEST_BODY",
  "response_body": "POISON_RESPONSE_BODY",
  "source_code": "POISON_SOURCE_CODE",
  "labels": {"poison_label_key":"POISON_LABEL_VALUE"},
  "system_labels": {"poison_sys_key":"POISON_SYS_VALUE"},
  "user_text": "POISON_USER_TEXT",
  "schema": {"fields": [
    {"name":"usage_start_time","type":"STRING"},
    {"name":"service_description","type":"STRING"},
    {"name":"sku_description","type":"STRING"},
    {"name":"cost","type":"STRING"},
    {"name":"currency","type":"STRING"},
    {"name":"usage_amount","type":"STRING"},
    {"name":"usage_unit","type":"STRING"}
  ]},
  "rows": [
    {
      "f":[
        {"v":"2026-05-20 12:00:00 UTC"},
        {"v":"Vertex AI"},
        {"v":"Gemini 2.5 Flash characters"},
        {"v":"1.25"},
        {"v":"USD"},
        {"v":"500000"},
        {"v":"characters"}
      ],
      "prompt": "POISON_ROW_PROMPT",
      "completion": "POISON_ROW_COMPLETION",
      "labels": {"foo":"POISON_ROW_LABEL"},
      "system_labels": {"bar":"POISON_ROW_SYS"}
    }
  ]
}`

func TestGCPBilling_StripsProhibitedFields(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(gcpPoisonQueryResponse))
	}
	p := newGCPBillingTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}

	js, err := json.Marshal(records[0])
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	assertGCPNoForbiddenSubstrings(t, string(js))
	assertGCPExpectedJSONKeys(t, js)
}

// TestGCPBilling_SQLOmitsLabelsAndSystemLabels asserts the SQL we send
// over the wire never references the forbidden columns — this is the
// metadata-only invariant at its source.
func TestGCPBilling_SQLOmitsLabelsAndSystemLabels(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	if _, err := p.Fetch(context.Background(), 7); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ts.capturedQuery) != 1 {
		t.Fatalf("want 1 query call, got %d", len(ts.capturedQuery))
	}
	body := ts.capturedQuery[0].Body
	for _, bad := range []string{"labels", "system_labels", "tags", "credits", "adjustment_info", "SELECT *", "select *"} {
		if strings.Contains(body, bad) {
			t.Errorf("SQL body contains forbidden token %q: %s", bad, body)
		}
	}
}

func assertGCPNoForbiddenSubstrings(t *testing.T, s string) {
	t.Helper()
	forbidden := []string{
		`"prompt"`, `"completion"`, `"messages"`, `"content"`,
		`"raw_response"`, `"request_body"`, `"response_body"`,
		`"source_code"`, `"labels"`, `"system_labels"`, `"user_text"`,
		"POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGES",
		"POISON_CONTENT", "POISON_RAW_RESPONSE", "POISON_REQUEST_BODY",
		"POISON_RESPONSE_BODY", "POISON_SOURCE_CODE",
		"POISON_LABEL_VALUE", "POISON_SYS_VALUE", "POISON_USER_TEXT",
		"POISON_ROW_PROMPT", "POISON_ROW_COMPLETION",
		"POISON_ROW_LABEL", "POISON_ROW_SYS",
		"poison_label_key", "poison_sys_key",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}
}

func assertGCPExpectedJSONKeys(t *testing.T, js []byte) {
	t.Helper()
	var back map[string]any
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	expected := expectedNormalizedJSONKeys()
	for k := range back {
		if !expected[k] {
			t.Errorf("unexpected JSON key %q", k)
		}
	}
	if len(back) != len(expected) {
		t.Errorf("got %d keys, want %d: %v", len(back), len(expected), back)
	}
}

// TestGCPBilling_HTTPErrorNoLeak is a holistic check across both the
// token and query endpoints that NO error string under any status leaks
// secrets — bearer tokens, JWT assertions, private key material,
// response bodies, or service account emails — even when the upstream
// fixture explicitly tries to inject them.
func TestGCPBilling_HTTPErrorNoLeak(t *testing.T) {
	cases := []struct {
		name    string
		breaker func(ts *gcpBillingTestServer, status int)
	}{
		{"token_failure", func(ts *gcpBillingTestServer, status int) {
			ts.tokenHandler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"POISON_BODY","secret":"LEAKED_TOKEN_BODY"}`))
			}
		}},
		{"query_failure", func(ts *gcpBillingTestServer, status int) {
			ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"POISON_QUERY_BODY","stack":"LEAKED_QUERY_BODY"}`))
			}
		}},
	}
	statuses := []int{400, 401, 403, 404, 500}
	for _, tc := range cases {
		for _, status := range statuses {
			tc, status := tc, status
			t.Run(tc.name+"_"+http.StatusText(status), func(t *testing.T) {
				ts := newGCPBillingTestServer(t)
				tc.breaker(ts, status)
				p := newGCPBillingTestProvider(t, ts)
				_, err := p.Fetch(context.Background(), 1)
				if err == nil {
					t.Fatalf("want error")
				}
				msg := err.Error()
				forbidden := []string{
					"POISON_BODY", "POISON_QUERY_BODY",
					"LEAKED_TOKEN_BODY", "LEAKED_QUERY_BODY",
					"private_key", "BEGIN PRIVATE KEY", "END PRIVATE KEY",
					gcpTestBearer, gcpTestEmail,
					"assertion=",
				}
				for _, bad := range forbidden {
					if strings.Contains(msg, bad) {
						t.Errorf("error leaked %q: %s", bad, msg)
					}
				}
			})
		}
	}
}
