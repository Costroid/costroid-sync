package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// syncKeyMap is the minimal keyboard map for the sync progress view: the run is
// non-interactive work, so only quit (always available, so Ctrl-C restores the
// terminal at any time) and the help toggle are bound. It implements
// help.KeyMap so the bubbles help component can render it.
type syncKeyMap struct {
	Help key.Binding
	Quit key.Binding
}

func newSyncKeyMap() syncKeyMap {
	return syncKeyMap{
		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k syncKeyMap) ShortHelp() []key.Binding  { return []key.Binding{k.Help, k.Quit} }
func (k syncKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{{k.Help, k.Quit}} }

// View renders the current frame. It never paints partial or animated money;
// the only animation is the active stage's spinner (a real in-flight task).
func (m syncModel) View() string {
	if !m.ready {
		return ""
	}
	if m.width < tinyMinWidth || m.height < tinyMinHeight {
		return m.syncTooSmallView()
	}
	lines := []string{m.syncHeader(), ""}
	for _, r := range m.rows {
		lines = append(lines, m.rowView(r))
	}
	lines = append(lines, "", m.statusLine(), m.help.View(m.keys))
	return strings.Join(lines, "\n")
}

// syncHeader is the single brand/context line above the stage checklist.
func (m syncModel) syncHeader() string {
	s := m.styles
	brand := s.Accent.Render(s.mark()+" "+wordmark) + "  " + s.Faint.Render("syncing usage metadata")
	return brand + "  ·  " + s.Faint.Render("live · local SQLite ("+m.progressCount()+")")
}

// rowView renders one stage as: label   action   status[ · detail].
func (m syncModel) rowView(r stageRow) string {
	s := m.styles
	label := padRight(r.label, m.labelW)
	action := s.Faint.Render(padRight(r.action, m.actionW))
	return label + "  " + action + "  " + m.statusView(r)
}

// statusView renders the right-hand status cell for a stage from its state. Red
// is reserved for errors and is always paired with the text marker "failed", so
// color is never the sole signal (terminal-design §8).
func (m syncModel) statusView(r stageRow) string {
	s := m.styles
	switch r.state {
	case StageRunning:
		return m.spinner.View() + " " + s.Faint.Render("…")
	case StageDone:
		return s.Accent.Render(m.glyphDone()+" done") + detailSuffix(s, r.detail)
	case StageSkipped:
		return s.Faint.Render(m.glyphSkip() + " skipped" + plainDetail(r.detail))
	case StageError:
		return s.Alert.Render(m.glyphErr()+" failed") + detailSuffix(s, r.detail)
	default: // StagePending
		return s.Faint.Render(m.glyphPending() + " waiting")
	}
}

// statusLine is the footer summarizing the overall run.
func (m syncModel) statusLine() string {
	s := m.styles
	switch {
	case m.aborted:
		return s.Alert.Render(m.glyphErr()+" sync failed") +
			s.Faint.Render(" · run `costroid-sync sync` for details · q to close")
	case m.done:
		return s.Accent.Render(m.glyphDone()+" sync complete") + s.Faint.Render(" · q to close")
	default:
		return s.Faint.Render("syncing… · Ctrl-C to cancel")
	}
}

// syncTooSmallView is the compact one-line fallback for tiny terminals.
func (m syncModel) syncTooSmallView() string {
	if m.aborted {
		return "sync failed (" + m.progressCount() + ")"
	}
	if m.done {
		return "sync complete (" + m.progressCount() + ") · q"
	}
	return "syncing… (" + m.progressCount() + ")"
}

// progressCount is "finished/total" where finished counts every terminal stage.
func (m syncModel) progressCount() string {
	finished := 0
	for _, r := range m.rows {
		if r.state == StageDone || r.state == StageSkipped || r.state == StageError {
			finished++
		}
	}
	return strconv.Itoa(finished) + "/" + strconv.Itoa(len(m.rows))
}

// detailSuffix renders a faint " · detail" suffix when detail is non-empty.
func detailSuffix(s Styles, detail string) string {
	if detail == "" {
		return ""
	}
	return s.Faint.Render(" · " + detail)
}

// plainDetail renders " · detail" (already inside a styled span) when non-empty.
func plainDetail(detail string) string {
	if detail == "" {
		return ""
	}
	return " · " + detail
}

func (m syncModel) glyphDone() string {
	if m.styles.ASCII {
		return "OK"
	}
	return "✓"
}

func (m syncModel) glyphErr() string {
	if m.styles.ASCII {
		return "X"
	}
	return "✗"
}

func (m syncModel) glyphSkip() string {
	if m.styles.ASCII {
		return "-"
	}
	return "·"
}

func (m syncModel) glyphPending() string {
	if m.styles.ASCII {
		return "."
	}
	return "·"
}
