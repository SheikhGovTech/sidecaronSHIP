# Proposal

## Why

The sidecaronSHIP fork (SheikhGovTech/sidecaronSHIP) runs Sidecar against SHIP-HATS GitLab. The GitLab PAT has `api` scope with full project access — pipelines, job traces, commits, diffs, files, MR creation — but the upstream CI adapter only uses one endpoint: pipeline listing. The triage agent and coding agent receive no error context, causing a 100% skip rate on CI failures.

The upstream uptime adapter already sets the precedent for signal enrichment: it fetches DNS/TCP/TLS diagnostics and includes them in the signal before triage. The CI adapter doesn't follow this pattern. Since we have full GitLab API access, we should enrich CI signals to the same standard — and go further by giving the coding agent commit diffs and error output so it can fix issues without guessing.

## What Changes

- **GitLab CI adapter** (`internal/adapter/gitlabci/gitlabci.go`): Enrich `ci.failure` signals with failed job name, smart-extracted error logs, commit diff, changed file list, and pipeline history for flake detection
- **Triage prompt** (`internal/triage/triage.go`): Surface all enriched CI fields in `BuildTriageMessage()` so triage sees error output, diff scope, and flake context
- **Coding agent prompts** (`internal/loop/loop.go`): Include error output and commit diff in `BuildSystemPrompt()` and failed job name in `userMessage()` so the coding agent knows exactly what broke and what changed
- **SHIP-HATS base URL** (`internal/adapter/gitlabci/gitlabci.go`): Change `defaultBaseURL` from `gitlab.com` to `sgts.gitlab-dedicated.com`
- **Anthropic base URL** (`internal/loop/loop.go`): Read `ANTHROPIC_BASE_URL` env var for PAi endpoint routing
- **UQD2 Dockerfile**: Point `SIDECAR_SHA` at the fork's latest commit

## Capabilities

### New Capabilities

- `sidecar-ship/ci-signal-enrichment`: Adapter-level enrichment of CI failure signals with job logs, commit diffs, changed files, pipeline history, and flake detection via the GitLab API
- `sidecar-ship/ci-triage-context`: Triage prompt surfaces all enriched CI data for informed act/skip decisions
- `sidecar-ship/ci-agent-context`: Coding agent receives error output and commit diff in its system prompt and user message
- `sidecar-ship/ship-hats-config`: SHIP-HATS-specific configuration — GitLab base URL, PAi endpoint routing

### Modified Capabilities

(none — all changes are in the fork, not upstream)

## Impact

- **Fork repo**: `SheikhGovTech/sidecaronSHIP` — 3 files modified (`gitlabci.go`, `triage.go`, `loop.go`)
- **UQD2 repo**: Only `services/sidecar/Dockerfile` changes (SHA pin + fork URL)
- **API calls**: 3-4 additional GitLab API calls per failed pipeline (jobs, trace, commit diff, pipeline history) — not per poll cycle
- **Token cost**: Triage prompt grows from ~300 tokens to ~1500-3000 tokens when job logs are present. Still Haiku, ~$0.005 per signal
- **No breaking changes**: Enriched fields are additive; empty when data unavailable
- **Upstream PR path**: Changes are structured to be proposable back to `sausheong/sidecar` as a GitLab CI enrichment PR
