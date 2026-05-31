package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestResolveTier pins the environment → tier resolution: --plain or NO_COLOR force
// mono, a truecolor COLORTERM wins, a 256color TERM selects ansi256, and everything
// else degrades to the always-safe ANSI-16 indices.
func TestResolveTier(t *testing.T) {
	cases := []struct {
		name            string
		plain, noColor  bool
		colorterm, term string
		want            ColorTier
	}{
		{"plain forces mono", true, false, "truecolor", "xterm-256color", mono},
		{"no_color forces mono", false, true, "truecolor", "xterm-256color", mono},
		{"truecolor wins", false, false, "truecolor", "xterm", trueColor},
		{"24bit wins", false, false, "24bit", "xterm", trueColor},
		{"256color term", false, false, "", "xterm-256color", ansi256},
		{"plain 16-color default", false, false, "", "xterm", ansi16},
		{"truecolor over 256 term", false, false, "TrueColor", "xterm-256color", trueColor},
	}
	for _, c := range cases {
		if got := ResolveTier(c.plain, c.noColor, c.colorterm, c.term); got != c.want {
			t.Errorf("%s: ResolveTier(%v,%v,%q,%q)=%v, want %v",
				c.name, c.plain, c.noColor, c.colorterm, c.term, got, c.want)
		}
	}
}

// TestPalette_TierDegradation pins the truecolor → 256 → ANSI-16 → monochrome
// degradation (T1.6): each tier selects the hand-picked brand token, Signal green
// is identical on both surfaces at every color tier, the cold and warm ramps are
// always distinct, and mono yields no color anywhere.
func TestPalette_TierDegradation(t *testing.T) {
	// Cold ramp shade 1 (the #185FA5 family) degrades to the exact per-tier token.
	for _, c := range []struct {
		tier ColorTier
		want lipgloss.TerminalColor
	}{
		{trueColor, lipgloss.Color("#185FA5")},
		{ansi256, lipgloss.Color("25")},
		{ansi16, lipgloss.Color("4")},
		{mono, lipgloss.NoColor{}},
	} {
		if got := newStyles(surfaceCold, c.tier, false).Ramp[1].GetForeground(); got != c.want {
			t.Errorf("cold ramp[1] tier %v = %v, want %v", c.tier, got, c.want)
		}
	}

	for _, tier := range []ColorTier{trueColor, ansi256, ansi16} {
		// Signal green (money / brand) is identical across surfaces.
		cold := newStyles(surfaceCold, tier, false).Accent.GetForeground()
		warm := newStyles(surfaceWarm, tier, false).Accent.GetForeground()
		if cold != warm {
			t.Errorf("signal accent differs across surfaces at tier %v: cold %v warm %v", tier, cold, warm)
		}
		// The cold and warm ramp fills carry distinct surface identities.
		fc := newStyles(surfaceCold, tier, false).meterFill().GetForeground()
		fw := newStyles(surfaceWarm, tier, false).meterFill().GetForeground()
		if fc == fw {
			t.Errorf("cold and warm meter fill identical at tier %v (%v)", tier, fc)
		}
	}

	// Monochrome strips all color, so the hierarchy must rest on glyph + text.
	ms := newStyles(surfaceWarm, mono, false)
	for i, st := range ms.Ramp {
		if st.GetForeground() != (lipgloss.NoColor{}) {
			t.Errorf("mono ramp[%d] has color %v", i, st.GetForeground())
		}
	}
	if ms.Accent.GetForeground() != (lipgloss.NoColor{}) {
		t.Errorf("mono accent has color %v", ms.Accent.GetForeground())
	}
}
