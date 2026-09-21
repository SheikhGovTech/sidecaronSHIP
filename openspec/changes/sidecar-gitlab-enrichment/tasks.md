# Tasks

## 1. GitLab CI Adapter Enrichment (`internal/adapter/gitlabci/gitlabci.go`)

- [x] 1.1 Change `defaultBaseURL` from `"https://gitlab.com"` to `"https://sgts.gitlab-dedicated.com"`
- [x] 1.2 Add `fetchFailedJobLogs()` method — fetch failed job name + trace via `/pipelines/:pid/jobs` and `/jobs/:jid/trace`
- [x] 1.3 Add `extractErrorContext()` function — scan log for error patterns with ±2 line context window, always include last 30 lines, cap at 150 lines
- [x] 1.4 Add default error patterns list (`Error:`, `FAIL`, `panic:`, `Uncaught Exception`, `not found`, `not defined`, `exit code`, `Test Files`, `AssertionError`, `TypeError`)
- [x] 1.5 Add `fetchCommitDiff()` method — fetch diff via `GET /commits/:sha/diff`, extract file list and full diff (64KB cap, fall back to file list only if >32KB)
- [x] 1.6 Add `detectFlake()` method — fetch last 5 pipelines for same ref, check alternating pass/fail pattern
- [x] 1.7 Update `poll()` to call all enrichment methods and include `failed_job`, `job_log`, `commit_diff`, `changed_files`, `is_flake` in signal payload
- [x] 1.8 Add `SetErrorPatterns()` for custom pattern support from config

## 2. Config Extension (`internal/config/config.go`)

- [x] 2.1 Add `ErrorPatterns []string` field to `SignalConfig` struct with yaml tag `error_patterns`

## 3. Adapter Builder (`internal/cli/attach.go`)

- [x] 3.1 Wire `ErrorPatterns` from config to GitLab CI adapter via `SetErrorPatterns()`

## 4. Triage Prompt (`internal/triage/triage.go`)

- [x] 4.1 Add `failed_job` and `job_log` extraction to CI failure case in `BuildTriageMessage()`
- [x] 4.2 Add `changed_files` extraction to CI failure case
- [x] 4.3 Update `is_flake` usage — note suspected flake in triage prompt
- [x] 4.4 Guard all new fields with `if != ""` / `if != false` checks

## 5. Coding Agent Prompts (`internal/loop/loop.go`)

- [x] 5.1 Change `NewAnthropicProvider(key, "")` to `NewAnthropicProvider(key, os.Getenv("ANTHROPIC_BASE_URL"))` for PAi routing
- [x] 5.2 Add `failed_job` and `job_log` to CI failure case in `BuildSystemPrompt()`
- [x] 5.3 Add `commit_diff` or `changed_files` to CI failure case in `BuildSystemPrompt()`
- [x] 5.4 Add `failed_job` to CI failure case in `userMessage()`

## 6. Verification (downstream deployment)

- [ ] 6.1 Deploy via downstream project and verify sidecar starts with no warnings
- [ ] 6.2 Trigger a CI failure and verify triage prompt includes error logs, changed files, flake status
- [ ] 6.3 Verify triage acts (should_act: true) instead of skipping
- [ ] 6.4 Verify coding agent sees error output and commit diff in its prompts
