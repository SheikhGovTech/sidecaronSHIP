# Proposal: PAi Model and Embedding Support

## Problem

Sidecar currently assumes direct vendor APIs: LLMs default to Anthropic, while
semantic memory supports only OpenAI and Voyage embeddings. PAi provides an
approved Anthropic-compatible gateway for chat models and OpenAI-compatible
embeddings, but Sidecar cannot currently configure its endpoints, credentials,
or Cohere models without downstream source overlays.

Without PAi-compatible embeddings, semantic memory is disabled or fails with
authentication errors. Without configurable LLM routing, model identifiers such
as `bedrock.claude-sonnet-4-5` cannot be used through PAi.

PAi routes requests to Bedrock internally; Sidecar does not need direct AWS
Bedrock SDK integration.

## Intended outcome

Support PAi and other compatible gateways through configuration:

```yaml
models:
  triage: bedrock.claude-haiku-4-5
  coding: bedrock.claude-sonnet-4-5
embedding:
  provider: cohere
  model: cohere.embed-english-v3
```

The implementation will:

- Route chat requests through a configurable Anthropic-compatible endpoint.
- Pass PAi model identifiers through unchanged.
- Support OpenAI-compatible `/v1/embeddings` endpoints.
- Support Cohere `input_type` for query and document embeddings.
- Use 1024-dimensional `cohere.embed-english-v3` vectors compatible with the
  existing memory schema.
- Configure endpoints and credentials through environment variables.
- Preserve existing Anthropic, OpenAI, and Voyage defaults.

## Benefits

This removes deployment-specific patches, restores semantic memory for PAi
deployments, supports enterprise gateway governance, and improves portability
to other compatible model gateways.
