package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/providers"
	"github.com/costroid/costroid/storage"
)

func readLocalRecords(cmd *cobra.Command, since time.Time) ([]providers.NormalizedCostRecord, error) {
	dbPath, err := storage.ResolveDBPath()
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	db, err := storage.InitDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	records, err := storage.GetRecords(ctx, db, since)
	if err != nil {
		return nil, fmt.Errorf("read records: %w", err)
	}
	return records, nil
}
