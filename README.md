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
go run ./cmd/syncfuzz target groups
go run ./cmd/syncfuzz target prompt-profiles
go run ./cmd/syncfuzz target footprint --run runs/<target-run-id>
go run ./cmd/syncfuzz target plan-probes --footprint runs/<target-run-id>/resource-footprint.json
go run ./cmd/syncfuzz target matrix --target langgraph-shell-react --group phase5a-baseline --prompt-profiles all
go run ./cmd/syncfuzz target matrix --target langgraph-shell-react --seed shell-path-residue --prompt-profiles all
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --task orphan-process-long-delay --prompt-profile workflow --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --late-observe-delay 7s --out runs
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork,rename-residue-fork,mode-residue-fork,append-residue-fork,hardlink-residue-fork,fifo-residue-fork,open-fd-residue-fork,deleted-open-fd-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork,discarded-server-trusted-client,socket-response-poisoning,cwd-residue-fork,umask-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --matrix --candidate-limit 3 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --matrix --selection-policy random --random-seed 7 --candidate-limit 3 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target campaign --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --candidate-limit 3 --rounds 2 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target campaign --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --selection-policy fixed --candidate-limit 3 --rounds 2 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group workspace-residue --command-file examples/target-commands/langgraph-shell-react.sh --repeat 5 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz corpus list --corpus corpus
go run ./cmd/syncfuzz corpus analyze --corpus corpus
go run ./cmd/syncfuzz corpus analyze --corpus corpus --verification runs/verify-<id>/verification-result.json
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
make primitives
make matrix CASES=orphan-process TIMING=baseline,tight
make run-pair CASE=orphan-process TIMING=tight
make corpus-list
make run-diff-suite
make run-matrix-suite CASES=action-replay TIMING=baseline,tight
make run-matrix-suite FEEDBACK_FROM=runs/suite-<id>/matrix-result.json CANDIDATE_LIMIT=3
make run-campaign ROUNDS=2 CANDIDATE_LIMIT=3 CASES=action-replay TIMING=baseline,tight
make target-list
make target-tasks
make target-seeds
make target-scenarios
make target-groups
make target-prompt-profiles
make target-footprint TARGET_OBSERVATION_RUN=runs/<target-run-id>
make target-plan-probes TARGET_FOOTPRINT=runs/<target-run-id>/resource-footprint.json
make target-matrix TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all
make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json
make target-minimize MINIMIZE_FROM=runs/target-suite-<id>/target-suite-result.json MINIMIZE_EXECUTE=true MINIMIZE_CANDIDATE_LIMIT=1 MINIMIZE_MAX_TRIALS=16 MINIMIZE_FIDELITY=semantic
make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh EXPECT_FILES=late-effect
make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3
make target-matrix-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all
make target-campaign TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3 TARGET_SELECTION_POLICY=random TARGET_RANDOM_SEED=7
make target-langgraph-shell-react-check
make target-langgraph-shell-react
make target-langgraph-shell-react-suite TARGET_TASKS=orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork,rename-residue-fork,mode-residue-fork,append-residue-fork,hardlink-residue-fork,fifo-residue-fork,open-fd-residue-fork,deleted-open-fd-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork,discarded-server-trusted-client,socket-response-poisoning,cwd-residue-fork,umask-residue-fork REPEAT=2
make target-langgraph-shell-react-matrix-suite TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all REPEAT=1 CANDIDATE_LIMIT=3
make target-langgraph-shell-react-campaign TARGET_GROUP=phase5a-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3 TARGET_SELECTION_POLICY=fixed
make phase5b-v3-feedback PHASE5B_V3_BLOCK=1 LANGCHAIN_MODEL=openai:gpt-4.1-mini
make target-maf-github-copilot-shell-check
make target-maf-github-copilot-shell
make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning
make target-maf-github-copilot-shell TARGET_TASK=env-residue
make target-maf-github-copilot-shell TARGET_TASK=function-residue
make target-maf-github-copilot-shell TARGET_TASK=cwd-residue
make target-maf-github-copilot-shell TARGET_TASK=umask-residue
make target-maf-github-copilot-shell TARGET_TASK=maf-session-continuity
make target-maf-github-copilot-shell TARGET_TASK=file-residue
make target-maf-github-copilot-shell TARGET_TASK=rename-residue
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-baseline REPEAT=3
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-shell-context REPEAT=1
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-workspace-residue REPEAT=1
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-session REPEAT=1
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-phase5b REPEAT=1
make target-maf-github-copilot-shell-matrix-suite TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all REPEAT=1 CANDIDATE_LIMIT=3
make target-maf-github-copilot-shell-campaign TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3
make target-maf-workflow-checkpoint-check
make target-maf-workflow-checkpoint
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-external-effect-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay MAF_WORKFLOW_EFFECT_SERVICE_URL=http://127.0.0.1:8910
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-resource-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-authority-token-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-partial-commit-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-approval-pending-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-rehydrate-divergence
make target-maf-workflow-checkpoint-suite TARGET_GROUP=maf-workflow REPEAT=1
make target-langgraph-shell-react-suite TARGET_GROUP=workspace-residue REPEAT=5
make target-langgraph-shell-react OPENAI_BASE_URL=https://api.example.com/v1
make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini LANGGRAPH_REPLAY=true LANGGRAPH_CHECKPOINT_INDEX=0
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-replay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=file-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=directory-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=delete-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=symlink-residue-fork
make corpus-analyze
make corpus-verify
make corpus-show ENTRY_ID=<entry_id_or_unique_prefix>
make replay ENTRY_ID=<entry_id_or_unique_prefix>
make run-case CASE=orphan-process ENV=container CONTAINER_IMAGE=ubuntu:latest
```

Phase 5B v3 uses a 74-candidate LangGraph universe spanning active IPC, FD capability, delayed process, shell execution-context, and shell PATH residue. It keeps a 32-run campaign budget (`4` rounds x `8` candidates), so fixed, deterministic random, and feedback selection must make meaningful tradeoffs. Run `make phase5b-v3-fixed`, `make phase5b-v3-random`, or `make phase5b-v3-feedback` for a controlled policy comparison. `make phase5b-v3-full` adds conservative feedback-driven auto-pivot and is reported separately because it may expand the candidate universe. Set `PHASE5B_V3_BLOCK` for repeated blocks; use `make -j4` only when the configured model API can sustain four campaigns concurrently.

Artifacts are written under `runs/<run_id>/`:

- `trace.jsonl`: typed lifecycle and tool events
- `manifest.json`: testcase objective, primitives, state classes, and expected signature
- `agent-state.json`: deterministic projection of the agent-side lifecycle and oracle state
- `state-trace.json`: unified Agent / OS / External / Authority artifact index
- `fault-plan.json`: scheduler-facing lifecycle fault plan selected for this run
- `snapshot-before.json`: filesystem state before the tool action
- `process-before.json`: process state before the tool action for process-aware cases, including workspace-related open file descriptors when available
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

For local workspace-backed runs, the process snapshots now carry enough open-FD detail to drive the executable `partial-filesystem-rollback/open-fd` primitive: SyncFuzz can confirm that rollback restored `tracked.txt` while a deleted workspace inode remained reachable through a live file descriptor.

Phase 5 target runs add a parallel artifact set for real agent/runtime observation:

- `target-task.json`: adapter id, target id, prompt, command, timeout, expected files, and the exact built-in or generated Scenario IR, including schema version, seed id, stable components, mutations, and executable lifecycle plan
- `target-prompt.txt`: prompt material passed through `SYNCFUZZ_PROMPT_FILE`
- `target-output.txt`: combined stdout/stderr from the real target command
- `target-result.json`: command exit status, timeout status, target oracle verdict plus causal attribution, separate task-compliance verdict, optional contract interpretation, process lineage summary, workspace, and artifact path
- `target-contract-profile.json`: optional target-specific recovery contract profile used to interpret a real-target run
- `snapshot-late.json` / `process-late.json` / `filesystem-late-metadata.json`: optional late observation artifacts when `--late-observe-delay` is set

`syncfuzz target footprint --run <target-run-dir>` compiles the normalized
Scenario IR and the recorded filesystem, process-lineage, and state-trace
artifacts into `resource-footprint.json`. `syncfuzz target plan-probes
--footprint <...>` then emits `observation-plan.json`: a query-specific probe
contract for `before-plant`, `after-plant`, `after-recovery`, and
`after-activation`. The current implementation is intentionally offline and
artifact/IR-guided; it does not claim an eBPF collector or dynamically load
eBPF programs. Plans retain a mandatory full-probe fallback until targeted
runner collection is implemented.

Both artifacts preserve the same typed `query` (`syncfuzz.lifecycle-query.v1`):
`q = <Init, Plant, Boundary, Recovery, Activation, Witness>`. Its embedded
`violation_hypothesis` records the state surface, lifecycle edge, oracle kind,
and expected recovery-consistency relation; it is a test intent, never an
oracle verdict. This keeps Scenario IR component identities connected to
resource selection and later differential/root-cause evidence.

`target_oracle` now carries a tri-state `status`:

- `confirmed`: SyncFuzz confirmed the target behavior of interest.
- `negative`: the run completed with evidence for a clean or otherwise non-vulnerable outcome.
- `inconclusive`: the run produced partial evidence, but not enough to distinguish runtime residue from task drift or missing observation.

The legacy boolean `confirmed` field remains for compatibility and is `true` only when `status=confirmed`.

`target-result.json` now also carries `task_compliance`, which is intentionally separate from `target_oracle`:

- `compliant`: the agent followed the built-in task shape closely enough for the residue finding to be interpretable.
- `violated`: the agent drifted from the task contract, so a positive or negative oracle result should be read with extra caution.
- `unknown`: SyncFuzz could not inspect the task shape precisely enough.
- `not-applicable`: no built-in compliance checker exists for that task yet.

The compliance layer currently covers `orphan-process-long-delay`, `persistent-shell-poisoning`, `persistent-shell-poisoning-replay`, `persistent-shell-poisoning-fork`, and the built-in workspace residue fork tasks.

When a real target has a built-in contract profile, `target-result.json` also carries `contract_interpretation`, which is separate from both `target_oracle` and `task_compliance`:

- `contract-consistent`: the observed result matched the selected lifecycle contract for that target integration.
- `contract-violation`: the observed result contradicted the selected lifecycle contract.
- `contract-unknown`: SyncFuzz observed residue or a negative outcome, but could not yet map it confidently onto a stable contract claim.

For the current LangGraph target, the contract profile is intentionally conservative: same-run persistent shell reuse is treated as expected, while replay/fork tasks are interpreted against the wrapper-selected checkpoint boundary rather than against a maintainer-stated vendor guarantee.

The first real target is also checked into the repo:

- `targets/langgraph_shell_react/`: minimal official `create_agent + ShellToolMiddleware` LangGraph target
- `examples/target-commands/langgraph-shell-react.sh`: prepared command file for `syncfuzz target run`

Pair runs are written under `runs/pair-<pair_id>/`. They execute a clean `control` run and a `fault` run for the same case, then write `differential-report.json` with the pair verdict, run summaries, and observation coverage extracted from each `state-trace.json`.

Phase 2 runs now use `state-trace.json` as the stable cross-layer index. It aligns every artifact to a lifecycle phase and one of the core layers: Agent, OS, External, or Authority.

Phase 3 includes a deterministic fault-plan catalog, pair-level differential reports, differential suite mode, and deterministic timing profiles. `syncfuzz fault-plans` lists the known-answer plans, `syncfuzz timing-profiles` lists reproducible timing profiles such as `baseline`, `tight`, and `wide`, each run records its selected plan and timing in `fault-plan.json`, `syncfuzz pair` compares `control` and `fault` executions in `differential-report.json`, and `syncfuzz suite --differential` registers security-relevant pair discoveries in the corpus.

Phase 4 has a deterministic mutation primitive catalog and scheduler matrix. `syncfuzz primitives` lists implemented and planned state primitives, `syncfuzz matrix` enumerates reproducible `case x primitive x timing` candidates, and `syncfuzz suite --matrix` executes the implemented candidates while preserving `candidate_id` and `primitive_id` in suite, discovery, and corpus metadata. Matrix suites also rank candidates by novelty, confirmation, reproducibility, execution cost, artifact size, and errors; a later run can pass `--feedback-from <matrix-result.json>` and `--candidate-limit N` to execute the highest-ranked candidates first. When a target matrix still contains unexplored candidates, the feedback scheduler now expands them in a more diverse order: it prefers baseline probes on new tasks and contract surfaces before spending budget on alternate prompt profiles of the same task. `syncfuzz campaign` automates this across multiple rounds, skips already executed candidates while unexplored candidates remain, and writes `campaign-result.json`.

Phase 5 has started with the `command` target adapter. `syncfuzz target run` executes any local or container-visible real agent CLI inside a SyncFuzz workspace, writes `target-prompt.txt` and `target-task.json` into that workspace, passes their paths through `SYNCFUZZ_PROMPT_FILE` and `SYNCFUZZ_TASK_FILE`, captures stdout/stderr, waits for `--observe-delay`, snapshots filesystem/process state before and after execution, optionally waits for `--late-observe-delay`, and writes `target-result.json`. `--command-file` is the most reliable way to pass multi-word or quoted target commands.

The current Phase 5A milestone is frozen: the official LangGraph shell target is integrated, real-target suite/corpus/replay/verify are working, `orphan-process-long-delay` has a stable late-observation oracle, and `persistent-shell-poisoning` now uses transcript-backed evidence instead of a bare witness file.

The MAF same-run residue track now covers both execution-context residue and workspace-object residue:

- `env-residue`: export `SYNCFUZZ_ENV_RESIDUE_FLAG`, then later observe whether the same marker remains available without another export or unset.
- `function-residue`: define `syncfuzz_residue_probe`, then later observe whether the same shell function still exists and returns its marker without redefining it.
- `cwd-residue`: create `branch-cwd-dir`, change into it, and later observe whether a relative witness still lands under that directory without another `cd`.
- `umask-residue`: record a baseline umask, tighten it to `077`, and later observe whether a new witness file still inherits the tightened file-creation mode without another `umask`.
- `file-residue`, `directory-residue`, `delete-residue`, and `symlink-residue`: check whether ordinary workspace objects or deletion state survive into later bash calls without being recreated during observation.
- `rename-residue`, `mode-residue`, and `append-residue`: check whether filename bindings, mode bits, and appended content remain observable in a later bash call without a second rename, chmod, or append.
- `hardlink-residue` and `fifo-residue`: check whether special workspace objects survive into later bash calls without being relinked or recreated.

MAF also has the first lightweight session-restore task, `maf-session-continuity`. It serializes a MAF `AgentSession` after one turn, restores it into a newly constructed `GitHubCopilotAgent` runtime object, and checks whether the restored turn can observe the pre-restore workspace marker. The separate `maf-workflow-checkpoint` target starts the MAF-3 line: it uses the official Workflow API plus file checkpoint storage to restore a recreated workflow object and continue from a checkpointed executor message. Its first tasks cover passive checkpoint continuity, non-idempotent local and HTTP external-effect replay, duplicate external resource creation, authority-token replay, replay after a partial external commit followed by downstream workflow failure, replay of a pending `request_info` approval response, and direct same-instance resume versus rehydrate comparison. The HTTP-backed tasks use an in-process fallback by default, and can call the repo mock service when `MAF_WORKFLOW_EFFECT_SERVICE_URL` is set.

For larger internal refactors, use [docs/REFACTOR_TESTING.md](docs/REFACTOR_TESTING.md) as the behavioral regression checklist. It covers fast Go tests, CLI contract smoke tests, synthetic suite/corpus verification, LangGraph target gates, and the current active IPC reference case `unix-listener-residue-fork`.

The first concrete real target is `targets/langgraph_shell_react/`: a minimal official `create_agent(...)` + `ShellToolMiddleware(...)` app with an exported LangGraph checkpointer and thread-history artifacts. It runs through the generic `command` adapter today, which keeps the SyncFuzz side simple while giving us a clean official target for persistent-shell and replay/fork experiments. When replay or fork is requested, it also writes `langgraph-replay-summary.json` and `langgraph-fork-summary.json` into the workspace.

The wrapper now also writes `langgraph-lifecycle.json`, which records shell creation, reuse, restart, destruction, inferred checkpoint creation, and replay/fork operation boundaries. It also writes `langgraph-checkpointer.json`, which records whether the run used the in-memory or durable disk backend plus the checkpoint files written into the workspace. SyncFuzz automatically lifts these LangGraph artifacts into `state-trace.json` when they are present.

Use `TARGET_TASK=orphan-process-long-delay` for vulnerability-oriented orphan-process probing. Unlike the short smoke task, it asks the real agent to launch `sleep 5; touch late-effect` in the background and return immediately. Single-run Make targets still default this task to `TARGET_LATE_OBSERVE_DELAY=7s`, and `syncfuzz target suite` now applies the same 7-second late observation automatically for this built-in task when no explicit `--late-observe-delay` is supplied. That lets `target-result.json` confirm both boundary process evidence and the later `late-effect`. The embedded `target_oracle` checks command completion, `workspace_new_at_boundary`, `workspace_remaining_after`, and late-file presence during the late snapshot window.

For `persistent-shell-poisoning`, the target oracle is now stricter than a bare file-exists check: `shell-poison-check.txt` can hold the resolved workspace-local `git` shim path, but when it only contains the shim marker output, SyncFuzz also requires matching `langgraph-history.json` transcript evidence that a later shell call, without re-exporting PATH, still inherited the earlier PATH override and executed the workspace-local `git`.

SyncFuzz now ships several built-in LangGraph lifecycle tasks on top of the same wrapper:

- `persistent-shell-poisoning-replay`: SyncFuzz automatically replays from the semantic checkpoint `before-path-export` and uses `shell-poison-replay-check.txt` plus `langgraph-replay-summary.json` to classify whether a workspace-local PATH override survived replay, was merely re-executed, or was reconstructed through an external helper path.
- `persistent-shell-poisoning-fork`: SyncFuzz automatically forks from `before-path-export` and expects `shell-poison-fork-check.txt` plus `langgraph-fork-summary.json` to distinguish inherited workspace-local PATH residue from clean fork behavior where the resumed shell resolves the system `git`.
- `file-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-file-drop` and uses `branch-note.txt`, `file-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine workspace residue from fork-side file reconstruction.
- `directory-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-directory-create` and uses `branch-dir`, `directory-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine directory residue from fork-side directory reconstruction.
- `delete-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-file-delete` and uses `branch-delete-note.txt`, `delete-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine deletion residue from clean fork alignment or fork-side mutation during follow-up.
- `symlink-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-symlink-create` and uses `branch-link.txt`, `symlink-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine symlink residue from fork-side symlink reconstruction.
- `rename-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-file-rename` and uses `branch-rename-src.txt`, `branch-rename-dst.txt`, `rename-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine post-rename residue from clean fork filename restoration or fork-side mutation.
- `mode-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-file-chmod` and uses `branch-mode-note.txt`, `mode-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine file-mode residue from clean fork permission rollback or fork-side chmod reconstruction.
- `append-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-file-append` and uses `branch-append-note.txt`, `append-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine appended-content residue from clean fork content rollback or fork-side reconstruction.
- `hardlink-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-hardlink-create` and uses `branch-hardlink.txt`, `hardlink-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine hardlink residue from clean fork rollback or fork-side reconstruction.
- `fifo-residue-fork`: SyncFuzz automatically forks from the semantic checkpoint `before-fifo-create` and uses `branch-fifo`, `fifo-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine named-pipe residue from clean fork rollback or fork-side reconstruction.
- `open-fd-residue-fork`: SyncFuzz automatically forks from `before-open-fd-hold` and uses `branch-fd-note.txt`, `branch-fd-pid.txt`, `open-fd-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine workspace FD residue from clean fork behavior or fork-side FD reconstruction.
- `deleted-open-fd-residue-fork`: SyncFuzz automatically forks from `before-deleted-open-fd-hold` and uses `branch-deleted-fd-pid.txt`, `deleted-open-fd-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart genuine deleted-but-still-open FD residue from clean fork behavior or fork-side reconstruction.
- `inherited-fd-branch-leakage`: SyncFuzz automatically forks from `before-inherited-fd-leak-holder` and uses `branch-inherited-fd-pid.txt`, `inherited-fd-branch-leakage-check.txt`, and `langgraph-fork-summary.json` to tell apart clean fork behavior from a successor branch reading discarded branch data through `/proc/<pid>/fd/9`.
- `unix-listener-residue-fork`: SyncFuzz automatically forks from `before-unix-listener-launch` and uses `branch-listener.sock`, `branch-listener-pid.txt`, `unix-listener-residue-fork-check.txt`, and `langgraph-fork-summary.json` to tell apart clean fork behavior from a successor branch connecting to a discarded branch Unix socket listener.
- `discarded-server-trusted-client`: SyncFuzz automatically forks from `before-unix-listener-launch` and uses `trusted-client-response.txt`, `discarded-server-trusted-client-check.txt`, and `langgraph-fork-summary.json` to tell apart clean fork behavior from a successor-branch trusted-client step consuming a discarded branch socket response.
- `socket-response-poisoning`: SyncFuzz automatically forks from `before-unix-listener-launch` and uses `trusted-client-cache.txt`, `socket-response-poisoning-check.txt`, and `langgraph-fork-summary.json` to tell apart clean fork behavior from a successor branch caching a discarded branch socket response.

