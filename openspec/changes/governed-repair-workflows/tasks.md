# Tasks

Status convention:

- `[x]` is implemented and verified in the current repository.
- `[ ]` is not complete. A `Partial:` note records work already present without
  claiming the full requirement is done.
- `Implemented by:` records the change group and commit that delivered work
  before this Governed Repair Workflows change is implemented.

## Phase 1: Close autonomy and scope safety gaps

- [x] Validate every non-empty autonomy field during configuration loading.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [x] Replace default auto-commit routing with explicit autonomy cases and a
      fail-closed unknown-value branch.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [x] Add runtime `invalid_autonomy` status/event/notification handling.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [ ] Define and validate normalized include/exclude pattern semantics.
- [ ] Enforce scope in write and edit tools.
- [ ] Collect exact changed paths from the worktree base, including staged,
      unstaged, committed, renamed, deleted, and untracked files.
- [ ] Enforce scope after coding and before verification, evaluation, or output.
- [ ] Add symlink and rename-boundary protections.
- [ ] Add tests proving invalid autonomy cannot obtain code-shipping output and
      out-of-scope changes cannot be delivered.
      Partial: configuration validation has unit coverage, but runtime
      invalid-autonomy and scope-delivery integration coverage is missing.

## Phase 2: Strict and configurable workflow policy

- [ ] Enable strict YAML known-field decoding with useful path/line errors.
- [ ] Document `models.planning` as a reserved inactive compatibility field.
- [ ] Add workflow configuration structs, defaults, validation, and safety caps
      for triage, coding, evaluator, reviewer, execution, and deduplication.
      Partial: evaluator `max_turns` and `on_error` are configurable and capped;
      the other stages and execution/deduplication controls are not.
- [ ] Expose defaults, normalization, and validation through the public Go API
      so embedding applications do not need to generate YAML.
      Partial: evaluator and agent-trace defaults are exposed through `Config`;
      the complete governed workflow policy is not.
- [ ] Validate programmatically supplied configuration at every library entry
      point that creates a repair loop.
      Partial: `config.Load` validates YAML configuration, but `loop.New` still
      accepts a programmatically constructed configuration without returning a
      validation error.
- [ ] Wire stage turn limits and timeouts into each runtime.
      Partial: evaluator `max_turns` is wired into its Harness runtime; triage,
      coding, reviewer, and stage timeouts remain fixed or implicit.
- [ ] Implement bounded retries and backoff for retryable coding/evaluator
      provider and transport failures.
- [ ] Implement explicit triage and coding `on_error` actions.
- [ ] Add configuration parsing, default, invalid-bound, timeout, retry, and
      non-retryable-failure tests.
      Partial: evaluator and trace defaults/invalid bounds are covered; stage
      timeout and retry policy tests are not.

## Phase 3: Evaluation, preservation, usage, and budgets

- [x] Introduce distinct evaluator pass/reject/error outcomes.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [ ] Pass structured bounded/redacted deterministic verification evidence to
      the evaluator and avoid redundant command reruns.
      Partial: the evaluator receives the configured command contract and
      resolved workspace, but not the prior structured command results, so it
      may still rerun tests.
- [ ] Implement evaluator `on_reject` and `on_error` independently.
      Partial: `on_error` supports `suggest`, `fail`, and
      `draft-change-request`; rejection still follows a fixed suggestion path.
- [ ] Add bounded coding-cycle transitions for verification failure and
      evaluator rejection, with prior evidence passed back to coding.
- [ ] Persist stage checkpoints and Sidecar-owned local checkpoint refs.
- [ ] Add task leases and bounded restart recovery from valid checkpoints.
- [ ] Rerun scope and deterministic verification after every repair cycle and
      resumed checkpoint before evaluation or delivery.
- [ ] Implement suggestion, local Sidecar-owned branch, and draft
      change-request preservation paths.
      Partial: suggestion and draft change-request paths exist; a configured
      local branch-preservation path does not.
- [ ] Add provider draft delivery with branch-preservation fallback.
      Partial: GitHub draft PRs and GitLab draft MRs are implemented, including
      the evaluation-error label; unsupported/failed draft delivery does not
      fall back to preserving a local Sidecar branch.
