# AWS Bedrock

## Provider Identity

- Provider name: AWS Bedrock
- CLI provider slug: `aws-bedrock`
- Supported aliases: `bedrock`
- Generic `aws` alias: Not supported

## Data Source

Costroid uses AWS Cost Explorer for spend. CloudWatch token metrics are optional enrichment only when `AWS_BEDROCK_REGIONS` is set.

## Required Environment Variables

```sh
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

## Optional Environment Variables

```sh
export AWS_SESSION_TOKEN=...
export AWS_REGION=us-east-1
export AWS_BEDROCK_REGIONS=us-east-1,us-west-2
export AWS_ACCOUNT_ID=123456789012
export AWS_COST_EXPLORER_REGION=us-east-1
```

## Minimal Setup Steps

1. Create or choose AWS credentials with Cost Explorer access.
2. Export `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
3. Optionally export `AWS_SESSION_TOKEN` for temporary credentials.
4. Optionally export `AWS_COST_EXPLORER_REGION`; it defaults to `us-east-1`.
5. Optionally export `AWS_BEDROCK_REGIONS` for CloudWatch token metric enrichment.
6. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid-sync sync --provider aws-bedrock --days 30
```

The alias is also supported:

```sh
costroid-sync sync --provider bedrock --days 30
```

## Permission Notes

Credentials need `ce:GetCostAndUsage` for Cost Explorer. If `AWS_BEDROCK_REGIONS` is set, credentials also need `cloudwatch:ListMetrics` and `cloudwatch:GetMetricData` for token metric enrichment.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from AWS. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- No Bedrock `InvokeModel`, `Converse`, or runtime APIs are called.
- CloudWatch Logs and invocation logging are not used.
- Cost Explorer is the authoritative spend source.
- Cost Explorer `UsageQuantity` is billing metadata only, not token count.
- Token counts can be zero when CloudWatch metrics are unavailable, not configured, or cannot be safely joined to a cost row.
- Only USD Cost Explorer rows are imported.
