package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// azureTokenResponse models only the fields Costroid reads from the
// Microsoft Entra token endpoint. Everything else in the response is
// silently dropped — encoding/json never reads what we don't declare.
type azureTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getAccessToken returns a cached bearer token or fetches a new one.
// The token is cached for 90% of the issuer's `expires_in` so we never
// race against expiry.
//
// SECURITY: the response body is never read on error and the client
// secret is never logged. The request body IS sent over TLS (URL-form
// encoded) but is never written to any error message or note.
func (p *AzureOpenAIProvider) getAccessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.accessToken != "" && p.now().Before(p.tokenExp) {
		return p.accessToken, nil
	}

	req, err := p.newTokenRequest(ctx)
	if err != nil {
		return "", err
	}
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("azure-openai token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body) // drain without reading
		return "", wrapAzureTokenPermissionHint(&azureHTTPError{
			StatusCode: resp.StatusCode,
			Endpoint:   "/oauth2/v2.0/token",
		})
	}

	var body azureTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("azure-openai token: decode response: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("azure-openai token: empty access_token in response")
	}

	return p.cacheAccessToken(body), nil
}

func (p *AzureOpenAIProvider) newTokenRequest(ctx context.Context) (*http.Request, error) {
	tokenURL := strings.TrimRight(p.tokenBaseURL(), "/") + "/" + p.TenantID + "/oauth2/v2.0/token"
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("scope", azureMgmtScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("azure-openai token: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.userAgent())
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (p *AzureOpenAIProvider) cacheAccessToken(body azureTokenResponse) string {
	p.accessToken = body.AccessToken
	cacheDuration := time.Duration(float64(body.ExpiresIn)*0.9) * time.Second
	if cacheDuration <= 0 {
		cacheDuration = 5 * time.Minute
	}
	p.tokenExp = p.now().Add(cacheDuration)
	return p.accessToken
}
