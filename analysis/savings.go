package analysis

import (
	"sort"

	"github.com/costroid/costroid-sync/providers"
)

// SavingsRecommendation describes a single cheaper-model swap.
// METADATA-ONLY — no prompt, completion, message, content, or any other
// user-generated text. The JSON key set is enforced by tests.
type SavingsRecommendation struct {
	Provider         string  `json:"provider"`
	CurrentModel     string  `json:"current_model"`
	RecommendedModel string  `json:"recommended_model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CurrentCostUSD   float64 `json:"current_cost_usd"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	SavingsUSD       float64 `json:"savings_usd"`
	SavingsPercent   float64 `json:"savings_percent"`
}

// Strict-greater-than thresholds. A 5%-exactly or $0.01-exactly recommendation
// is suppressed; anything strictly above both bars is shown.
const (
	minSavingsUSD = 0.01
	minSavingsPct = 5.0
)

// Recommend returns at most one cheaper-model recommendation per
// (provider, model) group of records. Returns nil if no qualifying
// alternative exists. The slice is sorted by SavingsUSD descending.
func Recommend(records []providers.NormalizedCostRecord) []SavingsRecommendation {
	type group struct {
		Provider, Model                string
		PromptTokens, CompletionTokens int
		CostUSD                        float64
	}
	groups := map[modelKey]*group{}
	for _, r := range records {
		if r.PromptTokens == 0 && r.CompletionTokens == 0 && r.CostUSD == 0 {
			continue
		}
		k := modelKey{r.Provider, r.Model}
		g := groups[k]
		if g == nil {
			g = &group{Provider: r.Provider, Model: r.Model}
			groups[k] = g
		}
		g.PromptTokens += r.PromptTokens
		g.CompletionTokens += r.CompletionTokens
		g.CostUSD += r.CostUSD
	}

	var out []SavingsRecommendation
	for k, g := range groups {
		if _, known := pricingTable[k]; !known {
			continue
		}
		var (
			bestName string
			bestSave float64
		)
		for altKey, altPrice := range pricingTable {
			if altKey.Provider != g.Provider || altKey.Model == g.Model {
				continue
			}
			altCost := estimateCost(altPrice, g.PromptTokens, g.CompletionTokens)
			save := g.CostUSD - altCost
			if save > bestSave {
				bestSave = save
				bestName = altKey.Model
			}
		}
		if bestName == "" || bestSave <= minSavingsUSD {
			continue
		}
		pct := 0.0
		if g.CostUSD > 0 {
			pct = bestSave / g.CostUSD * 100.0
		}
		if pct <= minSavingsPct {
			continue
		}
		out = append(out, SavingsRecommendation{
			Provider:         g.Provider,
			CurrentModel:     g.Model,
			RecommendedModel: bestName,
			PromptTokens:     g.PromptTokens,
			CompletionTokens: g.CompletionTokens,
			CurrentCostUSD:   g.CostUSD,
			EstimatedCostUSD: g.CostUSD - bestSave,
			SavingsUSD:       bestSave,
			SavingsPercent:   pct,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SavingsUSD != out[j].SavingsUSD {
			return out[i].SavingsUSD > out[j].SavingsUSD
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].CurrentModel < out[j].CurrentModel
	})
	return out
}

func estimateCost(p ModelPricing, prompt, completion int) float64 {
	return (float64(prompt)*p.InputCostPer1M + float64(completion)*p.OutputCostPer1M) / 1_000_000.0
}
