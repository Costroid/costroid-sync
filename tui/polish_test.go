package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestBrand_BrailleAndASCIIFallback proves the header identity carries the
// braille mark + braille wordmark in UTF-8 mode, and degrades to the bracketed
// mark + plain wordmark in ASCII with no braille leaking (the layout must hold
// without braille — design-language §6).
func TestBrand_BrailleAndASCIIFallback(t *testing.T) {
	utf := newStyles(surfaceCold, ansi16, false).brand()
	for _, want := range []string{glyphMark, brailleWordmark, wordmark} {
		if !strings.Contains(utf, want) {
			t.Errorf("utf-8 brand missing %q in %q", want, utf)
		}
	}
	asc := newStyles(surfaceCold, mono, true).brand()
	if !strings.Contains(asc, wordmark) || !strings.Contains(asc, asciiMark) {
		t.Errorf("ascii brand = %q, want %q + %q", asc, asciiMark, wordmark)
	}
	for _, bad := range []string{glyphMark, brailleWordmark} {
		if strings.Contains(asc, bad) {
			t.Errorf("ascii brand leaked non-ASCII glyph %q", bad)
		}
	}
}

// TestHeader_BrailleInNormalMode renders the full dashboard with UTF-8 styling
// and confirms the braille mark + wordmark survive into the painted frame.
func TestHeader_BrailleInNormalMode(t *testing.T) {
	m := newModel(demoDashboard(refTime()), Options{Tier: ansi16, ASCII: false})
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	out := stripANSI(nm.(model).View())
	for _, want := range []string{glyphMark, brailleWordmark} {
		if !strings.Contains(out, want) {
			t.Errorf("normal-mode header missing %q", want)
		}
	}
}

// TestNav_SelectedStateVisibleWithoutColor proves panel selection reads from the
// dot shape alone (filled vs hollow), with color stripped — the active marker
// must move with the selection and never sit on two panels at once.
func TestNav_SelectedStateVisibleWithoutColor(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40) // color off, ascii on
	nav := stripANSI(m.tabsLine())
	if !strings.HasPrefix(nav, asciiNavActive+"ovw") {
		t.Errorf("active panel not marked with filled dot: %q", nav)
	}
	if !strings.Contains(nav, asciiNavInactive+"prov") {
		t.Errorf("inactive panel not marked with hollow dot: %q", nav)
	}
	nm, _ := m.Update(runeKey("3")) // jump to Models (index 2)
	nav2 := stripANSI(nm.(model).tabsLine())
	if !strings.Contains(nav2, asciiNavActive+"models") {
		t.Errorf("active marker did not move to models: %q", nav2)
	}
	if strings.Contains(nav2, asciiNavActive+"ovw") {
		t.Errorf("overview must no longer be active: %q", nav2)
	}
}

func TestMeter_FillClampAndASCII(t *testing.T) {
	s := newStyles(surfaceCold, mono, true) // ascii
	cases := map[float64]string{0: "[----]", 0.5: "[##--]", 1: "[####]", 1.5: "[####]"}
	for frac, want := range cases {
		if got := stripANSI(meter(s, frac, 4, s.Accent)); got != want {
			t.Errorf("meter(%v,4) = %q, want %q", frac, got, want)
		}
	}
}

func TestMeter_UTF8Blocks(t *testing.T) {
	s := newStyles(surfaceCold, mono, false) // utf-8, color off
	got := stripANSI(meter(s, 0.5, 4, s.Accent))
	if !strings.Contains(got, meterFull) {
		t.Errorf("utf-8 meter missing full block: %q", got)
	}
	if strings.Contains(got, "[") {
		t.Errorf("utf-8 meter must not use the ASCII bracket form: %q", got)
	}
}

func TestSparkline_RampEmptyAndASCII(t *testing.T) {
	utf := newStyles(surfaceCold, mono, false)
	got := []rune(stripANSI(sparkline(utf, []float64{0, 5, 10}, utf.Accent)))
	if len(got) != 3 || got[0] != sparkLevels[0] || got[2] != sparkLevels[len(sparkLevels)-1] {
		t.Errorf("sparkline ramp = %q", string(got))
	}
	if sparkline(newStyles(surfaceCold, mono, true), []float64{1, 2, 3}, newStyles(surfaceCold, mono, true).Accent) != "" {
		t.Error("ascii sparkline must be empty (numbers are shown instead)")
	}
	if sparkline(utf, nil, utf.Accent) != "" {
		t.Error("empty-series sparkline must be empty")
	}
}

