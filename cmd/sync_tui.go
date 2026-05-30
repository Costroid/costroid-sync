package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/client"
	"github.com/costroid/costroid/output"
	"github.com/costroid/costroid/providers"
	"github.com/costroid/costroid/storage"
	"github.com/costroid/costroid/tui"
)

// syncTUIAllowed reports whether the animated --tui path may run: it requires an
// interactive terminal (both stdin and stdout TTYs, usable TERM) and that
// --no-animation was not requested. Otherwise sync falls back to plain output.
func syncTUIAllowed() bool {
	if syncNoAnimation {
		return false
	}
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	return tui.InteractiveAllowed(stdoutTTY, stdinTTY, os.Getenv("TERM"))
}

// provPlan is the pre-flighted intent for one provider: its registration, the
// resolved admin key, and any missing env vars (non-empty ⇒ a Skipped stage).
type provPlan struct {
	reg      providers.Registration
	adminKey string
	missing  []string
}

// syncAccumulator collects the results of the sequential sync stages. Because
// stages never run concurrently and the cmd layer reads these fields only after
// a completed run (RunSync returns completed==true, by which point every stage
// goroutine has finished), the access is race-free without a mutex.
type syncAccumulator struct {
	records []providers.NormalizedCostRecord
	tip     string
}

// runSyncTUI runs the opt-in animated sync. It performs the same real work as a
// normal sync (fetch → write → analyze → optional push) but surfaces each step
// as a live stage. On a completed run it prints the same closing summary as the
// plain path; otherwise it returns a safe error and saves nothing partial.
func runSyncTUI(cmd *cobra.Command) error {
	plans, err := buildSyncPlans()
	if err != nil {
		return err
	}

	// workCtx bounds the real sync work (mirrors plain sync's 60s budget).
	// progCtx is the view lifetime only, so idle "press q to close" time after a
	// fast sync never trips the work timeout.
	workCtx, workCancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer workCancel()
	progCtx, progCancel := context.WithCancel(cmd.Context())
	defer progCancel()

	acc := &syncAccumulator{}
	opts := tui.Options{Color: os.Getenv("NO_COLOR") == "", ASCII: !utf8Locale()}
	completed, err := tui.RunSync(progCtx, opts, buildSyncStages(plans, workCtx, acc))
	if err != nil {
		return fmt.Errorf("sync view failed to start: %w", err)
	}
	if !completed {
		return errors.New("sync did not complete; run `costroid sync` for details")
	}
	return writeSyncTUISummary(cmd, acc)
}

// buildSyncPlans resolves the selected providers and pre-flights credentials,
// returning the same friendly errors as plain sync before any screen is painted
// (so a misconfigured sync never shows an empty animation).
func buildSyncPlans() ([]provPlan, error) {
	regs, err := selectedRegistrations(syncProvider)
	if err != nil {
		return nil, err
	}
	plans := make([]provPlan, 0, len(regs))
	configured := 0
	for _, reg := range regs {
		adminKey := os.Getenv(reg.EnvVar)
		missing := missingEnvVars(reg, adminKey)
		if len(missing) == 0 {
			configured++
		}
		plans = append(plans, provPlan{reg: reg, adminKey: adminKey, missing: missing})
	}
	if syncProvider == "all" {
		if configured == 0 {
			return nil, errors.New(noProviderCredsHelp)
		}
		return plans, nil
	}
	if len(plans) == 1 && len(plans[0].missing) > 0 {
		return nil, errors.New(plans[0].reg.MissingEnvHelp)
	}
	return plans, nil
}

// buildSyncStages assembles the truthful stage list: one fetch per provider,
// then the local write and analysis steps, plus an optional cloud push.
func buildSyncStages(plans []provPlan, ctx context.Context, acc *syncAccumulator) []tui.Stage {
	stages := make([]tui.Stage, 0, len(plans)+3)
	for _, p := range plans {
		stages = append(stages, fetchStage(ctx, p, acc))
	}
	stages = append(stages, sqliteStage(ctx, acc), analysisStage(acc))
	if syncPush {
		stages = append(stages, pushStage(ctx, acc))
	}
	return stages
}

