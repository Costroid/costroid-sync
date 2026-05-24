# costroid-sync

See what your AI actually costs.

`costroid-sync` is an open-source Go CLI for tracking AI/LLM costs locally across providers. It reads usage and cost metadata from provider admin APIs, normalizes it, and stores it in a local SQLite database on your machine.

API keys stay on your machine. Costroid™ does not proxy your model calls, does not receive your provider credentials, and does not store prompt or completion data.

## What It Collects

Metadata only:

- token counts
- model names
- timestamps
- cost amounts
- API key IDs
- project or workspace IDs

Never collected, stored, logged, printed, cached, or transmitted:

- prompts
- completions
- messages
- content
- raw provider payloads
- request or response bodies

## Install

### Install Script

Linux prebuilt binaries are available for `amd64` and `arm64`.

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh | sh
```

To install a specific release:

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid-sync/main/install.sh | VERSION=vX.Y.Z sh
```

macOS and Windows prebuilt binaries are not available yet because this CLI uses `go-sqlite3`, which requires CGO. Use `go install` on those platforms for now.

### Go Install

```sh
go install github.com/costroid/costroid-sync@latest
```

### Manual Download

Download a release archive from:

```text
https://github.com/Costroid/costroid-sync/releases
```

Extract the archive and place `costroid-sync` somewhere on your `PATH`, such as `/usr/local/bin`.

## Quick Start

Set credentials for at least one provider:

```sh
export OPENAI_ADMIN_KEY=sk-admin-...
# or
export ANTHROPIC_ADMIN_KEY=sk-ant-admin-...
# or, for GitHub Copilot premium-request billing:
export GITHUB_PAT=ghp_...
export GITHUB_ORG=your-org
# or, for Amazon Bedrock cost metadata:
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

Sync recent usage:

```sh
costroid-sync sync --provider openai --days 7
```

Review savings and forecasts from local metadata:

```sh
costroid-sync savings
costroid-sync forecast
```

## Commands

### `sync`

Fetches provider usage and cost metadata and saves normalized local records.

```sh
costroid-sync sync --provider openai --days 7
costroid-sync sync --provider anthropic --days 7
costroid-sync sync --provider github-copilot --days 7
costroid-sync sync --provider copilot --days 7        # alias for github-copilot
costroid-sync sync --provider aws-bedrock --days 30
costroid-sync sync --provider bedrock --days 30       # alias for aws-bedrock
costroid-sync sync --provider all --days 30
```

`--provider` defaults to `openai`. With `--provider all`, only providers with their environment variables set are queried; others are skipped with a note.

### `savings`

Shows local savings recommendations based on seeded offline pricing estimates.

```sh
costroid-sync savings
```

### `history`

Shows recent local cost records.

```sh
costroid-sync history --last 30d
```

### `trend`

Aggregates local spend by week or month.

```sh
costroid-sync trend --weekly
costroid-sync trend --monthly
```

### `forecast`

Forecasts current calendar month spend from local daily totals.

```sh
costroid-sync forecast
```

### `anomalies`

Lists local daily spend spikes above the rolling baseline.

```sh
costroid-sync anomalies
```

### `budget`

Sets or checks a local spending budget.

```sh
costroid-sync budget --set 500 --period monthly
costroid-sync budget
costroid-sync budget --period weekly
```

### `export`

Exports local metadata records to stdout. Redirect stdout to a file.

```sh
costroid-sync export --format csv > costs.csv
costroid-sync export --format json > costs.json
costroid-sync export --format focus > costs-focus.csv
costroid-sync export --format markdown > costs.md
```

Supported formats:

- `csv`
- `json`
- `focus`
- `markdown`

### `version`

Prints the CLI version.

```sh
costroid-sync version
```

## Provider Setup

Costroid reads only provider billing and usage metadata. Provider secrets
stay in your shell and process.

```sh
# OpenAI organization usage/cost APIs
export OPENAI_ADMIN_KEY=sk-admin-...

