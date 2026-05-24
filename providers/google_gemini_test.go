package providers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const geminiTestProject = "my-proj"

func newGeminiTestProvider(t *testing.T, csv string, opts ...func(*GoogleGeminiProvider)) *GoogleGeminiProvider {
	t.Helper()
	path := filepath.Join(t.TempDir(), "billing.csv")
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	p := NewGoogleGeminiProvider(path)
	// Pin "now" to a fixed instant so DaysFilter tests are deterministic.
	p.Now = func() time.Time { return time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC) }
	for _, opt := range opts {
		opt(p)
	}
	return p
}

const geminiHappyCSV = `usage_start_time,service.description,sku.description,sku.id,project.id,cost,currency,usage.amount,usage.unit,usage_end_time,invoice.month,location.location,cost_type
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input tokens,gemini-2.5-flash-input,my-proj,1.25,USD,500000,tokens,2026-05-21T00:00:00Z,202605,us-central1,regular
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Pro Online Input Tokens,gemini-2.5-pro-input,my-proj,3.50,USD,200000,tokens,2026-05-21T00:00:00Z,202605,us-central1,regular
2026-05-21T00:00:00Z,Generative Language API,Gemini 2.5 Flash output tokens,gemini-2.5-flash-output,my-proj,2.50,USD,100000,tokens,2026-05-22T00:00:00Z,202605,us-central1,regular
`

func TestGoogleGemini_CSVHappyPath(t *testing.T) {
	p := newGeminiTestProvider(t, geminiHappyCSV)
	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("want 3 records, got %d (%+v)", len(records), records)
	}
	for _, r := range records {
		if r.Provider != "google-gemini" {
			t.Errorf("Provider = %q", r.Provider)
		}
		if r.ProjectID != geminiTestProject {
			t.Errorf("ProjectID = %q", r.ProjectID)
		}
		if r.APIKeyID != "" {
			t.Errorf("APIKeyID should be empty, got %q", r.APIKeyID)
		}
		if r.UnitType != "tokens" {
			t.Errorf("UnitType = %q, want tokens", r.UnitType)
		}
		if r.SourceHash == "" {
			t.Error("SourceHash empty")
		}
	}
	// Token unit gating: tokens unit + positive quantity → TotalTokens populated.
	for _, r := range records {
		if r.TotalTokens <= 0 {
			t.Errorf("TotalTokens not populated for token row: %+v", r)
		}
		if r.PromptTokens != 0 || r.CompletionTokens != 0 {
			t.Errorf("PromptTokens/CompletionTokens should be 0: %+v", r)
		}
	}
	// gpt-4.1-nano-isn't-in-pricing-but-doesn't-matter: model parsing.
	if records[0].Model != "gemini-2.5-flash" && records[0].Model != "gemini-2.5-pro" {
		t.Errorf("first record model unexpected: %q", records[0].Model)
	}
}

func TestGoogleGemini_NonGeminiRowsSkipped(t *testing.T) {
	csv := `usage_start_time,service.description,sku.description,sku.id,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Compute Engine,N1 Standard Instance Core,n1-cores,my-proj,5.00,USD,24,hour
2026-05-20T00:00:00Z,BigQuery,Analysis,bq-analysis,my-proj,3.00,USD,1000000000,byte
2026-05-20T00:00:00Z,Cloud Storage,Standard Storage,gcs-standard,my-proj,0.20,USD,10,gibibyte month
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input tokens,gemini-flash-input,my-proj,1.25,USD,500000,tokens
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 Gemini record, got %d (%+v)", len(records), records)
	}
	if records[0].SKU != "Gemini 2.5 Flash input tokens" {
		t.Errorf("unexpected SKU: %q", records[0].SKU)
	}
}

func TestGoogleGemini_RecognizesNewModelSlugs(t *testing.T) {
	cases := []struct {
		name, sku, want string
	}{
		{"canonical slug", "gemini-3.5-flash-lite", "gemini-3.5-flash-lite"},
		{"human readable flash", "Gemini 2.5 Flash", "gemini-2.5-flash"},
		{"human readable pro", "Gemini 1.5 Pro", "gemini-1.5-pro"},
		{"verbose flash with suffix words", "Gemini 2.5 Flash Online API Usage Input Tokens", "gemini-2.5-flash"},
		{"versionless pro", "Gemini Pro", "gemini-pro"},
		{"non-gemini row", "BigQuery Analysis", ""},
		{"gemini ultra preview", "Gemini 1.0 Ultra Preview", "gemini-1.0-ultra-preview"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractGeminiModel(c.sku)
			if got != c.want {
				t.Errorf("extractGeminiModel(%q) = %q, want %q", c.sku, got, c.want)
			}
		})
	}
}

func TestGoogleGemini_MissingOptionalColumns(t *testing.T) {
	// Only required columns plus sku.description (so the row passes filter)
	csv := `usage_start_time,cost,currency,sku.description
2026-05-20T00:00:00Z,1.00,USD,Gemini 2.5 Flash input tokens
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.CostUSD != 1.00 || r.Model != "gemini-2.5-flash" {
		t.Errorf("unexpected: %+v", r)
	}
	if r.Product != "Gemini API" {
		t.Errorf("Product fallback should be %q, got %q", "Gemini API", r.Product)
	}
	if r.UnitType != "" || r.UsageQuantity != 0 || r.ProjectID != "" {
		t.Errorf("optional fields not zero: %+v", r)
	}
}

