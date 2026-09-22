# Workspace-Aware Repair Execution and Deterministic Verification

## ADDED Requirements

### Requirement: Explicit workspace context

Every actionable task SHALL provide the resolved execution directory to all
filesystem tools, Bash, the coding runtime, and the evaluator. The coding-agent
prompt SHALL include the exact path and instruct the agent to use relative paths
without guessing repository locations.

#### Scenario: Code-shipping task

- **WHEN** Sidecar creates an isolated worktree
- **THEN** the generated worktree path is supplied consistently to tools,
  prompts, runtime, and evaluator

#### Scenario: Suggest-only task

- **WHEN** a task does not create a worktree
- **THEN** the attached repository path is supplied as the workspace context

### Requirement: Configurable verification commands

The verification configuration SHALL support named commands with a command
string, explicit required tools, timeout, worktree-relative working directory,
and environment allowlist. Commands SHALL run sequentially after the coding
agent completes.

#### Scenario: Successful verification

- **WHEN** all configured commands exit successfully
- **THEN** Sidecar proceeds to adversarial evaluation and configured output
  routing

#### Scenario: Failed verification

- **WHEN** any configured command exits non-zero or times out
- **THEN** Sidecar records a bounded verification failure event
- **AND** does not commit, push a branch, or create a merge request

#### Scenario: No configured commands

- **WHEN** verification commands are empty
- **THEN** existing behavior remains unchanged

#### Scenario: Verification disabled

- **WHEN** `verification.enabled` is false
- **THEN** deterministic commands and the evaluator gate are disabled
- **AND** existing output routing behavior is preserved

#### Scenario: Commands provided to agents

- **WHEN** verification is enabled for a code-shipping task
- **THEN** the coding-agent and evaluator prompts list each command, working
  directory, and timeout

#### Scenario: Suggest-only or unchanged task

- **WHEN** a task is suggest-only or produces no code changes
- **THEN** configured verification commands are not executed

#### Scenario: Command ordering

- **WHEN** multiple commands are configured
- **THEN** Sidecar executes them in configuration order
- **AND** stops at the first failure

### Requirement: Safe verification execution

Verification SHALL run as Sidecar's current OS user without privilege
elevation, enforce timeouts, bound output, reject absolute or
worktree-escaping directories, and avoid automatic dependency installation
from task branches. Deployments SHALL run Sidecar as an unprivileged user.

#### Scenario: Shell execution and timeout

- **WHEN** Sidecar executes a configured `run` string
- **THEN** it invokes the command through `/bin/sh -c`
- **AND** timeout cancellation terminates the entire process group, including
  child processes

#### Scenario: Invalid command configuration

- **WHEN** a command has an empty name, empty `run`, duplicate name, invalid
  timeout, absolute path, `..` segment, or symlink escape
- **THEN** configuration is rejected before task execution

#### Scenario: Restricted environment

- **WHEN** verification starts
- **THEN** the subprocess receives only a minimal environment and explicitly
  allowed `pass_env` variables
- **AND** Sidecar does not elevate privileges

### Requirement: Missing tool failure

When a configured command is unavailable, Sidecar SHALL report a clear failure
early rather than allowing the agent to repeatedly spend turns attempting the
same unavailable command.

#### Scenario: Missing executable

- **WHEN** preflight detects that a declared `required_tools` executable is
  unavailable on the verification `PATH`
- **THEN** Sidecar fails before starting the coding model
- **AND** reports the missing executable clearly

### Requirement: Workspace and verification audit trail

Sidecar SHALL record workspace preparation and verification failure events with
task context, branch/base metadata where applicable, command name, exit code,
and bounded output. Temporary absolute paths SHALL not be treated as durable
task state.

#### Scenario: Successful verification audit

- **WHEN** all configured commands pass
- **THEN** Sidecar records success with duration and output truncation state

#### Scenario: Failed verification routing

- **WHEN** a command fails or times out, including after an agent-created local
  commit
- **THEN** task status becomes `failed`
- **AND** failure notification is emitted
- **AND** evaluator and output routing are skipped
- **AND** worktree changes and local commits are discarded
- **AND** the temporary task branch is deleted where safe

### Requirement: Evaluator completion allowance

The adversarial evaluator SHALL be allowed up to 20 turns to assess a verified
repair before it is considered to have exceeded its turn limit.

#### Scenario: Evaluation requires more than eight turns

- **WHEN** assessment of a verified code-shipping repair requires more than
  eight evaluator turns
- **THEN** evaluation may continue up to the 20-turn limit
- **AND** Sidecar does not downgrade the repair to suggestion-only merely
  because the former eight-turn limit was reached

### Requirement: Deployment responsibility

Sidecar SHALL remain language-agnostic. Deployments SHALL provide required
runtimes, package managers, dependencies, and verification commands in the
repair environment.

#### Scenario: Prepared repair environment

- **WHEN** a deployment provides the configured runtime, package manager, and
  test dependencies
- **THEN** Sidecar invokes the configured verification commands without
  installing project dependencies itself
- **AND** the deployment runs Sidecar as an unprivileged OS user
