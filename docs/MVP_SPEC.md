# MVP Specification

> **路线状态（2026-07-23）**：本文件描述已完成的 deterministic MVP 与其 artifact contract。后续研究开发遵循 [RESEARCH_PLAN.md](RESEARCH_PLAN.md)；尤其不再将当前 mutation matrix 视为自动发现新 Query 的机制。V2.1a 已新增离线 profiling evidence contract；V2.2 的真实 eBPF/container collector 已完成 process、resource、FD identity 与 Unix-socket closure calibration。

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

## Recovery Relation Artifacts

State synthesis validation, recovery-relation classification, and contract
judgment are separate stages:

```text
Effect Validation -> StateSeed -> RecoveryRelationReport -> Contract Triage
```

`RecoveryRelationReport` records per before/after/head control: evidence
status, logical effect phase, OS presence, resource origin, multiplicity,
activity when an adapter can prove it, and a normalized relation class. The
initial classes are `aligned`, `uncommitted-original-residual`,
`missing-committed-effect`, `reconstruction`, `duplicate`, and `unknown`.
`unknown` means evidence is incomplete; `contract_status=not-evaluated` means
the relation is known but has not been compared with a framework contract.
Neither state is a vulnerability verdict.

The LangGraph target now emits `durable_tool_lifecycle` for every new native
checkpoint: complete tool-call IDs/names plus tool-result IDs from persisted
message history. The native binding and fork plan retain those snapshots, with
an absent snapshot reserved for legacy artifacts and an explicit empty snapshot
meaning no complete tool identity was durable. New lifecycle events also carry
`CLOCK_MONOTONIC` timestamps. A binding writes `tool_effect_provenance` only
when exactly one finished shell-command span fully contains the linked eBPF
effect interval and the selected after checkpoint contains that call/result;
missing timestamps, missing durable result, or multiple spans are unknown.
When `recovery execute --out-relation` or `recovery classify-relation` reads a
new LangGraph fork plan, it copies this result into
`causal_effect_evidence`: `proven` carries the immutable tool-effect proof;
legacy, missing, or ambiguous plan evidence is explicitly `unknown`. This is
adapter evidence for later relation-novelty work, not an Oracle. It does not
change the relation signature, logical phase, or `contract_status`, so the
adapter still derives only `effect-not-committed` / `effect-committed`, not
`PRE_CALL`, `CALL_DURABLE`, or `RESULT_DURABLE`. Future relation coverage will
use complete normalized signatures rather than the legacy aggregate outcome
field. Its current `seed_resource_ids` are the StateSeed's validated frontier
scope, not yet a per-effect ResourceGraph edge set; exact effect/resource graph
coverage remains future adapter work.

## V2.1a Profiling Evidence

`syncfuzz profile analyze` is the offline, deterministic half of the new profiling pipeline. It consumes a target-produced checkpoint catalog, collector-produced raw-event JSONL, and probe-produced checkpoint state summaries. It writes normalized OS effects and a checkpoint-effect map; an interval becomes a frontier only when both event evidence and a confirmed persistent state delta are present.

```bash
go run ./cmd/syncfuzz profile analyze \
  --checkpoints examples/profiling/unix-listener-checkpoints.example.json \
  --events examples/profiling/unix-listener-events.example.jsonl \
  --summaries examples/profiling/unix-listener-summaries.example.json \
  --out runs/profile-example
```

The current command is intentionally offline. The V2.2 host-side eBPF collector will produce its raw-event input while filtering by the per-run container cgroup; it must fail explicitly when the required BPF privilege or cgroup-v2 scope is unavailable.

The first V2.2 collector is available for an isolated smoke test. It records cgroup-filtered process `fork` / `exec` / `exit` events on the host; it does not run inside the Agent container.

