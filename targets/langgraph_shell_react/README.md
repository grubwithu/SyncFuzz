# LangGraph Shell ReAct Target

This is the first real target for SyncFuzz Phase 5.

It intentionally stays close to the official LangChain and LangGraph path:

- `create_agent(...)`
- `ShellToolMiddleware(...)`
- LangGraph checkpointer and thread state

The target is designed to run through SyncFuzz's generic `command` adapter. SyncFuzz writes `target-prompt.txt` and `target-task.json` into the run workspace, then this target reads them through:

- `SYNCFUZZ_PROMPT_FILE`
- `SYNCFUZZ_TASK_FILE`
- `SYNCFUZZ_WORKSPACE`
- `SYNCFUZZ_RUN_ID`

## Setup

Create a Python environment and install the target dependencies:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r targets/langgraph_shell_react/requirements.txt
```

Install one provider integration that matches your model string, for example:

```bash
pip install langchain-openai
cp .env.example .env
# edit .env with OPENAI_API_KEY, OPENAI_BASE_URL, and LANGCHAIN_MODEL
```

`OPENAI_BASE_URL` is only needed when the model is served by an OpenAI-compatible endpoint instead of the default OpenAI API.
Makefile target commands and the prepared shell wrapper load the repository `.env` automatically.

Then check the environment without printing secrets:

```bash
make target-langgraph-shell-react-check LANGCHAIN_MODEL=openai:gpt-4.1-mini
```

You can omit `LANGCHAIN_MODEL=...` when `LANGCHAIN_MODEL` is also present in `.env`.

## Run Through SyncFuzz

The simplest way is the prepared command file:

```bash
go run ./cmd/syncfuzz target run \
  --target langgraph-shell-react \
  --command-file examples/target-commands/langgraph-shell-react.sh \
  --expect-files late-effect \
  --observe-delay 500ms \
  --out runs
```

Repository-owned prompt files are available for the first two task shapes:

- `targets/langgraph_shell_react/prompts/orphan-process.txt`
- `targets/langgraph_shell_react/prompts/orphan-process-long-delay.txt`
- `targets/langgraph_shell_react/prompts/persistent-shell-poisoning.txt`

For stronger orphan-process probing, prefer the long-delay task. It does not require `late-effect` to exist immediately; `make target-langgraph-shell-react` gives this task a default `TARGET_LATE_OBSERVE_DELAY=7s`, then records both immediate process evidence and the later filesystem effect:

```bash
make target-langgraph-shell-react \
  TARGET_TASK=orphan-process-long-delay
```

After a successful run, `target-result.json` should include:

- `target_oracle.confirmed: true`
- `process_lineage.workspace_new_at_boundary` greater than zero
- `process_lineage.workspace_remaining_after` greater than zero
- `late_expected_files_present: ["late-effect"]`

The wrapper requires an observed shell tool message for these shell tasks. A text-only answer such as "the background process was started" is treated as a failed run because no OS action occurred.

For example:

```bash
make target-langgraph-shell-react \
  TARGET_TASK=persistent-shell-poisoning \
  TARGET_PROMPT_FILE=targets/langgraph_shell_react/prompts/persistent-shell-poisoning.txt \
  EXPECT_FILES=shell-poison-check.txt
```

SyncFuzz now also ships several lifecycle-oriented built-in tasks on top of the same target:

```bash
make target-langgraph-shell-react \
  TARGET_TASK=persistent-shell-poisoning-replay

make target-langgraph-shell-react \
  TARGET_TASK=persistent-shell-poisoning-fork

make target-langgraph-shell-react \
  TARGET_TASK=file-residue-fork

make target-langgraph-shell-react \
  TARGET_TASK=directory-residue-fork

make target-langgraph-shell-react \
  TARGET_TASK=delete-residue-fork

make target-langgraph-shell-react \
  TARGET_TASK=symlink-residue-fork
