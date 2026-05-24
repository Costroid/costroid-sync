package providers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	gcpTestProject  = "test-project"
	gcpTestTable    = "my-proj.billing_export_data.gcp_billing_export_v1_0123abcdef"
	gcpTestEmail    = "billing-reader@test-project.iam.gserviceaccount.com"
	gcpTestBearer   = "test-bearer-TOKEN_FAKE_GCP"
	gcpTestCurrency = "USD"
)

type gcpRecordedReq struct {
	Path          string
	Query         string
	Method        string
	Body          string
	Authorization string
	ContentType   string
}

type gcpBillingTestServer struct {
	server         *httptest.Server
	tokenHandler   http.HandlerFunc
	queryHandler   http.HandlerFunc
	getResHandler  http.HandlerFunc
	tokenCalls     int32
	queryCalls     int32
	getResCalls    int32
	capturedToken  []gcpRecordedReq
	capturedQuery  []gcpRecordedReq
	capturedGetRes []gcpRecordedReq
}

func newGCPBillingTestServer(t *testing.T) *gcpBillingTestServer {
	t.Helper()
	ts := &gcpBillingTestServer{}
	ts.tokenHandler = ts.defaultTokenHandler
	ts.queryHandler = ts.defaultQueryHandler
	ts.getResHandler = ts.defaultGetResHandler
	ts.server = httptest.NewServer(http.HandlerFunc(ts.dispatch))
	t.Cleanup(ts.server.Close)
	return ts
}

func (s *gcpBillingTestServer) dispatch(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	rec := gcpRecordedReq{
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Method:        r.Method,
		Body:          string(body),
		Authorization: r.Header.Get("Authorization"),
		ContentType:   r.Header.Get("Content-Type"),
	}
	switch {
	case r.URL.Path == "/token":
		atomic.AddInt32(&s.tokenCalls, 1)
		s.capturedToken = append(s.capturedToken, rec)
		s.tokenHandler(w, r)
	case strings.HasSuffix(r.URL.Path, "/queries") && r.Method == http.MethodPost:
		atomic.AddInt32(&s.queryCalls, 1)
		s.capturedQuery = append(s.capturedQuery, rec)
		s.queryHandler(w, r)
	case strings.Contains(r.URL.Path, "/queries/") && r.Method == http.MethodGet:
		atomic.AddInt32(&s.getResCalls, 1)
		s.capturedGetRes = append(s.capturedGetRes, rec)
		s.getResHandler(w, r)
	default:
		http.Error(w, "unhandled path: "+r.URL.Path, http.StatusNotFound)
	}
}

func (s *gcpBillingTestServer) defaultTokenHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"` + gcpTestBearer + `","token_type":"Bearer","expires_in":3600}`))
}

func (s *gcpBillingTestServer) defaultQueryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(gcpHappyPathQueryResponse))
}

func (s *gcpBillingTestServer) defaultGetResHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"jobComplete":true,"rows":[]}`))
}

// gcpHappyPathQueryResponse is a small but realistic BigQuery REST shape.
// Two rows of broad GCP billing — one Vertex AI character-billed row, one
// Cloud Run vCPU-second row — both USD.
const gcpHappyPathQueryResponse = `{
  "jobReference": {"projectId":"test-project","jobId":"job_abc","location":"US"},
  "jobComplete": true,
  "schema": {"fields": [
    {"name":"usage_start_time","type":"STRING"},
    {"name":"usage_end_time","type":"STRING"},
    {"name":"service_id","type":"STRING"},
    {"name":"service_description","type":"STRING"},
    {"name":"sku_id","type":"STRING"},
    {"name":"sku_description","type":"STRING"},
    {"name":"project_id","type":"STRING"},
    {"name":"project_name","type":"STRING"},
    {"name":"location_location","type":"STRING"},
    {"name":"cost","type":"STRING"},
    {"name":"currency","type":"STRING"},
    {"name":"usage_amount","type":"STRING"},
    {"name":"usage_unit","type":"STRING"},
    {"name":"invoice_month","type":"STRING"},
    {"name":"cost_type","type":"STRING"}
  ]},
  "rows": [
    {"f":[
      {"v":"2026-05-20 12:00:00 UTC"},
      {"v":"2026-05-20 13:00:00 UTC"},
      {"v":"6F81-5844-456A"},
      {"v":"Vertex AI"},
      {"v":"ABCD-EFGH-IJKL"},
      {"v":"Gemini 2.5 Flash characters"},
      {"v":"prod-project"},
      {"v":"Prod"},
      {"v":"us-central1"},
      {"v":"1.250000"},
      {"v":"USD"},
      {"v":"500000"},
      {"v":"characters"},
      {"v":"202605"},
      {"v":"regular"}
    ]},
    {"f":[
      {"v":"2026-05-20 13:00:00 UTC"},
      {"v":"2026-05-20 14:00:00 UTC"},
      {"v":"152E-C115-5142"},
      {"v":"Cloud Run"},
      {"v":"MNOP-QRST-UVWX"},
      {"v":"CPU Allocation Time"},
      {"v":"prod-project"},
      {"v":"Prod"},
      {"v":"us-central1"},
      {"v":"0.060000"},
      {"v":"USD"},
      {"v":"1800"},
      {"v":"seconds"},
      {"v":"202605"},
      {"v":"regular"}
    ]}
  ]
}`

