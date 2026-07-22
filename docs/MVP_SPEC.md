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
go run ./cmd/syncfuzz target signatures
go run ./cmd/syncfuzz target prompt-profiles
go run ./cmd/syncfuzz target footprint --run runs/<target-run-id>
go run ./cmd/syncfuzz target plan-probes --footprint runs/<target-run-id>/resource-footprint.json
go run ./cmd/syncfuzz target refine-plan --plan runs/<pilot-run>/observation-plan.json --fallback-report runs/<pruned-run>/targeted-probe-report.json
go run ./cmd/syncfuzz target compare --control runs/<control-run-id> --target runs/<target-run-id>
go run ./cmd/syncfuzz target pair-campaign --manifest target-pair-campaign.json --out runs/<pair-campaign>
go run ./cmd/syncfuzz target calibration-summary --inputs runs/<pair-campaign> --out runs/<pair-campaign>/target-pair-calibration-summary.json
go run ./cmd/syncfuzz target contract-propose --target langgraph-shell-react --tasks persistent-shell-poisoning-replay --source-root examples --sources target-contract-candidate-source.example.md --generator-command 'bash target-contract-proposal-generator.example.sh' --out runs
go run ./cmd/syncfuzz target contract-candidates --input examples/target-contract-candidates.example.json --source-root examples --out runs/target-contract-candidate-validation.json
go run ./cmd/syncfuzz target run --task <matching-task> --observation-plan runs/<target-run-id>/observation-plan.json --command-file examples/target-commands/orphan-process.sh --out runs
go run ./cmd/syncfuzz target run --task <matching-task> --observation-plan runs/<target-run-id>/observation-plan.json --observation-mode pruned-filesystem --command-file examples/target-commands/orphan-process.sh --out runs
go run ./cmd/syncfuzz target run --task <matching-task> --observation-plan runs/<target-run-id>/observation-plan.json --observation-mode pruned --command-file examples/target-commands/orphan-process.sh --out runs
go run ./cmd/syncfuzz target matrix --target langgraph-shell-react --group phase5a-baseline --prompt-profiles all
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
go run ./cmd/syncfuzz target run --target langgraph-shell-react --task orphan-process-long-delay --prompt-profile workflow --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --late-observe-delay 7s --out runs
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork,open-fd-residue-fork,deleted-open-fd-residue-fork,inherited-fd-branch-leakage,unix-listener-residue-fork,discarded-server-trusted-client,socket-response-poisoning,cwd-residue-fork,umask-residue-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --matrix --candidate-limit 3 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target suite --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --matrix --selection-policy random --random-seed 7 --candidate-limit 3 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target campaign --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --candidate-limit 3 --rounds 2 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target campaign --target langgraph-shell-react --group phase5a-baseline --prompt-profiles baseline,workflow,audit --selection-policy fixed --candidate-limit 3 --rounds 2 --command-file examples/target-commands/langgraph-shell-react.sh --repeat 1 --observe-delay 500ms --out runs --corpus corpus
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
make target-footprint TARGET_OBSERVATION_RUN=runs/<target-run-id>
make target-plan-probes TARGET_FOOTPRINT=runs/<target-run-id>/resource-footprint.json
make target-refine-plan TARGET_OBSERVATION_PLAN=runs/<pilot-run>/observation-plan.json TARGET_FALLBACK_REPORT=runs/<pruned-run>/targeted-probe-report.json
make target-compare TARGET_CONTROL_RUN=runs/<control-run-id> TARGET_COMPARE_RUN=runs/<target-run-id>
make target-run TARGET_TASK=<matching-task> TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh
make target-run TARGET_TASK=<matching-task> TARGET_OBSERVATION_PLAN=runs/<target-run-id>/observation-plan.json TARGET_OBSERVATION_MODE=pruned-filesystem TARGET_COMMAND_FILE=examples/target-commands/orphan-process.sh
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
  resource-footprint.json              # emitted by `target footprint`
  observation-plan.json                 # emitted by `target plan-probes`
  observation-plan-refined.json         # emitted by `target refine-plan`
  targeted-probe-report.json            # emitted when a rerun consumes a plan
  target-checkpoint-differential.json   # deterministic checkpoint delta evidence
  target-lifecycle-markers.jsonl        # optional target-emitted semantic markers
  snapshot-after-plant.json             # optional P4 snapshot from after-plant marker
  process-after-plant.json              # optional P4 process snapshot from marker
  snapshot-after-recovery-marker.json   # optional P6 snapshot from recovery marker
  process-after-recovery-marker.json    # optional P6 process snapshot from recovery marker
  snapshot-after-activation-marker.json # optional P7 snapshot from activation marker
  process-after-activation-marker.json  # optional P7 process snapshot from activation marker
  snapshot-full-fallback.json           # final fallback for pruned-filesystem mode
  process-full-fallback.json            # final process fallback for local pruned mode
  workspace/
