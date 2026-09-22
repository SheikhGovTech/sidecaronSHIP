# sidecaronSHIP — Sidecar for SHIP-HATS GitLab

This is a fork of [sausheong/sidecar](https://github.com/sausheong/sidecar) with enhancements for [SHIP-HATS](https://www.developer.tech.gov.sg/products/categories/devops/ship-hats/overview.html) GitLab (GovTech Singapore's CI/CD platform).

## What this fork changes

The upstream Sidecar CI adapters emit bare pipeline status with no error context — the triage agent skips 100% of CI failures because it has nothing to diagnose. This fork enriches CI signals to parity with the uptime adapter's diagnostic enrichment.

### Files changed

| File | What changed |
|---|---|
| `internal/adapter/gitlabci/gitlabci.go` | Job log fetching with smart error extraction, commit diff, changed files, flake detection, SHIP-HATS base URL |
| `internal/triage/triage.go` | Surfaces `failed_job`, `job_log`, `changed_files`, `is_flake` in triage prompt |
| `internal/loop/loop.go` | Error output + commit diff in coding agent prompt, PAi base URL via env var |
| `internal/config/config.go` | `ErrorPatterns` field for custom CI log patterns |
| `internal/cli/attach.go` | Wires `ErrorPatterns` config to the GitLab CI adapter |

### Signal enrichment (4 API calls per failed pipeline)

1. `GET /pipelines/:id/jobs` → find failed job name
2. `GET /jobs/:id/trace` → smart error extraction (pattern match + context window + tail summary)
3. `GET /commits/:sha/diff` → commit diff + changed file list
4. `GET /pipelines?ref=main` → pipeline history for strict pass/fail alternation detection

Changed files are always retained. The full diff is included only when it is
32 KiB or smaller, preventing oversized agent prompts without losing file scope.

### Smart error extraction

CI logs can be 5000+ lines. Errors appear in the middle, not the tail. The extraction:
- Scans full log for error patterns (`Error:`, `FAIL`, `panic:`, `Uncaught Exception`, etc.)
- Extracts matching lines with ±2 lines of context
- Always includes last 30 lines (test summary)
- Caps at 150 lines total
- Patterns configurable via `error_patterns` in `sidecar.yaml`

### Three prompt touchpoints

Enriched data reaches all three places where the LLM sees CI signals:
1. **Triage prompt** — decides act/skip with error context
2. **Coding agent system prompt** — sees error output + commit diff
3. **Coding agent user message** — knows which job failed

### SHIP-HATS specific config

- Default GitLab base URL: `sgts.gitlab-dedicated.com` (instead of `gitlab.com`)
- PAi LLM routing: reads `ANTHROPIC_BASE_URL` env var for Platform AI endpoint

## Upstream contribution

These changes are structured for an upstream PR to [sausheong/sidecar](https://github.com/sausheong/sidecar). The same pattern applies to GitHub CI and CircleCI adapters — only the API endpoints differ. See `UPSTREAM-PR-PROPOSAL.md` in the deployment repo for the full proposal.

## Specs

This fork follows [OpenSpec](https://openspec.dev) spec-driven development. The consolidated specification is maintained at `openspec/changes/ship-sidecar-enablement/`:

- [`proposal.md`](openspec/changes/ship-sidecar-enablement/proposal.md) — scope and motivation
- [`design.md`](openspec/changes/ship-sidecar-enablement/design.md) — architecture and key decisions
- [`spec.md`](openspec/changes/ship-sidecar-enablement/specs/ship-sidecar-enablement/spec.md) — consolidated requirements
- [`tasks.md`](openspec/changes/ship-sidecar-enablement/tasks.md) — implementation and verification status

## Deployment

Used by the UQD2 Sandbox project on GovPaaS (Northflank). The Dockerfile clones this fork at a pinned SHA and builds the Go binary with the enhancements.
