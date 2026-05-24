package providers

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func sampleGCPBillingRow() gcpBillingRow {
	return gcpBillingRow{
		UsageStartTime:     "2026-05-20T12:00:00Z",
		UsageEndTime:       "2026-05-20T13:00:00Z",
		ServiceID:          "6F81-5844-456A",
		ServiceDescription: "Vertex AI",
		SKUID:              "ABCD-EFGH-IJKL",
		SKUDescription:     "Gemini 2.5 Flash characters",
		ProjectID:          "prod-project",
		ProjectName:        "Prod",
		Location:           "us-central1",
		Cost:               1.25,
		Currency:           "USD",
		UsageAmount:        500000,
		UsageUnit:          "characters",
		InvoiceMonth:       "202605",
		CostType:           "regular",
		RawUsageAmount:     "500000",
		RawCost:            "1.25",
	}
}

func TestMapGCPBillingRow_Basic(t *testing.T) {
	row := sampleGCPBillingRow()
	rec := mapGCPBillingRow(row, gcpTestTable)

	if rec.Provider != "gcp-billing" {
		t.Errorf("Provider = %q", rec.Provider)
	}
	if rec.Model != "" {
		t.Errorf("Model = %q, want empty for broad gcp-billing", rec.Model)
	}
	if rec.RecordedAt != "2026-05-20T12:00:00Z" {
		t.Errorf("RecordedAt = %q", rec.RecordedAt)
	}
	if rec.CostUSD != 1.25 || rec.GrossAmountUSD != 1.25 {
		t.Errorf("Cost/Gross = %v / %v", rec.CostUSD, rec.GrossAmountUSD)
	}
	if rec.DiscountAmountUSD != 0 {
		t.Errorf("DiscountAmountUSD = %v, want 0 (deferred)", rec.DiscountAmountUSD)
	}
	if rec.UnitPriceUSD != 0 {
		t.Errorf("UnitPriceUSD = %v, want 0 (not present in export)", rec.UnitPriceUSD)
	}
	if rec.ProjectID != "prod-project" {
		t.Errorf("ProjectID = %q", rec.ProjectID)
	}
	if rec.Product != "Vertex AI" {
		t.Errorf("Product = %q", rec.Product)
	}
	if rec.SKU != "Gemini 2.5 Flash characters" {
		t.Errorf("SKU = %q (want sku_description preferred)", rec.SKU)
	}
	if rec.UnitType != "characters" {
		t.Errorf("UnitType = %q", rec.UnitType)
	}
	if rec.UsageQuantity != 500000 {
		t.Errorf("UsageQuantity = %v", rec.UsageQuantity)
	}
	if rec.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 (characters, not tokens)", rec.TotalTokens)
	}
	if rec.PromptTokens != 0 || rec.CompletionTokens != 0 {
		t.Errorf("prompt/completion tokens must always be 0")
	}
	if len(rec.SourceHash) != 64 {
		t.Errorf("SourceHash bad length: %s", rec.SourceHash)
	}
}

func TestMapGCPBillingRow_TokenUnit(t *testing.T) {
	row := sampleGCPBillingRow()
	row.UsageUnit = "tokens"
	row.UsageAmount = 12345
	row.RawUsageAmount = "12345"
	rec := mapGCPBillingRow(row, gcpTestTable)
	if rec.TotalTokens != 12345 {
		t.Errorf("TotalTokens = %d, want 12345", rec.TotalTokens)
	}
}

func TestMapGCPBillingRow_TokenUnitZeroAmount(t *testing.T) {
	row := sampleGCPBillingRow()
	row.UsageUnit = "tokens"
	row.UsageAmount = 0
	row.RawUsageAmount = "0"
	rec := mapGCPBillingRow(row, gcpTestTable)
	if rec.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 for zero amount", rec.TotalTokens)
	}
}

func TestMapGCPBillingRow_SKUFallback(t *testing.T) {
	row := sampleGCPBillingRow()
	row.SKUDescription = ""
	rec := mapGCPBillingRow(row, gcpTestTable)
	if rec.SKU != "ABCD-EFGH-IJKL" {
		t.Errorf("SKU = %q, want sku_id fallback", rec.SKU)
	}
}

func TestMapGCPBillingRow_SourceHashDeterministic(t *testing.T) {
	row := sampleGCPBillingRow()
	a := mapGCPBillingRow(row, gcpTestTable)
	b := mapGCPBillingRow(row, gcpTestTable)
	if a.SourceHash != b.SourceHash {
		t.Errorf("SourceHash not deterministic: %s vs %s", a.SourceHash, b.SourceHash)
	}

	// Change one identity field — hash must change.
	row2 := sampleGCPBillingRow()
	row2.RawCost = "2.50"
	c := mapGCPBillingRow(row2, gcpTestTable)
	if a.SourceHash == c.SourceHash {
		t.Errorf("SourceHash collision across distinct cost values")
	}

	// Different billing table — hash must change.
	d := mapGCPBillingRow(row, "other-proj.ds.tbl")
	if a.SourceHash == d.SourceHash {
		t.Errorf("SourceHash collision across distinct billing tables")
	}
}

// ---------- Currency filter / project filter (end-to-end through query layer) ----------

func TestGCPBilling_CurrencyFilterParameter(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	p.Currency = "EUR"

	// We don't care about response contents for this assertion — only that
	// the currency parameter went out as EUR.
	ts.queryHandler = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jobComplete":true,"rows":[]}`))
	}
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	body := ts.capturedQuery[0].Body
	if !strings.Contains(body, `"value":"EUR"`) {
		t.Errorf("query body missing currency=EUR parameter: %s", body)
	}
}

func TestGCPBilling_ServiceFilterClientSide(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	p.ServiceFilters = []string{"vertex ai"}

	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Happy-path fixture has Vertex AI + Cloud Run; only Vertex AI should
	// survive the client-side filter.
	if len(records) != 1 {
		t.Fatalf("want 1 record after filter, got %d", len(records))
	}
	if records[0].Product != "Vertex AI" {
		t.Errorf("kept wrong record: %q", records[0].Product)
	}

	// Service filter must NOT be interpolated into the SQL.
	body := ts.capturedQuery[0].Body
	if strings.Contains(strings.ToLower(body), "vertex ai") {
		t.Errorf("service filter leaked into SQL body: %s", body)
	}
}

func TestGCPBilling_DefaultNoServiceFilter(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	// No ServiceFilters set — broad import.
	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records (both pass), got %d", len(records))
	}
}

// ---------- Missing env vars ----------

func TestGCPBilling_MissingTableErrorMentionsEnvVar(t *testing.T) {
	p := NewGCPBillingProvider(GCPBillingConfig{
		ServiceAccountJSONPath: "/dev/null",
		BillingProject:         "proj",
		BillingTable:           "",
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("want error for missing table")
	}
	if !strings.Contains(err.Error(), "GCP_BILLING_TABLE") {
		t.Errorf("error missing env var name: %v", err)
	}
}

func TestGCPBilling_MissingProjectErrorMentionsEnvVar(t *testing.T) {
	p := NewGCPBillingProvider(GCPBillingConfig{
		ServiceAccountJSONPath: "/dev/null",
		BillingProject:         "",
		BillingTable:           gcpTestTable,
	})
	_, err := p.Fetch(context.Background(), 1)
	if err == nil {
		t.Fatal("want error for missing project")
	}
	if !strings.Contains(err.Error(), "GCP_BILLING_PROJECT") {
		t.Errorf("error missing env var name: %v", err)
	}
}

// Static check we never emit an http import-only test file.
var _ = http.StatusOK
