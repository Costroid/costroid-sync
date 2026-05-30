package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubStage builds a Stage whose Run returns a fixed outcome. The model's state
// machine is driven directly via stageDoneMsg in these tests, so Run is only
// exercised where noted.
func stubStage(label string) Stage {
	return stubStageAction(label, "fetching metadata")
}

func stubStageAction(label, action string) Stage {
	return Stage{Label: label, Action: action, Run: func() StageOutcome {
		return StageOutcome{State: StageDone}
	}}
}

// sizedSyncModel returns a sync model that has received an initial size, so
// View renders the full checklist. Color off + ASCII on keeps output stable.
func sizedSyncModel(t *testing.T, stages []Stage) syncModel {
	t.Helper()
	m := newSyncModel(stages, Options{Color: false, ASCII: true})
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return nm.(syncModel)
}

func TestSyncModel_FirstStageRunsImmediately(t *testing.T) {
	m := newSyncModel([]Stage{stubStage("openai")}, Options{})
	if m.done {
		t.Fatal("model with stages should not start done")
	}
	if m.rows[0].state != StageRunning {
		t.Errorf("stage 0 = %v, want StageRunning", m.rows[0].state)
	}
	if m.Init() == nil {
		t.Error("Init should return a command (spinner tick + first stage)")
	}
}

func TestSyncModel_EmptyStagesQuitsImmediately(t *testing.T) {
	m := newSyncModel(nil, Options{})
	if !m.done {
		t.Fatal("empty-stage model should be done")
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("empty Init should return tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("empty Init should produce a QuitMsg")
	}
}

func TestSyncModel_SequentialAdvance(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai"), stubStage("sqlite")})

	nm, cmd := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)
	if m.rows[0].state != StageDone || m.rows[0].detail != "12 records" {
		t.Errorf("stage 0 = %+v, want Done/12 records", m.rows[0])
	}
	if m.rows[1].state != StageRunning {
		t.Errorf("stage 1 = %v, want StageRunning after stage 0", m.rows[1].state)
	}
	if cmd == nil {
		t.Error("advancing to a next stage should return its run command")
	}
	if m.done {
		t.Error("run should not be done mid-sequence")
	}

	nm, cmd = m.Update(stageDoneMsg{index: 1, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)
	if !m.done || m.aborted {
		t.Errorf("after last stage: done=%v aborted=%v, want done/!aborted", m.done, m.aborted)
	}
	if !m.completed() {
		t.Error("completed() should be true after all stages succeed")
	}
	if cmd != nil {
		t.Error("no further command expected after the final stage")
	}
}

func TestSyncModel_ErrorAbortsAndLeavesRemainingPending(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai"), stubStage("sqlite"), stubStage("analysis")})

	nm, cmd := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageError, Detail: "request failed"}})
	m = nm.(syncModel)
	if !m.aborted || !m.done {
		t.Errorf("error should abort+finish: aborted=%v done=%v", m.aborted, m.done)
	}
	if m.completed() {
		t.Error("an aborted run must not report completed()")
	}
	if cmd != nil {
		t.Error("no stage should run after an error")
	}
	if m.rows[1].state != StagePending || m.rows[2].state != StagePending {
		t.Errorf("remaining stages must stay Pending after an error: %v %v",
			m.rows[1].state, m.rows[2].state)
	}
}

func TestSyncModel_QuitKeysRestoreTerminal(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai")})
	for _, msg := range []tea.KeyMsg{runeKey("q"), {Type: tea.KeyCtrlC}} {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("quit key %v produced no command", msg)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("quit key %v did not produce a QuitMsg (terminal must restore)", msg)
		}
	}
}

func TestSyncModel_SpinnerStopsWhenDone(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai")})
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageDone}})
	m = nm.(syncModel)
	// A spinner tick after completion must not reschedule itself.
	if _, cmd := m.Update(m.spinner.Tick()); cmd != nil {
		t.Error("spinner should stop ticking once the run is done")
	}
}

// TestSyncView_CompletedStructure is the golden/structural View check on a fully
// completed, deterministic run. It also asserts the metadata-only / anti-slop
// boundary: no money ($) ever appears in the animated stage rows, and ASCII mode
// emits no braille/check glyphs.
func TestSyncView_CompletedStructure(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai"), stubStageAction("sqlite", "writing records")})
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)
	nm, _ = m.Update(stageDoneMsg{index: 1, outcome: StageOutcome{State: StageDone, Detail: "12 records"}})
	m = nm.(syncModel)

	out := m.View()
	for _, want := range []string{
		wordmark, "syncing usage metadata", "2/2",
		"openai", "fetching metadata", "OK done | 12 records",
		"sqlite", "writing records",
		"OK sync complete", "q to close",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "$") {
		t.Errorf("sync stages must never render money:\n%s", out)
	}
	for _, glyph := range []string{"⣿", "⣾", "✓", "✗"} {
		if strings.Contains(out, glyph) {
			t.Errorf("ASCII View leaked non-ASCII glyph %q", glyph)
		}
	}
}

func TestSyncStatusView_RunningShowsNeitherDoneNorFailed(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("openai")}) // stage 0 is Running
	got := m.statusView(m.rows[0])
	if strings.Contains(got, "done") || strings.Contains(got, "failed") {
		t.Errorf("running status should show neither done nor failed: %q", got)
	}
}

func TestSyncView_SkippedAndErrorMarkers(t *testing.T) {
	m := sizedSyncModel(t, []Stage{stubStage("anthropic"), stubStage("openai")})
	nm, _ := m.Update(stageDoneMsg{index: 0, outcome: StageOutcome{State: StageSkipped, Detail: "missing key"}})
	m = nm.(syncModel)
	nm, _ = m.Update(stageDoneMsg{index: 1, outcome: StageOutcome{State: StageError, Detail: "request failed"}})
	m = nm.(syncModel)

	out := m.View()
	for _, want := range []string{"skipped | missing key", "X failed | request failed", "X sync failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q\n---\n%s", want, out)
		}
	}
}
