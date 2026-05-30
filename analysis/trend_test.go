package analysis

import (
	"testing"

	"github.com/costroid/costroid/providers"
)

func TestTrends_WeeklyAggregation(t *testing.T) {
	records := []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 18, 1.25, 10),
		testCostRecord(2026, 5, 19, 2.75, 20),
		testCostRecord(2026, 5, 25, 6.00, 30),
	}

	got := Trends(records, TrendWeekly)
	if len(got) != 2 {
		t.Fatalf("want 2 periods, got %+v", got)
	}
	if got[0].Period != "2026-W21" || got[0].CostUSD != 4.00 || got[0].TotalTokens != 30 {
		t.Errorf("weekly first period wrong: %+v", got[0])
	}
	if got[1].Period != "2026-W22" || got[1].CostUSD != 6.00 || got[1].TotalTokens != 30 {
		t.Errorf("weekly second period wrong: %+v", got[1])
	}
	if !got[1].HasChange || !approxEqual(got[1].ChangePercent, 50.0, 1e-9) {
		t.Errorf("weekly change wrong: %+v", got[1])
	}
}

func TestTrends_MonthlyAggregation(t *testing.T) {
	records := []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 1.00, 10),
		testCostRecord(2026, 5, 31, 2.00, 20),
		testCostRecord(2026, 6, 1, 4.00, 30),
	}

	got := Trends(records, TrendMonthly)
	if len(got) != 2 {
		t.Fatalf("want 2 periods, got %+v", got)
	}
	if got[0].Period != "2026-05" || got[0].CostUSD != 3.00 || got[0].TotalTokens != 30 {
		t.Errorf("monthly first period wrong: %+v", got[0])
	}
	if got[1].Period != "2026-06" || got[1].CostUSD != 4.00 || got[1].TotalTokens != 30 {
		t.Errorf("monthly second period wrong: %+v", got[1])
	}
}

func TestTrends_PreviousZeroHasNoChange(t *testing.T) {
	records := []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 0.00, 10),
		testCostRecord(2026, 6, 1, 4.00, 20),
	}

	got := Trends(records, TrendMonthly)
	if len(got) != 2 {
		t.Fatalf("want 2 periods, got %+v", got)
	}
	if got[1].HasChange {
		t.Errorf("previous zero should suppress percent change: %+v", got[1])
	}
}
