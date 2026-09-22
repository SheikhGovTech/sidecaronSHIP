# Design: PAi Model and Embedding Support

## Configuration

Provider endpoints, credentials, and model names SHALL be configurable through
environment variables and the existing YAML configuration. Secrets remain
outside repository configuration. At minimum, deployments need configurable
values equivalent to `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, an embedding
base URL, and an embedding credential. Model identifiers are opaque strings and
are passed to compatible providers without rewriting or validation that would
reject PAi names such as `bedrock.claude-sonnet-4-5`.

## LLM provider

The Anthropic-compatible provider receives an explicit base URL and credential
source. An unset endpoint preserves the current Anthropic default. PAi model
identifiers are sent unchanged in the provider request.

PAi routes requests to Bedrock internally; this change does not add an AWS
Bedrock SDK or AWS credential flow.

## Embedding providers

Add a PAi/OpenAI-compatible embedding implementation targeting
`/v1/embeddings`, with configurable base URL, credential, model, and vector
dimensions. The configured base URL may include a gateway prefix such as
`/platform/models`, resulting in a request to
`/platform/models/v1/embeddings`.

Add a Cohere implementation that sends `input_type` as `search_query` or
`search_document` according to the Sidecar embedding call site. Provider
authentication and non-success responses must return actionable errors while
leaving existing memory failure handling intact.

The default Cohere target is `cohere.embed-english-v3` with 1024 dimensions so
it remains compatible with the existing `vector(1024)` database column. The
provider must reject or clearly report incompatible vector dimensions before
they are stored.

## Compatibility and migration

Existing OpenAI and Voyage configuration remains supported. Existing default
endpoints remain unchanged when new variables are unset. No database schema
migration is required unless a future provider supports a different vector
dimension; dimensionality must be validated before storing vectors.

## Downstream cleanup

Once the upstream implementation is merged and consumed, downstream
deployments can remove their Docker overlays for `cohere.go` and
`embedding.go` in a follow-up deployment change.