```bash
GOCACHE=/tmp/syncfuzz-go-cache go build -o /tmp/syncfuzz-ebpf ./cmd/syncfuzz
docker run -d --rm --name syncfuzz-ebpf-probe --network none ubuntu:latest sleep infinity
/tmp/syncfuzz-ebpf profile container-scope --container syncfuzz-ebpf-probe
# Use the reported cgroup_id, then start this command before docker exec work.
sudo /tmp/syncfuzz-ebpf profile process-monitor --cgroup-id <cgroup_id> --duration 10s --out raw-os-events.jsonl
```

The collector requires Linux cgroup v2 and `CAP_BPF`/`CAP_PERFMON` (or root). The container remains unprivileged; when the controller is invoked through `sudo`, it preserves the original caller UID/GID for the container user.

For a real target command, add `--profile-processes`; SyncFuzz records controller observation checkpoints with `CLOCK_MONOTONIC` after the pre-command snapshot, after command return, and after the immediate observation. It starts the collector after the pre-command snapshot and stops it as soon as the command returns, avoiding observer-process noise in the lifecycle trace. Add `--profile-resources` to also collect successful cgroup-scoped filesystem, FD, and IPC syscalls on Linux/amd64.

```bash
sudo /tmp/syncfuzz-ebpf target run \
  --env container \
  --profile-processes \
  --profile-resources \
  --command 'sh -c "sleep 1 &"' \
  --out runs
```

The resulting target artifact directory contains `ebpf-process-scope.json`, `ebpf-process-events.jsonl`, `ebpf-resource-scope.json`, `ebpf-resource-events.jsonl`, `checkpoint-catalog.json`, `checkpoint-state-summaries.json`, `normalized-effects.json`, and `checkpoint-effect-map.json`. The resource collector records only successful selected syscalls and bounded path/FD facts. For target commands run from `/workspace`, SyncFuzz canonicalizes relative resource paths against that root and emits `evidence_links` only for an exact canonical-path, exact-path, or exact `(device,inode)` match to an added/removed probe resource. The latter is best-effort: eBPF events resolve a still-live host FD through procfs, while process snapshots resolve workspace-held FDs in the target namespace; both fields must be present, so a short-lived FD does not become a synthetic match. This retains a deleted-but-open file's identity despite its changed pathname. A target interval becomes a frontier only when such a link exists; unrelated loader or shell-initialization events in the same interval are insufficient. For a workspace-bound Unix socket, the probe separately emits the namespace pathname, kernel socket ID, holder FD, and holder process, with explicit `bound-at-path`, `references-unix-socket`, and `held-by-process` dependency edges. IPC effects link only through the exact socket ID. These are controller observation checkpoints, not yet framework-native durable Agent checkpoints.

`make ebpf-fd-identity-smoke` is the focused calibration: it writes a workspace file, opens it as FD 9 in a background process, unlinks it, and keeps that FD alive through the immediate checkpoint. A successful calibration has a deleted `container-fd` resource with nonzero `device` and `inode`, an `openat` event with the same pair, and an `exact-device-inode` entry in `checkpoint-effect-map.json`.

`make ebpf-unix-socket-smoke` is the IPC calibration. It starts a workspace-bound Unix listener and leaves it alive through the immediate checkpoint. The probe must emit the endpoint's bound pathname, kernel `socket_id`, holder FD, holder process, and their explicit dependency edges. A successful privileged run also links the eBPF `bind` and/or `listen` IPC effects to the endpoint through `exact-socket-id`.

The privileged calibration run `1784805732832067342` satisfies that contract: its cgroup-scoped resource trace records `bind` and `listen` with `socket:177721907`, while the container checkpoint records the same socket ID on a listener endpoint, FD 3, and its Perl holder. The frontier map includes two `exact-socket-id` links and the three closure edges. Its negative target oracle is expected: this command is a collector calibration fixture, not an `orphan-process` violation testcase.

Audit the three completed known-answer runs without BPF privileges:

```bash
make ebpf-calibration-audit \
  CALIBRATION_PATH_RUN=runs/<canonical-path-run-id> \
  CALIBRATION_FD_RUN=runs/<deleted-fd-run-id> \
  CALIBRATION_SOCKET_RUN=runs/<unix-socket-run-id>
```

