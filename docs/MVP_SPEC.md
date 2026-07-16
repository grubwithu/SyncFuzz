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

Repository-owned Scenario IR plans are now executable candidate inputs rather than descriptive metadata. The first frozen schema is `syncfuzz.target-scenario.v1`; every component has a stable `component_id`, `role`, and `kind_id`, with supported roles for `setup`, `plant`, `lifecycle`, `activation`, `fault`, and `oracle`. Built-in and generated scenarios are normalized and validated before execution, including structural components implied by primitive, lifecycle, activation, and oracle metadata. A matrix candidate passes its full Scenario IR into `target run`, including scenario and seed identity, components, mutation provenance, candidate-specific prompt, and `execution_plan`; the plan controls replay versus fork, semantic checkpoint selector, checkpoint backend, process mode, and fork follow-up environment. The exact candidate IR is preserved in `target-task.json` instead of being reconstructed from the built-in task catalog at execution time. Target corpus replay and minimization trials restore this stored IR, so verification and reduction exercise the discovered candidate semantics rather than the repository default. Generated scenarios can also select oracle, compliance, contract, and mismatch-signature semantics from the IR instead of inheriting those bindings from `task_id`. Minimization plans carry component IDs and kind IDs, and the execute path can now try deleting optional `setup` / `fault` IR components without parsing summaries. `default_late_observe_delay_ms` is likewise consumed from the candidate when the suite has no explicit override. Direct non-matrix runs retain the built-in task scenario.

The matrix now contains the first compatibility-aware primitive-substitution family in three layers. The first portable same-run family derives `persistent-shell-poisoning/primitive-shell-env-export`, `persistent-shell-poisoning/primitive-shell-function-define`, `persistent-shell-poisoning/primitive-shell-cwd-change`, and `persistent-shell-poisoning/primitive-shell-umask-set` from the PATH same-run seed while preserving the `run -> continue` lifecycle, and the same generated Scenario IR can now execute on both LangGraph and MAF via generic `env-residue` / `function-residue` / `cwd-residue` / `umask-residue` oracle and compliance dispatch. LangGraph also binds those generated same-run scenarios to direct `preserve` contract rules for the corresponding execution-context surface instead of inheriting the PATH baseline rule. The replay layer now derives `persistent-shell-poisoning-replay/primitive-shell-env-export` and `persistent-shell-poisoning-replay/primitive-shell-function-define` from the persistent-shell PATH replay seed while preserving the `checkpoint -> replay` lifecycle. Those generators supply `before-env-export` or `before-function-define` selectors plus replay-safe prompts that keep the final observation non-mutating; the replay-specific oracle and compliance path then distinguishes direct replay residue from replay-side reexecution and final-call reconstruction, and scenario-specific contract rules expect reset across the selected replay boundary. The fork layer still derives `primitive-shell-env-export` and `primitive-shell-function-define` from the persistent-shell PATH fork seed while preserving the `checkpoint -> fork` lifecycle. That generator supplies `before-env-export` or `before-function-define` selectors plus separate initial-branch and fork-activation instructions. The corresponding env/function oracles consume combined initial/fork command traces; compliance verifies exactly one initial plant plus a non-mutating fork observation; and scenario-specific contract rules expect reset across the selected fork boundary. These are controlled generated combinations, not yet arbitrary primitive cross-product generation. The earlier `phase-shift-single-process` generator remains available for split-process checkpoint scenarios.

The activation-substitution family is now executable across both communication and resource-access capability surfaces. The first portable same-run trusted-activation candidate, `unix-listener-residue/activation-trusted-action`, preserves the Unix-listener plant and `run -> continue` boundary, and the same generated Scenario IR now executes on both LangGraph and MAF through generic shell-trace dispatch. It replaces passive socket reachability with a fixed trusted policy in the later observation call, recording the listener response in `trusted-action-response.txt`, whether the fixed local action executed in `trusted-action-effect.txt`, and the combined classification in `trusted-action-check.txt`; response text is never executed as shell input. `unix-listener-residue-fork/activation-trusted-action` does the analogous substitution for the `checkpoint -> fork` boundary, while `inherited-fd-branch-leakage/activation-trusted-action` preserves the inherited-fd holder and `checkpoint -> fork` boundary but replaces passive secret observation with a fixed trusted policy that records the recovered fd input in `inherited-fd-trusted-input.txt`, the local consequence in `inherited-fd-trusted-effect.txt`, and the combined classification in `inherited-fd-trusted-check.txt`; the recovered secret is never executed as shell input. Scenario-specific trace analysis rejects listener relaunch, fd-holder relaunch, or other reconstruction, while the oracle, compliance checker, contract rule, and mismatch signature are selected from the generated Scenario IR.

The first generated lifecycle splice is also executable. `unix-listener-residue-fork/lifecycle-splice-checkpoint-replay` keeps the Unix-listener plant but swaps the lifecycle from `checkpoint -> fork` to `checkpoint -> replay`. Its replay-safe prompt skips relaunch when `branch-listener.sock` and `branch-listener-pid.txt` already exist, writes `unix-listener-residue-replay-check.txt`, and lets the oracle distinguish three cases from the replay transcript: direct runtime residue, legitimate replay-side relaunch, and clean replay reset. The generated contract rule interprets only the first case as preserving post-checkpoint listener state across replay.

The target feedback loop also records intermediate activation progress. Candidate summaries expose `max_activation_stage` and `activation_progress_score`; campaign coverage gain can report `new_activation_progress_values`; and stage-aware prompt repair can recover guidance from either outcome taxonomy or activation summaries. Lifecycle, planting, and activation stalls prefer the corresponding structural prompt variant, while frontier selection records `lifecycle-repair`, `state-plant-repair`, or `activation-repair`. These signals improve retention and scheduling before a candidate reaches a final positive or negative oracle verdict.

Target minimization now has both planning and execution artifacts. Without `--execute`, `syncfuzz target minimize --from ...` remains a read-only extraction step and writes `syncfuzz.target-minimization-batch.v1`. With `--execute`, it reads the original `target-task.json`, greedily attempts bounded prompt-line deletions, tries conservative Scenario IR component deletion for optional `setup` / `fault` nodes, then minimizes Scenario IR execution-plan fields in fresh workspaces, and writes `syncfuzz.target-minimization-result.v1`. Execution-plan trials clear process mode, checkpoint backend, checkpoint selector, fork follow-up, and replay one axis at a time. The default `--fidelity exact` mode retains a trial only if it preserves completion plus the source oracle status, attribution, full mismatch signature, task-compliance status, and contract interpretation. The optional `semantic` mode preserves oracle status, lifecycle / phase / state-class / relation / impact, task compliance, and contract status while allowing attribution and primitive-operation drift; `impact` mode preserves oracle status plus the same `signature.impact`. Under `semantic` or `impact`, a `primitive-minimization` plan step can now clear reducible plant metadata by removing the matching plant component and `PlantPrimitiveID` while leaving the concrete prompt and command unchanged. An `activation-minimization` plan step can also line-minimize multi-line fork activation messages while keeping at least one activation line. The result records the selected fidelity, original/minimized prompt line counts, component counts, execution plans, and accepted step IDs; `--candidate-limit` and `--max-trials` bound real-target cost. Command-level primitive rewriting, non-fork activation-command reduction, lifecycle / activation / oracle component reduction, and cross-seed reduction still remain future work.

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
