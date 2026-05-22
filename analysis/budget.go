package analysis

import (
	"errors"
	"fmt"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

type BudgetPeriod string

const (
	BudgetDaily   BudgetPeriod = "daily"
	BudgetWeekly  BudgetPeriod = "weekly"
	BudgetMonthly BudgetPeriod = "monthly"
)

var ErrInvalidBudget = errors.New("invalid budget")

type BudgetConfig struct {
	AmountUSD float64
	Period    BudgetPeriod
}

// BudgetStatus reports current-period spend against a configured local budget.
type BudgetStatus struct {
	Period          string
	BudgetAmountUSD float64
	SpendUSD        float64
	RemainingUSD    float64
	UsedPercent     float64
	IsOverBudget    bool
}

func CheckBudget(config BudgetConfig, records []providers.NormalizedCostRecord, now time.Time) (BudgetStatus, error) {
	if err := validateBudgetConfig(config); err != nil {
		return BudgetStatus{}, err
	}
	start, end := budgetWindow(config.Period, now.UTC())
	spend := budgetSpend(records, start, end)
	return BudgetStatus{
		Period:          string(config.Period),
		BudgetAmountUSD: config.AmountUSD,
		SpendUSD:        spend,
		RemainingUSD:    config.AmountUSD - spend,
		UsedPercent:     spend / config.AmountUSD * 100,
		IsOverBudget:    spend > config.AmountUSD,
	}, nil
}

func validateBudgetConfig(config BudgetConfig) error {
	if config.AmountUSD <= 0 {
		return fmt.Errorf("%w: amount must be greater than zero", ErrInvalidBudget)
	}
	switch config.Period {
	case BudgetDaily, BudgetWeekly, BudgetMonthly:
		return nil
	default:
		return fmt.Errorf("%w: period must be daily, weekly, or monthly", ErrInvalidBudget)
	}
}

func budgetWindow(period BudgetPeriod, now time.Time) (time.Time, time.Time) {
	today := dateOnly(now)
	switch period {
	case BudgetDaily:
		return today, today.AddDate(0, 0, 1)
	case BudgetWeekly:
		offset := (int(today.Weekday()) + 6) % 7
		start := today.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7)
	default:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
}

func budgetSpend(records []providers.NormalizedCostRecord, start, end time.Time) float64 {
	total := 0.0
	for _, r := range records {
		t, ok := parseRecordedAt(r.RecordedAt)
		if ok && !t.Before(start) && t.Before(end) {
			total += r.CostUSD
		}
	}
	return total
}
