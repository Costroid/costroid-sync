package analysis

// DetectAnomalies returns the indices of days whose spend deviates
// significantly from the rolling average.
// Real implementation lands in C5.
func DetectAnomalies(dailyTotalsUSD []float64) ([]int, error) {
	return nil, ErrNotImplemented
}
