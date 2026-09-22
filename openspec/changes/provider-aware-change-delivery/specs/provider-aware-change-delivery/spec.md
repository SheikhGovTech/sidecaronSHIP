# Provider-Aware Change Delivery

## ADDED Requirements

### Requirement: Repository-scoped delivery configuration

Sidecar SHALL select change delivery from repository configuration rather than
solely from the triggering signal. Configuration SHALL support provider,
repository slug, remote, API base URL, credential reference, and base branch.

#### Scenario: Explicit GitLab configuration

- **WHEN** `delivery.provider` is `gitlab`
- **THEN** Sidecar uses the configured GitLab API and repository for delivery
- **AND** the behavior is independent of which adapter emitted the signal

#### Scenario: Safe remote inference

- **WHEN** provider or repository is omitted and the configured remote is
  unambiguous
- **THEN** Sidecar derives the missing value from that remote

#### Scenario: Ambiguous delivery target

- **WHEN** provider, repository, or base branch cannot be resolved safely
- **THEN** Sidecar fails delivery with a clear error
- **AND** does not silently assume a provider, repository, or `main` branch

### Requirement: Provider-neutral change publication

Sidecar SHALL publish approved branches through a common provider-neutral
interface with GitHub and GitLab implementations.

#### Scenario: GitHub repository

- **WHEN** the resolved provider is GitHub
- **THEN** Sidecar pushes the branch and creates or reuses a GitHub pull request
- **AND** records its non-empty URL

#### Scenario: GitLab repository

- **WHEN** the resolved provider is GitLab
- **THEN** Sidecar pushes the branch and creates or reuses a GitLab merge request
- **AND** records its non-empty `web_url`

#### Scenario: Custom provider endpoint

- **WHEN** a custom `api_base_url` is configured
- **THEN** provider API requests use that endpoint
- **AND** no public provider hostname is substituted

### Requirement: Secure branch push

Sidecar SHALL push to the configured Git remote without exposing credentials
in process arguments, remote URLs, generated script contents, logs, or task
events.

#### Scenario: Existing SSH remote

- **WHEN** the configured remote uses SSH
- **THEN** Sidecar pushes through that remote without constructing an HTTPS URL

#### Scenario: HTTPS token authentication

- **WHEN** HTTPS delivery requires a token
- **THEN** Sidecar supplies credentials through a temporary environment-backed
  askpass helper
- **AND** removes the helper after the push

### Requirement: Accurate delivery status

A `pull-request` task that produced a change SHALL reach `completed` only after
branch delivery and change-request creation or lookup succeeds with a non-empty
URL. A no-change task MAY retain the existing completed no-op behavior.

#### Scenario: Successful delivery

- **WHEN** the branch is pushed and a change-request URL is returned
- **THEN** Sidecar records the URL, marks the task completed, and emits the
  completed notification

#### Scenario: Push failure

- **WHEN** branch push fails
- **THEN** Sidecar records `delivery_failed` with phase `push`
- **AND** marks the task failed and emits the failed notification
- **AND** does not attempt change-request creation

#### Scenario: API failure after push

- **WHEN** the branch is pushed but request lookup or creation fails
- **THEN** Sidecar records the successful push and a redacted
  `delivery_failed` event
- **AND** marks the task failed and emits the failed notification
- **AND** does not emit a completed notification

#### Scenario: Missing repository details

- **WHEN** output autonomy is `pull-request` but delivery details are unavailable
- **THEN** Sidecar records a resolution failure and marks the task failed
- **AND** does not report successful completion

### Requirement: Idempotent change-request creation

Sidecar SHALL avoid duplicate pull or merge requests by looking for an existing
open request with the same repository, source branch, and base branch.

#### Scenario: Existing open request

- **WHEN** a matching open change request already exists
- **THEN** Sidecar returns and records its URL
- **AND** records `change_request_reused` instead of creating another request

### Requirement: Backward-compatible GitHub delivery

Existing GitHub delivery SHALL continue to work when explicit delivery
configuration is absent and the current GitHub repository and credential can
be resolved unambiguously.

#### Scenario: Existing GitHub CI configuration

- **WHEN** a current GitHub CI configuration supplies repository and token
- **AND** no `delivery` block is present
- **THEN** Sidecar can create a GitHub pull request using the existing values

#### Scenario: Non-pull-request autonomy

- **WHEN** autonomy is `suggest-only`, `notify`, or `auto-commit`
- **THEN** provider change-request publication is not invoked
