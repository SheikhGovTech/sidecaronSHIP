# Tasks

## Completion model

- [ ] Add typed coding and evaluator terminal outcomes, including
      `incomplete` separately from `reject` and `error`.
- [ ] Define normalized coding completion records for changed and no-change
      outcomes.
- [ ] Define the strict evaluator verdict schema and bounded evidence
      references.
- [ ] Persist completion outcome, stop reason, attempt, requests used, and
      trace ID in task events.

## Evaluator evidence and stopping

- [ ] Pass structured bounded/redacted deterministic-verification results to
      the evaluator instead of only listing configured commands.
- [ ] Add a material-risk checklist and explicit immediate-verdict instruction
      to the evaluator prompt.
- [ ] Add a Sidecar-enforced investigation boundary below the Harness hard
      turn limit.
- [ ] Reserve at least one provider request for a tool-free verdict
      finalization phase.
- [ ] Stop the evaluator immediately after accepting a valid verdict.
- [ ] Ensure the hard Harness turn limit remains the final safety boundary.

## Redundant investigation control

- [ ] Identify equivalent tool calls using tool name and sanitized argument
      hash from durable tracing.
- [ ] Identify evaluator commands equivalent to completed deterministic
      verification.
- [ ] Return existing evidence instead of executing an unjustified duplicate
      command.
- [ ] Move evaluation to finalization after bounded repeated investigation.
- [ ] Record repetition counts, denied calls, and transition reasons without
      storing hidden reasoning.

## Incomplete routing and human handoff

- [ ] Record `coding_incomplete` and `evaluation_incomplete` without calling
      either a rejection.
- [ ] Build a bounded sanitized handoff with completed work, remaining work,
      stop reason, recommended action, task ID, and trace ID.
- [ ] Preserve useful coding findings as a suggestion while preventing normal
      delivery of partial code.
- [ ] Route a deterministically verified but unevaluated repair through the
      existing explicit preservation policy only.
- [ ] Mark evaluator-incomplete drafts `needs_review` and visibly state that
      evaluator approval was not obtained.
- [ ] Mark an incomplete run with no useful output as failed rather than
      creating an empty suggestion.
- [ ] Include the bounded handoff in suggested Slack, email, and webhook
      notifications.
- [ ] Add `sidecar task show <task-id>` for sanitized suggestion and trace
      details.

## Tests

- [ ] Test coding completion with a diff, explicit no-change, exhausted turns,
      and missing terminal output.
- [ ] Test that prior verification evidence is supplied and equivalent test
      commands are not rerun.
- [ ] Test early evaluator PASS and REJECT terminate without further tools.
- [ ] Test the investigation boundary preserves verdict-finalization capacity.
- [ ] Test repeated Bash, file, and Git-history calls trigger finalization.
- [ ] Test malformed final verdict becomes `evaluation_incomplete`.
- [ ] Test provider/runtime faults remain `evaluation_error`.
- [ ] Test incomplete outcomes never approve or normally deliver code.
- [ ] Test suggestion, draft, failed, and notification handoff paths.
- [ ] Test all handoff output is bounded and redacted.
- [ ] Reproduce the 20-request canary pattern and prove the evaluator emits or
      attempts its verdict before the hard limit.

## Documentation and verification

- [ ] Document completion outcomes and the difference between investigation
      boundaries and hard turn limits.
- [ ] Document the default incomplete-work handoff and draft warning.
- [ ] Update the evaluator incident record with the traced canary evidence.
- [ ] Run `openspec validate deterministic-agent-completion --strict`.
- [ ] Run `go test ./...`, `go vet ./...`, and `go build ./...`.
