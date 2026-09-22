# Proposal: Workspace-Aware Repair Execution and Deterministic Verification

## Problem

Sidecar creates isolated worktrees and configures tools to use them, but the
coding agent is not given the resolved workspace path. Agents can therefore
guess invalid repository locations, waste turns, and fail without producing a
repair.

Sidecar also asks agents and evaluators to run tests without a deterministic,
configurable verification command. This is unreliable in persistent container
deployments where project dependencies and tools may not be installed.

The adversarial evaluator is currently limited to eight turns. In an observed
repair, it exhausted that allowance before completing its assessment. Sidecar
then downgraded the merge-request fix to a suggestion, so no branch or GitLab
merge request was delivered despite the coding agent having produced a repair.

## Intended outcome

For every actionable task, Sidecar SHALL:

- Provide the exact resolved worktree path to tools, the coding agent, and the
  evaluator.
- Instruct agents to use relative paths and not guess repository locations.
- Run configured verification commands from the correct worktree directories.
- Fail closed before commit or merge-request output when verification fails.
- Allow the evaluator up to 20 turns to complete its assessment before treating
  it as exhausted.
- Continue to evaluation and output routing only after successful verification.

Sidecar remains language-agnostic. Deployments remain responsible for language
runtimes, package managers, dependencies, and prepared repair images.

## Scope

Add workspace context to prompts, add sequential configurable verification
commands with timeouts and bounded output, record workspace and verification
events, validate worktree-relative paths, and preserve existing behavior when
no verification commands are configured.

Each command declares its required executables explicitly. Sidecar executes the
configured `run` string through `/bin/sh -c`, terminates the complete process
group on timeout, and does not attempt to infer tools from shell syntax.