The report states fixture-scoped precision/recall only. It is an audit of the declared known-answer links, not a global detector-quality or coverage result.

After a real profiling run, V2.1b can promote a seed directly from its artifact directory:

```bash
go run ./cmd/syncfuzz profile promote-seed \
  --objective examples/objectives/unix-listener-survival.example.json \
  --target-run runs/<run_id> \
  --profile-kind synthesis-candidate \
  --synthesis-candidate runs/<candidate>.json \
  --frontier before-command..after-command \
  --out runs/<run_id>/state-seed.json
go run ./cmd/syncfuzz profile recovery-pair \
  --objective examples/objectives/unix-listener-survival.example.json \
  --seed runs/<run_id>/state-seed.json \
  --passive-observation passive-unix-listener-response \
  --out runs/<run_id>/recovery-pair.json
go run ./cmd/syncfuzz profile recovery-set \
  --objective examples/objectives/unix-listener-survival.example.json \
  --seed runs/<run_id>/state-seed.json \
  --passive-observation passive-unix-listener-response \
  --retention-policy retain-relevant-os-state \
  --out runs/<run_id>/historical-recovery-set.json
```

Promotion rejects an incomplete command, an unprofiled run, a calibration fixture, unlinked effects, non-persistent deltas, an objective atom absent from the selected frontier, or linked resources absent at the terminal materialization-head checkpoint. `synthesis-candidate` is reserved for V2.4 scheduler output: its `SynthesisCandidateID` is mandatory, and a hand-authored smoke or calibration run must use `calibration-fixture` and cannot create a StateSeed. `synthesis schedule`, `generate`, `execute-langgraph`, `evaluate`, and `promote` provide the objective-only scheduler, strict generator command contract, real candidate execution, execution-derived feedback, and retention gate. `execute-langgraph` requires the repository’s dedicated image, process/resource eBPF privileges, and explicit network permission for the model provider; it forwards provider credentials only as process environment and does not persist them. Its `langgraph-native-checkpoints.json` proves the disk-backed native checkpoint namespace and records the `persisted_monotonic_ns` of each exact checkpoint. `synthesis bind-langgraph-frontier` accepts only a matching native runtime and selects the closest native checkpoints strictly before and after the linked objective-effect window; manifest history order or controller checkpoint names alone are rejected. `synthesis prepare-langgraph-fork` also captures an immutable source-workspace digest, disk-checkpointer digest, exact native IDs, and a distinct post-frontier materialization-head coordinate. The resulting bound profile must be promoted to `StateSeed`, so the seed points at the V3 plan rather than the original target task. `profile recovery-set` writes the explicit `MaterializationHead`, `retain-relevant-os-state` policy, and `Q_before/Q_after/Q_head` controls; `recovery execute --set` verifies the source snapshot, clones its durable store, bind-mounts the retained Unix socket read-only, and restores the exact source checkpoint ID in each fresh container. It marks a non-consistent head control `inconclusive`. Historical V2 artifacts lack the retained runtime, snapshot and listener-holder evidence required by this executor and must be freshly profiled and prepared as V3. `synthesis bind-maf-frontier` currently has only before/after native queue coordinates, so it remains pair-only until its adapter gains a distinct head binding. No built-in LLM generator is selected. Pair construction reuses the seed's recorded plan artifact; it cannot substitute a different plan artifact.

The V2.3 recovery core requires an adapter to expose a real durable checkpoint recovery mechanism. The method normalizes that mechanism as a historical cut `C < H` under an explicit OS-retention policy; `fork`, `rewind`, and `replay` are only comparable when they implement the same retention / re-execution semantics. `HistoricalRecoverySet` records `Q_before`, `Q_after`, and `Q_head` with one materialization head, plan, passive observation, and retention policy. Its executor requires independent runtime instances for all three controls and suppresses a finding when `Q_head` is not `consistent`. The older `RecoveryPair` executor remains for compatibility fixtures. The first registered adapter is `maf-workflow`: its adapter plan currently maps only before/after V2 coordinates to exact `FileCheckpointStorage` IDs, so it remains pair-only pending a distinct head binding. The generic command adapter is deliberately ineligible: its controller checkpoints are profiling observations, not durable Agent recovery points.

