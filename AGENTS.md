# AGENTS.md — Costroid™

> **This file is the single source of truth for all AI coding agents working on this project.**
> Read this ENTIRE file before writing any code.

---

## Project Identity

- **Product:** Costroid™ — Open-source AI cost tracking CLI (Go) + paid team dashboard (Next.js)
- **What it does:** Tracks, normalizes, forecasts, and alerts on AI/LLM spending across multiple providers using metadata only.
- **What it does NOT do:** Observability, tracing, prompt debugging, proxy/gateway, or ANY processing of prompt/completion content.
- **Target:** Open-source CLI for individual developers (GitHub primary). Paid cloud dashboard for teams (costroid.com + Azure/AWS Marketplace).
- **Solo developer project.** Simplicity and shipping speed are paramount. Do not over-engineer.

---

## The One Unbreakable Rule

```
╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║   NEVER READ, STORE, LOG, CACHE, TRANSMIT, OR PROCESS       ║
║   ANY PROMPT OR COMPLETION CONTENT FROM ANY LLM.            ║
║                                                              ║
║   We handle METADATA ONLY:                                   ║
║   ✅ token counts    ✅ model names     ✅ timestamps        ║
║   ✅ cost amounts    ✅ resource IDs    ✅ team/project tags  ║
║                                                              ║
║   ❌ prompt text     ❌ completion text  ❌ message arrays   ║
║   ❌ system prompts  ❌ function args    ❌ tool call text   ║
║                                                              ║
║   If you are writing a function that COULD receive prompt    ║
║   data (e.g., parsing a full API response), you MUST         ║
║   explicitly destructure and extract ONLY the metadata       ║
║   fields, and discard everything else.                       ║
║                                                              ║
║   A violation of this rule is a critical security bug.       ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝
```

### Enforcement Pattern

```typescript
// ✅ CORRECT — explicitly extract only metadata
function processOpenAIUsage(rawData: any): CostRecord {
  return {
    model: rawData.model,
    prompt_tokens: rawData.usage?.prompt_tokens ?? 0,
    completion_tokens: rawData.usage?.completion_tokens ?? 0,
    total_tokens: rawData.usage?.total_tokens ?? 0,
    cost_usd: calculateCost(rawData.model, rawData.usage),
    recorded_at: rawData.created_at,
  };
}
// ❌ WRONG: return { ...rawData };       // NEVER spread raw API responses
// ❌ WRONG: console.log(response.data);  // May contain prompts
```

### Enforcement Pattern (Go — CLI)

```go
// ✅ CORRECT — explicitly extract only metadata
func processOpenAIUsage(raw map[string]interface{}) NormalizedCostRecord {
    return NormalizedCostRecord{
        Model:            raw["model"].(string),
        PromptTokens:     int(raw["input_tokens"].(float64)),
        CompletionTokens: int(raw["output_tokens"].(float64)),
        CostUSD:          raw["cost"].(float64),
        RecordedAt:       raw["date"].(string),
    }
}
// ❌ WRONG: json.Unmarshal(body, &fullResponse)  // May contain prompts
// ❌ WRONG: fmt.Println(string(body))             // Logs raw API response
```

---

## Tech Stack — Exact Choices

| Layer | Technology | Notes |
|-------|-----------|-------|
| **Runtime** | Node.js 20+ | LTS only — web dashboard |
| **Runtime (CLI)** | Go 1.24+ | CLI sync agent — single binary, zero npm deps |
| **Framework** | Next.js 14+ (App Router) | `app/` directory only, NOT `pages/` |
| **Language** | TypeScript (strict mode) | `"strict": true` in tsconfig |
| **Styling** | Tailwind CSS 3+ | No custom CSS except globals.css |
| **UI Components** | shadcn/ui | Install on demand |
| **Charts** | Recharts | Dashboard visualizations |
| **Icons** | Lucide React | Consistent with shadcn |
| **Database** | Supabase (PostgreSQL) | Supabase JS SDK for queries |
| **Auth** | Supabase Auth | Email/password + OAuth: Google, GitHub, **Azure/Microsoft** |
| **Query Layer** | Supabase JS SDK v2 | `@supabase/supabase-js` |
| **Server State** | TanStack Query v5 | `@tanstack/react-query` |
| **API Calls** | Native `fetch` | No axios |
| **Background Jobs** | Vercel Cron | Analytics only (forecasting + anomaly detection). NO provider API calls. |
| **Validation** | Zod | All inputs and env vars |
| **Email** | Resend (NOT SendGrid — free tier killed May 2025) | Alerts and transactional, React Email for Next.js |
| **JWT Validation** | jose | Azure webhook auth |
| **Testing** | Vitest + React Testing Library | All ingestion functions tested |
| **Linting** | ESLint + Prettier | Pre-commit via husky + lint-staged |
| **Package Manager** | pnpm | NOT npm, NOT yarn. Pin exact versions (no ^ or ~). Run `pnpm audit` before every deploy. |

