package providers

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
	Provider         string  // "openai", "anthropic", ...
	Model            string  // "gpt-4o", "claude-sonnet-4-6", ...
	PromptTokens     int     // input token count only — never the prompt itself
	CompletionTokens int     // output token count only — never the completion itself
	TotalTokens      int     // prompt + completion
	CostUSD          float64 // computed cost in USD
	RecordedAt       string  // RFC3339 timestamp from the provider
}
