package analysis

import (
	"testing"
	"time"

	"github.com/costroid/costroid/providers"
)

func TestDailyTotals_AggregatesAndZeroFills(t *testing.T) {
	records := []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 1.00, 10),
		testCostRecord(2026, 5, 1, 0.50, 5),  // same day → summed
		testCostRecord(2026, 5, 3, 4.00, 30), // gap on the 2nd → zero-filled
	}

	got := DailyTotals(records)
	if len(got) != 3 {
		t.Fatalf("want 3 days (1st..3rd inclusive), got %d: %+v", len(got), got)
	}

	want := []DailyTotal{
		{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), CostUSD: 1.50, TotalTokens: 15},
		{Date: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), CostUSD: 0, TotalTokens: 0},
		{Date: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), CostUSD: 4.00, TotalTokens: 30},
	}
	for i, w := range want {
		if !got[i].Date.Equal(w.Date) || got[i].CostUSD != w.CostUSD || got[i].TotalTokens != w.TotalTokens {
			t.Errorf("day %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestDailyTotals_EmptyInput(t *testing.T) {
	if got := DailyTotals(nil); got != nil {
		t.Errorf("DailyTotals(nil) = %+v, want nil", got)
	}
}
