# Design: Deterministic Agent Completion

## Completion is separate from exhaustion

`MaxTurns` remains the absolute safety ceiling. It is not the success
criterion. Each runtime stage receives a Sidecar-owned completion contract:

```text
start
  -> perform bounded work
  -> satisfy observable criteria
  -> emit the required structured result
  -> stop
```

If the required result is absent when bounded work ends, the outcome is
`incomplete`. Provider failures, cancellation, persistence failures, and other
runtime faults remain `error`. A valid negative evaluator decision is
`reject`. These states are never interchangeable.

## Coding completion contract

The coding stage completes only when the Harness runtime terminates normally
and produces one of these observable outcomes:

1. a relevant workspace change plus a bounded visible final summary; or
2. an explicit no-change result with a bounded explanation.

For code-shipping autonomy, Sidecar remains authoritative after the model
stops: it checks the diff, scope, and configured deterministic verification.
Agent claims that tests passed are not sufficient evidence.

An exhausted coding runtime, missing terminal result, or partial edit without
a completion result is `coding_incomplete`. Useful findings may become a
suggestion; partial code cannot pass directly to normal delivery.

## Evaluator evidence contract

The evaluator receives:

- the exact candidate diff;
- task intent and changed paths;
- each deterministic verification command and working directory;
- exit code, duration, truncation state, and bounded redacted output;
- prior evaluator tool hashes and repetition counts as they accumulate; and
- an explicit material-risk checklist.

When deterministic verification has passed, the evaluator is instructed not
to rerun an equivalent command unless it identifies a concrete inconsistency
or uncovered risk. An exception records the reason before execution.

Billing analysis, general repository exploration, repeated Git-history review,
and checks unrelated to the candidate diff are outside the evaluator contract.

## Investigation and finalization phases

Evaluation has two Sidecar-enforced phases:

```text
investigation
  -> inspect diff and bounded evidence
  -> run only targeted material-risk checks
  -> submit verdict early when sufficient

finalization
  -> no filesystem or Bash tools
  -> emit one structured verdict
  -> terminate
```

The hard evaluator limit remains 20 turns by default. Sidecar applies a lower
internal investigation boundary and reserves at least one provider request for
finalization. The exact public tuning of these limits is deferred to the host
configuration change; PR 5 ships conservative defaults and hard caps.

Sidecar enters finalization when any of these occurs:

- all required checks are complete and no material risk remains unresolved;
- the investigation boundary is reached;
- a repeated-tool guard is triggered; or
- the evaluator explicitly indicates that it is ready to decide.

Entering finalization disables investigative tools. Additional tool requests
are rejected without execution and cannot consume the reserved verdict
capacity.

## Structured verdict

The evaluator terminal result is strict JSON:

```json
{
  "outcome": "pass",
  "reasons": "Deterministic verification passed and no material regression was found.",
  "evidence": ["verification:backend-tests", "diff:src/model.go"]
}
```

`outcome` accepts only `pass` or `reject`. The reasons and evidence references
are bounded. Sidecar rejects malformed, missing, or contradictory results.

A valid verdict terminates the evaluator immediately. Sidecar does not permit
more investigation after receiving it.

## Repetition control

Tool calls are identified by tool name plus the sanitized argument hash from
durable tracing. Equivalent deterministic verification commands are also
identified by normalized command, working directory, and environment
allowlist.

The first duplicate is rejected with existing evidence and an instruction to
decide or identify a materially different check. A further duplicate moves the
runtime to finalization. Repetition counters remain in the trace and human
handoff.

This control does not compare or persist hidden reasoning.

## Outcomes and routing

Sidecar records these evaluator outcomes:

| Outcome | Meaning | Normal routing |
|---|---|---|
| `pass` | Valid positive verdict | Continue through output policy |
| `reject` | Valid negative verdict | Apply rejection policy with reasons |
| `incomplete` | No valid verdict within the completion contract | Preserve useful work without approval |
| `error` | Provider, runtime, persistence, or cancellation failure | Apply error policy without approval |

The default incomplete action is `suggest`. If deterministic verification
passed and existing policy explicitly selects `draft-change-request`, Sidecar
may create a draft marked `needs_review` and `sidecar:evaluation-incomplete`.
It is never represented as evaluator-approved or ready to merge.

If no useful bounded output or valid diff exists, Sidecar marks the task failed
instead of creating an empty suggestion.

## Human handoff

An incomplete handoff contains only sanitized, bounded evidence:

```yaml
status: suggested
reason: evaluator_incomplete
completed:
  - candidate diff produced
  - deterministic verification passed: 2612 tests
remaining:
  - independent evaluator verdict
recommended_action: Review the verified diff manually.
trace_id: <trace-id>
task_id: <task-id>
```

Sidecar persists this record as a task event. Suggested notifications include a
bounded summary and task ID. A task-detail CLI command displays the sanitized
handoff and trace reference. A verified draft change request embeds the same
notice prominently.

## Safety invariants

- `incomplete` and `error` can never become `pass`.
- Failed deterministic verification can never ship.
- Finalization always has at least one reserved request.
- Investigation cannot consume reserved finalization capacity.
- Tool denial cannot silently become approval.
- Hidden reasoning, complete prompts, raw logs, credentials, and file bodies
  are never included in completion evidence.
- The hard Harness turn limit remains enforced even if Sidecar completion
  logic fails.

## Relationship to other changes

`durable-agent-execution-tracing` supplies the observable event and usage
foundation. This change consumes those events to enforce completion.

A future `host-configurable-repair-workflows` change may expose bounded
investigation limits, stage timeouts, retry policy, and incomplete routing to
embedding applications. It may tune the contract but cannot weaken the safety
invariants above.
