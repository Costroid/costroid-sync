package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/output"
	"github.com/costroid/costroid/storage"
)

var savingsCmd = &cobra.Command{
	Use:   "savings",
	Short: "Show savings recommendations from local usage",
	RunE:  runSavings,
}

func init() {
	rootCmd.AddCommand(savingsCmd)
}

func runSavings(cmd *cobra.Command, args []string) error {
	dbPath, err := storage.ResolveDBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	db, err := storage.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	records, err := storage.GetRecords(ctx, db, time.Time{})
	if err != nil {
		return fmt.Errorf("read records: %w", err)
	}
	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No usage records yet. Run `costroid sync` first.")
		return nil
	}

	recs := analysis.Recommend(records)
	if len(recs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No savings recommendations — you're already on the cheapest known model per provider.")
		return nil
	}
	output.WriteSavingsTable(cmd.OutOrStdout(), recs)
	return nil
}
