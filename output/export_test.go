package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/costroid/costroid/providers"
)

func TestWriteCSV_MetadataHeadersAndValues(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []providers.NormalizedCostRecord{exportTestRecord()}); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, buf.String())
	wantHeader := []string{
		"recorded_at", "provider", "model", "project_id", "api_key_id",
		"prompt_tokens", "completion_tokens", "total_tokens", "cost_usd", "source_hash",
		"product", "sku", "unit_type",
		"usage_quantity", "unit_price_usd", "gross_amount_usd", "discount_amount_usd",
	}
	assertCSVRow(t, rows[0], wantHeader)
	assertCSVRow(t, rows[1], []string{
		"2026-05-22T00:00:00Z", "openai", "gpt-4o", "project-1", "key-1",
		"10", "5", "15", "0.1234", "hash-1",
		"", "", "", "0", "0", "0", "0",
	})
}

func TestWriteJSON_MetadataOnlyKeys(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []providers.NormalizedCostRecord{exportTestRecord()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("json should be valid: %v", err)
	}
	allowed := allowedJSONKeys()
	for key := range rows[0] {
		if !allowed[key] {
			t.Fatalf("unexpected json key %q", key)
		}
	}
	assertForbiddenJSONNamesAbsent(t, buf.String())
}

func TestWriteFOCUSCSV_RequiredColumns(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFOCUSCSV(&buf, []providers.NormalizedCostRecord{exportTestRecord()}); err != nil {
		t.Fatalf("WriteFOCUSCSV: %v", err)
	}
	rows := readCSV(t, buf.String())
	assertCSVRow(t, rows[0], focusHeaders)
	if rows[1][0] != "2026-05-22T00:00:00Z" || rows[1][1] != "2026-05-23T00:00:00Z" {
		t.Fatalf("unexpected charge period: %+v", rows[1][:2])
	}
	if rows[1][8] != "15" || rows[1][9] != "tokens" {
		t.Fatalf("ConsumedQuantity mapping wrong: %+v", rows[1][8:10])
	}
}

func TestWriteMarkdown_MetadataOnlyTable(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, []providers.NormalizedCostRecord{exportTestRecord()}); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "| Date | Provider | Model | Tokens | Cost |") {
		t.Fatalf("markdown header missing: %q", got)
	}
	if !strings.Contains(got, "| 2026-05-22 | openai | gpt-4o | 15 | $0.1234 |") {
		t.Fatalf("markdown row missing: %q", got)
	}
	assertForbiddenJSONNamesAbsent(t, got)
}

func TestExportWriters_EmptyOutputShapes(t *testing.T) {
	tests := map[string]func(*bytes.Buffer) error{
		"csv":      func(b *bytes.Buffer) error { return WriteCSV(b, nil) },
		"focus":    func(b *bytes.Buffer) error { return WriteFOCUSCSV(b, nil) },
		"markdown": func(b *bytes.Buffer) error { return WriteMarkdown(b, nil) },
		"json":     func(b *bytes.Buffer) error { return WriteJSON(b, nil) },
	}
	for name, write := range tests {
		var buf bytes.Buffer
		if err := write(&buf); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.TrimSpace(buf.String()) == "" {
			t.Fatalf("%s output should not be empty", name)
		}
		if name == "json" && strings.TrimSpace(buf.String()) != "[]" {
			t.Fatalf("empty json = %q, want []", buf.String())
		}
	}
}

func exportTestRecord() providers.NormalizedCostRecord {
	return providers.NormalizedCostRecord{
		Provider:         "openai",
		Model:            "gpt-4o",
		ProjectID:        "project-1",
		APIKeyID:         "key-1",
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		CostUSD:          0.1234,
		RecordedAt:       "2026-05-22T00:00:00Z",
		SourceHash:       "hash-1",
	}
}

func exportTestCopilotRecord() providers.NormalizedCostRecord {
	return providers.NormalizedCostRecord{
		Provider:          "github-copilot",
		Model:             "gpt-4-copilot",
		ProjectID:         "my-org",
		APIKeyID:          "",
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       0,
		CostUSD:           2.50,
		RecordedAt:        "2026-05-22T00:00:00Z",
		SourceHash:        "hash-copilot",
		Product:           "copilot",
		SKU:               "copilot_premium_request_user",
		UnitType:          "premium_requests",
		UsageQuantity:     250,
		UnitPriceUSD:      0.01,
		GrossAmountUSD:    2.50,
		DiscountAmountUSD: 0,
	}
}

