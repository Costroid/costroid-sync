package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	syncCmd.Flags().StringVar(&syncProvider, "provider", "openai", "provider to sync (currently: openai)")
	syncCmd.Flags().IntVar(&syncDays, "days", 30, "lookback window in days")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	if syncProvider != "openai" {
		return fmt.Errorf("provider %q not supported in C2 (only \"openai\")", syncProvider)
	}
	adminKey := os.Getenv("OPENAI_ADMIN_KEY")
	if adminKey == "" {
		return errors.New(
			"OPENAI_ADMIN_KEY is not set.\n" +
				"Create an admin key at https://platform.openai.com/settings/organization/admin-keys, then:\n" +
				"  export OPENAI_ADMIN_KEY=sk-admin-...",
		)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	p := providers.NewOpenAIProvider(adminKey)
	records, err := p.Fetch(ctx, syncDays)
	if err != nil {
		return fmt.Errorf("openai fetch: %w", err)
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

	if len(records) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No usage records for the last %d days.\n", syncDays)
		return nil
	}
	output.WriteTable(cmd.OutOrStdout(), records)
	return nil
}