### Do NOT Use

- ❌ Prisma, Drizzle, or any ORM
- ❌ tRPC
- ❌ Redux, Zustand, Jotai, SWR
- ❌ Express.js, Fastify, FastAPI, Flask
- ❌ MongoDB, DynamoDB
- ❌ Docker in development
- ❌ Monorepo tools (Turborepo, Nx)
- ❌ axios

### Go CLI Dependencies (costroid-sync)

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework (commands, flags, help) |
| `github.com/mattn/go-sqlite3` | Local SQLite storage (~/.costroid/costroid.db) |
| `github.com/charmbracelet/lipgloss` | Terminal table styling and colors |
| Standard library only for: | `net/http` (API calls), `encoding/json`, `fmt`, `os`, `context`, `crypto/sha256` |

**Do NOT add** Go DI frameworks, Go ORMs (GORM, ent), or Go web frameworks (gin, echo). Use standard library + these 3 packages.

**T1 dependency rule:** T1.1 statusline MVP must use the existing dependency set; it should be deterministic stdout over local SQLite and require no Bubble Tea dependency. Bubble Tea/Bubbles are not approved today. They may be added only for explicitly approved T1.2/T1.3 fullscreen TUI or TUI sync work, and only after an ADR/dependency review in `DECISIONS.md` per `MAINTAINABILITY.md`.

---

## Project Structure

### Cloud Dashboard (repo: costroid/costroid-cloud — Phase 3)

