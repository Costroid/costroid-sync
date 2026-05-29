package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

func TestOpenReadOnly_RejectsWritesAndReadsExistingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ro.db")

	// Seed via the normal read-write path.
	rw, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	rec := providers.NormalizedCostRecord{
		Provider: "openai", Model: "gpt-4o", RecordedAt: "2026-05-20T00:00:00Z",
		CostUSD: 1.50, TotalTokens: 10,
		SourceHash: providers.ComputeSourceHash("openai", "2026-05-20T00:00:00Z", "gpt-4o", "", ""),
	}
	if err := SaveRecords(context.Background(), rw, []providers.NormalizedCostRecord{rec}); err != nil {
		t.Fatalf("SaveRecords: %v", err)
	}
	rw.Close()

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()

	// Reads work.
	got, err := GetRecords(context.Background(), ro, time.Time{})
	if err != nil {
		t.Fatalf("GetRecords (ro): %v", err)
	}
	if len(got) != 1 || got[0].CostUSD != 1.50 {
		t.Fatalf("read-only data mismatch: %+v", got)
	}

	// Writes must fail on a read-only handle.
	if err := SaveRecords(context.Background(), ro, []providers.NormalizedCostRecord{rec}); err == nil {
		t.Fatalf("SaveRecords on read-only DB: want error, got nil")
	}
}

func TestOpenReadOnly_DoesNotCreateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")

	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// A query against a non-existent file fails (mode=ro never creates it)...
	if _, err := HasAnyRecords(context.Background(), db); err == nil {
		t.Fatalf("HasAnyRecords on missing DB: want error, got nil")
	}
	// ...and crucially the file was not created as a side effect.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created the DB file; stat err = %v, want not-exist", err)
	}
}

func TestHasAnyRecords(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	has, err := HasAnyRecords(ctx, db)
	if err != nil {
		t.Fatalf("HasAnyRecords (empty): %v", err)
	}
	if has {
		t.Fatalf("HasAnyRecords on empty table = true, want false")
	}

	rec := providers.NormalizedCostRecord{
		Provider: "openai", Model: "gpt-4o", RecordedAt: "2026-05-20T00:00:00Z", CostUSD: 1,
		SourceHash: providers.ComputeSourceHash("openai", "2026-05-20T00:00:00Z", "gpt-4o", "", ""),
	}
	if err := SaveRecords(ctx, db, []providers.NormalizedCostRecord{rec}); err != nil {
		t.Fatalf("SaveRecords: %v", err)
	}

	has, err = HasAnyRecords(ctx, db)
	if err != nil {
		t.Fatalf("HasAnyRecords (populated): %v", err)
	}
	if !has {
		t.Fatalf("HasAnyRecords on populated table = false, want true")
	}
}
