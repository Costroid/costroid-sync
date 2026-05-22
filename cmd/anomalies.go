package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
	"github.com/costroid/costroid-sync/output"
)

var anomaliesCmd = &cobra.Command{
	Use:   "anomalies",
	Short: "List unusual spending days",
	RunE:  runAnomalies,
}

func init() {
	rootCmd.AddCommand(anomaliesCmd)
}

func runAnomalies(cmd *cobra.Command, args []string) error {
	records, err := readLocalRecords(cmd, time.Time{})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No usage records yet. Run `costroid-sync sync` first.")
		return nil
	}
	anomalies := analysis.DetectAnomalies(records)
	if len(anomalies) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No unusual spending days found.")
		return nil
	}
	output.WriteAnomalyTable(cmd.OutOrStdout(), anomalies)
	return nil
}
