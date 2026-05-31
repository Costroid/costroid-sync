package tui

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/storage"
)

// loadWindowDays bounds how much local history the dashboard reads. It mirrors
// the statusline window: the current month-to-date plus the 7-day anomaly
// lookback. Reading a bounded window keeps the opt-in TUI cheap and
// deterministic; it is surfaced to the user as "last N days".
const loadWindowDays = 45

// DataStatus mirrors the statusline's deterministic states so the TUI can show
// a friendly empty/missing screen rather than leak a raw error.
type DataStatus int

const (
	// DataOK means cost_records were read (or exist outside the window).
	DataOK DataStatus = iota
	// DataEmpty means the database exists but holds no cost_records at all.
	DataEmpty
	// DataMissingDB means no local database file exists yet.
	DataMissingDB
	// DataUnavailable means the database could not be read.
	DataUnavailable
)

// Dashboard is the metadata-only snapshot rendered by the TUI. It is built once
// from read-only local SQLite and holds aggregates only — never any record
// content, raw provider payload, prompt/completion text, or credential.
type Dashboard struct {
	Status      DataStatus
	GeneratedAt time.Time
	DBPath      string
	WindowDays  int

	Overview      analysis.Statusline
	Forecast      *analysis.ForecastResult // nil when data is insufficient
	Budget        *analysis.BudgetStatus   // nil when no budget is configured
	Anomalies     []analysis.Anomaly
	Providers     []analysis.ProviderTotal
	Models        []analysis.ModelTotal
	Savings       []analysis.SavingsRecommendation
	Syncs         []analysis.ProviderActivity
	History       []analysis.DailyTotal  // daily spend rollups over the read window
	TrendsWeekly  []analysis.TrendPeriod // ISO-week rollups over the read window
	TrendsMonthly []analysis.TrendPeriod // calendar-month rollups over the read window
	LastSync      *time.Time

	// Spark is a metadata-only trailing daily-spend series (one total per UTC
	// day, oldest→newest) used only to draw the static dashboard sparkline.
	Spark []float64
}

// LoadDashboard opens the local SQLite database read-only and assembles a
// Dashboard. It never creates the database and performs no network, provider
// API, or credential access. Any data problem collapses to a deterministic
// Status so the caller always has something safe to render.
func LoadDashboard(ctx context.Context, now time.Time) Dashboard {
	now = now.UTC()
	d := Dashboard{Status: DataUnavailable, GeneratedAt: now, WindowDays: loadWindowDays}
	db, path, status := openLocalDB()
	d.DBPath = path
	if db == nil {
		d.Status = status
		return d
	}
	defer db.Close()
	records, err := storage.GetRecords(ctx, db, now.AddDate(0, 0, -loadWindowDays))
	if err != nil {
		return d // DataUnavailable
	}
	if len(records) == 0 {
		if has, err := storage.HasAnyRecords(ctx, db); err != nil {
			return d
		} else if !has {
			d.Status = DataEmpty
			return d
		}
	}
	d.Status = DataOK

	budget := loadBudgetConfig(ctx, db)
	d.Overview = analysis.BuildStatusline(records, budget, now)
	if budget != nil {
		if st, err := analysis.CheckBudget(*budget, records, now); err == nil {
			d.Budget = &st
		}
	}
	if f, err := analysis.Forecast(records, now); err == nil {
		d.Forecast = &f
	}
	d.Anomalies = analysis.DetectAnomalies(records)
	d.Providers = analysis.AggregateByProvider(records)
	d.Models = analysis.AggregateByModel(records)
	d.Savings = analysis.Recommend(records)
	d.Syncs = analysis.LatestActivityByProvider(records)
	d.History = analysis.DailyTotals(records)
	d.TrendsWeekly = analysis.Trends(records, analysis.TrendWeekly)
	d.TrendsMonthly = analysis.Trends(records, analysis.TrendMonthly)
	d.LastSync = loadLastSync(ctx, db)
	// Build the sparkline series by field access only — see spendPoint (no import).
	var pts []spendPoint
	for _, r := range records {
		if t, err := time.Parse(time.RFC3339, r.RecordedAt); err == nil {
			pts = append(pts, spendPoint{at: t.UTC(), cost: r.CostUSD})
		}
	}
	d.Spark = dailySparkSeries(pts, now, sparkDays)
	return d
}

// openLocalDB resolves the DB path and opens it read-only. It returns a nil
// *sql.DB plus a DataStatus when the database is missing or cannot be opened;
// it never creates the file (mode=ro fails instead).
func openLocalDB() (*sql.DB, string, DataStatus) {
	path, err := storage.ResolveDBPath()
	if err != nil {
		return nil, "", DataUnavailable
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, path, DataMissingDB
	}
	db, err := storage.OpenReadOnly(path)
	if err != nil {
		return nil, path, DataUnavailable
	}
	return db, path, DataOK
}

// loadBudgetConfig reads the optional local budget. A missing budget (or table)
// degrades to nil so the dashboard simply omits the budget panel detail.
func loadBudgetConfig(ctx context.Context, db *sql.DB) *analysis.BudgetConfig {
	b, err := storage.GetBudget(ctx, db)
	if err != nil {
		return nil
	}
	return &analysis.BudgetConfig{
		AmountUSD: b.AmountUSD,
		Period:    analysis.BudgetPeriod(b.Period),
	}
}

// loadLastSync reads the optional last-sync timestamp. A never-synced database
// (or missing table) degrades to nil → the UI shows "never".
func loadLastSync(ctx context.Context, db *sql.DB) *time.Time {
	t, err := storage.GetLastSync(ctx, db)
	if err != nil {
		return nil
	}
	return &t
}
