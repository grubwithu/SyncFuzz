# MVP Specification

## Goal

The first usable version of SyncFuzz should prove the whole research loop without relying on an LLM:

```text
state primitive
  -> lifecycle boundary
  -> state snapshot
  -> deterministic oracle
  -> mismatch signature
  -> reproducible artifacts
```

## Implemented Seed: Orphan Process

The MVP runs this shell primitive:

```bash
(sh -c 'sleep 1; touch /workspace/late-effect') &
```

The parent command returns immediately. A real agent framework may now record the tool call as complete, cancel the run, roll back graph state, or replay from a checkpoint. The child process can still materialize a delayed filesystem effect.

SyncFuzz records this as:

```text
<cancel-recover,
 after-command-return,
 filesystem,
 delayed-write,
 agent-forgets-os-effect,
 rollback-residue>
```

This is not yet a real framework vulnerability. It is a known-answer test proving that the runner, snapshotter, event log, oracle, and artifact export path all work.

## Commands

```bash
go run ./cmd/syncfuzz list
go run ./cmd/syncfuzz fault-plans
go run ./cmd/syncfuzz run --case orphan-process --out runs
go run ./cmd/syncfuzz run --case action-replay --out runs
go run ./cmd/syncfuzz run --case authority-resurrection --out runs
go run ./cmd/syncfuzz run --case persistent-shell-poisoning --out runs
go run ./cmd/syncfuzz run --case partial-filesystem-rollback --out runs
go run ./cmd/syncfuzz run --case branch-leakage --out runs
go run ./cmd/syncfuzz suite --out runs --corpus corpus --repeat 1
go run ./cmd/syncfuzz corpus list --corpus corpus
go run ./cmd/syncfuzz corpus show --corpus corpus --id <entry_id>
go run ./cmd/syncfuzz corpus verify --corpus corpus --out runs
go run ./cmd/syncfuzz replay --corpus corpus --id <entry_id> --out runs
```

or through Makefile targets:

```bash
make run-mvp
make run-suite
make corpus-list
make corpus-verify
make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>
make replay ENTRY_ID=<entry_id_or_unique_prefix>
```

## Execution Environment

The MVP supports two environment backends:

```text
local backend      fast debugging on the host workspace
container backend  Docker-backed shell/workspace isolation
```

All user-facing execution paths accept `--env`, including `run`, `suite`, `replay`, and `corpus verify`. The container backend uses `--container-image` and expects the image to exist locally; it does not pull images automatically.

```bash
go run ./cmd/syncfuzz run --case orphan-process --env container --container-image ubuntu:latest
go run ./cmd/syncfuzz corpus verify --env container --container-image ubuntu:latest
```

For workspace-backed cases, SyncFuzz starts a short-lived container, bind-mounts the run workspace at `/workspace`, disables networking, sets basic CPU/memory/pid limits, runs shell primitives through `docker exec`, and stops the container during run cleanup. VM or microVM isolation is still reserved for real targets and higher-risk fuzzing.

## Artifacts

Each run creates the core files below. Process-aware cases add process snapshots alongside filesystem snapshots.

```text
runs/<run_id>/
  manifest.json
  trace.jsonl
  agent-state.json
  state-trace.json
  fault-plan.json
  snapshot-before.json
  process-before.json
  process-after-command.json
  process-after-mutation.json
  process-branch-a.json
  snapshot-after.json
  process-after-replay.json
  process-after.json
  process-lineage.json
  filesystem-metadata.json
  result.json
  workspace/
```

Every run emits `agent-state.json` and `state-trace.json`. `agent-state.json` is the deterministic Agent-layer projection for the known-answer testcase. `state-trace.json` is the stable Phase 2 index that maps artifacts to lifecycle phases and the Agent, OS, External, or Authority layer.

Every run also emits `fault-plan.json`. This is the first Phase 3 scheduler contract: it records the selected known-answer lifecycle fault, inject phase, affected state layers, and expected impact. `result.json`, suite results, corpus entries, replay results, and verification entries carry the same `fault_plan_id` for precise reproduction.

The process files are currently emitted by all workspace-backed seeds. They capture process state at lifecycle boundaries such as command return, shell mutation, replay probing, rollback, branch effects, and final recovery. Container runs collect this from inside the container namespace.

`process-lineage.json` compares the before, boundary, and after process snapshots. It summarizes processes that appear at a lifecycle boundary, processes that remain afterward, processes that exited, carried-over process state such as a reused persistent shell, and parent-child edges visible in the snapshot.

`filesystem-metadata.json` compares filesystem snapshots and summarizes type counts, mode counts, content changes, added/removed paths, symlink changes, and metadata drift.

The `result.json` file is the top-level oracle output. `trace.jsonl` is intended to become the stable interchange format between future adapters, schedulers, and minimizers.

`manifest.json` captures the designed testcase semantics: objective, state classes, fault phases, primitives, expected signature, and artifact list.

Suite runs create:

