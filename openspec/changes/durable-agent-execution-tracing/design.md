# Design: Durable Agent Execution Tracing

## Responsibility boundary

Harness is the agent engine. It calls the model, executes the multi-turn loop,
dispatches tools, enforces `MaxTurns`, tracks sessions and request usage, and
emits runtime events. Sidecar owns task identity, agent role, database storage,
redaction, retention, budget accounting, evaluator classification, and output
policy.

The implementation consumes the Harness v0.4.2 event contract already used by
Sidecar:

- `EventToolCallReady`
- `EventToolResult`
- `EventRequestUsage`
- `EventTextDelta`
- `EventCompactionStart`, `EventCompactionDone`, and
  `EventCompactionSkipped`
- `EventDone`, `EventAborted`, and `EventError`

Harness thinking blocks are not exposed as `AgentEvent` text and SHALL NOT be
recovered from sessions or provider payloads.

## Storage model

Large runtime traces do not belong in the general-purpose `task_events` table.
Add an idempotent migration:

```sql
CREATE TABLE IF NOT EXISTS agent_trace_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    trace_id     UUID NOT NULL,
    role         TEXT NOT NULL,
    attempt      INTEGER NOT NULL DEFAULT 1,
    sequence     BIGINT NOT NULL,
    event_type   TEXT NOT NULL,
    tool_call_id TEXT,
    tool_name    TEXT,
    payload      JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (trace_id, sequence)
);

CREATE INDEX IF NOT EXISTS agent_trace_events_task_created_idx
    ON agent_trace_events (task_id, created_at);
CREATE INDEX IF NOT EXISTS agent_trace_events_created_idx
    ON agent_trace_events (created_at);
```

`trace_id` identifies one Harness runtime invocation. `role` is `coding`,
`evaluator`, or `reviewer`; application validation rejects unknown roles.
`attempt` supports future retries without changing the schema. `sequence` is a
monotonic arrival order assigned by Sidecar before asynchronous persistence, so
database insertion order is not treated as execution order.

General task milestones remain in `task_events`. Detailed agent activity is
stored only in `agent_trace_events`. Request-usage trace records also update the
existing budget ledger described below.

## Configuration

```yaml
observability:
  agent_traces:
    enabled: true
    capture_assistant_text: final-only  # none | final-only | all-visible
    capture_tool_arguments: sanitized  # none | sanitized
    capture_tool_output: bounded       # none | metadata | bounded
    output_limit: 16KiB
    retention_days: 30
    log_tool_activity: true
```

Defaults are the values above. The library applies the same defaults and
validation to YAML and programmatic configuration. `output_limit` has a 64-KiB
hard cap. `retention_days` must be between 1 and 365. Disabling detailed traces
does not disable request-usage accounting, terminal task events, or evaluator
outcome classification.

Expired trace rows are deleted in bounded batches by a best-effort daily
retention job. Retention failure is logged and recorded as an operational
event; it never causes raw data to be copied elsewhere.

## Event mapping and correlation

Each runtime invocation creates one collector with a trace ID, role, attempt,
sequence counter, visible-text buffer, and in-flight tool map keyed by Harness
tool-call ID.

| Harness event | Stored trace event |
|---|---|
| `EventToolCallReady` | `tool_call`, sanitized arguments, command hash, start time |
| `EventToolResult` | `tool_result`, duration, exit status, bounded/redacted result metadata |
| `EventRequestUsage` | `request_usage`, request ID, request sequence, model, category, status, source, token fields |
| `EventTextDelta` | buffered visible assistant text according to capture mode |
| compaction events | `compaction_started`, `compaction_completed`, or `compaction_skipped` without summary text |
| `EventDone` | `agent_completed` with aggregate reported usage metadata |
| `EventAborted` | `agent_aborted` with sanitized cause category |
| `EventError` | `agent_error` with sanitized error category and message |

`EventRequestUsage` supplies the provider request ID. Sidecar assigns a
monotonic `request_sequence` and associates buffered visible text/tool activity
with that request boundary. It does not claim access to provider-hidden
reasoning or invent unavailable usage.

