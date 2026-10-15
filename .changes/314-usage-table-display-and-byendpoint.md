### 📊 fix(usage): fix column label precedence in UsageBreakdownTable and aggregate byEndpoint

- **Background**: In Usage Breakdown, switching to `Usage by Account` or `Usage by API Key`
  consistently displayed the model ID rather than the account display name or API key name,
  because `row.rawModel` was evaluated first in the fallback expression. Additionally,
  `Usage by Endpoint` was always empty because `HandleUsageStats` never aggregated the
  `byEndpoint` data.
- **Frontend**: Explicitly checked `tableView` in `UsageBreakdownTable.svelte` to render
  `row.accountName` for accounts, `row.keyName` / `row.apiKeyMasked` for API keys, and
  `row.endpoint` for endpoints.
- **Backend**: Added `byEndpoint` parsing in `HandleUsageStats` daily rollup and `usage_window_cache.go`.