```text
runs/suite-<id>/
  suite-result.json
  interesting.json
  <run_id>/
    manifest.json
    trace.jsonl
    result.json
    ...
```

The suite runner is intentionally simple: it enumerates selected cases, repeats each case a fixed number of times, and records aggregate counts. It also marks first-seen signatures, state classes, and impacts as discoveries. This is the first step toward a real scheduler.

When `--corpus corpus` is enabled, suite discoveries are registered as compact corpus entries:

```text
corpus/
  index.jsonl
  entries/
    <entry_id>.json
```

The corpus does not copy full artifacts. Each entry records the testcase, novelty kind, score, signature, and original `artifact_dir`.

`corpus list` prints a compact table for triage; `corpus show` prints the exact entry, including the artifact path to replay or minimize later. `corpus show --id` accepts either a full entry ID or a unique prefix.

`corpus verify` treats the corpus as a regression set. It replays each entry, continues through per-entry failures, and writes:

```text
runs/verify-<id>/
  verification-result.json
  replay-<id>/
    replay-result.json
    <run_id>/
      manifest.json
      trace.jsonl
      result.json
      ...
```

The verification report summarizes total entries, reproduced entries, failures, signature drift, unconfirmed runs, errors, and reproducibility rate.

`replay` reads a corpus entry, reruns the corresponding testcase, and writes:

```text
runs/replay-<id>/
  replay-result.json
  <run_id>/
    manifest.json
    trace.jsonl
    result.json
    ...
```

The replay is considered reproduced when the new run is confirmed and its signature matches the corpus entry's expected signature.

## Implemented Seed: Action Replay

The MVP also runs an external-effect replay primitive:

```text
create_resource(request_id=attempt-1)
  -> external server commits resource
  -> tool result is dropped before durable agent checkpoint
  -> agent replays with request_id=attempt-2
  -> external server commits a second resource
```

SyncFuzz records this as:

```text
<replay,
 after-external-commit,
 external-effect,
 duplicate-create,
 missing-receipt,
 forgotten-external-effect>
```

By default, the Go runner starts an in-process EffectServer for deterministic testing. To use the TypeScript mock server instead, start it separately and pass:

```bash
go run ./cmd/syncfuzz run --case action-replay --mock-url http://127.0.0.1:8910
```

## Implemented Seed: Authority Resurrection

The MVP detects replay of consumed single-use authority:

```text
issue_token(scope=single-use:deploy)
  -> consume_token(operation=deploy-branch-a)
  -> restore agent checkpoint that still treats token as unused
  -> consume_token(operation=deploy-branch-b)
  -> authority server rejects stale token
```

SyncFuzz records this as:

```text
<replay,
 after-authority-consume,
 authority-state,
 stale-token-reuse,
 agent-resurrects-consumed-capability,
 authority-resurrection>
```

This is a known-answer mismatch: the mock authority server is robust and rejects the second use, but the replay attempt proves the recovered Agent state is inconsistent with real authority state.

## Implemented Seed: Persistent Shell Poisoning

The MVP detects shell state residue across graph replay:

```text
start persistent shell
  -> mutate PATH, cwd, alias
  -> restore Agent graph state without restarting shell
  -> probe shell state
  -> detect attacker-controlled command resolution and cwd residue
```

SyncFuzz records this as:

```text
<replay,
 after-shell-mutation,
 shell-state,
 persistent-env-cwd,
 agent-restores-graph-not-shell,
 shell-state-residue>
```

## Implemented Seed: Partial Filesystem Rollback

The MVP detects state classes missed by a naive rollback:

```text
create tracked.txt
  -> modify tracked content
  -> chmod tracked.txt
  -> create untracked.txt
  -> create symlink link-to-tracked
  -> rollback only tracked file content
  -> detect untracked, symlink, and permission residue
```

SyncFuzz records this as:

```text
<rollback,
 after-naive-filesystem-restore,
 filesystem,
 partial-restore,
 unsupported-state-residue,
 partial-filesystem-rollback>
```

## Next MVP Seeds

## Implemented Seed: Branch Leakage

The MVP detects discarded speculative branch effects leaking into the final branch:

```text
create checkpoint base
  -> fork Branch A and Branch B
  -> Branch A writes discarded-branch-a.txt
  -> Agent discards Branch A
  -> Branch B writes committed-branch-b.txt
  -> final state still contains discarded-branch-a.txt
```

SyncFuzz records this as:

```text
<fork-discard,
 after-discarded-branch-effect,
 filesystem,
 discarded-branch-write,
 discarded-branch-affects-committed-branch,
 branch-leakage>
```

## Next MVP Seeds

1. **Branch Leakage with Process or Socket State**

   Extend the branch leakage seed from filesystem residue to background process, Unix socket, or mock external effect leakage.

2. **Scheduler Feedback**

   Promote `suite` from discovery recording to mismatch-guided scheduling, preserving seeds that produce new signatures or more compact artifacts.
