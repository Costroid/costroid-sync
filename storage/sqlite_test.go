package storage

import (
	"context"
	"database/sql"
	"errors"
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
	assertNoForbiddenColumns(t, db, "cost_records")
	assertNoForbiddenColumns(t, db, "local_budgets")
}

func assertNoForbiddenColumns(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
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
		t.Fatalf("PRAGMA returned no columns for %s", table)
	}
}

func TestSaveBudget_GetBudget(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	first := BudgetRecord{AmountUSD: 500, Period: "monthly", UpdatedAt: "2026-05-22T00:00:00Z"}
	if err := SaveBudget(ctx, db, first); err != nil {
		t.Fatalf("SaveBudget first: %v", err)
	}
	got, err := GetBudget(ctx, db)
	if err != nil {
		t.Fatalf("GetBudget first: %v", err)
	}
	if got != first {
		t.Fatalf("first budget = %+v, want %+v", got, first)
	}

	second := BudgetRecord{AmountUSD: 100, Period: "weekly", UpdatedAt: "2026-05-23T00:00:00Z"}
	if err := SaveBudget(ctx, db, second); err != nil {
		t.Fatalf("SaveBudget second: %v", err)
	}
	got, err = GetBudget(ctx, db)
	if err != nil {
		t.Fatalf("GetBudget second: %v", err)
	}
	if got != second {
		t.Fatalf("second budget = %+v, want %+v", got, second)
	}
}

func TestGetBudget_Missing(t *testing.T) {
	db := newTestDB(t)
	_, err := GetBudget(context.Background(), db)
	if !errors.Is(err, ErrBudgetNotFound) {
		t.Fatalf("want ErrBudgetNotFound, got %v", err)
	}
}

func TestSaveGetRecords_BillingMetadataRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	rec := providers.NormalizedCostRecord{
		Provider:          "github-copilot",
		Model:             "gpt-4-copilot",
		ProjectID:         "my-org",
		APIKeyID:          "",
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       0,
		CostUSD:           2.50,
		RecordedAt:        "2026-05-22T00:00:00Z",
		SourceHash:        providers.ComputeSourceHash("github-copilot", "2026-05-22T00:00:00Z", "gpt-4-copilot", "my-org", ""),
		Product:           "copilot",
		SKU:               "copilot_premium_request_user",
		UnitType:          "premium_requests",
		UsageQuantity:     250,
		UnitPriceUSD:      0.01,
		GrossAmountUSD:    3.00,
		DiscountAmountUSD: 0.50,
	}
	if err := SaveRecords(ctx, db, []providers.NormalizedCostRecord{rec}); err != nil {
		t.Fatalf("SaveRecords: %v", err)
	}
	got, err := GetRecords(ctx, db, time.Time{})
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	if got[0] != rec {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", got[0], rec)
	}
}

func TestEnsureCostRecordColumns_Migration(t *testing.T) {
	// Create a DB containing only the pre-C9.1 cost_records schema (the 10
	// original columns). Then call InitDB on the same path — the migration
	// should add the 7 new columns without losing existing data.
	path := filepath.Join(t.TempDir(), "migrate.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const legacy = `CREATE TABLE cost_records (
		source_hash       TEXT PRIMARY KEY,
		provider          TEXT NOT NULL,
		model             TEXT NOT NULL,
		project_id        TEXT NOT NULL DEFAULT '',
		api_key_id        TEXT NOT NULL DEFAULT '',
		prompt_tokens     INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens      INTEGER NOT NULL DEFAULT 0,
		cost_usd          REAL NOT NULL DEFAULT 0,
		recorded_at       TEXT NOT NULL
	)`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	// Insert a row using the legacy schema (10 columns).
	const legacyInsert = `INSERT INTO cost_records
		(source_hash, provider, model, project_id, api_key_id, prompt_tokens, completion_tokens, total_tokens, cost_usd, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(legacyInsert,
		"legacy-hash", "openai", "gpt-4o", "p", "k",
		10, 5, 15, 0.50, "2026-05-01T00:00:00Z"); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	db.Close()

	// Reopen via InitDB — runs ensureCostRecordColumns.
	db, err = InitDB(path)
	if err != nil {
		t.Fatalf("InitDB (with migration): %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Verify the legacy row survives.
	got, err := GetRecords(context.Background(), db, time.Time{})
	if err != nil {
		t.Fatalf("GetRecords after migration: %v", err)
	}
	if len(got) != 1 || got[0].SourceHash != "legacy-hash" {
		t.Fatalf("legacy row lost: %+v", got)
	}
	// Verify the new columns exist with zero defaults.
	if got[0].Product != "" || got[0].UsageQuantity != 0 || got[0].UnitPriceUSD != 0 {
		t.Errorf("expected zero defaults on migrated row: %+v", got[0])
	}

	// Verify we can now write a record that uses the new columns.
	billing := providers.NormalizedCostRecord{
		Provider: "github-copilot", Model: "gpt-4-copilot", ProjectID: "my-org",
		RecordedAt: "2026-05-02T00:00:00Z", CostUSD: 1.00,
		SourceHash: providers.ComputeSourceHash("github-copilot", "2026-05-02T00:00:00Z", "gpt-4-copilot", "my-org", ""),
		Product:    "copilot", SKU: "sku1", UnitType: "premium_requests",
		UsageQuantity: 100, UnitPriceUSD: 0.01,
	}
	if err := SaveRecords(context.Background(), db, []providers.NormalizedCostRecord{billing}); err != nil {
		t.Fatalf("save after migration: %v", err)
	}

	// Calling InitDB again should be a no-op (idempotent).
	db.Close()
	db, err = InitDB(path)
	if err != nil {
		t.Fatalf("InitDB second time: %v", err)
	}
	t.Cleanup(func() { db.Close() })
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
