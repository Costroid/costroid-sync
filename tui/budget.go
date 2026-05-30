package tui

import (
	"strconv"
	"strings"
)

// budgetBody renders current-period budget status, or a hint to configure one.
// Over-budget is shown in red and always paired with the "OVER" text marker.
func budgetBody(d Dashboard, s Styles, _ int) string {
	if d.Budget == nil {
		return s.Faint.Render("No budget configured.") + "\n\n" +
			"Set one with:\n  costroid-sync budget --set <amount> --period daily|weekly|monthly"
	}
	const labelW = 12
	b := d.Budget
	state := "on track"
	if b.IsOverBudget {
		state = s.Alert.Render("OVER")
	}
	lines := []string{
		labeled(s, "Period", labelW, b.Period),
		labeled(s, "Budget", labelW, money(b.BudgetAmountUSD)),
		labeled(s, "Spend", labelW, money(b.SpendUSD)),
		labeled(s, "Remaining", labelW, money(b.RemainingUSD)),
		labeled(s, "Used", labelW, strconv.FormatFloat(b.UsedPercent, 'f', 1, 64)+"%"),
		labeled(s, "Status", labelW, state),
	}
	return strings.Join(lines, "\n")
}
