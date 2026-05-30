package tui

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// tabBarWidth returns the rune width of the rendered tab bar with styling
// removed, used to assert the eight tabs fit one 80-column row.
func tabBarWidth(m model) int {
	return len([]rune(stripANSI(m.tabsLine())))
}

// TestOverviewBody_Golden pins the Overview panel render (a View fragment) for a
// deterministic, metadata-only fixture. ANSI is stripped so the comparison is
// stable regardless of the terminal color profile.
func TestOverviewBody_Golden(t *testing.T) {
	s := newStyles(false, true)
	got := stripANSI(overviewBody(demoDashboard(refTime()), s, 80))
	want := strings.Join([]string{
		"Month to date   $38.00",
		"Forecast        $99.39",
		"Budget          60% (monthly)",
		"Anomalies       1  ALERT",
		"Last sync       4h ago",
		"Window          last 45 days",
	}, "\n")
	if got != want {
		t.Errorf("overview body mismatch:\n got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestProvidersBody_Content(t *testing.T) {
	s := newStyles(false, true)
	got := stripANSI(providersBody(demoDashboard(refTime()), s, 80))
	for _, sub := range []string{"Provider", "Cost", "Tokens", "Records", "openai", "$30.00", "anthropic", "$8.00"} {
		if !strings.Contains(got, sub) {
			t.Errorf("providers body missing %q in:\n%s", sub, got)
		}
	}
}

func TestView_FullLayout(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40)
	out := stripANSI(m.View())
	for _, sub := range []string{wordmark, "Overview", "$38.00", "1 Overview", "8 Export"} {
		if !strings.Contains(out, sub) {
			t.Errorf("full view missing %q", sub)
		}
	}
	// The tab bar must fit one row at the 80-col minimum without wrapping.
	if w := tabBarWidth(m); w > fullMinWidth {
		t.Errorf("tab bar is %d cols, exceeds full-layout minimum width %d", w, fullMinWidth)
	}
	if lines := strings.Count(out, "\n") + 1; lines > 40 {
		t.Errorf("full view rendered %d lines, exceeds height 40", lines)
	}
}

func TestView_TooSmall(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 10, 2)
	if out := stripANSI(m.View()); !strings.Contains(out, "terminal too small") {
		t.Errorf("tiny terminal view = %q, want too-small message", out)
	}
}

func TestView_Compact(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 60, 20) // usable but < 80x24
	out := stripANSI(m.View())
	if !strings.Contains(out, "MTD $38.00") {
		t.Errorf("compact view missing summary row:\n%s", out)
	}
	if !strings.Contains(out, "enlarge") {
		t.Errorf("compact view missing enlarge hint:\n%s", out)
	}
}

func TestView_StatusScreens(t *testing.T) {
	for _, c := range []struct {
		status DataStatus
		want   string
	}{
		{DataMissingDB, "No local data"},
		{DataEmpty, "No local data"},
		{DataUnavailable, "unavailable"},
	} {
		d := demoDashboard(refTime())
		d.Status = c.status
		m := sizedModel(t, d, 100, 40)
		if out := stripANSI(m.View()); !strings.Contains(out, c.want) {
			t.Errorf("status %v view missing %q:\n%s", c.status, c.want, out)
		}
	}
}

// TestView_MetadataOnly is the metadata-only render guard (t1-feasibility §6):
// no rendered panel may contain a token that looks like prompt/completion/
// message content or a credential. The fixture is pure metadata, so any hit is
// a regression.
func TestView_MetadataOnly(t *testing.T) {
	forbidden := []string{
		"prompt", "completion", "message", "content", "request body", "response body",
		"bearer", "secret", "password", "api_key", "sk-", "system prompt",
	}
	var all strings.Builder
	m := sizedModel(t, demoDashboard(refTime()), 100, 40)
	for i := range m.panels {
		nm, _ := m.Update(runeKey(m.panels[i].num))
		m = nm.(model)
		all.WriteString(stripANSI(m.View()))
		all.WriteByte('\n')
	}
	hay := strings.ToLower(all.String())
	for _, bad := range forbidden {
		if strings.Contains(hay, bad) {
			t.Errorf("rendered TUI contains forbidden token %q", bad)
		}
	}
}

func TestStyles_ColorGating(t *testing.T) {
	on := newStyles(true, false)
	off := newStyles(false, false)
	if on.Accent.GetForeground() == off.Accent.GetForeground() {
		t.Error("accent foreground not gated by color flag")
	}
	if on.Alert.GetForeground() == off.Alert.GetForeground() {
		t.Error("alert foreground not gated by color flag")
	}
}

func TestStyles_ASCIIMark(t *testing.T) {
	if newStyles(true, true).mark() != asciiMark {
		t.Errorf("ascii mark = %q, want %q", newStyles(true, true).mark(), asciiMark)
	}
	if newStyles(true, false).mark() != glyphMark {
		t.Errorf("utf8 mark = %q, want %q", newStyles(true, false).mark(), glyphMark)
	}
}