```

`syncfuzz target run` currently supports the implemented `command` adapter. It runs any local or container-visible agent command inside the SyncFuzz workspace, writes `target-prompt.txt` and `target-task.json` into that workspace, exports `SYNCFUZZ_PROMPT`, `SYNCFUZZ_PROMPT_FILE`, `SYNCFUZZ_TASK_FILE`, `SYNCFUZZ_RUN_ID`, `SYNCFUZZ_TARGET_ID`, `SYNCFUZZ_REPO_ROOT`, and `SYNCFUZZ_WORKSPACE`, captures combined stdout/stderr, waits for `--observe-delay`, optionally waits for `--late-observe-delay`, and checks optional `--expect-files`. `target-task.json` now also carries built-in executable Scenario IR metadata when the task is repository-owned: seed id, plant primitive, activation kind, oracle kind, mutation operators, and the lifecycle execution plan used to derive replay/fork runtime overrides. `target-result.json` embeds the process lineage summary, a task-specific `target_oracle`, and a separate `task_compliance` verdict, so real-target runs can be triaged for both boundary residue and prompt/task drift. `--command-file` is the most reliable way to pass quoted or multi-line commands. This is observation-only: it does not yet provide framework-native checkpoint/replay/cancel hooks, but it gives real Agent CLIs the same filesystem/process artifact contract as known-answer seeds.

When a target ships with a built-in contract profile, the same run now also writes `target-contract-profile.json` into the run artifact directory and adds `contract_interpretation` to `target-result.json`. This lets SyncFuzz distinguish three layers in a real-target result: raw residue evidence (`target_oracle`), prompt/task drift (`task_compliance`), and the current lifecycle-contract reading (`contract_interpretation`).

### Observation-plan v1

The FSE-facing method models recovery consistency as `S = <A, O>` and OS
state as `O = <N, Pi, H, E>`: namespace, process/descriptor, runtime or shell
context, and external effect state. The first implementation covers the
artifact-visible subset of `N`, `Pi`, and `H`; `E` stays represented by the
existing external-effect artifacts and contract/oracle path.

`syncfuzz target footprint --run <target-run-dir>` reads the persisted
Scenario IR plus `snapshot-*.json`, `process-lineage.json`, and
`state-trace.json` to write `resource-footprint.json`. It records why each
resource entered the footprint, preserving the distinction between declared
Scenario IR surfaces and runtime observations. `syncfuzz target plan-probes
--footprint <path>` compiles that footprint into `observation-plan.json` with
the fixed checkpoints `before-plant`, `after-plant`, `after-recovery`, and
`after-activation`.

The two artifacts carry one normalized `query` object with schema
`syncfuzz.lifecycle-query.v1`: `q = <Init, Plant, Boundary, Recovery,
Activation, Witness>`. Each stage preserves Scenario IR `component_id`,
`kind_id`, and summary. `violation_hypothesis` names the recovery-consistency
question (state surface, lifecycle edge, oracle kind, expected relation) but
does not carry a verdict; deterministic oracle and contract interpretation
remain the only verdict sources.

The compiler applies explicit static dependency closure: a Unix-domain socket
requires namespace, process, and file-descriptor probes; a file descriptor
requires a process probe. It emits a mandatory full-probe fallback policy
(`expand-once-then-full-probe`) for unplanned state. V1 is offline plan
compilation plus a shadow-mode runner consumer. `target run --observation-plan
<path>` rejects a plan whose query id does not match the target task, copies
the validated plan into the new run, and writes `targeted-probe-report.json`.
That report projects only planned filesystem paths and process/FD selectors
from broad adapter snapshots. The optional `--observation-mode
pruned-filesystem` uses those selected filesystem paths for `snapshot-before`,
`snapshot-after`, and optional late snapshots, then takes one final broad
`snapshot-full-fallback.json`. The report records every path visible only in
that fallback except SyncFuzz's own task/prompt control artifacts, so a later
compiler can expand the plan deterministically.
The local-only `--observation-mode pruned` additionally consumes enabled
process/FD selectors: it scans lightweight process identity first and reads
FD directories only after a selector matches, then preserves all descriptors
of the selected process. It writes `process-full-fallback.json` alongside the
filesystem fallback. It requires an enabled process or FD selector; otherwise
the caller must use `pruned-filesystem`. This mode rejects `--env container`;
container selected process collection is not implemented. It neither installs
eBPF probes nor claims causal tracing.

The generic command adapter exports an executable
`$SYNCFUZZ_LIFECYCLE_MARKER` plus `SYNCFUZZ_LIFECYCLE_MARKER_FILE`. A
cooperating command invokes it with `after-plant`, `after-recovery`, or
`after-activation` at the corresponding semantic boundary. The runner polls
the validated JSONL protocol while the command remains live, captures the
matching filesystem/process artifacts at P4/P6/P7, and acknowledges the
marker before the helper returns; the target therefore cannot advance past the
marker before capture completes. Markers must be strictly ordered as plant,
recovery, activation. The protocol is opt-in; absent an
`after-plant` marker, P5 remains the explicitly partial command-return process
observation rather than a fabricated filesystem boundary.

Every target run also writes `target-checkpoint-differential.json`. It fixes
P0 as the baseline, resolves the best artifact for each post-plant checkpoint
(a validated marker when available, otherwise the explicit adapter fallback),
and serializes deterministic filesystem metadata deltas plus process lineage
deltas. It is evidence for later differential/root-cause analysis, not a
causal classifier or an oracle verdict.

`syncfuzz target compare --control <run> --target <run>` requires matching
query identities and writes `target-pair-differential.json` beside the target
run unless `--out` is specified. It compares checkpoint filesystem state while
ignoring run timestamps and compares process name/cmdline multiplicities while
ignoring PIDs, and excludes SyncFuzz prompt/task/marker control artifacts.
`evidence_candidates` reports target-only paths/processes and
target/control path differences for review; it must not be interpreted as a
root-cause verdict. The v2 report also writes `contract_calibration`, including
the compact control/target contract readings and an explicit eligibility
decision. It adds checkpoint-bound `root_cause_candidates` only when a
confirmed target is paired with a negative control, both task-compliance
readings are `compliant`, both readings resolve to the same profile/rule, the
target is `contract-violation`, and the control is `contract-consistent`. Each
carries that profile/rule and source strength with
`confidence=contract-calibrated-evidence-hypothesis`; it identifies a state
surface and candidate mechanism, never a causal conclusion. A target without a
contract profile, a task-drifted pair, or a contract-consistent target retains
descriptive evidence only.

`syncfuzz target pair-campaign --manifest <path> --out <directory>` executes
the pre-recorded pairs listed in
`syncfuzz.target-pair-campaign-manifest.v1`. Each entry declares the intended
counterfactual control kind (`baseline`, `fresh-runtime`, `branch-cleanup`,
`namespace-restore`, or `custom`) before comparison, resolves relative run
directories against the manifest, and produces a copied manifest, an isolated
pair differential per entry, `target-pair-campaign-result.json`, and the
initial calibration summary. This keeps fresh-runtime, cleanup, and namespace
restoration controls distinguishable in the replication package; a `custom`
control requires a non-empty description. Each report also records a
deterministic counterfactual outcome (`target-only-violation`,
`violation-persists-under-control`, target/control inconclusive, or
`task-noncompliant`) and the target query stratum (root query, violation
signature, mutation operators, and semantic diff). Campaign output aggregates
those labels by control kind and stratum; they describe oracle outcomes only
and never promote a target-only difference to a causal verdict.

`syncfuzz target runtime-pair --control-kind <kind> --control-command-file
<path> --command-file <path> ...` executes one real control/target pair before
the same comparison. It requires identical adapter, target, and task identity,
creates independent control and target workspaces, and writes
`target-runtime-pair.json` next to `target-pair-differential.json`. The named
control intervention remains an explicit responsibility of the supplied
control command; SyncFuzz records its kind but does not infer or fabricate
fresh-runtime, cleanup, or namespace behavior. Its two run directories can be
listed in a later pair-campaign manifest to populate the campaign strata, or
its `target-runtime-pair.json` artifacts can be passed directly as the
comma-separated `target pair-campaign --runtime-pairs` input. The two
pair-campaign inputs are mutually exclusive, preserving a single auditable
source for each aggregation.

`syncfuzz target calibration-summary --inputs <report-or-directory>[,...] --out <path>`
recursively collects the canonical v2 pair reports produced for a controlled
campaign and writes `target-pair-calibration-summary.json`. The summary exposes
the denominator, calibration coverage, unresolved-reason frequencies,
per-contract-rule counts, and report-level provenance needed for later review.
An optional `--review-manifests` input accepts
`syncfuzz.target-pair-root-cause-review.v1` files and validates every label
against an actual candidate before reporting reviewed precision as
`supported / (supported + unsupported)`; `inconclusive` labels stay outside
the denominator. Without such independent candidate-level labels, the summary
does not derive hypothesis precision from its own report.

Every normalized `syncfuzz.target-scenario.v1` now writes a deterministic
`syncfuzz.target-violation-signature.v1` classification with five dimensions:
`relations`, `resource_classes`, `lifecycle_boundary`,
`persistence_mechanisms`, and `consequences`. It is the test intent used to
classify a seed/query, never an oracle result or causal verdict. `syncfuzz
target signatures` lists the built-in mapping. The same label is preserved in
target schedule candidates, suite runs, candidate summaries, and frontiers;
the feedback coverage model includes each taxonomy dimension and the canonical
signature id, so future campaigns can account for violation-class novelty.
Existing MAF external-effect and authority probes retain explicit labels for
compatibility, while the principal FSE study filters to the OS-facing `<A,O>`
classes.

Every Scenario IR also carries `syncfuzz.target-query-genealogy.v1`: a root
query has `query_id == root_query_id`, while a derived query names its
`parent_query_id`. Its atomic mutation records carry an operator, structured
parameters, and a semantic diff over `Plant`, `Boundary`, `Recovery`,
`Activation`, or `Witness`. These fields survive the target task/result,
matrix, suite, candidate-summary, and frontier artifacts. Coverage selection
also treats query root, mutation operator, and semantic-diff fields as explicit
novelty axes; a prompt-profile variant alone is not a new semantic query.

`syncfuzz target contract-candidates --input <path> --source-root <directory>
--out <path>` is the source-grounding boundary for future human or LLM contract
proposals. Its `syncfuzz.target-contract-candidates.v1` input requires a
candidate id, target/task, state surface, lifecycle edge, allowed expectation,
claim type (`documented-contract`, `derived-safety-invariant`, or
`scenario-assumption`), and an exact local source span. The validator resolves
only relative paths inside the explicit source root, rejects traversal or
symlink escapes, and checks that the quote exactly matches the declared source
lines after CRLF normalization. A candidate with any missing or mismatched span
is `unsupported`; a valid entry is only a `source-grounded-proposal`. The
result records `automatic_profile_adoption=disabled`: candidate validation does
not modify built-in profiles, select contract rules, or produce an oracle or
root-cause verdict. The checked-in example is self-validating with
`--source-root examples`, but it remains illustrative rather than an active
target contract.

`syncfuzz target contract-propose --target <id> --tasks <ids> --source-root
<directory> --sources <relative-files> --generator-command <command> --out
<directory>` supplies a narrow execution boundary for an LLM wrapper or other
proposal generator. SyncFuzz writes a versioned request containing only the
selected built-in Scenario IR task contexts and bounded, UTF-8 source files;
the command receives request/output file paths through environment variables
and must write the existing candidate-set schema. The generator's output is
then passed through the same validator with `allowed_source_paths` set to the
exact request bundle, so it cannot turn an unseen local file into accepted
support. A source is capped at 64 KiB and the complete bundle at 128 KiB. The
run records a hash—not the text—of the configured command, and
still fixes `automatic_profile_adoption=disabled`. The bundled shell example
only tests this I/O contract and does not call an LLM. The command is explicitly
caller-supplied and unsandboxed; SyncFuzz never adopts its output as a profile
or oracle input.

`examples/target-contract-proposal-openai.py` is the first real wrapper. It
uses `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and an explicit
`CONTRACT_PROPOSAL_MODEL`, then makes one OpenAI-compatible Chat Completions
request with JSON-object output. `CONTRACT_PROPOSAL_MODEL` has no fallback to
`LANGCHAIN_MODEL`: set it explicitly in `.env` or on the command line before a
run. It is opt-in through
`--generator-command 'python3 target-contract-proposal-openai.py'`; no test or
default command invokes it.

