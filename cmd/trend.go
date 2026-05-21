package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var trendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Show usage trends over time",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C5)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(trendCmd)
}
