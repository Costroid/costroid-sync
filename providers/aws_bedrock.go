package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	awsBedrockProviderName = "aws-bedrock"

	envAWSAccessKeyID        = "AWS_ACCESS_KEY_ID"
	envAWSSecretAccessKey    = "AWS_SECRET_ACCESS_KEY"
	envAWSSessionToken       = "AWS_SESSION_TOKEN"
	envAWSRegion             = "AWS_REGION"
	envAWSBedrockRegions     = "AWS_BEDROCK_REGIONS"
	envAWSAccountID          = "AWS_ACCOUNT_ID"
	envAWSCostExplorerRegion = "AWS_COST_EXPLORER_REGION"

	awsBedrockDefaultRegion = "us-east-1"
	awsBedrockMaxDays       = 366
	awsBedrockUserAgentDev  = "costroid-sync/dev"
	awsBedrockDateLayout    = "2006-01-02T15:04:05Z"
)

type AWSBedrockConfig struct {
	AccessKeyID        string
	SecretAccessKey    string
	SessionToken       string
	Region             string
	CostExplorerRegion string
	AccountID          string
	MetricRegions      []string
	CostExplorerURL    string
	CloudWatchURL      string
}

// AWSBedrockProvider fetches Amazon Bedrock billing metadata from Cost
// Explorer. Optional CloudWatch enrichment reads only aggregate token
// metrics. It never calls Bedrock runtime APIs, CloudWatch Logs, or any
// API that can expose prompts, completions, messages, content, or raw
// invocation payloads.
type AWSBedrockProvider struct {
	AccessKeyID        string
	SecretAccessKey    string
	SessionToken       string
	Region             string
	CostExplorerRegion string
	AccountID          string
	MetricRegions      []string

	CostExplorerURL string
	CloudWatchURL   string
	HTTPClient      *http.Client
	UserAgent       string
	Now             func() time.Time
}

var _ Provider = (*AWSBedrockProvider)(nil)

func NewAWSBedrockProvider(cfg AWSBedrockConfig) *AWSBedrockProvider {
	region := firstNonEmpty(cfg.Region, awsBedrockDefaultRegion)
	costRegion := firstNonEmpty(cfg.CostExplorerRegion, awsBedrockDefaultRegion)
	return &AWSBedrockProvider{
		AccessKeyID:        cfg.AccessKeyID,
		SecretAccessKey:    cfg.SecretAccessKey,
		SessionToken:       cfg.SessionToken,
		Region:             region,
		CostExplorerRegion: costRegion,
		AccountID:          cfg.AccountID,
		MetricRegions:      cfg.MetricRegions,
		CostExplorerURL:    cfg.CostExplorerURL,
		CloudWatchURL:      cfg.CloudWatchURL,
		HTTPClient:         &http.Client{Timeout: 30 * time.Second},
		UserAgent:          awsBedrockUserAgentDev,
	}
}

func (p *AWSBedrockProvider) Name() string { return awsBedrockProviderName }

func (p *AWSBedrockProvider) Fetch(ctx context.Context, days int) ([]NormalizedCostRecord, error) {
	days = clampAWSBedrockDays(days)
	start, end := p.costWindow(days)
	rows, err := p.fetchCostRows(ctx, start, end)
	if err != nil {
		return nil, err
	}

	records := make([]NormalizedCostRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, mapAWSBedrockCostRow(row, p.AccountID))
	}
	if len(p.MetricRegions) > 0 {
		_ = enrichWithBedrockCloudWatch(ctx, p, records, start, end)
	}
	sortAWSBedrockRecords(records)
	return records, nil
}

func (p *AWSBedrockProvider) costWindow(days int) (time.Time, time.Time) {
	now := p.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -days+1), today.AddDate(0, 0, 1)
}

func (p *AWSBedrockProvider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *AWSBedrockProvider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (p *AWSBedrockProvider) userAgent() string {
	if p.UserAgent != "" {
		return p.UserAgent
	}
	return awsBedrockUserAgentDev
}

func clampAWSBedrockDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > awsBedrockMaxDays {
		return awsBedrockMaxDays
	}
	return days
}

func parseAWSRegions(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		region := strings.ToLower(strings.TrimSpace(part))
		if region == "" || seen[region] {
			continue
		}
		seen[region] = true
		out = append(out, region)
	}
	return out
}

func awsBedrockSourceHash(date, accountID, service, usageType, operation, region string) string {
	h := sha256.New()
	fmt.Fprintf(h, "aws-bedrock|%s|%s|%s|%s|%s|%s",
		date, accountID, service, usageType, operation, region)
	return hex.EncodeToString(h.Sum(nil))
}

func sortAWSBedrockRecords(records []NormalizedCostRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].RecordedAt != records[j].RecordedAt {
			return records[i].RecordedAt < records[j].RecordedAt
		}
		if records[i].ProjectID != records[j].ProjectID {
			return records[i].ProjectID < records[j].ProjectID
		}
		if records[i].SKU != records[j].SKU {
			return records[i].SKU < records[j].SKU
		}
		return records[i].Model < records[j].Model
	})
}
