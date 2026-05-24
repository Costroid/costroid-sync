package providers

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	googleGeminiProviderName      = "google-gemini"
	envGeminiBillingExport        = "GEMINI_BILLING_EXPORT"
	envGeminiBillingProject       = "GEMINI_BILLING_PROJECT"
	envGeminiBillingServiceFilter = "GEMINI_BILLING_SERVICE_FILTER"
	googleGeminiMaxDays           = 366
	googleGeminiCtxCheckInterval  = 100
)

// defaultGeminiServiceFilters are the lowercased service-name substrings
// that, when found in a row's service.description, mark the row as
// Gemini-related. Conservative on purpose: avoid pulling in non-Gemini
// Vertex AI or generic "Generative AI" rows. Users can broaden via
// GEMINI_BILLING_SERVICE_FILTER.
var defaultGeminiServiceFilters = []string{"gemini", "generative language"}

// geminiModelRegex captures the canonical Gemini model slug from a
// SKU/product string. It accepts both the slug form ("gemini-2.5-flash")
// and Google's human-readable form ("Gemini 2.5 Flash") after lowercase
// + whitespace-to-dash normalization. Optional version, required tier,
// optional suffix.
var geminiModelRegex = regexp.MustCompile(`gemini(?:-\d+(?:\.\d+)*)?-(?:flash|pro|nano|ultra|exp)(?:-(?:lite|mini|preview))?`)

// errGeminiNoExport is returned when the user invokes the provider
// without setting GEMINI_BILLING_EXPORT. The message is safe to print
// verbatim — no file contents, no credentials.
var errGeminiNoExport = errors.New(
	"GEMINI_BILLING_EXPORT not set. Google Gemini uses Cloud Billing\n" +
		"export files rather than a live API. Export Cloud Billing data\n" +
		"from BigQuery as CSV, then:\n" +
		"  export GEMINI_BILLING_EXPORT=/path/to/google-billing-export.csv")

// GoogleGeminiProvider reads metadata-only Gemini billing rows from a
// pre-exported Google Cloud Billing CSV file. It NEVER calls Google
// APIs, NEVER reads Gemini prompts/completions/chat content/source code,
// and NEVER stores `labels` or `system_labels` columns (which can carry
// free-form user text).
type GoogleGeminiProvider struct {
	ExportPath     string
	ProjectFilter  string   // empty => no project filter
	ServiceFilters []string // lowercased substrings; empty => defaultGeminiServiceFilters
	Now            func() time.Time
}

var _ Provider = (*GoogleGeminiProvider)(nil)

func NewGoogleGeminiProvider(path string) *GoogleGeminiProvider {
	return &GoogleGeminiProvider{
		ExportPath: path,
		Now:        func() time.Time { return time.Now().UTC() },
	}
}

func (p *GoogleGeminiProvider) Name() string { return googleGeminiProviderName }

func (p *GoogleGeminiProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	if p.ExportPath == "" {
		return nil, errGeminiNoExport
	}
	f, err := os.Open(p.ExportPath)
	if err != nil {
		// os.PathError carries the path (intentional — useful) but never
		// the file contents (file may not even be readable).
		return nil, fmt.Errorf("google-gemini: open export: %w", err)
	}
	defer f.Close()
	return p.parseCSV(ctx, f, days)
}

// parseCSV reads the export CSV from r and returns Gemini-only records.
// The function never reads `labels` or `system_labels` columns even if
// they appear in the header — they are not added to the column index.
func (p *GoogleGeminiProvider) parseCSV(ctx context.Context, r io.Reader, days int) ([]NormalizedCostRecord, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("google-gemini: read header: %w", err)
	}

	colIndex := buildHeaderIndex(header)
	if missing := requiredColumnsMissing(colIndex); len(missing) > 0 {
		return nil, fmt.Errorf("google-gemini: missing required columns: %s", strings.Join(missing, ", "))
	}

	cutoff := p.computeCutoff(days)
	filters := p.activeServiceFilters()

	var out []NormalizedCostRecord
	rowNum := 0
	for {
		rowNum++
		if rowNum%googleGeminiCtxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("google-gemini: %w", err)
			}
		}

		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("google-gemini: read row %d: %w", rowNum, err)
		}

		fields := buildRowMap(row, colIndex)

		if p.ProjectFilter != "" && fields["project_id"] != p.ProjectFilter {
			continue
		}
		if !rowMatchesGemini(fields, filters) {
			continue
		}
		recordedAt, ok := parseUsageStart(fields["usage_start_time"])
		if !ok {
			continue
		}
		if !cutoff.IsZero() && recordedAt.Before(cutoff) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(fields["currency"]), "usd") {
			continue
		}
		usdCost, ok := parseFloat(fields["cost"])
		if !ok {
			continue
		}

		out = append(out, mapGeminiRow(fields, recordedAt, usdCost))
	}

	sortGoogleGeminiRecords(out)
	return out, nil
}

