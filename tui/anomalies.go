package tui

import "strconv"

// anomaliesBody lists days whose spend exceeded the rolling baseline. Each row
// is metadata only (date + money + deviation). The header count is shown in red
// when non-zero, always paired with the numeric count.
func anomaliesBody(d Dashboard, s Styles, _ int) string {
	if len(d.Anomalies) == 0 {
		return s.Faint.Render("No spend anomalies detected in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	header := strconv.Itoa(len(d.Anomalies)) + " anomalous day(s)"
	cols := []string{"Date", "Actual", "7d Avg", "Deviation"}
	rows := make([][]string, len(d.Anomalies))
	for i, a := range d.Anomalies {
		rows[i] = []string{
			a.Date,
			money(a.ActualCostUSD),
			money(a.RollingAverageUSD),
			"+" + strconv.FormatFloat(a.DeviationPercent, 'f', 1, 64) + "%",
		}
	}
	return s.Alert.Render(header) + "\n\n" + table(s, cols, rows)
}