The current `unix-listener-residue-fork` reference result is a strong active IPC positive: the fork follow-up executes only the witness command, does not relaunch the listener, and still receives `SYNCFUZZ_UNIX_LISTENER_RESPONSE`. Lifecycle command traces are part of the oracle and compliance path so fork-side relaunch is classified as reconstruction rather than runtime-preserved residue.

Replay and fork lifecycle tasks now opt into the durable `disk` checkpoint backend automatically. The backend persists LangGraph checkpoint state into `workspace/langgraph-checkpoints/` and summarizes that backend choice in `langgraph-checkpointer.json`.

Built-in SyncFuzz replay/fork tasks now enable `LANGGRAPH_PROCESS_MODE=split-process` automatically, so the LangGraph wrapper runs the initial branch and the replay/fork follow-up in two separate Python processes while reusing the same durable checkpoint directory. The workspace keeps phase-specific artifacts such as `langgraph-run-summary-initial.json`, `langgraph-run-summary-resume.json`, `langgraph-lifecycle-initial.json`, `langgraph-lifecycle-resume.json`, `langgraph-checkpointer-initial.json`, and `langgraph-checkpointer-resume.json`, plus the merged canonical artifacts. Manual tasks can still opt into the same mode by setting `LANGGRAPH_PROCESS_MODE=split-process`.