- [x] Build bounded sanitized repair-agent and evaluator evidence sections for
      every generated pull or merge request.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`), with the
      provider delivery foundation from `provider-aware-change-delivery`
      (`2916bcc`).
- [ ] Include trace IDs/authorized trace links, turns, command summaries,
      verification evidence, outcomes, and usage without copying full traces.
      Partial: generated descriptions include trace IDs, request counts, token
      usage, verification outcomes, evaluator outcome, and repeated-tool
      summaries. Authorized trace URLs, exact changed files, model identity,
      and full command-duration summaries are incomplete.
- [x] Mark evaluator-error delivery as `needs_review`, draft-only, human-gated,
      and `sidecar:evaluation-error` where provider labels are supported.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`), extending
      `provider-aware-change-delivery` (`2916bcc`).
- [x] Enforce the 32-KiB embedded evidence limit with deterministic truncation
      and incomplete-trace disclosure.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [ ] Return and persist partial model usage on every stage error and attempt.
      Partial: coding and evaluator request-level usage is persisted on Harness
      error paths; triage/reviewer and every pre-runtime failure path are not
      covered by the same ledger contract.
- [x] Represent unavailable metering separately from zero measured usage.
      Implemented by: `durable-agent-execution-tracing` (`2f4153d`).
- [ ] Add per-task budget, timezone, and metering-error policy.
- [ ] Check task and daily budgets before every model attempt.
      Partial: the workspace daily budget is checked before triage, and durable
      coding/evaluator request usage feeds its total; admission is not checked
      before every provider attempt and there is no per-task budget.
- [ ] Add tests that evaluator errors never approve, normal-deliver, or merge a
      repair and that partial usage remains accounted.
      Partial: collector and provider draft-payload tests exist, but an
      evaluator-error loop integration test proving the complete routing and
      accounting contract is missing.
- [ ] Add change-request rendering tests for approved, rejected, evaluator-error,
      truncated, redacted, and incomplete-trace evidence.
      Partial: GitHub/GitLab draft and label request payloads are tested; the
      generated evidence body variants are not.
- [ ] Add tests for host-supplied configuration, default equivalence, repair
      cycle exhaustion, feedback propagation, interruption recovery, invalid
      checkpoints, resume limits, and concurrent resume claims.

## Phase 4: Retry and operational controls

- [ ] Replace existence-only signal dedupe with atomic status-aware claims and
      failed-task retry delay.
- [ ] Add an operator manual-retry command that preserves all safety gates.
- [ ] Add configurable verification output limit and stop-on-failure behavior.
      Partial: verification output is bounded and execution currently stops on
      first failure, but neither behavior is configurable.
- [ ] Add validated task concurrency and queue-size controls with observable
      queue-full handling.
- [ ] Add bounded idempotent delivery timeout/retry/backoff controls.
      Partial: provider delivery detects/reuses an existing open change request
      and records delivery phases, but configurable timeout/retry/backoff is
      absent.
- [ ] Add notification timeout/retry and required/optional policy.
- [ ] Add validated Sidecar branch-prefix configuration and cleanup safeguards.
      Partial: generated worktrees use Sidecar-owned task branches and cleanup
      is scoped to those runtime artifacts, but the prefix is not configurable.
- [ ] Add memory top-k and similarity-threshold configuration.
- [ ] Add bounded GitLab CI log, diff, history, pagination, and API timeout
      controls.
      Partial: GitLab enrichment uses bounded/best-effort data collection, but
      the complete limits, pagination, history, and timeout policy are not
      configurable under this workflow contract.
- [ ] Add integration tests for dedupe races, manual retry, queue saturation,
      delivery idempotency, notification failure, and branch cleanup.

## Documentation and verification

- [ ] Update `sidecar.yaml` and README with defaults, caps, status transitions,
      preservation risks, and migration guidance for strict YAML decoding.
      Partial: evaluator policy, agent tracing, draft-error behavior, and caps
      are documented. Full workflow defaults/status transitions and strict-YAML
      migration guidance remain outstanding.
- [ ] Add an upgrade note identifying previously ignored unknown keys and
      invalid autonomy values as startup errors.
- [x] Run `openspec validate governed-repair-workflows --strict`.
      Completed in the current uncommitted Governed Repair Workflows OpenSpec
      authoring work; no implementation commit yet.
- [ ] Run `go test ./...`, `go vet ./...`, and `go build ./...` after each phase.
      Partial: the current implemented slice passes all three commands, but the
      remaining phases have not been implemented or verified.
