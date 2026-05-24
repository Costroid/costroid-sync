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

### OpenAI

`costroid-sync` requires an OpenAI Admin API key for usage and cost endpoints.

```sh
export OPENAI_ADMIN_KEY=sk-admin-...
```

Normal project API keys may not work for organization usage and cost APIs.

### Anthropic

`costroid-sync` requires an Anthropic Admin API key.

```sh
export ANTHROPIC_ADMIN_KEY=sk-ant-admin-...
```

Normal Anthropic API keys may not work for admin usage and cost APIs.

### GitHub Copilot

`costroid-sync` reads Copilot premium-request billing metadata from your
organization. It requires two environment variables:

```sh
export GITHUB_PAT=ghp_...
export GITHUB_ORG=your-org
```

The token must have organization billing / premium-request usage read
permission:

- Fine-grained PAT: Administration: Read at organization scope.
- Classic PAT: scopes vary by org/enterprise setup.

Premium-request billing data may be unavailable depending on your account
plan, organization permissions, or billing-platform eligibility. If
`costroid-sync` returns a permission error, double-check the token scope,
the org slug, and whether premium-request billing is enabled for your
account.

Costroid extracts billing metadata only — no Copilot prompts, completions,
chat content, code completions, source code, repositories, issues, or PRs
are read or stored.

The `sync --days N` flag issues one daily-billing query per UTC day
(clamped to 31). For longer windows, run multiple syncs across separate
days. Today's billing data may be partial or empty depending on GitHub's
processing lag.

### Google Gemini

`costroid-sync` reads Gemini billing metadata from a Google Cloud Billing
export file. Google's public REST APIs do not expose detailed per-SKU
Gemini usage directly; the official detailed-billing path is BigQuery
billing export.

Setup:

1. In the Google Cloud Console, enable Cloud Billing export to BigQuery
   for your billing account.
2. In BigQuery, query the billing export table for the date range you
   want and export the results to CSV. You can filter to Gemini SKUs
   yourself, or let Costroid do the filtering.
3. Set:

   ```sh
   export GEMINI_BILLING_EXPORT=/path/to/google-billing-export.csv
   # Optional — filter rows by GCP project ID:
   export GEMINI_BILLING_PROJECT=your-gcp-project-id
   # Optional — override the default Gemini service-name match
   # (comma-separated substrings):
   export GEMINI_BILLING_SERVICE_FILTER="gemini,vertex"
   ```

4. Sync:

   ```sh
   costroid-sync sync --provider google-gemini --days 30
   # or:
   costroid-sync sync --provider gemini --days 30
   ```

What gets stored:

- Cost, SKU, product, usage quantity, unit type, project ID,
  usage_start_time — billing metadata only.
- Gemini API prompts, completions, chat content, code, repository data,
  user text, or model responses are NEVER read or stored. Costroid does
  not call Gemini generation APIs.
- Even if your CSV export includes `labels` or `system_labels` columns,
  Costroid skips them entirely (they can contain free-form text).

Caveats:

- Only USD rows are imported. Non-USD rows are silently skipped.
- Cloud Billing data is typically delayed several hours after the actual
  API call. Re-run `sync` with a wider `--days` window to pick up late
  arrivals.
- Only one CSV file per sync. For larger histories, concatenate first or
  run multiple syncs (UPSERT keeps results clean).

### Azure OpenAI

`costroid-sync` reads Azure OpenAI cost metadata from the Azure Cost
Management Query API. When you provide your Azure OpenAI resource IDs,
it additionally enriches cost records with token counts from Azure
Monitor metrics.

Setup:

1. In Azure, create a service principal (Microsoft Entra ID application
   registration) for `costroid-sync`. Assign it:
   - `Cost Management Reader` on the subscription (or whatever scope you
     plan to query).
   - Optional: `Monitoring Reader` on each Azure OpenAI resource you
     want token enrichment for.
2. Set:

   ```sh
   export AZURE_TENANT_ID=...
   export AZURE_CLIENT_ID=...
   export AZURE_CLIENT_SECRET=...
   export AZURE_SUBSCRIPTION_ID=...
   # Optional — override the cost-management scope:
   export AZURE_COST_SCOPE=subscriptions/<id>
   # Optional — enable per-resource token enrichment via Azure Monitor:
   export AZURE_OPENAI_RESOURCE_IDS=/subscriptions/.../resourceGroups/.../providers/Microsoft.CognitiveServices/accounts/...
   ```

3. Sync:

   ```sh
   costroid-sync sync --provider azure-openai --days 30
   ```

What gets stored:

- Cost, meter/SKU, product/service name, usage quantity, unit type,
  resource ID/group, subscription scope, day — billing metadata only.
- When `AZURE_OPENAI_RESOURCE_IDS` is set and Azure Monitor metrics can
  be safely matched to a cost row, prompt/completion/total token counts
  are populated from the `ProcessedPromptTokens`, `GeneratedTokens`, and
  `TotalTokens` metrics.
- Azure OpenAI prompts, completions, chat content, messages, function
  arguments, tool call text, request bodies, response bodies, or
  diagnostic logs are NEVER read or stored. Costroid does not call Azure
  OpenAI generation APIs.

Caveats:

- Only USD rows are imported. Non-USD rows are silently skipped.
- Cost Management data typically lags actual usage by several hours.
  Re-run `sync` with a wider `--days` window to pick up late arrivals.
- A single Azure OpenAI resource can host multiple model deployments.
  When the cost meter does not uniquely identify a deployment, Costroid
  leaves token counts at 0 rather than guessing — Cost Management cost
  remains authoritative. To get full token attribution, deploy each
  model in a separate Azure OpenAI resource (or accept zero-token rows
  for shared-resource deployments).
- Request count metrics from Azure Monitor are NOT mapped to tokens.
  Tokens are tokens; requests are requests; conflating them would be
  misleading.

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
