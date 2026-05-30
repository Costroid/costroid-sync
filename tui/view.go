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
		m.ruleLine(),
		m.vp.View(),
		"",
		m.footerView(),
	}, "\n")
}

// headerView is the two-line header: the brand + current-period summary line,
// then the dot-rhythm panel nav.
func (m model) headerView() string {
	return m.summaryLine() + "\n" + m.tabsLine()
}

// brandToken renders the accented identity (mark + braille wordmark + wordmark).
func (m model) brandToken() string { return m.styles.Accent.Render(m.styles.brand()) }

// brandLine is the money-free identity + context line, safe in every data state
// (it is reused by the compact and status screens, where metrics may be absent).
func (m model) brandLine() string {
	s := m.styles
	sep := s.sepToken()
	return m.brandToken() +
		s.Faint.Render(sep+"ai cost"+sep+"last "+strconv.Itoa(m.data.WindowDays)+" days"+sep+"local sqlite")
}

// summaryLine is the full-dashboard header line: identity plus the current-period
// readout (MTD, forecast). Money is stable; this only ever shows when data is OK.
func (m model) summaryLine() string {
	s := m.styles
	mo := m.data.Overview
	seg := []string{m.brandToken(), s.Accent.Render("MTD " + money(mo.MTDCostUSD))}
	if mo.ForecastUSD != nil {
		seg = append(seg, "forecast "+money(*mo.ForecastUSD))
	}
	seg = append(seg, "last "+strconv.Itoa(m.data.WindowDays)+"d")
	return strings.Join(seg, s.Faint.Render(s.sepToken()))
}

// tabsLine is the dot-rhythm panel nav: each panel name carries a leading dot,
// filled for the active panel and hollow otherwise. Selection is conveyed by the
// dot shape (filled vs hollow), so it reads with zero color. Jump keys 1-8 stay
// in the help footer. Lowercase names keep the row inside the 80-column budget.
func (m model) tabsLine() string {
	s := m.styles
	parts := make([]string, len(m.panels))
	for i, p := range m.panels {
		active := i == m.active
		label := s.navDot(active) + strings.ToLower(p.tab)
		if active {
			parts[i] = s.Active.Render(label)
		} else {
			parts[i] = s.Inactive.Render(label)
		}
	}
	return strings.Join(parts, " ")
}

// ruleLine is the faint dotted separator between the header and the panel body.
func (m model) ruleLine() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	return m.styles.Faint.Render(strings.Repeat(m.styles.ruleChar(), w))
}

func (m model) footerView() string {
	return m.help.View(m.keys)
}

// compactView collapses the dashboard to a single statusline-style summary row
// when the terminal is usable but smaller than the full layout target.
func (m model) compactView() string {
	s := m.styles
	hint := s.Faint.Render("enlarge to " + s.gte() + strconv.Itoa(fullMinWidth) + s.times() +
		strconv.Itoa(fullMinHeight) + " for panels" + s.sepToken() + "? help" + s.sepToken() + "q quit")
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
	head := s.Accent.Render(s.brand())
	var msg string
	switch m.data.Status {
	case DataMissingDB, DataEmpty:
		msg = "No local data yet.\n\nRun  costroid sync  to fetch usage metadata,\n" +
			"then reopen the dashboard with  costroid tui."
	default:
		msg = "Local database unavailable.\n\nCheck COSTROID_DB or run  costroid sync."
	}
	return head + "\n\n" + msg + "\n\n" + s.Faint.Render("q to quit")
}

// tooSmallView is the single-line message shown below the tiny minimum size.
func (m model) tooSmallView() string {
	s := m.styles
	return "terminal too small (need " + s.gte() +
		strconv.Itoa(tinyMinWidth) + s.times() + strconv.Itoa(tinyMinHeight) + ")"
}

// lineCount returns the number of physical lines in s (0 for the empty string).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
