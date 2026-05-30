package analysis

import (
	"fmt"
	"sort"
	"time"

	"github.com/costroid/costroid/providers"
)

type TrendInterval string

const (
	TrendWeekly  TrendInterval = "weekly"
	TrendMonthly TrendInterval = "monthly"
)

type TrendPeriod struct {
	Period        string
	CostUSD       float64
	TotalTokens   int
	ChangePercent float64
	HasChange     bool
}

func Trends(records []providers.NormalizedCostRecord, interval TrendInterval) []TrendPeriod {
	groups := map[string]*TrendPeriod{}
	for _, r := range records {
		t, ok := parseRecordedAt(r.RecordedAt)
		if !ok {
			continue
		}
		label := trendLabel(t, interval)
		p := groups[label]
		if p == nil {
			p = &TrendPeriod{Period: label}
			groups[label] = p
		}
		p.CostUSD += r.CostUSD
		p.TotalTokens += r.TotalTokens
	}
	out := sortedTrendPeriods(groups)
	addTrendChanges(out)
	return out
}

func trendLabel(t time.Time, interval TrendInterval) string {
	if interval == TrendMonthly {
		return t.UTC().Format("2006-01")
	}
	year, week := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func sortedTrendPeriods(groups map[string]*TrendPeriod) []TrendPeriod {
	labels := make([]string, 0, len(groups))
	for label := range groups {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	out := make([]TrendPeriod, 0, len(labels))
	for _, label := range labels {
		out = append(out, *groups[label])
	}
	return out
}

func addTrendChanges(periods []TrendPeriod) {
	for i := 1; i < len(periods); i++ {
		prev := periods[i-1].CostUSD
		if prev == 0 {
			continue
		}
		periods[i].HasChange = true
		periods[i].ChangePercent = (periods[i].CostUSD - prev) / prev * 100
	}
}
