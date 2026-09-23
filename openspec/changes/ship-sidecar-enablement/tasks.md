# Tasks

## Documentation and implementation record

- [x] Consolidate fork-to-present features into one capability spec.
- [x] Document the standard OpenSpec proposal/design/spec/tasks structure.
- [x] Record core workflow, demos, uptime, notifications, safety controls,
      skills, budgets, SHIP-HATS configuration, CI enrichment, and dedup.
- [x] Implement and test persistent signal deduplication.

## Repository verification

- [x] Use the released `github.com/sausheong/harness v0.4.2` module and run
      `go build ./...`.
- [x] Run `go test ./internal/loop/...`.
- [x] Run PostgreSQL integration tests with `SIDECAR_TEST_DB_URL`.
