# costroid

See what your AI actually costs.

`costroid` is an open-source Go CLI for tracking AI/LLM costs locally across providers. It reads usage and cost metadata from provider admin APIs or billing exports, normalizes it, and stores it in a local SQLite database on your machine.

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

See [docs/install.md](docs/install.md) for OS-specific details, troubleshooting, PATH notes, and checksum verification.

### Linux and WSL (prebuilt)

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid/main/install.sh | sh
```

Supports `amd64` and `arm64`. The script verifies the SHA256 checksum of the downloaded archive. Pin a release with `VERSION=vX.Y.Z`:

```sh
curl -fsSL https://raw.githubusercontent.com/Costroid/costroid/main/install.sh | VERSION=vX.Y.Z sh
```

### macOS (from source)

No prebuilt macOS binary yet: `costroid` uses `go-sqlite3`, which requires CGO and isn't cross-compiled from the current Linux release runner. Install via Go:

```sh
xcode-select --install
go install github.com/costroid/costroid@latest
```

### Windows (from source via PowerShell)

```powershell
irm https://raw.githubusercontent.com/Costroid/costroid/main/install.ps1 | iex
```

Builds from source via `go install`; requires Go 1.24+ and a C compiler (MinGW-w64 or msys2). No prebuilt Windows binary yet.

### All platforms (Go fallback)

```sh
go install github.com/costroid/costroid@latest
```

### Manual download (Linux)

Download the tarball matching your architecture from <https://github.com/Costroid/costroid/releases>, extract `costroid`, and place it somewhere on your `PATH`.

## Quick Start

Set credentials for at least one provider. See [Provider Setup](#provider-setup) for the full list.

```sh
export OPENAI_ADMIN_KEY=sk-admin-...
```

Sync recent usage:

```sh
costroid sync --provider openai --days 7
```

Review savings and forecasts from local metadata:

```sh
costroid savings
costroid forecast
```

Or just run `costroid` with no arguments to open the interactive dashboard over the same
local data:

```sh
costroid
```

## Commands

| Command | Purpose | Example |
| --- | --- | --- |
| `sync` | Fetch provider usage and cost metadata into local SQLite. | `costroid sync --provider openai --days 7` |
| `savings` | Show local savings recommendations from offline pricing estimates. | `costroid savings` |
| `history` | Show recent local cost records. | `costroid history --last 30d` |
| `trend` | Aggregate local spend by week or month. | `costroid trend --weekly` |
| `forecast` | Forecast current calendar month spend from local daily totals. | `costroid forecast` |
| `anomalies` | List local daily spend spikes above the rolling baseline. | `costroid anomalies` |
| `budget` | Set or check a local spending budget. | `costroid budget --set 500 --period monthly` |
| `export` | Export local metadata records to stdout. | `costroid export --format csv > costs.csv` |
| `statusline` | Print a one-line local cost summary for tmux/Byobu/shell status bars. | `costroid statusline --format tmux` |
| `version` | Print the CLI version. | `costroid version` |

Running `costroid` with **no arguments** opens the interactive fullscreen dashboard (see [TUI](#tui)). In a pipe, in CI, or under `TERM=dumb` it prints help instead.

Supported `sync --provider` values: `openai`, `anthropic`, `github-copilot` (alias `copilot`), `google-gemini` (alias `gemini`), `gcp-billing` (alias `gcp`), `azure-openai`, `aws-bedrock` (alias `bedrock`), `all`. Defaults to `openai`. With `--provider all`, only providers with their environment variables set are queried; others are skipped with a note.

`sync --tui` is an opt-in animated progress view of the real sync stages (fetch → write → analyze) for interactive terminals. It does the same work as a normal sync and prints the same summary at the end. In a pipe, in CI, under `TERM=dumb`, or with `--no-animation`, it falls back to the plain deterministic `sync` output.

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

## Statusline

`costroid statusline` prints a single deterministic line summarising your local cost
metadata, for tmux/Byobu/shell status bars:

```text
⣿ costroid  MTD $38.00  forecast $99.39  budget 60%  anomalies 1  last sync 4h
```

It reads the local SQLite database **read-only** and makes **no** network request, provider API
call, or provider sync — run `costroid sync` separately on your own schedule. It always
prints one line and exits `0`; when there is no local data yet it prints
`costroid  no local data  run costroid sync`.

```sh
costroid statusline --format plain   # default; one line, ⣿ glyph when the locale is UTF-8
costroid statusline --format tmux    # adds tmux #[fg=...] style codes
costroid statusline --format byobu   # adds ANSI color for a Byobu status script
costroid statusline --format json    # stable machine-readable object for scripts
costroid statusline --plain          # force ASCII glyph + no color, regardless of --format
```

`--plain` and a non-empty `NO_COLOR` both suppress color. tmux and Byobu own the polling cadence —
there is no watch process or daemon:

```tmux
# tmux: ~/.tmux.conf
set -g status-interval 60
set -g status-right "#(costroid statusline --format tmux) %H:%M"
```

```bash
# Byobu: ~/.byobu/bin/60_costroid  (the numeric prefix sets the interval)
#!/usr/bin/env bash
costroid statusline --format byobu
```

## TUI

Running `costroid` with no arguments opens a keyboard-first fullscreen dashboard (alternate
screen) of your local cost metadata, with panels for Overview, Providers, Models, Budget,
Forecast, Anomalies, History, Trend, Recent Syncs, and Export hints:

```text
⣿ ⠉⠕⠎⠞⠗⠕⠊⠙ costroid · MTD $38.00 · forecast $99.39 · last 45d
●ovw ·prov ·models ·budget ·fcast ·anom ·hist ·trend ·syncs ·export
┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄
Overview
  Month to date   $38.00   ▁▂▃▄▅▆▇
  Forecast        $99.39
  Budget          ███████▎····  60% (monthly)
  Anomalies       1  ALERT  ⡟
  Last sync       4h ago
