package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

var csvHeaders = []string{
	"recorded_at", "provider", "model", "project_id", "api_key_id",
	"prompt_tokens", "completion_tokens", "total_tokens", "cost_usd", "source_hash",
}

func WriteCSV(w io.Writer, records []providers.NormalizedCostRecord) error {
	return writeRows(w, csvHeaders, records, costRecordCSVRow)
}

var focusHeaders = []string{
	"ChargePeriodStart", "ChargePeriodEnd", "ServiceProviderName", "ServiceName",
	"ServiceCategory", "ChargeCategory", "BilledCost", "BillingCurrency",
	"ConsumedQuantity", "ConsumedUnit", "ResourceId", "SubAccountId",
	"x_CostroidProvider", "x_CostroidModel", "x_PromptTokens",
	"x_CompletionTokens", "x_SourceHash",
}

func WriteFOCUSCSV(w io.Writer, records []providers.NormalizedCostRecord) error {
	return writeRows(w, focusHeaders, records, focusCSVRow)
}

func WriteMarkdown(w io.Writer, records []providers.NormalizedCostRecord) error {
	if _, err := fmt.Fprintln(w, "| Date | Provider | Model | Tokens | Cost |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- | --- | ---: | ---: |"); err != nil {
		return err
	}
	for _, r := range records {
		_, err := fmt.Fprintf(w, "| %s | %s | %s | %d | $%.4f |\n",
			markdownCell(formatRecordDate(r.RecordedAt)),
			markdownCell(r.Provider),
			markdownCell(r.Model),
			r.TotalTokens,
			r.CostUSD,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeRows(
	w io.Writer,
	headers []string,
	records []providers.NormalizedCostRecord,
	rowFn func(providers.NormalizedCostRecord) []string,
) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, r := range records {
		if err := cw.Write(rowFn(r)); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func costRecordCSVRow(r providers.NormalizedCostRecord) []string {
	return []string{
		r.RecordedAt,
		r.Provider,
		r.Model,
		r.ProjectID,
		r.APIKeyID,
		strconv.Itoa(r.PromptTokens),
		strconv.Itoa(r.CompletionTokens),
		strconv.Itoa(r.TotalTokens),
		formatFloat(r.CostUSD),
		r.SourceHash,
	}
}

func focusCSVRow(r providers.NormalizedCostRecord) []string {
	return []string{
		r.RecordedAt,
		chargePeriodEnd(r.RecordedAt),
		r.Provider,
		r.Model,
		"AI and Machine Learning",
		"Usage",
		formatFloat(r.CostUSD),
		"USD",
		strconv.Itoa(r.TotalTokens),
		"tokens",
		r.APIKeyID,
		r.ProjectID,
		r.Provider,
		r.Model,
		strconv.Itoa(r.PromptTokens),
		strconv.Itoa(r.CompletionTokens),
		r.SourceHash,
	}
}

func chargePeriodEnd(recordedAt string) string {
	t, err := time.Parse(time.RFC3339, recordedAt)
	if err != nil {
		return recordedAt
	}
	return t.UTC().Add(24 * time.Hour).Format(time.RFC3339)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func markdownCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
