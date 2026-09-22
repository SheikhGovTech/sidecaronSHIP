# Design: Consolidated Sidecar on SHIP Record

## Architecture

Adapters emit typed signals. `Loop.Run` creates a task, applies budget and
triage gates, optionally retrieves memory, runs the coding agent, evaluates
code-shipping changes, and routes output according to autonomy. PostgreSQL
stores workspaces, tasks, task events, policies, and memory.

## SHIP-HATS integration

The GitLab adapter uses the SHIP-HATS base URL by default and enriches failed
pipelines through best-effort jobs, traces, commit-diff, and pipeline-history
requests. Prompt builders surface the resulting context. The Anthropic
provider reads `ANTHROPIC_BASE_URL` so Platform AI can be used without changing
application routing.

## Deduplication design

`tasks.signal_key` is nullable and has a partial unique index scoped by
workspace. `Loop.Run` computes `ci.failure:<pipeline_id|run_id>` or
`git.commit:<hash>` before task creation. It skips an existing key, fails open
on lookup errors, and leaves schedule, on-demand, log, metric, uptime, and
identifier-less signals with `NULL`. The unique index is the race-condition
guard for concurrent runs.

## Operational trade-offs

Enrichment adds several GitLab requests per failed pipeline but is bounded by
log and diff limits. Missing API data degrades gracefully. Deduplication saves
LLM spend across restarts but intentionally does not deduplicate event-like
signals. Live deployment and database verification remain environment-specific.
