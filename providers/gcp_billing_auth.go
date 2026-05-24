package providers

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	gcpJWTGrantType  = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	gcpJWTLifetimeS  = 3600 // 1h — Google caps at 1h
	gcpJWTHeader     = `{"alg":"RS256","typ":"JWT"}`
	gcpTokenEndpoint = "/token"
)

// gcpServiceAccount is the narrow projection of the service-account JSON
// file. Only fields Costroid uses are declared; encoding/json discards
// every other field, including project_id, client_id, auth_uri,
// auth_provider_x509_cert_url, etc.
//
// SECURITY: The PrivateKey field is loaded into memory but is NEVER
// serialized back into errors, log lines, hashes, or any user-facing
// string. The parsed *rsa.PrivateKey lives only inside gcpServiceAccount
// and is consumed via signJWTAssertion.
type gcpServiceAccount struct {
	ClientEmail string
	TokenURI    string
	parsedKey   *rsa.PrivateKey
}

// loadGCPServiceAccount reads the service-account JSON file at path,
// extracts the three fields needed for JWT-bearer auth, and parses the
// PEM-encoded RSA private key. Returned errors never include the file
// contents or the private key.
func loadGCPServiceAccount(path string) (*gcpServiceAccount, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("gcp-billing: %s not set", envGCPServiceAccountJSON)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		// os.PathError carries the path (intentional — useful) but not
		// the file contents.
		return nil, fmt.Errorf("gcp-billing: read service account: %w", err)
	}

	var fields struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.New("gcp-billing: parse service account JSON failed")
	}
	if fields.ClientEmail == "" {
		return nil, errors.New("gcp-billing: service account JSON missing client_email")
	}
	if fields.PrivateKey == "" {
		return nil, errors.New("gcp-billing: service account JSON missing private_key")
	}
	tokenURI := fields.TokenURI
	if tokenURI == "" {
		tokenURI = gcpBillingDefaultTokenURL
	}

	key, err := parseGCPRSAKey(fields.PrivateKey)
	if err != nil {
		// Wrapping err preserves the high-level reason but parseGCPRSAKey
		// itself never includes key material in its error string.
		return nil, fmt.Errorf("gcp-billing: %w", err)
	}

	return &gcpServiceAccount{
		ClientEmail: fields.ClientEmail,
		TokenURI:    tokenURI,
		parsedKey:   key,
	}, nil
}

func parseGCPRSAKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("invalid PEM block in service account private_key")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("service account private_key is not RSA")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("service account private_key parse failed")
}

// loadedServiceAccount returns the cached service account, loading it
// from disk on first use. Safe for concurrent callers.
func (p *GCPBillingProvider) loadedServiceAccount() (*gcpServiceAccount, error) {
	p.saMu.Lock()
	defer p.saMu.Unlock()
	if p.sa != nil {
		return p.sa, nil
	}
	sa, err := loadGCPServiceAccount(p.ServiceAccountJSONPath)
	if err != nil {
		return nil, err
	}
	p.sa = sa
	return sa, nil
}

// getAccessToken returns a cached bearer token or fetches a new one via
// the JWT-bearer grant. Token is cached for 90% of expires_in so we
// never race expiry.
//
// SECURITY: response body is never read on error; private_key, the JWT
// assertion, and the access_token are never written to any error string
// or log line.
func (p *GCPBillingProvider) getAccessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.accessToken != "" && p.now().Before(p.tokenExp) {
		return p.accessToken, nil
	}

	sa, err := p.loadedServiceAccount()
	if err != nil {
		return "", err
	}

	assertion, err := buildGCPJWTAssertion(sa, p.now())
	if err != nil {
		return "", fmt.Errorf("gcp-billing token: %w", err)
	}

	req, err := p.newGCPTokenRequest(ctx, sa.TokenURI, assertion)
	if err != nil {
		return "", err
	}
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("gcp-billing token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", wrapGCPBillingTokenPermissionHint(&gcpBillingHTTPError{
			StatusCode: resp.StatusCode,
			Endpoint:   gcpTokenEndpoint,
		})
	}

	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", errors.New("gcp-billing token: decode response failed")
	}
	if body.AccessToken == "" {
		return "", errors.New("gcp-billing token: empty access_token in response")
	}

	return p.cacheGCPAccessToken(body.AccessToken, body.ExpiresIn), nil
}

func (p *GCPBillingProvider) newGCPTokenRequest(ctx context.Context, tokenURI, assertion string) (*http.Request, error) {
	target := tokenURI
	if p.TokenURL != "" {
		target = p.TokenURL
	}
	form := url.Values{}
	form.Set("grant_type", gcpJWTGrantType)
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("gcp-billing token: build request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.userAgent())
	return req, nil
}

func (p *GCPBillingProvider) cacheGCPAccessToken(token string, expiresIn int) string {
	p.accessToken = token
	cacheDuration := time.Duration(float64(expiresIn)*0.9) * time.Second
	if cacheDuration <= 0 {
		cacheDuration = 5 * time.Minute
	}
	p.tokenExp = p.now().Add(cacheDuration)
	return p.accessToken
}

// buildGCPJWTAssertion constructs a signed RS256 JWT for the JWT-bearer
// grant. Returned string is safe to put into a request body but MUST
// NOT be logged or echoed back to the user.
func buildGCPJWTAssertion(sa *gcpServiceAccount, now time.Time) (string, error) {
	iat := now.UTC().Unix()
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": gcpBillingScope,
		"aud":   sa.TokenURI,
		"iat":   iat,
		"exp":   iat + gcpJWTLifetimeS,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", errors.New("marshal claims failed")
	}
	signingInput := gcpJWTBase64(gcpJWTHeader) + "." + gcpJWTBase64String(claimsJSON)

	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, sa.parsedKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("sign JWT failed")
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func gcpJWTBase64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func gcpJWTBase64String(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
