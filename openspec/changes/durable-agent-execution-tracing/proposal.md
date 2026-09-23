# Proposal: Durable Agent Execution Tracing

## Problem

Sidecar uses Harness as its agent runtime. Harness already emits visible model
text, completed tool calls and results, per-request usage, compaction,
completion, abort, and error events. Sidecar currently consumes only enough of
that stream to assemble final text, terminal usage, and an error. Most runtime
evidence is discarded instead of becoming part of the task audit trail.

This made a production evaluator failure difficult to diagnose. The repair
agent produced a valid change and deterministic verification passed 2,612
tests. The evaluator then exceeded its 20-turn limit. Sidecar persisted
`pass: false` and converted the task to a suggestion even though the event was
a runtime error, not a semantic rejection. Server logs reported approximately
483,545 evaluator tokens, but no evaluator usage event was saved. The database
therefore recorded only approximately 98,496 tokens instead of the task's
approximately 582,000-token consumption and could not enforce the configured
500,000-token daily budget accurately.

## Intended outcome

Sidecar SHALL consume Harness runtime events for coding, evaluator, and reviewer
agents and persist a sanitized, ordered action trace. Operators SHALL be able to
answer which model requests occurred, which tools and commands were repeated,
what bounded results they produced, which files were accessed, how many tokens
each provider attempt consumed, and whether the run completed, rejected, or
failed.

The trace SHALL never store hidden chain-of-thought, thinking blocks, complete
prompts, credentials, environment values, or unbounded command/CI output.
Evaluator runtime errors SHALL be represented as errors, never as rejections.
Per-request usage SHALL remain durable on error paths and SHALL feed budget
accounting without double-counting terminal aggregates.

## Scope

This change adds:

- a dedicated `agent_trace_events` table;
- conservative trace configuration and retention;
- a Sidecar trace collector for existing Harness `AgentEvent` streams;
- sanitization, redaction, hashing, output bounds, and safe structured logs;
- per-request usage persistence on success and failure paths;
- explicit evaluator pass/reject/error trace outcomes; and
- tests and operator documentation for trace privacy and accounting.

No Harness change is required for this scope. Sidecar remains responsible for
choosing workflow policy and persisting runtime evidence; Harness remains
responsible for emitting runtime events and enforcing the configured turn loop.

## Non-goals

This change does not store hidden reasoning, create a replayable prompt archive,
retain raw CI logs, change model-provider wire formats, define evaluator retry
or worktree-preservation policy, or replace metrics/log aggregation. Broader
workflow recovery and preservation remain part of the separate governed repair
workflow change.
