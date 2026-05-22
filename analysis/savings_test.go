package analysis

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/costroid/costroid-sync/providers"
)

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestEstimateCost(t *testing.T) {
	// 10_000 input * $2.50/1M + 5_000 output * $10.00/1M
	// = $0.025 + $0.05 = $0.075
	got := estimateCost(ModelPricing{InputCostPer1M: 2.50, OutputCostPer1M: 10.00}, 10_000, 5_000)
	want := 0.075
	if !approxEqual(got, want, 1e-12) {
		t.Errorf("estimateCost = %v, want %v", got, want)
	}
}

func TestRecommend_ExpensiveSuggestsCheaper(t *testing.T) {
	// gpt-4o with meaningful usage should produce a cheaper-alternative rec.
	recs := Recommend([]providers.NormalizedCostRecord{
		{
			Provider: "openai", Model: "gpt-4o",
			PromptTokens: 1_000_000, CompletionTokens: 200_000,
			CostUSD: 4.50, // ≈ what gpt-4o would actually cost
		},
	})
	if len(recs) != 1 {
		t.Fatalf("want 1 recommendation, got %d (%+v)", len(recs), recs)
	}
	r := recs[0]
	if r.Provider != "openai" || r.CurrentModel != "gpt-4o" {
		t.Errorf("identity wrong: %+v", r)
	}
	if r.RecommendedModel == "" || r.RecommendedModel == "gpt-4o" {
		t.Errorf("recommendation must be a different model, got %q", r.RecommendedModel)
	}
	if r.SavingsUSD <= 0 || r.SavingsPercent <= 0 {
		t.Errorf("non-positive savings: %+v", r)
	}
}

func TestRecommend_AlreadyCheapestNoRec(t *testing.T) {
	// gpt-4.1-nano is the cheapest openai entry at $0.10/$0.40 — no cheaper
	// same-provider alternative exists, so no recommendation should fire.
	recs := Recommend([]providers.NormalizedCostRecord{
		{
			Provider: "openai", Model: "gpt-4.1-nano",
			PromptTokens: 10_000_000, CompletionTokens: 1_000_000,
			CostUSD: 1.40, // close to its own estimated cost
		},
	})
	if len(recs) != 0 {
		t.Errorf("want 0 recommendations for already-cheapest model, got %+v", recs)
	}
}

func TestRecommend_UnknownModelSkipped(t *testing.T) {
	recs := Recommend([]providers.NormalizedCostRecord{
		{
			Provider: "openai", Model: "gpt-9000-future",
			PromptTokens: 1_000_000, CompletionTokens: 500_000,
			CostUSD: 100.00,
		},
	})
	if len(recs) != 0 {
		t.Errorf("want 0 recommendations for unknown model, got %+v", recs)
	}
}

func TestRecommend_ZeroTokensAndZeroCostSkipped(t *testing.T) {
	// Must not panic on all-zero rows.
	recs := Recommend([]providers.NormalizedCostRecord{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "anthropic", Model: "claude-opus-4-7"},
	})
	if len(recs) != 0 {
		t.Errorf("want 0 recommendations for zero-token rows, got %+v", recs)
	}
}

func TestRecommend_BelowThresholdNotShown(t *testing.T) {
	// Case A: savings ≈ $0.004 (below the $0.01 USD threshold).
	// 1000 input tokens of gpt-4o; cheapest alt ≈ 1000 * $0.10 / 1M = $0.0001.
	// Set CostUSD = $0.0045 → savings = $0.0044 → fails USD threshold.
	recsA := Recommend([]providers.NormalizedCostRecord{{
		Provider: "openai", Model: "gpt-4o",
		PromptTokens: 1000, CompletionTokens: 0, CostUSD: 0.0045,
	}})
	if len(recsA) != 0 {
		t.Errorf("USD-threshold case: want 0, got %+v", recsA)
	}

	// Case B: savings is significant in absolute terms but < 5% of current.
	// Use 10M input tokens. Cheapest alt = 10M * $0.10 / 1M = $1.00.
	// Set CostUSD = $1.04 → savings = $0.04 (passes $0.01),
	// pct = 0.04/1.04 ≈ 3.85% (fails 5%).
	recsB := Recommend([]providers.NormalizedCostRecord{{
		Provider: "openai", Model: "gpt-4o",
		PromptTokens: 10_000_000, CompletionTokens: 0, CostUSD: 1.04,
	}})
	if len(recsB) != 0 {
		t.Errorf("pct-threshold case: want 0, got %+v", recsB)
	}
}

