# Plan — Setup Documentation

## Problem

The README documents environment variables in a reference table but lacks a step-by-step guide
for setting up the integrations GORT depends on (GitHub PAT, webhook, AI provider). Users must
piece together the setup from the variable names alone, which is error-prone — especially for
webhook configuration where the payload URL, content type, and event selection are not obvious
from the env var table.

The environment variables table is also missing entries for `GORT_AI_PROVIDER`,
`GORT_GITHUB_MODELS_TOKEN`, and `GORT_GITHUB_MODELS_MODEL`, and the listen address variables
do not explain the `host:port` format expected by Go's `net/http` server.

## Changes

1. Add a **Setup** section to the README with numbered steps for:
   - Creating a GitHub fine-grained PAT with the minimum required permissions
   - Configuring a GitHub webhook (payload URL, content type, secret, events)
   - Setting up an AI provider (Claude or GitHub Models)
2. Add missing environment variables to the reference table
3. Clarify the `host:port` format for `GORT_LISTEN_ADDR` and `GORT_METRICS_ADDR`

## Scope

Documentation only — no code changes.
