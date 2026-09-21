# Design

## Context

The sidecaronSHIP fork (`SheikhGovTech/sidecaronSHIP`) is a patched version of `sausheong/sidecar` for SHIP-HATS GitLab. The GitLab PAT has full `api` scope. Three files need patching: `gitlabci.go` (adapter), `triage.go` (triage prompt), `loop.go` (coding agent prompts + PAi routing). All changes go in the fork repo; the UQD2 Dockerfile points to the fork SHA.

The upstream uptime adapter already enriches signals with diagnostics (DNS/TCP/TLS) before triage — the CI adapter needs to follow this established pattern.

## Goals / Non-Goals

**Goals:**
- Give triage actual error output so it can act instead of skipping 100% of CI failures
- Give the coding agent error context + commit diff so it can fix issues without guessing
- Detect flaky tests from pipeline history to avoid wasting tokens on non-deterministic failures
- Make error extraction smart (pattern-based) not dumb (blind tail)
- Keep all changes backward-compatible — empty fields when data unavailable

**Non-Goals:**
- Changing triage decision logic or skip thresholds (that's the author's design choice)
- Adding GitLab API tools to the coding agent runtime (too much architecture change)
- Supporting GitHub or CircleCI in this fork (upstream concern)
- Changing autonomy level behavior

## Decisions

### 1. Smart error extraction over blind tail
**Decision:** Scan full log for error patterns, extract matching lines with ±2 context, always include last 30 lines (summary). Cap at 150 lines.
**Rationale:** A 5000-line CI log has errors at line 3530 (real case). Blind tail-150 misses them. Pattern matching finds errors wherever they are. The last 30 lines always capture the test summary.

### 2. Configurable error patterns
**Decision:** Ship default patterns covering common frameworks (vitest, jest, go test, pytest, gcc, docker). Allow `error_patterns` yaml field for user additions.
**Rationale:** Error formats depend on the CI framework, not GitLab. Defaults cover 80% of cases; config covers the rest.

### 3. Commit diff in signal payload
**Decision:** Fetch `GET /commits/:sha/diff` and include as `commit_diff` (full diff, 64KB cap) and `changed_files` (file list only).
**Rationale:** The coding agent currently has to `git show <sha>` to see what changed. Pre-fetching the diff saves an agent tool call and works even when the worktree is on a different branch. Large diffs (>32KB) fall back to file list only to avoid oversized prompts.

### 4. Flake detection from pipeline history
**Decision:** Fetch last 5 pipelines for the same ref. If failure pattern alternates (pass/fail/pass), set `is_flake: true`.
**Rationale:** Flaky tests waste tokens — the agent investigates, can't reproduce, and fails. Triage can skip flakes or classify them differently. Simple heuristic: >1 failure in last 5 with at least 1 pass = flaky.

### 5. Three prompt touchpoints
**Decision:** Surface enriched data in all three prompt builders: `BuildTriageMessage()`, `BuildSystemPrompt()`, `userMessage()`.
**Rationale:** Without all three, data flows through the payload but doesn't reach the LLM. Triage needs logs to decide act/skip. The coding agent needs logs to identify the error and diff to see what changed.

### 6. PAi base URL via environment variable
**Decision:** Change `NewAnthropicProvider(key, "")` to `NewAnthropicProvider(key, os.Getenv("ANTHROPIC_BASE_URL"))` in `loop.go`.
**Rationale:** PAi endpoint is Anthropic-compatible. The SDK respects base URL override. One line change, no config field needed.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Enriched triage prompt exceeds Haiku context | Low | Medium | 150-line cap on error extraction, 64KB cap on diff |
| Smart extraction misses error | Low | Low | Always includes last 30 lines as fallback |
| GitLab API rate limits hit | Low | Low | 3-4 calls per failure, not per poll. Best-effort on failure |
| Flake detection false positive | Medium | Low | Simple heuristic; triage still has final say |
| Fork diverges from upstream | Medium | Medium | Structured for upstream PR; changes are additive |
| Commit diff contains secrets | Low | High | Diff goes to LLM prompt only, never logged or stored |
