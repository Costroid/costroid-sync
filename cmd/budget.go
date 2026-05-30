package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/costroid/costroid/analysis"
	"github.com/costroid/costroid/output"
	"github.com/costroid/costroid/storage"
)

var (
	budgetSet    float64
	budgetPeriod string
)

var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Set or check a spending budget",
	RunE:  runBudget,
}

func init() {
	budgetCmd.Flags().Float64Var(&budgetSet, "set", 0, "set budget amount in USD")
	budgetCmd.Flags().StringVar(&budgetPeriod, "period", string(analysis.BudgetMonthly), "budget period: daily, weekly, or monthly")
	rootCmd.AddCommand(budgetCmd)
}

func runBudget(cmd *cobra.Command, args []string) error {
	db, err := openLocalDB()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	if cmd.Flags().Changed("set") {
		return saveAndPrintBudget(ctx, cmd, db)
	}
	budget, err := storage.GetBudget(ctx, db)
	if errors.Is(err, storage.ErrBudgetNotFound) {
		fmt.Fprintln(cmd.OutOrStdout(), "No budget configured. Set one with `costroid budget --set 500 --period monthly`.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("load budget: %w", err)
	}
	return printSavedBudget(ctx, cmd, db, budget)
}

func openLocalDB() (*sql.DB, error) {
	dbPath, err := storage.ResolveDBPath()
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	db, err := storage.InitDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init db: %w", err)
	}
	return db, nil
}

func saveAndPrintBudget(ctx context.Context, cmd *cobra.Command, db *sql.DB) error {
	now := time.Now().UTC()
	config := analysis.BudgetConfig{AmountUSD: budgetSet, Period: analysis.BudgetPeriod(budgetPeriod)}
	if _, err := analysis.CheckBudget(config, nil, now); err != nil {
		return err
	}
	budget := storage.BudgetRecord{
		AmountUSD: budgetSet,
		Period:    budgetPeriod,
		UpdatedAt: now.Format(time.RFC3339),
	}
	if err := storage.SaveBudget(ctx, db, budget); err != nil {
		return err
	}
	return printBudgetStatus(ctx, cmd, db, config, now)
}

func printSavedBudget(ctx context.Context, cmd *cobra.Command, db *sql.DB, budget storage.BudgetRecord) error {
	period := budget.Period
	if cmd.Flags().Changed("period") {
		period = budgetPeriod
	}
	config := analysis.BudgetConfig{AmountUSD: budget.AmountUSD, Period: analysis.BudgetPeriod(period)}
	return printBudgetStatus(ctx, cmd, db, config, time.Now().UTC())
}

func printBudgetStatus(ctx context.Context, cmd *cobra.Command, db *sql.DB, config analysis.BudgetConfig, now time.Time) error {
	records, err := storage.GetRecords(ctx, db, time.Time{})
	if err != nil {
		return fmt.Errorf("read records: %w", err)
	}
	status, err := analysis.CheckBudget(config, records, now)
	if err != nil {
		return err
	}
	output.WriteBudgetStatus(cmd.OutOrStdout(), status)
	return nil
}
