# Proposal: Immutable Repair Change Set

## Problem

Sidecar currently uses `git add -A` while preparing evaluation and again while
committing a repair. The repair agent, Harness, verification tools, and the
evaluator all share the same worktree, so generated files such as
`.harness/spill/**`, `.coverage`, test caches, and reports can be staged beside
the intended source change. An agent-created commit can also bypass Sidecar's
filtering and signing boundary.

Repository `.gitignore` rules reduce the risk for an individual project but do
not provide a reliable upstream guarantee. More importantly, restaging after
evaluation means the delivered diff may differ from the diff the evaluator
approved.

## Intended outcome

Sidecar SHALL prepare one explicit repair change set after coding completes and
before verification or evaluation. It SHALL normalize agent-created commits,
apply safe built-in and repository-configured exclusions, stage the intended
repair once, and record an approved path manifest and staged-diff digest.

Verification and evaluation SHALL operate on that prepared change set. Final
delivery SHALL commit the existing index without restaging and SHALL verify
that the committed paths and diff match the approved manifest and digest before
pushing or creating a change request.

This gives Sidecar a hard guarantee that runtime artifacts cannot enter a
repair unnoticed and that the evaluated repair is the repair delivered. Git
identity and signing remain deployment policy: Sidecar uses the Git
configuration supplied by the attached repository or runtime rather than
hardcoding a provider-specific identity or disabling signing.

## Scope

This change introduces repair change-set preparation, safe output exclusions,
normalization of agent-created commits, staged-diff evaluation, manifest and
digest audit events, commit-time integrity checks, and fail-closed publication.

It does not replace project `.gitignore` files, classify arbitrary generated
files automatically, move Harness spill storage, or change provider-specific
pull-request and merge-request APIs.