`syncfuzz target refine-plan --plan <plan> --fallback-report <report>` reads
the report's adjacent fallback snapshot and produces
`observation-plan-refined.json`. It admits each unplanned target path once;
an unplanned socket also enables its filesystem, process, and FD dependency
probes. The refined plan records `expansion_count`, source artifact, and added
paths. A second expansion is rejected, preserving the explicit
`expand-once-then-full-probe` policy rather than allowing silent plan drift.

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

Real-target exploration now has its own candidate scheduler. `syncfuzz target matrix` still enumerates repository-owned target tasks, but each candidate now also carries executable Scenario IR metadata: `scenario_id`, `seed_id`, `plant_primitive_id`, `lifecycle_operation_id`, `activation_kind_id`, `oracle_kind_id`, and `mutations`. `syncfuzz target seeds` lists the built-in seed families, and `--seed` / `--seeds` let target matrix, suite, and campaign runs expand those families directly. This is the first step from task-centric scheduling toward `scenario seed + mutator` scheduling. Matrix-backed target suites write `target-schedule-matrix.json`, `target-matrix-result.json`, and `candidate_summaries` so later runs can use `--feedback-from <target-matrix-result.json>` plus `--candidate-limit N` to focus on the highest-signal real-target candidates first. The target feedback scheduler now prefers previously unseen seeds, primitives, lifecycle operations, and mutation operators before spending budget on alternate prompt profiles of same-seed variants. For Phase 5B evaluation, target suite and campaign also expose `--selection-policy explore|feedback|fixed|random`; `fixed` preserves matrix order, while `random` uses a deterministic `--random-seed`, so feedback-guided scheduling can be compared against stable fixed-enumeration and uniform-random baselines without changing the candidate universe. `syncfuzz target campaign` automates the same feedback loop across rounds and skips already executed target candidates until the current candidate space is exhausted.