func fetchStage(ctx context.Context, p provPlan, acc *syncAccumulator) tui.Stage {
	return tui.Stage{
		Label:  p.reg.Name,
		Action: "fetching metadata",
		Run: func() tui.StageOutcome {
			if len(p.missing) > 0 {
				return tui.StageOutcome{State: tui.StageSkipped, Detail: "missing key"}
			}
			recs, err := p.reg.New(p.adminKey).Fetch(ctx, syncDays)
			if err != nil {
				return tui.StageOutcome{State: tui.StageError, Detail: safeStageError(err)}
			}
			acc.records = append(acc.records, recs...)
			return tui.StageOutcome{State: tui.StageDone, Detail: countDetail(len(recs), "record")}
		},
	}
}

func sqliteStage(ctx context.Context, acc *syncAccumulator) tui.Stage {
	return tui.Stage{
		Label:  "sqlite",
		Action: "writing records",
		Run: func() tui.StageOutcome {
			n, err := saveSyncedRecords(ctx, acc.records)
			if err != nil {
				return tui.StageOutcome{State: tui.StageError, Detail: "write failed"}
			}
			return tui.StageOutcome{State: tui.StageDone, Detail: countDetail(n, "record")}
		},
	}
}

// analysisStage runs the real local analysis used by the closing summary: it
// computes the best savings tip (stored for the summary) and scans anomalies.
// It reports a metadata count only — never a money value (no animated money).
func analysisStage(acc *syncAccumulator) tui.Stage {
	return tui.Stage{
		Label:  "analysis",
		Action: "analyzing usage",
		Run: func() tui.StageOutcome {
			acc.tip = bestSavingsTip(acc.records)
			anoms := analysis.DetectAnomalies(acc.records)
			detail := ""
			switch {
			case len(anoms) == 1:
				detail = "1 anomaly"
			case len(anoms) > 1:
				detail = strconv.Itoa(len(anoms)) + " anomalies"
			}
			return tui.StageOutcome{State: tui.StageDone, Detail: detail}
		},
	}
}

// pushStage is best-effort: a missing config or a failed push is reported as
// Skipped (not Error) so it never aborts an otherwise successful sync, mirroring
// plain sync where a push problem is non-fatal and the local records are kept.
func pushStage(ctx context.Context, acc *syncAccumulator) tui.Stage {
	return tui.Stage{
		Label:  "costroid cloud",
		Action: "pushing records",
		Run: func() tui.StageOutcome {
			if len(acc.records) == 0 {
				return tui.StageOutcome{State: tui.StageSkipped, Detail: "no records"}
			}
			cfg, missing := cloudPushConfigFromEnv()
			if len(missing) > 0 {
				return tui.StageOutcome{State: tui.StageSkipped, Detail: "set " + strings.Join(missing, ", ")}
			}
			if err := client.PushRecords(ctx, cfg.BaseURL, cfg.OrgID, cfg.AgentKey, acc.records); err != nil {
				return tui.StageOutcome{State: tui.StageSkipped, Detail: "push failed"}
			}
			return tui.StageOutcome{State: tui.StageDone, Detail: countDetail(len(acc.records), "record")}
		},
	}
}

// saveSyncedRecords persists the synced records and the last-sync timestamp,
// exactly as the plain sync path does.
func saveSyncedRecords(ctx context.Context, records []providers.NormalizedCostRecord) (int, error) {
	dbPath, err := storage.ResolveDBPath()
	if err != nil {
		return 0, err
	}
	db, err := storage.InitDB(dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if err := storage.SaveRecords(ctx, db, records); err != nil {
		return 0, err
	}
	if err := storage.SaveLastSync(ctx, db, time.Now().UTC()); err != nil {
		return 0, err
	}
	return len(records), nil
}

// writeSyncTUISummary prints the same closing output as plain sync (table +
// savings tip + cloud hint) for the synced records.
func writeSyncTUISummary(cmd *cobra.Command, acc *syncAccumulator) error {
	out := cmd.OutOrStdout()
	if len(acc.records) == 0 {
		fmt.Fprintf(out, "No usage records for the last %d days.\n", syncDays)
		return nil
	}
	output.WriteTable(out, acc.records)
	if acc.tip != "" {
		fmt.Fprintln(out, acc.tip)
	}
	if !syncPush {
		fmt.Fprintln(out, "Want shared team dashboards? costroid.com")
	}
	return nil
}

// safeStageError maps a provider/HTTP error to a short, user-facing phrase. It
// never returns the raw error text, which could embed a URL, identifier, or
// other sensitive detail (no raw payloads/secrets in the UI — terminal-design §17).
func safeStageError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "request failed"
}

// countDetail renders "N unit(s)" with simple pluralization for regular nouns.
func countDetail(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}
