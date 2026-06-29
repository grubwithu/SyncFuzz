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

The run writes extra workspace artifacts:

- `langgraph-history.json`
- `langgraph-run-summary.json`
- `langgraph-replay-summary.json` when replay is requested
- `langgraph-fork-summary.json` when fork is requested

These summarize thread history, checkpoint ids, and the final messages returned by the agent.

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

## Notes

- This target is intentionally observation-first. It proves that a real official `create_agent + ShellToolMiddleware` stack can sit inside the SyncFuzz artifact contract.
- The current implementation uses an in-process memory checkpointer. That is enough for first-run observation, replay, and fork within one target invocation, but it is not yet a durable multi-process checkpoint backend.