func (p *GoogleGeminiProvider) computeCutoff(days int) time.Time {
	if days < 1 {
		days = 1
	}
	if days > googleGeminiMaxDays {
		days = googleGeminiMaxDays
	}
	now := p.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return now().Add(-time.Duration(days) * 24 * time.Hour)
}

func (p *GoogleGeminiProvider) activeServiceFilters() []string {
	if len(p.ServiceFilters) > 0 {
		return p.ServiceFilters
	}
	return defaultGeminiServiceFilters
}

// requiredGeminiColumns are the absolute minimum CSV columns; missing
// any of them means the file isn't a recognizable Cloud Billing export.
var requiredGeminiColumns = []string{"usage_start_time", "cost", "currency"}

// indexedGeminiColumns are the columns the parser tracks. Everything
// else in the header (including labels, system_labels, project.name,
// invoice.month details we don't store, etc.) is skipped.
var indexedGeminiColumns = map[string]bool{
	"usage_start_time":    true,
	"usage_end_time":      true,
	"invoice_month":       true,
	"service_description": true,
	"service_id":          true,
	"sku_description":     true,
	"sku_id":              true,
	"project_id":          true,
	"cost":                true,
	"currency":            true,
	"usage_amount":        true,
	"usage_unit":          true,
	"location":            true,
	"cost_type":           true,
}

func buildHeaderIndex(header []string) map[string]int {
	idx := map[string]int{}
	for i, raw := range header {
		normalized := normalizeHeader(raw)
		if !indexedGeminiColumns[normalized] {
			continue
		}
		idx[normalized] = i
	}
	return idx
}

func requiredColumnsMissing(colIndex map[string]int) []string {
	var missing []string
	for _, c := range requiredGeminiColumns {
		if _, ok := colIndex[c]; !ok {
			missing = append(missing, c)
		}
	}
	return missing
}

// normalizeHeader lowercases the header and collapses dot-or-whitespace
// runs into single underscores. Examples:
//
//	"service.description"      -> "service_description"
//	"Service Description"      -> "service_description"
//	"location.location"        -> "location_location"  (special-cased below)
//	"usage.amount"             -> "usage_amount"
func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	var b strings.Builder
	prevSep := false
	for _, r := range h {
		if r == '.' || r == ' ' || r == '\t' || r == '_' {
			if !prevSep && b.Len() > 0 {
				b.WriteByte('_')
				prevSep = true
			}
			continue
		}
		b.WriteRune(r)
		prevSep = false
	}
	out := strings.TrimRight(b.String(), "_")
	// BigQuery emits "location.location" → normalized "location_location";
	// most callers refer to it simply as "location". Accept both.
	if out == "location_location" {
		return "location"
	}
	return out
}

func buildRowMap(row []string, colIndex map[string]int) map[string]string {
	fields := map[string]string{}
	for name, i := range colIndex {
		if i < 0 || i >= len(row) {
			fields[name] = ""
			continue
		}
		fields[name] = row[i]
	}
	return fields
}

// rowMatchesGemini returns true if a row's service or sku indicates a
// Gemini-related billing line. Conservative: requires a positive match.
func rowMatchesGemini(fields map[string]string, serviceFilters []string) bool {
	service := strings.ToLower(fields["service_description"])
	for _, sub := range serviceFilters {
		if sub != "" && strings.Contains(service, sub) {
			return true
		}
	}
	sku := strings.ToLower(fields["sku_description"])
	for _, sub := range []string{"gemini", "generative language"} {
		if strings.Contains(sku, sub) {
			return true
		}
	}
	if extractGeminiModel(fields["sku_description"]) != "" {
		return true
	}
	return false
}

