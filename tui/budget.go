package tui

import (
	"strconv"
	"strings"
)

// budgetBody renders current-period budget status with a prominent pressure
// meter, or a hint to configure one. Over-budget fills the meter red and is
// always paired with the "OVER" text marker. Money values are stable — never
// animated.
func budgetBody(d Dashboard, s Styles, _ int) string {
	if d.Budget == nil {
		return s.Faint.Render("No budget configured.") + "\n\n" +
			"Set one with:\n  costroid budget --set <amount> --period daily|weekly|monthly"
	}
	const labelW = 12
	const meterW = 16
	b := d.Budget
	fill := s.meterFill()
	state := "on track"
	if b.IsOverBudget {
		fill = s.Alert
		state = s.Alert.Render("OVER")
	}
	pressure := meter(s, b.UsedPercent/100, meterW, fill) + "  " +
		strconv.FormatFloat(b.UsedPercent, 'f', 1, 64) + "%"
	lines := []string{
		labeled(s, "Period", labelW, b.Period),
		labeled(s, "Budget", labelW, money(b.BudgetAmountUSD)),
		labeled(s, "Spend", labelW, money(b.SpendUSD)),
		labeled(s, "Remaining", labelW, money(b.RemainingUSD)),
		labeled(s, "Used", labelW, pressure),
		labeled(s, "Status", labelW, state),
	}
	return strings.Join(lines, "\n")
}
