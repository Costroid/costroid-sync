package analysis

import (
	"sort"

	"github.com/costroid/costroid-sync/providers"
)

type Anomaly struct {
	Date              string
	ActualCostUSD     float64
	RollingAverageUSD float64
	DeviationPercent  float64
}

func DetectAnomalies(records []providers.NormalizedCostRecord) []Anomaly {
	series := completeDailySeries(records)
	var out []Anomaly
	for i := 7; i < len(series); i++ {
		avg := rollingAverage(series[i-7 : i])
		if avg <= 0 || series[i].CostUSD <= 2*avg {
			continue
		}
		out = append(out, Anomaly{
			Date:              series[i].Date.Format(dateLayout),
			ActualCostUSD:     series[i].CostUSD,
			RollingAverageUSD: avg,
			DeviationPercent:  (series[i].CostUSD - avg) / avg * 100,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date
	})
	return out
}

func rollingAverage(series []dailyTotal) float64 {
	if len(series) == 0 {
		return 0
	}
	return sumDailyCosts(series) / float64(len(series))
}
