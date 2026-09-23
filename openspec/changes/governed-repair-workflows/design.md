# Design: Governed Repair Workflows

## Safety-first delivery order

The implementation is divided into four phases:

1. Validate autonomy, make output routing exhaustive, and enforce scope.
2. Add strict decoding, a validated host configuration API, and bounded
   workflow-stage configuration.
3. Separate evaluator outcomes, preserve repairs by policy, and complete usage
   and budget accounting; then add bounded repair cycles and recovery.
4. Add retryable deduplication and lower-priority operational controls.

Each phase must build and test independently. Phase 1 is suitable for a small
security-focused pull request; later phases may be separate pull requests under
this OpenSpec.

## Strict configuration and autonomy routing

Configuration is decoded with YAML known-field checking. Every non-empty
autonomy field must be one of `auto-commit`, `pull-request`, `suggest-only`, or
`notify`; empty fields retain the existing safe `suggest-only` fallback.
Validation errors identify the full configuration path and invalid value.

Output routing uses explicit cases for all four values. The default branch
records `invalid_autonomy`, marks the task failed, emits the failed
notification, performs no commit or delivery, and cleans any temporary
worktree. Runtime validation remains in place as defence in depth even though
startup validation should make the branch unreachable.

Unknown YAML keys are errors. `models.planning` remains a recognized reserved
field for compatibility, but documentation must not imply that a planning
stage currently runs.

## Scope enforcement

Scope patterns are repository-relative, slash-separated glob patterns. An
empty include list means all repository files are included. Excludes always
win. Absolute patterns, `..` segments, empty patterns, and paths resolving
outside the workspace are rejected. Sidecar's Git metadata is never mutable.

Write/edit tools reject known out-of-scope target paths before mutation. Bash
cannot be safely authorized by parsing arbitrary shell text, so Sidecar also
performs an authoritative post-agent diff check before deterministic
verification. That check includes staged, unstaged, committed-from-base,
renamed, deleted, and untracked paths. Both sides of a rename must be in scope.
Symlinks are resolved against the worktree before authorization.

An out-of-scope diff records `scope_violation`, fails the task, skips
verification/evaluation/output, and cleans the isolated worktree and temporary
branch. This is a delivery safety boundary, not a promise that an agent process
cannot temporarily touch an out-of-scope file inside its disposable worktree.

## Workflow policy

The workflow policy is part of the exported Go configuration model so an
application embedding Sidecar can construct or override it without generating
YAML. `config.Default()` returns documented defaults and `Config.Validate()`
applies the same rules used by YAML loading. Library entry points validate a
programmatically supplied configuration before creating a loop, so host code
cannot bypass safety checks. YAML decoding and Go construction converge on one
normalized immutable runtime policy.

```yaml
workflow:
  triage:
    max_turns: 1
    timeout: 2m
    on_error: suggest

  coding:
    max_turns: 20
    timeout: 30m
    retries: 0
    on_error: fail

  evaluator:
    enabled: true
    max_turns: 20
    timeout: 20m
    retries: 0
    retry_backoff: 2s
    use_verification_evidence: true
    on_reject: suggest
    on_error: suggest

  reviewer:
    enabled: true
    max_turns: 4
    timeout: 60s

  execution:
    max_concurrent_tasks: 1
    queue_size: 64

  repair:
    max_cycles: 1
    on_verification_failure: fail
    on_evaluator_reject: suggest
    on_interruption: fail
    max_resume_attempts: 0
```

These defaults preserve current behavior where possible. Every turns, timeout,
retry, queue, and concurrency value has a positive upstream cap. Invalid,
negative, zero where disallowed, or over-cap values fail configuration loading.
Timeout cancels the entire stage. Retries apply only to retryable provider or
transport errors, never to semantic rejections, configuration errors, scope
violations, or deterministic verification failures.

Triage `on_error` supports `suggest`, `skip`, and `fail`. Coding `on_error`
supports `fail` and `preserve-suggestion`. Evaluator `on_reject` supports
`suggest`, `preserve-branch`, and `draft-change-request`; evaluator `on_error`
supports those options plus `fail`. Defaults remain conservative and no error
path can yield approval, a normal ready-for-review request, or auto-merge.

## Bounded repair cycles and recovery

`workflow.repair.max_cycles` counts complete coding attempts, not individual
model turns. The default of one preserves the current single-pass flow. A host
may select `retry-coding` for verification failure or evaluator rejection. In
that case Sidecar starts another coding cycle in the same isolated worktree and
supplies the prior diff plus bounded verification/evaluator evidence as repair
feedback. Scope validation and deterministic verification run again from
scratch, and only the final passing cycle can reach output routing.

Cycles are bounded by `max_cycles`, stage retry limits, stage timeouts, and both
task and daily budgets. Infrastructure errors use stage retry/on-error policy;
they do not consume a repair cycle unless a new coding attempt starts. Scope
violations, invalid configuration, and budget exhaustion can never transition
back to coding.

For process interruption, Sidecar records durable stage checkpoints:
`triaged`, `workspace_prepared`, `coding_checkpointed`,
`verification_completed`, `evaluation_completed`, and `delivery_completed`.
After coding, changes may be stored in a local Sidecar-owned checkpoint ref;
this is not a delivered commit and is never pushed before all gates pass. With
`on_interruption: resume`, a restarted host may reconstruct the isolated
worktree from the recorded base and checkpoint ref, then continue from the last
safe stage. It must rerun scope validation and deterministic verification
before evaluation or delivery. Missing/mismatched checkpoints fail safely.

