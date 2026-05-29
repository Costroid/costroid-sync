package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/analysis"
	"github.com/costroid/costroid-sync/client"
	"github.com/costroid/costroid-sync/output"
	"github.com/costroid/costroid-sync/providers"
	"github.com/costroid/costroid-sync/storage"
)

const (
	defaultCostroidAPIURL = "https://costroid.com"
	envCostroidAPIURL     = "COSTROID_API_URL"
	envCostroidOrgID      = "COSTROID_ORG_ID"
	envCostroidAgentKey   = "COSTROID_AGENT_KEY"
)

var (
	syncProvider string
	syncDays     int
	syncPush     bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch usage from configured providers",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncProvider, "provider", "openai", "provider to sync (openai, anthropic, github-copilot (alias: copilot), google-gemini (alias: gemini), gcp-billing (alias: gcp), azure-openai, aws-bedrock (alias: bedrock), all)")
	syncCmd.Flags().IntVar(&syncDays, "days", 30, "lookback window in days")
	syncCmd.Flags().BoolVar(&syncPush, "push", false, "push synced metadata records to Costroid Cloud")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	regs, err := selectedRegistrations(syncProvider)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	records, notes, err := fetchSelectedProviders(ctx, regs, syncDays, syncProvider == "all")
	if err != nil {
		return err
	}

	dbPath, err := storage.ResolveDBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	db, err := storage.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	if err := storage.SaveRecords(ctx, db, records); err != nil {
		return fmt.Errorf("save records: %w", err)
	}
	if err := storage.SaveLastSync(ctx, db, time.Now().UTC()); err != nil {
		return fmt.Errorf("record sync time: %w", err)
	}
	maybePushCloud(cmd, records)

	for _, note := range notes {
		fmt.Fprintln(cmd.OutOrStdout(), note)
	}
	if len(records) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No usage records for the last %d days.\n", syncDays)
		return nil
	}
	output.WriteTable(cmd.OutOrStdout(), records)
	if tip := bestSavingsTip(records); tip != "" {
		fmt.Fprintln(cmd.OutOrStdout(), tip)
	}
	if !syncPush {
		fmt.Fprintln(cmd.OutOrStdout(), "Want shared team dashboards? costroid.com")
	}
	return nil
}

type cloudPushConfig struct {
	BaseURL  string
	OrgID    string
	AgentKey string
}

func maybePushCloud(cmd *cobra.Command, records []providers.NormalizedCostRecord) {
	if !syncPush || len(records) == 0 {
		return
	}
	cfg, missing := cloudPushConfigFromEnv()
	out := cmd.OutOrStdout()
	if len(missing) > 0 {
		fmt.Fprintf(out, "Cloud push skipped: set %s. Local records were saved.\n", strings.Join(missing, ", "))
		return
	}

	pushCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	if err := client.PushRecords(pushCtx, cfg.BaseURL, cfg.OrgID, cfg.AgentKey, records); err != nil {
		fmt.Fprintf(out, "Cloud push did not complete: %v. Local records were saved.\n", err)
		return
	}
	fmt.Fprintf(out, "Pushed %d records to Costroid Cloud.\n", len(records))
}

func cloudPushConfigFromEnv() (cloudPushConfig, []string) {
	cfg := cloudPushConfig{
		BaseURL:  strings.TrimSpace(os.Getenv(envCostroidAPIURL)),
		OrgID:    strings.TrimSpace(os.Getenv(envCostroidOrgID)),
		AgentKey: strings.TrimSpace(os.Getenv(envCostroidAgentKey)),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultCostroidAPIURL
	}
	var missing []string
	if cfg.OrgID == "" {
		missing = append(missing, envCostroidOrgID)
	}
	if cfg.AgentKey == "" {
		missing = append(missing, envCostroidAgentKey)
	}
	return cfg, missing
}

// bestSavingsTip returns a short one-liner for the largest cheaper-model
// opportunity in records, or "" if none qualifies. Wrapped in recover so
// a bug in savings analysis can never break a successful sync.
func bestSavingsTip(records []providers.NormalizedCostRecord) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
		}
	}()
	recs := analysis.Recommend(records)
	if len(recs) == 0 {
		return ""
	}
	top := recs[0]
	return fmt.Sprintf(
		"⚡ Spent $%.2f on %s. Estimated cost on %s: $%.2f (~%.0f%% lower).",
		top.CurrentCostUSD, top.CurrentModel,
		top.RecommendedModel, top.EstimatedCostUSD, top.SavingsPercent,
	)
}

func selectedRegistrations(name string) ([]providers.Registration, error) {
	if name == "all" {
		return providers.All(), nil
	}
	reg, ok := providers.Get(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not supported (available: %s)", name, availableProviders())
	}
	return []providers.Registration{reg}, nil
}

func fetchSelectedProviders(ctx context.Context, regs []providers.Registration, days int, skipMissing bool) ([]providers.NormalizedCostRecord, []string, error) {
	var (
		records    []providers.NormalizedCostRecord
		notes      []string
		configured int
	)
	for _, reg := range regs {
		adminKey := os.Getenv(reg.EnvVar)
		missing := missingEnvVars(reg, adminKey)
		if len(missing) > 0 {
			if skipMissing {
				notes = append(notes, fmt.Sprintf("Skipping %s: %s not set.", reg.Name, strings.Join(missing, ", ")))
				continue
			}
			return nil, nil, errors.New(reg.MissingEnvHelp)
		}
		configured++
		fetched, err := reg.New(adminKey).Fetch(ctx, days)
		if err != nil {
			return nil, nil, fmt.Errorf("%s fetch: %w", reg.Name, err)
		}
		records = append(records, fetched...)
	}
	if skipMissing && configured == 0 {
		return nil, nil, errors.New("no provider credentials configured; export at least one of OPENAI_ADMIN_KEY, ANTHROPIC_ADMIN_KEY, GITHUB_PAT + GITHUB_ORG, GEMINI_BILLING_EXPORT, GCP_SERVICE_ACCOUNT_JSON + GCP_BILLING_PROJECT + GCP_BILLING_TABLE, AZURE_TENANT_ID + AZURE_CLIENT_ID + AZURE_CLIENT_SECRET + AZURE_SUBSCRIPTION_ID, or AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY")
	}
	return records, notes, nil
}

func missingEnvVars(reg providers.Registration, adminKey string) []string {
	var missing []string
	if adminKey == "" {
		missing = append(missing, reg.EnvVar)
	}
	for _, v := range reg.ExtraEnvVars {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}
	return missing
}

func availableProviders() string {
	var names []string
	for _, reg := range providers.All() {
		names = append(names, reg.Name)
	}
	names = append(names, "all")
	return strings.Join(names, ", ")
}
