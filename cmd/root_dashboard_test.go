package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/mattn/go-isatty"
)

// TestRootCmd_NonTTYPrintsHelp verifies that bare `costroid` in a non-interactive
// context (piped std streams or TERM=dumb) prints help instead of painting an
// alternate screen into a pipe. `go test` runs the binary with piped std streams,
// so this is the normal case; the guard skips if a real terminal is attached so
// the dashboard never blocks waiting for input.
func TestRootCmd_NonTTYPrintsHelp(t *testing.T) {
	interactive := (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) &&
		(isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())) &&
		strings.ToLower(os.Getenv("TERM")) != "dumb"
	if interactive {
		t.Skip("interactive terminal attached; skipping non-TTY help-fallback test")
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := runRoot(rootCmd, nil); err != nil {
		t.Fatalf("runRoot in a non-TTY returned error %v; expected a clean help fallback", err)
	}
	help := out.String()
	if !strings.Contains(help, "Usage:") {
		t.Errorf("non-TTY runRoot did not print help; got:\n%s", help)
	}
}

// TestRootCmd_DashboardFlags verifies the root command exposes the dashboard
// rendering flags so `costroid --plain` and `costroid --no-animation` work.
func TestRootCmd_DashboardFlags(t *testing.T) {
	if rootCmd.Flags().Lookup("plain") == nil {
		t.Error("root command missing --plain flag")
	}
	if rootCmd.Flags().Lookup("no-animation") == nil {
		t.Error("root command missing --no-animation flag")
	}
}

// TestRootCmd_NoTUISubcommand verifies the standalone `tui` subcommand was
// removed; the dashboard is reachable only via bare `costroid`.
func TestRootCmd_NoTUISubcommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "tui" {
			t.Fatalf("rootCmd still has a %q subcommand; the dashboard should be the default", c.Name())
		}
	}
}
