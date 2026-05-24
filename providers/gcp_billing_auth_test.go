package providers

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// TestGCPBillingAuth_JWTAssertionConstruction asserts the JWT header,
// claims, and RS256 signature are all built correctly. The signature is
// verified against the public half of the test key.
func TestGCPBillingAuth_JWTAssertionConstruction(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)

	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ts.capturedToken) != 1 {
		t.Fatalf("token endpoint called %d times, want 1", len(ts.capturedToken))
	}

	assertion := assertGCPTokenFormAndExtractJWT(t, ts.capturedToken[0])
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion has %d segments, want 3", len(parts))
	}
	assertGCPJWTHeader(t, parts[0])
	assertGCPJWTClaims(t, parts[1], ts.server.URL+"/token")
	assertGCPJWTSignature(t, p, parts)
}

func assertGCPTokenFormAndExtractJWT(t *testing.T, rec gcpRecordedReq) string {
	t.Helper()
	form, err := url.ParseQuery(rec.Body)
	if err != nil {
		t.Fatalf("parse token form: %v", err)
	}
	if got := form.Get("grant_type"); got != gcpJWTGrantType {
		t.Errorf("grant_type = %q, want %q", got, gcpJWTGrantType)
	}
	if rec.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", rec.ContentType)
	}
	assertion := form.Get("assertion")
	if assertion == "" {
		t.Fatal("assertion missing from token request body")
	}
	return assertion
}

func assertGCPJWTHeader(t *testing.T, segment string) {
	t.Helper()
	headerJSON, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.Alg != "RS256" || header.Typ != "JWT" {
		t.Errorf("header = %+v, want alg=RS256 typ=JWT", header)
	}
}

func assertGCPJWTClaims(t *testing.T, segment, wantAud string) {
	t.Helper()
	claimsJSON, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims struct {
		Iss   string `json:"iss"`
		Scope string `json:"scope"`
		Aud   string `json:"aud"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims.Iss != gcpTestEmail {
		t.Errorf("iss = %q, want %q", claims.Iss, gcpTestEmail)
	}
	if claims.Scope != gcpBillingScope {
		t.Errorf("scope = %q, want %q", claims.Scope, gcpBillingScope)
	}
	if claims.Aud != wantAud {
		t.Errorf("aud = %q, want %q", claims.Aud, wantAud)
	}
	if claims.Exp-claims.Iat != gcpJWTLifetimeS {
		t.Errorf("exp - iat = %d, want %d", claims.Exp-claims.Iat, gcpJWTLifetimeS)
	}
}

func assertGCPJWTSignature(t *testing.T, p *GCPBillingProvider, parts []string) {
	t.Helper()
	sa, err := loadGCPServiceAccount(p.ServiceAccountJSONPath)
	if err != nil {
		t.Fatalf("loadGCPServiceAccount: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&sa.parsedKey.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("verify signature: %v", err)
	}
}

// TestGCPBillingAuth_TokenCaching asserts the token endpoint is called
// exactly once across multiple Fetches, and that the cached bearer is
// used on subsequent BigQuery calls.
func TestGCPBillingAuth_TokenCaching(t *testing.T) {
	ts := newGCPBillingTestServer(t)
	p := newGCPBillingTestProvider(t, ts)

	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if _, err := p.Fetch(context.Background(), 1); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if got := atomic.LoadInt32(&ts.tokenCalls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1 (cached)", got)
	}
	if got := atomic.LoadInt32(&ts.queryCalls); got != 2 {
		t.Errorf("query endpoint called %d times, want 2", got)
	}
	for i, r := range ts.capturedQuery {
		if r.Authorization != "Bearer "+gcpTestBearer {
			t.Errorf("query call %d Authorization = %q", i, r.Authorization)
		}
	}
}

// TestGCPBillingAuth_TokenErrorNoLeak verifies that token-endpoint
// failures never leak private_key, BEGIN PRIVATE KEY, the JWT assertion,
// or the access token through the error string.
func TestGCPBillingAuth_TokenErrorNoLeak(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 500} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			ts := newGCPBillingTestServer(t)
			ts.tokenHandler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"POISON_TOKEN_ERROR","secret_value":"LEAKED_SECRET"}`))
			}
			p := newGCPBillingTestProvider(t, ts)
			_, err := p.Fetch(context.Background(), 1)
			if err == nil {
				t.Fatalf("want error for status %d", status)
			}
			msg := err.Error()
			forbidden := []string{
				"POISON_TOKEN_ERROR", "LEAKED_SECRET",
				"private_key", "BEGIN PRIVATE KEY", "END PRIVATE KEY",
				"assertion=",
				gcpTestBearer, gcpTestEmail,
			}
			for _, bad := range forbidden {
				if strings.Contains(msg, bad) {
					t.Errorf("error string leaked %q: %s", bad, msg)
				}
			}
			if !strings.Contains(msg, fmt.Sprintf("HTTP %d", status)) {
				t.Errorf("missing HTTP status in error: %s", msg)
			}
			var he *gcpBillingHTTPError
			if !errors.As(err, &he) {
				t.Fatalf("missing gcpBillingHTTPError in chain: %v", err)
			}
			if status != 500 && !strings.Contains(msg, "Google OAuth token request failed") {
				t.Errorf("status %d should include permission hint: %s", status, msg)
			}
			if status == 500 && strings.Contains(msg, "Google OAuth token request failed") {
				t.Errorf("500 should not include permission hint: %s", msg)
			}
		})
	}
}

// TestGCPBillingAuth_MalformedServiceAccount asserts that a corrupt
// service-account JSON file fails fast without leaking file contents.
func TestGCPBillingAuth_MalformedServiceAccount(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"missing_client_email", `{"private_key":"x","token_uri":"https://oauth2.googleapis.com/token"}`, "client_email"},
		{"missing_private_key", `{"client_email":"a@b.com","token_uri":"https://oauth2.googleapis.com/token"}`, "private_key"},
		{"bad_pem", `{"client_email":"a@b.com","private_key":"not a pem block","token_uri":"https://oauth2.googleapis.com/token"}`, "PEM"},
		{"bad_json", `not json at all`, "parse service account JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeRawServiceAccount(t, tc.json)
			_, err := loadGCPServiceAccount(path)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), tc.json) {
				t.Errorf("error leaked file contents: %s", err.Error())
			}
		})
	}
}

func writeRawServiceAccount(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/sa.json"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write sa file: %v", err)
	}
	return path
}
