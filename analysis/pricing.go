package analysis

// ModelPricing is a single seed-pricing row. Self-describing so future
// pricing sources (bundled JSON, local SQLite table, or a
// `costroid pricing update` command) can populate it directly
// without touching savings.go.
type ModelPricing struct {
	Provider        string
	Model           string
	InputCostPer1M  float64
	OutputCostPer1M float64
	Source          string
	EffectiveDate   string // YYYY-MM-DD
	Notes           string
}

// modelKey is the lookup key over pricingTable.
type modelKey struct {
	Provider string
	Model    string
}

// seedPricing is the embedded C4 dataset. Verified by hand against the
// listed Source URLs on EffectiveDate. Update by hand for now; future
// versions can replace this slice's loader with one that reads from a
// bundled JSON file or a local SQLite pricing table.
//
// Unknown models (any provider/model combo absent from this list) are
// skipped safely by Recommend(). Do NOT add a row whose price you
// cannot verify against the Source.
//
// Static seed pricing for offline C4 savings recommendations. Not a
// live availability guarantee. Future versions should support updating
// this pricing source.
var seedPricing = []ModelPricing{
	// OpenAI — https://openai.com/api/pricing/
	{Provider: "openai", Model: "gpt-5.5", InputCostPer1M: 5.00, OutputCostPer1M: 30.00,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-5.4", InputCostPer1M: 2.50, OutputCostPer1M: 15.00,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-5.4-mini", InputCostPer1M: 0.75, OutputCostPer1M: 4.50,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-4.1", InputCostPer1M: 2.00, OutputCostPer1M: 8.00,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-4.1-mini", InputCostPer1M: 0.40, OutputCostPer1M: 1.60,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-4.1-nano", InputCostPer1M: 0.10, OutputCostPer1M: 0.40,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-4o", InputCostPer1M: 2.50, OutputCostPer1M: 10.00,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},
	{Provider: "openai", Model: "gpt-4o-mini", InputCostPer1M: 0.15, OutputCostPer1M: 0.60,
		Source: "https://openai.com/api/pricing/", EffectiveDate: "2026-05-22"},

	// Anthropic — https://docs.anthropic.com/en/docs/about-claude/models
	{Provider: "anthropic", Model: "claude-opus-4-7", InputCostPer1M: 5.00, OutputCostPer1M: 25.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-opus-4-6", InputCostPer1M: 5.00, OutputCostPer1M: 25.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-opus-4-5", InputCostPer1M: 5.00, OutputCostPer1M: 25.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-opus-4-1", InputCostPer1M: 15.00, OutputCostPer1M: 75.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-sonnet-4-6", InputCostPer1M: 3.00, OutputCostPer1M: 15.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-sonnet-4-5", InputCostPer1M: 3.00, OutputCostPer1M: 15.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-haiku-4-5", InputCostPer1M: 1.00, OutputCostPer1M: 5.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
	{Provider: "anthropic", Model: "claude-haiku-3-5", InputCostPer1M: 0.80, OutputCostPer1M: 4.00,
		Source: "https://docs.anthropic.com/en/docs/about-claude/models", EffectiveDate: "2026-05-22"},
}

// pricingTable is the O(1) lookup index over seedPricing, built at
// startup. Future loaders (JSON file, SQLite table) populate seedPricing
// instead; this derivation stays identical.
var pricingTable = func() map[modelKey]ModelPricing {
	m := make(map[modelKey]ModelPricing, len(seedPricing))
	for _, p := range seedPricing {
		m[modelKey{p.Provider, p.Model}] = p
	}
	return m
}()
