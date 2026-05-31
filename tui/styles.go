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

// Styles is the TUI's lipgloss palette. Signal green is the single shared accent
// (money / brand mark); each surface adds a cold (dashboard) or warm (sync) ramp
// used for selection, meters, the sparkline, and severity (T1.6). Color is gated
// by Tier (mono == --plain / NO_COLOR) so the monochrome fallback is deterministic,
// and every colored signal is also carried by glyph shape or a text marker, so
// color is never the sole signal.
type Styles struct {
	Surface surface
	Tier    ColorTier
	ASCII   bool

	Title    lipgloss.Style    // panel / header titles
	Faint    lipgloss.Style    // secondary labels (ash)
	Accent   lipgloss.Style    // Signal green — money / brand mark / sync success
	Alert    lipgloss.Style    // red — over-budget / critical severity / errors
	Header   lipgloss.Style    // table column headers
	Active   lipgloss.Style    // selected panel tab — surface ramp primary
	Inactive lipgloss.Style    // unselected panel tab
	Ramp     [4]lipgloss.Style // surface ramp shades, dim → bright
}

// newStyles builds the palette for a surface and color tier. Bold structural
// emphasis is kept in every tier (it emits no color); foregrounds are applied only
// when the tier supplies a non-empty token, so mono degrades to a fully uncolored,
// deterministic render.
func newStyles(surf surface, tier ColorTier, ascii bool) Styles {
	s := Styles{Surface: surf, Tier: tier, ASCII: ascii}
	s.Title = lipgloss.NewStyle().Bold(true)
	s.Header = lipgloss.NewStyle().Bold(true)
	s.Faint = fg(faintColor[tier])
	s.Accent = fg(signalColor[tier])
	s.Alert = fg(alertColor[tier]).Bold(true)
	s.Ramp = rampStyles(surf, tier)
	s.Active = s.Ramp[2].Bold(true) // selection also reads from the filled nav dot
	s.Inactive = s.Faint
	return s
}

// rampStyles builds the four surface-ramp shade styles for the tier (all plain in
// mono).
func rampStyles(surf surface, tier ColorTier) [4]lipgloss.Style {
	tokens := rampTokens(surf, tier)
	var out [4]lipgloss.Style
	for i, t := range tokens {
		out[i] = fg(t)
	}
	return out
}

// meterFill is the surface ramp shade used for meters, spend bars, the sparkline,
// and the sync dot-progress strip. Money keeps Accent (Signal green); this carries
// the cold/warm surface identity. The filled length alone conveys the value, so the
// color is never the sole signal.
func (s Styles) meterFill() lipgloss.Style { return s.Ramp[2] }

// sevStyle maps a 0..8 severity level to a ramp shade (dim → bright), with the
// critical top tier using the alert treatment. Severity also reads by braille dot
// density (severityGlyph) and the row's numeric deviation, so this is never the
// sole signal.
func (s Styles) sevStyle(level int) lipgloss.Style {
	switch {
	case level >= 8:
		return s.Alert
	case level >= 6:
		return s.Ramp[3]
	case level >= 4:
		return s.Ramp[2]
	case level >= 2:
		return s.Ramp[1]
	default:
		return s.Ramp[0]
	}
}

// mark returns the brand glyph (UTF-8 or ASCII fallback).
func (s Styles) mark() string {
	if s.ASCII {
		return asciiMark
	}
	return glyphMark
}
