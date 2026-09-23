# Governed Repair Workflows

## ADDED Requirements

### Requirement: Validated autonomy configuration

Sidecar SHALL validate every configured autonomy value during startup and SHALL
accept only `auto-commit`, `pull-request`, `suggest-only`, or `notify` when a
value is non-empty. Empty values SHALL retain the safe `suggest-only` fallback.

#### Scenario: Invalid configured autonomy

- **WHEN** any autonomy field contains an unknown non-empty value
- **THEN** configuration loading fails with the field path and invalid value
- **AND** no signal processing starts

#### Scenario: Unconfigured autonomy

- **WHEN** the autonomy field for a change type is empty
- **THEN** Sidecar resolves that change type to `suggest-only`

### Requirement: Fail-closed output routing

Sidecar SHALL route every supported autonomy value through an explicit branch
and SHALL never treat an unknown runtime value as `auto-commit`.

#### Scenario: Unknown runtime autonomy

- **WHEN** an unknown autonomy value reaches output routing
- **THEN** Sidecar records `invalid_autonomy` and marks the task failed
- **AND** no commit, branch push, or change request is created
- **AND** temporary Sidecar-owned work is cleaned safely

### Requirement: Strict configuration decoding

Sidecar SHALL reject unknown YAML fields and invalid bounded policy values while
continuing to recognize explicitly reserved compatibility fields.

#### Scenario: Misspelled configuration key

- **WHEN** configuration contains a key not defined by the active schema
- **THEN** startup fails with the key and its source location

#### Scenario: Reserved planning model

- **WHEN** `models.planning` is present
- **THEN** configuration loading succeeds
- **AND** documentation identifies the field as reserved and inactive

### Requirement: Enforced repository scope

Sidecar SHALL enforce `scope.include` and `scope.exclude` against normalized,
repository-relative paths. Empty includes SHALL include all files, excludes
SHALL win, and invalid or escaping patterns SHALL be rejected.

#### Scenario: File tool targets excluded path

- **WHEN** the coding agent asks a write or edit tool to change an excluded path
- **THEN** the tool rejects the operation without modifying that path

#### Scenario: Bash produces out-of-scope diff

- **WHEN** the final diff contains an excluded or non-included tracked,
  untracked, renamed, or deleted path
- **THEN** Sidecar records `scope_violation` and marks the task failed
- **AND** verification, evaluation, commit, and delivery are skipped

#### Scenario: Rename crosses scope boundary

- **WHEN** either the source or destination of a rename is outside scope
- **THEN** the diff fails scope validation

#### Scenario: Symlink escape

- **WHEN** a candidate path resolves outside the task worktree through a
  symlink
- **THEN** Sidecar rejects it as a scope violation

### Requirement: Bounded workflow stage policies

Sidecar SHALL support validated maximum turns and timeouts for triage, coding,
evaluation, and review; retry policy for coding and evaluation; and enabled
flags where defined. Absent settings SHALL preserve documented defaults.

#### Scenario: Default stage policy

- **WHEN** no `workflow` block is configured
- **THEN** triage uses 1 turn, coding 20 turns, evaluator 20 turns, and reviewer
  4 turns
- **AND** existing enabled behavior is preserved

#### Scenario: Invalid stage bound

- **WHEN** a turn, timeout, retry, concurrency, or queue value is invalid or
  exceeds its upstream cap
- **THEN** configuration loading fails before processing signals

#### Scenario: Retryable evaluator failure

- **WHEN** evaluation fails with a retryable provider or transport error
- **AND** retries remain
- **THEN** Sidecar retries with bounded backoff and records the attempt

#### Scenario: Non-retryable workflow failure

- **WHEN** scope, configuration, or deterministic verification fails
- **THEN** Sidecar does not retry a model stage

### Requirement: Host-configurable workflow policy

