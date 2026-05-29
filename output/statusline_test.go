package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/analysis"
)

func fptr(f float64) *float64 { return &f }
func iptr(i int) *int         { return &i }

var (
	genTime  = time.Date(2026, 5, 28, 18, 12, 0, 0, time.UTC)
	syncTime = genTime.Add(-4 * time.Hour) // → "4h" / 14400s
)

func render(t *testing.T, v StatuslineView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteStatusline(&buf, v); err != nil {
		t.Fatalf("WriteStatusline: %v", err)
	}
	return buf.String()
}

func okView(format string) StatuslineView {
	return StatuslineView{
		Status: StatusOK,
		Metrics: analysis.Statusline{
			MTDCostUSD: 38.0, ForecastUSD: fptr(99.39), BudgetPercent: iptr(60), AnomalyCount: 1,
		},
		LastSyncAt:  &syncTime,
		GeneratedAt: genTime,
		Format:      format,
	}
}

func assertSingleLine(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "\r") {
		t.Errorf("output contains carriage return: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output missing trailing newline: %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("output is not exactly one line: %q", out)
	}
}

func TestStatusline_PlainOKMatchesGrammar(t *testing.T) {
	out := render(t, okView("plain"))
	want := "⣿ costroid  MTD $38.00  forecast $99.39  budget 60%  anomalies 1  last sync 4h\n"
	if out != want {
		t.Errorf("plain ok:\n got %q\nwant %q", out, want)
	}
	assertSingleLine(t, out)
}

func TestStatusline_ASCIIFallbackGlyph(t *testing.T) {
	v := okView("plain")
	v.ASCIIGlyph = true
	out := render(t, v)
	if !strings.HasPrefix(out, "[costroid]  MTD ") {
		t.Errorf("ascii fallback prefix wrong: %q", out)
	}
	if strings.Contains(out, "⣿") {
		t.Errorf("ascii fallback still contains glyph: %q", out)
	}
}

func TestStatusline_OmitsForecastAndBudgetWhenAbsent(t *testing.T) {
	v := okView("plain")
	v.Metrics.ForecastUSD = nil
	v.Metrics.BudgetPercent = nil
	out := render(t, v)
	if strings.Contains(out, "forecast") {
		t.Errorf("forecast token present despite nil: %q", out)
	}
	if strings.Contains(out, "budget") {
		t.Errorf("budget token present despite nil: %q", out)
	}
	// Remaining tokens keep their order.
	want := "⣿ costroid  MTD $38.00  anomalies 1  last sync 4h\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStatusline_OverBudgetMarker(t *testing.T) {
	v := okView("plain")
	v.Metrics.BudgetPercent = iptr(118)
	v.Metrics.OverBudget = true
	out := render(t, v)
	if !strings.Contains(out, "budget 118% OVER") {
		t.Errorf("missing OVER marker: %q", out)
	}
}

func TestStatusline_NeverSynced(t *testing.T) {
	v := okView("plain")
	v.LastSyncAt = nil
	out := render(t, v)
	if !strings.Contains(out, "last sync never") {
		t.Errorf("never-synced wording missing: %q", out)
	}
}

func TestStatusline_MoneyGrouping(t *testing.T) {
	v := okView("plain")
	v.Metrics.MTDCostUSD = 1204.5
	v.Metrics.ForecastUSD = fptr(2310)
	out := render(t, v)
	if !strings.Contains(out, "MTD $1,204.50") {
		t.Errorf("thousands grouping wrong: %q", out)
	}
	if !strings.Contains(out, "forecast $2,310.00") {
		t.Errorf("forecast money wrong: %q", out)
	}
}

func TestStatusline_NoDataLines(t *testing.T) {
	for _, status := range []string{StatusEmpty, StatusMissingDB} {
		v := okView("plain")
		v.Status = status
		out := render(t, v)
		want := "costroid  no local data  run costroid-sync sync\n"
		if out != want {
			t.Errorf("%s line: got %q, want %q", status, out, want)
		}
		assertSingleLine(t, out)
	}
}

func TestStatusline_UnavailableLine(t *testing.T) {
	v := okView("plain")
	v.Status = StatusUnavailable
	out := render(t, v)
	want := "costroid  unavailable\n"
	if out != want {
		t.Errorf("unavailable line: got %q, want %q", out, want)
	}
}

