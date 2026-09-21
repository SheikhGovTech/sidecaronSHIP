# Tasks

## 1. GitLab CI Adapter Enrichment (fork: `internal/adapter/gitlabci/gitlabci.go`)

- [x] 1.1 Change `defaultBaseURL` from `"https://gitlab.com"` to `"https://sgts.gitlab-dedicated.com"`
- [x] 1.2 Add `fetchFailedJobLogs()` method — fetch failed job name + trace via `/pipelines/:pid/jobs` and `/jobs/:jid/trace`
- [x] 1.3 Add `extractErrorContext()` function — scan log for error patterns with ±2 line context window, always include last 30 lines, cap at 150 lines
- [x] 1.4 Add default error patterns list (`Error:`, `FAIL`, `panic:`, `Uncaught Exception`, `not found`, `not defined`, `exit code`, `Test Files`, `AssertionError`, `TypeError`)
- [x] 1.5 Add `fetchCommitDiff()` method — fetch diff via `GET /commits/:sha/diff`, extract file list and full diff (64KB cap, fall back to file list only if >32KB)
- [x] 1.6 Add `detectFlake()` method — fetch last 5 pipelines for same ref via `GET /pipelines?ref=:ref&per_page=5`, check alternating pass/fail pattern
- [x] 1.7 Update `poll()` to call all enrichment methods and include `failed_job`, `job_log`, `commit_diff`, `changed_files`, `is_flake` in signal payload
- [x] 1.8 Read `error_patterns` from `SignalConfig` via `SetErrorPatterns()` method

## 2. Config Extension (fork: `internal/config/config.go`)

- [x] 2.1 Add `ErrorPatterns []string` field to `SignalConfig` struct with yaml tag `error_patterns`

## 3. Triage Prompt (fork: `internal/triage/triage.go`)

- [x] 3.1 Add `failed_job` and `job_log` extraction to CI failure case in `BuildTriageMessage()`
- [x] 3.2 Add `changed_files` extraction to CI failure case — show list of modified files
- [x] 3.3 Update `is_flake` usage — note suspected flake in triage prompt when `true`
- [x] 3.4 Guard all new fields with `if != ""` / `if != false` checks — no regression when fields are empty

## 4. Coding Agent Prompts (fork: `internal/loop/loop.go`)

- [x] 4.1 Change `NewAnthropicProvider(key, "")` to `NewAnthropicProvider(key, os.Getenv("ANTHROPIC_BASE_URL"))` for PAi routing
- [x] 4.2 Add `failed_job` and `job_log` to CI failure case in `BuildSystemPrompt()` — include error output under "CI error output:" heading
- [x] 4.3 Add `commit_diff` or `changed_files` to CI failure case in `BuildSystemPrompt()` — include diff under "Commit diff:" heading (file list only if diff >32KB)
- [x] 4.4 Add `failed_job` to CI failure case in `userMessage()` — include job name in the task instruction

## 5. Push Fork and Update UQD2

- [x] 5.1 Commit all changes to fork with descriptive message
- [x] 5.2 Push fork to `SheikhGovTech/sidecaronSHIP` master (ca8ae6e)
- [x] 5.3 Update UQD2 `services/sidecar/Dockerfile` — set `SIDECAR_SHA` to ca8ae6e, clone from SheikhGovTech/sidecaronSHIP
- [x] 5.4 Globe mock already in `AdminManagementPage.test.tsx`
- [x] 5.5 Commit and push UQD2 changes (d2e4b79)
- [ ] 5.6 Wait for pipeline to build and deploy sidecar image

## 6. Test the Full Loop

- [ ] 6.1 Verify sidecar starts with no warnings (GitLab CI, uptime, Slack all connected)
- [ ] 6.2 Revert Globe mock fix to trigger test failure
- [ ] 6.3 Wait for pipeline to fail → sidecar detects it
- [ ] 6.4 Verify triage prompt includes: failed job name, error log extract, changed files, flake status
- [ ] 6.5 Verify triage acts (should_act: true) instead of skipping
- [ ] 6.6 Verify Slack notification includes error context
- [ ] 6.7 If coding agent runs: verify it sees error output and commit diff in its prompts
- [ ] 6.8 Restore Globe mock fix after test
