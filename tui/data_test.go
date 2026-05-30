package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/costroid/costroid-sync/providers"
	"github.com/costroid/costroid-sync/storage"
)

func rec(provider, model string, cost float64, tokens int, ts time.Time) providers.NormalizedCostRecord {
	at := ts.UTC().Format(time.RFC3339)
	return providers.NormalizedCostRecord{
		Provider: provider, Model: model, CostUSD: cost, TotalTokens: tokens,
		RecordedAt: at,
		SourceHash: providers.ComputeSourceHash(provider, at, model, "", ""),
	}
}

func seedDB(t *testing.T, recs []providers.NormalizedCostRecord, lastSync *time.Time, budget *storage.BudgetRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := storage.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if len(recs) > 0 {
		if err := storage.SaveRecords(ctx, db, recs); err != nil {
			t.Fatalf("SaveRecords: %v", err)
		}
	}
	if lastSync != nil {
		if err := storage.SaveLastSync(ctx, db, *lastSync); err != nil {
			t.Fatalf("SaveLastSync: %v", err)
		}
	}
	if budget != nil {
		if err := storage.SaveBudget(ctx, db, *budget); err != nil {
			t.Fatalf("SaveBudget: %v", err)
		}
	}
	return path
}

func TestLoadDashboard_MissingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	t.Setenv("COSTROID_DB", path)

	d := LoadDashboard(context.Background(), time.Now().UTC())
	if d.Status != DataMissingDB {
		t.Errorf("status = %v, want DataMissingDB", d.Status)
	}
}

func TestLoadDashboard_EmptyDB(t *testing.T) {
	path := seedDB(t, nil, nil, nil)
	t.Setenv("COSTROID_DB", path)

	d := LoadDashboard(context.Background(), time.Now().UTC())
	if d.Status != DataEmpty {
		t.Errorf("status = %v, want DataEmpty", d.Status)
	}
}

func TestLoadDashboard_Populated(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	recs := []providers.NormalizedCostRecord{
		rec("openai", "gpt-4o", 30.00, 1000, now.AddDate(0, 0, -2)),
		rec("openai", "gpt-4o-mini", 2.00, 800, now.AddDate(0, 0, -1)),
		rec("anthropic", "claude-4", 8.00, 500, now),
	}
	last := now.Add(-4 * time.Hour)
	budget := &storage.BudgetRecord{AmountUSD: 1000, Period: "monthly", UpdatedAt: now.Format(time.RFC3339)}
	path := seedDB(t, recs, &last, budget)
	t.Setenv("COSTROID_DB", path)

	d := LoadDashboard(context.Background(), now)
	if d.Status != DataOK {
		t.Fatalf("status = %v, want DataOK", d.Status)
	}
	if len(d.Providers) != 2 {
		t.Errorf("providers = %d, want 2", len(d.Providers))
	}
	if len(d.Models) != 3 {
		t.Errorf("models = %d, want 3", len(d.Models))
	}
	if d.Providers[0].Provider != "openai" { // highest spend first
		t.Errorf("top provider = %q, want openai", d.Providers[0].Provider)
	}
	if d.Budget == nil || d.Budget.IsOverBudget {
		t.Errorf("budget = %+v, want set and on-track", d.Budget)
	}
	if d.LastSync == nil {
		t.Error("LastSync = nil, want set")
	}
	if len(d.Syncs) != 2 {
		t.Errorf("syncs = %d, want 2 providers", len(d.Syncs))
	}
}

func TestLoadDashboard_NeverSynced(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	path := seedDB(t, []providers.NormalizedCostRecord{rec("openai", "gpt-4o", 1, 1, now)}, nil, nil)
	t.Setenv("COSTROID_DB", path)

	d := LoadDashboard(context.Background(), now)
	if d.Status != DataOK {
		t.Fatalf("status = %v, want DataOK", d.Status)
	}
	if d.LastSync != nil {
		t.Errorf("LastSync = %v, want nil (never synced)", d.LastSync)
	}
}

func TestLoadDashboard_OverBudget(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	recs := []providers.NormalizedCostRecord{
		rec("openai", "gpt-4o", 90.00, 1000, now.AddDate(0, 0, -1)),
		rec("openai", "gpt-4o", 60.00, 1000, now),
	}
	budget := &storage.BudgetRecord{AmountUSD: 100, Period: "monthly", UpdatedAt: now.Format(time.RFC3339)}
	path := seedDB(t, recs, nil, budget)
	t.Setenv("COSTROID_DB", path)

	d := LoadDashboard(context.Background(), now)
	if d.Budget == nil || !d.Budget.IsOverBudget {
		t.Errorf("budget = %+v, want over budget", d.Budget)
	}
	if !d.Overview.OverBudget {
		t.Error("overview OverBudget = false, want true")
	}
}