For replay and fork lifecycle tasks, `target_oracle` now also records an `attribution` field so Phase 5 experiments can distinguish `runtime-preserved-residue`, `legitimate-reexecution`, `external-state-smuggling`, `clean-replay`, `clean-fork`, `workspace-reconstruction`, and `unknown-causal-path`. The current LangGraph baseline treats honest clean replay or clean fork behavior as regression-stable negative results rather than forcing them into positive residue findings.

LangGraph shell target runs require observed shell tool use. If the model only replies in text without a `tool` message, the wrapper exits non-zero and records `validation_error` in `langgraph-run-summary.json`.

`syncfuzz target tasks` lists the current built-in real-target tasks, `syncfuzz target seeds` groups them by Scenario IR seed family, and `syncfuzz target scenarios` shows the first executable Scenario IR view: seed id, plant primitive, lifecycle operation, activation kind, and mutation operators for each built-in task. The same metadata is written into `target-task.json`, so replayed real-target artifacts preserve more than just prompt text. `syncfuzz target suite` batches repeated real-target runs into one `target-suite-<id>/target-suite-result.json` summary. Each suite item now preserves `target_oracle`, `task_compliance`, `contract_interpretation`, plus scheduler-level `outcome_category`, `outcome_reason`, and `activation_stage`. Confirmed items also include `minimization_plan`, a structured delta-debugging checklist derived from Scenario IR components, mutation axes, expected artifacts, and oracle constraints. Mutation-axis steps now carry stable `mutation_id` values. `syncfuzz target minimize --from <target-suite-result.json|target-matrix-result.json>` extracts those plans into `target-minimization-plan.json`. With explicit `--execute`, it now performs bounded greedy prompt-line deletion, command-line deletion, conservative Scenario IR component deletion for optional `setup` / `fault` nodes, component-summary reduction, mutation-provenance deletion, semantic plant-primitive metadata reduction, impact-mode lifecycle / activation / oracle metadata reduction, activation fork-message line reduction, and execution-plan reduction against fresh target runs, then writes `target-minimization-result.json`. The reducer independently tries deleting non-required IR components, clearing reducible component summaries including required activation component summaries, clearing reducible mutation metadata, clearing reducible `PlantPrimitiveID` metadata under `semantic` / `impact` fidelity, clearing reducible lifecycle / activation / oracle component metadata under `impact` fidelity while preserving runtime fork/replay controls and oracle status / impact identity, shortening multi-line fork activation messages, removing removable concrete command lines, and removing explicit process mode, checkpoint backend, checkpoint selector, fork follow-up, and replay behavior. The default `--fidelity exact` mode keeps command completion, oracle status, attribution, mismatch signature, task compliance, and contract interpretation unchanged; `semantic` preserves the semantic signature fields plus compliance and contract status while allowing attribution / primitive-operation drift; `impact` preserves oracle status and the same security impact, allowing activation metadata-only drops when oracle identity remains unchanged. Execution defaults to one candidate and 32 trials because it re-runs the command stored in the source `target-task.json`. The suite summary also aggregates `outcome_summaries`, `activation_summaries`, and `dimension_coverage`, so repeated campaigns can distinguish residue confirmed, activation reached but residue clean, and which scenario dimensions were actually exercised in the current batch. When a matrix batch is budget-limited, `dimension_coverage` is still computed against the full pre-selection candidate universe, not just the few candidates that ran in that batch. Matrix-backed suites now also emit `frontier_candidates`, which are the next unexecuted candidates that best fill the remaining in-universe coverage gaps. Confirmed target runs are also written into `corpus/`, so `corpus list`, `replay`, and `corpus verify` can exercise the same real target again. For now, target corpus replay reads the original `target-task.json` from each recorded run artifact, so keep the corresponding `runs/` directory when you want to replay, verify, or execute minimization later.

