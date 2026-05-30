package analysis

import (
	"errors"
	"time"

	"github.com/costroid/costroid/providers"
)

var ErrInsufficientData = errors.New("insufficient data")

type ForecastResult struct {
	CurrentMonthSpendUSD      float64
	ForecastMonthEndUSD       float64
	EMAForecastMonthEndUSD    float64
	LinearForecastMonthEndUSD float64
	Method                    string
	DaysObserved              int
	DaysRemaining             int
}

func Forecast(records []providers.NormalizedCostRecord, now time.Time) (ForecastResult, error) {
	series, hasCurrent := currentMonthSeries(records, now)
	result := ForecastResult{
		DaysObserved:  len(series),
		DaysRemaining: daysRemainingInMonth(now),
	}
	if !hasCurrent || len(series) < 2 {
		return result, ErrInsufficientData
	}

	spend := sumDailyCosts(series)
	result.CurrentMonthSpendUSD = spend
	result.EMAForecastMonthEndUSD = clampForecast(spend, spend+emaDailySpend(series)*float64(result.DaysRemaining))
	result.LinearForecastMonthEndUSD = clampForecast(spend, linearMonthEndForecast(series, result.DaysRemaining))
	result.Method = "ema"
	result.ForecastMonthEndUSD = result.EMAForecastMonthEndUSD
	if len(series) >= 3 {
		result.Method = "linear_regression"
		result.ForecastMonthEndUSD = result.LinearForecastMonthEndUSD
	}
	return result, nil
}

func currentMonthSeries(records []providers.NormalizedCostRecord, now time.Time) ([]dailyTotal, bool) {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := dateOnly(now)
	totals := costsByDate(records)
	series := fillDailyTotals(start, end, totals)
	for _, d := range series {
		if d.CostUSD > 0 || d.TotalTokens > 0 {
			return series, true
		}
	}
	return series, false
}

func daysRemainingInMonth(now time.Time) int {
	now = now.UTC()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := nextMonth.AddDate(0, 0, -1).Day()
	if now.Day() >= lastDay {
		return 0
	}
	return lastDay - now.Day()
}

func emaDailySpend(series []dailyTotal) float64 {
	const alpha = 0.4
	ema := series[0].CostUSD
	for _, d := range series[1:] {
		ema = alpha*d.CostUSD + (1-alpha)*ema
	}
	return ema
}

func linearMonthEndForecast(series []dailyTotal, daysRemaining int) float64 {
	intercept, slope := linearFit(series)
	total := sumDailyCosts(series)
	for i := 1; i <= daysRemaining; i++ {
		predicted := intercept + slope*float64(len(series)+i)
		if predicted > 0 {
			total += predicted
		}
	}
	return total
}

func linearFit(series []dailyTotal) (float64, float64) {
	var sumX, sumY, sumXY, sumXX float64
	for i, d := range series {
		x := float64(i + 1)
		y := d.CostUSD
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	n := float64(len(series))
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return sumY / n, 0
	}
	slope := (n*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / n
	return intercept, slope
}

func clampForecast(current, forecast float64) float64 {
	if forecast < current {
		return current
	}
	return forecast
}
