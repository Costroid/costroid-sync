package providers

import "context"

// AnthropicProvider fetches metadata-only usage from the Anthropic
// Usage Report API. Real implementation lands in C3.
type AnthropicProvider struct{}

func (AnthropicProvider) Name() string { return "anthropic" }

func (AnthropicProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	return nil, ErrNotImplemented
}
