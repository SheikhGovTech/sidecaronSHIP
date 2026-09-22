# Proposal: Consolidate Sidecar on SHIP Features

## Why

`sidecaronSHIP` is a fork of Sidecar adapted for SHIP-HATS GitLab and
GovPaaS/Platform AI operation. This change records the complete feature set
implemented from the fork through the current CI enrichment and persistent
signal deduplication work in one OpenSpec change.

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
