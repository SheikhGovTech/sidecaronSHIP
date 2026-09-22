# Tasks

## Workspace context

- [x] Add resolved workspace path to coding-agent system prompts.
- [x] Add resolved workspace path to evaluator prompts.
- [x] Instruct agents to use relative paths and not guess repository paths.
- [x] Include configured verification commands in coding-agent and evaluator
      prompts.
- [x] Pass one work directory consistently to tools, runtime, and evaluator.
- [x] Support attached repository paths for suggest-only tasks.
- [x] Record `workspace_prepared` task events.

## Deterministic verification

- [x] Add verification command configuration with name, command, timeout,
      relative working directory, `required_tools`, and `pass_env`.
- [x] Define and validate defaults: 10-minute timeout, 30-minute maximum,
      worktree-root directory, 64 KiB output cap, and stop-on-first-failure.
- [x] Define `verification.enabled` behavior for code-shipping, suggest-only,
      and no-change tasks.
- [x] Execute configured commands sequentially after coding completes.
- [x] Preflight declared `required_tools` before starting the coding agent.
- [x] Execute `run` through `/bin/sh -c` and terminate the full process group
      on timeout.
- [x] Enforce timeouts, bounded output, and non-zero exit failure.
- [x] Record started, succeeded, and failed verification events with duration
      and truncation metadata.
- [x] Set failed task status, notify failure, skip evaluator/output, and clean
      worktree/local commits/temporary branch after failed verification.
- [x] Preserve existing behavior when the command list is empty.

## Security and resilience

- [x] Reject absolute or worktree-escaping working directories.
- [x] Reject empty names/commands, duplicate names, invalid durations, `..`,
      and symlink escapes.
- [x] Run as the existing OS user without privilege elevation.
- [x] Document that deployments must run Sidecar as an unprivileged user.
- [x] Use a minimal environment with an explicit credential allowlist.
- [x] Redact known credentials from verification output.
- [x] Fail clearly and early when required verification tools are unavailable.
- [x] Do not install dependencies automatically.

## Tests and documentation

- [x] Add unit tests for prompt workspace injection and path validation.
- [x] Add configuration parsing/default tests, including disabled verification.
- [x] Add tests for timeout, bounded output, success, and failure behavior.
- [x] Add tests for ordering, stop-on-first-failure, suggest-only, and no-change
      behavior.
- [x] Add evaluator prompt, missing-tool, environment allowlist, secret
      redaction, self-commit failure, cleanup, process-group timeout,
      `required_tools`, and symlink escape tests.
- [x] Add integration tests for fail-closed output routing.
- [x] Document deployment responsibility for runtimes and dependencies.
- [x] Run `go test ./...`, `go vet ./...`, and `go build ./...`.