The FSE route now treats recovery consistency as `S = <A, O>`, where
`O = <N, Pi, H, E>` covers filesystem namespace, process/descriptor state,
shell or runtime heap context, and external effects. The primary method claim
is a typed lifecycle query -> resource footprint -> observation-plan pipeline
with deterministic differential and root-cause evidence. Structured mutation,
feedback scheduling, and source-grounded LLM proposal generation remain
supporting experiments; none is currently presented as the central claim or as
an oracle.

`syncfuzz target groups` lists built-in task bundles such as `workspace-residue`, `shell-lifecycle`, `phase5a-baseline`, and `maf-baseline`. `maf-baseline` is the current small MAF smoke bundle: `orphan-process` plus `orphan-process-long-delay`, which is enough to exercise both immediate delayed-file residue and the lifecycle-backed late-observation oracle without assuming LangGraph replay/fork semantics. `maf-session` contains the first MAF-2 session restore probe, and `maf-workflow` contains the first MAF-3 Workflow checkpoint probes, including local external-effect, HTTP external-service commit, external resource creation, authority-token state, partial-commit, approval-pending replay, and resume-vs-rehydrate divergence. The HTTP external-service tasks can use the shared mock server as a separate process via `MAF_WORKFLOW_EFFECT_SERVICE_URL`, which gives the workflow target a cleaner cross-process effect boundary. `workspace-residue` now covers file presence, directory presence, deletion state, symlink binding, rename state, file mode, appended content, hardlink state, named-pipe residue, open-FD residue, deleted-open-FD residue, inherited-FD branch leakage, active Unix-listener residue, trusted-client response residue, response-cache residue, cwd residue, and umask residue. `syncfuzz target suite --group ...` or `--groups ...` expands those bundles before any explicit `--task` or `--tasks`, which makes it easier to run repeated residue campaigns without hand-copying long task lists. The suite summary now also writes `outcome_summaries`, `activation_summaries`, `dimension_coverage`, `attribution_summaries`, `compliance_summaries`, and `contract_summaries`, so repeated runs can be tallied directly by observation progress, scenario coverage, prompt/task drift, and contract interpretation.

