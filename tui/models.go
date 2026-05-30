package tui

import "strconv"

// modelsBody renders per-(provider, model) spend and token totals over the read
// window, plus a cost bar scaled to the top model's spend. Metadata only —
// provider/model names, counts, money.
func modelsBody(d Dashboard, s Styles, _ int) string {
	if len(d.Models) == 0 {
		return s.Faint.Render("No model activity in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	top := 0.0
	for _, m := range d.Models {
		if m.CostUSD > top {
			top = m.CostUSD
		}
	}
	cols := []string{"Provider", "Model", "Cost", "Tokens", "Records", "Spend"}
	rows := make([][]string, len(d.Models))
	for i, m := range d.Models {
		frac := 0.0
		if top > 0 {
			frac = m.CostUSD / top
		}
		rows[i] = []string{
			m.Provider,
			m.Model,
			money(m.CostUSD),
			strconv.Itoa(m.TotalTokens),
			strconv.Itoa(m.Records),
			meter(s, frac, 10, s.Accent),
		}
	}
	return table(s, cols, rows)
}
