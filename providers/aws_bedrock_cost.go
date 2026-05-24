package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	awsCostExplorerService = "ce"
	awsCostExplorerTarget  = "AWSInsightsIndexService.GetCostAndUsage"
	awsCostMaxPages        = 50
)

type awsCostResponse struct {
	ResultsByTime []awsCostResultByTime `json:"ResultsByTime"`
	NextPageToken string                `json:"NextPageToken"`
}

type awsCostResultByTime struct {
	TimePeriod awsCostTimePeriod `json:"TimePeriod"`
	Groups     []awsCostGroup    `json:"Groups"`
}

type awsCostTimePeriod struct {
	Start string `json:"Start"`
	End   string `json:"End"`
}

type awsCostGroup struct {
	Keys    []string       `json:"Keys"`
	Metrics awsCostMetrics `json:"Metrics"`
}

type awsCostMetrics struct {
	UnblendedCost awsCostAmount `json:"UnblendedCost"`
	UsageQuantity awsCostAmount `json:"UsageQuantity"`
}

type awsCostAmount struct {
	Amount string `json:"Amount"`
	Unit   string `json:"Unit"`
}

type awsBedrockCostRow struct {
	Date          string
	Service       string
	UsageType     string
	CostUSD       float64
	UsageQuantity float64
	UsageUnit     string
}

func (p *AWSBedrockProvider) fetchCostRows(ctx context.Context, start, end time.Time) ([]awsBedrockCostRow, error) {
	body := buildAWSBedrockCostBody(start, end, "")
	var out []awsBedrockCostRow
	for page := 0; page < awsCostMaxPages; page++ {
		resp, err := p.doCostRequest(ctx, body)
		if err != nil {
			return nil, wrapAWSBedrockPermissionHint(err, "Cost Explorer")
		}
		out = append(out, decodeAWSBedrockCostRows(resp)...)
		if resp.NextPageToken == "" {
			break
		}
		body = buildAWSBedrockCostBody(start, end, resp.NextPageToken)
	}
	return out, nil
}

func (p *AWSBedrockProvider) doCostRequest(ctx context.Context, body any) (awsCostResponse, error) {
	var decoded awsCostResponse
	payload, err := json.Marshal(body)
	if err != nil {
		return decoded, fmt.Errorf("aws-bedrock cost: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.costEndpoint(), bytes.NewReader(payload))
	if err != nil {
		return decoded, fmt.Errorf("aws-bedrock cost: build request: %w", err)
	}
	setAWSJSONHeaders(req, awsCostExplorerTarget, "application/x-amz-json-1.1", "", p.userAgent())
	if err := p.signRequest(req, payload, p.CostExplorerRegion, awsCostExplorerService); err != nil {
		return decoded, err
	}
	return decoded, decodeAWSSafeResponse(p.httpClient(), req, &decoded, "cost-explorer")
}

func buildAWSBedrockCostBody(start, end time.Time, nextToken string) map[string]any {
	body := map[string]any{
		"TimePeriod": map[string]string{
			"Start": start.UTC().Format("2006-01-02"),
			"End":   end.UTC().Format("2006-01-02"),
		},
		"Granularity": "DAILY",
		"Metrics":     []string{"UnblendedCost", "UsageQuantity"},
		"Filter": map[string]any{
			"Dimensions": map[string]any{
				"Key":    "SERVICE",
				"Values": []string{"Amazon Bedrock"},
			},
		},
		"GroupBy": []map[string]string{
			{"Type": "DIMENSION", "Key": "SERVICE"},
			{"Type": "DIMENSION", "Key": "USAGE_TYPE"},
		},
	}
	if nextToken != "" {
		body["NextPageToken"] = nextToken
	}
	return body
}

func decodeAWSBedrockCostRows(resp awsCostResponse) []awsBedrockCostRow {
	var out []awsBedrockCostRow
	for _, bucket := range resp.ResultsByTime {
		date := formatAWSBedrockCostDate(bucket.TimePeriod.Start)
		if date == "" {
			continue
		}
		for _, group := range bucket.Groups {
			if row, ok := decodeAWSBedrockCostGroup(date, group); ok {
				out = append(out, row)
			}
		}
	}
	return out
}

func decodeAWSBedrockCostGroup(date string, group awsCostGroup) (awsBedrockCostRow, bool) {
	service, usageType := awsCostKeys(group.Keys)
	if !isAWSBedrockService(service) || !strings.EqualFold(group.Metrics.UnblendedCost.Unit, "USD") {
		return awsBedrockCostRow{}, false
	}
	cost, ok := parseAWSAmount(group.Metrics.UnblendedCost.Amount)
	if !ok {
		return awsBedrockCostRow{}, false
	}
	quantity, _ := parseAWSAmount(group.Metrics.UsageQuantity.Amount)
	return awsBedrockCostRow{
		Date:          date,
		Service:       service,
		UsageType:     usageType,
		CostUSD:       cost,
		UsageQuantity: quantity,
		UsageUnit:     group.Metrics.UsageQuantity.Unit,
	}, true
}

func mapAWSBedrockCostRow(row awsBedrockCostRow, accountID string) NormalizedCostRecord {
	return NormalizedCostRecord{
		Provider:          awsBedrockProviderName,
		Model:             row.UsageType,
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       0,
		CostUSD:           row.CostUSD,
		RecordedAt:        row.Date,
		APIKeyID:          "",
		ProjectID:         accountID,
		Product:           row.Service,
		SKU:               row.UsageType,
		UnitType:          row.UsageUnit,
		UsageQuantity:     row.UsageQuantity,
		UnitPriceUSD:      0,
		GrossAmountUSD:    row.CostUSD,
		DiscountAmountUSD: 0,
		SourceHash:        awsBedrockSourceHash(row.Date, accountID, row.Service, row.UsageType, "", ""),
	}
}

func (p *AWSBedrockProvider) costEndpoint() string {
	if p.CostExplorerURL != "" {
		return strings.TrimRight(p.CostExplorerURL, "/")
	}
	return "https://ce." + p.CostExplorerRegion + ".amazonaws.com"
}

func (p *AWSBedrockProvider) signRequest(req *http.Request, body []byte, region, service string) error {
	return awsSigner{Region: region, Service: service, Now: p.now(), Creds: p.credentials()}.sign(req, body)
}

func (p *AWSBedrockProvider) credentials() awsCredentials {
	return awsCredentials{AccessKeyID: p.AccessKeyID, SecretAccessKey: p.SecretAccessKey, SessionToken: p.SessionToken}
}

func decodeAWSSafeResponse(client *http.Client, req *http.Request, out any, service string) error {
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("aws-bedrock %s: %w", service, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return &awsBedrockHTTPError{Service: service, StatusCode: resp.StatusCode, Endpoint: "/"}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("aws-bedrock %s: decode response: %w", service, err)
	}
	return nil
}

func setAWSJSONHeaders(req *http.Request, target, contentType, encoding, userAgent string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("User-Agent", userAgent)
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
}

func awsCostKeys(keys []string) (string, string) {
	if len(keys) == 0 {
		return "", ""
	}
	if len(keys) == 1 {
		return keys[0], ""
	}
	return keys[0], keys[1]
}

func isAWSBedrockService(service string) bool {
	return strings.EqualFold(strings.TrimSpace(service), "Amazon Bedrock")
}

func parseAWSAmount(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil
}

func formatAWSBedrockCostDate(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		return ""
	}
	return t.Format(awsBedrockDateLayout)
}
