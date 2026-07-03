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
go run ./cmd/syncfuzz target groups
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group workspace-residue --command-file examples/target-commands/langgraph-shell-react.sh --repeat 5 --observe-delay 500ms --out runs --corpus corpus
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
make target-groups
make target-run TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh EXPECT_FILES=late-effect
make target-suite TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh REPEAT=3
make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini
make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASKS=orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork REPEAT=2
make target-langgraph-shell-react-suite LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_GROUP=workspace-residue REPEAT=5
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini OPENAI_BASE_URL=https://api.example.com/v1
make target-langgraph-shell-react TARGET_TASK=orphan-process-long-delay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini LANGGRAPH_REPLAY=true LANGGRAPH_CHECKPOINT_INDEX=0
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-replay
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=persistent-shell-poisoning-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=file-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=directory-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=delete-residue-fork
make target-langgraph-shell-react LANGCHAIN_MODEL=openai:gpt-4.1-mini TARGET_TASK=symlink-residue-fork
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

Phase 5 target runs add a parallel artifact set for real agent/runtime observation:

- `target-task.json`: adapter id, target id, prompt, command, timeout, and expected files
- `target-prompt.txt`: prompt material passed through `SYNCFUZZ_PROMPT_FILE`
- `target-output.txt`: combined stdout/stderr from the real target command
- `target-result.json`: command exit status, timeout status, target oracle verdict plus causal attribution, separate task-compliance verdict, process lineage summary, workspace, and artifact path
- `snapshot-late.json` / `process-late.json` / `filesystem-late-metadata.json`: optional late observation artifacts when `--late-observe-delay` is set

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

The first real target is also checked into the repo:

- `targets/langgraph_shell_react/`: minimal official `create_agent + ShellToolMiddleware` LangGraph target
- `examples/target-commands/langgraph-shell-react.sh`: prepared command file for `syncfuzz target run`

Pair runs are written under `runs/pair-<pair_id>/`. They execute a clean `control` run and a `fault` run for the same case, then write `differential-report.json` with the pair verdict, run summaries, and observation coverage extracted from each `state-trace.json`.

Phase 2 runs now use `state-trace.json` as the stable cross-layer index. It aligns every artifact to a lifecycle phase and one of the core layers: Agent, OS, External, or Authority.

Phase 3 includes a deterministic fault-plan catalog, pair-level differential reports, differential suite mode, and deterministic timing profiles. `syncfuzz fault-plans` lists the known-answer plans, `syncfuzz timing-profiles` lists reproducible timing profiles such as `baseline`, `tight`, and `wide`, each run records its selected plan and timing in `fault-plan.json`, `syncfuzz pair` compares `control` and `fault` executions in `differential-report.json`, and `syncfuzz suite --differential` registers security-relevant pair discoveries in the corpus.

Phase 4 has a deterministic mutation primitive catalog and scheduler matrix. `syncfuzz primitives` lists implemented and planned state primitives, `syncfuzz matrix` enumerates reproducible `case x primitive x timing` candidates, and `syncfuzz suite --matrix` executes the implemented candidates while preserving `candidate_id` and `primitive_id` in suite, discovery, and corpus metadata. Matrix suites also rank candidates by novelty, confirmation, reproducibility, execution cost, artifact size, and errors; a later run can pass `--feedback-from <matrix-result.json>` and `--candidate-limit N` to execute the highest-ranked candidates first. `syncfuzz campaign` automates this across multiple rounds, skips already executed candidates while unexplored candidates remain, and writes `campaign-result.json`.

Phase 5 has started with the `command` target adapter. `syncfuzz target run` executes any local or container-visible real agent CLI inside a SyncFuzz workspace, writes `target-prompt.txt` and `target-task.json` into that workspace, passes their paths through `SYNCFUZZ_PROMPT_FILE` and `SYNCFUZZ_TASK_FILE`, captures stdout/stderr, waits for `--observe-delay`, snapshots filesystem/process state before and after execution, optionally waits for `--late-observe-delay`, and writes `target-result.json`. `--command-file` is the most reliable way to pass multi-word or quoted target commands.

