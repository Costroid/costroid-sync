package tui

import (
	"strconv"
	"strings"
)

// exportCommands lists the local, offline export formats the CLI supports.
var exportCommands = []string{
	"costroid export --format csv",
	"costroid export --format json",
	"costroid export --format focus",
	"costroid export --format markdown",
}

// exportHintsBody renders offline savings recommendations (cheaper-model swaps,
// metadata only) plus the local export command hints. The savings numbers are
// cost estimates only and are labelled as such.
func exportHintsBody(d Dashboard, s Styles, _ int) string {
	var b strings.Builder
	b.WriteString(s.Title.Render("Export local metadata"))
	b.WriteByte('\n')
	for _, c := range exportCommands {
		b.WriteString("  " + s.Faint.Render(s.navDot(false)) + " " + c + "\n")
	}

	b.WriteByte('\n')
	b.WriteString(s.Title.Render("Savings recommendations"))
	b.WriteByte('\n')
	if len(d.Savings) == 0 {
		b.WriteString(s.Faint.Render("No cheaper-model recommendations for the current window."))
		return b.String()
	}
	cols := []string{"Provider", "Current", "Cheaper", "Now", "Est.", "Save", "Save%"}
	rows := make([][]string, len(d.Savings))
	for i, r := range d.Savings {
		rows[i] = []string{
			r.Provider, r.CurrentModel, r.RecommendedModel,
			money(r.CurrentCostUSD), money(r.EstimatedCostUSD), money(r.SavingsUSD),
			strconv.FormatFloat(r.SavingsPercent, 'f', 1, 64) + "%",
		}
	}
	b.WriteString(table(s, cols, rows))
	b.WriteString("\n" + s.Faint.Render("Estimates use token counts "+s.times()+" seeded pricing; "+
		"they ignore quality, latency, and capability differences."))
	return b.String()
}
