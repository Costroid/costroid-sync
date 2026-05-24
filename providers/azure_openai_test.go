package providers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	azureTestTenant       = "test-tenant"
	azureTestClientID     = "test-client"
	azureTestClientSecret = "test-secret-FAKE"
	azureTestSubscription = "11111111-2222-3333-4444-555555555555"
	azureTestBearer       = "test-bearer-TOKEN_FAKE"
	azureTestResource     = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.CognitiveServices/accounts/myacct"
)

type azureRecordedReq struct {
	Path          string
	Query         string
	Method        string
	Body          string
	Authorization string
	ContentType   string
}

type azureTestServer struct {
	server          *httptest.Server
	tokenHandler    http.HandlerFunc
	costHandler     http.HandlerFunc
	monitorHandler  http.HandlerFunc
	tokenCalls      int32
	costCalls       int32
	monitorCalls    int32
	capturedToken   []azureRecordedReq
	capturedCost    []azureRecordedReq
	capturedMonitor []azureRecordedReq
}

func newAzureTestServer(t *testing.T) *azureTestServer {
	t.Helper()
	ts := &azureTestServer{}
	ts.tokenHandler = ts.defaultTokenHandler
	ts.costHandler = ts.defaultCostHandler
	ts.monitorHandler = ts.defaultMonitorHandler
	ts.server = httptest.NewServer(http.HandlerFunc(ts.dispatch))
	t.Cleanup(ts.server.Close)
	return ts
}

func (s *azureTestServer) dispatch(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	rec := azureRecordedReq{
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Method:        r.Method,
		Body:          string(body),
		Authorization: r.Header.Get("Authorization"),
		ContentType:   r.Header.Get("Content-Type"),
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token"):
		atomic.AddInt32(&s.tokenCalls, 1)
		s.capturedToken = append(s.capturedToken, rec)
		s.tokenHandler(w, r)
	case strings.Contains(r.URL.Path, "/Microsoft.CostManagement/query"):
		atomic.AddInt32(&s.costCalls, 1)
		s.capturedCost = append(s.capturedCost, rec)
		s.costHandler(w, r)
	case strings.Contains(r.URL.Path, "/Microsoft.Insights/metrics"):
		atomic.AddInt32(&s.monitorCalls, 1)
		s.capturedMonitor = append(s.capturedMonitor, rec)
		s.monitorHandler(w, r)
	default:
		http.Error(w, "unhandled path: "+r.URL.Path, http.StatusNotFound)
	}
}

func (s *azureTestServer) defaultTokenHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"access_token":"` + azureTestBearer + `","token_type":"Bearer","expires_in":3599}`))
}

func (s *azureTestServer) defaultCostHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(azureCostHappyResponse))
}

func (s *azureTestServer) defaultMonitorHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"value":[]}`))
}

func newAzureTestProvider(t *testing.T, ts *azureTestServer) *AzureOpenAIProvider {
	t.Helper()
	p := NewAzureOpenAIProvider(AzureOpenAIConfig{
		TenantID:       azureTestTenant,
		ClientID:       azureTestClientID,
		ClientSecret:   azureTestClientSecret,
		SubscriptionID: azureTestSubscription,
		Scope:          "subscriptions/" + azureTestSubscription,
		ResourceIDs:    nil,
	})
	p.TokenBaseURL = ts.server.URL
	p.ManagementBaseURL = ts.server.URL
	p.HTTPClient = ts.server.Client()
	p.Now = func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) }
	return p
}

const azureCostHappyResponse = `{
  "properties": {
    "nextLink": null,
    "columns": [
      {"name":"totalCost","type":"Number"},
      {"name":"totalUsage","type":"Number"},
      {"name":"UsageDate","type":"Number"},
      {"name":"ResourceId","type":"String"},
      {"name":"ResourceGroup","type":"String"},
      {"name":"ServiceName","type":"String"},
      {"name":"Meter","type":"String"},
      {"name":"MeterCategory","type":"String"},
      {"name":"MeterSubCategory","type":"String"},
      {"name":"UnitOfMeasure","type":"String"},
      {"name":"Currency","type":"String"}
    ],
    "rows": [
      [1.25, 500, 20260520, "/subscriptions/sub/rg/myacct", "rg", "Azure OpenAI", "GPT-4o Input Tokens", "Azure OpenAI", "Azure OpenAI - GPT-4o", "1K Tokens", "USD"],
      [0.75, 200, 20260520, "/subscriptions/sub/rg/myacct", "rg", "Cognitive Services", "GPT-3.5-Turbo Output Tokens", "Cognitive Services", "GPT-3.5-Turbo", "1K Tokens", "USD"],
      [12.34, 24, 20260520, "/subscriptions/sub/rg/computevm", "rg", "Virtual Machines", "D2s v3 vCPU", "Virtual Machines", "D-Series", "Hour", "USD"]
    ]
  }
}`
