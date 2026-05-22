package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/costroid/costroid-sync/analysis"
	"github.com/costroid/costroid-sync/providers"
)

// HeaderStyle is the lipgloss style used for table column headers.
var HeaderStyle = lipgloss.NewStyle().Bold(true)

// WriteTable renders cost records as an aligned terminal table with columns
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
	renderTable(w, cols, rows)
}

// savingsFooterNote is printed after the table to remind the user that
// recommendations are cost estimates only.
const savingsFooterNote = "Estimates use token counts × seeded pricing. Costs only — they don't " +
	"account for quality, latency, context window, or capability differences."

// WriteSavingsTable renders savings recommendations with columns
// Provider | Current | Cheaper Alt | Tokens | Now | Est. | Save | Save%.
// Renders only metadata + cost fields — never raw provider content.
// Always prints a one-line footer clarifying that the numbers are estimates.
func WriteSavingsTable(w io.Writer, recs []analysis.SavingsRecommendation) {
	cols := []string{"Provider", "Current", "Cheaper Alt", "Tokens", "Now", "Est.", "Save", "Save%"}
	rows := make([][]string, len(recs))
	for i, r := range recs {
		rows[i] = []string{
			r.Provider,
			r.CurrentModel,
			r.RecommendedModel,
			strconv.Itoa(r.PromptTokens + r.CompletionTokens),
			fmt.Sprintf("$%.4f", r.CurrentCostUSD),
			fmt.Sprintf("$%.4f", r.EstimatedCostUSD),
			fmt.Sprintf("$%.4f", r.SavingsUSD),
			fmt.Sprintf("%.1f%%", r.SavingsPercent),
		}
	}
	renderTable(w, cols, rows)
	fmt.Fprintln(w, savingsFooterNote)
}

// WriteHistoryTable renders local cost records as
// Date | Provider | Model | Tokens | Cost.
func WriteHistoryTable(w io.Writer, records []providers.NormalizedCostRecord) {
	cols := []string{"Date", "Provider", "Model", "Tokens", "Cost"}
	rows := make([][]string, len(records))
	for i, r := range records {
		rows[i] = []string{
			formatRecordDate(r.RecordedAt),
			r.Provider,
			r.Model,
			strconv.Itoa(r.TotalTokens),
			formatUSD(r.CostUSD),
		}
	}
	renderTable(w, cols, rows)
}

// WriteTrendTable renders local period aggregates.
func WriteTrendTable(w io.Writer, periods []analysis.TrendPeriod) {
	cols := []string{"Period", "Cost", "Tokens", "Change"}
	rows := make([][]string, len(periods))
	for i, p := range periods {
		rows[i] = []string{
			p.Period,
			formatUSD(p.CostUSD),
			strconv.Itoa(p.TotalTokens),
			formatChange(p),
		}
	}
	renderTable(w, cols, rows)
}

// WriteForecast renders a stable month-end forecast summary.
func WriteForecast(w io.Writer, f analysis.ForecastResult) {
	rows := [][]string{
		{"Current month spend", formatUSD(f.CurrentMonthSpendUSD)},
		{"Forecast month-end", formatUSD(f.ForecastMonthEndUSD)},
		{"Method", f.Method},
		{"Days observed", strconv.Itoa(f.DaysObserved)},
		{"Days remaining", strconv.Itoa(f.DaysRemaining)},
	}
	renderTable(w, []string{"Metric", "Value"}, rows)
}

// WriteAnomalyTable renders days whose spend exceeded the rolling baseline.
func WriteAnomalyTable(w io.Writer, anomalies []analysis.Anomaly) {
	cols := []string{"Date", "Actual", "7d Avg", "Deviation"}
	rows := make([][]string, len(anomalies))
	for i, a := range anomalies {
		rows[i] = []string{
			a.Date,
			formatUSD(a.ActualCostUSD),
			formatUSD(a.RollingAverageUSD),
			fmt.Sprintf("%.1f%%", a.DeviationPercent),
		}
	}
	renderTable(w, cols, rows)
}

// renderTable writes a header + rows with two-space column padding.
// All cell values are rendered as plain strings — callers are responsible
// for ensuring no raw provider content reaches this layer.
func renderTable(w io.Writer, cols []string, rows [][]string) {
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

func formatUSD(v float64) string {
	return fmt.Sprintf("$%.4f", v)
}

func formatChange(p analysis.TrendPeriod) string {
	if !p.HasChange {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", p.ChangePercent)
}

func formatRecordDate(recordedAt string) string {
	if len(recordedAt) >= len("2006-01-02") {
		return recordedAt[:len("2006-01-02")]
	}
	return recordedAt
}
