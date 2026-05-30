package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/tui"
)

var (
	tuiPlain       bool
	tuiNoAnimation bool
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open an interactive fullscreen dashboard of local cost metadata",
	Long: "Open an opt-in, keyboard-first fullscreen dashboard (alternate screen) of your local " +
		"cost metadata: overview, providers, models, budget, forecast, anomalies, recent syncs, " +
		"and export hints.\n\n" +
		"It reads the local SQLite database read-only and performs no network request, provider " +
		"API call, or provider sync — run `costroid-sync sync` separately. It is opt-in and changes " +
		"no other command's output. Navigate with Tab/1-8 and j/k; press ? for help and q to quit.\n\n" +
		"In a pipe or non-interactive terminal (or TERM=dumb) it refuses cleanly instead of painting " +
		"an alternate screen into a pipe.",
	Args: cobra.NoArgs,
	RunE: runTUI,
}

func init() {
	tuiCmd.Flags().BoolVar(&tuiPlain, "plain", false,
		"ASCII-only glyphs and no color/style codes")
	tuiCmd.Flags().BoolVar(&tuiNoAnimation, "no-animation", false,
		"disable animation (no-op in this version; the dashboard has no animation)")
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	if !tui.InteractiveAllowed(stdoutTTY, stdinTTY, os.Getenv("TERM")) {
		return fmt.Errorf("the tui needs an interactive terminal; pipe-friendly output is available " +
			"via `costroid-sync statusline` or `costroid-sync history`")
	}

	noColor := os.Getenv("NO_COLOR") != ""
	return tui.Run(cmd.Context(), tui.Options{
		Color:       !tuiPlain && !noColor,
		ASCII:       tuiPlain || !utf8Locale(),
		NoAnimation: tuiNoAnimation,
	})
}
