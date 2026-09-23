# Tasks

## Completion model

- [x] Add typed coding and evaluator terminal outcomes, including
      `incomplete` separately from `reject` and `error`.
- [x] Define normalized coding completion records for changed and no-change
      outcomes.
- [x] Define the strict evaluator verdict schema and bounded evidence
      references.
- [x] Persist completion outcome, stop reason, attempt, requests used, and
      trace ID in task events.

## Evaluator evidence and stopping

- [x] Pass structured bounded/redacted deterministic-verification results to
      the evaluator instead of only listing configured commands.
- [x] Add a material-risk checklist and explicit immediate-verdict instruction
      to the evaluator prompt.
- [x] Add a Sidecar-enforced investigation boundary below the Harness hard
      turn limit.
- [x] Reserve at least one provider request for a tool-free verdict
      finalization phase.
- [x] Stop the evaluator immediately after accepting a valid verdict.
- [x] Ensure the hard Harness turn limit remains the final safety boundary.

## Redundant investigation control

- [x] Identify equivalent tool calls using tool name and sanitized argument
      hash from durable tracing.
- [x] Identify evaluator commands equivalent to completed deterministic
      verification.
- [x] Return existing evidence instead of executing an unjustified duplicate
      command.
- [x] Move evaluation to finalization after bounded repeated investigation.
- [x] Record repetition counts, denied calls, and transition reasons without
      storing hidden reasoning.

## Incomplete routing and human handoff

- [x] Record `coding_incomplete` and `evaluation_incomplete` without calling
      either a rejection.
- [x] Build a bounded sanitized handoff with completed work, remaining work,
      stop reason, recommended action, task ID, and trace ID.
- [x] Preserve useful coding findings as a suggestion while preventing normal
      delivery of partial code.
- [x] Route a deterministically verified but unevaluated repair through the
      existing explicit preservation policy only.
- [x] Mark evaluator-incomplete drafts `needs_review` and visibly state that
      evaluator approval was not obtained.
- [x] Mark an incomplete run with no useful output as failed rather than
      creating an empty suggestion.
- [x] Include the bounded handoff in suggested Slack, email, and webhook
      notifications.
- [x] Add `sidecar task show <task-id>` for sanitized suggestion and trace
      details.

## Tests

- [x] Test coding completion with a diff, explicit no-change, exhausted turns,
      and missing terminal output.
- [x] Test that prior verification evidence is supplied and equivalent test
      commands are not rerun.
- [x] Test early evaluator PASS and REJECT terminate without further tools.
- [x] Test the investigation boundary preserves verdict-finalization capacity.
- [x] Test repeated Bash, file, and Git-history calls trigger finalization.
- [x] Test malformed final verdict becomes `evaluation_incomplete`.
- [x] Test provider/runtime faults remain `evaluation_error`.
- [x] Test incomplete outcomes never approve or normally deliver code.
- [x] Test suggestion, draft, failed, and notification handoff paths.
- [x] Test all handoff output is bounded and redacted.
- [x] Reproduce the 20-request canary pattern and prove the evaluator emits or
      attempts its verdict before the hard limit.

## Documentation and verification

- [x] Document completion outcomes and the difference between investigation
      boundaries and hard turn limits.
- [x] Document the default incomplete-work handoff and draft warning.
- [x] Update the evaluator incident record with the traced canary evidence.
- [x] Run `openspec validate deterministic-agent-completion --strict`.
- [x] Run `go test ./...`, `go vet ./...`, and `go build ./...`.
