package analysis

import (
	"reflect"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

func aggTestRecords() []providers.NormalizedCostRecord {
	return []providers.NormalizedCostRecord{
		{Provider: "openai", Model: "gpt-4o", CostUSD: 3.00, TotalTokens: 300, RecordedAt: "2026-05-20T10:00:00Z"},
		{Provider: "openai", Model: "gpt-4o", CostUSD: 1.00, TotalTokens: 100, RecordedAt: "2026-05-21T10:00:00Z"},
		{Provider: "openai", Model: "gpt-4o-mini", CostUSD: 0.50, TotalTokens: 500, RecordedAt: "2026-05-22T10:00:00Z"},
		{Provider: "anthropic", Model: "claude-4", CostUSD: 5.00, TotalTokens: 200, RecordedAt: "2026-05-19T10:00:00Z"},
	}
}

func TestAggregateByProvider(t *testing.T) {
	got := AggregateByProvider(aggTestRecords())
	want := []ProviderTotal{
		{Provider: "anthropic", CostUSD: 5.00, TotalTokens: 200, Records: 1},
		{Provider: "openai", CostUSD: 4.50, TotalTokens: 900, Records: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AggregateByProvider:\n got %+v\nwant %+v", got, want)
	}
}

func TestAggregateByModel(t *testing.T) {
	got := AggregateByModel(aggTestRecords())
	want := []ModelTotal{
		{Provider: "anthropic", Model: "claude-4", CostUSD: 5.00, TotalTokens: 200, Records: 1},
		{Provider: "openai", Model: "gpt-4o", CostUSD: 4.00, TotalTokens: 400, Records: 2},
		{Provider: "openai", Model: "gpt-4o-mini", CostUSD: 0.50, TotalTokens: 500, Records: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AggregateByModel:\n got %+v\nwant %+v", got, want)
	}
}

func TestLatestActivityByProvider(t *testing.T) {
	got := LatestActivityByProvider(aggTestRecords())
	want := []ProviderActivity{
		{Provider: "openai", LatestActive: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)},
		{Provider: "anthropic", LatestActive: time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LatestActivityByProvider:\n got %+v\nwant %+v", got, want)
	}
}

// TestLatestActivity_UnparseableSinksLast verifies a provider whose timestamps
// never parse keeps the zero time and sorts after providers with real activity.
func TestLatestActivity_UnparseableSinksLast(t *testing.T) {
	recs := []providers.NormalizedCostRecord{
		{Provider: "good", RecordedAt: "2026-05-20T10:00:00Z"},
		{Provider: "bad", RecordedAt: "not-a-timestamp"},
	}
	got := LatestActivityByProvider(recs)
	if len(got) != 2 || got[0].Provider != "good" || got[1].Provider != "bad" {
		t.Fatalf("unexpected order: %+v", got)
	}
	if !got[1].LatestActive.IsZero() {
		t.Errorf("bad provider should have zero LatestActive, got %v", got[1].LatestActive)
	}
}

// TestAggregate_MetadataOnly is the metadata-only guard: the aggregation result
// types must expose only provider/model identifiers, counts, and money — never
// a field that could carry prompt/completion/message/content text.
func TestAggregate_MetadataOnly(t *testing.T) {
	forbidden := map[string]bool{
		"Prompt": true, "Completion": true, "Message": true, "Messages": true,
		"Content": true, "Body": true, "Payload": true, "Text": true, "Raw": true,
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ProviderTotal{}),
		reflect.TypeOf(ModelTotal{}),
		reflect.TypeOf(ProviderActivity{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			if forbidden[typ.Field(i).Name] {
				t.Errorf("%s has forbidden field %q", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
