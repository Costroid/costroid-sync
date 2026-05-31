package tui

import (
	"strconv"
	"strings"
)

// forecastBody renders the month-end spend forecast, or an honest "insufficient
// data" message. It adds a recent-spend sparkline beside the current spend and a
// current→forecast progress meter. Forecast numbers are stable — never animated.
func forecastBody(d Dashboard, s Styles, _ int) string {
	if d.Forecast == nil {
		return s.Faint.Render("Insufficient data to forecast.") + "\n\n" +
			"At least two days of recorded spend in the current month are needed."
	}
	const labelW = 20
	f := d.Forecast

	current := money(f.CurrentMonthSpendUSD)
	if spark := sparkline(s, d.Spark, s.meterFill()); spark != "" {
		current += "  " + spark
	}

	lines := []string{
		labeled(s, "Current month spend", labelW, current),
		labeled(s, "Forecast month-end", labelW, s.Accent.Render(money(f.ForecastMonthEndUSD))),
		labeled(s, "Method", labelW, f.Method),
		labeled(s, "Days observed", labelW, strconv.Itoa(f.DaysObserved)),
		labeled(s, "Days remaining", labelW, strconv.Itoa(f.DaysRemaining)),
	}
	if f.ForecastMonthEndUSD > 0 {
		frac := f.CurrentMonthSpendUSD / f.ForecastMonthEndUSD
		progress := meter(s, frac, 16, s.meterFill()) + "  " + pct(frac) + " of forecast"
		lines = append(lines, labeled(s, "Progress", labelW, progress))
	}
	return strings.Join(lines, "\n")
}