The real-target candidate space now has one deterministic wording dimension as well: built-in `prompt-profile`s. `syncfuzz target prompt-profiles` lists the current profiles, and `--prompt-profile` / `--prompt-profiles` let a run, suite, matrix suite, or campaign compare the same task under `baseline`, `workflow`, or `audit` framing. This is intentionally narrower than full prompt fuzzing: the task semantics stay fixed, while only the operator-style wording changes.

Repository-owned Scenario IR plans are now executable candidate inputs rather than descriptive metadata. The first frozen schema is `syncfuzz.target-scenario.v1`; every component has a stable `component_id`, `role`, and `kind_id`, with supported roles for `setup`, `plant`, `lifecycle`, `activation`, `fault`, and `oracle`. Built-in and generated scenarios are normalized and validated before execution, including structural components implied by primitive, lifecycle, activation, and oracle metadata. A matrix candidate passes its full Scenario IR into `target run`, including scenario and seed identity, components, mutation provenance, candidate-specific prompt, and `execution_plan`; the plan controls replay versus fork, semantic checkpoint selector, checkpoint backend, process mode, and fork follow-up environment. The exact candidate IR is preserved in `target-task.json` instead of being reconstructed from the built-in task catalog at execution time. Target corpus replay and minimization trials restore this stored IR, so verification and reduction exercise the discovered candidate semantics rather than the repository default. Generated scenarios can also select oracle, compliance, contract, and mismatch-signature semantics from the IR instead of inheriting those bindings from `task_id`. Minimization plans carry component IDs, kind IDs, and mutation IDs, and the execute path can now try deleting optional `setup` / `fault` IR components plus clearing reducible component summaries, mutation provenance, plant, lifecycle, and oracle metadata without parsing summaries. `default_late_observe_delay_ms` is likewise consumed from the candidate when the suite has no explicit override. Direct non-matrix runs retain the built-in task scenario.

