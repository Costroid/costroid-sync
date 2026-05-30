package tui

import (
	"strconv"
	"strings"
	"time"
)

// money renders a USD amount as "$1,234.56": leading $, thousands separators,
// fixed 2 decimals. Money is always whole and stable — never animated, never
// partially rendered (terminal-design §4.2). Std lib only.
func money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	dot := strings.IndexByte(s, '.')
	out := "$" + groupThousands(s[:dot]) + s[dot:]
	if neg {
		out = "-" + out
	}
	return out
}

func groupThousands(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	lead := n % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// age renders a compact relative age: <60s → Ns, <60m → Nm, <48h → Nh, else Nd.
// Negative durations (clock skew) clamp to "0s". Matches the statusline grammar.
func age(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours())/24) + "d"
	}
}

// syncAge renders the freshness value for a last-sync time: "never" when nil,
// otherwise a compact age relative to now.
func syncAge(last *time.Time, now time.Time) string {
	if last == nil {
		return "never"
	}
	return age(now.Sub(*last)) + " ago"
}

// table renders an aligned text table (header + rows) with two-space column
// padding, mirroring output/table.go. All cells must already be plain metadata
// strings; this layer never receives raw provider content.
func table(s Styles, cols []string, rows [][]string) string {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, row := range rows {
		for i, cell := range row {
			if l := len(cell); i < len(widths) && l > widths[i] {
				widths[i] = l
			}
		}
	}

	var b strings.Builder
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = padRight(c, widths[i])
	}
	b.WriteString(s.Header.Render(strings.Join(header, "  ")))
	for _, row := range rows {
		b.WriteByte('\n')
		parts := make([]string, len(row))
		for i, cell := range row {
			parts[i] = padRight(cell, widths[i])
		}
		b.WriteString(strings.Join(parts, "  "))
	}
	return b.String()
}

func padRight(str string, n int) string {
	if len(str) >= n {
		return str
	}
	return str + strings.Repeat(" ", n-len(str))
}

// labeled renders a "label   value" line with the label faint-styled and padded
// to width for column alignment within a panel body.
func labeled(s Styles, label string, width int, value string) string {
	return s.Faint.Render(padRight(label, width)) + "  " + value
}