func TestDotStrip_RealFillAndClamp(t *testing.T) {
	s := newStyles(surfaceCold, mono, true) // ascii: filled "*", hollow "."
	cases := []struct {
		total, filled int
		want          string
	}{{3, 0, "..."}, {3, 1, "*.."}, {3, 3, "***"}, {3, 5, "***"}, {0, 0, ""}}
	for _, c := range cases {
		if got := stripANSI(dotStrip(s, c.total, c.filled, s.Accent)); got != c.want {
			t.Errorf("dotStrip(%d,%d) = %q, want %q", c.total, c.filled, got, c.want)
		}
	}
}

func TestDailySparkSeries_Buckets(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	pts := []spendPoint{
		{at: now, cost: 3},
		{at: now.Add(-2 * time.Hour), cost: 1}, // same UTC day → today = 4
		{at: now.AddDate(0, 0, -1), cost: 5},   // yesterday
		{at: now.AddDate(0, 0, -10), cost: 99}, // outside the 3-day window
	}
	got := dailySparkSeries(pts, now, 3)
	want := []float64{0, 5, 4} // day-2, day-1, today (oldest→newest)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("series = %v, want %v", got, want)
		}
	}
	if dailySparkSeries(nil, now, 0) != nil {
		t.Error("days < 1 must return nil")
	}
}

// TestOverviewBody_SparklineInUTF8 proves the recent-spend sparkline renders in
// normal mode (using a level unique to the vertical sparkline, not the meter).
func TestOverviewBody_SparklineInUTF8(t *testing.T) {
	s := newStyles(surfaceCold, mono, false) // utf-8, color off
	got := stripANSI(overviewBody(demoDashboard(refTime()), s, 80))
	has := false
	for _, r := range []rune("▂▃▄▅▆▇") { // mid sparkline levels, not used by the meter
		if strings.ContainsRune(got, r) {
			has = true
		}
	}
	if !has {
		t.Errorf("utf-8 overview missing sparkline glyphs:\n%s", got)
	}
}

// TestView_PlainFallbackASCIISafe renders every panel in ASCII mode and proves no
// load-bearing braille/block glyph or nav dot leaks, while the key readouts stay
// present and readable (NO_COLOR / --plain degradation).
func TestView_PlainFallbackASCIISafe(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40) // color off, ascii on
	var all strings.Builder
	for i := range m.panels {
		nm, _ := m.Update(runeKey(m.panels[i].num))
		m = nm.(model)
		all.WriteString(stripANSI(m.View()))
		all.WriteByte('\n')
	}
	out := all.String()
	for _, glyph := range []string{glyphMark, brailleWordmark, meterFull, glyphNavActive, glyphRule, "▁", "▂", "▄", "▇", "⡿", "⣿"} {
		if strings.Contains(out, glyph) {
			t.Errorf("ASCII fallback leaked non-ASCII glyph %q", glyph)
		}
	}
	for _, want := range []string{wordmark, "MTD $38.00", "[", "#"} {
		if !strings.Contains(out, want) {
			t.Errorf("ASCII fallback missing readable token %q", want)
		}
	}
}

// firstNonASCII returns the first rune > 127 in s, or "" when s is pure ASCII.
func firstNonASCII(s string) string {
	for _, r := range s {
		if r > 127 {
			return string(r)
		}
	}
	return ""
}

// TestSepToken_GatedByMode pins the punctuation gating: UTF-8 keeps the dot
// rhythm, --plain uses pure ASCII (no middle dot, ellipsis, ×, or ≥).
func TestSepToken_GatedByMode(t *testing.T) {
	utf := newStyles(surfaceCold, ansi16, false)
	asc := newStyles(surfaceCold, mono, true)
	cases := []struct {
		name             string
		utfVal, asciiVal string
	}{
		{"sep", utf.sepToken(), asc.sepToken()},
		{"ellipsis", utf.ellipsis(), asc.ellipsis()},
		{"times", utf.times(), asc.times()},
		{"gte", utf.gte(), asc.gte()},
	}
	for _, c := range cases {
		if firstNonASCII(c.utfVal) == "" {
			t.Errorf("%s: UTF-8 value %q lost its non-ASCII glyph", c.name, c.utfVal)
		}
		if bad := firstNonASCII(c.asciiVal); bad != "" {
			t.Errorf("%s: --plain value %q contains non-ASCII %q", c.name, c.asciiVal, bad)
		}
	}
}

