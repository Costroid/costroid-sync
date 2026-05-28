package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/costroid/costroid-sync/providers"
)

const testAgentKey = "csk_TEST_AGENT_KEY_DO_NOT_PRINT"

func TestPushRecordsSendsWireContract(t *testing.T) {
	var bodyText string
	server := newWireContractServer(t, &bodyText)
	defer server.Close()

	record := testCloudRecord("github-copilot", "hash-1")
	if err := PushRecords(context.Background(), server.URL+"/", "org-123", testAgentKey, []providers.NormalizedCostRecord{record}); err != nil {
		t.Fatalf("PushRecords: %v", err)
	}
	if record.Provider != "github-copilot" {
		t.Fatalf("PushRecords mutated original record provider to %q", record.Provider)
	}
	assertRecordsOnlyBody(t, bodyText)

	parsed := parsePushBody(t, bodyText)
	if len(parsed.Records) != 1 {
		t.Fatalf("records length = %d, want 1", len(parsed.Records))
	}
	if got := parsed.Records[0].Provider; got != "github_copilot" {
		t.Fatalf("provider = %q, want github_copilot", got)
	}
}

func newWireContractServer(t *testing.T, bodyText *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertWireRequest(t, r)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		*bodyText = string(body)
		w.WriteHeader(http.StatusOK)
	}))
}

func assertWireRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.URL.Path != "/api/orgs/org-123/agent-sync" {
		t.Errorf("path = %s", r.URL.Path)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer "+testAgentKey {
		t.Errorf("authorization header = %q", got)
	}
	if got := r.Header.Get("X-Costroid-Wire-Version"); got != "1" {
		t.Errorf("wire version = %q", got)
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
}

func assertRecordsOnlyBody(t *testing.T, bodyText string) {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bodyText), &payload); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("top-level keys = %v, want records only", keys(payload))
	}
	if _, ok := payload["records"]; !ok {
		t.Fatal("request body missing records")
	}
	if strings.Contains(bodyText, `"agent_key"`) {
		t.Fatalf("request body leaked agent_key field: %s", bodyText)
	}
	for _, forbidden := range []string{`"prompt"`, `"completion"`, `"messages"`, `"content"`} {
		if strings.Contains(bodyText, forbidden) {
			t.Fatalf("request body contains forbidden field %s: %s", forbidden, bodyText)
		}
	}
}

func parsePushBody(t *testing.T, bodyText string) pushRequest {
	t.Helper()
	var parsed pushRequest
	if err := json.Unmarshal([]byte(bodyText), &parsed); err != nil {
		t.Fatalf("unmarshal records: %v", err)
	}
	return parsed
}

func TestPushRecordsChunksBatches(t *testing.T) {
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var parsed struct {
			Records []providers.NormalizedCostRecord `json:"records"`
		}
		if err := json.NewDecoder(r.Body).Decode(&parsed); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		batchSizes = append(batchSizes, len(parsed.Records))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	records := make([]providers.NormalizedCostRecord, 1001)
	for i := range records {
		records[i] = testCloudRecord("openai", fmt.Sprintf("hash-%d", i))
	}

	if err := PushRecords(context.Background(), server.URL, "org-123", testAgentKey, records); err != nil {
		t.Fatalf("PushRecords: %v", err)
	}
	if len(batchSizes) != 2 || batchSizes[0] != 1000 || batchSizes[1] != 1 {
		t.Fatalf("batch sizes = %v, want [1000 1]", batchSizes)
	}
}

func TestWireProviderSlugMapsCloudEnumValues(t *testing.T) {
	for input, want := range map[string]string{
		"openai":         "openai",
		"anthropic":      "anthropic",
		"github-copilot": "github_copilot",
		"google-gemini":  "google_gemini",
		"gcp-billing":    "gcp_billing",
		"azure-openai":   "azure_openai",
		"aws-bedrock":    "aws_bedrock",
	} {
		if got := wireProviderSlug(input); got != want {
			t.Fatalf("wireProviderSlug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWireRecordsUsesMetadataFallbackForEmptyModel(t *testing.T) {
	record := testCloudRecord("gcp-billing", "hash-1")
	record.Model = ""
	record.SKU = "Gemini API input tokens"
	record.Product = "Vertex AI"

	wire := wireRecords([]providers.NormalizedCostRecord{record})
	if got := wire[0].Model; got != "Gemini API input tokens" {
		t.Fatalf("wire model = %q, want SKU fallback", got)
	}
	if record.Model != "" {
		t.Fatalf("wireRecords mutated original model to %q", record.Model)
	}
}

func TestPushRecordsSafeStatusErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "validation", statusCode: http.StatusBadRequest, want: "metadata payload"},
		{name: "bad key", statusCode: http.StatusUnauthorized, want: "agent key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "raw body with "+testAgentKey, tc.statusCode)
			}))
			defer server.Close()

			err := PushRecords(context.Background(), server.URL, "org-123", testAgentKey, []providers.NormalizedCostRecord{testCloudRecord("openai", "hash-1")})
			if err == nil {
				t.Fatal("expected error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error = %q, want it to mention %q", msg, tc.want)
			}
			assertSafeCloudError(t, msg)
		})
	}
}

func TestPushRecordsNetworkFailureSafeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("closed server should not receive requests")
	}))
	baseURL := server.URL
	server.Close()

	err := PushRecords(context.Background(), baseURL, "org-123", testAgentKey, []providers.NormalizedCostRecord{testCloudRecord("openai", "hash-1")})
	if err == nil {
		t.Fatal("expected network error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "network error") {
		t.Fatalf("error = %q, want network error", msg)
	}
	assertSafeCloudError(t, msg)
}

func assertSafeCloudError(t *testing.T, msg string) {
	t.Helper()
	if strings.Contains(msg, testAgentKey) || strings.Contains(msg, "raw body") {
		t.Fatalf("unsafe cloud error: %s", msg)
	}
}

func testCloudRecord(provider, sourceHash string) providers.NormalizedCostRecord {
	return providers.NormalizedCostRecord{
		Provider:         provider,
		Model:            "gpt-5",
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		CostUSD:          0.01,
		RecordedAt:       "2026-05-28T00:00:00Z",
		APIKeyID:         "key-1",
		ProjectID:        "project-1",
		SourceHash:       sourceHash,
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
