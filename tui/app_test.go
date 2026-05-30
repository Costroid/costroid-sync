package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/costroid/costroid-sync/analysis"
)

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// sizedModel builds a model that has received an initial WindowSizeMsg, so the
// viewport is laid out and View() renders the full dashboard.
func sizedModel(t *testing.T, d Dashboard, w, h int) model {
	t.Helper()
	m := newModel(d, Options{Color: false, ASCII: true})
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm.(model)
}

func TestUpdate_WindowSizeMakesReady(t *testing.T) {
	m := newModel(demoDashboard(refTime()), Options{})
	if m.ready {
		t.Fatal("model ready before WindowSizeMsg")
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	got := nm.(model)
	if !got.ready || got.width != 100 || got.height != 40 {
		t.Errorf("after resize: ready=%v w=%d h=%d", got.ready, got.width, got.height)
	}
}

func TestUpdate_PanelNavigation(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := next.(model).active; got != 1 {
		t.Errorf("after Tab active=%d, want 1", got)
	}

	jumped, _ := m.Update(runeKey("3"))
	if got := jumped.(model).active; got != 2 {
		t.Errorf("after '3' active=%d, want 2", got)
	}

	wrapped, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := wrapped.(model).active; got != len(m.panels)-1 {
		t.Errorf("after Shift-Tab active=%d, want %d", got, len(m.panels)-1)
	}

	// out-of-range jump key is ignored (only 1-8 are panels)
	if nm, _ := m.Update(runeKey("9")); nm.(model).active != 0 {
		t.Errorf("'9' should not change active panel")
	}
}

func TestUpdate_HelpToggle(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40)
	toggled, _ := m.Update(runeKey("?"))
	if !toggled.(model).help.ShowAll {
		t.Error("? did not expand help")
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	m := sizedModel(t, demoDashboard(refTime()), 100, 40)
	for _, msg := range []tea.KeyMsg{runeKey("q"), {Type: tea.KeyCtrlC}} {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("key %v produced no command, want tea.Quit", msg)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("key %v did not produce tea.QuitMsg", msg)
		}
	}
}

func TestInteractiveAllowed(t *testing.T) {
	cases := []struct {
		name          string
		stdout, stdin bool
		term          string
		want          bool
	}{
		{"both tty", true, true, "xterm-256color", true},
		{"stdout piped", false, true, "xterm", false},
		{"stdin piped", true, false, "xterm", false},
		{"term dumb", true, true, "dumb", false},
		{"term DUMB upper", true, true, "DUMB", false},
		{"empty term still ok on tty", true, true, "", true},
	}
	for _, c := range cases {
		if got := InteractiveAllowed(c.stdout, c.stdin, c.term); got != c.want {
			t.Errorf("%s: InteractiveAllowed(%v,%v,%q)=%v, want %v",
				c.name, c.stdout, c.stdin, c.term, got, c.want)
		}
	}
}

func TestJumpIndex(t *testing.T) {
	cases := map[string]int{"1": 0, "8": 7, "9": -1, "0": -1, "": -1, "12": -1, "a": -1}
	for in, want := range cases {
		if got := jumpIndex(in, 8); got != want {
			t.Errorf("jumpIndex(%q)=%d, want %d", in, got, want)
		}
	}
}

func refTime() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) }

// demoDashboard is a deterministic, metadata-only fixture shared by the view
// and update tests. Every value here is an aggregate count, name, money amount,
// or timestamp — never any prompt/completion/message content.
func demoDashboard(now time.Time) Dashboard {
	fc := 99.39
	pct := 60
	last := now.Add(-4 * time.Hour)
	return Dashboard{
		Status:      DataOK,
		GeneratedAt: now,
		WindowDays:  loadWindowDays,
		LastSync:    &last,
		Overview: analysis.Statusline{
			MTDCostUSD: 38.00, ForecastUSD: &fc, BudgetPercent: &pct, OverBudget: false, AnomalyCount: 1,
		},
		Forecast: &analysis.ForecastResult{
			CurrentMonthSpendUSD: 38.00, ForecastMonthEndUSD: 99.39, Method: "linear_regression",
			DaysObserved: 15, DaysRemaining: 2,
		},
		Budget: &analysis.BudgetStatus{
			Period: "monthly", BudgetAmountUSD: 60, SpendUSD: 36, RemainingUSD: 24, UsedPercent: 60,
		},
		Anomalies: []analysis.Anomaly{
			{Date: "2026-05-20", ActualCostUSD: 12, RollingAverageUSD: 4, DeviationPercent: 200},
		},
		Providers: []analysis.ProviderTotal{
			{Provider: "openai", CostUSD: 30, TotalTokens: 1000, Records: 5},
			{Provider: "anthropic", CostUSD: 8, TotalTokens: 500, Records: 2},
		},
		Models: []analysis.ModelTotal{
			{Provider: "openai", Model: "gpt-4o", CostUSD: 30, TotalTokens: 1000, Records: 5},
		},
		Savings: []analysis.SavingsRecommendation{
			{Provider: "openai", CurrentModel: "gpt-4o", RecommendedModel: "gpt-4o-mini",
				CurrentCostUSD: 30, EstimatedCostUSD: 6, SavingsUSD: 24, SavingsPercent: 80},
		},
		Syncs: []analysis.ProviderActivity{
			{Provider: "openai", LatestActive: now.Add(-2 * time.Hour)},
			{Provider: "anthropic", LatestActive: now.Add(-26 * time.Hour)},
		},
	}
}
