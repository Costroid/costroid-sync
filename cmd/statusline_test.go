package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/providers"
	"github.com/costroid/costroid/storage"
)

// runStatuslineCapture invokes the statusline command in-process and returns
// its stdout. It avoids the shared rootCmd so package-level flag state is
// explicit per call.
func runStatuslineCapture(t *testing.T, format string, plain bool) (string, error) {
	t.Helper()
	statuslineFormat = format
	statuslinePlain = plain
	c := &cobra.Command{}
	var buf bytes.Buffer
	c.SetOut(&buf)
	err := runStatusline(c, nil)
	return buf.String(), err
}

// newEmptyDBAt creates a fresh read-write DB at path (schema only, no rows)
// and closes it, leaving an on-disk empty database for the statusline to read.
func newEmptyDBAt(t *testing.T, path string) {
	t.Helper()
	db, err := storage.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Close()
}

func TestStatuslineCmd_MissingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	t.Setenv("COSTROID_DB", path)

	out, err := runStatuslineCapture(t, "plain", false)
	if err != nil {
		t.Fatalf("runStatusline: %v", err)
	}
	if out != "costroid  no local data  run costroid sync\n" {
		t.Errorf("missing DB line: %q", out)
	}
	// The statusline must never create the database.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("statusline created the DB file; stat err = %v", statErr)
	}

	jsonOut, err := runStatuslineCapture(t, "json", false)
	if err != nil {
		t.Fatalf("runStatusline json: %v", err)
	}
	if status := jsonStatus(t, jsonOut); status != "missing_db" {
		t.Errorf("json status = %q, want missing_db", status)
	}
}

func TestStatuslineCmd_EmptyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	newEmptyDBAt(t, path)
	t.Setenv("COSTROID_DB", path)

	out, err := runStatuslineCapture(t, "plain", false)
	if err != nil {
		t.Fatalf("runStatusline: %v", err)
	}
	if out != "costroid  no local data  run costroid sync\n" {
		t.Errorf("empty DB line: %q", out)
	}

	jsonOut, _ := runStatuslineCapture(t, "json", false)
	if status := jsonStatus(t, jsonOut); status != "empty" {
		t.Errorf("json status = %q, want empty", status)
	}
}

func TestStatuslineCmd_PopulatedOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := storage.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339)
	rec := providers.NormalizedCostRecord{
		Provider: "openai", Model: "gpt-4o", RecordedAt: ts, CostUSD: 12.34, TotalTokens: 5,
		SourceHash: providers.ComputeSourceHash("openai", ts, "gpt-4o", "", ""),
	}
	if err := storage.SaveRecords(context.Background(), db, []providers.NormalizedCostRecord{rec}); err != nil {
		t.Fatalf("SaveRecords: %v", err)
	}
	if err := storage.SaveLastSync(context.Background(), db, now); err != nil {
		t.Fatalf("SaveLastSync: %v", err)
	}
	db.Close()
	t.Setenv("COSTROID_DB", path)

	out, err := runStatuslineCapture(t, "plain", false)
	if err != nil {
		t.Fatalf("runStatusline: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("costroid")) || !bytes.Contains([]byte(out), []byte("MTD $")) {
		t.Errorf("ok line missing expected tokens: %q", out)
	}
	if bytes.Contains([]byte(out), []byte("last sync never")) {
		t.Errorf("expected a real sync age, got never: %q", out)
	}
	if n := bytes.Count([]byte(out), []byte("\n")); n != 1 {
		t.Errorf("ok line is not exactly one physical line: %q", out)
	}

	jsonOut, _ := runStatuslineCapture(t, "json", false)
	if status := jsonStatus(t, jsonOut); status != "ok" {
		t.Errorf("json status = %q, want ok", status)
	}
}

func TestStatuslineCmd_NeverSynced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nosync.db")
	db, err := storage.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339)
	rec := providers.NormalizedCostRecord{
		Provider: "openai", Model: "gpt-4o", RecordedAt: ts, CostUSD: 1, TotalTokens: 1,
		SourceHash: providers.ComputeSourceHash("openai", ts, "gpt-4o", "", ""),
	}
	if err := storage.SaveRecords(context.Background(), db, []providers.NormalizedCostRecord{rec}); err != nil {
		t.Fatalf("SaveRecords: %v", err)
	}
	db.Close() // note: no SaveLastSync
	t.Setenv("COSTROID_DB", path)

	out, _ := runStatuslineCapture(t, "plain", false)
	if !bytes.Contains([]byte(out), []byte("last sync never")) {
		t.Errorf("expected 'last sync never': %q", out)
	}
}

func TestStatuslineCmd_InvalidFormat(t *testing.T) {
	if _, err := runStatuslineCapture(t, "xml", false); err == nil {
		t.Errorf("invalid --format: want error, got nil")
	}
}

func jsonStatus(t *testing.T, out string) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, out)
	}
	s, _ := m["status"].(string)
	return s
}
