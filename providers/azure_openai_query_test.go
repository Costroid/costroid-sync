package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAzure_CostQueryConstruction(t *testing.T) {
	ts := newAzureTestServer(t)
	p := newAzureTestProvider(t, ts)
	if _, err := p.Fetch(context.Background(), 7); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ts.capturedCost) == 0 {
		t.Fatal("cost endpoint never called")
	}
	r := ts.capturedCost[0]
	wantPath := "/subscriptions/" + azureTestSubscription + "/providers/Microsoft.CostManagement/query"
	if r.Path != wantPath {
		t.Errorf("path = %q, want %q", r.Path, wantPath)
	}
	if !strings.Contains(r.Query, "api-version="+azureCostAPIVersion) {
		t.Errorf("query missing api-version: %s", r.Query)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		t.Fatalf("body not JSON: %v\n%s", err, r.Body)
	}
	if body["type"] != "ActualCost" {
		t.Errorf("type = %v", body["type"])
	}
	if body["timeframe"] != "Custom" {
		t.Errorf("timeframe = %v", body["timeframe"])
	}
	tp := body["timePeriod"].(map[string]any)
	if tp["from"] == "" || tp["to"] == "" {
		t.Errorf("timePeriod missing from/to: %v", tp)
	}
	dataset := body["dataset"].(map[string]any)
	if dataset["granularity"] != "Daily" {
		t.Errorf("granularity = %v", dataset["granularity"])
	}
	assertAzureCostQueryFilter(t, dataset)
	assertAzureCostQueryGrouping(t, dataset)
}

func TestAzure_CostHappyPath(t *testing.T) {
	ts := newAzureTestServer(t)
	p := newAzureTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records (Compute filtered), got %d: %+v", len(records), records)
	}
	for _, r := range records {
		assertAzureCostRecordDefaults(t, r)
	}
	assertAzureGPT4oCostRecord(t, findAzureRecordByModel(records, "gpt-4o"))
}

func TestAzure_NonUSDSkipped(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"columns":[
			{"name":"totalCost","type":"Number"},
			{"name":"UsageDate","type":"Number"},
			{"name":"ResourceId","type":"String"},
			{"name":"ServiceName","type":"String"},
			{"name":"MeterCategory","type":"String"},
			{"name":"Currency","type":"String"}
		],"rows":[
			[1.50, 20260520, "/subscriptions/sub/rg/x", "Azure OpenAI", "Azure OpenAI", "USD"],
			[2.00, 20260520, "/subscriptions/sub/rg/y", "Azure OpenAI", "Azure OpenAI", "EUR"]
		]}}`))
	}
	p := newAzureTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 USD record, got %d: %+v", len(records), records)
	}
	if records[0].CostUSD != 1.50 {
		t.Errorf("CostUSD = %v, want 1.50 (the USD row)", records[0].CostUSD)
	}
}

func TestAzure_PaginationFollowsNextLink(t *testing.T) {
	ts := newAzureTestServer(t)
	var costCallCount int32
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&costCallCount, 1)
		if n == 1 {
			next := ts.server.URL + "/subscriptions/" + azureTestSubscription + "/providers/Microsoft.CostManagement/query?api-version=" + azureCostAPIVersion + "&$skiptoken=PAGE2"
			fmt.Fprintf(w, `{"properties":{"nextLink":%q,"columns":[
				{"name":"totalCost","type":"Number"},
				{"name":"UsageDate","type":"Number"},
				{"name":"ResourceId","type":"String"},
				{"name":"ServiceName","type":"String"},
				{"name":"MeterCategory","type":"String"},
				{"name":"Meter","type":"String"},
				{"name":"MeterSubCategory","type":"String"},
				{"name":"Currency","type":"String"}
			],"rows":[
				[1.00, 20260520, "/subscriptions/sub/rg/page1", "Azure OpenAI", "Azure OpenAI", "GPT-4o Input", "GPT-4o", "USD"]
			]}}`, next)
			return
		}
		_, _ = w.Write([]byte(`{"properties":{"columns":[
			{"name":"totalCost","type":"Number"},
			{"name":"UsageDate","type":"Number"},
			{"name":"ResourceId","type":"String"},
			{"name":"ServiceName","type":"String"},
			{"name":"MeterCategory","type":"String"},
			{"name":"Meter","type":"String"},
			{"name":"MeterSubCategory","type":"String"},
			{"name":"Currency","type":"String"}
		],"rows":[
			[2.00, 20260520, "/subscriptions/sub/rg/page2", "Azure OpenAI", "Azure OpenAI", "GPT-4o Output", "GPT-4o", "USD"]
		]}}`))
	}
	p := newAzureTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records (one per page), got %d", len(records))
	}
	if got := atomic.LoadInt32(&ts.costCalls); got != 2 {
		t.Errorf("cost endpoint called %d times, want 2 (two pages)", got)
	}
}

func TestAzure_DefenseInDepthFilter(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"columns":[
			{"name":"totalCost","type":"Number"},
			{"name":"UsageDate","type":"Number"},
			{"name":"ResourceId","type":"String"},
			{"name":"ServiceName","type":"String"},
			{"name":"MeterCategory","type":"String"},
			{"name":"Currency","type":"String"}
		],"rows":[
			[1.00, 20260520, "/subscriptions/sub/rg/ml", "Machine Learning", "Machine Learning", "USD"],
			[2.00, 20260520, "/subscriptions/sub/rg/openai", "Azure OpenAI", "Azure OpenAI", "USD"]
		]}}`))
	}
	p := newAzureTestProvider(t, ts)
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 (Machine Learning filtered client-side), got %d: %+v", len(records), records)
	}
	if records[0].ProjectID != "/subscriptions/sub/rg/openai" {
		t.Errorf("wrong row survived: %+v", records[0])
	}
}

