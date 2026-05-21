package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/providers"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch usage from configured providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w (lands in C2)", providers.ErrNotImplemented)
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