func TestRecommend_SavingsPercentExact(t *testing.T) {
	// 10M input tokens of gpt-4o; cheapest alt cost = 10M * $0.10/1M = $1.00.
	// Set CostUSD = $2.00 → savings = $1.00 → pct = 50.0%.
	recs := Recommend([]providers.NormalizedCostRecord{{
		Provider: "openai", Model: "gpt-4o",
		PromptTokens: 10_000_000, CompletionTokens: 0, CostUSD: 2.00,
	}})
	if len(recs) != 1 {
		t.Fatalf("want 1 rec, got %+v", recs)
	}
	if !approxEqual(recs[0].SavingsUSD, 1.00, 1e-9) {
		t.Errorf("SavingsUSD = %v, want 1.00", recs[0].SavingsUSD)
	}
	if !approxEqual(recs[0].SavingsPercent, 50.0, 1e-9) {
		t.Errorf("SavingsPercent = %v, want 50.0", recs[0].SavingsPercent)
	}
	if !approxEqual(recs[0].EstimatedCostUSD, 1.00, 1e-9) {
		t.Errorf("EstimatedCostUSD = %v, want 1.00", recs[0].EstimatedCostUSD)
	}
}

func TestSavingsRecommendation_JSONKeySet(t *testing.T) {
	r := SavingsRecommendation{
		Provider:         "openai",
		CurrentModel:     "gpt-4o",
		RecommendedModel: "gpt-4o-mini",
		PromptTokens:     1,
		CompletionTokens: 1,
		CurrentCostUSD:   1.0,
		EstimatedCostUSD: 0.1,
		SavingsUSD:       0.9,
		SavingsPercent:   90.0,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)

	forbidden := []string{
		`"messages"`, `"content"`, `"tool_calls"`, `"raw_response"`,
		`"raw_payload"`, `"request_body"`, `"response_body"`,
		`"system_prompt"`, `"function_args"`,
	}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("forbidden substring %q in JSON: %s", bad, s)
		}
	}

	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	expected := map[string]bool{
		"provider": true, "current_model": true, "recommended_model": true,
		"prompt_tokens": true, "completion_tokens": true,
		"current_cost_usd": true, "estimated_cost_usd": true,
		"savings_usd": true, "savings_percent": true,
	}
	for k := range back {
		if !expected[k] {
			t.Errorf("unexpected JSON key %q in %s", k, s)
		}
	}
	if len(back) != len(expected) {
		t.Errorf("got %d keys, want %d: %v", len(back), len(expected), back)
	}
}

func TestRecommend_SameProviderOnly(t *testing.T) {
	// Both providers, both expensive top-tier models with serious usage.
	recs := Recommend([]providers.NormalizedCostRecord{
		{
			Provider: "openai", Model: "gpt-4o",
			PromptTokens: 5_000_000, CompletionTokens: 1_000_000, CostUSD: 22.50,
		},
		{
			Provider: "anthropic", Model: "claude-opus-4-7",
			PromptTokens: 5_000_000, CompletionTokens: 1_000_000, CostUSD: 150.00,
		},
	})
	if len(recs) != 2 {
		t.Fatalf("want 2 recommendations, got %d (%+v)", len(recs), recs)
	}
	for _, r := range recs {
		// Recommended model must be in the SAME provider as the current.
		key := modelKey{r.Provider, r.RecommendedModel}
		if _, ok := pricingTable[key]; !ok {
			t.Errorf("recommended %q not in same-provider pricing table for %s", r.RecommendedModel, r.Provider)
		}
		// Defensive: explicit cross-provider check.
		if r.Provider == "openai" && strings.HasPrefix(r.RecommendedModel, "claude") {
			t.Errorf("cross-provider rec: openai → claude: %+v", r)
		}
		if r.Provider == "anthropic" && !strings.HasPrefix(r.RecommendedModel, "claude") {
			t.Errorf("cross-provider rec: anthropic → non-claude: %+v", r)
		}
	}
	// Sort order: SavingsUSD descending. Anthropic opus has the larger spend.
	if recs[0].SavingsUSD < recs[1].SavingsUSD {
		t.Errorf("recommendations not sorted by SavingsUSD desc: %+v", recs)
	}
}

func TestRecommend_BestSingleAlternativeChosen(t *testing.T) {
	// gpt-4o with 1M output tokens. Cheapest openai output entry in the
	// seed map is gpt-4.1-nano @ $0.40/1M => $0.40 for 1M tokens.
	// gpt-4o-mini is next-cheapest at $0.60/1M.
	recs := Recommend([]providers.NormalizedCostRecord{{
		Provider: "openai", Model: "gpt-4o",
		PromptTokens: 0, CompletionTokens: 1_000_000, CostUSD: 10.00,
	}})
	if len(recs) != 1 {
		t.Fatalf("want 1 rec, got %+v", recs)
	}
	r := recs[0]
	if r.RecommendedModel != "gpt-4.1-nano" {
		t.Errorf("RecommendedModel = %q, want gpt-4.1-nano (cheapest output in map)", r.RecommendedModel)
	}
	if !approxEqual(r.EstimatedCostUSD, 0.40, 1e-9) {
		t.Errorf("EstimatedCostUSD = %v, want 0.40", r.EstimatedCostUSD)
	}
}
