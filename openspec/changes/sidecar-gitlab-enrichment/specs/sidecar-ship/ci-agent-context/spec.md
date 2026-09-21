# Spec Delta

## Purpose

Gives the coding agent actual CI error output and commit diff in its prompts so it can identify and fix the root cause without guessing from the code alone.

## ADDED Requirements

### Requirement: Include error output in coding agent system prompt
The `BuildSystemPrompt()` function SHALL include `job_log` in the CI failure system prompt when present.

#### Scenario: Error output available
- **WHEN** a CI failure signal reaches the coding agent with non-empty `job_log`
- **THEN** the system prompt includes the CI error output under a clear heading
- **AND** the agent can identify the failing test, error message, and stack trace

#### Scenario: Error output unavailable
- **WHEN** `job_log` is empty
- **THEN** the system prompt falls back to the upstream format ("check recent changes, read failing test output if accessible")

### Requirement: Include commit diff in coding agent system prompt
The `BuildSystemPrompt()` function SHALL include `commit_diff` in the CI failure system prompt when present.

#### Scenario: Diff available
- **WHEN** a CI failure signal has non-empty `commit_diff`
- **THEN** the system prompt includes the diff so the agent can see exactly what changed
- **AND** the agent can correlate the error with the specific code change

#### Scenario: Large diff
- **WHEN** the commit diff exceeds 32KB
- **THEN** only `changed_files` (file list) is included instead of the full diff

### Requirement: Include failed job name in user message
The `userMessage()` function SHALL include `failed_job` in the CI failure user message when present.

#### Scenario: Failed job known
- **WHEN** a CI failure signal has non-empty `failed_job`
- **THEN** the user message includes "Failed job: {name}" so the agent knows which CI step broke

#### Scenario: Failed job unknown
- **WHEN** `failed_job` is empty
- **THEN** the user message is identical to the upstream format
