# Tasks

## Configuration and providers

- [x] Add configurable Anthropic-compatible endpoint and credential settings.
- [x] Preserve opaque PAi model identifiers such as `bedrock.claude-sonnet-4-5`.
- [x] Add PAi/OpenAI-compatible `/v1/embeddings` support.
- [x] Add configurable embedding endpoint, credential, model, and dimensions.
- [x] Add Cohere provider support with `input_type` mapping (`query` →
      `search_query`, `document` → `search_document`).
- [x] Add `cohere.embed-english-v3` at 1024 dimensions.

## Tests and documentation

- [ ] Add LLM endpoint/model request tests.
- [x] Add PAi embedding request and authentication tests.
- [x] Add Cohere `input_type` mapping and dimension tests.
- [x] Add configuration examples and environment-variable documentation.
- [x] Run provider/loop tests and `go build ./...`.

## Downstream adoption cleanup

- [ ] After upstream merge and adoption, remove downstream `cohere.go` overlay.
- [ ] After upstream merge and adoption, remove downstream `embedding.go` overlay.
