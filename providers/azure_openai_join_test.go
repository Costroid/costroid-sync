package providers

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

// Azure Monitor enrichment tests. These exercise the optional join from
// azure_openai_join.go: cost rows enriched with prompt/completion/total
// token counts when AZURE_OPENAI_RESOURCE_IDS is set and Monitor returns
// data that can be safely matched.

const azureMonitorHappyResponse = `{"value":[
  {
    "name":{"value":"ProcessedPromptTokens"},
    "timeseries":[{
      "metadatavalues":[{"name":{"value":"ModelDeploymentName"},"value":"my-gpt4o"},{"name":{"value":"ModelName"},"value":"gpt-4o"}],
      "data":[{"timeStamp":"2026-05-20T00:00:00Z","total":1000}]
    }]
  },
  {
    "name":{"value":"GeneratedTokens"},
    "timeseries":[{
      "metadatavalues":[{"name":{"value":"ModelDeploymentName"},"value":"my-gpt4o"},{"name":{"value":"ModelName"},"value":"gpt-4o"}],
      "data":[{"timeStamp":"2026-05-20T00:00:00Z","total":500}]
    }]
  },
  {
    "name":{"value":"TotalTokens"},
    "timeseries":[{
      "metadatavalues":[{"name":{"value":"ModelDeploymentName"},"value":"my-gpt4o"},{"name":{"value":"ModelName"},"value":"gpt-4o"}],
      "data":[{"timeStamp":"2026-05-20T00:00:00Z","total":1500}]
    }]
  }
]}`

func TestAzure_MonitorEnrichmentHappyPath(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"properties":{"columns":[
			{"name":"totalCost","type":"Number"},
			{"name":"totalUsage","type":"Number"},
			{"name":"UsageDate","type":"Number"},
			{"name":"ResourceId","type":"String"},
			{"name":"ServiceName","type":"String"},
			{"name":"MeterCategory","type":"String"},
			{"name":"Meter","type":"String"},
			{"name":"MeterSubCategory","type":"String"},
			{"name":"UnitOfMeasure","type":"String"},
			{"name":"Currency","type":"String"}
		],"rows":[
			[1.25, 500, 20260520, "` + azureTestResource + `", "Azure OpenAI", "Azure OpenAI", "GPT-4o Input Tokens", "Azure OpenAI - GPT-4o", "1K Tokens", "USD"]
		]}}`))
	}
	ts.monitorHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(azureMonitorHappyResponse))
	}
	p := newAzureTestProvider(t, ts)
	p.ResourceIDs = []string{azureTestResource}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.PromptTokens != 1000 {
		t.Errorf("PromptTokens = %d, want 1000", r.PromptTokens)
	}
	if r.CompletionTokens != 500 {
		t.Errorf("CompletionTokens = %d, want 500", r.CompletionTokens)
	}
	if r.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", r.TotalTokens)
	}
	if atomic.LoadInt32(&ts.monitorCalls) != 1 {
		t.Errorf("monitor called %d times, want 1", ts.monitorCalls)
	}
}

func TestAzure_MonitorAmbiguousLeavesZero(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
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
			[1.25, 20260520, "` + azureTestResource + `", "Azure OpenAI", "Azure OpenAI", "GPT-4o Input Tokens", "Azure OpenAI - GPT-4o", "USD"],
			[0.50, 20260520, "` + azureTestResource + `", "Azure OpenAI", "Azure OpenAI", "Custom Meter", "Generic Service", "USD"]
		]}}`))
	}
	ts.monitorHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[
			{"name":{"value":"ProcessedPromptTokens"},"timeseries":[
				{"metadatavalues":[{"name":{"value":"ModelName"},"value":"gpt-4o-mini"}],"data":[{"timeStamp":"2026-05-20T00:00:00Z","total":111}]},
				{"metadatavalues":[{"name":{"value":"ModelName"},"value":"gpt-4o"}],"data":[{"timeStamp":"2026-05-20T00:00:00Z","total":222}]}
			]}
		]}`))
	}
	p := newAzureTestProvider(t, ts)
	p.ResourceIDs = []string{azureTestResource}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	var gpt4o, generic NormalizedCostRecord
	for _, r := range records {
		if r.Model == "gpt-4o" {
			gpt4o = r
		} else {
			generic = r
		}
	}
	if gpt4o.PromptTokens != 222 {
		t.Errorf("gpt-4o PromptTokens = %d, want 222 (exact-model join)", gpt4o.PromptTokens)
	}
	if generic.PromptTokens != 0 {
		t.Errorf("generic PromptTokens = %d, want 0 (ambiguous)", generic.PromptTokens)
	}
}

func TestAzure_MonitorFetchFailureNonFatal(t *testing.T) {
	ts := newAzureTestServer(t)
	ts.monitorHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"transient"}`))
	}
	p := newAzureTestProvider(t, ts)
	p.ResourceIDs = []string{azureTestResource}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch should not propagate Monitor 503, got error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 cost records (Compute filtered), got %d", len(records))
	}
	for _, r := range records {
		if r.PromptTokens != 0 || r.CompletionTokens != 0 || r.TotalTokens != 0 {
			t.Errorf("Monitor failed but tokens populated: %+v", r)
		}
	}
}
