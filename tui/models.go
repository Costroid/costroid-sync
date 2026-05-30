package tui

import "strconv"

// modelsBody renders per-(provider, model) spend and token totals over the read
// window. Metadata only — provider/model names, counts, money.
func modelsBody(d Dashboard, s Styles, _ int) string {
	if len(d.Models) == 0 {
		return s.Faint.Render("No model activity in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	cols := []string{"Provider", "Model", "Cost", "Tokens", "Records"}
	rows := make([][]string, len(d.Models))
	for i, m := range d.Models {
		rows[i] = []string{
			m.Provider,
			m.Model,
			money(m.CostUSD),
			strconv.Itoa(m.TotalTokens),
			strconv.Itoa(m.Records),
		}
	}
	return table(s, cols, rows)
}
