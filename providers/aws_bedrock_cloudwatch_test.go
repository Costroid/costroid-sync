package providers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAWSBedrock_CloudWatchEnrichmentHappyPath(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(awsCostHappyResponse))
	}
	ts.cloudHandler = cloudWatchHappyHandler(t, "anthropic.claude-3-sonnet")
	p := newAWSBedrockTestProvider(ts)
	p.MetricRegions = []string{"us-east-1"}

	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Model != "anthropic.claude-3-sonnet" {
		t.Errorf("Model = %q", r.Model)
	}
	if r.PromptTokens != 1000 || r.CompletionTokens != 500 || r.TotalTokens != 1500 {
		t.Errorf("tokens not enriched: %+v", r)
	}
	assertCloudWatchCalls(t, ts.captured)
}

func TestAWSBedrock_CloudWatchMissingDataCostOnly(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(awsCostHappyResponse))
	}
	ts.cloudHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Metrics":[]}`))
	}
	p := newAWSBedrockTestProvider(ts)
	p.MetricRegions = []string{"us-east-1"}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 || records[0].TotalTokens != 0 {
		t.Fatalf("want cost-only record, got %+v", records)
	}
}

func TestAWSBedrock_CloudWatchFailureNonFatal(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(awsCostHappyResponse))
	}
	ts.cloudHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"POISON_BODY ` + awsTestSecret + `"}`))
	}
	p := newAWSBedrockTestProvider(ts)
	p.MetricRegions = []string{"us-east-1"}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("CloudWatch failure should be non-fatal: %v", err)
	}
	if len(records) != 1 || records[0].TotalTokens != 0 {
		t.Fatalf("want cost-only record, got %+v", records)
	}
}

func TestAWSBedrock_CloudWatchAmbiguousLeavesZero(t *testing.T) {
	ts := newAWSBedrockTestServer(t)
	ts.costHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(twoBedrockCostRows()))
	}
	ts.cloudHandler = cloudWatchTwoModelHandler(t)
	p := newAWSBedrockTestProvider(ts)
	p.MetricRegions = []string{"us-east-1"}
	records, err := p.Fetch(context.Background(), 1)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, r := range records {
		if r.TotalTokens != 0 || strings.HasPrefix(r.Model, "model-") {
			t.Fatalf("ambiguous join populated tokens/model: %+v", records)
		}
	}
}

func cloudWatchHappyHandler(t *testing.T, model string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Amz-Target") {
		case awsCloudWatchListTarget:
			fmt.Fprintf(w, `{"Metrics":[{"Namespace":"AWS/Bedrock","MetricName":"InputTokenCount","Dimensions":[{"Name":"ModelId","Value":%q}]}]}`, model)
		case awsCloudWatchDataTarget:
			fmt.Fprintf(w, `{"MetricDataResults":[
				{"Id":"i0","Timestamps":[%d],"Values":[1000]},
				{"Id":"o0","Timestamps":[%d],"Values":[500]}
			]}`, metricUnix(), metricUnix())
		default:
			t.Fatalf("unexpected CloudWatch target %q", r.Header.Get("X-Amz-Target"))
		}
	}
}

func cloudWatchTwoModelHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") == awsCloudWatchListTarget {
			_, _ = w.Write([]byte(`{"Metrics":[
				{"Dimensions":[{"Name":"ModelId","Value":"model-a"}]},
				{"Dimensions":[{"Name":"ModelId","Value":"model-b"}]}
			]}`))
			return
		}
		fmt.Fprintf(w, `{"MetricDataResults":[
			{"Id":"i0","Timestamps":[%d],"Values":[100]},
			{"Id":"o1","Timestamps":[%d],"Values":[200]}
		]}`, metricUnix(), metricUnix())
	}
}

func twoBedrockCostRows() string {
	return `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-05-20"},"Groups":[
		{"Keys":["Amazon Bedrock","usage-a"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"},"UsageQuantity":{"Amount":"1","Unit":"N/A"}}},
		{"Keys":["Amazon Bedrock","usage-b"],"Metrics":{"UnblendedCost":{"Amount":"2","Unit":"USD"},"UsageQuantity":{"Amount":"1","Unit":"N/A"}}}
	]}]}`
}

func assertCloudWatchCalls(t *testing.T, reqs []awsBedrockRecordedReq) {
	t.Helper()
	foundData := false
	for _, r := range reqs {
		if r.Target == awsCloudWatchDataTarget {
			foundData = true
			if r.ContentEncoding != "amz-1.0" || r.ContentType != "application/json" {
				t.Errorf("bad CloudWatch headers: %+v", r)
			}
		}
	}
	if !foundData {
		t.Fatal("GetMetricData call not captured")
	}
}

func metricUnix() int64 {
	return time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC).Unix()
}
