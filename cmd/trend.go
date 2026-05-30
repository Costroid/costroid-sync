package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/output"
)

var (
	trendWeekly  bool
	trendMonthly bool
)

var trendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Show usage trends over time",
	RunE:  runTrend,
}

func init() {
	trendCmd.Flags().BoolVar(&trendWeekly, "weekly", false, "aggregate by ISO week")
	trendCmd.Flags().BoolVar(&trendMonthly, "monthly", false, "aggregate by calendar month")
	rootCmd.AddCommand(trendCmd)
}

func runTrend(cmd *cobra.Command, args []string) error {
	interval, err := selectedTrendInterval()
	if err != nil {
		return err
	}
	records, err := readLocalRecords(cmd, time.Time{})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No usage records yet. Run `costroid sync` first.")
		return nil
	}
	periods := analysis.Trends(records, interval)
	if len(periods) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No trend data available from local records.")
		return nil
	}
	output.WriteTrendTable(cmd.OutOrStdout(), periods)
	return nil
}

func selectedTrendInterval() (analysis.TrendInterval, error) {
	if trendWeekly && trendMonthly {
		return "", fmt.Errorf("choose either --weekly or --monthly, not both")
	}
	if trendMonthly {
		return analysis.TrendMonthly, nil
	}
	return analysis.TrendWeekly, nil
}