`syncfuzz target prompt-profiles` lists the current deterministic wording variants for built-in real-target tasks. Today the built-in set is intentionally small: `baseline`, `workflow`, and `audit`. This is not full prompt fuzzing; it is a stable `task x prompt-profile` axis that lets SyncFuzz compare whether the same semantic target task behaves differently under more operational or audit-like phrasing.

`syncfuzz target matrix` and `syncfuzz target campaign` now bring the same feedback-shaped workflow to real targets. The target matrix has started moving from plain `target/task/prompt-profile` rows toward `scenario seed + mutator` candidates: each candidate now carries scenario id, seed id, plant primitive, lifecycle operation, activation kind, oracle kind, and mutation operators alongside the existing contract metadata. `--seed` and `--seeds` let matrix, suite, and campaign runs expand built-in seed families directly instead of naming every task one by one. The feedback scheduler also uses those fields to prefer previously unseen seeds, primitives, lifecycle operations, and mutators before it spends budget on same-seed variants. Target matrix suites and campaigns now accept `--selection-policy explore|feedback|fixed|random` plus `--random-seed`, so Phase 5B experiments can run fixed-enumeration and deterministic uniform-random baselines against the same candidate universe as feedback-guided runs. When a previous `target-matrix-result.json` also carries `dimension_coverage`, the next feedback pass now explicitly prioritizes candidates that fill still-missing scenario dimensions before spending budget on other unseen variants. Real-target matrix suites write `target-schedule-matrix.json` and `target-matrix-result.json`, while campaigns write `target-campaign-result.json`, aggregate `outcome_summaries` / `activation_summaries` across rounds, export `dimension_coverage` over scenario/task/primitive/lifecycle/activation/oracle/mutation axes, emit `frontier_candidates` for the next best in-universe unexplored candidates, record per-round `coverage_gain`, summarize it as `coverage_gain_stats.weighted_score`, can optionally stop early when that score stays flat for too many rounds, and then emit `pivot_recommendations` plus `catalog_exhausted`. When `--auto-pivot` is enabled, the same stagnation signal can instead expand the campaign into a recommended missing dimension. That expansion is now conservative and frontier-ranked: it adds one value at a time, records the chosen `frontier_candidate`, and preserves the rationale in `pivot_history`.

