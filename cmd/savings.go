package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var savingsCmd = &cobra.Command{
	Use:   "savings",
	Short: "Show savings recommendations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C4)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(savingsCmd)
}
