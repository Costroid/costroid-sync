package analysis

import (
	"errors"
	"testing"
	"time"

	"github.com/costroid/costroid/providers"
)

func TestCheckBudget_MonthlyOnTrack(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(monthlyBudget(500), []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 150, 100),
		testCostRecord(2026, 5, 20, 250, 100),
	}, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 400 || status.IsOverBudget {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestCheckBudget_MonthlyOverBudget(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(monthlyBudget(500), []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 1, 300, 100),
		testCostRecord(2026, 5, 20, 300, 100),
	}, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 600 || !status.IsOverBudget || status.RemainingUSD != -100 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestCheckBudget_DailyUsesCurrentUTCDayOnly(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(BudgetConfig{AmountUSD: 500, Period: BudgetDaily}, []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 21, 100, 100),
		testCostRecord(2026, 5, 22, 25, 100),
		testCostRecord(2026, 5, 23, 75, 100),
	}, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 25 {
		t.Fatalf("daily spend = %v, want 25", status.SpendUSD)
	}
}

func TestCheckBudget_WeeklyUsesMondayStartWeekOnly(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(BudgetConfig{AmountUSD: 500, Period: BudgetWeekly}, []providers.NormalizedCostRecord{
		testCostRecord(2026, 5, 17, 100, 100),
		testCostRecord(2026, 5, 18, 25, 100),
		testCostRecord(2026, 5, 24, 75, 100),
		testCostRecord(2026, 5, 25, 200, 100),
	}, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 100 {
		t.Fatalf("weekly spend = %v, want 100", status.SpendUSD)
	}
}

func TestCheckBudget_MonthlyUsesCurrentCalendarMonthOnly(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(monthlyBudget(500), []providers.NormalizedCostRecord{
		testCostRecord(2026, 4, 30, 100, 100),
		testCostRecord(2026, 5, 1, 25, 100),
		testCostRecord(2026, 5, 31, 75, 100),
		testCostRecord(2026, 6, 1, 200, 100),
	}, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 100 {
		t.Fatalf("monthly spend = %v, want 100", status.SpendUSD)
	}
}

func TestCheckBudget_InvalidConfig(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	cases := []BudgetConfig{
		{AmountUSD: 0, Period: BudgetMonthly},
		{AmountUSD: -1, Period: BudgetMonthly},
		{AmountUSD: 100, Period: "quarterly"},
	}
	for _, tc := range cases {
		if _, err := CheckBudget(tc, nil, now); !errors.Is(err, ErrInvalidBudget) {
			t.Fatalf("config %+v: want ErrInvalidBudget, got %v", tc, err)
		}
	}
}

func TestCheckBudget_ZeroRecordsDoNotPanic(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	status, err := CheckBudget(monthlyBudget(500), nil, now)
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if status.SpendUSD != 0 || status.UsedPercent != 0 || status.IsOverBudget {
		t.Fatalf("unexpected empty status: %+v", status)
	}
}

func monthlyBudget(amount float64) BudgetConfig {
	return BudgetConfig{AmountUSD: amount, Period: BudgetMonthly}
}
