package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/costroid/costroid/providers"
	"github.com/costroid/costroid/storage"
	"github.com/costroid/costroid/tui"
)

// stubProvider is a fake metadata source for exercising the sync-TUI stage
// closures without any network. It returns canned NormalizedCostRecord metadata
// (which structurally cannot hold prompt/completion content) or a fixed error.
type stubProvider struct {
	recs []providers.NormalizedCostRecord
	err  error
}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) Fetch(_ context.Context, _ int) ([]providers.NormalizedCostRecord, error) {
	return s.recs, s.err
}

func stubPlan(name string, p providers.Provider, missing ...string) provPlan {
	return provPlan{
		reg:      providers.Registration{Name: name, New: func(string) providers.Provider { return p }},
		adminKey: "key",
		missing:  missing,
	}
}

func mockRecords(n int) []providers.NormalizedCostRecord {
	out := make([]providers.NormalizedCostRecord, n)
	for i := range out {
		out[i] = providers.NormalizedCostRecord{Provider: "stub", Model: "demo", TotalTokens: 10, CostUSD: 1}
	}
	return out
}

func TestFetchStage_SuccessAccumulatesRecords(t *testing.T) {
	acc := &syncAccumulator{}
	out := fetchStage(context.Background(), stubPlan("openai", stubProvider{recs: mockRecords(3)}), acc).Run()
	if out.State != tui.StageDone {
		t.Fatalf("state = %v, want StageDone", out.State)
	}
	if out.Detail != "3 records" {
		t.Errorf("detail = %q, want \"3 records\"", out.Detail)
	}
	if len(acc.records) != 3 {
		t.Errorf("accumulated %d records, want 3", len(acc.records))
	}
}

func TestFetchStage_MissingKeyIsSkippedWithoutCallingProvider(t *testing.T) {
	called := false
	p := stubProvider{recs: mockRecords(1)}
	plan := provPlan{
		reg: providers.Registration{Name: "anthropic", New: func(string) providers.Provider {
			called = true
			return p
		}},
		missing: []string{"ANTHROPIC_ADMIN_KEY"},
	}
	acc := &syncAccumulator{}
	out := fetchStage(context.Background(), plan, acc).Run()
	if out.State != tui.StageSkipped || out.Detail != "missing key" {
		t.Errorf("got %v/%q, want StageSkipped/\"missing key\"", out.State, out.Detail)
	}
	if called {
		t.Error("provider constructor must not run when the key is missing")
	}
	if len(acc.records) != 0 {
		t.Error("skipped stage must not accumulate records")
	}
}

// TestFetchStage_ErrorDetailLeaksNoSecret is the metadata-only / safe-error
// guard: a provider error that embeds a URL and an API-key-looking token must
// surface only a short, sanitized phrase — never the raw error text.
func TestFetchStage_ErrorDetailLeaksNoSecret(t *testing.T) {
	rawErr := fmt.Errorf("GET https://api.example.com/v1/usage?api_key=sk-SUPERSECRET123: %w", io.EOF)
	acc := &syncAccumulator{}
	out := fetchStage(context.Background(), stubPlan("openai", stubProvider{err: rawErr}), acc).Run()
	if out.State != tui.StageError {
		t.Fatalf("state = %v, want StageError", out.State)
	}
	for _, leak := range []string{"SUPERSECRET", "sk-", "https", "api_key", "api.example.com"} {
		if strings.Contains(out.Detail, leak) {
			t.Errorf("error detail %q leaked %q", out.Detail, leak)
		}
	}
	if len(acc.records) != 0 {
		t.Error("a failed fetch must not accumulate records")
	}
}

func TestSafeStageError(t *testing.T) {
	cases := map[error]string{
		context.DeadlineExceeded: "timed out",
		context.Canceled:         "canceled",
		errors.New("dial tcp 10.0.0.1:443: connection refused"): "request failed",
	}
	for err, want := range cases {
		if got := safeStageError(err); got != want {
			t.Errorf("safeStageError(%v) = %q, want %q", err, got, want)
		}
		// Never echo the raw error text.
		if got := safeStageError(err); strings.Contains(got, "10.0.0.1") {
			t.Errorf("safeStageError leaked address: %q", got)
		}
	}
}

func TestCountDetail(t *testing.T) {
	cases := map[int]string{0: "0 records", 1: "1 record", 5: "5 records"}
	for n, want := range cases {
		if got := countDetail(n, "record"); got != want {
			t.Errorf("countDetail(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestSyncTUIAllowed_FallsBack proves the deterministic-output fallback: under
// `go test` std streams are not a TTY, so the animated path is never taken;
// --no-animation also forces the fallback. Both keep CI/non-TTY on plain output.
func TestSyncTUIAllowed_FallsBack(t *testing.T) {
	syncNoAnimation = false
	defer func() { syncNoAnimation = false }()
	if syncTUIAllowed() {
		t.Error("syncTUIAllowed() should be false under non-TTY test streams")
	}
	syncNoAnimation = true
	if syncTUIAllowed() {
		t.Error("--no-animation must force the plain-output fallback")
	}
}

func TestSyncCmd_HasTUIFlags(t *testing.T) {
	for _, name := range []string{"tui", "no-animation"} {
		if syncCmd.Flags().Lookup(name) == nil {
			t.Errorf("sync command missing --%s flag", name)
		}
	}
}

func TestBuildSyncPlans_NoCredsErrors(t *testing.T) {
	clearProviderEnv(t)

	syncProvider = "all"
	defer func() { syncProvider = "openai" }()
	if _, err := buildSyncPlans(); err == nil || !strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") {
		t.Errorf("all-provider preflight without creds = %v, want noProviderCredsHelp", err)
	}

	syncProvider = "openai"
	_, err := buildSyncPlans()
	if err == nil || !strings.Contains(err.Error(), "OPENAI_ADMIN_KEY") {
		t.Errorf("single-provider preflight without creds = %v, want MissingEnvHelp", err)
	}
}

// TestSyncStages_FullPipelineWritesToTempDB exercises the sqlite + analysis
// stages end-to-end against an isolated temp database (COSTROID_DB), proving the
// sequential stage closures persist exactly the fetched records.
func TestSyncStages_FullPipelineWritesToTempDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "costroid.db")
	t.Setenv("COSTROID_DB", dbPath)
	syncProvider = "openai"
	syncPush = false
	defer func() { syncProvider = "openai" }()

	acc := &syncAccumulator{}
	plans := []provPlan{stubPlan("openai", stubProvider{recs: mockRecords(4)})}
	stages := buildSyncStages(plans, context.Background(), acc)

	for _, st := range stages {
		if out := st.Run(); out.State == tui.StageError {
			t.Fatalf("stage %q errored: %q", st.Label, out.Detail)
		}
	}
	if len(acc.records) != 4 {
		t.Fatalf("accumulated %d records, want 4", len(acc.records))
	}

	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	defer db.Close()
	ok, err := storage.HasAnyRecords(context.Background(), db)
	if err != nil || !ok {
		t.Errorf("expected persisted records in temp db (ok=%v err=%v)", ok, err)
	}
}
