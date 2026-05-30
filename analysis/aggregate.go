package analysis

import (
	"sort"
	"time"

	"github.com/costroid/costroid-sync/providers"
)

// ProviderTotal aggregates spend and token metadata for a single provider.
// METADATA-ONLY — provider name, counts, and money amounts only. Never carries
// prompts, completions, messages, raw payloads, or any other content.
type ProviderTotal struct {
	Provider    string
	CostUSD     float64
	TotalTokens int
	Records     int
}

// ModelTotal aggregates spend and token metadata for one (provider, model)
// pair. METADATA-ONLY — provider/model names, counts, and money amounts only.
type ModelTotal struct {
	Provider    string
	Model       string
	CostUSD     float64
	TotalTokens int
	Records     int
}

// ProviderActivity reports the most recent recorded_at seen for a provider,
// used by the Recent Syncs panel. METADATA-ONLY — a provider name and a
// timestamp only. LatestActive is the zero time when no parseable timestamp
// exists for the provider.
type ProviderActivity struct {
	Provider     string
	LatestActive time.Time
}

// AggregateByProvider sums cost and tokens per provider over the supplied
// records. The result is sorted by spend descending, then provider name, so
// rendering is deterministic.
func AggregateByProvider(records []providers.NormalizedCostRecord) []ProviderTotal {
	groups := map[string]*ProviderTotal{}
	for _, r := range records {
		g := groups[r.Provider]
		if g == nil {
			g = &ProviderTotal{Provider: r.Provider}
			groups[r.Provider] = g
		}
		g.CostUSD += r.CostUSD
		g.TotalTokens += r.TotalTokens
		g.Records++
	}
	out := make([]ProviderTotal, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

// AggregateByModel sums cost and tokens per (provider, model) over the supplied
// records. The result is sorted by spend descending, then provider, then model.
func AggregateByModel(records []providers.NormalizedCostRecord) []ModelTotal {
	groups := map[modelKey]*ModelTotal{}
	for _, r := range records {
		k := modelKey{r.Provider, r.Model}
		g := groups[k]
		if g == nil {
			g = &ModelTotal{Provider: r.Provider, Model: r.Model}
			groups[k] = g
		}
		g.CostUSD += r.CostUSD
		g.TotalTokens += r.TotalTokens
		g.Records++
	}
	out := make([]ModelTotal, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// LatestActivityByProvider returns, per provider, the most recent parseable
// recorded_at timestamp. Providers whose timestamps never parse keep the zero
// time and sort last. The slice is sorted most-recent-first, then by provider.
func LatestActivityByProvider(records []providers.NormalizedCostRecord) []ProviderActivity {
	latest := map[string]time.Time{}
	seen := map[string]bool{}
	for _, r := range records {
		seen[r.Provider] = true
		t, ok := parseRecordedAt(r.RecordedAt)
		if !ok {
			continue
		}
		if cur := latest[r.Provider]; t.After(cur) {
			latest[r.Provider] = t
		}
	}
	out := make([]ProviderActivity, 0, len(seen))
	for p := range seen {
		out = append(out, ProviderActivity{Provider: p, LatestActive: latest[p]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LatestActive.Equal(out[j].LatestActive) {
			return out[i].Provider < out[j].Provider
		}
		return out[i].LatestActive.After(out[j].LatestActive)
	})
	return out
}