// newGCPBillingTestProvider builds a provider wired up to ts that loads
// its key from a service-account JSON file written into t.TempDir().
func newGCPBillingTestProvider(t *testing.T, ts *gcpBillingTestServer) *GCPBillingProvider {
	t.Helper()
	saPath := writeTestServiceAccount(t, ts.server.URL+"/token")
	p := NewGCPBillingProvider(GCPBillingConfig{
		ServiceAccountJSONPath: saPath,
		BillingProject:         gcpTestProject,
		BillingTable:           gcpTestTable,
		Currency:               gcpTestCurrency,
	})
	p.TokenURL = ts.server.URL + "/token"
	p.BigQueryURL = ts.server.URL
	p.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	p.Now = func() time.Time {
		return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	}
	return p
}

// writeTestServiceAccount generates a fresh 2048-bit RSA key, writes a
// minimal service-account JSON file into a temp dir, and returns its
// path. The key never leaves the test process.
func writeTestServiceAccount(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	jsonBytes, err := json.Marshal(map[string]any{
		"type":         "service_account",
		"client_email": gcpTestEmail,
		"private_key":  string(pemBytes),
		"token_uri":    tokenURI,
	})
	if err != nil {
		t.Fatalf("marshal sa json: %v", err)
	}
	path := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(path, jsonBytes, 0o600); err != nil {
		t.Fatalf("write sa file: %v", err)
	}
	return path
}

// TestGCPBilling_HappyPath verifies an end-to-end Fetch:
// token exchange → BigQuery query → row decoding → metadata-only mapping.
func TestGCPBilling_HappyPath(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)

	records, err := p.Fetch(context.Background(), 7)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := len(records); got != 2 {
		t.Fatalf("want 2 records, got %d", got)
	}
	assertGCPHappyPathFirstRecord(t, records[0])

	if got := atomic.LoadInt32(&ts.queryCalls); got != 1 {
		t.Errorf("query endpoint called %d times, want 1", got)
	}
	if got := atomic.LoadInt32(&ts.getResCalls); got != 0 {
		t.Errorf("getResults called %d times, want 0", got)
	}
	if got := ts.capturedQuery[0].Authorization; got != "Bearer "+gcpTestBearer {
		t.Errorf("query Authorization = %q", got)
	}
}

func assertGCPHappyPathFirstRecord(t *testing.T, first NormalizedCostRecord) {
	t.Helper()
	if first.Provider != "gcp-billing" {
		t.Errorf("Provider = %q", first.Provider)
	}
	if first.RecordedAt != "2026-05-20T12:00:00Z" {
		t.Errorf("RecordedAt = %q", first.RecordedAt)
	}
	if first.CostUSD != 1.25 {
		t.Errorf("CostUSD = %v", first.CostUSD)
	}
	if first.Product != "Vertex AI" {
		t.Errorf("Product = %q", first.Product)
	}
	if first.SKU != "Gemini 2.5 Flash characters" {
		t.Errorf("SKU = %q", first.SKU)
	}
	if first.ProjectID != "prod-project" {
		t.Errorf("ProjectID = %q", first.ProjectID)
	}
	if first.Model != "" {
		t.Errorf("Model = %q (expected empty for broad gcp-billing)", first.Model)
	}
	if len(first.SourceHash) != 64 {
		t.Errorf("SourceHash unexpected len: %s", first.SourceHash)
	}
}

// TestGCPBilling_TableValidationBlocksFetch makes sure that an invalid
// table identifier prevents *any* HTTP call from being made — there is
// no scenario in which Fetch can issue a query against a non-validated
// table ID.
func TestGCPBilling_TableValidationBlocksFetch(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)
	p.BillingTable = "proj.ds.tbl; DROP TABLE x"

	_, err := p.Fetch(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error from invalid table id")
	}
	if !strings.Contains(err.Error(), "GCP_BILLING_TABLE") {
		t.Errorf("error missing env var name: %v", err)
	}
	if atomic.LoadInt32(&ts.tokenCalls)+atomic.LoadInt32(&ts.queryCalls) != 0 {
		t.Errorf("invalid table id triggered HTTP calls: token=%d query=%d",
			ts.tokenCalls, ts.queryCalls)
	}
}
