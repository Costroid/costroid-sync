package tui

import (
	"strconv"
	"strings"
)

// overviewBody renders the headline metric rhythm: month-to-date spend (with a
// recent-spend sparkline), forecast, budget usage (with a braille meter), anomaly
// count + severity, last-sync freshness, and the read window. Reuses the same
// metadata-only metrics as the statusline. Color is never the sole signal:
// over-budget and anomalies always carry a text marker, and the meter/severity
// glyphs read by shape alone.
func overviewBody(d Dashboard, s Styles, _ int) string {
	const labelW = 14
	const meterW = 12
	m := d.Overview
	var lines []string

	mtd := s.Accent.Render(money(m.MTDCostUSD))
	if spark := sparkline(s, d.Spark, s.meterFill()); spark != "" {
		mtd += "  " + spark
	}
	lines = append(lines, labeled(s, "Month to date", labelW, mtd))

	forecast := "insufficient data"
	if m.ForecastUSD != nil {
		forecast = money(*m.ForecastUSD)
	}
	lines = append(lines, labeled(s, "Forecast", labelW, forecast))

	lines = append(lines, labeled(s, "Budget", labelW, overviewBudget(d, s, meterW)))
	lines = append(lines, labeled(s, "Anomalies", labelW, overviewAnomalies(d, s)))
	lines = append(lines, labeled(s, "Last sync", labelW, syncAge(d.LastSync, d.GeneratedAt)))
	lines = append(lines, labeled(s, "Window", labelW, "last "+strconv.Itoa(d.WindowDays)+" days"))

	return strings.Join(lines, "\n")
}

// overviewBudget formats the budget cell: a braille meter + percent + period,
// with a red OVER marker when spend exceeds the budget, or a hint when none is
// configured. The meter fill is accent when healthy, alert when over.
func overviewBudget(d Dashboard, s Styles, meterW int) string {
	m := d.Overview
	if m.BudgetPercent == nil {
		return s.Faint.Render("not set")
	}
	fill := s.meterFill()
	if m.OverBudget {
		fill = s.Alert
	}
	cell := meter(s, float64(*m.BudgetPercent)/100, meterW, fill) + "  " + strconv.Itoa(*m.BudgetPercent) + "%"
	if d.Budget != nil {
		cell += " (" + d.Budget.Period + ")"
	}
	if m.OverBudget {
		cell += "  " + s.Alert.Render("OVER")
	}
	return cell
}

// overviewAnomalies formats the anomaly cell: count + ALERT marker + a severity
// glyph (the worst day's deviation), or a plain "0" when clean. Color is never
// the sole signal — the count, the word ALERT, and the glyph shape all carry it.
func overviewAnomalies(d Dashboard, s Styles) string {
	m := d.Overview
	if m.AnomalyCount == 0 {
		return "0"
	}
	worst := 0.0
	for _, a := range d.Anomalies {
		if a.DeviationPercent > worst {
			worst = a.DeviationPercent
		}
	}
	return s.Alert.Render(strconv.Itoa(m.AnomalyCount)+"  ALERT") + "  " + severityGlyph(s, worst)
}