Update, 2026-07-24: the LangGraph listener path now requires
`execute-langgraph --retain-runtime`. Its V3 fork plan records the immutable
source container lease plus eBPF-linked kernel socket ID and holder FD.
Recovery joins that source PID/network namespace and reads `/proc/net/unix`
and `/proc/<pid>/fd`; it does not classify a bare socket filesystem node as a
live listener. Fork-plan preparation rejects a materialization head with more
than one surviving eBPF-linked listener endpoint rather than selecting the
latest bind. The metadata-only V2 calibration is consequently retained only
as an `inconclusive` baseline. `synthesis release-langgraph-runtime` removes
the lease after recovery. The LangGraph V3 full/pruned observer pair records
structured post-query probe metrics in every recovery observation. Its batch
wrapper repeats independent same-source pairs and writes `fidelity-report.json`;
`LANGGRAPH_V3_FIDELITY_REPEAT` names accepted pairs while
`LANGGRAPH_V3_FIDELITY_MAX_ATTEMPTS` bounds provider invocations. Each attempt
is retained as `accepted`, `rejected-source-baseline`, or `execution-failed` in
the report denominator; only accepted pairs are aggregated. The report rejects
any pair whose recorded plan, retained source identity, workspace/checkpoint
snapshot, listener identity, native coordinates, or causal-effect evidence
differ. Its aggregate separately counts `proven` and `unknown` causal evidence
among accepted trials; per-run tool-call IDs and command hashes are audit data,
not coverage keys. Pruned samples can report exact holder identity but remain
multiplicity `unknown`, so they cannot independently emit a `residual` verdict.

`synthesis generate --feedback <evaluation.json>` imports only bounded
atom-level `CandidateEvaluation` feedback from a prior profiled candidate. It
rejects malformed or cross-objective evaluation artifacts. The
`synthesis-langgraph-statefuzz-attempt` Make target wires one external-generator
attempt through generate, privileged profile, evaluation, native binding,
StateSeed promotion, and before/after/head recovery; a manual baseline remains
outside this path and cannot count as generated coverage.

`synthesis statefuzz-batch-report` scans the generated `attempt-*` roots after
the run. It retains every root in the experimental denominator, including
retention rejections, rejected source baselines, execution failures, and roots
whose candidate/profile/seed/recovery lineage is invalid. Only an audited,
linked recovery execution contributes a recovery outcome count.

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
go run ./cmd/syncfuzz target seeds
go run ./cmd/syncfuzz target scenarios
go run ./cmd/syncfuzz target prompt-profiles
go run ./cmd/syncfuzz target matrix --target langgraph-shell-react --group phase5a-baseline --prompt-profiles all
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --task orphan-process-long-delay --prompt-profile workflow --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --late-observe-delay 7s --out runs
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork,open-fd-residue-fork,deleted-open-fd-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork,discarded-server-trusted-client,socket-response-poisoning,cwd-residue-fork,umask-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --matrix --candidate-limit 3 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target campaign --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --candidate-limit 3 --rounds 2 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz corpus list --corpus corpus
go run ./cmd/syncfuzz corpus analyze --corpus corpus
go run ./cmd/syncfuzz corpus analyze --corpus corpus --verification runs/verify-<id>/verification-result.json
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
make target-scenarios
make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh EXPECT_FILES=late-effect
make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3
make target-langgraph-shell-react
make target-langgraph-shell-react-suite TARGET_TASKS=orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork,open-fd-residue-fork,deleted-open-fd-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork,discarded-server-trusted-client,socket-response-poisoning,cwd-residue-fork,umask-residue-fork REPEAT=2
make target-langgraph-shell-react OPENAI_BASE_URL=https://api.example.com/v1
make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay
make target-langgraph-shell-react LANGGRAPH_REPLAY=true LANGGRAPH_CHECKPOINT_INDEX=0
make target-langgraph-shell-react TARGET_TASK=persistent-shell-poisoning
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-replay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=file-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=directory-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=delete-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=symlink-residue-fork
make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning
make target-maf-github-copilot-shell TARGET_TASK=env-residue
make target-maf-github-copilot-shell TARGET_TASK=function-residue
make target-maf-github-copilot-shell TARGET_TASK=cwd-residue
make target-maf-github-copilot-shell TARGET_TASK=umask-residue
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-shell-context REPEAT=1
make corpus-list
make corpus-analyze
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