```
costroid-cloud/
├── AGENTS.md
├── spec.md
├── SKILL.md
├── .env.local                     # NEVER commit
├── .env.example
├── .gitignore
├── next.config.mjs
├── tailwind.config.ts
├── tsconfig.json
├── vitest.config.ts
├── package.json
├── pnpm-lock.yaml
│
├── app/
│   ├── layout.tsx                 # Root layout (QueryClientProvider, Supabase)
│   ├── page.tsx                   # Landing / marketing page
│   │
│   ├── marketplace/
│   │   └── azure/
│   │       └── page.tsx           # Azure Marketplace SSO landing page (see SKILL.md)
│   │
│   ├── error/
│   │   └── page.tsx               # Error display page
│   │
│   ├── (auth)/
│   │   ├── login/page.tsx
│   │   ├── signup/page.tsx
│   │   └── callback/route.ts      # Supabase OAuth callback
│   │
│   ├── dashboard/
│   │   ├── layout.tsx             # Dashboard shell (sidebar + header + auth guard)
│   │   ├── page.tsx               # Main dashboard (/dashboard)
│   │   ├── providers/page.tsx     # /dashboard/providers
│   │   ├── models/page.tsx        # /dashboard/models
│   │   ├── teams/page.tsx         # /dashboard/teams
│   │   ├── forecasts/page.tsx     # /dashboard/forecasts
│   │   ├── budgets/page.tsx       # /dashboard/budgets
│   │   ├── anomalies/page.tsx     # /dashboard/anomalies
│   │   ├── alerts/page.tsx        # /dashboard/alerts
│   │   ├── savings/page.tsx       # /dashboard/savings
│   │   ├── transparency/page.tsx  # /dashboard/transparency ("What We Collected")
│   │   ├── upload/page.tsx        # /dashboard/upload (CSV upload)
│   │   ├── audit-log/page.tsx     # /dashboard/audit-log (Team+)
│   │   └── settings/page.tsx      # /dashboard/settings (agent key display)
│   │
│   └── api/
│       ├── marketplace/azure/
│       │   ├── resolve/route.ts   # Exchange marketplace token → subscription details
│       │   └── activate/route.ts  # Activate subscription + provision customer
│       ├── webhooks/
│       │   ├── azure-marketplace/route.ts  # Azure SaaS lifecycle events
│       │   └── aws-marketplace/route.ts
│       ├── ingest/route.ts        # SDK metadata ingestion (post-launch backlog)
│       ├── orgs/[orgId]/
│       │   ├── costs/route.ts
│       │   ├── providers/route.ts
│       │   ├── budgets/route.ts
│       │   ├── forecasts/route.ts
│       │   ├── agent-sync/route.ts    # Receives data from Go CLI (POST)
│       │   ├── upload/route.ts        # CSV upload (POST)
│       │   ├── transparency/route.ts  # "What We Collected" (GET)
│       │   └── audit-log/route.ts     # Data audit log (GET, Team+)
│       └── cron/
│           ├── run-forecasts/route.ts        # Analytics cron — NO provider API calls
│           └── detect-anomalies/route.ts
│
├── components/
│   ├── ui/                        # shadcn/ui (auto-generated)
│   ├── dashboard/                 # Dashboard widgets
│   ├── providers/                 # Provider connection UI
│   └── layout/                    # Sidebar, header, plan badge
│
├── lib/
│   ├── supabase/
│   │   ├── client.ts              # Browser client (anon key)
│   │   ├── server.ts              # Server client (cookies-based, for RSC)
│   │   ├── admin.ts               # Service role client (cron/webhooks only)
│   │   └── middleware.ts          # Auth middleware helper
│   ├── providers/
│   │   ├── types.ts               # NormalizedCostRecord, ProviderConfig
│   │   ├── csv-parsers.ts         # Parse OpenAI/Anthropic CSV exports for upload feature
│   │   └── normalizer.ts          # Normalize CSV data to unified schema
│   │   # NOTE: Provider API calls (OpenAI, Anthropic, etc.) live in the Go CLI repo,
│   │   # NOT here. The cloud dashboard only receives pre-normalized data via agent-sync.
│   ├── forecasting/
│   │   ├── ema.ts
│   │   ├── linear-trend.ts
│   │   └── anomaly-detector.ts
│   ├── marketplace/
│   │   ├── azure-auth.ts
│   │   ├── azure-fulfillment.ts
│   │   ├── azure-webhook-auth.ts
│   │   ├── provisioning.ts
│   │   └── aws-metering.ts
│   ├── notifications/
│   │   ├── slack.ts
│   │   ├── email.ts
│   │   └── teams.ts
│   ├── pricing/
│   │   └── model-pricing.ts
│   └── utils/
│       ├── validation.ts          # Zod schemas
│       └── constants.ts           # Plan limits, feature flags
│
├── middleware.ts                   # Next.js middleware: protect /dashboard/*
│
├── types/
│   ├── database.ts                # Generated: pnpm supabase gen types typescript
│   ├── api.ts
│   └── providers.ts
│
├── supabase/
│   └── migrations/
│       ├── 001_initial_schema.sql # W1 core schema from W0 R2/spec.md
│       ├── 002_seed_model_pricing.sql
│       ├── 003_team_features.sql  # W4
│       └── 004_marketplace.sql    # W6
│
└── tests/
    ├── lib/providers/
    │   ├── csv-parsers.test.ts    # Must test metadata-only extraction from CSV
    │   └── normalizer.test.ts
    ├── lib/forecasting/
    │   └── anomaly-detector.test.ts
    └── api/
        ├── agent-sync.test.ts     # Must test Bearer agent key auth + prompt content rejection
        ├── upload.test.ts         # Must test CSV prompt column rejection
        └── ingest.test.ts         # Must test prompt content rejection
```

### CLI Sync Agent (separate Go repo: `costroid/costroid-sync`)

```
costroid-sync/
├── go.mod
├── go.sum
├── main.go                        # CLI entry point
├── cmd/
│   ├── root.go                    # Root command (cobra)
│   ├── sync.go                    # Sync command (--provider, --days; --push after C11)
│   ├── history.go                 # View local history (--last 30d)
│   ├── trend.go                   # Show trends (--weekly, --monthly)
│   ├── forecast.go                # Predict month-end spend
│   ├── anomalies.go               # List unusual spending days
│   ├── savings.go                 # Show savings recommendations
│   ├── budget.go                  # Set/check budget (--set, --period)
│   ├── export.go                  # Export (--format csv|json|focus|markdown)
│   └── version.go                 # Print version
├── providers/
│   ├── types.go                   # NormalizedCostRecord struct
│   ├── registry.go                # Provider registry
│   ├── openai.go                  # GET /v1/organization/usage/completions
│   ├── anthropic.go               # GET /v1/organizations/usage_report/messages
│   ├── github_copilot.go          # GET /organizations/ORG/settings/billing/... (Phase 2)
│   ├── azure_openai.go            # Azure Cost Management API (Phase 2)
│   ├── aws_bedrock.go             # AWS Cost Explorer API (Phase 2)
│   └── gcp_billing.go             # GCP Billing export API (Phase 2)
├── storage/
│   └── sqlite.go                  # Local SQLite at ~/.costroid/costroid.db
├── analysis/
│   ├── forecast.go                # EMA + linear regression
│   ├── anomaly.go                 # Rolling average anomaly detection
│   ├── savings.go                 # Model comparison savings calculator
│   └── budget.go                  # Budget tracking
├── output/
│   ├── table.go                   # Terminal table formatting (lipgloss)
│   ├── json.go                    # JSON output
│   └── csv.go                     # CSV + FOCUS export
├── client/
│   └── cloud.go                   # Optional POST to costroid.com (created in C11)
├── .env.example                   # All supported env vars
├── Makefile                       # build, test, release targets
├── .goreleaser.yaml               # Release artifacts; cross-platform claims require validation
├── LICENSE                        # MIT
├── README.md                      # Setup instructions, demo GIF
└── .github/
    └── workflows/
        ├── ci.yml                 # go test + go vet on every PR
        └── release.yml            # GoReleaser on tag push
```

