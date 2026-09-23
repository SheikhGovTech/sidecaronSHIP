# Proposal: Governed Repair Workflows

## Problem

Sidecar's repair lifecycle contains policy decisions that are either unsafe or
hard-coded. Most urgently, configuration loading accepts unknown autonomy
values. An unknown value is not treated as code-shipping during workspace and
verification setup, but later falls through output routing as `auto-commit`.
This can bypass the isolated worktree and evaluator while still granting write
and Bash tools. In addition, configured `scope.include` and `scope.exclude`
rules are parsed but never enforced.

Other lifecycle controls—agent limits, evaluator error handling, repair
preservation, deduplication, budgets, verification limits, concurrency,
delivery retries, notification retries, branch naming, memory retrieval, and
CI enrichment bounds—are compiled into the binary. Operators cannot tune them
for repository risk, deployment capacity, or model-gateway behavior.

## Intended outcome

Sidecar SHALL provide a governed repair workflow that:

- rejects unsafe or unknown configuration before starting;
- fails closed if an unknown autonomy value reaches runtime routing;
- enforces repository scope on every proposed diff, including untracked files;
- lets an embedding host application supply the same validated workflow policy
  as YAML, including bounded turns, timeouts, retries, and repair cycles;
- exposes bounded workflow controls with backward-compatible defaults;
- distinguishes evaluator rejection from evaluator infrastructure failure;
- can return a failed verification or rejection to coding with evidence, or
  resume an interrupted task from a durable checkpoint, when explicitly
  configured;
- preserves or delivers failed/rejected repairs only according to explicit
  policy, never by implicit approval;
- gives human reviewers bounded, sanitized repair-agent and evaluator decision
  evidence in every generated change request, with references to durable trace
  records;
- accounts for partial model usage and supports per-task cost limits;
- allows controlled retry of failed signals without reprocessing completed
  work; and
- makes operational limits observable and configurable within upstream safety
  caps.

## Scope

This change introduces strict configuration decoding and validation, scoped
mutation enforcement, workflow stage policies, explicit evaluator outcomes,
bounded repair cycles and recovery checkpoints, repair-preservation policies,
complete usage accounting, budget and dedupe controls, and bounded operational
settings. Configuration is available both through YAML and the public Go
configuration API used by applications embedding Sidecar as a library.

Implementation is split into independently reviewable phases. The autonomy and
scope fixes are the first release gate; later policy controls SHALL NOT delay
those fixes.

## Non-goals

This change does not grant models additional privileges, install project
dependencies, replace CI, auto-merge evaluator failures, add a new planning
agent, or remove upstream safety caps. The existing `models.planning` field is
documented as reserved and remains inactive until a separate planning-stage
change defines its behavior.