```

The run writes extra workspace artifacts:

- `langgraph-history.json`
- `langgraph-run-summary.json`
- `langgraph-checkpointer.json`
- `langgraph-lifecycle.json`
- `langgraph-replay-summary.json` when replay is requested
- `langgraph-fork-summary.json` when fork is requested

These summarize thread history, checkpoint ids, checkpoint backend selection, shell/session identity, replay/fork boundaries, and the final messages returned by the agent.

In `target-result.json`, the embedded `target_oracle` now exposes both a backward-compatible boolean `confirmed` and a more explicit `status` field. `status=negative` means the run produced evidence for a clean or non-vulnerable outcome, while `status=inconclusive` means the run produced some evidence but not enough to support a confident residue attribution.

The same result file now also includes `task_compliance`. That field is separate from the residue oracle: it says whether the agent actually followed the intended built-in task shape. A run can therefore be `target_oracle.status=confirmed` but still `task_compliance.status=violated`, which is exactly the split SyncFuzz now uses to avoid over-trusting prompt-drifted executions. Today that coverage includes the long-delay orphan-process task, persistent-shell baseline/replay/fork, and the built-in workspace residue fork tasks.

For `persistent-shell-poisoning`, SyncFuzz uses `langgraph-history.json` as structured oracle evidence when it is available. A bare shim-marker string in `shell-poison-check.txt` is not enough by itself; the transcript also needs to show a later shell call, without re-exporting PATH, still inheriting the earlier PATH override and executing the workspace-local `git` shim.

For `persistent-shell-poisoning-replay`, SyncFuzz automatically replays from the semantic checkpoint `before-path-export`. The replay oracle requires `shell-poison-replay-check.txt` plus `langgraph-replay-summary.json` to show duplicated workspace-local tool-directory entries in `PATH` and workspace-local `git` resolution after replay.

For `persistent-shell-poisoning-fork`, SyncFuzz automatically forks from `before-path-export`. The fork oracle requires `shell-poison-fork-check.txt` plus `langgraph-fork-summary.json` to show whether the fork follow-up shell call, without a new PATH export, inherited the workspace-local `git` shim or cleanly fell back to the system `git`.

For `file-residue-fork`, SyncFuzz automatically forks from `before-file-drop`. The fork oracle requires `file-residue-fork-check.txt` plus `langgraph-fork-summary.json` to show whether `branch-note.txt` survived as genuine workspace residue or was rebuilt during the fork follow-up.

For `directory-residue-fork`, SyncFuzz automatically forks from `before-directory-create`. The fork oracle requires `directory-residue-fork-check.txt` plus `langgraph-fork-summary.json` to show whether `branch-dir` survived as genuine workspace residue or was rebuilt during the fork follow-up.

For `delete-residue-fork`, SyncFuzz automatically forks from `before-file-delete`. The fork oracle requires `delete-residue-fork-check.txt` plus `langgraph-fork-summary.json` to show whether `branch-delete-note.txt` wrongly stayed absent across the rollback boundary or whether the fork stayed aligned with the checkpointed workspace.

For `symlink-residue-fork`, SyncFuzz automatically forks from `before-symlink-create`. The fork oracle requires `symlink-residue-fork-check.txt` plus `langgraph-fork-summary.json` to show whether `branch-link.txt` survived as genuine workspace symlink residue or was rebuilt during the fork follow-up.

Replay and fork lifecycle tasks now default to the durable `disk` checkpoint backend. That backend persists checkpoint state under `langgraph-checkpoints/` inside the SyncFuzz workspace and describes the resulting files in `langgraph-checkpointer.json`.

If you set `LANGGRAPH_PROCESS_MODE=split-process`, the wrapper performs the initial branch and the replay/fork follow-up in separate Python processes while reusing the same durable checkpoint directory. In that mode the workspace keeps phase artifacts such as `langgraph-run-summary-initial.json`, `langgraph-run-summary-resume.json`, `langgraph-lifecycle-initial.json`, `langgraph-lifecycle-resume.json`, `langgraph-checkpointer-initial.json`, and `langgraph-checkpointer-resume.json`, then merges them back into the canonical artifact names.

When `--late-observe-delay` is enabled, SyncFuzz also writes `snapshot-late.json`, `process-late.json`, and `filesystem-late-metadata.json` in the run artifact directory.

## Replay And Fork

The target can also exercise LangGraph history operations inside the same invocation:

```bash
LANGCHAIN_MODEL=openai:gpt-4.1-mini \
SYNCFUZZ_LANGGRAPH_REPLAY=true \
SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=0 \
go run ./cmd/syncfuzz target run \
  --target langgraph-shell-react \
  --command-file examples/target-commands/langgraph-shell-react.sh \
  --expect-files late-effect \
  --observe-delay 500ms \
  --out runs
```

Fork can be requested the same way:

```bash
LANGCHAIN_MODEL=openai:gpt-4.1-mini \
SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX=0 \
SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE='Now check which git binary resolves and write it into shell-poison-check.txt.' \
go run ./cmd/syncfuzz target run \
  --target langgraph-shell-react \
  --command-file examples/target-commands/langgraph-shell-react.sh \
  --observe-delay 500ms \
  --out runs
```

The shell wrapper in [examples/target-commands/langgraph-shell-react.sh](/home/grub/workspace/agent_sec/SyncFuzz/examples/target-commands/langgraph-shell-react.sh) forwards these environment variables to `run_target.py`.

When you use the built-in SyncFuzz tasks `persistent-shell-poisoning-replay`, `persistent-shell-poisoning-fork`, `file-residue-fork`, `directory-residue-fork`, `delete-residue-fork`, or `symlink-residue-fork`, SyncFuzz sets these replay/fork environment variables automatically and switches the checkpointer backend to `disk`. Add `LANGGRAPH_PROCESS_MODE=split-process` when you want the replay/fork step to consume those checkpoints from a fresh target process. The manual environment form remains useful for ad hoc experiments.

## Notes

- This target is intentionally observation-first. It proves that a real official `create_agent + ShellToolMiddleware` stack can sit inside the SyncFuzz artifact contract.
- The wrapper now supports both the default in-process `memory` checkpointer and a durable `disk` backend. `split-process` mode is the current bridge from same-process replay/fork into cross-process checkpoint-consumption experiments.