Sidecar SHALL expose workflow policy through its public Go configuration API as
well as YAML. Both paths SHALL produce the same defaults, normalization,
validation, safety caps, and immutable runtime policy.

#### Scenario: Embedding application configures turns

- **WHEN** a host application constructs a configuration with custom bounded
  triage, coding, evaluator, or reviewer turns
- **THEN** Sidecar applies those values to the corresponding runtimes
- **AND** does not require the host to generate or load a YAML file

#### Scenario: Programmatic configuration is invalid

- **WHEN** host code supplies an invalid autonomy, turn limit, transition, or
  other bounded workflow value
- **THEN** the library entry point rejects it before creating a repair loop

#### Scenario: Defaults are equivalent

- **WHEN** YAML and programmatic configurations omit the same workflow fields
- **THEN** both resolve to the same documented defaults

### Requirement: Bounded multi-cycle repair flow

Sidecar SHALL support a configured number of coding cycles and MAY return a
verification failure or evaluator rejection to coding only when the configured
transition is `retry-coding` and another cycle remains.

#### Scenario: Verification feedback starts another cycle

- **WHEN** deterministic verification fails
- **AND** `on_verification_failure` is `retry-coding`
- **AND** the cycle and token budgets allow another coding attempt
- **THEN** Sidecar supplies bounded failure evidence to coding in the same
  isolated workspace
- **AND** reruns scope and verification gates on the new result

#### Scenario: Evaluator rejection starts another cycle

- **WHEN** evaluation returns `reject`
- **AND** `on_evaluator_reject` is `retry-coding`
- **AND** another cycle remains
- **THEN** Sidecar supplies the rejection reasons to coding for another attempt
- **AND** does not deliver the rejected revision

#### Scenario: Repair cycles exhausted

- **WHEN** no configured coding cycle remains
- **THEN** Sidecar applies the terminal failure or preservation policy
- **AND** creates no approved change request from an unverified or rejected
  revision

#### Scenario: Safety failure cannot loop

- **WHEN** configuration, scope validation, or budget enforcement fails
- **THEN** Sidecar terminates the workflow without returning to coding

### Requirement: Durable interruption recovery

Sidecar SHALL persist bounded workflow checkpoints and SHALL resume an
interrupted task only when explicitly configured, safely leased, and backed by
a matching Sidecar-owned base and checkpoint ref.

#### Scenario: Resume after coding interruption

- **WHEN** a task restarts with `on_interruption: resume`
- **AND** its base and coding checkpoint are valid
- **THEN** Sidecar reconstructs an isolated worktree from that checkpoint
- **AND** reruns scope validation and deterministic verification before any
  evaluation or delivery

#### Scenario: Invalid recovery checkpoint

- **WHEN** a recorded checkpoint is missing, mismatched, outside the configured
  repository, or not Sidecar-owned
- **THEN** Sidecar fails the resume attempt without executing or delivering it

#### Scenario: Resume limit exhausted

- **WHEN** `max_resume_attempts` has been reached
- **THEN** Sidecar applies terminal failure/preservation policy and performs
  configured cleanup

#### Scenario: Concurrent resume claim

- **WHEN** two workers attempt to resume the same task
- **THEN** at most one worker obtains the task lease

### Requirement: Explicit stage error policies

Sidecar SHALL apply only documented error actions for triage, coding, and
evaluation. An error action SHALL NOT convert an error into evaluator approval
or automatic merge.

#### Scenario: Triage error policy

- **WHEN** triage fails after configured retries
- **THEN** Sidecar applies exactly the configured `suggest`, `skip`, or `fail`
  action

#### Scenario: Coding error policy

- **WHEN** coding fails after configured retries
- **THEN** Sidecar applies exactly `fail` or `preserve-suggestion` as configured

### Requirement: Distinct evaluator outcomes

Sidecar SHALL represent evaluator `pass`, `reject`, and `error` separately.
Provider failures, timeouts, parse failures, and exhausted turns SHALL be
errors, not semantic rejections.

