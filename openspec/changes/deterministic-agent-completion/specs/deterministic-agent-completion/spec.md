# Deterministic Agent Completion

## ADDED Requirements

### Requirement: Observable coding completion

Sidecar SHALL classify coding as complete only after normal runtime termination
with either a relevant workspace change and bounded final summary or an
explicit no-change result with an explanation.

#### Scenario: Coding produces a repair

- **WHEN** the coding runtime terminates normally with a relevant diff and
  final summary
- **THEN** Sidecar records coding completion
- **AND** applies scope and deterministic verification before evaluation or
  delivery

#### Scenario: Coding exhausts its turns

- **WHEN** coding reaches its hard turn limit without a completion result
- **THEN** Sidecar records `coding_incomplete`
- **AND** no partial code reaches normal delivery

#### Scenario: Coding finds no required change

- **WHEN** coding terminates normally with an explicit no-change explanation
- **THEN** Sidecar records a completed no-change outcome
- **AND** does not manufacture an empty repair

### Requirement: Structured evaluator evidence

Sidecar SHALL provide the evaluator with the candidate diff and structured,
bounded, redacted deterministic-verification evidence.

#### Scenario: Verification passed before evaluation

- **WHEN** deterministic verification succeeds
- **THEN** the evaluator receives command identity, working directory, exit
  code, duration, truncation state, and bounded output
- **AND** receives an instruction not to rerun equivalent checks without a
  concrete unresolved risk

#### Scenario: Evidence contains sensitive output

- **WHEN** verification output contains a configured secret or exceeds the
  evidence limit
- **THEN** Sidecar redacts and truncates it before evaluation
- **AND** records the truncation state

### Requirement: Material-risk investigation boundary

Sidecar SHALL bound evaluator investigation separately from the hard runtime
turn limit and SHALL reserve at least one request for verdict finalization.

#### Scenario: Material checks are complete

- **GIVEN** deterministic verification passed
- **AND** the evaluator completed all material-risk checks
- **WHEN** no unresolved failure evidence remains
- **THEN** Sidecar moves evaluation to finalization
- **AND** the evaluator stops invoking investigative tools

#### Scenario: Investigation boundary reached

- **WHEN** the evaluator reaches the investigation boundary without a verdict
- **THEN** Sidecar disables investigative tools
- **AND** preserves the reserved finalization capacity
- **AND** requests the structured verdict immediately

#### Scenario: Hard limit remains a safety boundary

- **WHEN** evaluator completion enforcement fails to produce a terminal result
- **THEN** Harness still stops execution at the hard turn limit
- **AND** Sidecar records `evaluator_incomplete`, never `pass`

### Requirement: Immediate structured verdict

The evaluator SHALL emit a bounded structured verdict as soon as sufficient
evidence exists, and Sidecar SHALL terminate evaluation after accepting it.

#### Scenario: Evaluator finds no material problem

- **WHEN** required verification and material-risk checks pass
- **THEN** the evaluator emits a `pass` verdict immediately
- **AND** performs no subsequent tool calls

#### Scenario: Evaluator finds a material failure

- **WHEN** evidence proves the candidate does not satisfy the task or safety
  requirements
- **THEN** the evaluator emits a `reject` verdict with bounded reasons
- **AND** performs no subsequent tool calls

#### Scenario: Verdict is malformed

- **WHEN** finalization returns malformed, missing, or contradictory verdict
  data
- **THEN** Sidecar records `evaluator_incomplete`
- **AND** does not infer either approval or semantic rejection

### Requirement: Redundant tool-call control

Sidecar SHALL detect equivalent evaluator tool calls using the tool name and
sanitized argument hash and SHALL prevent repetition from consuming all
available requests.

#### Scenario: Evaluator repeats a completed test command

- **WHEN** the evaluator requests a command equivalent to successful
  deterministic verification without identifying a new material risk
- **THEN** Sidecar does not execute the command
- **AND** returns the existing evidence to the evaluator

#### Scenario: Evaluator repeats the same investigation

- **WHEN** the evaluator repeats a previously completed tool call beyond the
  allowed internal threshold
- **THEN** Sidecar records the repetition
- **AND** moves evaluation to finalization

#### Scenario: Different targeted check is justified

- **WHEN** the evaluator identifies a concrete uncovered risk and requests a
  materially different bounded check
- **THEN** Sidecar may execute it within the investigation boundary

### Requirement: Distinct terminal outcomes

Sidecar SHALL represent `pass`, `reject`, `incomplete`, and `error` as distinct
terminal outcomes.

#### Scenario: Valid negative verdict

- **WHEN** the evaluator emits a valid negative verdict
- **THEN** Sidecar records `evaluation_rejected` with its reasons

#### Scenario: No verdict before completion boundary

- **WHEN** the evaluator cannot emit a valid verdict within its completion
  contract
- **THEN** Sidecar records `evaluation_incomplete`
- **AND** does not describe the result as rejection

#### Scenario: Runtime failure

- **WHEN** evaluation ends because of a provider, runtime, persistence, or
  cancellation failure
- **THEN** Sidecar records `evaluation_error`
- **AND** does not describe the result as rejection or approval

### Requirement: Safe incomplete-work preservation

Sidecar SHALL preserve useful incomplete work only through an explicit
non-approved human-handoff path.

#### Scenario: Coding is incomplete with useful findings

- **WHEN** coding is incomplete but has a bounded useful diagnosis
- **THEN** Sidecar records a suggestion containing that diagnosis and remaining
  work
- **AND** does not deliver partial code normally

#### Scenario: Verified repair lacks evaluator verdict

- **WHEN** deterministic verification passed but evaluation is incomplete
- **AND** existing preservation policy selects `draft-change-request`
- **THEN** Sidecar creates a human-gated draft marked `needs_review`
- **AND** prominently states that evaluator approval was not obtained

#### Scenario: Incomplete run has no useful output

- **WHEN** an incomplete stage has no bounded useful diagnosis or valid repair
- **THEN** Sidecar marks the task failed
- **AND** does not create an empty suggestion

### Requirement: Human-readable incomplete handoff

Sidecar SHALL provide a bounded, sanitized handoff that tells a human what was
completed, what remains, why the stage stopped, and what action is recommended.

#### Scenario: Suggestion notification

- **WHEN** incomplete work is preserved as a suggestion
- **THEN** the configured suggested notification contains a bounded summary and
  task identifier
- **AND** excludes hidden reasoning, complete prompts, credentials, raw logs,
  and file bodies

#### Scenario: Operator inspects a suggestion

- **WHEN** an operator requests task details by task identifier
- **THEN** Sidecar displays the sanitized handoff, outcome, verification
  summary, and trace reference

### Requirement: Completion safety invariants

Sidecar SHALL enforce completion safety independently of model instructions.

#### Scenario: Model ignores stop instruction

- **WHEN** a model continues requesting tools after finalization starts
- **THEN** Sidecar rejects those requests without execution
- **AND** does not consume reserved verdict capacity for tool work

#### Scenario: Incomplete evaluator has passing tests

- **WHEN** deterministic tests pass but no valid evaluator verdict exists
- **THEN** Sidecar does not convert test success into evaluator approval

#### Scenario: Verification fails

- **WHEN** deterministic verification fails
- **THEN** no completion or preservation policy may deliver the repair as an
  approved change
