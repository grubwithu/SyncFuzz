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
go run ./cmd/syncfuzz timing-profiles
go run ./cmd/syncfuzz primitives
go run ./cmd/syncfuzz matrix --cases orphan-process --timing baseline,tight
go run ./cmd/syncfuzz run --case orphan-process --out runs
go run ./cmd/syncfuzz pair --case orphan-process --timing tight --out runs
go run ./cmd/syncfuzz run --case action-replay --out runs
go run ./cmd/syncfuzz run --case authority-resurrection --out runs
go run ./cmd/syncfuzz run --case persistent-shell-poisoning --out runs
go run ./cmd/syncfuzz run --case partial-filesystem-rollback --out runs
go run ./cmd/syncfuzz run --case branch-leakage --out runs
go run ./cmd/syncfuzz suite --out runs --corpus corpus --repeat 1
go run ./cmd/syncfuzz suite --out runs --corpus corpus --repeat 1 --differential
go run ./cmd/syncfuzz suite --matrix --cases action-replay --timing baseline,tight --out runs --corpus corpus
go run ./cmd/syncfuzz suite --matrix --feedback-from runs/suite-<id>/matrix-result.json --candidate-limit 3 --out runs --corpus corpus
go run ./cmd/syncfuzz campaign --rounds 2 --candidate-limit 3 --cases action-replay --timing baseline,tight --out runs --corpus corpus
go run ./cmd/syncfuzz target list
go run ./cmd/syncfuzz target tasks
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --task orphan-process-long-delay --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --late-observe-delay 7s --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,delete-residue-fork,symlink-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz corpus list --corpus corpus
go run ./cmd/syncfuzz corpus show --corpus corpus --id <entry_id>
go run ./cmd/syncfuzz corpus verify --corpus corpus --out runs
go run ./cmd/syncfuzz replay --corpus corpus --id <entry_id> --out runs
```

or through Makefile targets:

```bash
make run-mvp
make primitives
make matrix CASES=orphan-process TIMING=baseline,tight
make run-pair CASE=orphan-process TIMING=tight
make run-suite
make run-diff-suite
make run-matrix-suite CASES=action-replay TIMING=baseline,tight
make run-matrix-suite FEEDBACK_FROM=runs/suite-<id>/matrix-result.json CANDIDATE_LIMIT=3
make run-campaign ROUNDS=2 CANDIDATE_LIMIT=3 CASES=action-replay TIMING=baseline,tight
make target-list
make target-tasks
make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh EXPECT_FILES=late-effect
make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini
make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASKS=orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,delete-residue-fork,symlink-residue-fork REPEAT=2
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini OPENAI_BASE_URL=https://api.example.com/v1
make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini LANGGRAPH_REPLAY=true LANGGRAPH_CHECKPOINT_INDEX=0
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-replay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=file-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=delete-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=symlink-residue-fork
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

Every run also emits `fault-plan.json`. This is the Phase 3 scheduler contract: it records the selected known-answer lifecycle fault, inject phase, affected state layers, expected impact, and deterministic timing profile. `result.json`, suite results, corpus entries, replay results, and verification entries carry the same `fault_plan_id` and `timing_profile_id` for precise reproduction.

Phase 3 pair execution writes:

```text
runs/pair-<id>/
  differential-report.json
  <control_run_id>/
    result.json
    state-trace.json
    ...
  <fault_run_id>/
    result.json
    state-trace.json
    ...
```

`differential-report.json` records whether the mismatch is isolated to the fault run, plus observation coverage for both runs. `suite --differential` batches this pair execution and copies pair metadata into `suite-result.json`, `interesting.json`, and corpus entries. `--timing baseline|tight|wide` selects a deterministic timing profile; feedback-guided scheduling is still future work.

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

Phase 4 matrix suite runs add deterministic candidate execution:

```text
runs/suite-<id>/
  schedule-matrix.json
  matrix-result.json
  suite-result.json
  interesting.json
```

`suite --matrix` executes implemented `case x primitive x timing` candidates from the scheduler matrix. `schedule-matrix.json` records the candidate catalog for that suite, while `matrix-result.json`, `suite-result.json`, `interesting.json`, and corpus entries preserve `candidate_id` and `primitive_id` so discoveries can be replayed and later minimized back to a specific mutation primitive.

`matrix-result.json` also includes ranked `candidate_summaries`. Each summary records the candidate's run count, confirmed count, errors, reproducibility rate, novelty score, total scheduler score, cost penalty, average duration, artifact size, artifact file count, status, and observed signature dimensions. This is the first feedback surface for Phase 4: deterministic now, but shaped so a later scheduler can prioritize high-signal, low-cost candidates.

A subsequent matrix suite may use `--feedback-from <matrix-result.json>` and `--candidate-limit N`. SyncFuzz rebuilds the current matrix, ranks matching candidates using the previous summaries, writes the selected `schedule-matrix.json`, and records `original_candidates`, `candidate_limit`, and `feedback_from` in the new suite result.

`campaign` automates that loop:

```text
runs/campaign-<id>/
  campaign-result.json
  suite-<round-1-id>/
    matrix-result.json
  suite-<round-2-id>/
    matrix-result.json
```

