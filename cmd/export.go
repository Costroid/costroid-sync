package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/output"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export usage data (csv, json, focus, markdown)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C6)", output.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
