package tui

// Dot/braille glyph vocabulary (design-language §6: every glyph carries meaning
// or reinforces the mark, and each has an ASCII-safe fallback so the layout holds
// when braille/block glyphs are replaced). All accessors gate on Styles.ASCII so
// --plain and non-UTF-8 environments degrade to pure ASCII.

const (
	// brailleWordmark spells "costroid" in Braille (design-language §2). It is an
	// identity accent shown beside the plain wordmark, never a layout dependency.
	brailleWordmark = "⠉⠕⠎⠞⠗⠕⠊⠙"

	// Navigation selected-state dots. Selection is carried by the filled/hollow
	// shape, so it reads with zero color.
	glyphNavActive   = "●"
	glyphNavInactive = "·"
	asciiNavActive   = "*"
	asciiNavInactive = "."

	// Dotted rule between the header and the panel body.
	glyphRule = "┄"
	asciiRule = "-"

	// Meter cells: a full block, eighth-block partials for sub-cell precision, and
	// a faint empty cell. ASCII renders a bracketed [####----] bar instead.
	meterFull    = "█"
	meterEmpty   = "·"
	asciiBarFull = "#"
	asciiBarGap  = "-"
)

// meterPartials are the eighth-block fills for the fractional cell, indexed 1..7
// (index 0 is an empty cell, handled by the caller).
var meterPartials = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// sparkLevels are the eight vertical block levels low→high used for sparklines.
var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// wordmarkGlyph returns the Braille wordmark, or "" in ASCII mode (the plain
// wordmark alone leads there).
func (s Styles) wordmarkGlyph() string {
	if s.ASCII {
		return ""
	}
	return brailleWordmark
}

// navDot returns the selected-state dot for a navigation entry.
func (s Styles) navDot(active bool) string {
	if s.ASCII {
		if active {
			return asciiNavActive
		}
		return asciiNavInactive
	}
	if active {
		return glyphNavActive
	}
	return glyphNavInactive
}

// ruleChar returns the single character tiled to draw the dotted header rule.
func (s Styles) ruleChar() string {
	if s.ASCII {
		return asciiRule
	}
	return glyphRule
}

// brand returns the header identity token: the braille mark, the braille
// wordmark (UTF-8 only), and the plain wordmark. Braille leads in the terminal,
// always paired with the plain wordmark (design-language §2). ASCII degrades to
// the bracketed mark + plain wordmark, holding the layout without braille.
func (s Styles) brand() string {
	if w := s.wordmarkGlyph(); w != "" {
		return s.mark() + " " + w + " " + wordmark
	}
	return s.mark() + " " + wordmark
}

// The plain (--plain / NO_COLOR / non-UTF-8) fallback must be pure ASCII: no
// braille, no block glyphs, and no non-ASCII punctuation. These accessors gate
// the small punctuation glyphs on Styles.ASCII so the fallback contains only
// 7-bit characters while UTF-8 mode keeps the dot-rhythm typography.

// sepToken is the inline token separator: a middle dot in UTF-8, an ASCII pipe
// under --plain.
func (s Styles) sepToken() string {
	if s.ASCII {
		return " | "
	}
	return " · "
}

// ellipsis is "…" in UTF-8 and "..." under --plain.
func (s Styles) ellipsis() string {
	if s.ASCII {
		return "..."
	}
	return "…"
}

// times is the "×" multiplication sign in UTF-8 and "x" under --plain.
func (s Styles) times() string {
	if s.ASCII {
		return "x"
	}
	return "×"
}

// gte is "≥" in UTF-8 and ">=" under --plain.
func (s Styles) gte() string {
	if s.ASCII {
		return ">="
	}
	return "≥"
}