func TestStatusline_TmuxColorAndSuppression(t *testing.T) {
	v := okView("tmux")
	v.Metrics.AnomalyCount = 2 // red anomalies token
	out := render(t, v)
	if !strings.Contains(out, "#[fg=green]MTD $38.00#[default]") {
		t.Errorf("expected green MTD style codes: %q", out)
	}
	if !strings.Contains(out, "#[fg=red]anomalies 2#[default]") {
		t.Errorf("expected red anomalies style codes: %q", out)
	}
	assertSingleLine(t, out)

	// NO_COLOR suppresses all style codes.
	v.NoColor = true
	out = render(t, v)
	if strings.Contains(out, "#[") {
		t.Errorf("NO_COLOR should suppress tmux style codes: %q", out)
	}

	// --plain also suppresses (and is checked independently of NoColor).
	v2 := okView("tmux")
	v2.PlainFlag = true
	if strings.Contains(render(t, v2), "#[") {
		t.Errorf("--plain should suppress tmux style codes")
	}
}

func TestStatusline_ByobuUsesANSI(t *testing.T) {
	v := okView("byobu")
	out := render(t, v)
	if !strings.Contains(out, "\033[32mMTD $38.00\033[0m") {
		t.Errorf("expected ANSI green MTD: %q", out)
	}
}

func TestStatusline_JSONOK(t *testing.T) {
	out := render(t, okView("json"))
	assertSingleLine(t, out)

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, out)
	}
	wantNum := map[string]float64{
		"mtd_cost_usd": 38, "forecast_cost_usd": 99.39, "budget_percent": 60,
		"anomaly_count": 1, "last_sync_age_seconds": 14400,
	}
	for k, want := range wantNum {
		if got, ok := m[k].(float64); !ok || got != want {
			t.Errorf("%s = %v, want %v", k, m[k], want)
		}
	}
	if m["status"] != "ok" || m["source"] != "local_sqlite" || m["currency"] != "USD" {
		t.Errorf("scalar fields wrong: %+v", m)
	}
	if m["last_sync_at"] != "2026-05-28T14:12:00Z" {
		t.Errorf("last_sync_at = %v", m["last_sync_at"])
	}
	if m["generated_at"] != "2026-05-28T18:12:00Z" {
		t.Errorf("generated_at = %v", m["generated_at"])
	}
}

func TestStatusline_JSONNullSemantics(t *testing.T) {
	v := okView("json")
	v.Metrics.ForecastUSD = nil
	v.Metrics.BudgetPercent = nil
	v.LastSyncAt = nil
	out := render(t, v)

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, k := range []string{"forecast_cost_usd", "budget_percent", "last_sync_at", "last_sync_age_seconds"} {
		val, ok := m[k]
		if !ok {
			t.Errorf("key %s missing; the contract keeps the field present as null", k)
		}
		if val != nil {
			t.Errorf("%s = %v, want null", k, val)
		}
	}
}

func TestStatusline_JSONNonOKStatesZeroAndNull(t *testing.T) {
	for _, status := range []string{StatusEmpty, StatusMissingDB, StatusUnavailable} {
		v := okView("json")
		v.Status = status
		v.LastSyncAt = nil
		out := render(t, v)

		var m map[string]interface{}
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("invalid JSON for %s: %v", status, err)
		}
		if m["status"] != status {
			t.Errorf("status = %v, want %s", m["status"], status)
		}
		if m["mtd_cost_usd"].(float64) != 0 || m["anomaly_count"].(float64) != 0 {
			t.Errorf("%s: numeric fields not zeroed: %+v", status, m)
		}
		for _, k := range []string{"forecast_cost_usd", "budget_percent", "last_sync_at", "last_sync_age_seconds"} {
			if m[k] != nil {
				t.Errorf("%s: %s = %v, want null", status, k, m[k])
			}
		}
	}
}

func TestStatusline_AgeFormatting(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{12 * time.Minute, "12m"},
		{4 * time.Hour, "4h"},
		{50 * time.Hour, "2d"},
		{-5 * time.Second, "0s"},
	}
	for _, c := range cases {
		if got := formatAge(c.d); got != c.want {
			t.Errorf("formatAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
