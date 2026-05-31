package tui

import "strconv"

// historyBody renders per-day spend over the read window, newest first, with an
// inline spend meter scaled to the busiest day. Metadata only — a date plus
// aggregate cost and token counts. Gap days are zero-filled by the data layer so
// the series reads as a continuous calendar.
func historyBody(d Dashboard, s Styles, _ int) string {
	if len(d.History) == 0 {
		return s.Faint.Render("No usage history in the last " +
			strconv.Itoa(d.WindowDays) + " days. Run  costroid sync  first.")
	}
	top := 0.0
	for _, h := range d.History {
		if h.CostUSD > top {
			top = h.CostUSD
		}
	}
	cols := []string{"Date", "Cost", "Tokens", "Spend"}
	rows := make([][]string, len(d.History))
	// Render newest day first for an at-a-glance read; the data layer supplies
	// the series oldest→newest, so walk it in reverse.
	for i := range d.History {
		h := d.History[len(d.History)-1-i]
		frac := 0.0
		if top > 0 {
			frac = h.CostUSD / top
		}
		rows[i] = []string{
			h.Date.Format("2006-01-02"),
			money(h.CostUSD),
			strconv.Itoa(h.TotalTokens),
			meter(s, frac, 10, s.Accent),
		}
	}
	return table(s, cols, rows)
}