# Anthropic organization usage/cost APIs
export ANTHROPIC_ADMIN_KEY=sk-ant-admin-...

# GitHub Copilot premium-request billing
export GITHUB_PAT=ghp_...
export GITHUB_ORG=your-org

# Google Gemini Cloud Billing export CSV
export GEMINI_BILLING_EXPORT=/path/to/google-billing-export.csv
export GEMINI_BILLING_PROJECT=your-gcp-project-id # optional
export GEMINI_BILLING_SERVICE_FILTER="gemini,vertex" # optional

# Azure OpenAI Cost Management, optional Azure Monitor token metrics
export AZURE_TENANT_ID=...
export AZURE_CLIENT_ID=...
export AZURE_CLIENT_SECRET=...
export AZURE_SUBSCRIPTION_ID=...
export AZURE_COST_SCOPE=subscriptions/<id> # optional
export AZURE_OPENAI_RESOURCE_IDS=/subscriptions/.../Microsoft.CognitiveServices/accounts/... # optional

# Amazon Bedrock Cost Explorer, optional CloudWatch token metrics
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_SESSION_TOKEN=... # optional
export AWS_ACCOUNT_ID=123456789012 # optional metadata
export AWS_COST_EXPLORER_REGION=us-east-1 # optional, defaults to us-east-1
export AWS_BEDROCK_REGIONS=us-east-1,us-west-2 # optional token enrichment
```

Provider notes:

- GitHub Copilot requires organization billing / premium-request usage
  read permission; the `copilot` alias maps to `github-copilot`.
- Google Gemini imports exported Cloud Billing CSV rows only. It skips
  `labels` and `system_labels` because they can contain free-form text.
- Azure OpenAI uses Cost Management for spend and optional Azure Monitor
  metrics for tokens. Request counts are never mapped to tokens.
- Amazon Bedrock uses Cost Explorer as the authoritative spend source
  and optional CloudWatch `InputTokenCount` / `OutputTokenCount` metrics
  for token enrichment. `bedrock` maps to `aws-bedrock`; there is no
  generic `aws` alias.
- Cost Explorer `UsageQuantity` is billing metadata only and is never
  mapped into prompt, completion, or total token fields.
- Token counts can be zero when metrics are unavailable, not configured,
  or cannot be safely joined to an authoritative cost row.
- Bedrock InvokeModel, Converse, runtime, invocation logging, CloudWatch
  Logs, prompt/completion/message/content/request/response/raw payload
  APIs are never called.
- Only USD rows are imported for Gemini, Azure OpenAI, and AWS Bedrock.

## Local Storage

Default database:

```text
~/.costroid/costroid.db
```

Override with:

```sh
export COSTROID_DB=/path/to/costroid.db
```

The database stores normalized metadata records and local budget settings only.

## Security And Privacy

- No proxy: your model traffic does not pass through Costroid.
- No prompt reading: provider integrations extract only cost metadata fields.
- No prompt storage: prompts, completions, messages, content, and raw payloads are never stored.
- No credentials on Costroid servers: provider admin keys remain local to your shell and process.
- Local SQLite only: open-core analytics run on your machine.

## Pricing Note

Savings recommendations use static seed pricing for offline estimates. They are useful for directionally comparing model costs, but they are not a live provider pricing or availability guarantee.

## Demo

Example terminal output:

```text
$ costroid-sync sync --provider openai --days 7
Provider  Model   Tokens  Cost
openai    gpt-4o  152400  $2.3819

$ costroid-sync forecast
Metric                Value
Current month spend   $24.5180
Forecast month-end    $39.2264
Method                linear_regression
Days observed         18
Days remaining        12
```

## Build From Source

```sh
go build -o costroid-sync .
./costroid-sync --version
```

Because `go-sqlite3` uses CGO, source builds require a working C compiler.
