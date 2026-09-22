# Proposal: Provider-Aware Change Delivery

## Problem

Sidecar accepts CI signals from GitHub, GitLab, and CircleCI, but its
`pull-request` output path is hard-coded to GitHub. A GitLab-triggered repair
therefore attempts to push to `github.com` and open a GitHub pull request.

Delivery errors are only logged. The task is subsequently marked `completed`
and a completion notification is emitted even when no branch was pushed or
change request was created. Tasks without repository details can fail to
attempt delivery at all and still report success.

## Intended outcome

Sidecar SHALL deliver an approved repair through the provider configured for
the attached repository:

- GitHub repositories receive pull requests.
- GitLab repositories receive merge requests, including repositories hosted
  on configurable self-managed or dedicated GitLab instances.
- Delivery configuration is independent of the signal source.
- A `pull-request` task is completed only after a change-request URL is
  returned and recorded.
- Push or change-request failures produce a failed task and notification with
  an auditable delivery event.

## Scope

Introduce repository-level delivery configuration, a provider-neutral change
delivery interface, GitHub and GitLab implementations, secure branch pushing,
provider-specific API calls, idempotent existing-request detection, explicit
delivery events, and fail-closed output status handling.

Existing GitHub configurations remain compatible. This change does not add
Bitbucket support, merge change requests automatically, or replace provider
branch-protection and CI policies.