The current Phase 5A milestone is frozen: the official LangGraph shell target is integrated, real-target suite/corpus/replay/verify are working, `orphan-process-long-delay` has a stable late-observation oracle, and `persistent-shell-poisoning` now uses transcript-backed evidence instead of a bare witness file.

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

Replay and fork lifecycle tasks now opt into the durable `disk` checkpoint backend automatically. The backend persists LangGraph checkpoint state into `workspace/langgraph-checkpoints/` and summarizes that backend choice in `langgraph-checkpointer.json`.

When you also set `LANGGRAPH_PROCESS_MODE=split-process`, the LangGraph wrapper runs the initial branch and the replay/fork follow-up in two separate Python processes while reusing the same durable checkpoint directory. The workspace keeps phase-specific artifacts such as `langgraph-run-summary-initial.json`, `langgraph-run-summary-resume.json`, `langgraph-lifecycle-initial.json`, `langgraph-lifecycle-resume.json`, `langgraph-checkpointer-initial.json`, and `langgraph-checkpointer-resume.json`, plus the merged canonical artifacts.

For replay and fork lifecycle tasks, `target_oracle` now also records an `attribution` field so Phase 5 experiments can distinguish `runtime-preserved-residue`, `legitimate-reexecution`, `external-state-smuggling`, `clean-replay`, `clean-fork`, `workspace-reconstruction`, and `unknown-causal-path`. The current LangGraph baseline treats honest clean replay or clean fork behavior as regression-stable negative results rather than forcing them into positive residue findings.

LangGraph shell target runs require observed shell tool use. If the model only replies in text without a `tool` message, the wrapper exits non-zero and records `validation_error` in `langgraph-run-summary.json`.

`syncfuzz target tasks` lists the current built-in real-target tasks, and `syncfuzz target suite` batches repeated real-target runs into one `target-suite-<id>/target-suite-result.json` summary. Each suite item now preserves both `target_oracle` and `task_compliance`, so repeated campaigns can separate real residue from task drift. Confirmed target runs are also written into `corpus/`, so `corpus list`, `replay`, and `corpus verify` can exercise the same real target again. For now, target corpus replay reads the original `target-task.json` from each recorded run artifact, so keep the corresponding `runs/` directory when you want to replay or verify those entries later.

`syncfuzz target groups` lists built-in task bundles such as `workspace-residue`, `shell-lifecycle`, and `phase5a-baseline`. `workspace-residue` now covers file, directory, deletion-state, and symlink residue primitives. `syncfuzz target suite --group ...` or `--groups ...` expands those bundles before any explicit `--task` or `--tasks`, which makes it easier to run repeated filesystem-residue campaigns without hand-copying long task lists. The suite summary now also writes both `attribution_summaries` and `compliance_summaries`, so repeated runs can be tallied directly by residue outcome and by prompt/task drift.

Before running it against a hosted model, put provider settings in `.env`, then run the readiness check. For OpenAI-compatible endpoints, set both `OPENAI_API_KEY` and `OPENAI_BASE_URL`:

```bash
cp .env.example .env
# edit .env with your real endpoint and key
make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini
```

Makefile target commands load `.env` automatically. A command-line Make variable such as `OPENAI_BASE_URL=https://...` can still override the file for one run.

Suite runs are written under `runs/suite-<suite_id>/` with a top-level `suite-result.json`, `interesting.json`, and one subdirectory per testcase run. Matrix suite runs also write `schedule-matrix.json` and `matrix-result.json`; the latter includes ranked `candidate_summaries` with average duration and artifact-size metrics. The suite summary marks runs that produce new signatures, state classes, or impacts as `interesting`.

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
targets/                   Real target adapters and runtime-specific entrypoints
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