The process files are currently emitted by all workspace-backed seeds. They capture process state at lifecycle boundaries such as command return, shell mutation, replay probing, rollback, branch effects, and final recovery. Local runs now also preserve workspace-related open file descriptor targets inside each process entry, and the synthetic `partial-filesystem-rollback/open-fd` primitive uses that probe to confirm deleted workspace inode residue after rollback. Container runs still focus on process/cwd lineage first and can grow richer FD capture later.

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

`syncfuzz target run` currently supports the implemented `command` adapter. It runs any local or container-visible agent command inside the SyncFuzz workspace, writes `target-prompt.txt` and `target-task.json` into that workspace, exports `SYNCFUZZ_PROMPT`, `SYNCFUZZ_PROMPT_FILE`, `SYNCFUZZ_TASK_FILE`, `SYNCFUZZ_RUN_ID`, `SYNCFUZZ_TARGET_ID`, `SYNCFUZZ_REPO_ROOT`, and `SYNCFUZZ_WORKSPACE`, captures combined stdout/stderr, waits for `--observe-delay`, optionally waits for `--late-observe-delay`, and checks optional `--expect-files`. `target-task.json` now also carries built-in executable Scenario IR metadata when the task is repository-owned: seed id, plant primitive, activation kind, oracle kind, mutation operators, and the lifecycle execution plan used to derive replay/fork runtime overrides. `target-result.json` embeds the process lineage summary, a task-specific `target_oracle`, and a separate `task_compliance` verdict, so real-target runs can be triaged for both boundary residue and prompt/task drift. `--command-file` is the most reliable way to pass quoted or multi-line commands. This is observation-only: it does not yet provide framework-native checkpoint/replay/cancel hooks, but it gives real Agent CLIs the same filesystem/process artifact contract as known-answer seeds.

When a target ships with a built-in contract profile, the same run now also writes `target-contract-profile.json` into the run artifact directory and adds `contract_interpretation` to `target-result.json`. This lets SyncFuzz distinguish three layers in a real-target result: raw residue evidence (`target_oracle`), prompt/task drift (`task_compliance`), and the current lifecycle-contract reading (`contract_interpretation`).

For `orphan-process-long-delay`, the target oracle requires the command to return successfully, a workspace-related process to appear at the command boundary, that process to remain through immediate observation, and, when late observation is enabled, `late-effect` to appear during the late snapshot window.

For `persistent-shell-poisoning`, SyncFuzz now treats `langgraph-history.json` as part of the oracle contract. A bare workspace-local shim marker in `shell-poison-check.txt` is not enough; the transcript must also show that a later shell call, without re-exporting PATH, still inherited the earlier PATH override.

For `persistent-shell-poisoning-replay`, SyncFuzz replays from the semantic checkpoint `before-path-export` and requires both `shell-poison-replay-check.txt` and `langgraph-replay-summary.json` to show duplicated workspace-local tool-directory entries inside `PATH`.

