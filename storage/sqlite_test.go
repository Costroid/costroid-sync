package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSaveRecords_UpsertsByHash(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	hash := providers.ComputeSourceHash("openai", "2026-05-21T00:00:00Z", "gpt-4o", "p", "k")
	r1 := providers.NormalizedCostRecord{
		Provider: "openai", Model: "gpt-4o", ProjectID: "p", APIKeyID: "k",
		PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CostUSD: 0.01,
		RecordedAt: "2026-05-21T00:00:00Z", SourceHash: hash,
	}
	r2 := r1
	r2.PromptTokens = 20
	r2.CompletionTokens = 10
	r2.TotalTokens = 30
	r2.CostUSD = 0.02

	if err := SaveRecords(ctx, db, []providers.NormalizedCostRecord{r1}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := SaveRecords(ctx, db, []providers.NormalizedCostRecord{r2}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	rows, err := GetRecords(ctx, db, time.Time{})
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row after upsert, got %d", len(rows))
	}
	got := rows[0]
	if got.PromptTokens != 20 || got.CompletionTokens != 10 || got.TotalTokens != 30 || got.CostUSD != 0.02 {
		t.Errorf("volatile fields not updated: %+v", got)
	}
	if got.SourceHash != hash || got.Model != "gpt-4o" || got.ProjectID != "p" || got.APIKeyID != "k" {
		t.Errorf("identity changed: %+v", got)
	}
}

func TestSaveRecords_BulkInsertDistinct(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	recs := []providers.NormalizedCostRecord{
		{
			Provider: "openai", Model: "gpt-4o", ProjectID: "p", APIKeyID: "k1",
			PromptTokens: 1, TotalTokens: 1, RecordedAt: "2026-05-20T00:00:00Z",
			SourceHash: providers.ComputeSourceHash("openai", "2026-05-20T00:00:00Z", "gpt-4o", "p", "k1"),
		},
		{
			Provider: "openai", Model: "gpt-4o", ProjectID: "p", APIKeyID: "k2",
			PromptTokens: 2, TotalTokens: 2, RecordedAt: "2026-05-20T00:00:00Z",
			SourceHash: providers.ComputeSourceHash("openai", "2026-05-20T00:00:00Z", "gpt-4o", "p", "k2"),
		},
	}
	if err := SaveRecords(ctx, db, recs); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := GetRecords(ctx, db, time.Time{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
}

func TestGetRecords_FilterSince(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	old := providers.NormalizedCostRecord{
		Provider: "openai", Model: "m", RecordedAt: "2026-05-01T00:00:00Z",
		SourceHash: providers.ComputeSourceHash("openai", "2026-05-01T00:00:00Z", "m", "", ""),
	}
	recent := providers.NormalizedCostRecord{
		Provider: "openai", Model: "m", RecordedAt: "2026-05-21T00:00:00Z",
		SourceHash: providers.ComputeSourceHash("openai", "2026-05-21T00:00:00Z", "m", "", ""),
	}
	if err := SaveRecords(ctx, db, []providers.NormalizedCostRecord{old, recent}); err != nil {
		t.Fatalf("save: %v", err)
	}

	since := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	got, err := GetRecords(ctx, db, since)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 || got[0].RecordedAt != "2026-05-21T00:00:00Z" {
		t.Errorf("want only the recent row, got %+v", got)
	}
}

func TestSchema_NoForbiddenColumns(t *testing.T) {
	db := newTestDB(t)
	rows, err := db.Query("PRAGMA table_info(cost_records)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{
		"prompt": true, "completion": true, "messages": true, "content": true,
		"request_body": true, "response_body": true, "raw_payload": true,
		"raw_response": true, "system_prompt": true, "function_args": true,
		"tool_calls": true,
	}
	var found []string
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		found = append(found, name)
		if forbidden[name] {
			t.Errorf("forbidden column %q present in schema", name)
		}
	}
	if len(found) == 0 {
		t.Fatal("PRAGMA returned no columns")
	}
}

func TestResolveDBPath_EnvOverride(t *testing.T) {
	t.Setenv("COSTROID_DB", "/tmp/xyz.db")
	p, err := ResolveDBPath()
	if err != nil {
		t.Fatalf("ResolveDBPath: %v", err)
	}
	if p != "/tmp/xyz.db" {
		t.Errorf("want override, got %q", p)
	}

	t.Setenv("COSTROID_DB", "")
	p2, err := ResolveDBPath()
	if err != nil {
		t.Fatalf("ResolveDBPath default: %v", err)
	}
	if p2 == "" {
		t.Error("default path empty")
	}
}
