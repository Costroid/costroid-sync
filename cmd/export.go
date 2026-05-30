package cmd

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/output"
	"github.com/costroid/costroid/providers"
)

var exportFormat string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export usage data (csv, json, focus, markdown)",
	RunE:  runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "csv", "export format: csv, json, focus, or markdown")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	records, err := readLocalRecords(cmd, time.Time{})
	if err != nil {
		return err
	}
	sortExportRecords(records)

	switch exportFormat {
	case "csv":
		return output.WriteCSV(cmd.OutOrStdout(), records)
	case "json":
		return output.WriteJSON(cmd.OutOrStdout(), records)
	case "focus":
		return output.WriteFOCUSCSV(cmd.OutOrStdout(), records)
	case "markdown":
		return output.WriteMarkdown(cmd.OutOrStdout(), records)
	default:
		return fmt.Errorf("--format must be one of csv, json, focus, or markdown")
	}
}

func sortExportRecords(records []providers.NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		return exportRecordKey(records[i]) < exportRecordKey(records[j])
	})
}

func exportRecordKey(r providers.NormalizedCostRecord) string {
	return r.RecordedAt + "\x00" + r.Provider + "\x00" + r.Model + "\x00" +
		r.ProjectID + "\x00" + r.APIKeyID + "\x00" + r.SourceHash
}
