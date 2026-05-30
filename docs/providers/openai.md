# OpenAI

## Provider Identity

- Provider name: OpenAI
- CLI provider slug: `openai`
- Supported aliases: None

## Data Source

Costroid uses OpenAI organization usage and cost metadata APIs.

## Required Environment Variables

```sh
export OPENAI_ADMIN_KEY=sk-admin-...
```

## Optional Environment Variables

None.

## Minimal Setup Steps

1. Create an OpenAI organization admin key.
2. Export `OPENAI_ADMIN_KEY` in the shell where you run `costroid`.
3. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid sync --provider openai --days 7
```

## Permission Notes

Normal OpenAI project API keys may not work. This provider needs access to OpenAI organization usage and cost metadata APIs.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from OpenAI. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- Non-USD OpenAI cost rows are skipped.
- Cost rows are joined to usage rows by metadata dimensions; rows that cannot be safely matched are not converted into stored prompt or completion data.
