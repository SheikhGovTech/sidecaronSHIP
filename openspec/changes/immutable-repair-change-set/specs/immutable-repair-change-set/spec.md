# Immutable Repair Change Set

## ADDED Requirements

### Requirement: Sidecar-owned repair history

Sidecar SHALL normalize commits created by an agent back to the task base in
the isolated task worktree while preserving their file changes. Sidecar SHALL
remain the sole creator of the final repair commit.

#### Scenario: Agent creates a local commit

- **WHEN** the coding agent commits changes in the task worktree
- **THEN** Sidecar resets the temporary branch history to the task base while
  preserving the changed files
- **AND** prepares those files through the normal filtering and staging path

#### Scenario: Task base cannot be validated

- **WHEN** the task base is missing or cannot be resolved safely
- **THEN** Sidecar fails change-set preparation
- **AND** does not rewrite history, push a branch, or create a change request

### Requirement: Configurable and safe output exclusions

Sidecar SHALL exclude built-in runtime artifacts and SHALL support additional
worktree-relative output exclusion patterns. Configured exclusions SHALL NOT
override or negate built-in exclusions.

#### Scenario: Runtime artifacts exist

- **WHEN** Harness spill files, coverage data, caches, or configured generated
  outputs exist after coding or verification
- **THEN** matching files are absent from the prepared repair change set

#### Scenario: Invalid exclusion pattern

- **WHEN** an exclusion is empty, absolute, traverses outside the worktree,
  negates a built-in exclusion, or uses malformed syntax
- **THEN** configuration validation fails before task execution

#### Scenario: Excluded tracked change

- **WHEN** an exclusion matches a tracked file modified during the repair
- **THEN** Sidecar omits it from the repair change set
- **AND** records the excluded path in bounded audit metadata

### Requirement: Single prepared staged change set

Sidecar SHALL stage the intended repair exactly once after coding completes and
before deterministic verification or evaluation. The prepared index SHALL be
the authoritative repair snapshot for the remainder of that delivery attempt.

#### Scenario: Repair contains eligible changes

- **WHEN** normalization and filtering leave eligible repair files
- **THEN** Sidecar stages them and records the task base, sorted changed-path
  manifest, change types, and canonical staged-diff digest

#### Scenario: Prepared change set is empty

- **WHEN** no eligible changes remain after filtering
- **THEN** Sidecar follows the existing no-change outcome
- **AND** does not create a repair commit or change request

#### Scenario: Artifact appears after preparation

- **WHEN** verification or evaluation creates or modifies an unstaged file
  after the repair change set is prepared
- **THEN** that file is not added to the approved change set
- **AND** Sidecar does not restage it before committing

### Requirement: Evaluation of the authoritative snapshot

The evaluator SHALL inspect the exact staged repair relative to the task base,
not an unfiltered worktree-wide diff.

#### Scenario: Evaluator reviews a repair

- **WHEN** evaluation begins for a prepared repair
- **THEN** the evaluator receives the approved manifest and staged diff from
  `git diff --cached <task-base>`
- **AND** its verdict applies to that immutable snapshot

#### Scenario: Evaluator creates files

- **WHEN** evaluator tools create or modify worktree files
- **THEN** those changes remain outside the prepared index
- **AND** cannot enter the final commit without a new preparation and
  evaluation cycle

### Requirement: Commit without restaging

After approval, Sidecar SHALL create the final repair commit from the existing
prepared index and SHALL NOT run `git add -A` or any equivalent restaging step.

#### Scenario: Approved repair is committed

- **WHEN** deterministic verification and evaluation permit publication
- **THEN** Sidecar commits the existing index using the attached repository or
  deployment runtime's configured Git identity and signing policy
- **AND** no later worktree artifacts are included

#### Scenario: Index changes after approval

- **WHEN** the staged index no longer matches the approved digest before commit
- **THEN** Sidecar fails closed and does not commit or publish the repair

### Requirement: Pre-publication integrity verification

Before pushing, Sidecar SHALL verify that the final `task-base..HEAD` path
manifest and canonical patch digest exactly match the approved repair snapshot.

#### Scenario: Final commit matches approval

- **WHEN** the committed manifest and digest match the approved snapshot
- **THEN** Sidecar records `repair_change_set_verified`
- **AND** provider-aware branch push and change-request creation may proceed

#### Scenario: Final commit differs from approval

- **WHEN** any committed path, change type, or canonical patch differs from the
  approved snapshot
- **THEN** Sidecar records `repair_change_set_failed` with bounded mismatch
  metadata
- **AND** marks the task failed and emits a failed notification
- **AND** does not push or create a change request

### Requirement: Repair change-set audit trail

Sidecar SHALL record preparation, successful integrity verification, and
failure events without persisting complete source patches or secrets.

#### Scenario: Change set is prepared

- **WHEN** staging succeeds
- **THEN** Sidecar records `repair_change_set_prepared` with the base commit,
  path manifest, exclusion summary, and staged-diff digest

#### Scenario: Change-set operation fails

- **WHEN** normalization, filtering, staging, commit, or integrity checking
  fails
- **THEN** Sidecar records `repair_change_set_failed` with the failed phase and
  bounded redacted diagnostics
