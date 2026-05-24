package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsCloudWatchService    = "monitoring"
	awsCloudWatchListTarget = "GraniteServiceVersion20100801.ListMetrics"
	awsCloudWatchDataTarget = "GraniteServiceVersion20100801.GetMetricData"
	awsCloudWatchMaxPages   = 20
	awsCloudWatchMaxModels  = 100
)

type awsListMetricsResponse struct {
	Metrics   []awsCloudWatchMetric `json:"Metrics"`
	NextToken string                `json:"NextToken"`
}

type awsCloudWatchMetric struct {
	MetricName string               `json:"MetricName"`
	Namespace  string               `json:"Namespace"`
	Dimensions []awsMetricDimension `json:"Dimensions"`
}

type awsMetricDimension struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type awsMetricDataResponse struct {
	MetricDataResults []awsMetricDataResult `json:"MetricDataResults"`
}

type awsMetricDataResult struct {
	ID         string            `json:"Id"`
	Label      string            `json:"Label"`
	Timestamps []json.RawMessage `json:"Timestamps"`
	Values     []float64         `json:"Values"`
}

type awsBedrockTokenTotals struct {
	Model      string
	Prompt     int
	Completion int
	HasPrompt  bool
	HasOutput  bool
}

func enrichWithBedrockCloudWatch(ctx context.Context, p *AWSBedrockProvider, records []NormalizedCostRecord, start, end time.Time) error {
	totalsByDate := map[string][]awsBedrockTokenTotals{}
	var fetchErrors []error
	for _, region := range p.MetricRegions {
		totals, err := p.fetchCloudWatchTokens(ctx, region, start, end)
		if err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("aws-bedrock cloudwatch %s: %w", region, err))
			continue
		}
		mergeBedrockTokenTotals(totalsByDate, totals)
	}
	applyBedrockTokenTotals(records, totalsByDate)
	if len(fetchErrors) > 0 {
		return joinAWSBedrockErrors(fetchErrors)
	}
	return nil
}

func (p *AWSBedrockProvider) fetchCloudWatchTokens(ctx context.Context, region string, start, end time.Time) (map[string][]awsBedrockTokenTotals, error) {
	models, err := p.listBedrockMetricModels(ctx, region)
	if err != nil || len(models) == 0 {
		return nil, err
	}
	if len(models) > awsCloudWatchMaxModels {
		models = models[:awsCloudWatchMaxModels]
	}
	return p.getBedrockMetricData(ctx, region, models, start, end)
}

func (p *AWSBedrockProvider) listBedrockMetricModels(ctx context.Context, region string) ([]string, error) {
	seen := map[string]bool{}
	for _, metric := range []string{"InputTokenCount", "OutputTokenCount"} {
		if err := p.collectMetricModels(ctx, region, metric, seen); err != nil {
			return nil, err
		}
	}
	models := make([]string, 0, len(seen))
	for model := range seen {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

func (p *AWSBedrockProvider) collectMetricModels(ctx context.Context, region, metric string, seen map[string]bool) error {
	body := map[string]any{"Namespace": "AWS/Bedrock", "MetricName": metric}
	for page := 0; page < awsCloudWatchMaxPages; page++ {
		var resp awsListMetricsResponse
		if err := p.doCloudWatchRequest(ctx, region, awsCloudWatchListTarget, body, &resp); err != nil {
			return wrapAWSBedrockPermissionHint(err, "CloudWatch")
		}
		for _, m := range resp.Metrics {
			if model := metricModelID(m.Dimensions); model != "" {
				seen[model] = true
			}
		}
		if resp.NextToken == "" {
			break
		}
		body["NextToken"] = resp.NextToken
	}
	return nil
}

func (p *AWSBedrockProvider) getBedrockMetricData(ctx context.Context, region string, models []string, start, end time.Time) (map[string][]awsBedrockTokenTotals, error) {
	queries, idMap := buildBedrockMetricQueries(models)
	body := map[string]any{
		"StartTime":         start.UTC().Unix(),
		"EndTime":           end.UTC().Unix(),
		"MetricDataQueries": queries,
	}
	var resp awsMetricDataResponse
	if err := p.doCloudWatchRequest(ctx, region, awsCloudWatchDataTarget, body, &resp); err != nil {
		return nil, wrapAWSBedrockPermissionHint(err, "CloudWatch")
	}
	return decodeBedrockMetricData(resp, idMap), nil
}

func (p *AWSBedrockProvider) doCloudWatchRequest(ctx context.Context, region, target string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("aws-bedrock cloudwatch: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cloudWatchEndpoint(region), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("aws-bedrock cloudwatch: build request: %w", err)
	}
	setAWSJSONHeaders(req, target, "application/json", "amz-1.0", p.userAgent())
	if err := p.signRequest(req, payload, region, awsCloudWatchService); err != nil {
		return err
	}
	return decodeAWSSafeResponse(p.httpClient(), req, out, "cloudwatch")
}

func buildBedrockMetricQueries(models []string) ([]map[string]any, map[string]awsMetricQueryInfo) {
	var queries []map[string]any
	idMap := map[string]awsMetricQueryInfo{}
	for i, model := range models {
		for _, metric := range []string{"InputTokenCount", "OutputTokenCount"} {
			id := fmt.Sprintf("%s%d", strings.ToLower(metric[:1]), i)
			idMap[id] = awsMetricQueryInfo{Model: model, Metric: metric}
			queries = append(queries, bedrockMetricQuery(id, model, metric))
		}
	}
	return queries, idMap
}

type awsMetricQueryInfo struct {
	Model  string
	Metric string
}

func bedrockMetricQuery(id, model, metric string) map[string]any {
	return map[string]any{
		"Id":         id,
		"ReturnData": true,
		"MetricStat": map[string]any{
			"Period": 86400,
			"Stat":   "Sum",
			"Metric": map[string]any{
				"Namespace":  "AWS/Bedrock",
				"MetricName": metric,
				"Dimensions": []map[string]string{{"Name": "ModelId", "Value": model}},
			},
		},
	}
}

func decodeBedrockMetricData(resp awsMetricDataResponse, idMap map[string]awsMetricQueryInfo) map[string][]awsBedrockTokenTotals {
	byDateModel := map[string]*awsBedrockTokenTotals{}
	for _, result := range resp.MetricDataResults {
		info, ok := idMap[result.ID]
		if !ok {
			continue
		}
		for i, value := range result.Values {
			date := metricTimestampDate(result.Timestamps, i)
			if date == "" {
				continue
			}
			key := date + "|" + info.Model
			addBedrockMetricValue(byDateModel, key, info, value)
		}
	}
	return groupTokenTotalsByDate(byDateModel)
}

func addBedrockMetricValue(out map[string]*awsBedrockTokenTotals, key string, info awsMetricQueryInfo, value float64) {
	total := out[key]
	if total == nil {
		total = &awsBedrockTokenTotals{Model: info.Model}
		out[key] = total
	}
	if info.Metric == "InputTokenCount" {
		total.Prompt += int(value)
		total.HasPrompt = true
	} else if info.Metric == "OutputTokenCount" {
		total.Completion += int(value)
		total.HasOutput = true
	}
}
