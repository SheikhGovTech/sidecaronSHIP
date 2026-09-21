# Spec Delta

## Purpose

Configures the sidecaronSHIP fork for SHIP-HATS GitLab and Platform AI endpoints instead of upstream defaults (gitlab.com, api.anthropic.com).

## ADDED Requirements

### Requirement: SHIP-HATS GitLab base URL
The GitLab CI adapter SHALL default to `https://sgts.gitlab-dedicated.com` instead of `https://gitlab.com`.

#### Scenario: Default base URL
- **WHEN** the adapter is initialized without an explicit base URL override
- **THEN** it uses `https://sgts.gitlab-dedicated.com` for all GitLab API calls

### Requirement: PAi endpoint routing
The LLM provider SHALL read `ANTHROPIC_BASE_URL` from the environment for Platform AI routing.

#### Scenario: PAi configured
- **WHEN** `ANTHROPIC_BASE_URL` is set to `https://api.ai.tech.gov.sg/platform/models`
- **THEN** all LLM calls route through PAi instead of api.anthropic.com

#### Scenario: PAi not configured
- **WHEN** `ANTHROPIC_BASE_URL` is empty
- **THEN** the provider falls back to the Anthropic default (upstream behavior)
