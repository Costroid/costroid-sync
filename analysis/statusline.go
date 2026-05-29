package analysis

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

// Statusline holds the metadata-only metrics shown by the `statusline` command.
// It carries aggregate numbers only — never provider/model names, prompts,
// completions, or any other content. Nil pointers mark genuinely absent values
// so callers can render an omitted token / JSON null rather than a fabricated 0.
type Statusline struct {
	MTDCostUSD    float64  // month-to-date spend (current UTC calendar month)
	ForecastUSD   *float64 // nil when forecast data is insufficient
	BudgetPercent *int     // nil when no budget is configured
	OverBudget    bool     // true when spend exceeds the configured budget
	AnomalyCount  int      // anomalies within the current calendar month
}

// BudgetInput is the optional configured budget passed to BuildStatusline.
// nil means "no budget configured".
type BudgetInput = BudgetConfig

// BuildStatusline computes the statusline metrics from already-stored local
// records by reusing the existing forecast, budget, and anomaly calculations.
// It performs no I/O and reads only metadata fields. budget may be nil.
func BuildStatusline(records []providers.NormalizedCostRecord, budget *BudgetInput, now time.Time) Statusline {
	now = now.UTC()
	out := Statusline{
		MTDCostUSD:   monthToDateSpend(records, now),
		AnomalyCount: currentMonthAnomalyCount(records, now),
	}

	if forecast, err := Forecast(records, now); err == nil {
		f := forecast.ForecastMonthEndUSD
		out.ForecastUSD = &f
	} else if !errors.Is(err, ErrInsufficientData) {
		// Any other forecast error degrades to an omitted token rather than
		// failing the whole statusline; the metric is simply absent.
		out.ForecastUSD = nil
	}

	if budget != nil {
		if status, err := CheckBudget(*budget, records, now); err == nil {
			pct := int(math.Round(status.UsedPercent))
			out.BudgetPercent = &pct
			out.OverBudget = status.IsOverBudget
		}
	}
	return out
}

// monthToDateSpend sums CostUSD for records recorded in the current UTC month.
func monthToDateSpend(records []providers.NormalizedCostRecord, now time.Time) float64 {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	total := 0.0
	for _, r := range records {
		t, ok := parseRecordedAt(r.RecordedAt)
		if ok && !t.Before(start) && !t.After(now) {
			total += r.CostUSD
		}
	}
	return total
}

// currentMonthAnomalyCount counts detected anomalies whose date falls in the
// current calendar month. DetectAnomalies needs ~7 prior days of context, so
// callers pass a read window that extends before the month start.
func currentMonthAnomalyCount(records []providers.NormalizedCostRecord, now time.Time) int {
	prefix := now.Format("2006-01")
	count := 0
	for _, a := range DetectAnomalies(records) {
		if strings.HasPrefix(a.Date, prefix) {
			count++
		}
	}
	return count
}
