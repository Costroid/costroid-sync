package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/costroid/costroid-sync/providers"
)

// HeaderStyle is the lipgloss style used for table column headers.
var HeaderStyle = lipgloss.NewStyle().Bold(true)

// WriteTable renders records as an aligned terminal table with columns
// Provider | Model | Tokens | Cost. Renders ONLY metadata fields — never
// prompts, completions, or any other raw provider content.
func WriteTable(w io.Writer, records []providers.NormalizedCostRecord) {
	cols := []string{"Provider", "Model", "Tokens", "Cost"}
	rows := make([][]string, len(records))
	for i, r := range records {
		rows[i] = []string{
			r.Provider,
			r.Model,
			strconv.Itoa(r.TotalTokens),
			fmt.Sprintf("$%.4f", r.CostUSD),
		}
	}

	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, row := range rows {
		for i, cell := range row {
			if l := len(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}

	headerParts := make([]string, len(cols))
	for i, c := range cols {
		headerParts[i] = padRight(c, widths[i])
	}
	fmt.Fprintln(w, HeaderStyle.Render(strings.Join(headerParts, "  ")))

	for _, row := range rows {
		parts := make([]string, len(row))
		for i, cell := range row {
			parts[i] = padRight(cell, widths[i])
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
