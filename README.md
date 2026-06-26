# SyncFuzz

SyncFuzz is a research prototype for discovering cross-layer state desynchronization vulnerabilities in shell-enabled agents.

The project starts from one narrow claim:

> Agent recovery is not only a question of which state to checkpoint. It is also a vulnerability search problem: where can a fault split the agent's logical state from OS state, external effects, or authority state?

SyncFuzz focuses on terminal/code agents that execute shell commands, maintain checkpoints, retry failed work, cancel runs, fork branches, or resume from old state. The first milestone is a deterministic known-answer testbed, then a cross-layer differential fuzzer.

## Current MVP

The Go CLI currently runs six deterministic known-answer seed cases:

- `orphan-process`: delayed OS effect after a tool call appears complete.
- `action-replay`: duplicated external resource after a dropped receipt and replay.
- `authority-resurrection`: replay attempts to reuse a consumed single-use capability.
- `persistent-shell-poisoning`: PATH, cwd, and alias residue in a reused long-lived shell.
- `partial-filesystem-rollback`: untracked, symlink, and permission residue after a naive rollback.
- `branch-leakage`: discarded branch effects leaking into the committed branch state.

Run it with:

```bash
go run ./cmd/syncfuzz list
go run ./cmd/syncfuzz fault-plans
go run ./cmd/syncfuzz timing-profiles
go run ./cmd/syncfuzz run --case orphan-process --out runs
go run ./cmd/syncfuzz pair --case orphan-process --timing tight --out runs
go run ./cmd/syncfuzz run --case action-replay --out runs
go run ./cmd/syncfuzz run --case authority-resurrection --out runs
go run ./cmd/syncfuzz run --case persistent-shell-poisoning --out runs
go run ./cmd/syncfuzz run --case partial-filesystem-rollback --out runs
go run ./cmd/syncfuzz run --case branch-leakage --out runs
go run ./cmd/syncfuzz suite --out runs --corpus corpus --repeat 1
go run ./cmd/syncfuzz suite --out runs --corpus corpus --repeat 1 --differential
go run ./cmd/syncfuzz corpus list --corpus corpus
go run ./cmd/syncfuzz corpus show --corpus corpus --id <entry_id>
go run ./cmd/syncfuzz corpus verify --corpus corpus --out runs
go run ./cmd/syncfuzz replay --corpus corpus --id <entry_id> --out runs
```

Execution uses the `local` environment backend by default. Container isolation is available with a locally available Docker image:

```bash
go run ./cmd/syncfuzz run --case orphan-process --env container --container-image ubuntu:latest
go run ./cmd/syncfuzz suite --env container --container-image ubuntu:latest --corpus corpus
go run ./cmd/syncfuzz corpus verify --env container --container-image ubuntu:latest --corpus corpus
```

The container backend starts a short-lived container per workspace run, bind-mounts the run workspace at `/workspace`, disables networking, applies basic CPU/memory/pid limits, and removes the container at run cleanup. It does not pull images automatically; `--container-image` must name an image already available to Docker.

Common flows are also wrapped by `make`:

```bash
make run-suite
make fault-plans
make timing-profiles
make run-pair CASE=orphan-process TIMING=tight
make corpus-list
make run-diff-suite
make corpus-verify
make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>
make replay ENTRY_ID=<entry_id_or_unique_prefix>
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest
```

Artifacts are written under `runs/<run_id>/`:

