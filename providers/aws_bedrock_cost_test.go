package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const awsCostHappyResponse = `{
  "ResultsByTime": [{
    "TimePeriod": {"Start": "2026-05-20", "End": "2026-05-21"},
    "Groups": [{
      "Keys": ["Amazon Bedrock", "USE1-ModelInvocation-InputTokens"],
      "Metrics": {
        "UnblendedCost": {"Amount": "1.25", "Unit": "USD"},
        "UsageQuantity": {"Amount": "500", "Unit": "N/A"}
      }
    }]
  }]
}`

func TestAWSBedrock_CostQueryConstruction(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	p := newAWSBedrockTestProvider(ts)
	if _, err := p.Fetch(context.Background(), 7); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	req := ts.captured[0]
	if req.Path != "/" || req.Method != http.MethodPost {
		t.Fatalf("request = %s %s, want POST /", req.Method, req.Path)
	}
	if req.Target != awsCostExplorerTarget {
		t.Errorf("target = %q", req.Target)
	}
	if req.ContentType != "application/x-amz-json-1.1" {
		t.Errorf("content-type = %q", req.ContentType)
	}
	if req.SecurityToken != awsTestSession || req.AmzDate == "" || req.Authorization == "" {
		t.Errorf("signed headers missing: %+v", req)
	}
	assertAWSBedrockCostBody(t, req.Body)
}

func TestAWSBedrock_CostHappyPath(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(awsCostHappyResponse))
	}
	p := newAWSBedrockTestProvider(ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d: %+v", len(records), records)
	}
	assertAWSBedrockCostRecord(t, records[0])
}

func TestAWSBedrock_CostFiltersNonBedrockAndNonUSD(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-05-20"},"Groups":[
			{"Keys":["Amazon Bedrock","use1-good"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"},"UsageQuantity":{"Amount":"2","Unit":"N/A"}}},
			{"Keys":["Amazon EC2","ec2"],"Metrics":{"UnblendedCost":{"Amount":"9","Unit":"USD"},"UsageQuantity":{"Amount":"1","Unit":"N/A"}}},
			{"Keys":["Amazon Bedrock","eur"],"Metrics":{"UnblendedCost":{"Amount":"3","Unit":"EUR"},"UsageQuantity":{"Amount":"1","Unit":"N/A"}}}
		]}]}`))
	}
	p := newAWSBedrockTestProvider(ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 || records[0].SKU != "use1-good" {
		t.Fatalf("unexpected records: %+v", records)
	}
}

func TestAWSBedrock_CostPagination(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	calls := 0
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"NextPageToken":"P2","ResultsByTime":[]}`)
			return
		}
		_, _ = w.Write([]byte(awsCostHappyResponse))
	}
	p := newAWSBedrockTestProvider(ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if calls != 2 || len(records) != 1 {
		t.Fatalf("calls=%d records=%d", calls, len(records))
	}
	if !strings.Contains(ts.captured[1].Body, `"NextPageToken":"P2"`) {
		t.Errorf("second request missing next token: %s", ts.captured[1].Body)
	}
}

func assertAWSBedrockCostBody(t *testing.T, body string) {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if decoded["Granularity"] != "DAILY" {
		t.Errorf("Granularity = %v", decoded["Granularity"])
	}
	wantTime := map[string]string{"Start": "2026-05-18", "End": "2026-05-25"}
	gotTime := decoded["TimePeriod"].(map[string]any)
	for k, want := range wantTime {
		if gotTime[k] != want {
			t.Errorf("TimePeriod.%s = %v, want %s", k, gotTime[k], want)
		}
	}
	assertJSONContains(t, decoded["Filter"], "SERVICE", "Amazon Bedrock")
	assertJSONContains(t, decoded["GroupBy"], "SERVICE", "USAGE_TYPE")
	assertJSONContains(t, decoded["Metrics"], "UnblendedCost", "UsageQuantity")
}

func assertAWSBedrockCostRecord(t *testing.T, r NormalizedCostRecord) {
	t.Helper()
	if r.Provider != awsBedrockProviderName || r.ProjectID != awsTestAccount {
		t.Fatalf("wrong provider/project: %+v", r)
	}
	if r.Model != "USE1-ModelInvocation-InputTokens" || r.SKU != r.Model {
		t.Errorf("wrong model/sku: %+v", r)
	}
	if r.CostUSD != 1.25 || r.GrossAmountUSD != 1.25 {
		t.Errorf("wrong cost: %+v", r)
	}
	if r.UsageQuantity != 500 || r.UnitType != "N/A" {
		t.Errorf("wrong usage metadata: %+v", r)
	}
	if r.PromptTokens != 0 || r.CompletionTokens != 0 || r.TotalTokens != 0 {
		t.Errorf("Cost Explorer quantity must not map to tokens: %+v", r)
	}
	if r.RecordedAt != time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC).Format(time.RFC3339) {
		t.Errorf("RecordedAt = %q", r.RecordedAt)
	}
}

func assertJSONContains(t *testing.T, v any, wants ...string) {
	t.Helper()
	js, _ := json.Marshal(v)
	for _, want := range wants {
		if !strings.Contains(string(js), want) {
			t.Errorf("JSON missing %q: %s", want, js)
		}
	}
}
