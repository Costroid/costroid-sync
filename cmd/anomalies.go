package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
)

var anomaliesCmd = &cobra.Command{
	Use:   "anomalies",
	Short: "List unusual spending days",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C5)", analysis.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(anomaliesCmd)
}
