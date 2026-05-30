package tui

import "strconv"

// providersBody renders per-provider spend and token totals over the read
// window. Metadata only — provider names, counts, money. Sorted by spend
// descending (the aggregation layer guarantees deterministic order).
func providersBody(d Dashboard, s Styles, _ int) string {
	if len(d.Providers) == 0 {
		return s.Faint.Render("No provider activity in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	cols := []string{"Provider", "Cost", "Tokens", "Records"}
	rows := make([][]string, len(d.Providers))
	for i, p := range d.Providers {
		rows[i] = []string{
			p.Provider,
			money(p.CostUSD),
			strconv.Itoa(p.TotalTokens),
			strconv.Itoa(p.Records),
		}
	}
	return table(s, cols, rows)
}