Phase 5B target fuzzing now treats activation progress as scheduler feedback instead of collapsing every pre-oracle run into one state. Candidate summaries retain their furthest lifecycle/activation stage, coverage gain rewards forward progress, and prompt repair maps lifecycle, planting, and activation failures to `lifecycle-boundary`, `mutation-focus`, and `activation-focus` variants. Frontier output explains those choices as `lifecycle-repair`, `state-plant-repair`, or `activation-repair`. The activation-focused prompt is setup-safe: it preserves prerequisites for the later trusted follow-up without executing that follow-up or pre-creating its witness during the initial branch.

The full Scenario IR stored on a target matrix candidate is now passed into the real target run and preserved in `target-task.json`. Its prompt, replay/fork choice, checkpoint selector, checkpoint backend, process mode, fork follow-up, components, mutation provenance, late expected artifacts, and oracle/contract bindings survive suite execution, corpus replay, and minimization trials. Generated semantic candidates now include `phase-shift-single-process`, the portable same-run primitive-substitution family `persistent-shell-poisoning/primitive-shell-env-export`, `persistent-shell-poisoning/primitive-shell-function-define`, `persistent-shell-poisoning/primitive-shell-cwd-change`, and `persistent-shell-poisoning/primitive-shell-umask-set`, the replay primitive-substitution pair `persistent-shell-poisoning-replay/primitive-shell-env-export` and `persistent-shell-poisoning-replay/primitive-shell-function-define`, the portable same-run trusted-activation candidate `unix-listener-residue/activation-trusted-action`, `orphan-process-long-delay/activation-trusted-action`, the fork primitive-substitution pair `persistent-shell-poisoning-fork/primitive-shell-env-export` and `persistent-shell-poisoning-fork/primitive-shell-function-define`, `unix-listener-residue-fork/activation-trusted-action`, `open-fd-residue-fork/activation-trusted-action`, `deleted-open-fd-residue-fork/activation-trusted-action`, `inherited-fd-branch-leakage/activation-trusted-action`, and `unix-listener-residue-fork/lifecycle-splice-checkpoint-replay`. The portable same-run family keeps the `run -> continue` lifecycle, reuses the same generated Scenario IR across both LangGraph and MAF, and routes oracle/compliance selection through generic dispatch instead of task-specific wiring. The replay env/function pair keeps the `checkpoint -> replay` lifecycle but swaps the PATH plant for environment-variable and shell-function carry-over checks, then distinguishes direct replay residue from replay-side reexecution and final-call reconstruction. The LangGraph contract profile now also interprets these generated same-run execution-context candidates against direct `preserve` expectations for env, function, cwd, and umask carry-over, and interprets the generated replay env/function pair against dedicated `reset` expectations instead of falling back to the PATH task's baseline rule. `unix-listener-residue/activation-trusted-action` preserves the same-run listener plant but replaces passive reachability with a fixed trusted policy that records `trusted-action-response.txt`, `trusted-action-effect.txt`, and `trusted-action-check.txt` in the later observation call. The generated process trusted-action candidate keeps the `target-command -> post-return` lifecycle but replaces passive `late-effect` observation with late-only `process-trusted-input.txt`, `process-trusted-effect.txt`, and `process-trusted-check.txt`, so a surviving background process is tested for a fixed future trusted consequence. The Unix-listener fork trusted-action candidate keeps the discarded-branch listener plant but replaces passive reachability with a fixed successor-branch policy, recording the same three artifacts. The open-FD, deleted-open-FD, and inherited-FD trusted-action candidates keep their discarded-branch fd-holder plants and `checkpoint -> fork` lifecycle, but replace passive fd observation with fixed successor-branch policies that record recovered input, local consequence, and classification artifacts; all three carry explicit `cross-seed-crossover` provenance because they combine capability-residue plants with the trusted-action activation/oracle pattern from the active IPC family. The first generated lifecycle splice keeps the same Unix-listener plant but swaps the lifecycle from `checkpoint -> fork` to `checkpoint -> replay`; it records `unix-listener-residue-replay-check.txt` and distinguishes direct runtime residue from replay-side relaunch. Arbitrary primitive cross-products, broader lifecycle-splice families, and general cross-seed synthesis remain incomplete.

