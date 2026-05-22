package providers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type anthropicGroupKey struct {
	Bucket      string
	WorkspaceID string
	Model       string
}

type anthropicUsageEntry struct {
	bucket string
	result anthropicUsageResult
}

func joinAnthropicProportional(usage []anthropicUsageBucket, costs []anthropicCostBucket) ([]NormalizedCostRecord, error) {
	costsByGroup, err := indexAnthropicCosts(costs)
	if err != nil {
		return nil, err
	}
	usageByGroup := groupAnthropicUsage(usage)

	var out []NormalizedCostRecord
	for k, entries := range usageByGroup {
		out = append(out, allocateAnthropicCostGroup(k, entries, costsByGroup[k])...)
	}
	sortAnthropicRecords(out)
	return out, nil
}

func indexAnthropicCosts(costs []anthropicCostBucket) (map[anthropicGroupKey]float64, error) {
	costsByGroup := map[anthropicGroupKey]float64{}
	for _, b := range costs {
		bucket := canonicalRFC3339(b.StartingAt)
		for _, r := range b.Results {
			if !strings.EqualFold(r.Currency, "usd") {
				continue
			}
			costUSD, err := parseAnthropicCents(r.Amount)
			if err != nil {
				return nil, fmt.Errorf("anthropic %s: %w", anthropicCostPath, err)
			}
			k := anthropicGroupKey{Bucket: bucket, WorkspaceID: r.WorkspaceID, Model: anthropicCostModel(r)}
			costsByGroup[k] += costUSD
		}
	}
	return costsByGroup, nil
}

func groupAnthropicUsage(usage []anthropicUsageBucket) map[anthropicGroupKey][]anthropicUsageEntry {
	usageByGroup := map[anthropicGroupKey][]anthropicUsageEntry{}
	for _, b := range usage {
		bucket := canonicalRFC3339(b.StartingAt)
		for _, r := range b.Results {
			k := anthropicGroupKey{Bucket: bucket, WorkspaceID: r.WorkspaceID, Model: r.Model}
			usageByGroup[k] = append(usageByGroup[k], anthropicUsageEntry{bucket: bucket, result: r})
		}
	}
	return usageByGroup
}

func allocateAnthropicCostGroup(_ anthropicGroupKey, entries []anthropicUsageEntry, groupCost float64) []NormalizedCostRecord {
	var out []NormalizedCostRecord
	groupTokens := 0
	for _, e := range entries {
		groupTokens += e.result.inputTokens() + e.result.OutputTokens
	}
	for _, e := range entries {
		inputTokens := e.result.inputTokens()
		totalTokens := inputTokens + e.result.OutputTokens
		cost := 0.0
		if groupTokens > 0 {
			cost = groupCost * float64(totalTokens) / float64(groupTokens)
		}
		out = append(out, NormalizedCostRecord{
			Provider:         anthropicProviderName,
			Model:            e.result.Model,
			PromptTokens:     inputTokens,
			CompletionTokens: e.result.OutputTokens,
			TotalTokens:      totalTokens,
			CostUSD:          cost,
			RecordedAt:       e.bucket,
			APIKeyID:         e.result.APIKeyID,
			ProjectID:        e.result.WorkspaceID,
			SourceHash:       ComputeSourceHash(anthropicProviderName, e.bucket, e.result.Model, e.result.WorkspaceID, e.result.APIKeyID),
		})
	}
	return out
}

func parseAnthropicCents(amount string) (float64, error) {
	cents, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return 0, errInvalidAnthropicAmount
	}
	return cents / 100, nil
}

func anthropicCostModel(r anthropicCostResult) string {
	if r.Model != "" {
		return r.Model
	}
	return extractClaudeSlug(r.Description)
}

func extractClaudeSlug(s string) string {
	lower := strings.ToLower(s)
	i := strings.Index(lower, "claude-")
	if i < 0 {
		return ""
	}
	tail := lower[i:]
	n := 0
	for n < len(tail) {
		c := tail[n]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			n++
			continue
		}
		break
	}
	return strings.Trim(tail[:n], "-_")
}

func canonicalRFC3339(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

func sortAnthropicRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].ProjectID != records[j].ProjectID {
			return records[i].ProjectID < records[j].ProjectID
		}
		if records[i].Model != records[j].Model {
			return records[i].Model < records[j].Model
		}
		return records[i].APIKeyID < records[j].APIKeyID
	})
}
