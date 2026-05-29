package output

import (
	"strconv"
	"strings"
	"time"
)

// Color identifiers used by colorWrap; the actual escape/style codes are
// chosen per format (tmux style codes vs. ANSI SGR for byobu).
const (
	colorGreen = "green"
	colorRed   = "red"
)

// colorWrap wraps s in the given format's color codes. tmux uses its embedded
// style codes (#[...]); byobu uses ANSI SGR. Any other format returns s
// unchanged (plain/json are never colored). Color is never the sole carrier of
// meaning, so a monochrome render still reads correctly.
func colorWrap(format, color, s string) string {
	switch format {
	case "tmux":
		return "#[fg=" + color + "]" + s + "#[default]"
	case "byobu":
		return ansiSGR(color) + s + "\033[0m"
	default:
		return s
	}
}

func ansiSGR(color string) string {
	if color == colorRed {
		return "\033[31m"
	}
	return "\033[32m"
}

// formatMoney renders a USD amount as "$1,204.50": leading $, thousands
// separators, fixed 2 decimals. Money is always whole and stable — never
// animated or partially rendered. Uses std lib only (no dependency).
func formatMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64) // e.g. "1234.50"
	dot := strings.IndexByte(s, '.')
	intPart, decPart := s[:dot], s[dot:]

	out := "$" + groupThousands(intPart) + decPart
	if neg {
		out = "-" + out
	}
	return out
}

// groupThousands inserts commas every three digits from the right.
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

// formatAge renders a compact relative age: <60s → Ns, <60m → Nm, <48h → Nh,
// else Nd. Negative durations (clock skew) clamp to "0s".
func formatAge(d time.Duration) string {
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