For `persistent-shell-poisoning-fork`, SyncFuzz forks from `before-path-export` and requires both `shell-poison-fork-check.txt` and `langgraph-fork-summary.json` to show whether the fork follow-up shell call, without a new PATH export, inherited the workspace-local `git` shim or cleanly fell back to the system `git`.

For `file-residue-fork`, SyncFuzz forks from `before-file-drop` and requires both `file-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-note.txt` survived as genuine workspace residue or was merely recreated during the fork follow-up.

For `directory-residue-fork`, SyncFuzz forks from `before-directory-create` and requires both `directory-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-dir` survived as genuine workspace residue or was merely recreated during the fork follow-up.

For `delete-residue-fork`, SyncFuzz forks from `before-file-delete` and requires both `delete-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-delete-note.txt` wrongly stayed absent across the rollback boundary or was instead present because the fork stayed aligned with the checkpointed workspace.

For `symlink-residue-fork`, SyncFuzz forks from `before-symlink-create` and requires both `symlink-residue-fork-check.txt` and `langgraph-fork-summary.json` to show whether `branch-link.txt` survived as genuine workspace symlink residue or was merely recreated during the fork follow-up.

For real-target runs, `target-result.json` now distinguishes `target_oracle.status=confirmed`, `negative`, and `inconclusive`. The legacy `confirmed` boolean remains backward-compatible and is only true for the `confirmed` status; `negative` captures clean or otherwise non-vulnerable outcomes, while `inconclusive` captures partial evidence that still needs stronger attribution.

`task_compliance.status` is separate: `compliant` means the target followed the built-in task contract closely enough for interpretation, `violated` means it drifted from that contract, `unknown` means SyncFuzz lacked enough structure to judge, and `not-applicable` means no compliance checker exists for that task. The current compliance coverage includes `orphan-process-long-delay`, persistent-shell baseline/replay/fork, and the built-in workspace residue fork tasks. `target-suite-result.json` now aggregates these as `compliance_summaries` both globally and per task.

Real-target exploration now has its own candidate scheduler. `syncfuzz target matrix` still enumerates repository-owned target tasks, but each candidate now also carries executable Scenario IR metadata: `scenario_id`, `seed_id`, `plant_primitive_id`, `lifecycle_operation_id`, `activation_kind_id`, `oracle_kind_id`, and `mutations`. `syncfuzz target seeds` lists the built-in seed families, and `--seed` / `--seeds` let target matrix, suite, and campaign runs expand those families directly. This is the first step from task-centric scheduling toward `scenario seed + mutator` scheduling. Matrix-backed target suites write `target-schedule-matrix.json`, `target-matrix-result.json`, and `candidate_summaries` so later runs can use `--feedback-from <target-matrix-result.json>` plus `--candidate-limit N` to focus on the highest-signal real-target candidates first. The target feedback scheduler now prefers previously unseen seeds, primitives, lifecycle operations, and mutation operators before spending budget on alternate prompt profiles of same-seed variants. `syncfuzz target campaign` automates the same feedback loop across rounds and skips already executed target candidates until the current candidate space is exhausted.

The real-target candidate space now has one deterministic wording dimension as well: built-in `prompt-profile`s. `syncfuzz target prompt-profiles` lists the current profiles, and `--prompt-profile` / `--prompt-profiles` let a run, suite, matrix suite, or campaign compare the same task under `baseline`, `workflow`, or `audit` framing. This is intentionally narrower than full prompt fuzzing: the task semantics stay fixed, while only the operator-style wording changes.

Repository-owned Scenario IR plans are now executable candidate inputs rather than descriptive metadata. A matrix candidate may pass an explicit `execution_plan` into `target run`; the plan controls replay versus fork, semantic checkpoint selector, checkpoint backend, process mode, and fork follow-up environment, and the exact plan is preserved in `target-task.json`. Target corpus replay restores this stored plan, so verify exercises the discovered candidate semantics rather than the repository default. `default_late_observe_delay_ms` is likewise consumed from the candidate when the suite has no explicit override. Direct non-matrix runs retain the built-in task plan. The first semantic generator uses this path to derive `phase-shift-single-process` candidates from split-process checkpoint scenarios, preserving the task/oracle while changing the process boundary. This establishes an executable phase-shift mutation without claiming that arbitrary lifecycle or cross-seed composition is implemented.

