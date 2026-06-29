DO NOT send optional commentary

# Repository Guidelines

## Project Structure

SyncFuzz is a Go-first research prototype for finding cross-layer state desynchronization bugs in shell-enabled agents.

- `cmd/syncfuzz/`: CLI entry point.
- `internal/syncfuzz/`: runner, deterministic seeds, oracles, environment backends, corpus/replay/verify logic, and tests.
- `services/mock-servers/`: TypeScript mock EffectServer and AuthorityServer.
- `docs/`: project brief, MVP spec, roadmap, and research notes.
- `examples/`: future PoC and testcase examples.
- `runs/` and `corpus/`: generated artifacts, intentionally ignored by Git.

## Common Commands

Use the Makefile wrappers unless a lower-level command is needed:

```bash
make test-go
make run-suite
make corpus-list
make corpus-verify
make run-case CASE=orphan-process
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest
```

`make test-go` sets `GOCACHE=/tmp/syncfuzz-go-cache`, which avoids host cache permission issues in restricted environments.

## Coding Style

Keep Go code formatted with `gofmt`:

```bash
make fmt-go
```

Prefer small, explicit structs and deterministic testcases. Keep seed semantics, oracle signatures, and artifact names stable unless the docs and tests are updated together.

## Testing And Regression

Before handing off changes, run:

```bash
make test-go
make corpus-verify
```

For container-specific work, also run a focused container case if Docker is available:

```bash
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest
```

The container backend requires a local Docker image and should not pull images implicitly.

## Artifact Discipline

Do not commit generated `runs/`, `corpus/`, logs, caches, `node_modules/`, or build outputs. Commit source, tests, docs, Makefile changes, and mock-server source only.

When adding new artifacts, update `README.md`, `docs/MVP_SPEC.md`, and relevant tests in the same change.