The matrix now contains the first compatibility-aware primitive-substitution family in three layers. The first portable same-run family derives `persistent-shell-poisoning/primitive-shell-env-export`, `persistent-shell-poisoning/primitive-shell-function-define`, `persistent-shell-poisoning/primitive-shell-cwd-change`, and `persistent-shell-poisoning/primitive-shell-umask-set` from the PATH same-run seed while preserving the `run -> continue` lifecycle, and the same generated Scenario IR can now execute on both LangGraph and MAF via generic `env-residue` / `function-residue` / `cwd-residue` / `umask-residue` oracle and compliance dispatch. LangGraph also binds those generated same-run scenarios to direct `preserve` contract rules for the corresponding execution-context surface instead of inheriting the PATH baseline rule. The replay layer now derives `persistent-shell-poisoning-replay/primitive-shell-env-export` and `persistent-shell-poisoning-replay/primitive-shell-function-define` from the persistent-shell PATH replay seed while preserving the `checkpoint -> replay` lifecycle. Those generators supply `before-env-export` or `before-function-define` selectors plus replay-safe prompts that keep the final observation non-mutating; the replay-specific oracle and compliance path then distinguishes direct replay residue from replay-side reexecution and final-call reconstruction, and scenario-specific contract rules expect reset across the selected replay boundary. The fork layer still derives `primitive-shell-env-export` and `primitive-shell-function-define` from the persistent-shell PATH fork seed while preserving the `checkpoint -> fork` lifecycle. That generator supplies `before-env-export` or `before-function-define` selectors plus separate initial-branch and fork-activation instructions. The corresponding env/function oracles consume combined initial/fork command traces; compliance verifies exactly one initial plant plus a non-mutating fork observation; and scenario-specific contract rules expect reset across the selected fork boundary. These are controlled generated combinations, not yet arbitrary primitive cross-product generation. The earlier `phase-shift-single-process` generator remains available for split-process checkpoint scenarios.

