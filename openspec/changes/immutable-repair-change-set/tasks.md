# Tasks

## Configuration and exclusions

- [x] Add `output.exclude` configuration with documented worktree-relative
      glob semantics.
- [x] Add safe built-in exclusions for Harness and Sidecar runtime artifacts.
- [x] Reject empty, malformed, absolute, traversing, and built-in-negating
      exclusion patterns.
- [x] Document how repository `.gitignore`, built-in exclusions, and configured
      exclusions interact.

## Repair change-set preparation

- [x] Track the immutable task base commit when creating the worktree.
- [x] Detect agent-created commits and normalize the temporary branch with a
      mixed reset to the task base.
- [x] Apply exclusions to tracked modifications and untracked files.
- [x] Stage eligible repair files exactly once after coding completes.
- [x] Build a sorted manifest containing paths and Git change types.
- [x] Compute a stable digest of the canonical staged patch.
- [x] Preserve the existing no-change behavior when filtering leaves an empty
      change set.

## Verification and evaluation

- [x] Run deterministic verification without mutating the prepared index.
- [x] Provide the evaluator with the approved manifest and
      `git diff --cached <task-base>`.
- [x] Ensure evaluator-created files remain unstaged and cannot modify the
      approved snapshot.
- [x] Detect unexpected staged-index mutation before final commit.

## Commit and publication integrity

- [x] Remove final `git add -A` and equivalent restaging from evaluation and
      commit handling.
- [x] Commit only the already-prepared index while preserving the attached
      repository or deployment runtime's configured Git identity and signing
      policy.
- [x] Recompute the manifest and canonical digest from `task-base..HEAD` before
      push.
- [x] Fail the task, notify failure, and skip publication when the committed
      change differs from the approved snapshot.
- [x] Allow provider-aware publication only after integrity verification.

## Audit trail

- [x] Record `repair_change_set_prepared` with base, manifest, exclusions, and
      digest.
- [x] Record `repair_change_set_verified` before publication.
- [x] Record bounded and redacted `repair_change_set_failed` diagnostics for
      each lifecycle phase.
- [x] Avoid persisting full source patches, generated output, or credentials in
      task events.

## Tests and documentation

- [x] Test built-in and configured exclusions for tracked and untracked files.
- [x] Test invalid patterns and attempts to negate built-in exclusions.
- [x] Test normalization of one and multiple agent-created commits.
- [x] Test invalid or missing task-base failure without source-repository
      mutation.
- [x] Test stable manifest ordering and canonical digest generation.
- [x] Test no-change behavior after filtering.
- [x] Test verification and evaluator artifacts remain outside the commit.
- [x] Test evaluator inspection uses the staged diff.
- [x] Test index mutation after approval fails closed.
- [x] Test agent self-commit cannot bypass Sidecar filtering and that the final
      commit preserves the configured identity and signing policy.
- [x] Test pre-push manifest and digest mismatch blocks publication and emits a
      failed notification.
- [x] Test the matching approved commit proceeds to provider-aware delivery.
- [x] Document configuration, defaults, lifecycle events, and security
      guarantees in the README.
- [x] Run `openspec validate immutable-repair-change-set --strict`,
      `go test ./...`, `go vet ./...`, and `go build ./...`.
