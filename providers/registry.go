package providers

import (
	"context"
	"errors"
	"sort"
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

// Registration describes how the CLI can construct a provider from its
// environment variable(s).
type Registration struct {
	Name           string
	Aliases        []string // alternate names accepted by Get; usually empty
	EnvVar         string
	ExtraEnvVars   []string // additional env vars that must be set; checked at sync time
	MissingEnvHelp string
	New            func(adminKey string) Provider
}

var (
	registry      = map[string]Registration{}
	providerOrder = []string{"openai", "anthropic", "github-copilot", "google-gemini", "azure-openai", "aws-bedrock"}
)

// Register adds a provider registration. Safe to call from init().
func Register(reg Registration) {
	registry[reg.Name] = reg
}

// Get returns the provider registration matching name OR any of its
// Aliases. Canonical names take precedence over aliases.
func Get(name string) (Registration, bool) {
	if reg, ok := registry[name]; ok {
		return reg, true
	}
	for _, reg := range registry {
		for _, a := range reg.Aliases {
			if a == name {
				return reg, true
			}
		}
	}
	return Registration{}, false
}

// All returns every registered provider in stable CLI order.
func All() []Registration {
	out := make([]Registration, 0, len(registry))
	seen := map[string]bool{}
	for _, name := range providerOrder {
		if reg, ok := registry[name]; ok {
			out = append(out, reg)
			seen[name] = true
		}
	}

	var rest []string
	for name := range registry {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	for _, name := range rest {
		out = append(out, registry[name])
	}
	return out
}