```

Every local feature is reachable from the dashboard: the **History** panel shows recent daily
spend, **Trend** shows weekly and monthly rollups, and Forecast, Budget, Anomalies, savings
(under Export hints), and per-provider/model totals each get a panel.

The dashboard and `sync --tui` share a dot/braille identity but read in distinct hues: the
dashboard uses a **cold** cyan-blue palette and `sync --tui` a **warm** coral-amber one, with
Signal green reserved for money and the brand mark on both. Color is layered on top of shape,
never instead of it — the selected panel is marked by a filled dot (`●`), spend shares and budgets
render as braille/block meters, anomaly severity as escalating braille dot-density, and a static
sparkline shows recent daily spend. Truecolor only enhances: the palette degrades
truecolor → 256 → 16-color → monochrome, and under `--plain`, `NO_COLOR`, or a non-UTF-8 locale it
falls back to ASCII (`*`/`.` nav dots, `[####----]` meters, numbers in place of the sparkline) so
the layout reads with zero color. Money values are always stable — never animated.

Like `statusline`, it reads the local SQLite database **read-only** and makes **no** network
request, provider API call, or provider sync — run `costroid sync` separately. It changes no
other command's output.

```sh
costroid                      # open the dashboard
costroid --plain              # ASCII-only glyphs, no color
```

Navigation: `Tab`/`Shift-Tab` or `1`–`9` and `0` switch panels, `j`/`k` (or arrows) scroll, `g`/`G`
jump to top/bottom, `?` toggles help, and `q` (or `Ctrl-C`) quits — the terminal is always restored
on exit. `NO_COLOR` and `--plain` suppress color. In a pipe or non-interactive terminal (or
`TERM=dumb`) `costroid` prints help instead of painting an alternate screen into a pipe; use
`statusline` or `history` for scriptable output.

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
go build -o costroid .
./costroid --version
```

Because `go-sqlite3` uses CGO, source builds require a working C compiler.
