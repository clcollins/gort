# Plan — Add Ollama AI Provider

## Problem

GORT supports Claude (Anthropic) and GitHub Models as AI providers, both of which are
cloud-hosted and require API keys. There is no option for local AI inference, which is
useful for development, testing, air-gapped environments, and cost reduction.

## Solution

Add Ollama as a third AI provider. Ollama exposes an OpenAI-compatible endpoint at
`/v1/chat/completions`, so the implementation mirrors the existing GitHub Models client.

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `GORT_AI_PROVIDER` | `claude` | Set to `ollama` to use Ollama |
| `GORT_OLLAMA_URL` | `http://localhost:11434` | Ollama server base URL |
| `GORT_OLLAMA_MODEL` | `llama3` | Model name to use |

No API key is required — Ollama runs without authentication by default.

For Kubernetes deployments (e.g. using
[clcollins/ollama-container](https://github.com/clcollins/ollama-container)),
set `GORT_OLLAMA_URL` to the in-cluster service address.

## Changes

1. Create `internal/ollama/` package implementing `pkg/ai.Client`
2. Add `"ollama"` case to the provider switch in `cmd/gort/main.go`
3. Add `GORT_OLLAMA_URL` and `GORT_OLLAMA_MODEL` env var loading
4. Update README with new provider and env vars

## Key Decisions

- Uses Ollama's OpenAI-compatible endpoint for code reuse with ghmodels
- HTTP timeout set to 300s (vs 120s for cloud providers) since local models are slower
- No authentication headers sent
