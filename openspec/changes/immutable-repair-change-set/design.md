# Design: Immutable Repair Change Set

## Change-set boundary

After the coding agent finishes, Sidecar becomes the sole authority for the
repair commit. It resolves the task base commit, normalizes any commits created
inside the task worktree, filters generated output, and stages the intended
repair exactly once.

```text
coding agent completes
  -> normalize HEAD to the task base with a mixed reset
  -> apply built-in and configured exclusions
  -> stage the intended repair once
  -> record approved paths and staged-diff digest
  -> run deterministic verification
  -> evaluate the staged diff
  -> commit the existing index without restaging, using deployment Git policy
  -> verify base..HEAD against the approved snapshot
  -> sign, push, and create the change request
```

The prepared index is the authoritative change set. Worktree files produced
after preparation are never included unless a future repair cycle explicitly
prepares and evaluates a new snapshot.

## Agent-created commits

The agent may invoke Git through Bash. Before staging, Sidecar runs the
equivalent of `git reset --mixed <task-base>` within the isolated worktree.
This preserves file changes while removing agent-created commits from the
delivery history. Sidecar then owns filtering and the commit-content boundary.
The attached repository or deployment runtime owns final commit identity and
signing through its normal Git configuration.

Normalization fails closed if the task base is missing, is not an ancestor of
the prepared worktree state, or cannot be resolved safely. Sidecar never
rewrites the attached source repository; this operation is restricted to the
task's isolated worktree and temporary branch.

## Output exclusions

Configuration extends the existing output section:

```yaml
output:
  exclude:
    - ".harness/**"
    - ".coverage"
    - ".coverage.*"
    - ".pytest_cache/**"
    - "htmlcov/**"
    - "coverage.xml"
```

Sidecar provides built-in exclusions for Sidecar- and Harness-owned runtime
artifacts, including `.harness/**`. Deployments may add project-specific
patterns. Patterns are worktree-relative, use documented glob semantics, and
cannot negate built-in exclusions. Absolute paths, traversal, empty patterns,
and malformed patterns are rejected during configuration validation.

Exclusions apply to untracked files and tracked-file modifications. If a
configured exclusion would hide a tracked source change, Sidecar records the
excluded path so the omission is visible in the audit trail.

## Approved snapshot

After normalization and filtering, Sidecar stages eligible changes once. It
derives:

- a sorted manifest of changed paths and their change types;
- a digest of the canonical staged patch relative to the task base; and
- the task base commit.

The manifest and digest are immutable inputs to subsequent verification,
evaluation, and publication. Empty prepared change sets follow the existing
no-change outcome and are not published.

The evaluator receives and inspects `git diff --cached <task-base>`. Its tools
may read the worktree, but files it creates or modifies after preparation do
not alter the approved index. Deterministic verification may run against the
working tree while the staged snapshot remains authoritative.

## Commit and publication integrity

After approval, Sidecar invokes `git commit` against the existing index. It
does not run `git add`, `git add -A`, an equivalent restaging operation, or
command-level Git configuration that overrides identity or signing. The
deployment's Git configuration determines the committer identity and whether
the commit is signed.
Before any push, Sidecar recomputes the canonical patch and path manifest from
`<task-base>..HEAD` and compares both with the approved snapshot.

A mismatch is an integrity failure. Sidecar records the differing paths,
marks the task failed, skips push and change-request creation, emits a failed
notification, and cleans up according to the task worktree policy. Sensitive
patch contents are not persisted in events.

## Audit events

`repair_change_set_prepared` records the base commit, sorted path manifest,
change types, exclusion summary, and staged-diff digest. It does not contain
full source patches.

`repair_change_set_verified` records the matching commit and digest before
publication. `repair_change_set_failed` records the phase and bounded mismatch
metadata for normalization, staging, commit, or pre-push integrity failures.

## Compatibility and boundaries

Existing configurations without `output.exclude` receive the safe built-in
exclusions. Provider-aware delivery continues to own remote push and pull- or
merge-request creation; it may only run after change-set integrity succeeds.
Workspace verification continues to own test execution, while this change
defines exactly which repair those tests and the evaluator authorize for
delivery.
