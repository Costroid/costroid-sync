package tui

import "time"

// sparkDays is the trailing window (in days) summarized by the dashboard
// sparkline. It is a small, bounded visual aid over the data already read
// read-only; it adds no I/O.
const sparkDays = 14

// spendPoint is a single metadata-only (timestamp, cost) pair extracted from a
// stored record. The tui package never spells the providers record type (the
// import guard forbids importing providers); callers build these by field access
// over the inferred record slice, so no forbidden import is introduced.
type spendPoint struct {
	at   time.Time
	cost float64
}

// dailySparkSeries buckets spend by UTC calendar day over the trailing `days`
// window ending at now, returning exactly `days` totals oldest→newest (0 for
// quiet days). It is pure and deterministic — used only to draw the static
// sparkline; it never animates or encodes anything but daily cost totals.
func dailySparkSeries(points []spendPoint, now time.Time, days int) []float64 {
	if days < 1 {
		return nil
	}
	end := dayStart(now)
	start := end.AddDate(0, 0, -(days - 1))

	byDay := make(map[time.Time]float64, len(points))
	for _, p := range points {
		d := dayStart(p.at)
		if d.Before(start) || d.After(end) {
			continue
		}
		byDay[d] += p.cost
	}

	out := make([]float64, 0, days)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, byDay[d])
	}
	return out
}

// dayStart truncates t to the start of its UTC day.
func dayStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