#### Scenario: Valid evaluator rejection

- **WHEN** the evaluator returns a valid negative verdict
- **THEN** Sidecar records `evaluation_rejected` and applies `on_reject`

#### Scenario: Evaluator infrastructure error

- **WHEN** evaluation cannot produce a valid verdict
- **THEN** Sidecar records `evaluation_error` with redacted details
- **AND** applies `on_error` without approving the repair

### Requirement: Reuse deterministic verification evidence

Sidecar SHALL provide the evaluator with structured, bounded, redacted results
from deterministic verification when evidence reuse is enabled.

#### Scenario: Verification has succeeded

- **WHEN** evaluation starts with `use_verification_evidence: true`
- **THEN** the evaluator receives each command, directory, exit code, duration,
  truncation state, and bounded output
- **AND** is instructed not to rerun equivalent commands unnecessarily

### Requirement: Auditable change-request evidence

Every Sidecar-created pull or merge request SHALL include bounded, sanitized
repair-agent and evaluator decision summaries and SHALL reference their durable
trace records when available. The evidence SHALL NOT include hidden reasoning,
complete prompts, credentials, raw CI logs, unbounded output, or complete file
contents.

#### Scenario: Evaluator-approved change request

- **WHEN** deterministic verification and evaluation pass
- **THEN** the change request identifies both agent traces, models, attempts,
  turns used/allowed, changed files, command/outcome summaries, verification
  results, evaluator verdict, and reported usage
- **AND** embedded evidence is sanitized and bounded to 32 KiB

#### Scenario: Evaluator exceeds turn limit

- **WHEN** deterministic verification passes but the evaluator reaches its
  configured maximum turns without a verdict
- **AND** `on_error` is `draft-change-request`
- **THEN** Sidecar creates a draft change request with task status
  `needs_review`
- **AND** labels or prominently marks it as `sidecar:evaluation-error`
- **AND** states that evaluator approval was not obtained
- **AND** includes the sanitized evaluator error, turns used/allowed, usage,
  action summary, and trace reference
- **AND** the draft requires human approval and cannot be auto-merged

#### Scenario: Detailed trace exceeds provider evidence limit

- **WHEN** the agent records exceed the 32-KiB embedded evidence limit
- **THEN** Sidecar truncates the embedded summary deterministically
- **AND** records truncation and references the authorized durable trace

#### Scenario: Trace persistence is incomplete

- **WHEN** Sidecar cannot persist all detailed agent records
- **THEN** a draft change request explicitly marks its audit evidence as
  incomplete
- **AND** does not claim full evaluator approval from missing evidence

### Requirement: Policy-controlled repair preservation

Sidecar SHALL apply the configured reject/error preservation action only to
Sidecar-owned branches and SHALL distinguish suggestions, preserved local
branches, and draft change requests in task status and events.

#### Scenario: Preserve rejected repair as suggestion

- **WHEN** evaluation rejects and `on_reject` is `suggest`
- **THEN** Sidecar stores bounded repair and evaluation details
- **AND** creates no branch push or change request

#### Scenario: Preserve branch

- **WHEN** configured policy is `preserve-branch`
- **THEN** Sidecar retains the exact checked repair on a local Sidecar-owned
  branch and records its name

#### Scenario: Draft change request

- **WHEN** configured policy is `draft-change-request`
- **THEN** Sidecar creates or reuses a provider draft request visibly marked as
  failed or rejected evaluation
- **AND** never represents it as approved or ready to merge

#### Scenario: Provider lacks draft support

- **WHEN** draft publication is requested but unsupported
- **THEN** Sidecar preserves the Sidecar-owned branch and records the fallback

### Requirement: Complete model usage accounting

Every model stage SHALL report and persist usage accumulated before either
success or error, including attempt and cache usage when provided. Missing
provider usage SHALL be distinguishable from measured zero usage.

