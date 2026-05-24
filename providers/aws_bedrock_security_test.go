package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const awsBedrockPoisonCostResponse = `{
  "ResultsByTime": [{
    "TimePeriod": {"Start": "2026-05-20", "End": "2026-05-21"},
    "Groups": [{
      "Keys": ["Amazon Bedrock", "USE1-ModelInvocation-InputTokens"],
      "Metrics": {
        "UnblendedCost": {"Amount": "1.25", "Unit": "USD"},
        "UsageQuantity": {"Amount": "500", "Unit": "N/A"},
        "prompt": "POISON_PROMPT_TEXT",
        "completion": "POISON_COMPLETION_TEXT"
      },
      "prompt": "POISON_PROMPT_TEXT",
      "completion": "POISON_COMPLETION_TEXT",
      "messages": "POISON_MESSAGES",
      "content": "POISON_CONTENT",
      "raw_response": "POISON_RAW_RESPONSE",
      "request_body": "POISON_REQUEST_BODY",
      "response_body": "POISON_RESPONSE_BODY",
      "source_code": "POISON_SOURCE_CODE",
      "invocation_log": "POISON_INVOCATION_LOG",
      "user_text": "POISON_USER_TEXT"
    }]
  }]
}`

func TestAWSBedrock_StripsProhibitedFields(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(awsBedrockPoisonCostResponse))
	}
	p := newAWSBedrockTestProvider(ts)
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
	assertAWSNoForbiddenSubstrings(t, string(js))
	assertAWSExpectedJSONKeys(t, js)
}

func TestAWSBedrock_HTTPErrorNoLeak(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			err := fetchAWSBedrockStatusError(t, status)
			msg := err.Error()
			for _, bad := range []string{awsTestSecret, awsTestSession, "POISON_BODY", "Authorization:"} {
				if strings.Contains(msg, bad) {
					t.Fatalf("error leaked %q: %s", bad, msg)
				}
			}
			if !strings.Contains(msg, fmt.Sprintf("HTTP %d", status)) {
				t.Fatalf("missing HTTP status in error: %s", msg)
			}
			hasHint := strings.Contains(msg, "AWS Cost Explorer request failed")
			if status == 500 && hasHint {
				t.Fatalf("500 should not include permission hint: %s", msg)
			}
			if status != 500 && !hasHint {
				t.Fatalf("status %d should include permission hint: %s", status, msg)
			}
			var he *awsBedrockHTTPError
			if !errors.As(err, &he) {
				t.Fatalf("missing awsBedrockHTTPError in chain: %v", err)
			}
		})
	}
}

func fetchAWSBedrockStatusError(t *testing.T, status int) error {
	t.Helper()
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message":"POISON_BODY ` + awsTestSecret + ` ` + awsTestSession + `"}`))
	}
	p := newAWSBedrockTestProvider(ts)
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatalf("want error for status %d", status)
	}
	return err
}

func assertAWSNoForbiddenSubstrings(t *testing.T, s string) {
	t.Helper()
	forbidden := []string{
		`"prompt"`, `"completion"`, `"messages"`, `"content"`,
		`"raw_response"`, `"request_body"`, `"response_body"`,
		`"source_code"`, `"invocation_log"`, `"user_text"`,
		"POISON_PROMPT_TEXT", "POISON_COMPLETION_TEXT", "POISON_MESSAGES",
		"POISON_CONTENT", "POISON_RAW_RESPONSE", "POISON_REQUEST_BODY",
		"POISON_RESPONSE_BODY", "POISON_SOURCE_CODE", "POISON_INVOCATION_LOG",
		"POISON_USER_TEXT",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}
}

func assertAWSExpectedJSONKeys(t *testing.T, js []byte) {
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

func expectedNormalizedJSONKeys() map[string]bool {
	return map[string]bool{
		"provider": true, "model": true,
		"prompt_tokens": true, "completion_tokens": true, "total_tokens": true,
		"cost_usd": true, "recorded_at": true,
		"api_key_id": true, "project_id": true, "source_hash": true,
		"product": true, "sku": true, "unit_type": true,
		"usage_quantity": true, "unit_price_usd": true,
		"gross_amount_usd": true, "discount_amount_usd": true,
	}
}
