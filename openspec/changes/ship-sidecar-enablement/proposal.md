# Proposal: Consolidate Sidecar on SHIP Features

## Why

`sidecaronSHIP` is a fork of Sidecar adapted for SHIP-HATS GitLab and
GovPaaS/Platform AI operation. This change records the complete feature set
implemented from the fork through the current CI enrichment and persistent
signal deduplication work in one OpenSpec change.

## TL;DR

The uptime adapter enriches signals with DNS, TCP, and TLS diagnostics before
triage. The CI adapters—GitLab, GitHub, and CircleCI—previously emitted only
bare pipeline status, with no error context. As a result, triage had no useful
information to diagnose and the coding agent never ran.

This was the only signal path where the adapter-to-triage pipeline routinely
produced no actionable output.

## Background and problem statement

### Observed behavior

CI failures were skipped with messages equivalent to:

```text
Insufficient information provided to determine root cause - no error logs,
failure details, or specific test output available for diagnosis.
```

This occurred across test failures, build errors, and dependency issues. The
observed CI failure skip rate was 100% across more than ten pipeline failures.

### Expected behavior

Triage should receive enough diagnostic context to distinguish an actionable
failure—such as a broken test or build—from noise such as a canceled pipeline
or infrastructure flake. The CI path should provide the same decision-making
context that the uptime path already provides.

### Restart and duplicate-processing problem

CI adapters also used in-memory `seen` maps. Restarting Sidecar reset those
maps, causing recent failed pipelines to be emitted again. Each duplicate
could trigger triage, the coding agent, and the evaluator, spending tokens on
work already represented in the tasks database. Persistent signal keys and a
database-backed gate solve this restart problem before any LLM call.

## What changes

- Document the core Sidecar signal-to-triage-to-agent workflow and storage.
- Document uptime diagnostics, notifications, demo applications, worktree and
  evaluator safety, skills, and token budgets.
- Document SHIP-HATS GitLab and Platform AI configuration.
- Document GitLab CI failure enrichment for logs, diffs, changed files, and
  flaky pipeline detection.
- Document restart-safe database-backed deduplication for CI and Git signals.

## Compatibility

The changes are additive. Existing tasks remain valid, enrichment fields are
optional, non-idempotent event signals remain non-deduplicated, and existing
autonomy and routing behavior is preserved.