// extractGeminiModel parses a Gemini model slug from a SKU/product
// description. Accepts both canonical and human-readable forms.
// Returns "" if no recognizable Gemini model is found.
func extractGeminiModel(s string) string {
	if s == "" {
		return ""
	}
	normalized := strings.ToLower(s)
	// Collapse whitespace runs into single dashes so "Gemini 2.5 Flash"
	// becomes "gemini-2.5-flash" before regex matching.
	var b strings.Builder
	prevSpace := false
	for _, r := range normalized {
		if r == ' ' || r == '\t' {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte('-')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	normalized = b.String()
	return geminiModelRegex.FindString(normalized)
}

func parseUsageStart(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.000000 MST",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func parseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func mapGeminiRow(fields map[string]string, recordedAt time.Time, usdCost float64) NormalizedCostRecord {
	skuKey := firstNonEmpty(fields["sku_description"], fields["sku_id"])
	quantity, _ := parseFloat(fields["usage_amount"])
	unit := fields["usage_unit"]

	totalTokens := 0
	if strings.Contains(strings.ToLower(unit), "token") && quantity > 0 {
		totalTokens = int(quantity)
	}

	model := extractGeminiModel(fields["sku_description"])
	product := productOrDefault(fields["service_description"])

	recordedAtStr := recordedAt.UTC().Format(time.RFC3339)

	return NormalizedCostRecord{
		Provider:          googleGeminiProviderName,
		Model:             model,
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       totalTokens,
		CostUSD:           usdCost,
		RecordedAt:        recordedAtStr,
		APIKeyID:          "",
		ProjectID:         fields["project_id"],
		Product:           product,
		SKU:               skuKey,
		UnitType:          unit,
		UsageQuantity:     quantity,
		UnitPriceUSD:      0,
		GrossAmountUSD:    usdCost,
		DiscountAmountUSD: 0,
		SourceHash: geminiSourceHash(
			fields["usage_start_time"], fields["usage_end_time"], fields["invoice_month"],
			fields["currency"],
			fields["project_id"],
			fields["service_id"], fields["service_description"],
			fields["sku_id"], fields["sku_description"],
			fields["location"],
			fields["cost_type"],
			fields["usage_amount"], fields["cost"],
		),
	}
}

func productOrDefault(serviceDesc string) string {
	if serviceDesc != "" {
		return serviceDesc
	}
	return "Gemini API"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// geminiSourceHash is the per-provider identity hash. UNLIKE other
// providers, this hash includes the volatile usage_amount and cost
// strings — Cloud Billing CSV is line-level and the same
// (date, project, sku) tuple can have multiple distinct rows for
// different cost components, taxes, credits, etc. Excluding the
// volatile fields would collapse them into one bucket on UPSERT.
// Trade-off: a corrected re-issue with new amount/cost will appear as
// a new row rather than updating the existing one. Re-importing the
// same unchanged file is fully idempotent.
//
// Inputs are the raw CSV strings (not parsed floats) so the hash is
// bit-stable across re-imports.
func geminiSourceHash(
	usageStart, usageEnd, invoiceMonth string,
	currency string,
	projectID string,
	serviceID, serviceDesc string,
	skuID, skuDesc string,
	location string,
	costType string,
	usageAmount, cost string,
) string {
	h := sha256.New()
	fmt.Fprintf(h, "google-gemini|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		usageStart, usageEnd, invoiceMonth,
		currency,
		projectID,
		serviceID, serviceDesc,
		skuID, skuDesc,
		location,
		costType,
		usageAmount, cost,
	)
	return hex.EncodeToString(h.Sum(nil))
}

// parseServiceFilters splits a comma-separated env-var value into a
// lower-cased slice of substrings. Empty entries are dropped.
func parseServiceFilters(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sortGoogleGeminiRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].ProjectID != records[j].ProjectID {
			return records[i].ProjectID < records[j].ProjectID
		}
		if records[i].SKU != records[j].SKU {
			return records[i].SKU < records[j].SKU
		}
		if records[i].UsageQuantity != records[j].UsageQuantity {
			return records[i].UsageQuantity < records[j].UsageQuantity
		}
		return records[i].CostUSD < records[j].CostUSD
	})
}
