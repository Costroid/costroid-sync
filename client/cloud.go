package client

import (
	"context"
	"errors"

	"github.com/costroid/costroid-sync/providers"
)

// ErrNotImplemented is returned by client stubs not yet wired up.
var ErrNotImplemented = errors.New("cloud client not implemented yet")

// Push uploads metadata-only records to costroid.com when the user
// passes --push. Real implementation lands in C11.
//
// METADATA-ONLY: this function MUST only ever transmit NormalizedCostRecord
// values. Never raw provider responses, never anything containing prompts.
func Push(ctx context.Context, agentKey string, records []providers.NormalizedCostRecord) error {
	return ErrNotImplemented
}