Tool duration is measured from `EventToolCallReady` to the matching
`EventToolResult`. Bash exit code, cancellation, timeout, byte counts, and
truncation are taken from Harness result metadata when present. Missing values
are stored as unknown, not fabricated. File tools record normalized
workspace-relative paths and operation type; default tracing does not persist
file contents.

The event channel is always drained through completion, including after error
or cancellation. Trace writes use a bounded batch queue. Usage and terminal
records are flushed synchronously before Sidecar applies task status or cleanup
policy. Non-critical trace-write failure does not deadlock the Harness stream;
Sidecar continues draining, emits `trace_persistence_failed`, and never falls
back to raw logging.

## Visible text and chain-of-thought boundary

Only `EventTextDelta` is eligible for assistant-text capture. No session
thinking blocks, provider reasoning fields, system prompts, user prompts, or
full message history are persisted.

- `none`: no assistant text is stored.
- `final-only`: store only the bounded visible response associated with normal
  completion or the last visible response before terminal failure.
- `all-visible`: store bounded visible assistant messages by request sequence.

All modes apply redaction and a configured byte bound. Truncation is explicit.

## Sanitization and redaction

Sanitization occurs before both persistence and structured logging:

- redact JSON keys matching token, password, secret, authorization, cookie,
  API-key, DSN, database URL, private-key, and environment-value classes;
- redact values equal to known configured credentials or resolved secret
  environment values;
- redact authorization/header syntax, URL user-info, private-key blocks, and
  common credential formats from free text;
- store repository-relative file paths rather than workspace absolute paths;
- for Bash, store a SHA-256 command hash and a redacted bounded preview, never
  an unbounded raw command;
- for file tools, store operation/path/size/hash metadata and no file body by
  default;
- bound output before database insertion and record original byte count and
  truncation state; and
- omit images and binary payloads, recording only safe type/count/size metadata.

Redaction is recursive across nested maps, arrays, tool metadata, errors, and
text. If JSON arguments cannot be parsed safely, Sidecar stores only their hash,
byte length, and a parse-error marker.

INFO logs contain task ID, trace ID, role, sequence, tool name, duration, exit
code, and success/error state. Detailed arguments and output remain in the
sanitized database trace and are not emitted to normal server logs.

## Usage accounting

`EventRequestUsage` is the authoritative unit for consumption. Sidecar persists
every provider attempt, including generation, retry, and compaction records on
error paths. A nil usage is stored with `source: unavailable`; it is not treated
as zero or replaced by another request's usage.

For budget accounting, each reported request ID is inserted idempotently into a
durable usage ledger before terminal evaluator policy runs. Terminal
`EventDone.Usage` is retained for trace diagnostics but is not added again when
request-level records exist. This prevents both the observed error-path
undercount and aggregate double-counting.

If durable request-usage persistence fails, Sidecar cancels further model work,
continues draining the current event stream, marks metering as failed, and does
not start another model request. The configured metering-error policy may
decide final task status, but it cannot silently report the attempt as zero.

## Evaluator outcome classification

A parsed negative evaluator verdict is `evaluation_rejected`. A Harness
`EventError`, `EventAborted`, exhausted-turn error, provider/transport failure,
or unparseable terminal response is `evaluation_error`. Both fail closed, but
their trace records, task events, status reason, and logs remain distinct.

This change does not select retry, suggestion, draft-request, or worktree
preservation behavior. Those workflow policies are defined separately. It does
prevent logs and records from describing an evaluator runtime error as a model
rejection.

## Queries and access

Store methods list trace events by task, trace ID, role, and sequence with a
bounded page size. CLI/API presentation defaults to metadata and requires an
explicit detailed view for sanitized payloads. Trace access follows the same
authorization boundary as task access. No endpoint returns unredacted values.

The completion and delivery layer may embed a bounded sanitized action/decision
summary and authorized trace reference in a pull or merge request. It SHALL NOT
copy the complete trace into the code-hosting provider. Change-request outcome,
incomplete status, draft warnings, and human-handoff policy remain the
responsibility of the `deterministic-agent-completion` change.
