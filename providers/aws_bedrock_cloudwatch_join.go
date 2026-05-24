package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func groupTokenTotalsByDate(in map[string]*awsBedrockTokenTotals) map[string][]awsBedrockTokenTotals {
	out := map[string][]awsBedrockTokenTotals{}
	for key, totals := range in {
		date := strings.SplitN(key, "|", 2)[0]
		out[date] = append(out[date], *totals)
	}
	return out
}

func mergeBedrockTokenTotals(dst, src map[string][]awsBedrockTokenTotals) {
	for date, totals := range src {
		dst[date] = append(dst[date], totals...)
	}
	for date, totals := range dst {
		dst[date] = mergeSameModelTotals(totals)
	}
}

func mergeSameModelTotals(totals []awsBedrockTokenTotals) []awsBedrockTokenTotals {
	byModel := map[string]*awsBedrockTokenTotals{}
	for _, t := range totals {
		cur := byModel[t.Model]
		if cur == nil {
			cp := t
			byModel[t.Model] = &cp
			continue
		}
		cur.Prompt += t.Prompt
		cur.Completion += t.Completion
		cur.HasPrompt = cur.HasPrompt || t.HasPrompt
		cur.HasOutput = cur.HasOutput || t.HasOutput
	}
	out := make([]awsBedrockTokenTotals, 0, len(byModel))
	for _, t := range byModel {
		out = append(out, *t)
	}
	return out
}

func applyBedrockTokenTotals(records []NormalizedCostRecord, totalsByDate map[string][]awsBedrockTokenTotals) {
	recordsByDate := countRecordsByDate(records)
	for i := range records {
		candidates := totalsByDate[records[i].RecordedAt]
		if len(candidates) == 0 {
			continue
		}
		if match, ok := safeBedrockTokenMatch(records[i], recordsByDate[records[i].RecordedAt], candidates); ok {
			applyBedrockTokens(&records[i], match)
		}
	}
}

func safeBedrockTokenMatch(r NormalizedCostRecord, costRows int, candidates []awsBedrockTokenTotals) (awsBedrockTokenTotals, bool) {
	var match *awsBedrockTokenTotals
	for i := range candidates {
		if strings.EqualFold(candidates[i].Model, r.Model) {
			if match != nil {
				return awsBedrockTokenTotals{}, false
			}
			match = &candidates[i]
		}
	}
	if match != nil {
		return *match, true
	}
	if costRows == 1 && len(candidates) == 1 {
		return candidates[0], true
	}
	return awsBedrockTokenTotals{}, false
}

func applyBedrockTokens(r *NormalizedCostRecord, t awsBedrockTokenTotals) {
	r.Model = t.Model
	if t.HasPrompt {
		r.PromptTokens = t.Prompt
	}
	if t.HasOutput {
		r.CompletionTokens = t.Completion
	}
	if t.HasPrompt || t.HasOutput {
		r.TotalTokens = r.PromptTokens + r.CompletionTokens
	}
}

func countRecordsByDate(records []NormalizedCostRecord) map[string]int {
	out := map[string]int{}
	for _, r := range records {
		out[r.RecordedAt]++
	}
	return out
}

func metricModelID(dimensions []awsMetricDimension) string {
	for _, d := range dimensions {
		if strings.EqualFold(d.Name, "ModelId") {
			return d.Value
		}
	}
	return ""
}

func metricTimestampDate(values []json.RawMessage, i int) string {
	if i < 0 || i >= len(values) {
		return ""
	}
	var unix float64
	if err := json.Unmarshal(values[i], &unix); err == nil {
		t := time.Unix(int64(unix), 0).UTC()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(awsBedrockDateLayout)
	}
	var s string
	if err := json.Unmarshal(values[i], &s); err != nil {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC).Format(awsBedrockDateLayout)
}

func joinAWSBedrockErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, len(errs))
	for i, err := range errs {
		parts[i] = err.Error()
	}
	return fmt.Errorf("aws-bedrock cloudwatch: %s", strings.Join(parts, "; "))
}

func (p *AWSBedrockProvider) cloudWatchEndpoint(region string) string {
	if p.CloudWatchURL != "" {
		return strings.TrimRight(p.CloudWatchURL, "/")
	}
	return "https://monitoring." + region + ".amazonaws.com"
}
