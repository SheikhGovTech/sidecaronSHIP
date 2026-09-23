# Sidecar on SHIP — Consolidated Capability Specification

## ADDED Requirements

### Requirement: Complete fork capability record

The project SHALL document the implemented Sidecar fork capabilities in this
single capability spec, including core workflow, adapters, PostgreSQL storage,
triage, autonomy, memory, notifications, uptime diagnostics, worktree and
evaluator safety, skills, budgets, demo applications, and CLI behavior.

#### Scenario: Capability record is reviewed

- **WHEN** an operator reviews the consolidated change
- **THEN** the proposal, design, specification, and tasks describe the
  repository capabilities and their verification state

### Requirement: SHIP-HATS configuration

The GitLab adapter SHALL default to `https://sgts.gitlab-dedicated.com`, and
the LLM provider SHALL honor `ANTHROPIC_BASE_URL` while preserving its default
when unset. GitLab CI error patterns SHALL be configurable through
`error_patterns`.

#### Scenario: SHIP-HATS endpoints are configured

- **WHEN** GitLab and an Anthropic-compatible model gateway are configured
- **THEN** Sidecar uses those endpoints and the configured CI error patterns

### Requirement: Enriched CI failure context

GitLab CI failure signals SHALL best-effort include the failed job, smart error
log context, changed files, commit diff, and flake status. Logs SHALL be
bounded to 150 lines. Commit diffs SHALL be included only when they are at
most 32 KiB; larger diffs SHALL fall back to the complete changed-file list.
Missing API data SHALL degrade to empty optional fields.

Triage SHALL receive failed-job, log, changed-file, and flake context. The
coding agent SHALL receive job logs and the commit diff or changed-file list,
plus the failed job name when available.

#### Scenario: Failed pipeline is enriched

- **WHEN** GitLab reports a failed pipeline and optional diagnostic APIs are
  available
- **THEN** Sidecar supplies bounded failed-job, log, changed-file, diff, and
  flake context to the appropriate agents

#### Scenario: Optional enrichment is unavailable

- **WHEN** an optional GitLab diagnostic request fails
- **THEN** Sidecar continues with empty optional context

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

#### Scenario: Previously processed pipeline is received

- **WHEN** a CI signal has a signal key already stored for the workspace
- **THEN** Sidecar skips task creation before any LLM use

#### Scenario: Signal has no stable identity

- **WHEN** a schedule, on-demand, log, metric, uptime, or identifier-less
  signal is received
- **THEN** Sidecar creates the task without a deduplication key

### Requirement: Backward compatibility

Migrations SHALL be idempotent and additive. Existing tasks SHALL remain valid
with `NULL` signal keys, and existing routing, autonomy, adapter, triage,
evaluator, output, and CLI behavior SHALL remain compatible except where this
spec explicitly adds context or deduplication.

#### Scenario: Existing data and configuration are used

- **WHEN** the migration runs against an existing installation
- **THEN** existing tasks remain valid with a null signal key
- **AND** existing configurations continue using their prior defaults

## Verification status

Unit and integration test source coverage exists for signal-key derivation,
store lookup/uniqueness/null behavior, duplicate CI signals, and
non-deduplicated schedule ticks. Executing the database-tagged tests requires
`SIDECAR_TEST_DB_URL`. Downstream deployment acceptance is outside this
repository's verification scope.
