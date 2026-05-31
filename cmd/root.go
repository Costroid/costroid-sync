package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/costroid/costroid/tui"
)

// Version is overridden at link time:
//
//	go build -o costroid -ldflags "-X github.com/costroid/costroid/cmd.Version=v1.2.3" .
var Version = "dev"

var (
	rootPlain       bool
	rootNoAnimation bool
)

var rootCmd = &cobra.Command{
	Use:   "costroid",
	Short: "Track and forecast AI/LLM spending from your terminal",
	Long: "costroid is the open-source Costroid CLI for tracking, forecasting, and alerting on " +
		"AI/LLM spend across providers using metadata only.\n\n" +
		"Run `costroid` with no arguments to open the interactive fullscreen dashboard: a " +
		"keyboard-first view of your local cost metadata (overview, providers, models, budget, " +
		"forecast, anomalies, history, trend, recent syncs, and export hints). It reads the local " +
		"SQLite database read-only and makes no network or provider API call — run `costroid sync` " +
		"to fetch usage metadata first. In a pipe, CI, or non-interactive terminal it prints this " +
		"help instead of painting a dashboard.",
	Version: Version,
	Args:    cobra.NoArgs,
	RunE:    runRoot,
}

func init() {
	rootCmd.Flags().BoolVar(&rootPlain, "plain", false,
		"dashboard: ASCII-only glyphs and no color/style codes")
	rootCmd.Flags().BoolVar(&rootNoAnimation, "no-animation", false,
		"dashboard: disable animation (no-op; the dashboard has no animation)")
}

func Execute() error {
	return rootCmd.Execute()
}

// runRoot is the default action for bare `costroid`. In an interactive terminal
// it opens the fullscreen dashboard; in a pipe, CI, or TERM=dumb it prints help
// instead of painting an alternate screen into a non-interactive stream.
func runRoot(cmd *cobra.Command, args []string) error {
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	if !tui.InteractiveAllowed(stdoutTTY, stdinTTY, os.Getenv("TERM")) {
		return cmd.Help()
	}

	noColor := os.Getenv("NO_COLOR") != ""
	return tui.Run(cmd.Context(), tui.Options{
		Tier:        tui.ResolveTier(rootPlain, noColor, os.Getenv("COLORTERM"), os.Getenv("TERM")),
		ASCII:       rootPlain || !utf8Locale(),
		NoAnimation: rootNoAnimation,
	})
}