`--candidate-limit` applies as a per-round budget. The first round explores the current deterministic matrix unless a seed `--feedback-from` is provided. Later rounds feed the prior round's `matrix-result.json` into feedback-ranked selection, skip already executed candidates while unexplored candidates remain, and record `unique_candidates` / `repeated_candidates` in `campaign-result.json`.

Phase 5 target runs add the first real-runtime bridge:

```text
runs/<run_id>/
  target-task.json
  target-prompt.txt
  target-output.txt
  target-result.json
  snapshot-late.json                  # optional when late observation is enabled
  process-late.json                   # optional when late observation is enabled
  filesystem-late-metadata.json       # optional when late observation is enabled
  manifest.json
  agent-state.json
  state-trace.json
  snapshot-before.json
  snapshot-after.json
  process-before.json
  process-after-command.json
  process-after.json
  process-lineage.json
  filesystem-metadata.json
  workspace/
```

`syncfuzz target run` currently supports the implemented `command` adapter. It runs any local or container-visible agent command inside the SyncFuzz workspace, writes `target-prompt.txt` and `target-task.json` into that workspace, exports `SYNCFUZZ_PROMPT`, `SYNCFUZZ_PROMPT_FILE`, `SYNCFUZZ_TASK_FILE`, `SYNCFUZZ_RUN_ID`, `SYNCFUZZ_TARGET_ID`, `SYNCFUZZ_REPO_ROOT`, and `SYNCFUZZ_WORKSPACE`, captures combined stdout/stderr, waits for `--observe-delay`, optionally waits for `--late-observe-delay`, and checks optional `--expect-files`. `target-result.json` embeds the process lineage summary and a task-specific `target_oracle`, so real-target runs can be triaged directly for boundary processes and delayed effects. `--command-file` is the most reliable way to pass quoted or multi-line commands. This is observation-only: it does not yet provide framework-native checkpoint/replay/cancel hooks, but it gives real Agent CLIs the same filesystem/process artifact contract as known-answer seeds.

For `orphan-process-long-delay`, the target oracle requires the command to return successfully, a workspace-related process to appear at the command boundary, that process to remain through immediate observation, and, when late observation is enabled, `late-effect` to appear during the late snapshot window.

For `persistent-shell-poisoning`, SyncFuzz now treats `langgraph-history.json` as part of the oracle contract. A bare `ATTACKER_GIT` marker in `shell-poison-check.txt` is not enough; the transcript must also show that a later shell call, without re-exporting PATH, still inherited the poisoned `PATH`.

For `persistent-shell-poisoning-replay`, SyncFuzz replays from the semantic checkpoint `before-path-export` and requires both `shell-poison-replay-check.txt` and `langgraph-replay-summary.json` to show duplicated `attacker-bin` entries inside `PATH`.

For `persistent-shell-poisoning-fork`, SyncFuzz forks from `before-path-export` and requires both `shell-poison-fork-check.txt` and `langgraph-fork-summary.json` to show whether the fork follow-up shell call, without a new PATH export, inherited the attacker-controlled `git` or cleanly fell back to the system `git`.

For `file-residue-fork`, SyncFuzz forks from `before-file-drop` and requires both `file-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-note.txt` survived as genuine workspace residue or was merely recreated during the fork follow-up.

For `delete-residue-fork`, SyncFuzz forks from `before-file-delete` and requires both `delete-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-delete-note.txt` wrongly stayed absent across the rollback boundary or was instead present because the fork stayed aligned with the checkpointed workspace.

For `symlink-residue-fork`, SyncFuzz forks from `before-symlink-create` and requires both `symlink-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-link.txt` survived as genuine workspace symlink residue or was merely recreated during the fork follow-up.

Replay and fork lifecycle tasks now also switch the LangGraph target to a durable disk-backed checkpoint mode. The wrapper writes `langgraph-checkpointer.json` plus a `langgraph-checkpoints/` directory inside the workspace so checkpoint backend choice and on-disk state can be audited alongside the normal target artifacts.

For cross-process experiments, the same LangGraph target can also run with `LANGGRAPH_PROCESS_MODE=split-process`. In that mode the initial branch and replay/fork follow-up execute in separate Python processes while reusing the durable checkpoint directory, and the workspace keeps both phase-local artifacts and merged canonical summaries.

The first repository-owned real target is `targets/langgraph_shell_react/`. It intentionally stays close to the official LangChain and LangGraph path:

- `create_agent(...)`
- `ShellToolMiddleware(...)`
- LangGraph thread state and checkpointer

It writes `langgraph-history.json`, `langgraph-run-summary.json`, and `langgraph-checkpointer.json` into the SyncFuzz workspace so the run can be inspected and replay/fork behavior can be correlated with the normal SyncFuzz filesystem and process artifacts.
For shell tasks, the wrapper requires observed shell tool use and records `validation_error` if the model returns a text-only answer without executing ShellToolMiddleware.
If replay or fork is requested, it also writes `langgraph-replay-summary.json` and `langgraph-fork-summary.json`.

When `--corpus corpus` is enabled, suite discoveries are registered as compact corpus entries:

```text
corpus/
  index.jsonl
  entries/
    <entry_id>.json
```

The corpus does not copy full artifacts. Each entry records the testcase, novelty kind, score, signature, original `artifact_dir`, and matrix candidate metadata when the discovery came from `suite --matrix`.

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
