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
	lines := []string{m.syncHeader(), m.syncRule()}
	for _, r := range m.rows {
		lines = append(lines, m.rowView(r))
	}
	lines = append(lines, "", m.statusLine(), m.help.View(m.keys))
	return strings.Join(lines, "\n")
}

// syncHeader is the brand/context line above the stage checklist. It carries the
// braille mark + wordmark and a real-state dot-progress strip (filled dots =
// finished stages). The strip reflects only true stage outcomes — never a
// fabricated or animated value (terminal-design §17).
func (m syncModel) syncHeader() string {
	s := m.styles
	brand := s.Accent.Render(s.brand()) + "  " + s.Faint.Render("syncing usage metadata")
	dots := dotStrip(s, len(m.rows), m.finishedStages(), s.Accent)
	return brand + s.sepToken() + dots + "  " + s.Faint.Render(m.progressCount())
}

// syncRule is the faint dotted separator between the header and the stage rows.
func (m syncModel) syncRule() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	return m.styles.Faint.Render(strings.Repeat(m.styles.ruleChar(), w))
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
		return m.spinner.View() + " " + s.Faint.Render(s.ellipsis())
	case StageDone:
		return s.Accent.Render(m.glyphDone()+" done") + detailSuffix(s, r.detail)
	case StageSkipped:
		return s.Faint.Render(m.glyphSkip() + " skipped" + plainDetail(s, r.detail))
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
			s.Faint.Render(s.sepToken()+"run `costroid sync` for details"+s.sepToken()+"q to close")
	case m.done:
		return s.Accent.Render(m.glyphDone()+" sync complete") + s.Faint.Render(s.sepToken()+"q to close")
	default:
		return s.Faint.Render("syncing" + s.ellipsis() + s.sepToken() + "Ctrl-C to cancel")
	}
}

// syncTooSmallView is the compact one-line fallback for tiny terminals.
func (m syncModel) syncTooSmallView() string {
	if m.aborted {
		return "sync failed (" + m.progressCount() + ")"
	}
	if m.done {
		return "sync complete (" + m.progressCount() + ")" + m.styles.sepToken() + "q"
	}
	return "syncing" + m.styles.ellipsis() + " (" + m.progressCount() + ")"
}

// progressCount is "finished/total" where finished counts every terminal stage.
func (m syncModel) progressCount() string {
	return strconv.Itoa(m.finishedStages()) + "/" + strconv.Itoa(len(m.rows))
}

// finishedStages counts the stages that have reached a terminal outcome (done,
// skipped, or errored). It is the truth behind the header dot-progress strip.
func (m syncModel) finishedStages() int {
	finished := 0
	for _, r := range m.rows {
		if r.state == StageDone || r.state == StageSkipped || r.state == StageError {
			finished++
		}
	}
	return finished
}

// detailSuffix renders a faint separator + detail suffix when detail is non-empty.
func detailSuffix(s Styles, detail string) string {
	if detail == "" {
		return ""
	}
	return s.Faint.Render(s.sepToken() + detail)
}

// plainDetail renders a separator + detail (already inside a styled span) when
// non-empty. The separator degrades to ASCII under --plain via Styles.sepToken.
func plainDetail(s Styles, detail string) string {
	if detail == "" {
		return ""
	}
	return s.sepToken() + detail
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
