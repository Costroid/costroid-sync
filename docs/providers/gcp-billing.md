# GCP Billing

## Provider Identity

- Provider name: GCP Billing
- CLI provider slug: `gcp-billing`
- Supported aliases: `gcp`

## Data Source

Costroid uses the BigQuery REST API to query Google Cloud Billing detailed export data.

## Required Environment Variables

```sh
export GCP_SERVICE_ACCOUNT_JSON=/path/to/service-account.json
export GCP_BILLING_PROJECT=your-query-project
export GCP_BILLING_TABLE=your-project.billing_export_data.gcp_billing_export_v1_XXXXXX
```

## Optional Environment Variables

```sh
export GCP_BILLING_SERVICE_FILTER="Vertex AI,Gemini,Cloud Run"
export GCP_BILLING_PROJECT_FILTER=your-gcp-project-id
export GCP_BILLING_CURRENCY=USD
```

## Minimal Setup Steps

1. Enable Cloud Billing detailed export to BigQuery.
2. Create a service account JSON key.
3. Grant the service account `BigQuery Data Viewer` on the export dataset.
4. Grant the service account `BigQuery Job User` on `GCP_BILLING_PROJECT`.
5. Export the three required GCP Billing environment variables.
6. Optionally set service, project, or currency filters.
7. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid sync --provider gcp-billing --days 30
```

The alias is also supported:

```sh
costroid sync --provider gcp --days 30
```

## Permission Notes

The service account needs `BigQuery Data Viewer` on the Cloud Billing export dataset and `BigQuery Job User` on the query project named by `GCP_BILLING_PROJECT`.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from GCP Billing. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- This provider does not replace the Google Gemini CSV importer.
- No Google SDKs, `gcloud`, or `bq` are required or called.
- Vertex metrics, Cloud Logging, Audit Logs, prompt logs, and content APIs are not used.
- `labels` and `system_labels` are not selected.
- `GCP_BILLING_CURRENCY` defaults to `USD` and filters rows; no FX conversion is performed.
