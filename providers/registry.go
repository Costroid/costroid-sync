package providers

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by provider stubs that have not been
// wired up yet.
var ErrNotImplemented = errors.New("provider not implemented yet")

// Provider is the interface every cost source must implement.
// Implementations live in providers/<name>.go and MUST return only
// NormalizedCostRecord values — see the metadata-only rule in types.go.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error)
}

// registry holds available providers keyed by Name().
// Population happens in C2 once OpenAI is the first real provider.
var registry = map[string]Provider{}

// Register adds a provider to the registry. Safe to call from init().
func Register(p Provider) {
	registry[p.Name()] = p
}

// Get returns the provider registered under name, or nil if absent.
func Get(name string) Provider {
	return registry[name]
}

// All returns every registered provider.
func All() []Provider {
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	return out
}
