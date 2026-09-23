# Tasks

## Schema and configuration

- [x] Add the idempotent `agent_trace_events` table and indexes.
- [x] Add store methods for batched append, bounded ordered retrieval, and
      bounded retention deletion.
- [x] Add trace configuration, defaults, enum validation, 64-KiB output cap,
      and 1–365-day retention bounds for YAML and public Go configuration.
- [x] Add an idempotent request-usage ledger keyed by provider request ID for
      budget accounting without terminal aggregate double-counting.

## Sanitization

- [x] Implement recursive key/value/free-text redaction for configured secrets,
      credentials, headers, URLs, DSNs, private keys, and environment values.
- [x] Implement UTF-8-safe byte bounding with original size and truncation
      metadata.
- [x] Implement SHA-256 command/argument hashing and malformed-JSON fallback.
- [x] Normalize file-tool paths to repository-relative paths and omit file
      bodies by default.
- [x] Omit image/binary content while retaining safe metadata.

## Harness event collection

- [x] Implement a collector with trace ID, role, attempt, atomic sequence,
      request sequence, text buffers, and in-flight tools keyed by tool-call ID.
- [x] Persist sanitized `EventToolCallReady` and `EventToolResult` events with
      measured duration and available exit/truncation metadata.
- [x] Persist every `EventRequestUsage`, including unavailable usage and events
      preceding errors, aborts, retries, and compaction.
- [x] Implement `none`, `final-only`, and `all-visible` visible-text capture
      without reading Harness thinking blocks or full sessions.
- [x] Persist compaction lifecycle without compaction summary text.
- [x] Persist completed, aborted, and error terminal events and always drain the
      Harness event channel.
- [x] Add bounded batching/backpressure and safe trace-persistence failure
      reporting with no raw-log fallback.

## Sidecar integration and accounting

- [x] Wire tracing into coding, evaluator, and reviewer runtime consumers.
- [x] Emit safe INFO tool activity with task/trace/role/tool/duration/exit only.
- [x] Replace terminal-only evaluator accounting with per-request idempotent
      usage persistence before evaluator policy and worktree cleanup.
- [x] Use request-level usage for budgets and retain terminal aggregates only as
      non-additive diagnostics.
- [x] Cancel further model work and drain the active stream after metering
      persistence failure.
- [x] Record evaluator pass, rejection, and runtime error as distinct outcomes
      and remove misleading rejection logs for error paths.
- [x] Add the bounded daily trace-retention job.

## Tests and documentation

- [x] Add schema/store integration tests for ordering, idempotent usage,
      pagination, cascade deletion, and retention batches.
- [x] Add unit tests for nested redaction, known-secret values, malformed JSON,
      UTF-8 truncation, command hashes, file paths, and binary omission.
- [x] Add collector tests for concurrent tools, request boundaries, compaction,
      completion, abort, maximum-turn error, and channel draining.
- [x] Add evaluator-error integration coverage proving usage survives failure,
      budget totals include it once, and the outcome is not a rejection.
- [x] Add tests proving hidden reasoning, prompts, raw CI logs, file bodies, and
      credentials are never persisted or logged.
- [x] Document configuration, trace schema, privacy boundary, retention,
      retrieval, and the Sidecar/Harness responsibility split.
- [x] Run `openspec validate durable-agent-execution-tracing --strict`.
- [x] Run `go test ./...`, `go vet ./...`, and `go build ./...`.
