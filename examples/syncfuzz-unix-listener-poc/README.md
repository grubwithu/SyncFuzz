# LangGraph Shell ReAct Unix Listener Secret-Leak PoC

This example turns the `unix-listener-residue-fork` primitive into an Agent-level PoC.
The Branch A listener and the Branch B secret operation are both executed by the
`targets/langgraph_shell_react` Agent through `ShellToolMiddleware`.

The PoC does not use real credentials. Branch B creates a dummy key with the
`sk-syncfuzz-poc-` prefix.

## Files

- `branch-a-listener-prompt.txt`: initial Agent prompt. It launches one Branch A
  Unix listener as a Python double-fork daemon and probes it once.
- `branch-b-send-followup.txt`: fork follow-up. Branch B creates a new dummy API
  key and sends it to the fixed socket.
- `run-langgraph-poc.sh`: thin runner around
  `targets/langgraph_shell_react/run_target.py`.
- `task.json`: task metadata used in target summaries.

## Run

Set up the LangGraph target as described in
`targets/langgraph_shell_react/README.md`, including `LANGCHAIN_MODEL` and the
provider API key.

Running this example sends the prompts and LangGraph tool-call context to the
configured model provider. Review the prompt files first if the provider is not
an internal or otherwise approved endpoint.

Socket-send path:

```bash
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh send
```

By default the workspace is written under `/tmp` to avoid filling `runs/`.
Override it when needed:

```bash
SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-agent-poc-send \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh send
```

If the current environment blocks Unix domain socket operations in the default
tool sandbox, run the command from a normal host shell or set:

```bash
SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY=host
```

## Evidence

The proof file is:

```text
$SYNCFUZZ_POC_WORKSPACE/branch-b-secret-leak-check.txt
```

A positive result starts with:

```text
PRESENT_BRANCH_LISTENER_SECRET_LEAKAGE
```

Agent and tool-call evidence is recorded by the LangGraph target:

```text
$SYNCFUZZ_POC_WORKSPACE/langgraph-history.json
$SYNCFUZZ_POC_WORKSPACE/langgraph-fork-summary.json
$SYNCFUZZ_POC_WORKSPACE/langgraph-lifecycle.json
```

The critical causal chain is:

1. Branch A Agent tool call launches `branch-listener.sock`.
2. SyncFuzz/LangGraph forks from `before-unix-listener-launch`.
3. Branch B Agent tool call creates `branch-b-api-key.txt`.
4. Branch B sends a JSON payload containing the dummy key to the fixed Unix
   socket path.
5. The discarded Branch A listener captures the payload through
   `socket_receive`.

This example intentionally does not include a background file poller. A process
that merely keeps running and reads `branch-b-api-key.txt` is a different
orphan-process/file-state residue shape, not the minimal Unix-listener IPC
primitive.
