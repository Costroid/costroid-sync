package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid-sync/output"
	"github.com/costroid/costroid-sync/providers"
	"github.com/costroid/costroid-sync/storage"
)

var (
	syncProvider string
	syncDays     int
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch usage from configured providers",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncProvider, "provider", "openai", "provider to sync (openai, anthropic, all)")
	syncCmd.Flags().IntVar(&syncDays, "days", 30, "lookback window in days")
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

	for _, note := range notes {
		fmt.Fprintln(cmd.OutOrStdout(), note)
	}
	if len(records) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No usage records for the last %d days.\n", syncDays)
		return nil
	}
	output.WriteTable(cmd.OutOrStdout(), records)
	return nil
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
		if adminKey == "" {
			if skipMissing {
				notes = append(notes, fmt.Sprintf("Skipping %s: %s is not set.", reg.Name, reg.EnvVar))
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
		return nil, nil, errors.New("no provider admin keys are set; set OPENAI_ADMIN_KEY or ANTHROPIC_ADMIN_KEY")
	}
	return records, notes, nil
}

func availableProviders() string {
	var names []string
	for _, reg := range providers.All() {
		names = append(names, reg.Name)
	}
	names = append(names, "all")
	return strings.Join(names, ", ")
}
