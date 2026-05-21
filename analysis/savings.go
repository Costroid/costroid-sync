package analysis

// Recommendation is a suggested change that would reduce spend.
type Recommendation struct {
	Title         string
	EstimatedUSD  float64
	Justification string
}

// Recommend returns savings recommendations from recent usage.
// Real implementation (cheaper-model comparisons, etc.) lands in C4.
func Recommend() ([]Recommendation, error) {
	return nil, ErrNotImplemented
}
