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

	// Billing metadata for providers whose APIs report quantity/price
	// breakdowns rather than tokens (e.g. github-copilot premium-request
	// billing). METADATA ONLY. The seven fields below carry identifiers,
	// counts, and money amounts — nothing else. Adding any field here that
	// could hold prompt, completion, message, content, raw payload, source
	// code, repository contents, issue/PR text, or any other user-generated
	// text is a critical security bug. Tests assert the JSON keyset is
	// exactly these 17 entries.
	Product           string  `json:"product"`
	SKU               string  `json:"sku"`
	UnitType          string  `json:"unit_type"`
	UsageQuantity     float64 `json:"usage_quantity"`
	UnitPriceUSD      float64 `json:"unit_price_usd"`
	GrossAmountUSD    float64 `json:"gross_amount_usd"`
	DiscountAmountUSD float64 `json:"discount_amount_usd"`
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
