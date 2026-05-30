package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNoSyncState is returned by GetLastSync when no sync has been recorded yet.
var ErrNoSyncState = errors.New("no sync recorded")

// sync_state is a single-row table (id is pinned to 1) that records when
// `costroid sync` last completed. It exists so read-only consumers — the
// T1.1 statusline in particular — can show data freshness ("last sync 4h")
// without performing any provider/network call. It holds a timestamp only;
// never any prompt, completion, or other content.
const syncStateSchema = `
CREATE TABLE IF NOT EXISTS sync_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_sync_at TEXT NOT NULL
);`

// ensureSyncStateTable creates the sync_state table if it does not exist.
// Idempotent; safe to call on every startup from InitDB.
func ensureSyncStateTable(db *sql.DB) error {
	if _, err := db.Exec(syncStateSchema); err != nil {
		return fmt.Errorf("create sync_state: %w", err)
	}
	return nil
}

const upsertSyncStateSQL = `
INSERT INTO sync_state (id, last_sync_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET
    last_sync_at = excluded.last_sync_at
`

// SaveLastSync records t as the time of the most recent successful sync.
// The timestamp is stored as UTC RFC3339.
func SaveLastSync(ctx context.Context, db *sql.DB, t time.Time) error {
	if _, err := db.ExecContext(ctx, upsertSyncStateSQL,
		t.UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("save last sync: %w", err)
	}
	return nil
}

const selectSyncStateSQL = `
SELECT last_sync_at
  FROM sync_state
 WHERE id = 1
`

// GetLastSync returns the time of the most recent successful sync in UTC.
// It returns ErrNoSyncState when no sync has been recorded yet.
func GetLastSync(ctx context.Context, db *sql.DB) (time.Time, error) {
	var raw string
	err := db.QueryRowContext(ctx, selectSyncStateSQL).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNoSyncState
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get last sync: %w", err)
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse last sync %q: %w", raw, err)
	}
	return t.UTC(), nil
}
