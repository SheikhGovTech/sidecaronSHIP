# Design: Provider-Aware Change Delivery

## Repository delivery contract

Delivery belongs to the attached repository, not to the signal adapter. A
schedule, log, uptime, or CircleCI signal can target a GitLab repository, while
a GitLab CI signal can describe a repository mirrored elsewhere. Signal source
SHALL therefore not be the sole provider-selection mechanism.

Add repository-level configuration:

```yaml
delivery:
  provider: gitlab
  repo: group/project
  remote: origin
  api_base_url: https://gitlab.example.gov/api/v4
  token: $GITLAB_TOKEN
  base_branch: main
```

Fields:

- `provider`: `github` or `gitlab`; empty means infer from the configured Git
  remote when unambiguous.
- `repo`: provider repository slug; empty means parse it from the remote URL.
- `remote`: Git remote name, default `origin`.
- `api_base_url`: provider API root. Defaults to
  `https://api.github.com` for GitHub and `https://gitlab.com/api/v4` for
  GitLab.
- `token`: literal value or `$ENV_VAR`. When empty, use `GITHUB_TOKEN` or
  `GITLAB_TOKEN` according to the resolved provider.
- `base_branch`: target branch. When empty, resolve `refs/remotes/<remote>/HEAD`;
  failure to resolve requires an explicit value rather than silently assuming
  `main`.

Explicit configuration takes precedence over remote inference. Supported
remote forms include HTTPS and SSH/scp syntax. Ambiguous or unsupported hosts
fail configuration or delivery clearly.

For backward compatibility, an absent `delivery` block MAY use the existing
GitHub signal repository and token when they are available. New deployments
SHOULD configure delivery explicitly.

## Provider-neutral interface

Separate local commit preparation from remote delivery:

```go
type Publisher interface {
    Publish(ctx context.Context, req Request) (Result, error)
}

type Request struct {
    RepoPath   string
    Remote     string
    Repo       string
    Branch     string
    BaseBranch string
    Title      string
    Body       string
}

type Result struct {
    Provider string
    URL      string
}
```

`Publish` pushes the prepared branch and creates or locates the provider's
open change request. It returns success only with a non-empty URL.

## GitHub delivery

Preserve GitHub behavior while moving it behind `Publisher`:

1. Push the prepared branch to the configured remote.
2. Find an existing open pull request for the source and base branches.
3. Create the pull request when none exists.
4. Return its HTML URL.

The GitHub API path remains `/repos/{owner}/{repo}/pulls`. GitHub Enterprise is
supported through `api_base_url`.

## GitLab delivery

GitLab delivery SHALL:

1. Push the prepared branch to the configured remote.
2. URL-encode the full project path.
3. Query open merge requests for the source and target branches.
4. Create one with `POST /projects/{project}/merge_requests` when none exists.
5. Authenticate API requests with the configured GitLab token.
6. Return the merge request `web_url`.

This supports GitLab.com and configurable self-managed or dedicated GitLab
instances without embedding a deployment-specific hostname in output code.

## Secure branch push

Sidecar SHALL push to the configured Git remote rather than constructing a
provider hostname from a repository slug. This preserves SSH remotes, custom
hosts, and deployment credential helpers.

For HTTPS token authentication, use a temporary `GIT_ASKPASS` helper that
reads credentials from environment variables. Token values SHALL not be
embedded in command arguments, remote URLs, generated script contents, logs,
or task events. Temporary helpers use mode `0700` and are removed after use.

## State and event semantics

For `pull-request` autonomy:

```text
verified change
  → local commit prepared
  → branch push
  → change request create/find
  → record URL
  → completed notification
```

Events:

- `branch_pushed`: provider, remote, repository, and branch; no credential.
- `change_request_created`: provider, URL, source branch, and base branch.
- `change_request_reused`: same fields when an existing open request is found.
- `delivery_failed`: provider, phase (`resolve`, `push`, `lookup`, or `create`),
  branch, and redacted error.

A push or API failure sets task status to `failed`, emits the failed
notification, and returns an error. Sidecar SHALL NOT emit `completed` or
record a change-request event without a non-empty URL.

A successfully pushed branch may remain available after API failure for
diagnosis or retry. Sidecar SHALL record that partial state accurately.

## Idempotency

Before creating a request, each provider checks for an existing open request
with the same repository, source branch, and base branch. Retrying delivery
returns the existing URL instead of creating a duplicate.

## Non-goals

- Automatically merging pull or merge requests.
- Bypassing protected branches, required reviews, or provider CI.
- Adding providers other than GitHub and GitLab in this change.
- Changing CI ingestion, triage, coding, verification, or evaluation.
- Treating CircleCI as a source-code hosting provider.