func TestGoogleGemini_MissingRequiredColumnsError(t *testing.T) {
	// Missing cost
	csv := `usage_start_time,currency,sku.description
2026-05-20T00:00:00Z,USD,Gemini 2.5 Flash
`
	p := newGeminiTestProvider(t, csv)
	_, err := p.Fetch(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error for missing cost")
	}
	if !strings.Contains(err.Error(), "missing required columns") {
		t.Errorf("error wording wrong: %s", err)
	}
	if !strings.Contains(err.Error(), "cost") {
		t.Errorf("error should name cost: %s", err)
	}
	if strings.Contains(err.Error(), "USD,Gemini") {
		t.Errorf("error leaked row contents: %s", err)
	}
}

func TestGoogleGemini_NonUSDSkipped(t *testing.T) {
	csv := `usage_start_time,service.description,sku.description,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input tokens,my-proj,1.00,USD,500000,tokens
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Pro input tokens,my-proj,2.00,EUR,300000,tokens
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Pro output tokens,my-proj,3.00,JPY,100000,tokens
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 USD record, got %d (%+v)", len(records), records)
	}
	if records[0].CostUSD != 1.00 {
		t.Errorf("USD record cost = %v, want 1.00", records[0].CostUSD)
	}
}

func TestGoogleGemini_StripsProhibitedFields(t *testing.T) {
	// Include poison columns that should be ignored entirely.
	csv := `usage_start_time,service.description,sku.description,sku.id,project.id,cost,currency,usage.amount,usage.unit,prompt,completion,messages,content,tool_calls,raw_response,raw_payload,request_body,response_body,system_prompt,function_args,source_code,repository_content,labels,system_labels
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input tokens,gemini-2.5-flash-input,my-proj,1.25,USD,500000,tokens,POISON_PROMPT,POISON_COMPLETION,POISON_MESSAGES,POISON_CONTENT,POISON_TOOL,POISON_RAW_RESPONSE,POISON_RAW_PAYLOAD,POISON_REQUEST_BODY,POISON_RESPONSE_BODY,POISON_SYSTEM_PROMPT,POISON_FN_ARGS,POISON_SOURCE_CODE,POISON_REPO_CONTENT,user=POISON_LABEL,env=POISON_SYS_LABEL
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	js, err := json.Marshal(records[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(js)
	forbidden := []string{
		`"prompt"`, `"completion"`, `"messages"`, `"content"`, `"tool_calls"`,
		`"raw_response"`, `"raw_payload"`, `"request_body"`, `"response_body"`,
		`"system_prompt"`, `"function_args"`, `"source_code"`, `"repository_content"`,
		`"labels"`, `"system_labels"`,
		"POISON_PROMPT", "POISON_COMPLETION", "POISON_MESSAGES", "POISON_CONTENT",
		"POISON_TOOL", "POISON_RAW_RESPONSE", "POISON_RAW_PAYLOAD",
		"POISON_REQUEST_BODY", "POISON_RESPONSE_BODY", "POISON_SYSTEM_PROMPT",
		"POISON_FN_ARGS", "POISON_SOURCE_CODE", "POISON_REPO_CONTENT",
		"POISON_LABEL", "POISON_SYS_LABEL",
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q present in JSON: %s", bad, s)
		}
	}
	var back map[string]any
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	expected := map[string]bool{
		"provider": true, "model": true,
		"prompt_tokens": true, "completion_tokens": true, "total_tokens": true,
		"cost_usd": true, "recorded_at": true,
		"api_key_id": true, "project_id": true, "source_hash": true,
		"product": true, "sku": true, "unit_type": true,
		"usage_quantity": true, "unit_price_usd": true,
		"gross_amount_usd": true, "discount_amount_usd": true,
	}
	for k := range back {
		if !expected[k] {
			t.Errorf("unexpected JSON key %q in %s", k, s)
		}
	}
	if len(back) != len(expected) {
		t.Errorf("got %d keys, want %d", len(back), len(expected))
	}
}

func TestGeminiSourceHash_Deterministic(t *testing.T) {
	base := func() string {
		return geminiSourceHash(
			"2026-05-20T00:00:00Z", "2026-05-21T00:00:00Z", "202605",
			"USD",
			"my-proj",
			"sid", "svc",
			"skuid", "Gemini 2.5 Flash input",
			"us-central1",
			"regular",
			"500000", "1.25",
		)
	}
	a := base()
	b := base()
	if a != b {
		t.Fatalf("same inputs -> different hashes: %s vs %s", a, b)
	}
	// Different cost → different hash (intentional)
	diff := geminiSourceHash(
		"2026-05-20T00:00:00Z", "2026-05-21T00:00:00Z", "202605",
		"USD", "my-proj", "sid", "svc", "skuid", "Gemini 2.5 Flash input",
		"us-central1", "regular", "500000", "1.26",
	)
	if diff == a {
		t.Error("different cost should produce different hash")
	}
	// Different usage_amount → different hash
	diff2 := geminiSourceHash(
		"2026-05-20T00:00:00Z", "2026-05-21T00:00:00Z", "202605",
		"USD", "my-proj", "sid", "svc", "skuid", "Gemini 2.5 Flash input",
		"us-central1", "regular", "500001", "1.25",
	)
	if diff2 == a {
		t.Error("different usage_amount should produce different hash")
	}
	// Different location → different hash
	diff3 := geminiSourceHash(
		"2026-05-20T00:00:00Z", "2026-05-21T00:00:00Z", "202605",
		"USD", "my-proj", "sid", "svc", "skuid", "Gemini 2.5 Flash input",
		"us-east1", "regular", "500000", "1.25",
	)
	if diff3 == a {
		t.Error("different location should produce different hash")
	}
}

func TestGoogleGemini_FileMissingSafeError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.csv")
	p := NewGoogleGeminiProvider(missing)
	_, err := p.Fetch(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "open export") {
		t.Errorf("error wording wrong: %s", msg)
	}
	// Path may appear (it's the user's path, not credentials), but file contents must not (file doesn't exist anyway).
	if strings.Contains(msg, "POISON") {
		t.Errorf("error leaked: %s", msg)
	}
}

