# Tasks

## Configuration and exclusions

- [ ] Add `output.exclude` configuration with documented worktree-relative
      glob semantics.
- [ ] Add safe built-in exclusions for Harness and Sidecar runtime artifacts.
- [ ] Reject empty, malformed, absolute, traversing, and built-in-negating
      exclusion patterns.
- [ ] Document how repository `.gitignore`, built-in exclusions, and configured
      exclusions interact.

## Repair change-set preparation

- [ ] Track the immutable task base commit when creating the worktree.
- [ ] Detect agent-created commits and normalize the temporary branch with a
      mixed reset to the task base.
- [ ] Apply exclusions to tracked modifications and untracked files.
- [ ] Stage eligible repair files exactly once after coding completes.
- [ ] Build a sorted manifest containing paths and Git change types.
- [ ] Compute a stable digest of the canonical staged patch.
- [ ] Preserve the existing no-change behavior when filtering leaves an empty
      change set.

## Verification and evaluation

- [ ] Run deterministic verification without mutating the prepared index.
- [ ] Provide the evaluator with the approved manifest and
      `git diff --cached <task-base>`.
- [ ] Ensure evaluator-created files remain unstaged and cannot modify the
      approved snapshot.
- [ ] Detect unexpected staged-index mutation before final commit.

## Commit and publication integrity

- [ ] Remove final `git add -A` and equivalent restaging from evaluation and
      commit handling.
- [ ] Commit only the already-prepared index with Sidecar-controlled metadata
      and signing.
- [ ] Recompute the manifest and canonical digest from `task-base..HEAD` before
      push.
- [ ] Fail the task, notify failure, and skip publication when the committed
      change differs from the approved snapshot.
- [ ] Allow provider-aware publication only after integrity verification.

## Audit trail

- [ ] Record `repair_change_set_prepared` with base, manifest, exclusions, and
      digest.
- [ ] Record `repair_change_set_verified` before publication.
- [ ] Record bounded and redacted `repair_change_set_failed` diagnostics for
      each lifecycle phase.
- [ ] Avoid persisting full source patches, generated output, or credentials in
      task events.

## Tests and documentation

- [ ] Test built-in and configured exclusions for tracked and untracked files.
- [ ] Test invalid patterns and attempts to negate built-in exclusions.
- [ ] Test normalization of one and multiple agent-created commits.
- [ ] Test invalid or missing task-base failure without source-repository
      mutation.
- [ ] Test stable manifest ordering and canonical digest generation.
- [ ] Test no-change behavior after filtering.
- [ ] Test verification and evaluator artifacts remain outside the commit.
- [ ] Test evaluator inspection uses the staged diff.
- [ ] Test index mutation after approval fails closed.
- [ ] Test agent self-commit cannot bypass Sidecar filtering or signing.
- [ ] Test pre-push manifest and digest mismatch blocks publication and emits a
      failed notification.
- [ ] Test the matching approved commit proceeds to provider-aware delivery.
- [ ] Document configuration, defaults, lifecycle events, and security
      guarantees in the README.
- [ ] Run `openspec validate immutable-repair-change-set --strict`,
      `go test ./...`, `go vet ./...`, and `go build ./...`.
