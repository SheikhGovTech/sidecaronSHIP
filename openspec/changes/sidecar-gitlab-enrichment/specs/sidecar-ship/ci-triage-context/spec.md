# Spec Delta

## Purpose

Surfaces all enriched CI signal data in the triage prompt so the triage agent can make informed act/skip decisions based on actual error output, change scope, and failure history.

## ADDED Requirements

### Requirement: Include job logs in triage prompt
The `BuildTriageMessage()` function SHALL include `failed_job` and `job_log` in the CI failure triage prompt when present.

#### Scenario: Job logs available
- **WHEN** a CI failure signal has non-empty `failed_job` and `job_log`
- **THEN** the triage prompt includes the job name and error log extract
- **AND** triage has enough context to identify the error type (test failure, build error, dependency issue)

#### Scenario: Job logs unavailable
- **WHEN** `failed_job` and `job_log` are empty
- **THEN** the triage prompt is identical to the upstream format (no regression)

### Requirement: Include commit diff context in triage prompt
The `BuildTriageMessage()` function SHALL include `changed_files` in the CI failure triage prompt to convey the scope of the change.

#### Scenario: Changed files available
- **WHEN** a CI failure signal has non-empty `changed_files`
- **THEN** the triage prompt includes the list of changed file paths
- **AND** triage can assess whether the change scope is likely related to the failure

### Requirement: Include flake indicator in triage prompt
The `BuildTriageMessage()` function SHALL include the `is_flake` determination when pipeline history detected an intermittent failure pattern.

#### Scenario: Flaky failure detected
- **WHEN** `is_flake` is `true`
- **THEN** the triage prompt notes this is a suspected flaky test
- **AND** triage can decide to skip rather than fixing a non-deterministic issue

### Requirement: Consistency with uptime adapter prompt
- **WHEN** comparing the CI failure triage prompt with the uptime failure triage prompt
- **THEN** both provide diagnostic context sufficient for triage to make an informed decision
- **AND** the CI prompt follows the same pattern: metadata + diagnostic detail + decision question
