# Sidecar on SHIP — Consolidated Capability Specification

## ADDED Requirements

### Requirement: Complete fork capability record

The project SHALL document the implemented Sidecar fork capabilities in this
single capability spec, including core workflow, adapters, PostgreSQL storage,
triage, autonomy, memory, notifications, uptime diagnostics, worktree and
evaluator safety, skills, budgets, demo applications, and CLI behavior.

#### Scenario: Fork capability documentation

- **WHEN** an operator evaluates the SHIP-enabled fork
- **THEN** the capability spec identifies the implemented behavior and its
  operational boundaries

### Requirement: SHIP-HATS configuration

The GitLab adapter SHALL default to `https://sgts.gitlab-dedicated.com`, and
the LLM provider SHALL honor `ANTHROPIC_BASE_URL` while preserving its default
when unset. GitLab CI error patterns SHALL be configurable through
`error_patterns`.

#### Scenario: SHIP-HATS deployment configuration

- **WHEN** a deployment configures a compatible model endpoint and GitLab
  failure patterns
- **THEN** Sidecar uses that endpoint and applies those patterns during GitLab
  CI enrichment

### Requirement: Enriched CI failure context

GitLab CI failure signals SHALL best-effort include the failed job, smart error
log context, changed files, commit diff, and flake status. Logs SHALL be
bounded to 150 lines. Commit diffs SHALL be included only when they are at
most 32 KiB; larger diffs SHALL fall back to the complete changed-file list.
Missing API data SHALL degrade to empty optional fields.

Triage SHALL receive failed-job, log, changed-file, and flake context. The
coding agent SHALL receive job logs and the commit diff or changed-file list,
plus the failed job name when available.

#### Scenario: Failed GitLab pipeline

- **WHEN** GitLab reports a failed pipeline with accessible job and commit data
- **THEN** the emitted signal contains bounded failed-job logs and available
  changed-file or diff context for triage and coding

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

#### Scenario: Pipeline signal is replayed after restart

- **WHEN** a CI failure signal has a signal key already stored for the workspace
- **THEN** Sidecar skips it before task creation and model usage

### Requirement: Backward compatibility

Migrations SHALL be idempotent and additive. Existing tasks SHALL remain valid
with `NULL` signal keys, and existing routing, autonomy, adapter, triage,
evaluator, output, and CLI behavior SHALL remain compatible except where this
spec explicitly adds context or deduplication.

#### Scenario: Existing database is upgraded

- **WHEN** the schema is applied to a database containing existing tasks
- **THEN** the migration succeeds idempotently and existing rows retain a
  nullable signal key

## Verification status

Unit and integration test source coverage exists for signal-key derivation,
store lookup/uniqueness/null behavior, duplicate CI signals, and
non-deduplicated schedule ticks. Database integration tests require
`SIDECAR_TEST_DB_URL`. Live deployment verification remains outstanding.
