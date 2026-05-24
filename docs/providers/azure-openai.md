# Azure OpenAI

## Provider Identity

- Provider name: Azure OpenAI
- CLI provider slug: `azure-openai`
- Supported aliases: None
- Generic `azure` alias: Not supported

## Data Source

Costroid uses Azure Cost Management for spend. Azure Monitor token metrics are optional enrichment only when `AZURE_OPENAI_RESOURCE_IDS` is set.

## Required Environment Variables

```sh
export AZURE_TENANT_ID=...
export AZURE_CLIENT_ID=...
export AZURE_CLIENT_SECRET=...
export AZURE_SUBSCRIPTION_ID=...
```

## Optional Environment Variables

```sh
export AZURE_COST_SCOPE=subscriptions/<id>
export AZURE_OPENAI_RESOURCE_IDS=/subscriptions/.../Microsoft.CognitiveServices/accounts/...
```

## Minimal Setup Steps

1. Create a service principal for Costroid.
2. Grant it `Cost Management Reader` on the subscription or configured scope.
3. Export the four required Azure environment variables.
4. Optionally set `AZURE_COST_SCOPE`; otherwise Costroid uses `subscriptions/<AZURE_SUBSCRIPTION_ID>`.
5. Optionally set comma-separated `AZURE_OPENAI_RESOURCE_IDS` for Azure Monitor token metric enrichment.
6. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid-sync sync --provider azure-openai --days 30
```

## Permission Notes

The service principal needs `Cost Management Reader` for spend data. If `AZURE_OPENAI_RESOURCE_IDS` is set, it also needs `Monitoring Reader` on each listed Azure OpenAI resource for token metric enrichment.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from Azure. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- No Azure OpenAI generation, chat, or completions APIs are called.
- Log Analytics and diagnostic logs are not used.
- Azure Monitor token metrics are optional enrichment only; Cost Management remains authoritative for spend.
- Request counts are never mapped to tokens.
- Azure OpenAI imports USD rows only. There is no implemented Azure currency override.
