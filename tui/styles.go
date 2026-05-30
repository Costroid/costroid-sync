package tui

import "github.com/charmbracelet/lipgloss"

// Brand glyphs. The braille mark is an identity accent only, never a layout
// dependency; ASCII mode falls back to a bracketed wordmark (design-language
// §6, terminal-design §8).
const (
	glyphMark = "⣿"
	asciiMark = "[*]"
	wordmark  = "costroid"
)

// Styles is the TUI's lipgloss palette. It is monochrome-first with a single
// green accent and red reserved for alerts (over-budget / anomalies). Color is
// gated explicitly by Color so --plain and NO_COLOR produce a fully monochrome,
// deterministic render without relying on terminal profile auto-detection.
type Styles struct {
	Color bool
	ASCII bool

	Title    lipgloss.Style // panel / header titles
	Faint    lipgloss.Style // secondary labels
	Accent   lipgloss.Style // single green accent (primary money / brand)
	Alert    lipgloss.Style // red — alerts only, always paired with a text marker
	Header   lipgloss.Style // table column headers
	Active   lipgloss.Style // currently selected panel tab
	Inactive lipgloss.Style // unselected panel tab
}

// newStyles builds the palette. When color is false every style degrades to a
// plain (uncolored) variant; structural emphasis (bold) is kept because it does
// not emit color and golden tests gate on the color flag, not on bold.
func newStyles(color, ascii bool) Styles {
	s := Styles{Color: color, ASCII: ascii}
	s.Title = lipgloss.NewStyle().Bold(true)
	s.Header = lipgloss.NewStyle().Bold(true)
	s.Faint = lipgloss.NewStyle()
	s.Accent = lipgloss.NewStyle()
	s.Alert = lipgloss.NewStyle().Bold(true)
	s.Active = lipgloss.NewStyle().Bold(true)
	s.Inactive = lipgloss.NewStyle()
	if color {
		// ANSI 16-color indices for broad terminal compatibility:
		// 2 = green (accent), 1 = red (alert), 8 = bright-black (faint).
		s.Faint = s.Faint.Foreground(lipgloss.Color("8"))
		s.Accent = s.Accent.Foreground(lipgloss.Color("2"))
		s.Alert = s.Alert.Foreground(lipgloss.Color("1"))
		s.Active = s.Active.Foreground(lipgloss.Color("2"))
		s.Inactive = s.Inactive.Foreground(lipgloss.Color("8"))
	}
	return s
}

// mark returns the brand glyph (UTF-8 or ASCII fallback).
func (s Styles) mark() string {
	if s.ASCII {
		return asciiMark
	}
	return glyphMark
}
