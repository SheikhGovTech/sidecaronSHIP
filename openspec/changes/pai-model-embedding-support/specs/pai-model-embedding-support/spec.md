# PAi Model and Embedding Support

## ADDED Requirements

### Requirement: Configurable Anthropic-compatible LLM access

The LLM provider SHALL support a configurable Anthropic-compatible endpoint and
credential source, including environment-based deployment configuration. When
the endpoint is unset, existing provider defaults SHALL remain unchanged.

#### Scenario: PAi endpoint configured

- **WHEN** an Anthropic-compatible PAi endpoint is configured
- **THEN** LLM requests are sent to that endpoint using the configured
  credentials

#### Scenario: Default endpoint compatibility

- **WHEN** no custom LLM endpoint is configured
- **THEN** Sidecar continues using the existing Anthropic provider endpoint and
  authentication behavior

### Requirement: Opaque PAi model identifiers

The provider SHALL pass model identifiers such as `bedrock.claude-sonnet-4-5`
unchanged to the compatible endpoint.

### Requirement: PAi-compatible embeddings

The embedding layer SHALL support an OpenAI-compatible `/v1/embeddings`
endpoint with configurable endpoint, credentials, model, and dimensions.

#### Scenario: PAi embedding request

- **WHEN** PAi embedding configuration is selected
- **THEN** Sidecar sends the configured model and input to `/v1/embeddings`
- **AND** parses the returned vectors for memory storage and retrieval

#### Scenario: PAi gateway path prefix

- **WHEN** the embedding base URL is configured with a gateway prefix such as
  `/platform/models`
- **THEN** the request is sent to `/platform/models/v1/embeddings`

### Requirement: Cohere embeddings

The embedding layer SHALL support Cohere models and send the configured
`input_type` for query and document embeddings.

#### Scenario: Default Cohere model

- **WHEN** Cohere embeddings are configured without an explicit model
- **THEN** Sidecar uses `cohere.embed-english-v3`
- **AND** produces vectors compatible with the existing 1024-dimensional
  pgvector schema

#### Scenario: Cohere query and document input types

- **WHEN** Sidecar embeds a search query
- **THEN** the provider sends `input_type: "search_query"`
- **WHEN** Sidecar stores memory content
- **THEN** the provider sends `input_type: "search_document"`

### Requirement: Configuration documentation and tests

The project SHALL document endpoint, credential, provider, model, dimension,
and input-type configuration and SHALL provide unit tests for request routing,
authentication, model propagation, response parsing, and dimension handling.

### Requirement: No database migration for provider support

The change SHALL not require a database migration for supported 1024-dimensional
providers. Providers returning another dimension SHALL be rejected or handled
explicitly rather than writing vectors incompatible with `vector(1024)`.

### Requirement: Downstream overlay removal path

After the upstream capability is merged and adopted, downstream deployments
SHALL be able to remove their local `cohere.go` and `embedding.go` Docker
overlays without losing provider functionality.
