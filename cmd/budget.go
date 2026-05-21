package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Set or check a spending budget",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C6)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(budgetCmd)
}
