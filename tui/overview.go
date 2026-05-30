package tui

import (
	"strconv"
	"strings"
)

// overviewBody renders the headline metric rhythm: month-to-date spend,
// forecast, budget usage, anomaly count, last-sync freshness, and the read
// window. Reuses the same metadata-only metrics as the statusline. Color is
// never the sole signal: over-budget and anomalies always carry a text marker.
func overviewBody(d Dashboard, s Styles, _ int) string {
	const labelW = 14
	m := d.Overview
	var lines []string

	lines = append(lines, labeled(s, "Month to date", labelW, s.Accent.Render(money(m.MTDCostUSD))))

	forecast := "insufficient data"
	if m.ForecastUSD != nil {
		forecast = money(*m.ForecastUSD)
	}
	lines = append(lines, labeled(s, "Forecast", labelW, forecast))

	lines = append(lines, labeled(s, "Budget", labelW, overviewBudget(d, s)))

	anomalies := strconv.Itoa(m.AnomalyCount)
	if m.AnomalyCount > 0 {
		anomalies = s.Alert.Render(anomalies + "  ALERT")
	}
	lines = append(lines, labeled(s, "Anomalies", labelW, anomalies))

	lines = append(lines, labeled(s, "Last sync", labelW, syncAge(d.LastSync, d.GeneratedAt)))
	lines = append(lines, labeled(s, "Window", labelW, "last "+strconv.Itoa(d.WindowDays)+" days"))

	return strings.Join(lines, "\n")
}

// overviewBudget formats the budget cell: percent + period, with a red OVER
// marker when spend exceeds the budget, or a hint when none is configured.
func overviewBudget(d Dashboard, s Styles) string {
	m := d.Overview
	if m.BudgetPercent == nil {
		return s.Faint.Render("not set")
	}
	cell := strconv.Itoa(*m.BudgetPercent) + "%"
	if d.Budget != nil {
		cell += " (" + d.Budget.Period + ")"
	}
	if m.OverBudget {
		return s.Alert.Render(cell + "  OVER")
	}
	return cell
}
