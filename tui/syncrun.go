package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// StageState is the lifecycle of one sync stage. Every value maps to a real
// task outcome — never a fabricated or cosmetic state (terminal-design §17:
// "Animation maps only to real sync task state").
type StageState int

const (
	StagePending StageState = iota // not started yet (no work done)
	StageRunning                   // work in flight (the only animated row)
	StageDone                      // real work succeeded
	StageSkipped                   // intentionally not run (e.g. missing key)
	StageError                     // real work failed; remaining stages do not run
)

// StageOutcome is the provider-free result a stage's Run returns. Detail is a
// short, already-sanitized metadata string ("12 records", "missing key") — it
// must never carry a raw provider payload, secret, credential, or error dump.
type StageOutcome struct {
	State  StageState
	Detail string
}

// Stage is one truthful unit of sync work. Label is the left column
// ("openai", "sqlite"); Action is the middle-column verb ("fetching metadata",
// "writing records"); Run performs the real work and returns its outcome.
//
// Run is supplied entirely by the cmd layer, so this package never imports
// providers/client/net/http (enforced by imports_test.go) — the metadata-only
// and no-network boundary is structural, not merely conventional.
type Stage struct {
	Label  string
	Action string
	Run    func() StageOutcome
}

// stageRow is the live render state of one stage.
type stageRow struct {
	label, action, detail string
	state                 StageState
}

// stageDoneMsg reports a finished stage so the model can record it and advance.
type stageDoneMsg struct {
	index   int
	outcome StageOutcome
}

type syncModel struct {
	rows    []stageRow
	runs    []func() StageOutcome
	spinner spinner.Model
	keys    syncKeyMap
	help    help.Model
	styles  Styles
	labelW  int
	actionW int
	cur     int  // index of the running/next stage
	done    bool // all stages finished, or a stage errored (run halted)
	aborted bool // a stage errored → remaining stages were not run
	width   int
	height  int
	ready   bool
}

func newSyncModel(stages []Stage, opts Options) syncModel {
	sp := spinner.New()
	styles := newStyles(surfaceWarm, opts.Tier, opts.ASCII)
	if opts.ASCII {
		sp.Spinner = spinner.Line // ASCII frames: | / - \
	} else {
		sp.Spinner = spinner.MiniDot // braille dot, matches the dot identity
	}
	if opts.Tier != mono {
		sp.Style = styles.meterFill() // warm ramp — the in-flight process voice
	}
	rows := make([]stageRow, len(stages))
	runs := make([]func() StageOutcome, len(stages))
	labelW, actionW := 0, 0
	for i, s := range stages {
		rows[i] = stageRow{label: s.Label, action: s.Action, state: StagePending}
		runs[i] = s.Run
		labelW = max(labelW, len(s.Label))
		actionW = max(actionW, len(s.Action))
	}
	m := syncModel{
		rows: rows, runs: runs, spinner: sp, keys: newSyncKeyMap(),
		help: newHelp(opts.ASCII), styles: styles, labelW: labelW, actionW: actionW,
	}
	if len(rows) == 0 {
		m.done = true
	} else {
		m.rows[0].state = StageRunning
	}
	return m
}

func (m syncModel) Init() tea.Cmd {
	if m.done {
		return tea.Quit
	}
	return tea.Batch(m.spinner.Tick, m.runStage(0))
}

// runStage returns a command that executes stage i on a background goroutine and
// reports the outcome. Stages are issued one at a time (advance only starts the
// next stage after the current one's message arrives), so the cmd-side work
// never runs concurrently — keeping the shared record accumulator race-free.
func (m syncModel) runStage(i int) tea.Cmd {
	run := m.runs[i]
	return func() tea.Msg { return stageDoneMsg{index: i, outcome: run()} }
}

func (m syncModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.ready = true
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		if m.done {
			return m, nil // stop ticking once there is no active stage
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case stageDoneMsg:
		return m.advance(msg)
	}
	return m, nil
}

// handleKey handles the only two interactions: quit (always available, so
// Ctrl-C restores the terminal at any time) and the help toggle.
func (m syncModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}
	return m, nil
}

// advance records a finished stage and either halts (on error) or starts the
// next stage. Remaining stages after an error are left Pending (they truthfully
// did not run) rather than marked skipped.
func (m syncModel) advance(msg stageDoneMsg) (tea.Model, tea.Cmd) {
	r := &m.rows[msg.index]
	r.state = msg.outcome.State
	r.detail = msg.outcome.Detail
	if msg.outcome.State == StageError {
		m.aborted = true
		m.done = true
		return m, nil
	}
	next := msg.index + 1
	if next >= len(m.rows) {
		m.done = true
		return m, nil
	}
	m.cur = next
	m.rows[next].state = StageRunning
	return m, m.runStage(next)
}

// completed reports whether every stage finished without error. The cmd layer
// reads the synced records and prints its summary only when this is true; at
// that point no stage goroutine is still running, so the read is race-free.
func (m syncModel) completed() bool { return m.done && !m.aborted }

// Options reuse note: RunSync shares tui.Options with the dashboard. Only
// Color/ASCII/Now are consulted here; NoAnimation is handled in the cmd layer
// (it falls back to plain sync output, so the TUI is never launched with it).

// RunSync launches the opt-in alternate-screen sync progress view over the given
// real stages and reports whether the sync completed. Bubble Tea restores the
// terminal (leaves the alt screen, restores cooked mode, shows the cursor) on
// every exit path — normal quit, Ctrl-C/SIGINT, SIGTERM, and panic.
//
// ctx is the program lifetime only; cancelling it quits the view. Per-stage
// work (provider fetch timeouts, cancellation) is owned by the cmd-side Run
// closures, so the user's idle "press q to close" time never times out a fetch.
func RunSync(ctx context.Context, opts Options, stages []Stage) (completed bool, err error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	p := tea.NewProgram(newSyncModel(stages, opts), tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	sm, ok := final.(syncModel)
	return ok && sm.completed(), nil
}
