package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/output"
)

var forecastCmd = &cobra.Command{
	Use:   "forecast",
	Short: "Predict month-end spend",
	RunE:  runForecast,
}

func init() {
	rootCmd.AddCommand(forecastCmd)
}

func runForecast(cmd *cobra.Command, args []string) error {
	records, err := readLocalRecords(cmd, time.Time{})
	if err != nil {
		return err
	}
	result, err := analysis.Forecast(records, time.Now().UTC())
	if err != nil {
		if errors.Is(err, analysis.ErrInsufficientData) {
			fmt.Fprintln(cmd.OutOrStdout(), "Not enough current-month usage data to forecast yet. Run `costroid sync` on at least two observed days.")
			return nil
		}
		return err
	}
	output.WriteForecast(cmd.OutOrStdout(), result)
	return nil
}
