package analysis

import (
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

func TestDetectAnomalies_FindsSpike(t *testing.T) {
	records := steadyRecords(2026, 5, 1, 7, 10)
	records = append(records, testCostRecord(2026, 5, 8, 30, 100))

	got := DetectAnomalies(records)
	if len(got) != 1 {
		t.Fatalf("want 1 anomaly, got %+v", got)
	}
	if got[0].Date != "2026-05-08" || got[0].ActualCostUSD != 30 {
		t.Errorf("unexpected anomaly: %+v", got[0])
	}
}

func TestDetectAnomalies_SteadyDataNoAnomalies(t *testing.T) {
	got := DetectAnomalies(steadyRecords(2026, 5, 1, 14, 10))
	if len(got) != 0 {
		t.Fatalf("want no anomalies, got %+v", got)
	}
}

func TestDetectAnomalies_ZeroCostNoDivideByZero(t *testing.T) {
	var records []providers.NormalizedCostRecord
	for day := 1; day <= 10; day++ {
		records = append(records, testCostRecord(2026, 5, day, 0, 100))
	}
	got := DetectAnomalies(records)
	if len(got) != 0 {
		t.Fatalf("want no anomalies for zero baseline, got %+v", got)
	}
}

func TestDetectAnomalies_RequiresSevenPriorDays(t *testing.T) {
	records := steadyRecords(2026, 5, 1, 3, 10)
	records = append(records, testCostRecord(2026, 5, 4, 100, 100))

	got := DetectAnomalies(records)
	if len(got) != 0 {
		t.Fatalf("want no early anomaly, got %+v", got)
	}
}

func steadyRecords(year int, month time.Month, startDay, days int, cost float64) []providers.NormalizedCostRecord {
	records := make([]providers.NormalizedCostRecord, 0, days)
	for i := 0; i < days; i++ {
		records = append(records, testCostRecord(year, month, startDay+i, cost, 100))
	}
	return records
}