func TestGoogleGemini_NoEnvSafeError(t *testing.T) {
	p := NewGoogleGeminiProvider("")
	_, err := p.Fetch(context.Background(), 7)
	if !errors.Is(err, errGeminiNoExport) {
		t.Fatalf("want errGeminiNoExport, got %v", err)
	}
}

func TestGoogleGemini_ProjectFilterApplied(t *testing.T) {
	csv := `usage_start_time,service.description,sku.description,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input,proj-a,1.00,USD,1000,tokens
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input,proj-b,2.00,USD,2000,tokens
`
	p := newGeminiTestProvider(t, csv, func(p *GoogleGeminiProvider) {
		p.ProjectFilter = "proj-a"
	})
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 || records[0].ProjectID != "proj-a" {
		t.Errorf("project filter not applied: %+v", records)
	}
}

func TestGoogleGemini_ServiceFilterOverride(t *testing.T) {
	// Rows where neither default match would catch — but a custom override "vertex" should.
	csv := `usage_start_time,service.description,sku.description,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Vertex AI Platform,Custom model serving,my-proj,1.00,USD,1000,request
2026-05-20T00:00:00Z,Compute Engine,N1 Standard,my-proj,5.00,USD,24,hour
`
	p := newGeminiTestProvider(t, csv, func(p *GoogleGeminiProvider) {
		p.ServiceFilters = []string{"vertex"}
	})
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record (vertex), got %d (%+v)", len(records), records)
	}
	if !strings.Contains(strings.ToLower(records[0].Product), "vertex") {
		t.Errorf("unexpected product: %q", records[0].Product)
	}
	// TotalTokens should be 0 (unit is "request", not "tokens")
	if records[0].TotalTokens != 0 {
		t.Errorf("TotalTokens should be 0 for non-token unit, got %d", records[0].TotalTokens)
	}
	if records[0].UsageQuantity != 1000 {
		t.Errorf("UsageQuantity = %v, want 1000", records[0].UsageQuantity)
	}
}