**Why Go for the CLI:**
- **Zero language-runtime dependencies** — single Go binary, no npm/pip/node required; validate OS-level dynamic linking per release artifact
- **No supply chain risk at runtime** — nothing downloaded when user runs it
- **API keys stay in the Go binary's process memory** — never exposed to npm dependency tree
- **Cross-platform goal** — Linux/macOS/Windows release artifacts must be validated before they are claimed, especially because CGO/go-sqlite3 can affect builds
- **Users can audit the source** — `go build` is reproducible

---

## Agent Workflow Rules

### Rule 1: Read Before Writing
Read in order: (1) This file. (2) `spec.md`. (3) `SKILL.md` if working on marketplace.
For CLI agent work: read the `costroid-sync/README.md` as well.

### Rule 2: Work in Priority Order

```
CURRENT ROADMAP: CLI is provider-complete, R3 is complete, W0 is approved in `W0-cloud-architecture.md`, and W1-W4 plus C11 are complete. Public launch is intentionally deferred until W5, W6, and the approved D1/T1 pre-launch design gates are complete. See PROMPTS.md for full details.

Completed:
C1: Go project setup
C2: OpenAI provider
C3: Anthropic provider
C4: Savings recommendations
C5: History, trends, forecasting, anomaly detection
C6: Budget tracking + multi-format export
C7: README, GoReleaser, install script, CI/CD
v0.1.0 release
R2 / v0.2.0 validation, tag, push, and install smoke
C9.1: GitHub Copilot provider
C9.2: Google Gemini CSV billing provider
C10.1: Azure OpenAI provider
C10.2: AWS Bedrock provider
C10.3: GCP Billing provider
Provider docs extraction
R3: Cross-platform install experience
W0: Cloud architecture checkpoint accepted in W0-cloud-architecture.md
W1: Next.js project scaffolding
W2: Auth + orgs + agent-sync endpoint + CSV upload
C11: CLI cloud push to the real W2 ingestion endpoint
W3: Dashboard + charts + transparency view
W4: Team features and alerts

Active sequence:
W5: Landing page, pricing page, and cloud docs
W6: Azure Marketplace SaaS integration
D1/T1: Founder-approved pre-C8 design and terminal experience gates after W6
C8: Public launch on GitHub/HN/Reddit/Product Hunt when R3/W0, W1-W6/C11, and approved D1/T1 pre-launch gates are complete
L1: Post-launch feedback triage

W0 R2 cloud contract:
- Agent-sync auth uses `Authorization: Bearer csk_...`, not in-body `agent_key`.
- Agent-sync includes `X-Costroid-Wire-Version: 1` and body `{ "records": [...] }`.
- `cost_records` deduplicates per org with `(org_id, source_hash)`.
- Ingest routes find-or-create `connected_providers` by `(org_id, provider_type, sync_method)`.
- W1 core migrations are only orgs, members, agent keys, connected providers, cost records, audit log, and model pricing. W4 adds team/alert analytics tables; W6 adds marketplace tables.
- `/api/ingest` SDK endpoint is post-launch backlog, not W2.

Post-launch backlog and launch gates:
M3: Recommendation Intelligence Platform — source-agnostic, metadata-only model alternatives to evaluate.
Allowed inputs: provider/model/SKU metadata, token counts, usage quantities, cost, timestamps, safe team/project/account IDs, catalog data, pricing sources, benchmark/eval sources, context windows, capability tags, speed/latency, source URLs, source freshness dates, confidence scores, and recommendation evidence.
Forbidden inputs: prompts, completions, messages, content, request/response bodies, raw provider payloads, traces, logs, source code, repository contents, free-form user text, or free-form labels/system labels.
It must not become model routing, proxying, automatic switching, or LLM observability.

T1: Terminal Experience Layer — founder-approved pre-C8 design/statusline gate after W6 for D1/T1.0/T1.1. T1.2/T1.3 remain conditional and require explicit founder approval plus ADR/dependency review.
T1.1 first implementation is statusline MVP: `costroid-sync statusline --format plain|tmux|byobu|json`, local SQLite only, deterministic one-line stdout, no provider API/network calls, no provider sync on redraw, no new Go dependency.
T1 must not become default CLI behavior, raw-terminal overlay, tmux replacement, child-PTY wrapper, model gateway, proxy, tracing, or LLM observability. Bubble Tea/Bubbles are future-only for explicitly approved T1.2/T1.3 work with ADR/dependency approval.
```

