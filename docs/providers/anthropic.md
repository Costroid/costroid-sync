# Anthropic

## Provider Identity

- Provider name: Anthropic
- CLI provider slug: `anthropic`
- Supported aliases: None

## Data Source

Costroid uses Anthropic admin usage and cost metadata APIs.

## Required Environment Variables

```sh
export ANTHROPIC_ADMIN_KEY=sk-ant-admin-...
```

## Optional Environment Variables

None.

## Minimal Setup Steps

1. Create an Anthropic Admin API key in the Claude Console.
2. Export `ANTHROPIC_ADMIN_KEY` in the shell where you run `costroid-sync`.
3. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid-sync sync --provider anthropic --days 7
```

## Permission Notes

Normal Anthropic API keys may not work. This provider needs access to Anthropic organization usage and cost metadata APIs.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from Anthropic. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- Non-USD Anthropic cost rows are skipped.
- Cost rows are joined to usage rows by metadata dimensions; token counts can be zero when usage data is unavailable or cannot be safely joined.
