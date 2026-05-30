package analysis

import (
	"testing"
	"time"

	"github.com/costroid/costroid/providers"
)

// dailyRecords builds one record per consecutive UTC day starting at start,
// using the supplied per-day costs. Metadata only — no content fields exist.
func dailyRecords(start time.Time, costs ...float64) []providers.NormalizedCostRecord {
	recs := make([]providers.NormalizedCostRecord, 0, len(costs))
	for i, c := range costs {
		ts := start.AddDate(0, 0, i).UTC().Format(time.RFC3339)
		recs = append(recs, providers.NormalizedCostRecord{
			Provider: "openai", Model: "m", RecordedAt: ts, CostUSD: c, TotalTokens: 1,
		})
	}
	return recs
}

func TestBuildStatusline_Populated(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// May 1..8: seven $1 baseline days then a $10 spike (anomaly on May 8).
	records := dailyRecords(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		1, 1, 1, 1, 1, 1, 1, 10)
	budget := &BudgetConfig{AmountUSD: 100, Period: BudgetMonthly}

	got := BuildStatusline(records, budget, now)

	if got.MTDCostUSD != 17 {
		t.Errorf("MTDCostUSD = %v, want 17", got.MTDCostUSD)
	}
	if got.ForecastUSD == nil {
		t.Errorf("ForecastUSD = nil, want non-nil with enough history")
	}
	if got.BudgetPercent == nil || *got.BudgetPercent != 17 {
		t.Errorf("BudgetPercent = %v, want 17", got.BudgetPercent)
	}
	if got.OverBudget {
		t.Errorf("OverBudget = true, want false")
	}
	if got.AnomalyCount != 1 {
		t.Errorf("AnomalyCount = %d, want 1", got.AnomalyCount)
	}
}

func TestBuildStatusline_OverBudget(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	records := dailyRecords(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 1, 1, 1, 1, 1, 1, 1, 10)
	budget := &BudgetConfig{AmountUSD: 10, Period: BudgetMonthly}

	got := BuildStatusline(records, budget, now)

	if got.BudgetPercent == nil || *got.BudgetPercent != 170 {
		t.Errorf("BudgetPercent = %v, want 170", got.BudgetPercent)
	}
	if !got.OverBudget {
		t.Errorf("OverBudget = false, want true")
	}
}

func TestBuildStatusline_InsufficientForecast(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	records := dailyRecords(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 5)

	got := BuildStatusline(records, nil, now)

	if got.MTDCostUSD != 5 {
		t.Errorf("MTDCostUSD = %v, want 5", got.MTDCostUSD)
	}
	if got.ForecastUSD != nil {
		t.Errorf("ForecastUSD = %v, want nil with one day of data", *got.ForecastUSD)
	}
	if got.BudgetPercent != nil {
		t.Errorf("BudgetPercent = %v, want nil (no budget)", *got.BudgetPercent)
	}
	if got.AnomalyCount != 0 {
		t.Errorf("AnomalyCount = %d, want 0", got.AnomalyCount)
	}
}

func TestBuildStatusline_AnomalyScopedToCurrentMonth(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	// Contiguous Apr 15 → May 5 with an April spike and a May spike.
	costs := make([]float64, 21)
	for i := range costs {
		costs[i] = 1
	}
	costs[8] = 10  // Apr 23 → April anomaly (excluded from current-month count)
	costs[20] = 10 // May 5  → May anomaly (counted)
	records := dailyRecords(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), costs...)

	got := BuildStatusline(records, nil, now)

	if got.AnomalyCount != 1 {
		t.Errorf("AnomalyCount = %d, want 1 (only the May anomaly counts)", got.AnomalyCount)
	}
}
