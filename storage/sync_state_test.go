package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetLastSync_FreshDBReturnsErrNoSyncState(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	_, err := GetLastSync(ctx, db)
	if !errors.Is(err, ErrNoSyncState) {
		t.Fatalf("GetLastSync on fresh DB: got %v, want ErrNoSyncState", err)
	}
}

func TestSaveLastSync_RoundTripUTCSeconds(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// A non-UTC instant with sub-second precision; RFC3339 storage keeps
	// second precision and the value comes back normalized to UTC.
	loc := time.FixedZone("UTC+2", 2*3600)
	want := time.Date(2026, 5, 28, 16, 12, 30, 500_000_000, loc)

	if err := SaveLastSync(ctx, db, want); err != nil {
		t.Fatalf("SaveLastSync: %v", err)
	}

	got, err := GetLastSync(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSync: %v", err)
	}
	if !got.Equal(want.UTC().Truncate(time.Second)) {
		t.Fatalf("GetLastSync = %s, want %s", got, want.UTC().Truncate(time.Second))
	}
	if got.Location() != time.UTC {
		t.Fatalf("GetLastSync location = %s, want UTC", got.Location())
	}
}

func TestSaveLastSync_OverwritesPreviousAndStaysSingleRow(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	first := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 29, 9, 30, 0, 0, time.UTC)

	if err := SaveLastSync(ctx, db, first); err != nil {
		t.Fatalf("SaveLastSync first: %v", err)
	}
	if err := SaveLastSync(ctx, db, second); err != nil {
		t.Fatalf("SaveLastSync second: %v", err)
	}

	got, err := GetLastSync(ctx, db)
	if err != nil {
		t.Fatalf("GetLastSync: %v", err)
	}
	if !got.Equal(second) {
		t.Fatalf("GetLastSync = %s, want latest %s", got, second)
	}

	var rows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_state").Scan(&rows); err != nil {
		t.Fatalf("count sync_state: %v", err)
	}
	if rows != 1 {
		t.Fatalf("sync_state row count = %d, want 1 (single-row table)", rows)
	}
}

func TestEnsureSyncStateTable_Idempotent(t *testing.T) {
	db := newTestDB(t)

	// InitDB already created it; calling again must not error.
	if err := ensureSyncStateTable(db); err != nil {
		t.Fatalf("ensureSyncStateTable (re-run): %v", err)
	}
}
