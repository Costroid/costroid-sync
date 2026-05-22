package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/costroid/costroid-sync/providers"
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
	}
	assertCSVRow(t, rows[0], wantHeader)
	assertCSVRow(t, rows[1], []string{
		"2026-05-22T00:00:00Z", "openai", "gpt-4o", "project-1", "key-1",
		"10", "5", "15", "0.1234", "hash-1",
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
