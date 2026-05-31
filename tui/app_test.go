package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/costroid/costroid/analysis"
)

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// sizedModel builds a model that has received an initial WindowSizeMsg, so the
// viewport is laid out and View() renders the full dashboard.
func sizedModel(t *testing.T, d Dashboard, w, h int) model {
	t.Helper()
	m := newModel(d, Options{ASCII: true})
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

	// '0' jumps to the tenth panel (index 9).
	if nm, _ := m.Update(runeKey("0")); nm.(model).active != 9 {
		t.Errorf("'0' should jump to the last panel (index 9)")
	}
	// a non-jump key leaves the active panel unchanged.
	if nm, _ := m.Update(runeKey("z")); nm.(model).active != 0 {
		t.Errorf("'z' should not change active panel")
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
	// With 8 panels, '9' and '0' map past the end and are rejected.
	cases := map[string]int{"1": 0, "8": 7, "9": -1, "0": -1, "": -1, "12": -1, "a": -1}
	for in, want := range cases {
		if got := jumpIndex(in, 8); got != want {
			t.Errorf("jumpIndex(%q, 8)=%d, want %d", in, got, want)
		}
	}
}

func TestJumpIndex_TenPanels(t *testing.T) {
	// With 10 panels, '1'..'9' select 0..8 and '0' selects the tenth (index 9).
	cases := map[string]int{"1": 0, "8": 7, "9": 8, "0": 9, "": -1, "10": -1, "a": -1}
	for in, want := range cases {
		if got := jumpIndex(in, 10); got != want {
			t.Errorf("jumpIndex(%q, 10)=%d, want %d", in, got, want)
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
		History: []analysis.DailyTotal{
			{Date: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC), CostUSD: 10, TotalTokens: 400},
			{Date: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC), CostUSD: 12, TotalTokens: 500},
			{Date: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC), CostUSD: 16, TotalTokens: 600},
		},
		TrendsWeekly: []analysis.TrendPeriod{
			{Period: "2026-W21", CostUSD: 22, TotalTokens: 900},
			{Period: "2026-W22", CostUSD: 16, TotalTokens: 600, ChangePercent: -27.3, HasChange: true},
		},
		TrendsMonthly: []analysis.TrendPeriod{
			{Period: "2026-05", CostUSD: 38, TotalTokens: 1500},
		},
		// Deterministic, metadata-only daily-spend series for the sparkline.
		Spark: []float64{1, 1, 2, 2, 3, 4, 4, 5, 6, 7, 6, 8, 9, 10},
	}
}
