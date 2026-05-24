package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// ---------- typed row (metadata-only by construction) ----------

// gcpBillingRow holds the safe billing-metadata projection from a single
// BigQuery row. No labels, system_labels, credits, adjustment_info, or
// any other field that could carry user-supplied free-form text. The
// query in gcp_billing_query.go selects exactly these columns; adding a
// field here without updating the SELECT (and the security test) is a
// review blocker.
type gcpBillingRow struct {
	UsageStartTime     string // RFC3339 UTC
	UsageEndTime       string // RFC3339 UTC; informational
	ServiceID          string
	ServiceDescription string
	SKUID              string
	SKUDescription     string
	ProjectID          string
	ProjectName        string
	Location           string
	Cost               float64
	Currency           string
	UsageAmount        float64
	UsageUnit          string
	InvoiceMonth       string // "YYYYMM"
	CostType           string

	// Raw strings retained ONLY for SourceHash stability across re-imports.
	RawUsageAmount string
	RawCost        string
}

// ---------- filter ----------

func rowMatchesGCPServiceFilters(row gcpBillingRow, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	service := strings.ToLower(row.ServiceDescription)
	sku := strings.ToLower(row.SKUDescription)
	for _, sub := range filters {
		if sub == "" {
			continue
		}
		if strings.Contains(service, sub) || strings.Contains(sku, sub) {
			return true
		}
	}
	return false
}

func lowerCopy(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---------- mapping ----------

func mapGCPBillingRow(row gcpBillingRow, billingTable string) NormalizedCostRecord {
	skuKey := firstNonEmpty(row.SKUDescription, row.SKUID)
	totalTokens := 0
	if strings.Contains(strings.ToLower(row.UsageUnit), "token") && row.UsageAmount > 0 {
		totalTokens = int(row.UsageAmount)
	}
	return NormalizedCostRecord{
		Provider:          gcpBillingProviderName,
		Model:             "",
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       totalTokens,
		CostUSD:           row.Cost,
		RecordedAt:        row.UsageStartTime,
		APIKeyID:          "",
		ProjectID:         row.ProjectID,
		Product:           row.ServiceDescription,
		SKU:               skuKey,
		UnitType:          row.UsageUnit,
		UsageQuantity:     row.UsageAmount,
		UnitPriceUSD:      0,
		GrossAmountUSD:    row.Cost,
		DiscountAmountUSD: 0,
		SourceHash: gcpBillingSourceHash(
			row.UsageStartTime, row.UsageEndTime,
			billingTable, row.ProjectID,
			row.ServiceID, row.ServiceDescription,
			row.SKUID, row.SKUDescription,
			row.Currency, row.InvoiceMonth, row.CostType,
			row.RawUsageAmount, row.RawCost,
		),
	}
}

// gcpBillingSourceHash mirrors geminiSourceHash: includes raw usage and
// cost strings as identity components so that line-level rows with
// matching identity tuples but distinct amounts don't collapse on
// UPSERT. Re-importing the same BigQuery result is fully idempotent.
// billingTable is included so reconfiguring GCP_BILLING_TABLE doesn't
// collide with previously imported rows.
func gcpBillingSourceHash(
	usageStart, usageEnd string,
	billingTable string,
	projectID string,
	serviceID, serviceDesc string,
	skuID, skuDesc string,
	currency, invoiceMonth, costType string,
	usageAmount, cost string,
) string {
	h := sha256.New()
	fmt.Fprintf(h, "gcp-billing|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		usageStart, usageEnd,
		billingTable,
		projectID,
		serviceID, serviceDesc,
		skuID, skuDesc,
		currency, invoiceMonth, costType,
		usageAmount, cost,
	)
	return hex.EncodeToString(h.Sum(nil))
}

func sortGCPBillingRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].ProjectID != records[j].ProjectID {
			return records[i].ProjectID < records[j].ProjectID
		}
		if records[i].Product != records[j].Product {
			return records[i].Product < records[j].Product
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
