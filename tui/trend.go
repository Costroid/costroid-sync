package tui

import (
	"strconv"
	"strings"

	"github.com/costroid/costroid/analysis"
)

// trendBody renders ISO-week and calendar-month spend rollups over the read
// window, each with a period-over-period change percent. Metadata only — period
// labels plus aggregate cost and token counts. Money is stable, never animated.
func trendBody(d Dashboard, s Styles, _ int) string {
	if len(d.TrendsWeekly) == 0 && len(d.TrendsMonthly) == 0 {
		return s.Faint.Render("No trend data in the last " +
			strconv.Itoa(d.WindowDays) + " days. Run  costroid sync  first.")
	}
	weekly := trendSection(s, "Weekly", d.TrendsWeekly)
	monthly := trendSection(s, "Monthly", d.TrendsMonthly)
	return strings.Join([]string{weekly, "", monthly}, "\n")
}

// trendSection renders one labelled trend table (or an honest empty line when
// the interval has no periods in the window).
func trendSection(s Styles, label string, periods []analysis.TrendPeriod) string {
	head := s.Title.Render(label)
	if len(periods) == 0 {
		return head + "\n" + s.Faint.Render("No "+strings.ToLower(label)+" periods in the window.")
	}
	cols := []string{"Period", "Cost", "Tokens", "Change"}
	rows := make([][]string, len(periods))
	for i, p := range periods {
		rows[i] = []string{
			p.Period,
			money(p.CostUSD),
			strconv.Itoa(p.TotalTokens),
			trendChange(p),
		}
	}
	return head + "\n" + table(s, cols, rows)
}

// trendChange formats the period-over-period change like the CLI trend table:
// a signed percent, or "n/a" for the first period (no prior to compare).
func trendChange(p analysis.TrendPeriod) string {
	if !p.HasChange {
		return "n/a"
	}
	v := strconv.FormatFloat(p.ChangePercent, 'f', 1, 64)
	if p.ChangePercent >= 0 {
		v = "+" + v // FormatFloat already prefixes "-" for negatives
	}
	return v + "%"
}
