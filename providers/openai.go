package providers

import "context"

// OpenAIProvider fetches metadata-only usage from the OpenAI Usage API.
// Real implementation lands in C2.
type OpenAIProvider struct{}

func (OpenAIProvider) Name() string { return "openai" }

func (OpenAIProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	return nil, ErrNotImplemented
}