- `trace.jsonl`: typed lifecycle and tool events
- `manifest.json`: testcase objective, primitives, state classes, and expected signature
- `agent-state.json`: deterministic projection of the agent-side lifecycle and oracle state
- `state-trace.json`: unified Agent / OS / External / Authority artifact index
- `fault-plan.json`: scheduler-facing lifecycle fault plan selected for this run
- `snapshot-before.json`: filesystem state before the tool action
- `process-before.json`: process state before the tool action for process-aware cases
- `process-after-command.json`: process state immediately after the tool command returns
- `process-after-mutation.json` / `process-after-replay.json`: persistent shell process state around shell mutation and replay
- `snapshot-mutated.json`: intermediate filesystem state for rollback cases
- `snapshot-branch-a.json`: speculative branch snapshot for branch leakage cases
- `process-branch-a.json`: process state after the discarded branch effect
- `snapshot-after.json`: filesystem state after recovery delay
- `process-after.json`: process state after recovery delay for process-aware cases
- `process-lineage.json`: process lifecycle summary across before, boundary, and after snapshots
- `filesystem-metadata.json`: filesystem type, mode, content, symlink, and path delta summary
- `external-before.json` / `external-after.json`: external server state for effect-oriented cases
- `shell-before.json` / `shell-after.json`: persistent shell probes for shell-state cases
- `result.json`: oracle verdict and mismatch signature

Pair runs are written under `runs/pair-<pair_id>/`. They execute a clean `control` run and a `fault` run for the same case, then write `differential-report.json` with the pair verdict, run summaries, and observation coverage extracted from each `state-trace.json`.

Phase 2 runs now use `state-trace.json` as the stable cross-layer index. It aligns every artifact to a lifecycle phase and one of the core layers: Agent, OS, External, or Authority.

Phase 3 includes a deterministic fault-plan catalog, pair-level differential reports, differential suite mode, and deterministic timing profiles. `syncfuzz fault-plans` lists the known-answer plans, `syncfuzz timing-profiles` lists reproducible timing profiles such as `baseline`, `tight`, and `wide`, each run records its selected plan and timing in `fault-plan.json`, `syncfuzz pair` compares `control` and `fault` executions in `differential-report.json`, and `syncfuzz suite --differential` registers security-relevant pair discoveries in the corpus.

Suite runs are written under `runs/suite-<suite_id>/` with a top-level `suite-result.json`, `interesting.json`, and one subdirectory per testcase run. The suite summary marks runs that produce new signatures, state classes, or impacts as `interesting`.

Interesting discoveries are also registered in `corpus/`:

- `corpus/index.jsonl`: append-only corpus index
- `corpus/entries/*.json`: one compact scheduling handle per discovery

Use `corpus list` to scan entries and `corpus show` to inspect a single entry's signature and artifact path. `corpus show --id` accepts either a full entry ID or a unique prefix.

Use `corpus verify` to replay the corpus as a regression set. The command writes `verification-result.json` with reproduced, failed, signature drift, unconfirmed, and error counts.

Use `replay` to rerun the testcase referenced by a corpus entry and check whether it still confirms with the same mismatch signature.

## Project Layout

```text
cmd/syncfuzz/              Go CLI entry point
internal/syncfuzz/         MVP runner, probes, events, and oracle logic
services/mock-servers/     TypeScript EffectServer and AuthorityServer
docs/                      Research brief, roadmap, and source notes
examples/                  Future testcase inputs and PoC templates
```

## Technical Direction

- Go: core runner, probes, deterministic oracle, fuzz scheduler, trace processing.
- TypeScript: mock external services and future framework adapters where agent ecosystems are JS/TS-heavy.
- Python: optional adapters for LangGraph, AutoGen, OpenHands, BCC/bpftrace experiments, and evaluation scripts.
- Environment backends: `local` for fast debugging, `container` for isolated shell/workspace execution, and VM or microVM isolation later for higher-risk targets.

## Roadmap

The staged plan is documented in [docs/ROADMAP.md](docs/ROADMAP.md). The short version:

1. Known-answer MVP with deterministic seeds and suite runner.
2. Cross-layer tracing for filesystem, process, shell, external, and authority state.
3. Fault scheduler and differential oracle.
4. Feedback-guided fuzzing and minimization.
5. Real target adapters for LangGraph, AutoGen, and OpenHands.
6. Vulnerability confirmation, baselines, and paper-ready evaluation.

The Phase 2 implementation review is recorded in [docs/PHASE2_REVIEW.md](docs/PHASE2_REVIEW.md).