func TestWriteCSV_IncludesBillingMetadata(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []providers.NormalizedCostRecord{exportTestCopilotRecord()}); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	rows := readCSV(t, buf.String())
	if len(rows) != 2 {
		t.Fatalf("want header + 1 data row, got %d rows", len(rows))
	}
	row := rows[1]
	want := map[int]string{
		1:  "github-copilot",
		2:  "gpt-4-copilot",
		8:  "2.5",                          // cost_usd
		10: "copilot",                      // product
		11: "copilot_premium_request_user", // sku
		12: "premium_requests",             // unit_type
		13: "250",                          // usage_quantity
	}
	for idx, expected := range want {
		if row[idx] != expected {
			t.Errorf("row[%d] = %q, want %q (row=%+v)", idx, row[idx], expected, row)
		}
	}
}

func TestWriteFOCUSCSV_QuantityFallback(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFOCUSCSV(&buf, []providers.NormalizedCostRecord{exportTestCopilotRecord()}); err != nil {
		t.Fatalf("WriteFOCUSCSV: %v", err)
	}
	rows := readCSV(t, buf.String())
	// ConsumedQuantity (index 8) should be UsageQuantity; ConsumedUnit (index 9) UnitType.
	if got := rows[1][8]; got != "250" {
		t.Errorf("ConsumedQuantity = %q, want 250", got)
	}
	if got := rows[1][9]; got != "premium_requests" {
		t.Errorf("ConsumedUnit = %q, want premium_requests", got)
	}
	// Token-bearing row still uses tokens.
	buf.Reset()
	if err := WriteFOCUSCSV(&buf, []providers.NormalizedCostRecord{exportTestRecord()}); err != nil {
		t.Fatalf("WriteFOCUSCSV (tokens): %v", err)
	}
	rows = readCSV(t, buf.String())
	if got := rows[1][8]; got != "15" {
		t.Errorf("token row ConsumedQuantity = %q, want 15", got)
	}
	if got := rows[1][9]; got != "tokens" {
		t.Errorf("token row ConsumedUnit = %q, want tokens", got)
	}
}

func TestWriteJSON_BillingKeysPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []providers.NormalizedCostRecord{exportTestCopilotRecord()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("json should be valid: %v", err)
	}
	for _, want := range []string{
		"product", "sku", "unit_type",
		"usage_quantity", "unit_price_usd", "gross_amount_usd", "discount_amount_usd",
	} {
		if _, ok := rows[0][want]; !ok {
			t.Errorf("billing key %q missing from JSON output: %v", want, rows[0])
		}
	}
	assertForbiddenJSONNamesAbsent(t, buf.String())
}

func readCSV(t *testing.T, value string) [][]string {
	t.Helper()
	rows, err := csv.NewReader(strings.NewReader(value)).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	return rows
}

func assertCSVRow(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("row len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row[%d] = %q, want %q; row=%+v", i, got[i], want[i], got)
		}
	}
}

func allowedJSONKeys() map[string]bool {
	return map[string]bool{
		"provider": true, "model": true, "prompt_tokens": true,
		"completion_tokens": true, "total_tokens": true, "cost_usd": true,
		"recorded_at": true, "api_key_id": true, "project_id": true,
		"source_hash": true,
		"product":     true, "sku": true, "unit_type": true,
		"usage_quantity": true, "unit_price_usd": true,
		"gross_amount_usd": true, "discount_amount_usd": true,
	}
}

func assertForbiddenJSONNamesAbsent(t *testing.T, value string) {
	t.Helper()
	forbidden := []string{
		`"prompt"`, `"completion"`, `"messages"`, `"content"`, `"request_body"`,
		`"response_body"`, `"raw_payload"`, `"raw_response"`, `"system_prompt"`,
		`"function_args"`, `"tool_calls"`,
	}
	for _, field := range forbidden {
		if strings.Contains(value, field) {
			t.Fatalf("forbidden field name %s present in output: %s", field, value)
		}
	}
}