The activation-substitution family is now executable across communication, active-execution, and resource-access capability surfaces. The first portable same-run trusted-activation candidate, `unix-listener-residue/activation-trusted-action`, preserves the Unix-listener plant and `run -> continue` boundary, and the same generated Scenario IR now executes on both LangGraph and MAF through generic shell-trace dispatch. It replaces passive socket reachability with a fixed trusted policy in the later observation call, recording the listener response in `trusted-action-response.txt`, whether the fixed local action executed in `trusted-action-effect.txt`, and the combined classification in `trusted-action-check.txt`; response text is never executed as shell input. `orphan-process-long-delay/activation-trusted-action` preserves the command-boundary background process plant but replaces passive `late-effect` observation with a delayed fixed trusted policy that writes `process-trusted-input.txt`, `process-trusted-effect.txt`, and `process-trusted-check.txt` only during the late observation window. `unix-listener-residue-fork/activation-trusted-action` does the analogous substitution for the `checkpoint -> fork` boundary. `open-fd-residue-fork/activation-trusted-action`, `deleted-open-fd-residue-fork/activation-trusted-action`, and `inherited-fd-branch-leakage/activation-trusted-action` preserve their fd-holder plants and `checkpoint -> fork` boundaries but replace passive fd observation with fixed trusted policies that record recovered input, local consequence, and combined classification artifacts; the recovered marker or secret is never executed as shell input. The process and fd trusted-action candidates record explicit `cross-seed-crossover` mutation provenance, because their plants come from active-execution or capability-residue families while their trusted-action activation/oracle pattern comes from the active IPC family. Scenario-specific trace analysis rejects listener relaunch, fd-holder relaunch, process-command drift, or other reconstruction, while the oracle, compliance checker, contract rule, and mismatch signature are selected from the generated Scenario IR.