func TestAzure_DaysClampedTo366(t *testing.T) {
	for _, tc := range []struct {
		days     int
		wantDiff int
	}{
		{days: 10000, wantDiff: 365},
		{days: 366, wantDiff: 365},
		{days: 0, wantDiff: 0},
		{days: -5, wantDiff: 0},
	} {
		t.Run(fmt.Sprintf("days=%d", tc.days), func(t *testing.T) {
			ts := newAzureTestServer(t)
			p := newAzureTestProvider(t, ts)
			if _, err := p.Fetch(context.Background(), tc.days); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(ts.capturedCost) == 0 {
				t.Fatal("no cost call captured")
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(ts.capturedCost[0].Body), &body); err != nil {
				t.Fatalf("body: %v", err)
			}
			tp := body["timePeriod"].(map[string]any)
			from, _ := time.Parse(time.RFC3339, tp["from"].(string))
			to, _ := time.Parse(time.RFC3339, tp["to"].(string))
			gotDays := int(to.Sub(from).Hours() / 24)
			if gotDays != tc.wantDiff {
				t.Errorf("from=%s to=%s diff=%d days, want %d", from, to, gotDays, tc.wantDiff)
			}
		})
	}
}

func assertAzureCostQueryFilter(t *testing.T, dataset map[string]any) {
	t.Helper()
	filterJSON, _ := json.Marshal(dataset["filter"])
	if !strings.Contains(string(filterJSON), "MeterCategory") ||
		!strings.Contains(string(filterJSON), "Azure OpenAI") ||
		!strings.Contains(string(filterJSON), "Cognitive Services") {
		t.Errorf("filter missing expected MeterCategory restriction: %s", filterJSON)
	}
}

func assertAzureCostQueryGrouping(t *testing.T, dataset map[string]any) {
	t.Helper()
	groupingJSON, _ := json.Marshal(dataset["grouping"])
	for _, dim := range []string{"ResourceId", "ResourceGroup", "ServiceName", "Meter", "MeterCategory", "MeterSubCategory", "UnitOfMeasure"} {
		if !strings.Contains(string(groupingJSON), dim) {
			t.Errorf("grouping missing %q: %s", dim, groupingJSON)
		}
	}
}

func assertAzureCostRecordDefaults(t *testing.T, r NormalizedCostRecord) {
	t.Helper()
	if r.Provider != azureProviderName {
		t.Errorf("Provider = %q", r.Provider)
	}
	if r.APIKeyID != "" {
		t.Errorf("APIKeyID should be empty, got %q", r.APIKeyID)
	}
	if r.PromptTokens != 0 || r.CompletionTokens != 0 || r.TotalTokens != 0 {
		t.Errorf("tokens should be 0 without Monitor: %+v", r)
	}
	if r.SourceHash == "" {
		t.Errorf("SourceHash empty: %+v", r)
	}
	if r.RecordedAt != "2026-05-20T00:00:00Z" {
		t.Errorf("RecordedAt = %q", r.RecordedAt)
	}
}

func findAzureRecordByModel(records []NormalizedCostRecord, model string) NormalizedCostRecord {
	for _, r := range records {
		if r.Model == model {
			return r
		}
	}
	return NormalizedCostRecord{}
}

func assertAzureGPT4oCostRecord(t *testing.T, r NormalizedCostRecord) {
	t.Helper()
	if r.CostUSD != 1.25 {
		t.Errorf("gpt-4o CostUSD = %v, want 1.25", r.CostUSD)
	}
	if r.Product != "Azure OpenAI" {
		t.Errorf("gpt-4o Product = %q", r.Product)
	}
	if r.SKU != "Azure OpenAI - GPT-4o" {
		t.Errorf("gpt-4o SKU = %q", r.SKU)
	}
	if r.UnitType != "1K Tokens" {
		t.Errorf("gpt-4o UnitType = %q", r.UnitType)
	}
	if r.UsageQuantity != 500 {
		t.Errorf("gpt-4o UsageQuantity = %v", r.UsageQuantity)
	}
	if r.ProjectID != "/subscriptions/sub/rg/myacct" {
		t.Errorf("gpt-4o ProjectID = %q", r.ProjectID)
	}
}