#### Scenario: Evaluator fails after consuming tokens

- **WHEN** the evaluator returns an error after one or more model calls
- **THEN** Sidecar records the partial usage before applying error policy

#### Scenario: Provider omits usage

- **WHEN** a provider response has no usage data
- **THEN** the usage event identifies metering as unavailable
- **AND** does not record the consumption as measured zero

### Requirement: Per-task and daily budget policy

Sidecar SHALL enforce configured per-task and daily token budgets before each
model attempt and SHALL apply an explicit metering-error action.

#### Scenario: Per-task budget exhausted

- **WHEN** recorded task usage reaches the configured per-task limit
- **THEN** Sidecar starts no further model attempt
- **AND** records `budget_exceeded`

#### Scenario: Metering lookup fails

- **WHEN** Sidecar cannot determine budget consumption
- **THEN** it applies exactly the configured `allow`, `skip`, or `fail` action

### Requirement: Status-aware signal deduplication

Signal deduplication SHALL account for task status and retry delay rather than
permanently suppressing every signal key after its first task record.

#### Scenario: Completed duplicate

- **WHEN** a matching signal has a configured terminal task status
- **THEN** Sidecar suppresses it and records `dedupe_suppressed`

#### Scenario: Failed signal after retry delay

- **WHEN** the prior matching task failed and `retry_failed_after` has elapsed
- **THEN** Sidecar may atomically claim and process a new attempt

#### Scenario: Concurrent duplicate

- **WHEN** two workers claim the same signal concurrently
- **THEN** at most one claim becomes active

#### Scenario: Manual retry

- **WHEN** an operator retries a prior task explicitly
- **THEN** Sidecar links a new attempt to the original task and bypasses only
  deduplication
- **AND** budget, scope, verification, and evaluation remain enforced

### Requirement: Configurable bounded verification behavior

Sidecar SHALL support validated verification output limits and failure strategy
without permitting values above upstream safety caps.

#### Scenario: Configured output limit

- **WHEN** command output exceeds the configured limit
- **THEN** Sidecar truncates it, records truncation, and never exceeds the hard
  upstream cap

#### Scenario: Stop-on-failure disabled

- **WHEN** `stop_on_failure` is false
- **THEN** Sidecar runs remaining commands for evidence collection
- **AND** the overall verification result remains failed

### Requirement: Bounded execution and side-effect delivery

Sidecar SHALL support validated task concurrency, queue capacity, delivery
retry, notification retry, and timeout policies. Retries SHALL preserve
idempotency and SHALL NOT duplicate change requests.

#### Scenario: Queue is full

- **WHEN** the configured queue has no capacity
- **THEN** Sidecar rejects or defers the signal with an observable event
- **AND** does not silently drop it

#### Scenario: Retryable delivery failure

- **WHEN** branch or change-request delivery fails transiently
- **THEN** Sidecar retries within configured bounds
- **AND** reuses an existing branch or request if the prior attempt succeeded

#### Scenario: Required notification fails

- **WHEN** a required notification exhausts its retries
- **THEN** Sidecar records the notification failure and applies its configured
  failure policy
- **AND** does not undo or misreport an already completed external delivery

### Requirement: Configurable repository and adapter limits

Sidecar SHALL expose validated branch-prefix, memory-retrieval, and GitLab CI
enrichment limits while retaining safe defaults and hard caps.

#### Scenario: Custom branch prefix

- **WHEN** a valid branch prefix is configured
- **THEN** new Sidecar-owned branches use it
- **AND** cleanup remains restricted to branches with the validated prefix

#### Scenario: Memory retrieval controls

- **WHEN** memory top-k or similarity threshold is configured
- **THEN** retrieval uses those values within upstream bounds

#### Scenario: GitLab enrichment controls

- **WHEN** log, diff, history, pagination, or API timeout limits are configured
- **THEN** GitLab enrichment applies those limits without exceeding hard caps
