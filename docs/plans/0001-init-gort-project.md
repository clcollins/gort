# Plan 0001 — GORT Initial Project Scaffold

## Problem

After a merge to main there is no automated feedback loop verifying that Flux successfully
reconciled declared resources, or that the result matches the intent described in `docs/`
plan documents. Failed deployments require manual investigation of Flux status, pod logs,
and events — then manual authoring of a fix PR.

## Proposed Solution

GORT (GitOps Reconcile Tools) is a containerized Go service that:

1. Receives GitHub webhook events on pushes to `main`.
2. Reads the associated `GitOpsWatcher` CRDs from the cluster to determine which Flux apps
   to watch.
3. Polls Flux until reconciliation completes (success or failure).
4. On **Flux failure**: submits logs, events, CRs, and `docs/plans/` content to the Claude
   API to produce a fix plan, then opens a PR against the target repo.
5. On **Flux success**: collects runtime state (pod readiness, events, endpoints) and
   submits it alongside `docs/plans/` content to the Claude API for intent validation.
   If the running state does not match declared intent, opens a fix PR.
6. Exposes Prometheus metrics and ships Alertmanager alert rules for all failure modes.

## Architecture

```
GitHub push to main
       │
       ▼
internal/webhook  (net/http, HMAC validation, pure parse functions)
       │
       ▼
internal/reconciler  (operator-style reconcile loop, only non-pure orchestrator)
       │  reads GitOpsWatcher CRDs from cluster
       ▼
pkg/gitops.Client  →  internal/flux  (Flux Kustomization/HelmRelease + K8s read-only)
       │
       ├─ Flux failure  ──► pkg/ai.Client → internal/claudeai → fix PR via pkg/vcs.Client
       │
       └─ Flux success
              │
              ▼
         collect runtime state (pods, deployments, events, endpoints)
              │
              ▼
         pkg/ai.Client.ValidateIntent → internal/claudeai
              │
              ├─ intent met  →  update GitOpsWatcher status, done
              └─ intent not met → fix PR via pkg/vcs.Client
```

## Interface Extensibility

| Interface | Default Impl | Extensible To |
|---|---|---|
| `pkg/gitops.Client` | Flux (kustomize + helm + source controllers) | ArgoCD, Rancher Fleet |
| `pkg/vcs.Client` | GitHub (`go-github`) | GitLab, Gitea |
| `pkg/ai.Client` | Claude (Anthropic SDK) | OpenAI, Gemini, local LLMs |

## GitOps App Definition: CRD

`GitOpsWatcher` (group `gitops.gort.io/v1alpha1`) is a cluster-scoped CRD that tells GORT
which GitOps apps to watch:

```yaml
apiVersion: gitops.gort.io/v1alpha1
kind: GitOpsWatcher
metadata:
  name: cluster-config
spec:
  type: flux
  appName: cluster-config
  namespace: flux-system
  targetRepo: clcollins/cluster-config
  fixRepo: clcollins/cluster-config
  docsPaths:
    - docs/plans/
  reconcileTimeout: 10m
```

## Observability

- `/metrics` endpoint (Prometheus)
- `/healthz` liveness probe
- `/readyz` readiness probe
- Alertmanager alerts: `FluxReconcileFailed`, `ResourceDeploymentFailed`, `IntentNotMet`

## Files Added

- `go.mod`, `go.sum` — Go module
- `Makefile` — build, test, lint, docker, generate targets
- `Dockerfile` — multi-stage UBI9 minimal
- `.github/workflows/ci.yaml` — CI: test, fmt, vet, lint, docker build, docs check, markdown lint
- `pkg/gitops/` — GitOps interface + types
- `pkg/vcs/` — VCS interface + types
- `pkg/ai/` — AI interface + types
- `api/v1alpha1/` — GitOpsWatcher CRD Go types
- `internal/metrics/` — Prometheus metric registrations
- `internal/webhook/` — HTTP webhook handler
- `internal/k8s/` — Kubernetes client interface + wrapper
- `internal/flux/` — Flux gitops.Client implementation
- `internal/github/` — GitHub vcs.Client implementation
- `internal/claudeai/` — Claude ai.Client implementation
- `internal/reconciler/` — Core reconcile orchestration
- `cmd/gort/` — Main entrypoint
- `config/rbac/` — ClusterRole (read-only, broad)
- `config/alerting/` — PrometheusRule manifest
- `hack/boilerplate.go.txt` — License header for generated code
- `docs/plans/0001-init-gort-project.md` — This document

## Verification

- `make fmt` — no diff
- `make vet` — no errors
- `make test` — all tests pass with `-race`
- `make docker-build` — image builds successfully
- `make docs-check` — passes
- `make markdown-lint` — no lint errors