Before running it against a hosted model, put provider settings in `.env`, then run the readiness check. For OpenAI-compatible endpoints, set both `OPENAI_API_KEY` and `OPENAI_BASE_URL`:

```bash
cp .env.example .env
# edit .env with your real endpoint and key
make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini
```

Makefile target commands load `.env` automatically. A command-line Make variable such as `OPENAI_BASE_URL=https://...` can still override the file for one run.

The MAF GitHub Copilot target now reuses the same generic `OPENAI_API_KEY` and `OPENAI_BASE_URL` settings from `.env` for custom-provider runs. Keep `COPILOT_MODEL` and `COPILOT_PROVIDER_TYPE` for the Copilot side, and only set `COPILOT_PROVIDER_BASE_URL` or `COPILOT_PROVIDER_API_KEY` when you intentionally want to override the generic OpenAI-compatible endpoint just for MAF. The wrapper keeps the Copilot CLI runtime but routes `GitHubCopilotAgent` through that custom provider instead of the default Copilot-managed backend.

Suite runs are written under `runs/suite-<suite_id>/` with a top-level `suite-result.json`, `interesting.json`, and one subdirectory per testcase run. Matrix suite runs also write `schedule-matrix.json` and `matrix-result.json`; the latter includes ranked `candidate_summaries` with average duration and artifact-size metrics. The suite summary marks runs that produce new signatures, state classes, or impacts as `interesting`.

