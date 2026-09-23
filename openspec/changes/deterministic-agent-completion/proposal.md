# Proposal: Deterministic Agent Completion

## Problem

Sidecar bounds agent execution with `MaxTurns`, but a hard ceiling does not
define when an agent has completed its job. An agent can continue investigating
after sufficient evidence exists and consume every available turn without
emitting its required result.

A traced production canary demonstrated this failure mode. The coding agent
restored the intended model, deterministic verification passed 2,612 tests,
and the evaluator collected enough evidence to approve or reject the repair.
The evaluator nevertheless reran long tests, reread covered cases, explored
unrelated billing impact, and repeatedly inspected Git history. It exhausted
20 requests without emitting a verdict.

Increasing `MaxTurns` would postpone the same failure. Sidecar needs an
enforceable completion contract, not a larger investigation budget.

## Intended outcome

Sidecar SHALL define observable success criteria for coding and evaluation,
stop agents when those criteria are satisfied, and reserve capacity for their
required terminal result.

The evaluator SHALL reuse deterministic verification evidence, perform only
material risk checks, stop repeated investigation, and enter a verdict-only
finalization phase before its hard turn limit. A valid verdict SHALL terminate
evaluation immediately.

If an agent cannot satisfy its completion contract, Sidecar SHALL record an
`incomplete` outcome rather than misclassifying it as success or rejection.
Useful partial work SHALL be handed to a human as a bounded suggestion or, for
a verified repair, an explicitly unapproved draft change request according to
the existing preservation policy.

## Scope

This change adds:

- explicit coding and evaluator completion contracts;
- structured deterministic-verification evidence for evaluation;
- a bounded evaluator investigation phase and reserved finalization phase;
- repeated and redundant tool-call controls;
- an authoritative structured evaluator verdict;
- distinct `pass`, `reject`, `incomplete`, and `error` outcomes;
- bounded human handoff details for incomplete work; and
- tests covering timely completion and safe exhaustion.

The implementation uses the durable events introduced by
`durable-agent-execution-tracing` to measure requests, repeated tools, usage,
and terminal outcomes. It does not store or depend on hidden chain-of-thought.

## Non-goals

This change does not expose every workflow control to embedding applications,
add unbounded retries, increase model privileges, install project dependencies,
replace CI, or auto-approve incomplete evaluation. Broader host configuration,
repair cycles, restart recovery, concurrency, and operational tuning belong to
a separate `host-configurable-repair-workflows` change.
