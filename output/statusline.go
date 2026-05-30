package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/costroid/costroid/analysis"
)

// Statusline status states. JSON `status` is a compatibility contract.
const (
	StatusOK          = "ok"
	StatusEmpty       = "empty"
	StatusMissingDB   = "missing_db"
	StatusUnavailable = "unavailable"
)

const (
	glyphPrefix = "⣿ costroid" // brand glyph + wordmark (UTF-8)
	asciiPrefix = "[costroid]" // ASCII fallback
	plainPrefix = "costroid"   // bare wordmark for no-data / unavailable lines
	tokenSep    = "  "         // exactly two spaces between tokens
)

// StatuslineView is everything needed to render one statusline. It carries
// metadata-only aggregates plus presentation flags; the cmd layer fills it from
// read-only local SQLite and injected env/clock values so rendering stays pure
// and deterministic.
type StatuslineView struct {
	Status      string
	Metrics     analysis.Statusline
	LastSyncAt  *time.Time
	GeneratedAt time.Time

	Format     string // "plain" | "tmux" | "byobu" | "json"
	PlainFlag  bool   // --plain: force ASCII glyph + no color/style codes
	NoColor    bool   // NO_COLOR present and non-empty
	ASCIIGlyph bool   // true ⇒ use [costroid]; set when --plain or non-UTF-8 env
}

// WriteStatusline renders v to w. It emits one physical line + trailing newline
// for plain/tmux/byobu and one JSON object for json — never embedded newlines.
func WriteStatusline(w io.Writer, v StatuslineView) error {
	if v.Format == "json" {
		return writeStatuslineJSON(w, v)
	}
	fmt.Fprintln(w, v.line())
	return nil
}

// line builds the single statusline string (without trailing newline).
func (v StatuslineView) line() string {
	switch v.Status {
	case StatusOK:
		return v.okLine()
	case StatusUnavailable:
		return plainPrefix + tokenSep + "unavailable"
	default: // empty, missing_db
		return plainPrefix + tokenSep + "no local data" + tokenSep + "run costroid sync"
	}
}

// okLine renders the populated statusline per the binding token grammar.
func (v StatuslineView) okLine() string {
	color := v.colorEnabled()
	segments := []string{v.prefix()}

	segments = append(segments, v.green("MTD "+formatMoney(v.Metrics.MTDCostUSD), color))
	if v.Metrics.ForecastUSD != nil {
		segments = append(segments, "forecast "+formatMoney(*v.Metrics.ForecastUSD))
	}
	if v.Metrics.BudgetPercent != nil {
		budget := "budget " + strconv.Itoa(*v.Metrics.BudgetPercent) + "%"
		if v.Metrics.OverBudget {
			budget = v.red(budget+" OVER", color)
		}
		segments = append(segments, budget)
	}
	anomalies := "anomalies " + strconv.Itoa(v.Metrics.AnomalyCount)
	if v.Metrics.AnomalyCount > 0 {
		anomalies = v.red(anomalies, color)
	}
	segments = append(segments, anomalies)
	segments = append(segments, "last sync "+v.lastSyncAge())

	return strings.Join(segments, tokenSep)
}

// prefix returns the brand prefix for an ok line (glyph or ASCII fallback).
func (v StatuslineView) prefix() string {
	if v.ASCIIGlyph {
		return asciiPrefix
	}
	return glyphPrefix
}

// lastSyncAge renders the freshness token value: "never" or a compact age.
func (v StatuslineView) lastSyncAge() string {
	if v.LastSyncAt == nil {
		return "never"
	}
	return formatAge(v.GeneratedAt.Sub(*v.LastSyncAt))
}

// colorEnabled reports whether style codes may be emitted. Color is
// format-driven (tmux/byobu only) and suppressed by --plain and NO_COLOR.
func (v StatuslineView) colorEnabled() bool {
	if v.PlainFlag || v.NoColor {
		return false
	}
	return v.Format == "tmux" || v.Format == "byobu"
}

func (v StatuslineView) green(s string, enabled bool) string {
	if !enabled {
		return s
	}
	return colorWrap(v.Format, colorGreen, s)
}

func (v StatuslineView) red(s string, enabled bool) string {
	if !enabled {
		return s
	}
	return colorWrap(v.Format, colorRed, s)
}

// writeStatuslineJSON emits the stable, always-uncolored JSON contract (§6).
func writeStatuslineJSON(w io.Writer, v StatuslineView) error {
	out := statusJSON{
		Status:      v.Status,
		Source:      "local_sqlite",
		Currency:    "USD",
		GeneratedAt: v.GeneratedAt.UTC().Format(time.RFC3339),
	}
	if v.Status == StatusOK {
		out.MTDCostUSD = v.Metrics.MTDCostUSD
		out.ForecastCostUSD = v.Metrics.ForecastUSD
		out.BudgetPercent = v.Metrics.BudgetPercent
		out.AnomalyCount = v.Metrics.AnomalyCount
	}
	if v.LastSyncAt != nil {
		at := v.LastSyncAt.UTC().Format(time.RFC3339)
		out.LastSyncAt = &at
		age := int(v.GeneratedAt.Sub(*v.LastSyncAt).Seconds())
		if age < 0 {
			age = 0
		}
		out.LastSyncAgeSeconds = &age
	}
	enc := json.NewEncoder(w)
	return enc.Encode(out)
}

// statusJSON is the on-the-wire shape of `statusline --format json`. Field
// names/types are a compatibility contract; evolution is additive only.
type statusJSON struct {
	Status             string   `json:"status"`
	Source             string   `json:"source"`
	Currency           string   `json:"currency"`
	MTDCostUSD         float64  `json:"mtd_cost_usd"`
	ForecastCostUSD    *float64 `json:"forecast_cost_usd"`
	BudgetPercent      *int     `json:"budget_percent"`
	AnomalyCount       int      `json:"anomaly_count"`
	LastSyncAt         *string  `json:"last_sync_at"`
	LastSyncAgeSeconds *int     `json:"last_sync_age_seconds"`
	GeneratedAt        string   `json:"generated_at"`
}
