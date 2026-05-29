package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
	"github.com/costroid/costroid-sync/output"
	"github.com/costroid/costroid-sync/storage"
)

var (
	statuslineFormat string
	statuslinePlain  bool
)

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Print a one-line local cost summary for tmux/Byobu/shell status bars",
	Long: "Print a single deterministic line summarising local cost metadata (MTD spend, " +
		"forecast, budget, anomalies, and last-sync freshness) for tmux/Byobu/shell status bars, " +
		"or a stable JSON object for scripts.\n\n" +
		"It reads the local SQLite database read-only and performs no network request, provider " +
		"API call, or provider sync — run `costroid-sync sync` separately on your own schedule. " +
		"tmux/Byobu own the polling cadence; there is no watch process or daemon.",
	RunE: runStatusline,
}

func init() {
	statuslineCmd.Flags().StringVar(&statuslineFormat, "format", "plain",
		"output format: plain, tmux, byobu, or json")
	statuslineCmd.Flags().BoolVar(&statuslinePlain, "plain", false,
		"force ASCII glyphs and no color/style codes, regardless of --format")
	rootCmd.AddCommand(statuslineCmd)
}

func runStatusline(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(statuslineFormat))
	if !validStatuslineFormat(format) {
		return fmt.Errorf("invalid --format %q: must be plain, tmux, byobu, or json", statuslineFormat)
	}
	view := buildStatuslineView(format, time.Now().UTC())
	return output.WriteStatusline(cmd.OutOrStdout(), view)
}

func validStatuslineFormat(format string) bool {
	switch format {
	case "plain", "tmux", "byobu", "json":
		return true
	default:
		return false
	}
}

// buildStatuslineView resolves the local DB read-only and assembles the view.
// It never returns an error: data problems collapse to deterministic statuses
// (missing_db / empty / unavailable) so the poller always prints one line and
// exits 0. The DB is opened read-only and is never created here.
func buildStatuslineView(format string, now time.Time) output.StatuslineView {
	view := output.StatuslineView{
		Format:      format,
		PlainFlag:   statuslinePlain,
		NoColor:     os.Getenv("NO_COLOR") != "",
		ASCIIGlyph:  statuslinePlain || !utf8Locale(),
		GeneratedAt: now,
		Status:      output.StatusUnavailable, // safe default on any early failure
	}

	path, err := storage.ResolveDBPath()
	if err != nil {
		return view
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		view.Status = output.StatusMissingDB
		return view
	}
	db, err := storage.OpenReadOnly(path)
	if err != nil {
		return view
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return populateStatuslineView(ctx, db, view, now)
}

// populateStatuslineView reads metadata from an already-open read-only DB and
// fills the metrics. Only a cost_records read failure maps to "unavailable";
// optional budget/sync_state reads degrade gracefully so a missing optional
// table never poisons the line.
func populateStatuslineView(ctx context.Context, db *sql.DB, view output.StatuslineView, now time.Time) output.StatuslineView {
	// Window covers the current month plus the anomaly 7-day lookback; far
	// cheaper than scanning full history for a frequently polled command.
	records, err := storage.GetRecords(ctx, db, now.AddDate(0, 0, -45))
	if err != nil {
		view.Status = output.StatusUnavailable
		return view
	}
	if len(records) == 0 {
		has, err := storage.HasAnyRecords(ctx, db)
		if err != nil {
			view.Status = output.StatusUnavailable
			return view
		}
		if !has {
			view.Status = output.StatusEmpty
			return view
		}
		// Table has only rows older than the window: ok with zero metrics.
	}
	view.Status = output.StatusOK
	view.LastSyncAt = readLastSync(ctx, db)
	view.Metrics = analysis.BuildStatusline(records, readBudget(ctx, db), now)
	return view
}

func readLastSync(ctx context.Context, db *sql.DB) *time.Time {
	t, err := storage.GetLastSync(ctx, db)
	if err != nil {
		return nil // never synced (or missing table) → deterministic "never"
	}
	return &t
}

func readBudget(ctx context.Context, db *sql.DB) *analysis.BudgetConfig {
	budget, err := storage.GetBudget(ctx, db)
	if err != nil {
		return nil // no budget configured (or missing table) → token omitted
	}
	return &analysis.BudgetConfig{
		AmountUSD: budget.AmountUSD,
		Period:    analysis.BudgetPeriod(budget.Period),
	}
}

// utf8Locale reports whether the environment locale indicates UTF-8. The first
// non-empty of LC_ALL, LC_CTYPE, LANG decides (standard precedence). When none
// confirm UTF-8 we fall back to ASCII glyphs — the braille mark is an accent,
// never a layout dependency.
func utf8Locale() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := os.Getenv(k)
		if v == "" {
			continue
		}
		lv := strings.ToLower(v)
		return strings.Contains(lv, "utf-8") || strings.Contains(lv, "utf8")
	}
	return false
}
