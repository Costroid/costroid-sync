package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// clamp01 bounds a fraction to [0,1] so a meter never overflows its cells.
func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// meter renders a fixed-width horizontal fill bar for frac in [0,1]. In UTF-8
// mode it uses a full block plus eighth-block partials for sub-cell precision; in
// ASCII it renders a bracketed [####----] bar. fill styles the filled portion
// (accent for healthy, alert for over-budget); empties are faint. Color is never
// the sole signal — the filled length alone conveys the value (monochrome-safe).
func meter(s Styles, frac float64, width int, fill lipgloss.Style) string {
	if width < 1 {
		width = 1
	}
	frac = clamp01(frac)
	if s.ASCII {
		f := int(frac*float64(width) + 0.5)
		if f > width {
			f = width
		}
		return "[" + fill.Render(strings.Repeat(asciiBarFull, f)) +
			s.Faint.Render(strings.Repeat(asciiBarGap, width-f)) + "]"
	}
	full := frac * float64(width)
	cells := int(full)
	partial := int((full-float64(cells))*8 + 0.5)
	if partial >= 8 {
		cells++
		partial = 0
	}
	if cells > width {
		cells, partial = width, 0
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(meterFull, cells))
	used := cells
	if partial > 0 && used < width {
		b.WriteString(meterPartials[partial])
		used++
	}
	return fill.Render(b.String()) + s.Faint.Render(strings.Repeat(meterEmpty, width-used))
}

// sparkline renders vals as an inline ▁▂▃▄▅▆▇█ trend, scaled min→max. It is a
// static accent only — never animated. ASCII mode and an empty series return ""
// so the caller falls back to the numeric readout (the layout must hold without
// the glyph). Money is never encoded here, only relative daily shape.
func sparkline(s Styles, vals []float64, fill lipgloss.Style) string {
	if s.ASCII || len(vals) == 0 {
		return ""
	}
	min, max := vals[0], vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	top := len(sparkLevels) - 1
	b := make([]rune, len(vals))
	for i, v := range vals {
		idx := 0
		if span > 0 {
			idx = int((v-min)/span*float64(top) + 0.5)
		}
		if idx < 0 {
			idx = 0
		} else if idx > top {
			idx = top
		}
		b[i] = sparkLevels[idx]
	}
	return fill.Render(string(b))
}

// dotStrip renders total position dots with the first `filled` lit — a truthful,
// non-animated progress/position indicator (e.g. finished sync stages). Filled
// dots carry the accent; the rest are faint. Shape conveys progress with no color.
func dotStrip(s Styles, total, filled int, fill lipgloss.Style) string {
	if total < 1 {
		return ""
	}
	if filled < 0 {
		filled = 0
	} else if filled > total {
		filled = total
	}
	return fill.Render(strings.Repeat(s.navDot(true), filled)) +
		s.Faint.Render(strings.Repeat(s.navDot(false), total-filled))
}
