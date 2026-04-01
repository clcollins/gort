# Plan 0002 — Standardize Secret Names

## Problem

The environment variable `GORT_WEBHOOK_SECRET` does not follow the `GORT_<PROVIDER>_<ITEM>` naming
convention used by all other provider-specific secrets. This secret is GitHub-specific (a GitHub
HMAC webhook signing secret), but its name implies it is a generic GORT-level config like
`GORT_LISTEN_ADDR`. As GORT adds support for additional VCS providers (GitLab, Gitea), each will
have its own webhook secret, and the current name will be ambiguous.

## Current State

| Variable | Convention | Issue |
| --- | --- | --- |
| `GORT_WEBHOOK_SECRET` | `GORT_<ITEM>` | Missing provider prefix — should be `GORT_GITHUB_WEBHOOK_SECRET` |
| `GORT_GITHUB_TOKEN` | `GORT_GITHUB_<ITEM>` | Already correct |
| `GORT_CLAUDE_API_KEY` | `GORT_CLAUDE_<ITEM>` | Already correct |
| `GORT_CLAUDE_MODEL` | `GORT_CLAUDE_<ITEM>` | Already correct |
| `GORT_GITHUB_MODELS_TOKEN` | `GORT_GITHUB_MODELS_<ITEM>` | Already correct |
| `GORT_GITHUB_MODELS_MODEL` | `GORT_GITHUB_MODELS_<ITEM>` | Already correct |
| `GORT_LISTEN_ADDR` | `GORT_<ITEM>` | Correct — global config, not provider-specific |
| `GORT_METRICS_ADDR` | `GORT_<ITEM>` | Correct — global config, not provider-specific |
| `GORT_AI_PROVIDER` | `GORT_<ITEM>` | Correct for now — but removed in Plan 0003 |

## Proposed Solution

Rename `GORT_WEBHOOK_SECRET` to `GORT_GITHUB_WEBHOOK_SECRET` with a backwards-compatible
deprecation shim.

## Changes

### `cmd/gort/main.go`

1. Rename `appConfig.webhookSecret` field to `githubWebhookSecret`.
2. Add helper function:

    ```go
    // envWithDeprecatedFallback reads newKey first; if empty, falls back to oldKey
    // with a deprecation warning. Exits if neither is set.
    func envWithDeprecatedFallback(newKey, oldKey string) string {
        if v := os.Getenv(newKey); v != "" {
            return v
        }
        if v := os.Getenv(oldKey); v != "" {
            slog.Warn("deprecated environment variable, use new name instead",
                "old", oldKey, "new", newKey)
            return v
        }
        slog.Error("required environment variable not set", "key", newKey)
        os.Exit(1)
        return "" // unreachable
    }
    ```

3. In `loadConfig()`, replace:

    ```go
    cfg.webhookSecret = mustEnv("GORT_WEBHOOK_SECRET")
    ```

    with:

    ```go
    cfg.githubWebhookSecret = envWithDeprecatedFallback("GORT_GITHUB_WEBHOOK_SECRET", "GORT_WEBHOOK_SECRET")
    ```

4. Update all references from `cfg.webhookSecret` to `cfg.githubWebhookSecret` (lines 115, 186).
5. Update `flag.Usage` help text to show `GORT_GITHUB_WEBHOOK_SECRET` as the required var,
   with a note that `GORT_WEBHOOK_SECRET` is accepted but deprecated.

### `cmd/gort/main_test.go`

- Update the `--help` output assertion from `"GORT_WEBHOOK_SECRET"` to
  `"GORT_GITHUB_WEBHOOK_SECRET"`.

### `internal/webhook/handler.go`

- No changes. The handler receives the secret as a string parameter and has no knowledge of
  env var names.

### `internal/github/client.go`

- No changes. It receives `webhookSecret` as a constructor parameter.

### `README.md`

- Update the environment variable documentation to show the new name.

## New Files

- `docs/plans/0002-standardize-secret-names.md` — this plan document.

## Backwards Compatibility

- The `envWithDeprecatedFallback()` function ensures the old `GORT_WEBHOOK_SECRET` name continues
  to work. When it is used, GORT emits a `slog.Warn`.
- The old name support should be removed in a future release.

## Verification

- `make test` — all tests pass (including updated help-flag assertion).
- Run with only `GORT_WEBHOOK_SECRET` set: deprecation warning logged, starts normally.
- Run with only `GORT_GITHUB_WEBHOOK_SECRET` set: starts normally, no warning.
- Run with neither set: exits with error referencing `GORT_GITHUB_WEBHOOK_SECRET`.
- `make fmt && make vet` — clean.
