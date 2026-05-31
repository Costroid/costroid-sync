package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorTier is the resolved terminal color capability. Color is gated explicitly
// (mono == --plain / NO_COLOR) so the monochrome fallback stays deterministic; the
// remaining tiers select hand-picked values from the approved 16/256/truecolor
// brand ramps rather than relying on lipgloss auto-downsampling, which guarantees
// the exact 16-color mapping (T1.6).
type ColorTier int

const (
	mono      ColorTier = iota // no color: --plain or a non-empty NO_COLOR
	ansi16                     // 16 ANSI indices — always safe
	ansi256                    // 256-color palette
	trueColor                  // 24-bit truecolor (COLORTERM=truecolor/24bit)
)

// surface is which opt-in TUI is being styled: the cold dashboard (the CLI
// surface) or the warm sync view. It selects the brand ramp hue; Signal green and
// the alert/ash monotones are shared across both surfaces (T1.6).
type surface int

const (
	surfaceCold surface = iota // dashboard — cyan-blue ramp
	surfaceWarm                // sync --tui — coral-amber ramp
)

// ResolveTier maps environment signals to a ColorTier. It is pure (the caller
// passes the env strings) so the resolution is unit-testable and the cmd layer
// keeps every os.Getenv read. --plain or a non-empty NO_COLOR force mono; a
// truecolor COLORTERM wins; a 256color TERM selects ansi256; everything else
// degrades to the always-safe ANSI-16 indices.
func ResolveTier(plain, noColor bool, colorterm, term string) ColorTier {
	if plain || noColor {
		return mono
	}
	switch strings.ToLower(strings.TrimSpace(colorterm)) {
	case "truecolor", "24bit":
		return trueColor
	}
	if strings.Contains(strings.ToLower(term), "256color") {
		return ansi256
	}
	return ansi16
}

// Brand ramps, dim → bright, per color tier. The 16-color rows are hand-picked
// per the founder direction: cold → blue/cyan, warm → yellow (kept off red so the
// surface never reads as an alert). mono is absent → a zero [4]string{} (no color).
var coldRamp = map[ColorTier][4]string{
	trueColor: {"#042C53", "#185FA5", "#378ADD", "#85B7EB"},
	ansi256:   {"17", "25", "68", "117"},
	ansi16:    {"4", "4", "6", "14"}, // blue → cyan → bright-cyan
}

var warmRamp = map[ColorTier][4]string{
	trueColor: {"#712B13", "#D85A30", "#F0997B", "#F5C4B3"},
	ansi256:   {"52", "166", "216", "223"},
	ansi16:    {"3", "3", "11", "11"}, // yellow → bright-yellow
}

// Shared tokens — identical on both surfaces at every tier. Signal green is the
// money / brand accent; ash is the faint monotone; alert red stays distinct from
// the warm ramp. mono is absent → "" (no color).
var (
	signalColor = map[ColorTier]string{trueColor: "#C8FF3D", ansi256: "154", ansi16: "2"}
	faintColor  = map[ColorTier]string{trueColor: "#888780", ansi256: "102", ansi16: "8"}
	alertColor  = map[ColorTier]string{trueColor: "#FF5C57", ansi256: "203", ansi16: "1"}
)

// rampTokens returns the four surface-ramp color tokens for the tier (empty
// strings in mono → no color).
func rampTokens(surf surface, tier ColorTier) [4]string {
	if surf == surfaceWarm {
		return warmRamp[tier]
	}
	return coldRamp[tier]
}

// fg builds a style with the given foreground token, or a plain (uncolored) style
// when token is empty (the mono tier). Centralizing the empty-token check keeps
// every color strictly gated by the resolved tier.
func fg(token string) lipgloss.Style {
	st := lipgloss.NewStyle()
	if token != "" {
		st = st.Foreground(lipgloss.Color(token))
	}
	return st
}
