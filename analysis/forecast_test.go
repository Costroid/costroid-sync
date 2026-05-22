package analysis

import (
	"errors"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

func TestForecast_LinearThirtyDaysWithinFivePercent(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	var records []providers.NormalizedCostRecord
	for day := 1; day <= 30; day++ {
		records = append(records, testCostRecord(2026, 5, day, float64(day), 100))
	}

	got, err := Forecast(records, now)
	if err != nil {
		t.Fatalf("Forecast error: %v", err)
	}
	want := 496.0
	if !approxEqual(got.ForecastMonthEndUSD, want, want*0.05) {
		t.Errorf("forecast = %v, want within 5%% of %v", got.ForecastMonthEndUSD, want)
	}
	if got.CurrentMonthSpendUSD != 465 || got.Method != "linear_regression" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestForecast_InsufficientData(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	_, err := Forecast([]providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 1, 1),
	}, now)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("want ErrInsufficientData, got %v", err)
	}
}

func TestForecast_EmptyAndZeroDoNotPanic(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	if _, err := Forecast(nil, now); !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("empty: want ErrInsufficientData, got %v", err)
	}

	records := []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 0, 10),
		testCostRecord(2026, 5, 2, 0, 10),
		testCostRecord(2026, 5, 3, 0, 10),
	}
	got, err := Forecast(records, now)
	if err != nil {
		t.Fatalf("zero-cost forecast should not error: %v", err)
	}
	if got.ForecastMonthEndUSD != 0 || got.CurrentMonthSpendUSD != 0 {
		t.Errorf("zero-cost forecast should stay zero: %+v", got)
	}
}

func TestForecast_IgnoresPriorMonth(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	_, err := Forecast([]providers.NormalizedCostRecord{
		testCostRecord(2026, 4, 30, 100, 100),
	}, now)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("want ErrInsufficientData, got %v", err)
	}
}

func testCostRecord(year int, month time.Month, day int, cost float64, tokens int) providers.NormalizedCostRecord {
	t := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
	return providers.NormalizedCostRecord{
		Provider:    "openai",
		Model:       "gpt-4o",
		TotalTokens: tokens,
		CostUSD:     cost,
		RecordedAt:  t.Format(time.RFC3339),
		SourceHash:  providers.ComputeSourceHash("openai", t.Format(time.RFC3339), "gpt-4o", "", ""),
	}
}
