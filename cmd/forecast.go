package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var forecastCmd = &cobra.Command{
	Use:   "forecast",
	Short: "Predict month-end spend",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C5)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(forecastCmd)
}
