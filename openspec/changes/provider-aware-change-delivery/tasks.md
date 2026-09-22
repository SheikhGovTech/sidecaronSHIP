# Tasks

## Configuration and resolution

- [x] Add repository-level delivery configuration for provider, repository,
      remote, API base URL, token, and base branch.
- [x] Resolve `$ENV_VAR` token references without persisting token values.
- [x] Parse HTTPS, SSH, and scp-style Git remote URLs.
- [x] Infer provider and repository only when unambiguous.
- [x] Resolve the remote default branch without silently assuming `main`.
- [x] Preserve the existing GitHub configuration fallback.
- [x] Validate provider names, repository slugs, API URLs, and remote names.

## Provider-neutral publication

- [x] Introduce provider-neutral publication request, result, and interface
      types.
- [x] Refactor existing GitHub branch push and pull-request creation behind the
      interface.
- [x] Add GitHub lookup for an existing open pull request.
- [x] Add GitLab merge-request lookup and creation.
- [x] URL-encode nested GitLab project paths.
- [x] Support configurable GitHub and GitLab API base URLs.
- [x] Require a non-empty change-request URL for successful publication.

## Secure branch delivery

- [x] Push through the configured Git remote rather than a constructed host URL.
- [x] Preserve SSH remote and credential-helper behavior.
- [x] Add environment-backed HTTPS askpass authentication.
- [x] Ensure credentials never appear in arguments, URLs, scripts, logs, errors,
      or task events.
- [x] Remove temporary askpass files on success and failure.

## Loop state and audit trail

- [x] Route `pull-request` output through the resolved publisher.
- [x] Record `branch_pushed`, `change_request_created`,
      `change_request_reused`, and `delivery_failed` events.
- [x] Mark push, lookup, creation, empty-URL, and resolution failures as failed.
- [x] Emit failed rather than completed notifications on delivery failure.
- [x] Preserve current behavior for `suggest-only`, `notify`, and `auto-commit`.
- [x] Keep successfully delivered branch and request metadata in the task audit
      trail without credentials.

## Tests and documentation

- [x] Add configuration parsing, validation, default, and credential-resolution
      tests.
- [x] Add remote parsing tests for HTTPS, SSH, scp syntax, custom hosts, and
      malformed or ambiguous remotes.
- [x] Add GitHub create, reuse, custom endpoint, and failure tests.
- [x] Add GitLab create, reuse, nested project, custom endpoint, authentication,
      and failure tests.
- [x] Add push tests for SSH, HTTPS askpass, cleanup, and credential redaction.
- [x] Add loop tests proving delivery failures cannot produce completed status
      or completed notifications.
- [x] Add integration tests for GitHub pull-request and GitLab merge-request
      output routing.
- [x] Document explicit GitHub, GitHub Enterprise, GitLab.com, and self-managed
      GitLab delivery configuration.
- [x] Run `openspec validate provider-aware-change-delivery --strict`,
      `go test ./...`, `go vet ./...`, and `go build ./...`.
