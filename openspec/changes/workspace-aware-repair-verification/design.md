# Design: Workspace-Aware Repair Verification

## Workspace contract

After triage and worktree creation, one resolved `workDir` is the execution
authority for the task. It is passed to filesystem tools, Bash, the coding
runtime, prompt context, and the evaluator. Prompts identify the exact path and
instruct the agent to use relative paths and run `pwd` rather than guessing a
repository location.

Suggest-only tasks use the attached repository path; code-shipping tasks use
their generated isolated worktree.

## Verification configuration

Extend `verification` with sequential commands:

```yaml
verification:
  enabled: true
  commands:
    - name: backend-tests
      run: python -m pytest tests/ --tb=short
      required_tools: [python]
      timeout: 10m
      working_directory: .
      pass_env: [PATH, HOME]
```

`working_directory` is relative to the worktree. Empty commands preserve
current behavior. The default timeout is 10 minutes and the maximum is 30
minutes. The default working directory is the worktree root. Captured output is
capped at 64 KiB per command, commands stop on the first failure, and non-zero
exits fail verification.

The command list is included in both coding-agent and evaluator prompts. The
agent is encouraged to run the commands during repair; Sidecar independently
runs them after the agent completes.

`verification.enabled: false` disables deterministic commands and the evaluator
gate. Commands run only for code-shipping tasks when verification is enabled;
suggest-only tasks and tasks with no diff do not run them. Commands run before
the evaluator and output routing.

`run` executes through `/bin/sh -c`. Sidecar does not parse arbitrary shell
syntax to discover executables; each command declares `required_tools`, which
are resolved during preflight using the verification environment's `PATH`.
Timeout cancellation terminates the command's entire process group, including
child processes.

## Execution flow

```text
agent completes
  → configured verification commands
  → fail closed on error
  → adversarial evaluator
  → commit / pull request output
```

Verification failure must prevent branch push and merge-request creation. The
task becomes `failed`, failure notification fires, evaluator/output routing is
skipped, the worktree is cleaned up, agent-created local commits are discarded
with it, and the temporary task branch is deleted where safe.

## Evaluator turn allowance

The evaluator runtime uses a 20-turn allowance instead of eight. Eight turns
proved insufficient for an observed repair assessment: the evaluator exhausted
its allowance, and Sidecar downgraded the merge-request fix to a suggestion
without delivering a branch or GitLab merge request. Twenty turns gives the
evaluator room to inspect the repair and verification evidence while retaining
a finite upper bound on model work.

## Security

- Reject absolute and escaping working directories.
- Apply per-command timeouts and bounded output.
- Redact known credentials from recorded output.
- Run as the existing Sidecar OS user and never elevate privileges. Deployments
  are responsible for running Sidecar as an unprivileged user.
- Use a minimal subprocess environment with an explicit `pass_env` allowlist.
- Never install dependencies automatically from an untrusted branch.
- Do not expose unrelated credentials to verification runners.

Before starting the coding model, preflight every explicitly declared
`required_tools` executable. Missing tools fail clearly without spending agent
turns.
Configuration rejects empty names, empty commands, duplicate names, invalid
durations, absolute paths, `..` traversal, and symlink escapes.

## Audit events

Record `workspace_prepared` with task ID, branch, and base commit. Record
`verification_started`, `verification_succeeded`, and `verification_failed`
with command name, exit code, duration, truncation state, and bounded output. A
temporary absolute path may be logged for runtime diagnosis but is not durable
task state.

## Non-goals

This change does not install dependencies, become a language-specific build
system, orchestrate containers, replace CI, or act as a remote job runner.
GitLab CI remains the final merge-time verification authority.
