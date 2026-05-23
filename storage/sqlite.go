package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver

	"github.com/costroid/costroid-sync/providers"
)

const (
	DefaultDirName    = ".costroid"
	DefaultDBFilename = "costroid.db"
	envDBPath         = "COSTROID_DB"
)

var ErrBudgetNotFound = errors.New("budget not found")

type BudgetRecord struct {
	AmountUSD float64
	Period    string
	UpdatedAt string
}

// Open returns a handle to the SQLite database at path.
// Low-level primitive — InitDB is what callers normally want.
func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

// DefaultDBPath returns ~/.costroid/costroid.db.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, DefaultDirName, DefaultDBFilename), nil
}

// ResolveDBPath returns $COSTROID_DB if set (non-empty), else DefaultDBPath().
func ResolveDBPath() (string, error) {
	if p := os.Getenv(envDBPath); p != "" {
		return p, nil
	}
	return DefaultDBPath()
}

// InitDB ensures the parent directory exists (0700), opens the SQLite DB,
// applies the schema, and migrates older schemas in place (adds any new
// columns missing from pre-C9.1 databases). Safe to call on every startup.
func InitDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := Open(path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := ensureCostRecordColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate cost_records: %w", err)
	}
	return db, nil
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS cost_records (
    source_hash         TEXT PRIMARY KEY,
    provider            TEXT NOT NULL,
    model               TEXT NOT NULL,
    project_id          TEXT NOT NULL DEFAULT '',
    api_key_id          TEXT NOT NULL DEFAULT '',
    prompt_tokens       INTEGER NOT NULL DEFAULT 0,
    completion_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    cost_usd            REAL NOT NULL DEFAULT 0,
    recorded_at         TEXT NOT NULL,
    product             TEXT NOT NULL DEFAULT '',
    sku                 TEXT NOT NULL DEFAULT '',
    unit_type           TEXT NOT NULL DEFAULT '',
    usage_quantity      REAL NOT NULL DEFAULT 0,
    unit_price_usd      REAL NOT NULL DEFAULT 0,
    gross_amount_usd    REAL NOT NULL DEFAULT 0,
    discount_amount_usd REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cost_records_recorded_at ON cost_records(recorded_at);

CREATE TABLE IF NOT EXISTS local_budgets (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    amount_usd REAL NOT NULL,
    period TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

// ensureCostRecordColumns adds C9.1 billing-metadata columns to pre-existing
// cost_records tables that were created before this version. SQLite doesn't
// support `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, so we introspect via
// PRAGMA table_info first. Idempotent.
func ensureCostRecordColumns(db *sql.DB) error {
	existing, err := costRecordColumnSet(db)
	if err != nil {
		return err
	}
	migrations := []struct{ Name, DDL string }{
		{"product", "ALTER TABLE cost_records ADD COLUMN product TEXT NOT NULL DEFAULT ''"},
		{"sku", "ALTER TABLE cost_records ADD COLUMN sku TEXT NOT NULL DEFAULT ''"},
		{"unit_type", "ALTER TABLE cost_records ADD COLUMN unit_type TEXT NOT NULL DEFAULT ''"},
		{"usage_quantity", "ALTER TABLE cost_records ADD COLUMN usage_quantity REAL NOT NULL DEFAULT 0"},
		{"unit_price_usd", "ALTER TABLE cost_records ADD COLUMN unit_price_usd REAL NOT NULL DEFAULT 0"},
		{"gross_amount_usd", "ALTER TABLE cost_records ADD COLUMN gross_amount_usd REAL NOT NULL DEFAULT 0"},
		{"discount_amount_usd", "ALTER TABLE cost_records ADD COLUMN discount_amount_usd REAL NOT NULL DEFAULT 0"},
	}
	for _, m := range migrations {
		if existing[m.Name] {
			continue
		}
		if _, err := db.Exec(m.DDL); err != nil {
			return fmt.Errorf("add column %s: %w", m.Name, err)
		}
	}
	return nil
}

func costRecordColumnSet(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(cost_records)")
	if err != nil {
		return nil, fmt.Errorf("pragma table_info: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
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
			return nil, fmt.Errorf("scan pragma: %w", err)
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

const upsertSQL = `
INSERT INTO cost_records (
    source_hash, provider, model, project_id, api_key_id,
    prompt_tokens, completion_tokens, total_tokens, cost_usd, recorded_at,
    product, sku, unit_type, usage_quantity, unit_price_usd, gross_amount_usd, discount_amount_usd
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_hash) DO UPDATE SET
    prompt_tokens       = excluded.prompt_tokens,
    completion_tokens   = excluded.completion_tokens,
    total_tokens        = excluded.total_tokens,
    cost_usd            = excluded.cost_usd,
    usage_quantity      = excluded.usage_quantity,
    unit_price_usd      = excluded.unit_price_usd,
    gross_amount_usd    = excluded.gross_amount_usd,
    discount_amount_usd = excluded.discount_amount_usd
`

// SaveRecords UPSERTs each record by source_hash inside a single transaction.
// On conflict, only the volatile columns (token counts, cost, quantities)
// are updated; identity columns (provider, model, project_id, api_key_id,
// recorded_at, product, sku, unit_type) are never overwritten.
func SaveRecords(ctx context.Context, db *sql.DB, records []providers.NormalizedCostRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	for i, r := range records {
		if _, err := stmt.ExecContext(ctx,
			r.SourceHash, r.Provider, r.Model, r.ProjectID, r.APIKeyID,
			r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.CostUSD, r.RecordedAt,
			r.Product, r.SKU, r.UnitType, r.UsageQuantity, r.UnitPriceUSD, r.GrossAmountUSD, r.DiscountAmountUSD,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("save record %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

const selectAllSQL = `
SELECT source_hash, provider, model, project_id, api_key_id,
       prompt_tokens, completion_tokens, total_tokens, cost_usd, recorded_at,
       product, sku, unit_type, usage_quantity, unit_price_usd, gross_amount_usd, discount_amount_usd
  FROM cost_records
 ORDER BY recorded_at DESC
`

const selectSinceSQL = `
SELECT source_hash, provider, model, project_id, api_key_id,
       prompt_tokens, completion_tokens, total_tokens, cost_usd, recorded_at,
       product, sku, unit_type, usage_quantity, unit_price_usd, gross_amount_usd, discount_amount_usd
  FROM cost_records
 WHERE recorded_at >= ?
 ORDER BY recorded_at DESC
`

// GetRecords returns rows ordered by recorded_at DESC. A zero `since`
// returns every row.
func GetRecords(ctx context.Context, db *sql.DB, since time.Time) ([]providers.NormalizedCostRecord, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if since.IsZero() {
		rows, err = db.QueryContext(ctx, selectAllSQL)
	} else {
		rows, err = db.QueryContext(ctx, selectSinceSQL, since.UTC().Format(time.RFC3339))
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var out []providers.NormalizedCostRecord
	for rows.Next() {
		var r providers.NormalizedCostRecord
		if err := rows.Scan(
			&r.SourceHash, &r.Provider, &r.Model, &r.ProjectID, &r.APIKeyID,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.CostUSD, &r.RecordedAt,
			&r.Product, &r.SKU, &r.UnitType, &r.UsageQuantity, &r.UnitPriceUSD, &r.GrossAmountUSD, &r.DiscountAmountUSD,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

const upsertBudgetSQL = `
INSERT INTO local_budgets (id, amount_usd, period, updated_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    amount_usd = excluded.amount_usd,
    period = excluded.period,
    updated_at = excluded.updated_at
`

func SaveBudget(ctx context.Context, db *sql.DB, budget BudgetRecord) error {
	if _, err := db.ExecContext(ctx, upsertBudgetSQL,
		budget.AmountUSD, budget.Period, budget.UpdatedAt,
	); err != nil {
		return fmt.Errorf("save budget: %w", err)
	}
	return nil
}

const selectBudgetSQL = `
SELECT amount_usd, period, updated_at
  FROM local_budgets
 WHERE id = 1
`

func GetBudget(ctx context.Context, db *sql.DB) (BudgetRecord, error) {
	var budget BudgetRecord
	err := db.QueryRowContext(ctx, selectBudgetSQL).Scan(
		&budget.AmountUSD, &budget.Period, &budget.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return BudgetRecord{}, ErrBudgetNotFound
	}
	if err != nil {
		return BudgetRecord{}, fmt.Errorf("get budget: %w", err)
	}
	return budget, nil
}
