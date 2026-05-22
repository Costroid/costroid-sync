package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/output"
	"github.com/costroid/costroid-sync/providers"
)

var historyLast string

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View local usage history",
	RunE:  runHistory,
}

func init() {
	historyCmd.Flags().StringVar(&historyLast, "last", "30d", "lookback window, such as 7d, 30d, or 90d")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	days, err := parseLastDays(historyLast)
	if err != nil {
		return err
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	records, err := readLocalRecords(cmd, since)
	if err != nil {
		return err
	}
	sortHistoryRecords(records)
	if len(records) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No usage records for the last %dd. Run `costroid-sync sync` first.\n", days)
		return nil
	}
	output.WriteHistoryTable(cmd.OutOrStdout(), records)
	return nil
}

func parseLastDays(value string) (int, error) {
	if !strings.HasSuffix(value, "d") {
		return 0, fmt.Errorf("--last must be a day window like 7d, 30d, or 90d")
	}
	days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || days < 1 {
		return 0, fmt.Errorf("--last must be a positive day window like 7d, 30d, or 90d")
	}
	return days, nil
}

func sortHistoryRecords(records []providers.NormalizedCostRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].RecordedAt > records[j].RecordedAt
	})
}
