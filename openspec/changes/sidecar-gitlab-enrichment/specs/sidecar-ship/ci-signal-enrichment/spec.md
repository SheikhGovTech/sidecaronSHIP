# Spec Delta

## Purpose

Enriches CI failure signals with job logs, commit diffs, changed files, and pipeline history by leveraging the GitLab API, bringing CI signals to parity with the uptime adapter's diagnostic enrichment.

## ADDED Requirements

### Requirement: Fetch failed job logs with smart error extraction
The adapter SHALL fetch the failed job's trace via the GitLab API and extract error-relevant lines using pattern matching rather than blind tail truncation.

#### Scenario: Test failure with error in the middle of a 5000-line log
- **WHEN** a pipeline fails and the failed job trace is 5000+ lines with the error at line 3500
- **THEN** the signal payload `job_log` contains the error lines (matching patterns like `Error:`, `FAIL`, `panic:`, `Uncaught Exception`) with ±2 lines of context, plus the last 30 lines (test summary)
- **AND** the total extracted content is capped at 150 lines

#### Scenario: Build error at the end of the log
- **WHEN** a pipeline fails with a compilation error in the last 50 lines
- **THEN** the signal payload `job_log` contains those error lines via both pattern matching and tail inclusion

#### Scenario: No failed jobs (canceled pipeline)
- **WHEN** a pipeline is canceled with no failed jobs
- **THEN** `failed_job` and `job_log` are empty strings
- **AND** the signal still fires with pipeline-level metadata

#### Scenario: Job trace API unavailable
- **WHEN** the job trace API returns 403 or times out
- **THEN** `failed_job` and `job_log` are empty strings (graceful degradation)

### Requirement: Fetch commit diff
The adapter SHALL fetch the commit diff for the failed pipeline's HEAD SHA via the GitLab API.

#### Scenario: Commit diff available
- **WHEN** a pipeline fails on commit `abc123`
- **THEN** the signal payload `commit_diff` contains the diff output (capped at 64KB)
- **AND** `changed_files` contains the list of modified file paths

#### Scenario: Commit diff API unavailable
- **WHEN** the diff API returns an error
- **THEN** `commit_diff` and `changed_files` are empty (graceful degradation)

### Requirement: Pipeline history for flake detection
The adapter SHALL fetch the last 5 pipelines for the same branch to detect flaky failures.

#### Scenario: First-time failure
- **WHEN** the previous 4 pipelines on the same branch all passed
- **THEN** `is_flake` is `false` in the signal payload

#### Scenario: Intermittent failure pattern
- **WHEN** the last 5 pipelines alternate between pass and fail
- **THEN** `is_flake` is `true` in the signal payload

#### Scenario: Pipeline history unavailable
- **WHEN** the pipeline history API fails
- **THEN** `is_flake` defaults to `false`

### Requirement: Configurable error patterns
The adapter SHALL use a default set of error patterns and allow additional patterns via the `error_patterns` config field.

#### Scenario: Default patterns
- **WHEN** no `error_patterns` are configured
- **THEN** the adapter uses built-in patterns: `Error:`, `FAIL`, `panic:`, `Uncaught Exception`, `not found`, `not defined`, `exit code`, `Test Files`, `AssertionError`, `TypeError`

#### Scenario: Custom patterns
- **WHEN** `error_patterns: ["MyCustomError", "REGRESSION"]` is configured
- **THEN** those patterns are added to the defaults for log extraction
