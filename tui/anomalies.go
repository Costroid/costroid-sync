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

// severityDots renders the 0..8 severity level as a single braille cell whose dot
// density grows with severity, so severity reads by shape with color stripped.
var severityDots = []string{"⠀", "⠄", "⠆", "⠇", "⡇", "⡏", "⡟", "⡿", "⣿"}

// severityGlyph maps a deviation percent to a severity marker that reads by shape,
// not color alone (a row already shows its numeric deviation). In UTF-8 mode it is
// a single braille cell of escalating dot density, shaded by the surface ramp
// (critical → alert). ASCII degrades to ". : !" by the documented deviation tiers
// (≥200% high, ≥100% medium).
func severityGlyph(s Styles, deviationPct float64) string {
	if s.ASCII {
		switch {
		case deviationPct >= 200:
			return "!"
		case deviationPct >= 100:
			return ":"
		default:
			return "."
		}
	}
	level := severityLevel(deviationPct)
	return s.sevStyle(level).Render(severityDots[level])
}

// severityLevel buckets a deviation percent into a 0..8 level for the braille
// dot-density glyph and the ramp shade. Each step adds one braille dot; level 8
// (≥400%) is the critical/alert tier.
func severityLevel(deviationPct float64) int {
	switch {
	case deviationPct >= 400:
		return 8
	case deviationPct >= 300:
		return 7
	case deviationPct >= 200:
		return 6
	case deviationPct >= 150:
		return 5
	case deviationPct >= 100:
		return 4
	case deviationPct >= 50:
		return 3
	case deviationPct >= 25:
		return 2
	case deviationPct > 0:
		return 1
	default:
		return 0
	}
}
