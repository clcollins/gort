# Plan — Fix Dependabot Vulnerabilities

## Problem

GitHub Dependabot flagged 3 vulnerabilities in GORT's dependencies:

| Severity | Package | Summary |
| --- | --- | --- |
| High | `golang.org/x/oauth2` | Improper Validation of Syntactic Correctness of Input |
| Medium | `golang.org/x/net` | Cross-site Scripting |
| Medium | `golang.org/x/net` | HTTP Proxy bypass using IPv6 Zone IDs |

## Changes

Update vulnerable dependencies to their latest versions:

| Package | From | To |
| --- | --- | --- |
| `golang.org/x/oauth2` | v0.25.0 | v0.36.0 |
| `golang.org/x/net` | v0.29.0 | v0.52.0 |

Transitive dependencies also updated: `golang.org/x/sys`, `golang.org/x/term`, `golang.org/x/text`.

## Scope

Dependency updates only — no code changes.
