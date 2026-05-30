package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/mattn/go-isatty"
)

// TestTUICmd_RefusesNonTTY verifies the opt-in TUI refuses cleanly (a friendly
// error, not an alternate screen painted into a pipe) when stdout/stdin are not
// interactive. `go test` runs the binary with piped std streams, so this is the
// normal case; the guard skips if a real terminal is somehow attached, so the
// program never blocks waiting for input.
func TestTUICmd_RefusesNonTTY(t *testing.T) {
	interactive := (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) &&
		(isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())) &&
		strings.ToLower(os.Getenv("TERM")) != "dumb"
	if interactive {
		t.Skip("interactive terminal attached; skipping non-TTY refusal test")
	}

	err := runTUI(tuiCmd, nil)
	if err == nil {
		t.Fatal("runTUI in a non-TTY returned nil; expected a clean refusal error")
	}
	if !strings.Contains(err.Error(), "interactive terminal") {
		t.Errorf("refusal error = %q, want it to mention an interactive terminal", err.Error())
	}
}

func TestTUICmd_Flags(t *testing.T) {
	if tuiCmd.Flags().Lookup("plain") == nil {
		t.Error("tui command missing --plain flag")
	}
	if tuiCmd.Flags().Lookup("no-animation") == nil {
		t.Error("tui command missing --no-animation flag")
	}
}