func TestGoogleGemini_DaysFilter(t *testing.T) {
	// "now" is fixed at 2026-05-25 via newGeminiTestProvider.
	csv := `usage_start_time,service.description,sku.description,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input,p,1.00,USD,1000,tokens
2026-02-10T00:00:00Z,Generative Language API,Gemini 2.5 Flash input,p,5.00,USD,5000,tokens
`
	p := newGeminiTestProvider(t, csv)
	// days=10 → cutoff = 2026-05-15. Only the May 20 row survives.
	records, err := p.Fetch(context.Background(), 10)
	if err != nil {
		t.Fatalf("Fetch days=10: %v", err)
	}
	if len(records) != 1 || records[0].RecordedAt != "2026-05-20T00:00:00Z" {
		t.Errorf("days=10 filter wrong: %+v", records)
	}

	// days=10000 → clamps to 366. Both rows are within ~104 days of May 25.
	records, err = p.Fetch(context.Background(), 10000)
	if err != nil {
		t.Fatalf("Fetch days=10000: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("days=10000 (clamped to 366) should keep both: got %d", len(records))
	}

	// days=0 → clamps to 1. Only rows since 2026-05-24 survive — none in the fixture.
	records, err = p.Fetch(context.Background(), 0)
	if err != nil {
		t.Fatalf("Fetch days=0: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("days=0 (clamped to 1) should return 0 rows, got %d", len(records))
	}
}

func TestGoogleGemini_TokenUnitGating(t *testing.T) {
	csv := `usage_start_time,service.description,sku.description,project.id,cost,currency,usage.amount,usage.unit
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input,p,1.00,USD,1500,tokens
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash request,p,2.00,USD,42,requests
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	for _, r := range records {
		switch r.UnitType {
		case "tokens":
			if r.TotalTokens != 1500 {
				t.Errorf("tokens row TotalTokens = %d, want 1500", r.TotalTokens)
			}
			if r.UsageQuantity != 1500 {
				t.Errorf("tokens row UsageQuantity = %v, want 1500", r.UsageQuantity)
			}
		case "requests":
			if r.TotalTokens != 0 {
				t.Errorf("requests row TotalTokens = %d, want 0", r.TotalTokens)
			}
			if r.UsageQuantity != 42 {
				t.Errorf("requests row UsageQuantity = %v, want 42", r.UsageQuantity)
			}
		default:
			t.Errorf("unexpected unit: %q", r.UnitType)
		}
	}
}

func TestGoogleGemini_FlexibleHeaders(t *testing.T) {
	// Mix of header conventions: dotted, spaced (with capitals), underscored.
	csv := `Usage Start Time,Service Description,Sku Description,Project Id,Cost,Currency,Usage Amount,Usage Unit
2026-05-20T00:00:00Z,Generative Language API,Gemini 2.5 Flash input tokens,my-proj,1.25,USD,500000,tokens
`
	p := newGeminiTestProvider(t, csv)
	records, err := p.Fetch(context.Background(), 30)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d (%+v)", len(records), records)
	}
	r := records[0]
	if r.CostUSD != 1.25 || r.ProjectID != "my-proj" || r.Model != "gemini-2.5-flash" {
		t.Errorf("flexible header parsing broke fields: %+v", r)
	}
}

func TestGoogleGemini_NormalizeHeader(t *testing.T) {
	cases := []struct{ in, want string }{
		{"usage_start_time", "usage_start_time"},
		{"service.description", "service_description"},
		{"Service Description", "service_description"},
		{"  USAGE  AMOUNT  ", "usage_amount"},
		{"location.location", "location"},
		{"usage.unit", "usage_unit"},
	}
	for _, c := range cases {
		if got := normalizeHeader(c.in); got != c.want {
			t.Errorf("normalizeHeader(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGoogleGemini_ParseServiceFilters(t *testing.T) {
	got := parseServiceFilters("gemini, vertex ,, GenerativeAI")
	want := []string{"gemini", "vertex", "generativeai"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
