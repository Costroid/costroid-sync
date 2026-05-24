# Google Gemini

## Provider Identity

- Provider name: Google Gemini
- CLI provider slug: `google-gemini`
- Supported aliases: `gemini`

## Data Source

Costroid reads a local Google Cloud Billing CSV export file. This is a file-backed provider; no Gemini generation APIs are called.

## Required Environment Variables

```sh
export GEMINI_BILLING_EXPORT=/path/to/google-billing-export.csv
```

## Optional Environment Variables

```sh
export GEMINI_BILLING_PROJECT=your-gcp-project-id
export GEMINI_BILLING_SERVICE_FILTER="gemini,vertex"
```

## Minimal Setup Steps

1. Export Google Cloud Billing data from BigQuery as CSV.
2. Export `GEMINI_BILLING_EXPORT` with the CSV path.
3. Optionally set `GEMINI_BILLING_PROJECT` to limit rows to one project.
4. Optionally set `GEMINI_BILLING_SERVICE_FILTER` to adjust service-name matching.
5. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid-sync sync --provider google-gemini --days 30
```

The alias is also supported:

```sh
costroid-sync sync --provider gemini --days 30
```

## Permission Notes

Costroid only needs read access to the local CSV file path in `GEMINI_BILLING_EXPORT`.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from the CSV. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- No Gemini generation APIs are called.
- `labels` and `system_labels` columns are ignored and not stored.
- Non-USD rows are skipped.
- This importer is separate from the broader GCP Billing provider.
