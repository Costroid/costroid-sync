# GitHub Copilot

## Provider Identity

- Provider name: GitHub Copilot
- CLI provider slug: `github-copilot`
- Supported aliases: `copilot`

## Data Source

Costroid uses GitHub premium request billing usage metadata:

- `/organizations/{org}/settings/billing/premium_request/usage`

## Required Environment Variables

```sh
export GITHUB_PAT=ghp_...
export GITHUB_ORG=your-org
```

## Optional Environment Variables

None.

## Minimal Setup Steps

1. Create a GitHub personal access token with access to organization billing usage metadata.
2. Export `GITHUB_PAT`.
3. Export `GITHUB_ORG` with the organization login.
4. Run a sync for the desired lookback window.

## Example Sync Command

```sh
costroid-sync sync --provider github-copilot --days 7
```

The alias is also supported:

```sh
costroid-sync sync --provider copilot --days 7
```

## Permission Notes

Billing data may be unavailable depending on organization/account permissions and platform availability. Check `GITHUB_PAT` permissions, `GITHUB_ORG`, organization admin access, and whether premium request billing data is available for the account.

## Metadata-Only Privacy Notes

Costroid stores only normalized billing/usage metadata from GitHub Copilot. It never stores prompts, completions, messages, content, request bodies, response bodies, raw provider payloads, source code, repository contents, issues, pull request text, diagnostic logs, or invocation logs.

## Known Limitations / Caveats

- No repository contents, prompts, completions, source code, issues, or pull request text are requested.
- Token fields are populated only when GitHub reports the billing unit as tokens.
