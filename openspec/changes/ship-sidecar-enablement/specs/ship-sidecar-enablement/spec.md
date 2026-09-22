# Sidecar on SHIP — Consolidated Capability Specification

## ADDED Requirements

### Requirement: Complete fork capability record

The project SHALL document the implemented Sidecar fork capabilities in this
single capability spec, including core workflow, adapters, PostgreSQL storage,
triage, autonomy, memory, notifications, uptime diagnostics, worktree and
evaluator safety, skills, budgets, demo applications, and CLI behavior.

### Requirement: SHIP-HATS configuration

The GitLab adapter SHALL default to `https://sgts.gitlab-dedicated.com`, and
the LLM provider SHALL honor `ANTHROPIC_BASE_URL` while preserving its default
when unset. GitLab CI error patterns SHALL be configurable through
`error_patterns`.

### Requirement: Enriched CI failure context

GitLab CI failure signals SHALL best-effort include the failed job, smart error
log context, changed files, commit diff, and flake status. Logs SHALL be
bounded to 150 lines and diffs SHALL be bounded to 64 KiB. Missing API data
SHALL degrade to empty optional fields.

Triage SHALL receive failed-job, log, changed-file, and flake context. The
coding agent SHALL receive job logs and the commit diff or changed-file list,
plus the failed job name when available.

### Requirement: Restart-safe signal deduplication

The tasks table SHALL contain a nullable `signal_key` and a partial unique
index on `(workspace_id, signal_key)`. The loop SHALL check for an existing
key before task creation or LLM use.

- CI signals use `ci.failure:<pipeline_id>` or `ci.failure:<run_id>`.
- Git commits use `git.commit:<hash>`.
- Schedule, on-demand, log, metric, uptime, and identifier-less signals use
  no key and SHALL not be deduplicated.
- Dedup lookup errors SHALL fail open with a warning.
- The unique index SHALL prevent concurrent duplicate task insertion.

### Requirement: Backward compatibility

Migrations SHALL be idempotent and additive. Existing tasks SHALL remain valid
with `NULL` signal keys, and existing routing, autonomy, adapter, triage,
evaluator, output, and CLI behavior SHALL remain compatible except where this
spec explicitly adds context or deduplication.

## Verification status

Unit and integration test source coverage exists for signal-key derivation,
store lookup/uniqueness/null behavior, duplicate CI signals, and
non-deduplicated schedule ticks. Full execution still requires the missing
`../harness` module and, for database tests, `SIDECAR_TEST_DB_URL`. Live
deployment verification remains outstanding.