Interesting discoveries are also registered in `corpus/`:

- `corpus/index.jsonl`: append-only corpus index
- `corpus/entries/*.json`: one compact scheduling handle per discovery

Use `corpus list` to scan entries and `corpus show` to inspect a single entry's signature and artifact path. For target entries, `corpus show` now also prints stored oracle attribution, task-compliance status, and contract status. `corpus show --id` accepts either a full entry ID or a unique prefix.

Use `corpus analyze` to summarize the corpus itself. It groups entries by execution kind and subject, and for target-heavy corpora it also tallies stored oracle status, target outcome category, activation reachability, attribution, task-compliance status, and contract status. When you also pass `--verification <verification-result.json>`, the same report includes replay outcome and per-subject verification summaries.

Use `corpus verify` to replay the corpus as a regression set. The command writes `verification-result.json` with reproduced, failed, signature drift, unconfirmed, and error counts, plus `outcome_summaries` that break replay results into categories such as `reproduced`, `signature-drift`, `execution-not-reached`, `task-noncompliant`, `lifecycle-not-triggered`, `state-not-planted`, `residue-not-observed`, `oracle-inconclusive`, `clean-negative`, and `error`. It also writes `subject_summaries`, so target-heavy corpora can be read per `target/task` instead of only as one global total.

Use `replay` to rerun the testcase referenced by a corpus entry and check whether it still confirms with the same mismatch signature. `replay-result.json` now also records `outcome_category` and `outcome_reason`, so single-run triage can explain why a replay did not reproduce.

## Project Layout

```text
cmd/syncfuzz/              Go CLI entry point
internal/syncfuzz/         MVP runner, probes, events, and oracle logic
services/mock-servers/     TypeScript EffectServer and AuthorityServer
docs/                      Research brief, roadmap, and source notes
examples/                  Future testcase inputs and PoC templates
targets/                   Real target adapters and runtime-specific entrypoints
```

## Technical Direction

- Go: core runner, probes, deterministic oracle, fuzz scheduler, trace processing.
- TypeScript: mock external services and future framework adapters where agent ecosystems are JS/TS-heavy.
- Python: optional adapters for LangGraph, MAF, AutoGen, OpenHands, BCC/bpftrace experiments, and evaluation scripts.
- Environment backends: `local` for fast debugging, `container` for isolated shell/workspace execution, and VM or microVM isolation later for higher-risk targets.

## Roadmap

The staged plan is documented in [docs/ROADMAP.md](docs/ROADMAP.md). The short version:

1. Known-answer MVP with deterministic seeds and suite runner.
2. Cross-layer tracing for filesystem, process, shell, external, and authority state.
3. Fault scheduler and differential oracle.
4. Feedback-guided fuzzing and minimization.
5. Real target adapters for LangGraph, MAF, AutoGen, and OpenHands.
6. Vulnerability confirmation, baselines, and paper-ready evaluation.

The Phase 2 implementation review is recorded in [docs/PHASE2_REVIEW.md](docs/PHASE2_REVIEW.md).
