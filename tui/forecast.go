package tui

import (
	"strconv"
	"strings"
)

// forecastBody renders the month-end spend forecast, or an honest "insufficient
// data" message. Forecast numbers are stable — never animated or rolled.
func forecastBody(d Dashboard, s Styles, _ int) string {
	if d.Forecast == nil {
		return s.Faint.Render("Insufficient data to forecast.") + "\n\n" +
			"At least two days of recorded spend in the current month are needed."
	}
	const labelW = 20
	f := d.Forecast
	lines := []string{
		labeled(s, "Current month spend", labelW, money(f.CurrentMonthSpendUSD)),
		labeled(s, "Forecast month-end", labelW, s.Accent.Render(money(f.ForecastMonthEndUSD))),
		labeled(s, "Method", labelW, f.Method),
		labeled(s, "Days observed", labelW, strconv.Itoa(f.DaysObserved)),
		labeled(s, "Days remaining", labelW, strconv.Itoa(f.DaysRemaining)),
	}
	return strings.Join(lines, "\n")
}
