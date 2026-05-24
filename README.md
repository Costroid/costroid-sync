# costroid-sync

See what your AI actually costs.

`costroid-sync` is an open-source Go CLI for tracking AI/LLM costs locally across providers. It reads usage and cost metadata from provider admin APIs or billing exports, normalizes it, and stores it in a local SQLite database on your machine.

API keys stay on your machine. Costroid™ does not proxy your model calls, does not receive your provider credentials, and does not store prompt or completion data.

## What It Collects

Metadata only:

- token counts
- model names
- timestamps
- cost amounts
- API key IDs
- project, workspace, resource, account, or billing IDs

Never collected, stored, logged, printed, cached, or transmitted:

- prompts
- completions
- messages
- content
- raw provider payloads
- request or response bodies
- source code
- diagnostic or invocation logs

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

Set credentials for at least one provider. See [Provider Setup](#provider-setup) for the full list.

```sh
export OPENAI_ADMIN_KEY=sk-admin-...
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

| Command | Purpose | Example |
| --- | --- | --- |
| `sync` | Fetch provider usage and cost metadata into local SQLite. | `costroid-sync sync --provider openai --days 7` |
| `savings` | Show local savings recommendations from offline pricing estimates. | `costroid-sync savings` |
| `history` | Show recent local cost records. | `costroid-sync history --last 30d` |
| `trend` | Aggregate local spend by week or month. | `costroid-sync trend --weekly` |
| `forecast` | Forecast current calendar month spend from local daily totals. | `costroid-sync forecast` |
| `anomalies` | List local daily spend spikes above the rolling baseline. | `costroid-sync anomalies` |
| `budget` | Set or check a local spending budget. | `costroid-sync budget --set 500 --period monthly` |
| `export` | Export local metadata records to stdout. | `costroid-sync export --format csv > costs.csv` |
| `version` | Print the CLI version. | `costroid-sync version` |

Supported `sync --provider` values: `openai`, `anthropic`, `github-copilot` (alias `copilot`), `google-gemini` (alias `gemini`), `gcp-billing` (alias `gcp`), `azure-openai`, `aws-bedrock` (alias `bedrock`), `all`. Defaults to `openai`. With `--provider all`, only providers with their environment variables set are queried; others are skipped with a note.

## Provider Setup

Costroid reads only provider billing and usage metadata. Provider secrets stay in your shell and process.

| Provider | Slug and aliases | Data source | Required env vars | Docs |
| --- | --- | --- | --- | --- |
| OpenAI | `openai` | OpenAI organization usage/cost metadata APIs | `OPENAI_ADMIN_KEY` | [docs/providers/openai.md](docs/providers/openai.md) |
| Anthropic | `anthropic` | Anthropic admin usage/cost metadata APIs | `ANTHROPIC_ADMIN_KEY` | [docs/providers/anthropic.md](docs/providers/anthropic.md) |
| GitHub Copilot | `github-copilot`, `copilot` | GitHub premium request billing metadata | `GITHUB_PAT`, `GITHUB_ORG` | [docs/providers/github-copilot.md](docs/providers/github-copilot.md) |
| Google Gemini | `google-gemini`, `gemini` | Google Cloud Billing CSV export | `GEMINI_BILLING_EXPORT` | [docs/providers/google-gemini.md](docs/providers/google-gemini.md) |
| Azure OpenAI | `azure-openai` | Azure Cost Management, optional Azure Monitor token metrics | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SUBSCRIPTION_ID` | [docs/providers/azure-openai.md](docs/providers/azure-openai.md) |
| AWS Bedrock | `aws-bedrock`, `bedrock` | AWS Cost Explorer, optional CloudWatch token metrics | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | [docs/providers/aws-bedrock.md](docs/providers/aws-bedrock.md) |
| GCP Billing | `gcp-billing`, `gcp` | BigQuery Cloud Billing detailed export via REST | `GCP_SERVICE_ACCOUNT_JSON`, `GCP_BILLING_PROJECT`, `GCP_BILLING_TABLE` | [docs/providers/gcp-billing.md](docs/providers/gcp-billing.md) |

Provider docs index: [docs/providers/README.md](docs/providers/README.md)

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

## Export Formats

Supported export formats:

- `csv`
- `json`
- `focus`
- `markdown`

## Security And Privacy

- No proxy: your model traffic does not pass through Costroid.
- No prompt reading: provider integrations extract only cost metadata fields.
- No prompt storage: prompts, completions, messages, content, and raw payloads are never stored.
- No request or response body storage: Costroid stores normalized records only.
- No credentials on Costroid servers: provider admin keys remain local to your shell and process.
- Local SQLite only: open-core analytics run on your machine.

## Pricing Note

Savings recommendations use static seed pricing for offline estimates. They are useful for directionally comparing model costs, but they are not a live provider pricing or availability guarantee.

## Build From Source

```sh
go build -o costroid-sync .
./costroid-sync --version
```

Because `go-sqlite3` uses CGO, source builds require a working C compiler.
