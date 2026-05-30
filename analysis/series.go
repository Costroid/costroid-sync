package analysis

import (
	"sort"
	"time"

	"github.com/costroid/costroid/providers"
)

const dateLayout = "2006-01-02"

type dailyTotal struct {
	Date        time.Time
	CostUSD     float64
	TotalTokens int
}

func costsByDate(records []providers.NormalizedCostRecord) map[time.Time]dailyTotal {
	out := map[time.Time]dailyTotal{}
	for _, r := range records {
		t, ok := parseRecordedAt(r.RecordedAt)
		if !ok {
			continue
		}
		date := dateOnly(t)
		d := out[date]
		d.Date = date
		d.CostUSD += r.CostUSD
		d.TotalTokens += r.TotalTokens
		out[date] = d
	}
	return out
}

func completeDailySeries(records []providers.NormalizedCostRecord) []dailyTotal {
	totals := costsByDate(records)
	if len(totals) == 0 {
		return nil
	}
	dates := sortedDailyKeys(totals)
	return fillDailyTotals(dates[0], dates[len(dates)-1], totals)
}

func sortedDailyKeys(totals map[time.Time]dailyTotal) []time.Time {
	keys := make([]time.Time, 0, len(totals))
	for d := range totals {
		keys = append(keys, d)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Before(keys[j])
	})
	return keys
}

func fillDailyTotals(start, end time.Time, totals map[time.Time]dailyTotal) []dailyTotal {
	if end.Before(start) {
		return nil
	}
	var out []dailyTotal
	for d := dateOnly(start); !d.After(end); d = d.AddDate(0, 0, 1) {
		total := totals[d]
		total.Date = d
		out = append(out, total)
	}
	return out
}

func sumDailyCosts(series []dailyTotal) float64 {
	total := 0.0
	for _, d := range series {
		total += d.CostUSD
	}
	return total
}

func parseRecordedAt(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func dateOnly(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
