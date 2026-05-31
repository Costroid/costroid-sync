package tui

import "strconv"

// providersBody renders per-provider spend and token totals over the read
// window, plus a spend share and a braille-density spend meter. Metadata only —
// provider names, counts, money. Sorted by spend descending (the aggregation
// layer guarantees deterministic order).
func providersBody(d Dashboard, s Styles, _ int) string {
	if len(d.Providers) == 0 {
		return s.Faint.Render("No provider activity in the last " +
			strconv.Itoa(d.WindowDays) + " days.")
	}
	total := 0.0
	for _, p := range d.Providers {
		total += p.CostUSD
	}
	cols := []string{"Provider", "Cost", "Tokens", "Records", "Share", "Spend"}
	rows := make([][]string, len(d.Providers))
	for i, p := range d.Providers {
		share := 0.0
		if total > 0 {
			share = p.CostUSD / total
		}
		rows[i] = []string{
			p.Provider,
			money(p.CostUSD),
			strconv.Itoa(p.TotalTokens),
			strconv.Itoa(p.Records),
			pct(share),
			meter(s, share, 10, s.meterFill()),
		}
	}
	return table(s, cols, rows)
}