The first generated lifecycle splice is also executable. `unix-listener-residue-fork/lifecycle-splice-checkpoint-replay` keeps the Unix-listener plant but swaps the lifecycle from `checkpoint -> fork` to `checkpoint -> replay`. Its replay-safe prompt skips relaunch when `branch-listener.sock` and `branch-listener-pid.txt` already exist, writes `unix-listener-residue-replay-check.txt`, and lets the oracle distinguish three cases from the replay transcript: direct runtime residue, legitimate replay-side relaunch, and clean replay reset. The generated contract rule interprets only the first case as preserving post-checkpoint listener state across replay.

The target feedback loop also records intermediate activation progress. Candidate summaries expose `max_activation_stage` and `activation_progress_score`; campaign coverage gain can report `new_activation_progress_values`; and stage-aware prompt repair can recover guidance from either outcome taxonomy or activation summaries. Lifecycle, planting, and activation stalls prefer the corresponding structural prompt variant, while frontier selection records `lifecycle-repair`, `state-plant-repair`, or `activation-repair`. These signals improve retention and scheduling before a candidate reaches a final positive or negative oracle verdict.

Target minimization now has both planning and execution artifacts. Without `--execute`, `syncfuzz target minimize --from ...` remains a read-only extraction step and writes `syncfuzz.target-minimization-batch.v1`. With `--execute`, it reads the original `target-task.json`, greedily attempts bounded prompt-line deletions and concrete command-line deletions, tries conservative Scenario IR component deletion for optional `setup` / `fault` nodes, then minimizes Scenario IR execution-plan fields in fresh workspaces, and writes `syncfuzz.target-minimization-result.v1`. Execution-plan trials clear process mode, checkpoint backend, checkpoint selector, fork follow-up, and replay one axis at a time. The default `--fidelity exact` mode retains a trial only if it preserves completion plus the source oracle status, attribution, full mismatch signature, task-compliance status, and contract interpretation. The optional `semantic` mode preserves oracle status, lifecycle / phase / state-class / relation / impact, task compliance, and contract status while allowing attribution and primitive-operation drift; `impact` mode preserves oracle status plus the same `signature.impact`, and can accept activation metadata-only impact clearing when the oracle identity is unchanged. A `mutation-axis-check` step can remove reducible mutation provenance from the minimized Scenario IR. Component steps are now multi-stage: the runner can try a stronger component deletion or semantic metadata reduction, and still try clearing reducible summary text for the same component if the stronger trial is not accepted. Under `semantic` or `impact`, a `primitive-minimization` plan step can clear reducible plant metadata by removing the matching plant component and `PlantPrimitiveID` while leaving the concrete prompt and command unchanged. Under `impact`, a `lifecycle-tightening` step can clear reducible lifecycle metadata by removing the lifecycle component plus `LifecycleEdge` / `LifecycleOperationID` while retaining runtime fork/replay controls such as selectors, backend, process mode, and fork activation text; an `activation-minimization` step can clear reducible activation metadata by removing the activation component plus `ActivationKindID` while preserving oracle status and oracle identity; and an `oracle-preservation` step can clear reducible oracle metadata by removing the oracle component plus `OracleKindID` while preserving the same oracle status and activation impact. An `activation-minimization` plan step can also line-minimize multi-line fork activation messages while keeping at least one activation line. The result records the selected fidelity, original/minimized prompt and command line counts, component and mutation counts, execution plans, and accepted step IDs; `--candidate-limit` and `--max-trials` bound real-target cost. Semantic command rewriting, non-fork activation-command reduction, full lifecycle command rewriting, and cross-seed reduction still remain future work.

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
