package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View local usage history",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C5)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
