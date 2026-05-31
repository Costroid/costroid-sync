# AGENTS.md - Costroid

This is the canonical instruction file for coding agents in `costroid`, the Go CLI repo.
Keep it short. Add durable rules only; do not paste plans, chat transcripts, long file trees, or launch copy here.

## Project Identity

- Product: Costroid, an open-source Go CLI for AI/LLM cost tracking, plus a separate paid cloud dashboard for teams.
- This repo: the local-first CLI. It fetches provider billing/usage metadata, normalizes it, stores it in local SQLite, analyzes it locally, and optionally pushes normalized metadata to Costroid Cloud.
- Not Costroid: observability, tracing, prompt debugging, model gateway, proxy, automatic routing, Kubernetes/cloud remediation, or prompt/content processing.
- Principle: solo-developer project; prefer simple, auditable, shippable code over abstraction.

## Metadata-Only Rule

NEVER READ, STORE, LOG, CACHE, TRANSMIT, DISPLAY, EXPORT, OR PROCESS PROMPT OR COMPLETION CONTENT.

Allowed data:

- provider names and slugs
- model, SKU, product, service, meter, and resource identifiers
- token counts and usage quantities
- timestamps and sync freshness
- cost amounts, currency, unit price, discounts
- safe project, workspace, account, API key, team, and org IDs
- source hashes and deduplication IDs

Forbidden data:

- prompts, completions, messages, content, system prompts, user/assistant text
- tool call text, function arguments, input/output text fields
- request bodies, response bodies, raw provider payloads
- traces, spans, logs, diagnostic dumps, source code, repository contents
- free-form labels or user/system labels that may contain user text
- provider credentials, OAuth tokens, service-account private keys, authorization headers

Implementation rules:

- Explicitly extract known metadata fields from provider/API responses; never spread or persist raw responses.
- Never log raw HTTP bodies, raw database rows containing secrets, raw provider errors, or full request payloads.
- Every ingestion/provider path must have a test containing poison prompt-like fields and proving only metadata survives or the payload is rejected.
- CSV upload paths reject prompt-like columns loudly before parsing; do not silently drop forbidden columns.

## Current Repo Shape

- `cmd/`: Cobra commands only. Parse flags, call domain logic, format output. No business logic.
- `providers/`: provider API/export readers and normalization. Use `context.WithTimeout` for HTTP calls and safe/friendly errors.
- `storage/`: SQLite persistence at `~/.costroid/costroid.db`, plus read-only helpers.
- `analysis/`: local forecasting, anomaly detection, savings, budgets, aggregation, statusline summaries.
- `output/`: table, CSV, JSON, FOCUS, Markdown, and statusline formatting.
- `tui/`: Bubble Tea dashboard and sync views. Dashboard is read-only over local SQLite.
- `client/`: optional cloud push of normalized metadata only.
- `docs/providers/`: provider setup docs. Keep README concise and link out.

Supported providers are OpenAI, Anthropic, GitHub Copilot, Google Gemini CSV, Azure OpenAI, AWS Bedrock, and GCP Billing. Do not add pre-launch providers unless explicitly approved.

## Approved Stack

Runtime: Go 1.24+. Direct Go dependencies are:

- `github.com/spf13/cobra`
- `github.com/mattn/go-sqlite3`
- `github.com/charmbracelet/lipgloss`
- `github.com/charmbracelet/bubbletea v1.3.10`
- `github.com/charmbracelet/bubbles v0.21.0`
- `github.com/mattn/go-isatty`

Standard library is preferred for HTTP, JSON, crypto, context, filesystem, and formatting.

Do not add Go DI frameworks, ORMs, web frameworks, axios-like HTTP wrappers, or generic utility libraries. Any new dependency or architecture change requires an ADR-style note in this file or the target repo's decision record before installation.

TUI dependency scope:

- Bubble Tea/Bubbles are approved only for the shipped dashboard and `sync --tui`.
- Bubbles allowed components: read-only table/viewport/help/key/paginator, plus spinner/progress only for real sync stages.
- Bubbles forbidden components: textinput, textarea, filepicker, list, or anything introducing free-form text/filesystem input.

## CLI Behavior

- Bare `costroid` opens the fullscreen dashboard in an interactive terminal.
- In a pipe, CI, or `TERM=dumb`, bare `costroid` prints help and never paints an alternate screen.
- There is no standalone `tui` subcommand.
- Plain commands (`sync`, `history`, `trend`, `forecast`, `anomalies`, `savings`, `budget`, `export`, `version`) must stay deterministic and scriptable.
- `costroid statusline --format plain|tmux|byobu|json` is deterministic one-line stdout, local SQLite only, no provider API/network calls, no credential reads, no sync on redraw.
- `costroid sync --tui` is opt-in. It may call provider APIs only because the user explicitly invoked `sync`.
- `sync --tui` animation must reflect real stages only: fetch metadata, write SQLite, refresh analysis, optional push. No fake scanning, no fake AI thinking, no animated money.
- `--plain`, `NO_COLOR`, non-UTF-8, non-TTY, CI, `TERM=dumb`, and `--no-animation` fall back gracefully as appropriate.

TUI safety invariants:

- Dashboard reads local SQLite only and imports no `providers`, `client`, `net`, `net/http`, or `os/exec`.
- No raw-terminal overlay, child PTY wrapper, tmux replacement, background daemon, watch process, timer, socket, or model gateway.
- No provider credentials, raw payloads, prompt-like data, logs, traces, or source code in UI or errors.
- Ctrl-C/quit must restore the terminal.

T1.6 visual identity is founder-approved:

- Dashboard/CLI uses the cold cyan-blue ramp: `#042C53 #185FA5 #378ADD #85B7EB`.
- `sync --tui` uses the warm coral-amber ramp: `#712B13 #D85A30 #F0997B #F5C4B3`.
- Signal green `#C8FF3D` is shared for brand/primary money; alert red is reserved for over/critical states.
- Color is never the only signal; glyph shape, labels, and position must carry meaning in monochrome.
- Money values never animate, roll, or partially render.

## Cloud Boundary

Costroid Cloud is a separate Next.js/Supabase repo. This CLI repo carries only the shared contract and safety rules.

Cloud must never:

- call provider APIs from server routes, cron, webhooks, or server components
- store, encrypt, cache, log, or proxy provider credentials
- accept prompt/completion/message/content/raw payload fields
- become observability, tracing, model routing, proxying, or automatic switching

Cloud receives normalized metadata from:

- CLI agent sync
- CSV upload after forbidden-column rejection

Agent-sync contract:

- Endpoint: `POST /api/orgs/{orgId}/agent-sync`
- Auth: `Authorization: Bearer csk_...`; never in-body `agent_key`
- Header: `X-Costroid-Wire-Version: 1`
- Body: `{ "records": [...] }`
- CLI env vars: `COSTROID_ORG_ID`, `COSTROID_AGENT_KEY`, optional `COSTROID_API_URL`
- Deduplication: `(org_id, source_hash)`
- Provider row resolution: find-or-create `connected_providers (org_id, provider_type, sync_method)`

Allowed `NormalizedCostRecord` fields:

- Required: `provider`, `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `cost_usd`, `recorded_at`, `source_hash`
- Optional: `api_key_id`, `project_id`, `product`, `sku`, `unit_type`, `usage_quantity`, `unit_price_usd`, `gross_amount_usd`, `discount_amount_usd`

Cloud schema facts to preserve:

- Core: orgs, members, agent keys, connected providers, cost records, audit log, model pricing.
- Team/alerts: budgets, alerts, notification channels, forecasts, anomalies.
- Marketplace: marketplace subscriptions, webhook events.
- Service role writes cost records/analytics/audit/marketplace state; users read through org-scoped RLS.
- Agent keys are `csk_<32 url-safe base64 chars>`, hash-only lookup, plaintext shown once.

## Pricing And Public Claims

- CLI Free forever: every local feature, every provider, no account, no artificial limits.
- Cloud Free: $0/month, 1 user, 1-year cloud history, dashboard, transparency view, CSV upload, CLI `--push`.
- Cloud Team: $19/month, monthly only, multi-user workspace, team attribution, budgets, alerts, Slack/Teams/email notifications, audit log, SSO/Azure Marketplace when verified, long-term/full historical retention subject to fair use.
- Enterprise/contact-us is future procurement only, not a public launch pricing card.
- No public Business tier, founder discount, early-access price, annual pricing, yearly plan, or annual discount.
- Do not claim cloud dashboard, `--push`, alerts, team sync, audit log, marketplace checkout, SSO, demo assets, or OS-specific installers until implemented and verified.
- Verify time-sensitive claims, provider pricing, marketplace status, compliance claims, customer counts, and social proof before publishing.

Recommended category language:

- "open-source AI billing visibility"
- "local-first AI/LLM spend tracking"
- "metadata-only AI cost CLI"
- "actual provider billing, not traces or estimates"

Avoid: "LLM observability", "gateway", "proxy", "cloud cost optimizer", "autonomous remediation", "enterprise FinOps replacement", or guaranteed model equivalence/savings.

M3 recommendation intelligence is post-launch unless explicitly reprioritized. It may suggest "models to evaluate" using metadata, pricing, catalogs, benchmarks, source URLs, freshness dates, confidence scores, and evidence. It must not use prompts, completions, traces, logs, source code, raw payloads, request/response bodies, or free-form user text.

## Azure Marketplace Guardrails

- Build marketplace work only after W5 landing/pricing/docs and explicit W6 scope.
- Azure first; AWS Marketplace is later backlog.
- Public marketplace plans: Free and Team only.
- Landing page must be a real UI page with Entra ID SSO before activation; no auto-activation API redirect.
- Supabase Azure OAuth redirect URI is the Supabase callback URL, not the app URL.
- Supabase Azure provider uses `common`; configure `xms_edov` optional claim.
- SaaS Fulfillment API scope is fixed: `20e940b3-4c77-4b0b-9a53-9e16a1b010a7/.default`.
- Resolve uses `POST` with `x-ms-marketplace-token`; Activate must be called to start billing.
- Webhook auth validates Azure JWTs with `jose`.
- Webhooks are idempotent and return `200` even on internal handling errors.
- Acknowledge only `ChangePlan`, `ChangeQuantity`, and `Reinstate`.
- `Suspend`, `Unsubscribe`, and `Renew` are notify-only; do not call `updateOperationStatus`.
- On unsubscribe, set a 7-day post-cancellation deletion window.

## Quality Gates

Limits:

- 300 lines max per file.
- 50 lines max per function.
- 150 lines max per React component in the cloud repo.
- Prefer existing patterns and narrow edits over new abstractions.

Go verification for code changes:

```bash
gofmt -w .
go build ./...
go test ./...
go vet ./...
```

Use `go test -race ./...` for TUI, concurrency, sync orchestration, storage, release-sensitive changes, or launch readiness. `make check` runs fmt, vet, test, and race.

Cloud verification for code changes:

```bash
pnpm build
pnpm test
pnpm audit
```

Documentation/update rules:

- New env var: update `.env.example`.
- New user-visible feature: update README or user docs.
- New provider: update `docs/providers/` and add metadata-only tests.
- Schema change in cloud: add migration and regenerate Supabase types.
- New endpoint/wire change: update docs and tests.
- Pricing/tier change: update code constants, public copy, marketplace mapping, and this file.

Git:

- Branch examples: `feat/C2-openai-provider`, `fix/anomaly-threshold`, `feat/W3-dashboard-charts`.
- Commit prefixes: `feat:`, `fix:`, `chore:`, `docs:`.
- Never commit `.env`, `.env.local`, `node_modules`, credentials, private keys, raw provider payloads, or real customer data.