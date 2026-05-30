package tui

import "strconv"

// anomaliesBody lists days whose spend exceeded the rolling baseline. Each row is
// metadata only (date + money + deviation + a severity glyph). The header count
// is shown in red when non-zero, always paired with the numeric count.
func anomaliesBody(d Dashboard, s Styles, _ int) string {
	if len(d.Anomalies) == 0 {
		return s.Faint.Render("No spend anomalies detected in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	header := strconv.Itoa(len(d.Anomalies)) + " anomalous day(s)"
	cols := []string{"Date", "Actual", "7d Avg", "Deviation", "Sev"}
	rows := make([][]string, len(d.Anomalies))
	for i, a := range d.Anomalies {
		rows[i] = []string{
			a.Date,
			money(a.ActualCostUSD),
			money(a.RollingAverageUSD),
			"+" + strconv.FormatFloat(a.DeviationPercent, 'f', 1, 64) + "%",
			severityGlyph(s, a.DeviationPercent),
		}
	}
	return s.Alert.Render(header) + "\n\n" + table(s, cols, rows)
}

// severityGlyph maps a deviation percent to an escalating dot/block glyph so
// severity reads by shape, not color alone (a row already shows its numeric
// deviation). ASCII degrades to ". : !". Thresholds: ≥200% high, ≥100% medium.
func severityGlyph(s Styles, deviationPct float64) string {
	switch {
	case deviationPct >= 200:
		if s.ASCII {
			return "!"
		}
		return "█"
	case deviationPct >= 100:
		if s.ASCII {
			return ":"
		}
		return "▄"
	default:
		if s.ASCII {
			return "."
		}
		return "▁"
	}
}
