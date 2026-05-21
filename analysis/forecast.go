package analysis

import "errors"

// ErrNotImplemented is returned by analysis stubs not yet wired up.
var ErrNotImplemented = errors.New("analysis not implemented yet")

// Forecast predicts month-end spend from recent daily totals.
// Real EMA + linear regression lands in C5.
func Forecast(dailyTotalsUSD []float64) (float64, error) {
	return 0, ErrNotImplemented
}