// TestPlainRender_DashboardIsPureASCII renders every dashboard surface in ASCII
// mode — all panels, the expanded help overlay, the compact/too-small fallbacks,
// and the empty/missing/unavailable status screens — and asserts the painted
// frame contains no non-ASCII byte at all (full --plain ASCII-safety).
func TestPlainRender_DashboardIsPureASCII(t *testing.T) {
	var b strings.Builder
	m := sizedModel(t, demoDashboard(refTime()), 100, 40) // color off, ascii on
	for i := range m.panels {
		nm, _ := m.Update(runeKey(m.panels[i].num))
		m = nm.(model)
		b.WriteString(stripANSI(m.View()))
		b.WriteByte('\n')
	}
	hm, _ := m.Update(runeKey("?")) // expanded help overlay
	b.WriteString(stripANSI(hm.(model).View()))
	b.WriteString(stripANSI(sizedModel(t, demoDashboard(refTime()), 60, 20).View())) // compact
	b.WriteString(stripANSI(sizedModel(t, demoDashboard(refTime()), 10, 2).View()))  // too small
	for _, st := range []DataStatus{DataMissingDB, DataEmpty, DataUnavailable} {
		d := demoDashboard(refTime())
		d.Status = st
		b.WriteString(stripANSI(sizedModel(t, d, 100, 40).View()))
	}
	if bad := firstNonASCII(b.String()); bad != "" {
		t.Errorf("plain dashboard render contains non-ASCII %q:\n%s", bad, b.String())
	}
}

// TestPlainRender_SyncIsPureASCII renders the sync view in every state in ASCII
// mode (running, skipped→done, error, and the tiny fallback) and asserts the
// frame is pure ASCII.
func TestPlainRender_SyncIsPureASCII(t *testing.T) {
	var b strings.Builder
	m := sizedSyncModel(t, []Stage{stubStage("openai"), stubStage("sqlite")}) // stage 0 running
	b.WriteString(stripANSI(m.View()))
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageSkipped, Detail: "missing key"}})
	m = nm.(syncModel)
	nm, _ = m.Update(stageDoneMsg{index: 1, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	b.WriteString(stripANSI(nm.(syncModel).View()))

	me := sizedSyncModel(t, []Stage{stubStage("a")})
	nme, _ := me.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageError, Detail: "request failed"}})
	b.WriteString(stripANSI(nme.(syncModel).View()))

	mt := newSyncModel([]Stage{stubStage("a")}, Options{ASCII: true})
	nmt, _ := mt.Update(tea.WindowSizeMsg{Width: 10, Height: 2})
	b.WriteString(stripANSI(nmt.(syncModel).View()))

	if bad := firstNonASCII(b.String()); bad != "" {
		t.Errorf("plain sync render contains non-ASCII %q:\n%s", bad, b.String())
	}
}

// TestSyncView_MetadataOnly is the sync-progress metadata-only guard: no rendered
// token may look like prompt/completion/message/content, a credential, or a raw
// provider payload (terminal-design §17).
func TestSyncView_MetadataOnly(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai"), stubStageAction("sqlite", "writing records")})
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)
	nm, _ = m.Update(stageDoneMsg{index: 1, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)

	hay := strings.ToLower(stripANSI(m.View()))
	forbidden := []string{
		"prompt", "completion", "message", "content", "request body", "response body",
		"bearer", "secret", "password", "api_key", "sk-", "system prompt",
		"credential", "raw payload",
	}
	for _, bad := range forbidden {
		if strings.Contains(hay, bad) {
			t.Errorf("sync view contains forbidden token %q", bad)
		}
	}
}

// TestSyncHeader_ProgressReflectsRealState proves the header progress count (and
// thus the dot-progress strip) tracks only real finished stages, never a
// fabricated value. The Running stage 0 is not "finished" until its outcome lands.
func TestSyncHeader_ProgressReflectsRealState(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("a"), stubStage("b"), stubStage("c")})
	if got := m.finishedStages(); got != 0 {
		t.Errorf("no outcome yet: finishedStages = %d, want 0", got)
	}
	if !strings.Contains(stripANSI(m.syncHeader()), "0/3") {
		t.Errorf("header should show 0/3: %q", stripANSI(m.syncHeader()))
	}
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageDone}})
	m = nm.(syncModel)
	if got := m.finishedStages(); got != 1 {
		t.Errorf("after one done: finishedStages = %d, want 1", got)
	}
	if !strings.Contains(stripANSI(m.syncHeader()), "1/3") {
		t.Errorf("header should show 1/3: %q", stripANSI(m.syncHeader()))
	}
}