The host may also choose `on_interruption: fail`, which is the default. Resume
attempts are bounded by `max_resume_attempts`; final failure applies the
configured artifact-preservation policy and cleans internal checkpoint refs
when preservation is not requested. A task lease prevents simultaneous resume
and normal execution of the same task.

## Evaluation evidence and outcomes

Deterministic verification results are structured once and passed to the
evaluator: command, working directory, exit code, duration, truncation flag,
and bounded/redacted output. When `use_verification_evidence` is true, the
evaluator is instructed to assess this evidence and the exact diff rather than
rerunning the same commands. It may use read-only investigation commands, but
cannot mutate the workspace.

Evaluation returns one of `pass`, `reject`, or `error`. Parse failures,
provider failures, timeouts, and exhausted turns are `error`; a valid negative
verdict is `reject`. Retries occur before applying `on_error`. Neither outcome
is converted to `pass`.

Draft change requests must be visibly marked as failed evaluation, include the
reasons or error, and use provider draft semantics. If the provider cannot
create drafts, delivery falls back to preserving the Sidecar-owned branch and
records that fallback.

## Change-request decision evidence

Every Sidecar-created pull or merge request includes bounded, sanitized
decision evidence for both autonomous roles. This evidence is derived from
structured task/trace records; it is not hidden chain-of-thought and does not
contain complete prompts, credentials, raw CI logs, unbounded tool output, or
full source-file contents.

The repair-agent section includes:

- task and coding trace identifiers;
- model, attempt/cycle, turns used, and configured turn limit;
- visible final repair summary;
- changed-file list;
- sanitized tool/command summary with repetition counts, durations, exit codes,
  and truncation state;
- deterministic verification commands and outcomes; and
- reported token usage, with unavailable metering identified explicitly.

The evaluator section includes:

- evaluator trace identifier, model, attempt, turns used, and turn limit;
- outcome `pass`, `reject`, or `error`;
- bounded verdict reasons or sanitized terminal error;
- verification evidence consulted;
- repeated tool/command summary and reported token usage; and
- an explicit statement of whether evaluator approval was obtained.

The complete sanitized trace remains in the durable trace store defined by the
`durable-agent-execution-tracing` change. The change request includes trace IDs
and an authorized trace URL when configured; it does not copy the complete
trace into the provider. The combined embedded evidence is capped at 32 KiB,
with deterministic truncation markers and trace references for omitted detail.

When an evaluator exhausts its turn limit, the outcome is `error`, never
`pass`. If `on_error: draft-change-request` is configured, Sidecar creates a
draft with task status `needs_review`, applies the
`sidecar:evaluation-error` label where supported, and posts/embeds a prominent
notice that deterministic verification passed but adversarial evaluation did
not complete. The draft requires human approval and cannot be auto-merged. If
trace persistence is incomplete, the draft states that audit evidence is
incomplete rather than silently omitting it.

## Artifact preservation

Preservation applies only to isolated Sidecar-owned worktrees and branches.
`suggest` stores the bounded coding summary and evaluation details, then cleans
the worktree. `preserve-branch` commits the exact checked diff to the local
Sidecar-owned branch without pushing it. `draft-change-request` pushes the
branch and creates a draft request. Cleanup never deletes a branch not matching
the validated Sidecar-owned prefix.

## Usage and budgets

All model stages return usage accumulated before success or error. Usage events
record stage, model, attempt, input, output, cache, and total tokens. Provider
usage that is unavailable is represented explicitly rather than as zero usage.

```yaml
budget:
  daily_tokens: 0
  per_task_tokens: 0
  timezone: UTC
  on_metering_error: allow
```

`0` remains unlimited. Before each model attempt, Sidecar checks both budgets.
After each attempt it records returned usage, including partial usage on error.
`on_metering_error` supports `allow`, `skip`, and `fail`; `allow` preserves the
current daily-budget behavior. Budget exhaustion prevents another attempt but
does not retroactively discard recorded usage.

## Deduplication and retry

```yaml
workflow:
  deduplication:
    terminal_statuses: [completed, skipped, suggested, notified]
    retry_failed_after: 15m
```

A signal is suppressed only when a matching task has a configured terminal
status, is currently active, or failed within the retry delay. Failed tasks may
be retried after the delay. A manual retry creates a new attempt linked to the
original task and bypasses only the dedupe decision; it does not bypass budget,
scope, verification, or evaluation gates. Concurrency-safe database claims
prevent two workers from processing the same signal simultaneously.

## Bounded operational controls

Verification adds configurable output limit and stop-on-failure behavior while
retaining a hard output cap and maximum timeout. Delivery and notifications add
bounded timeout, retry, and exponential-backoff settings. Branch prefix,
memory top-k/similarity threshold, task concurrency/queue size, and GitLab CI
log/diff/history/API limits become validated configuration.

Notifications remain best-effort by default. A required notification may fail
the task but can never reverse a successful external delivery. Delivery retries
must be idempotent and reuse an existing branch or change request.

## Observability

Task events record policy decisions and attempt numbers without credentials:
`invalid_autonomy`, `scope_violation`, `stage_retry`, `evaluation_error`,
`evaluation_rejected`, `artifact_preserved`, `budget_exceeded`,
`metering_error`, `dedupe_suppressed`, and `manual_retry`.
