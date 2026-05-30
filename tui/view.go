package tui

import (
	"strconv"
	"strings"
)

// View renders the current frame. It picks a layout from the terminal size and
// data status: too-small message, friendly empty/missing screen, compact
// summary, or the full dashboard. It never paints partial/animated content.
func (m model) View() string {
	if !m.ready {
		return ""
	}
	if m.width < tinyMinWidth || m.height < tinyMinHeight {
		return m.tooSmallView()
	}
	if m.data.Status != DataOK {
		return m.statusView()
	}
	if m.width < fullMinWidth || m.height < fullMinHeight {
		return m.compactView()
	}
	return m.fullView()
}

func (m model) fullView() string {
	return strings.Join([]string{
		m.headerView(),
		"",
		m.vp.View(),
		"",
		m.footerView(),
	}, "\n")
}

// headerView is the two-line header: brand/context line + panel tab bar.
func (m model) headerView() string {
	return m.brandLine() + "\n" + m.tabsLine()
}

func (m model) brandLine() string {
	s := m.styles
	brand := s.Accent.Render(s.mark()+" "+wordmark) + "  " + s.Faint.Render("AI cost dashboard")
	context := s.Faint.Render("last " + strconv.Itoa(m.data.WindowDays) + " days · local SQLite")
	return brand + "  ·  " + context
}

func (m model) tabsLine() string {
	parts := make([]string, len(m.panels))
	for i, p := range m.panels {
		label := p.num + " " + p.tab
		if i == m.active {
			parts[i] = m.styles.Active.Render(label)
		} else {
			parts[i] = m.styles.Inactive.Render(label)
		}
	}
	return strings.Join(parts, " ")
}

func (m model) footerView() string {
	return m.help.View(m.keys)
}

// compactView collapses the dashboard to a single statusline-style summary row
// when the terminal is usable but smaller than the full layout target.
func (m model) compactView() string {
	hint := m.styles.Faint.Render("enlarge to ≥80×24 for panels · ? help · q quit")
	return strings.Join([]string{m.brandLine(), "", m.compactSummary(), "", hint}, "\n")
}

// compactSummary mirrors the one-line statusline grammar from the Overview
// metrics. Color is never the sole signal: OVER / anomaly markers are textual.
func (m model) compactSummary() string {
	s := m.styles
	mo := m.data.Overview
	seg := []string{s.Accent.Render("MTD " + money(mo.MTDCostUSD))}
	if mo.ForecastUSD != nil {
		seg = append(seg, "forecast "+money(*mo.ForecastUSD))
	}
	if mo.BudgetPercent != nil {
		b := "budget " + strconv.Itoa(*mo.BudgetPercent) + "%"
		if mo.OverBudget {
			b = s.Alert.Render(b + " OVER")
		}
		seg = append(seg, b)
	}
	an := "anomalies " + strconv.Itoa(mo.AnomalyCount)
	if mo.AnomalyCount > 0 {
		an = s.Alert.Render(an)
	}
	seg = append(seg, an, "last sync "+syncAge(m.data.LastSync, m.data.GeneratedAt))
	return strings.Join(seg, "  ")
}

// statusView is the friendly screen for a missing/empty/unavailable database.
func (m model) statusView() string {
	s := m.styles
	head := s.Accent.Render(s.mark() + " " + wordmark)
	var msg string
	switch m.data.Status {
	case DataMissingDB, DataEmpty:
		msg = "No local data yet.\n\nRun  costroid-sync sync  to fetch usage metadata,\n" +
			"then reopen the dashboard with  costroid-sync tui."
	default:
		msg = "Local database unavailable.\n\nCheck COSTROID_DB or run  costroid-sync sync."
	}
	return head + "\n\n" + msg + "\n\n" + s.Faint.Render("q to quit")
}

// tooSmallView is the single-line message shown below the tiny minimum size.
func (m model) tooSmallView() string {
	return "terminal too small (need ≥" +
		strconv.Itoa(tinyMinWidth) + "×" + strconv.Itoa(tinyMinHeight) + ")"
}

// lineCount returns the number of physical lines in s (0 for the empty string).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
