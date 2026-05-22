package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// NormalizedCostRecord is the ONLY shape providers may emit.
//
// METADATA-ONLY RULE (see AGENTS.md "The One Unbreakable Rule"):
//   - NEVER add fields for prompt, completion, messages, content,
//     system prompts, function arguments, tool call text, or any
//     other user-generated text.
//   - When parsing a raw provider response, explicitly extract the
//     fields below and discard everything else. Do not Unmarshal
//     into a catch-all map and forward it.
//   - A violation of this rule is a critical security bug.
type NormalizedCostRecord struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	RecordedAt       string  `json:"recorded_at"`
	APIKeyID         string  `json:"api_key_id"`
	ProjectID        string  `json:"project_id"`
	SourceHash       string  `json:"source_hash"`
}

// ComputeSourceHash returns the deterministic identity hash for a record.
// Inputs are identity-only; volatile fields (token counts, cost) are
// intentionally excluded so revisions to the same bucket UPSERT in place
// instead of inserting new rows.
func ComputeSourceHash(provider, recordedAt, model, projectID, apiKeyID string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s", provider, recordedAt, model, projectID, apiKeyID)
	return hex.EncodeToString(h.Sum(nil))
}
