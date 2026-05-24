package providers

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAzure_TokenRequestConstruction(t *testing.T) {
	ts := newAzureTestServer(t)
	p := newAzureTestProvider(t, ts)
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ts.capturedToken) == 0 {
		t.Fatal("token endpoint never called")
	}
	r := ts.capturedToken[0]
	wantPath := "/" + azureTestTenant + "/oauth2/v2.0/token"
	if r.Path != wantPath {
		t.Errorf("path = %q, want %q", r.Path, wantPath)
	}
	if r.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", r.Method)
	}
	if r.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", r.ContentType)
	}
	wantBodyParts := []string{
		"grant_type=client_credentials",
		"client_id=" + azureTestClientID,
		"client_secret=" + azureTestClientSecret,
		"scope=https%3A%2F%2Fmanagement.azure.com%2F.default",
	}
	for _, p := range wantBodyParts {
		if !strings.Contains(r.Body, p) {
			t.Errorf("body missing %q: %s", p, r.Body)
		}
	}
}

func TestAzure_TokenCaching(t *testing.T) {
	ts := newAzureTestServer(t)
	p := newAzureTestProvider(t, ts)
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if got := atomic.LoadInt32(&ts.tokenCalls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1 (cached)", got)
	}
	if got := atomic.LoadInt32(&ts.costCalls); got != 2 {
		t.Errorf("cost endpoint called %d times, want 2", got)
	}
	for i, r := range ts.capturedCost {
		if r.Authorization != "Bearer "+azureTestBearer {
			t.Errorf("cost call %d Authorization = %q", i, r.Authorization)
		}
	}
}
