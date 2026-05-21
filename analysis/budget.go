package analysis

// BudgetStatus reports how close current spend is to a configured budget.
type BudgetStatus struct {
	BudgetUSD float64
	SpentUSD  float64
	Period    string // e.g. "monthly"
}

// Check returns the current status against the configured budget.
// Real implementation lands in C6.
func Check() (BudgetStatus, error) {
	return BudgetStatus{}, ErrNotImplemented
}
