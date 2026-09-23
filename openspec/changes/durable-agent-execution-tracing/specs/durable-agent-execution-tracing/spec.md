# Durable Agent Execution Tracing

## ADDED Requirements

### Requirement: Durable ordered agent traces

Sidecar SHALL persist ordered, task-linked traces for coding, evaluator, and
reviewer Harness runtime invocations in a dedicated trace table rather than
storing detailed output in general task events.

#### Scenario: Coding agent uses tools

- **WHEN** Harness emits tool-call and tool-result events for a coding run
- **THEN** Sidecar stores ordered trace records linked to the task, trace ID,
  role, attempt, sequence, and tool-call ID

#### Scenario: Concurrent tool results

- **WHEN** tool results complete in a different order from tool calls
- **THEN** each result remains correlated by tool-call ID
- **AND** the Sidecar-assigned sequence preserves observed event order

### Requirement: Configurable bounded trace capture

Sidecar SHALL support validated trace enablement, assistant-text mode,
tool-argument mode, tool-output mode, output limit, retention period, and INFO
activity logging through YAML and the public Go configuration API.

#### Scenario: Configuration omitted

- **WHEN** no agent-trace configuration is provided
- **THEN** tracing uses conservative documented defaults with a 16-KiB output
  limit and 30-day retention

#### Scenario: Invalid trace bound

- **WHEN** output or retention configuration is invalid or exceeds the hard
  upstream cap
- **THEN** configuration loading fails before agents start

#### Scenario: Detailed tracing disabled

- **WHEN** `agent_traces.enabled` is false
- **THEN** detailed assistant/tool trace rows are not stored
- **AND** request usage, terminal task events, and evaluator outcome
  classification remain active

### Requirement: No hidden reasoning capture

Sidecar SHALL NOT persist hidden chain-of-thought, thinking blocks, complete
system/user prompts, provider reasoning fields, or full agent message history.

#### Scenario: Provider emits a thinking block

- **WHEN** a provider or Harness session contains hidden reasoning content
- **THEN** the trace collector ignores that content
- **AND** no trace payload or normal log contains it

#### Scenario: Assistant text capture enabled

- **WHEN** visible `EventTextDelta` capture is configured
- **THEN** Sidecar stores only bounded/redacted visible assistant text according
  to `none`, `final-only`, or `all-visible` mode

### Requirement: Sanitized tool activity

Sidecar SHALL sanitize tool arguments, results, metadata, and errors before
persistence or logging and SHALL record safe tool duration and outcome fields.

#### Scenario: Bash command completes

- **WHEN** a Bash tool call has matching ready and result events
- **THEN** Sidecar records the tool name, command hash, bounded redacted preview,
  duration, exit code when reported, byte counts, and truncation state

#### Scenario: File tool accesses a path

- **WHEN** a read, write, or edit tool operates on a workspace file
- **THEN** Sidecar records operation and normalized repository-relative path
- **AND** does not store the complete file body by default

#### Scenario: Tool arguments are malformed

- **WHEN** tool arguments cannot be parsed safely as JSON
- **THEN** Sidecar stores only a hash, byte length, and parse-error marker

### Requirement: Credential and sensitive-data redaction

Sidecar SHALL recursively redact credential keys, known configured secret
values, authorization/header forms, URL user-info, private keys, DSNs, and
environment values before trace persistence and structured logging.

#### Scenario: Tool output contains credentials

- **WHEN** command output contains a configured token, database URL, or
  authorization header
- **THEN** the persisted bounded output and INFO log contain a redaction marker
  instead of the secret

#### Scenario: Binary or image result

- **WHEN** a tool result includes binary or image content
- **THEN** Sidecar stores only safe type/count/size metadata

### Requirement: Per-request usage on all terminal paths

Sidecar SHALL persist every `EventRequestUsage` record by request ID for
generation, retry, and compaction attempts, including attempts followed by
agent error or abort.

#### Scenario: Evaluator exceeds maximum turns

- **WHEN** the evaluator consumes reported tokens and then emits a maximum-turn
  error
- **THEN** every preceding request-usage record remains durable
- **AND** budget accounting includes each reported request exactly once

#### Scenario: Usage unavailable

- **WHEN** Harness emits request usage with nil token usage
- **THEN** Sidecar records the source as unavailable
- **AND** does not treat or display the attempt as measured zero

#### Scenario: Terminal aggregate follows request usage

- **WHEN** `EventDone` includes cumulative usage after request-level records
- **THEN** Sidecar may store the aggregate for diagnostics
- **AND** does not add it again to budget consumption

### Requirement: Metering persistence safety

Sidecar SHALL not continue initiating model requests after durable
request-usage persistence fails, and SHALL continue draining the active Harness
event stream without copying unsanitized events to logs.

#### Scenario: Usage insert fails

- **WHEN** persistence of a request-usage record fails
- **THEN** Sidecar cancels further model work and drains the active stream
- **AND** records a metering failure without reporting zero consumption

### Requirement: Explicit evaluator outcomes

Sidecar SHALL distinguish evaluator pass, semantic rejection, and runtime error
in traces, task events, status reasons, and logs.

#### Scenario: Evaluator returns negative verdict

- **WHEN** a valid evaluator response has `pass: false`
- **THEN** Sidecar records `evaluation_rejected` with its reasons

#### Scenario: Evaluator runtime fails

- **WHEN** Harness emits an error, abort, exhausted-turn error, provider error,
  or the response cannot be parsed
- **THEN** Sidecar records `evaluation_error`
- **AND** never describes the outcome as evaluator rejection

### Requirement: Compaction and terminal tracing

Sidecar SHALL trace compaction lifecycle and terminal completion, abort, and
error events without storing compaction summary text or hidden reasoning.

#### Scenario: Context compaction occurs

- **WHEN** Harness emits compaction events
- **THEN** Sidecar records reason, status, duration, turns compacted, token
  counts, and request usage when available
- **AND** omits the generated summary text

#### Scenario: Agent terminates with error

- **WHEN** a coding, evaluator, or reviewer runtime emits `EventError`
- **THEN** the terminal trace contains a sanitized error category/message and
  the last sequence number

### Requirement: Trace retention and retrieval

Sidecar SHALL purge expired traces in bounded batches and SHALL provide bounded,
ordered retrieval by task, trace, and role under the task-access authorization
boundary.

#### Scenario: Trace exceeds retention period

- **WHEN** a trace row is older than configured retention
- **THEN** the retention job deletes it in a bounded batch
- **AND** leaves task milestone and budget records intact

#### Scenario: Operator views a trace

- **WHEN** an authorized operator requests agent activity for a task
- **THEN** events are returned in sequence order with bounded pagination
- **AND** no unredacted payload is available

### Requirement: Trace persistence observability

Trace persistence failures SHALL be observable without blocking event-channel
drain or falling back to raw event logging.

#### Scenario: Non-usage trace insert fails

- **WHEN** persistence of a non-critical sanitized trace batch fails
- **THEN** Sidecar continues draining the Harness stream
- **AND** records `trace_persistence_failed` through the bounded task audit path
- **AND** does not log raw arguments or output