The target feedback loop also records intermediate activation progress. Candidate summaries expose `max_activation_stage` and `activation_progress_score`; campaign coverage gain can report `new_activation_progress_values`; and stage-aware prompt repair can recover guidance from either outcome taxonomy or activation summaries. Lifecycle, planting, and activation stalls prefer the corresponding structural prompt variant, while frontier selection records `lifecycle-repair`, `state-plant-repair`, or `activation-repair`. These signals improve retention and scheduling before a candidate reaches a final positive or negative oracle verdict.

Target minimization now has both planning and execution artifacts. Without `--execute`, `syncfuzz target minimize --from ...` remains a read-only extraction step and writes `syncfuzz.target-minimization-batch.v1`. With `--execute`, it reads the original `target-task.json`, greedily attempts bounded prompt-line deletions and then minimizes Scenario IR execution-plan fields in fresh workspaces, and writes `syncfuzz.target-minimization-result.v1`. Execution-plan trials clear process mode, checkpoint backend, checkpoint selector, fork follow-up, and replay one axis at a time. A trial is retained only if it preserves completion plus the source oracle status, attribution, mismatch signature, task-compliance status, and contract interpretation. The result records original/minimized plans and accepted step IDs; `--candidate-limit` and `--max-trials` bound real-target cost. Component, primitive, activation-command, and cross-seed reduction still remain future work.

`contract_interpretation.status` is the next layer above the oracle: `contract-consistent` means the observed result matched the selected lifecycle contract for that integrated target, `contract-violation` means it contradicted that contract, and `contract-unknown` means SyncFuzz still lacks a stable contract claim even though the residue observation itself may already be real. `target-suite-result.json` now also aggregates these as `contract_summaries`.

Replay and fork lifecycle tasks now also switch the LangGraph target to a durable disk-backed checkpoint mode. The wrapper writes `langgraph-checkpointer.json` plus a `langgraph-checkpoints/` directory inside the workspace so checkpoint backend choice and on-disk state can be audited alongside the normal target artifacts.

For cross-process experiments, the same LangGraph target can also run with `LANGGRAPH_PROCESS_MODE=split-process`. In that mode the initial branch and replay/fork follow-up execute in separate Python processes while reusing the durable checkpoint directory, and the workspace keeps both phase-local artifacts and merged canonical summaries.

The first repository-owned real target is `targets/langgraph_shell_react/`. It intentionally stays close to the official LangChain and LangGraph path:

- `create_agent(...)`
- `ShellToolMiddleware(...)`
- LangGraph thread state and checkpointer

It writes `langgraph-history.json`, `langgraph-run-summary.json`, and `langgraph-checkpointer.json` into the SyncFuzz workspace so the run can be inspected and replay/fork behavior can be correlated with the normal SyncFuzz filesystem and process artifacts. The current SyncFuzz integration also ships a first built-in LangGraph contract profile: same-run persistent shell reuse is treated as expected, while replay/fork residue tasks are interpreted against the wrapper-selected checkpoint boundary.
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

`corpus analyze` summarizes the corpus without replaying it. It groups entries by execution kind and subject, and for real-target entries it also summarizes stored oracle status, attribution, task-compliance status, and contract status. If you also pass a `verification-result.json`, the same report includes replay outcome taxonomy and per-subject verification summaries.

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

For replay triage, `replay-result.json` now also records `outcome_category` and `outcome_reason`. `verification-result.json` aggregates the same taxonomy as `outcome_summaries`, and now also emits `subject_summaries`, which lets real-target campaigns distinguish clean negatives, task drift, lifecycle failures, missing planted state, and plain residue misses per `target/task` instead of collapsing every non-reproduction into one bucket.

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