### Rule 3: One Feature Per Session
Each session = ONE priority item. At session end:
- **Go CLI:** Zero `go vet` warnings. `go test ./...` passes. `gofmt` applied. Git commit.
- **TypeScript cloud:** Zero TS errors. `pnpm build` passes. New functions have tests. Git commit.

### Rule 4: File Size Limits
- 300 lines max per file. Split if exceeded.
- 50 lines max per function.
- 150 lines max per React component.

### Rule 5: Error Handling

**TypeScript (cloud):**
- API routes: proper HTTP status + Zod error messages.
- External calls: try/catch, update `connected_providers.status = 'error'`.
- NEVER expose internals to frontend.

**Go (CLI):**
- Keep `cmd/*.go` thin — parse flags, call logic, format output. NO business logic in cmd/.
- Business logic belongs in `analysis/` (forecasting, anomaly, savings, budget).
- Provider API calls belong in `providers/`. Database operations in `storage/`.
- Always check and return errors: `if err != nil { return fmt.Errorf("fetch openai: %w", err) }`
- Use `errors.Is` / `errors.As` for known error types.
- Define domain errors: `var ErrProviderUnavailable = errors.New("provider unavailable")`
- NEVER expose raw API/database errors to the user — translate to friendly messages.
- Use `context.WithTimeout` for all HTTP calls to provider APIs.

### Rule 6: Testing the Metadata-Only Rule
Every ingestion function MUST have a test that passes mock data WITH prompt content and asserts ONLY metadata survives.

### Rule 7: Git
- Branches: `feat/C2-openai-provider`, `fix/anomaly-threshold`, `feat/W3-dashboard-charts`
- Commits: `feat:`, `fix:`, `chore:`, `docs:`
- NEVER commit: `.env.local`, `node_modules/`, credentials

### Rule 8: Agent Coordination

**Cloud dashboard repo (costroid-cloud):**

| Agent | Owns | Does NOT Touch |
|-------|------|----------------|
| Backend | `app/api/`, `lib/`, `supabase/`, `tests/` | `components/`, dashboard pages |
| Frontend | `app/dashboard/`, `app/(auth)/`, `components/` | `lib/marketplace/`, `app/api/` |
| Integration | `lib/marketplace/`, `lib/notifications/`, `app/api/webhooks/`, `app/marketplace/` | Dashboard UI, auth |

**Go CLI repo (costroid-sync):** Single developer — no agent coordination needed. All code in one repo.

Backend wins on conflicts.

### Rule 9: Supabase Type Safety
After ANY schema change:
```bash
pnpm supabase gen types typescript --local > types/database.ts
```
All Supabase queries use the generated `Database` type.

---

## Key Design Decisions — Do Not Override

These are FINAL. If you think a decision is wrong, do NOT change it — ask the founder.

1. **Supabase** over self-hosted Postgres — zero DB ops for solo developer.
2. **Next.js API routes** over separate backend — single deployable unit.
3. **TanStack Query v5** — NOT SWR, NOT useEffect+useState.
4. **pnpm** over npm/yarn — content-addressable storage, strict mode.
5. **shadcn/ui** over MUI / Chakra — copy-paste components, no dependency lock-in.
6. **Recharts** over Chart.js / D3 — React-native, works with Server Components.
7. **Zod** for all validation — TypeScript type inference from schemas.
8. **Server Components by default.** `'use client'` only when needed.
9. **Vercel Cron** for background jobs — analytics only, never provider API calls.
10. **Zero credential storage** — provider API keys never touch our server. WHY: "We encrypt your keys" still requires trust. "We never see your keys — read our code" requires zero trust. This eliminates the #1 conversion barrier and is our strongest competitive differentiator.

### The Open-Core Principle

**If it runs on the user's machine and costs us nothing to operate, it's FREE.** Every feature that CAN run locally DOES run locally — forecasting, anomaly detection, savings, budget, export. We only charge for features that require a server (cloud storage, multi-user, notifications). The CLI is the REAL product, not a teaser.
