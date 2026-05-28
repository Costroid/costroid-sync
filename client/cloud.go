package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

const (
	wireVersion       = "1"
	maxRecordsPerPush = 1000
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type pushRequest struct {
	Records []providers.NormalizedCostRecord `json:"records"`
}

// PushRecords uploads metadata-only records to Costroid Cloud.
//
// METADATA-ONLY: this function MUST only ever transmit NormalizedCostRecord
// values. Never raw provider responses, never anything containing prompts.
func PushRecords(ctx context.Context, baseURL, orgID, agentKey string, records []providers.NormalizedCostRecord) error {
	if len(records) == 0 {
		return nil
	}
	if strings.TrimSpace(orgID) == "" {
		return errors.New("cloud push skipped: COSTROID_ORG_ID is not set")
	}
	if strings.TrimSpace(agentKey) == "" {
		return errors.New("cloud push skipped: COSTROID_AGENT_KEY is not set")
	}

	endpoint, err := agentSyncURL(baseURL, orgID)
	if err != nil {
		return err
	}

	for start := 0; start < len(records); start += maxRecordsPerPush {
		end := start + maxRecordsPerPush
		if end > len(records) {
			end = len(records)
		}
		if err := pushBatch(ctx, endpoint, agentKey, records[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func agentSyncURL(baseURL, orgID string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("cloud push skipped: COSTROID_API_URL is invalid")
	}
	endpoint := parsed.JoinPath("api", "orgs", orgID, "agent-sync")
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func pushBatch(ctx context.Context, endpoint, agentKey string, records []providers.NormalizedCostRecord) error {
	body, err := json.Marshal(pushRequest{Records: wireRecords(records)})
	if err != nil {
		return errors.New("cloud push failed: could not encode metadata records")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("cloud push failed: could not build request")
	}
	req.Header.Set("Authorization", "Bearer "+agentKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Costroid-Wire-Version", wireVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.New("cloud push failed: network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cloudStatusError(resp.StatusCode)
	}
	return nil
}

func wireRecords(records []providers.NormalizedCostRecord) []providers.NormalizedCostRecord {
	out := make([]providers.NormalizedCostRecord, len(records))
	copy(out, records)
	for i := range out {
		out[i].Provider = wireProviderSlug(out[i].Provider)
		out[i].Model = wireModel(out[i])
	}
	return out
}

func wireProviderSlug(provider string) string {
	switch provider {
	case "github-copilot":
		return "github_copilot"
	case "google-gemini":
		return "google_gemini"
	case "gcp-billing":
		return "gcp_billing"
	case "azure-openai":
		return "azure_openai"
	case "aws-bedrock":
		return "aws_bedrock"
	default:
		return provider
	}
}

func wireModel(record providers.NormalizedCostRecord) string {
	if strings.TrimSpace(record.Model) != "" {
		return record.Model
	}
	if strings.TrimSpace(record.SKU) != "" {
		return record.SKU
	}
	if strings.TrimSpace(record.Product) != "" {
		return record.Product
	}
	return "unknown"
}

func cloudStatusError(statusCode int) error {
	switch statusCode {
	case http.StatusBadRequest:
		return errors.New("cloud push failed: Costroid Cloud rejected the metadata payload")
	case http.StatusUnauthorized:
		return errors.New("cloud push failed: Costroid Cloud rejected the agent key")
	case http.StatusRequestEntityTooLarge:
		return errors.New("cloud push failed: metadata batch is too large")
	case http.StatusTooManyRequests:
		return errors.New("cloud push failed: Costroid Cloud rate limit exceeded")
	default:
		return fmt.Errorf("cloud push failed: Costroid Cloud returned HTTP %d", statusCode)
	}
}
