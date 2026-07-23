# LangGraph Shell ReAct Unix Listener Secret-Leak PoC

This example turns the `unix-listener-residue-fork` primitive into an Agent-level PoC.
The Branch A listener and the Branch B secret operation are both executed by the
`targets/langgraph_shell_react` Agent through `ShellToolMiddleware`.

The PoC does not use real credentials. Branch B creates a dummy key with the
`sk-syncfuzz-poc-` prefix.

## Files

- `branch-a-listener-prompt.txt`: initial Agent prompt. It launches one Branch A
  Unix listener as a Python double-fork daemon and probes it once.
- `branch-a-benign-prompt.txt`: E0 control prompt; it leaves the pre-existing
  benign broker untouched.
- `branch-b-send-followup.txt`: fork follow-up. Branch B invokes the staged
  trusted client helper, which creates the dummy key only after the fork.
- `syncfuzz_root_cause_lib.py`, `root_cause_probe.py`, and
  `branch_b_client.py`: dependency-free Linux `procfs` probes, root-cause
  artifact generator, and Branch-B `SO_PEERCRED` client witness.
- `benign_broker.py`: local E0 control service; it stores only request length
  and digest, never its payload.
- `run-langgraph-poc.sh`: thin runner around
  `targets/langgraph_shell_react/run_target.py` that stages helpers and writes
  the T0--T3/root-cause artifacts.
- `task.json`: task metadata used in target summaries.

## Run

Set up the LangGraph target as described in
`targets/langgraph_shell_react/README.md`, including `LANGCHAIN_MODEL` and the
provider API key.

Running this example sends the prompts and LangGraph tool-call context to the
configured model provider. Review the prompt files first if the provider is not
an internal or otherwise approved endpoint.

The original socket-send path (E1) is still the default:

```bash
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh send
```

By default the workspace is written under `/tmp` to avoid filling `runs/`.
Override it when needed:

```bash
SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-agent-poc-send \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh send
```

The complete differential set is:

```bash
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e0  # benign control
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e1  # original
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e2  # cleanup control
examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e3  # fresh workspace
```

Use a distinct `SYNCFUZZ_POC_WORKSPACE` for each run. E3 optionally accepts
`SYNCFUZZ_POC_RESUME_WORKSPACE`; otherwise the runner chooses a sibling
`-fresh-runtime` workspace. See
[`docs/MOTIVATION_EXAMPLE_ROOT_CAUSE.md`](../../docs/MOTIVATION_EXAMPLE_ROOT_CAUSE.md)
for the control definitions and result interpretation.

E3 restores the pre-initial-prompt baseline checkpoint. This prevents the
successor from treating the historical Branch-A prompt as unfinished work. If
the resume lifecycle nevertheless contains a listener `bind`/`listen`,
`root-cause.json` reports `invalid-control-reconstruction` and the run must not
be used as the fresh-runtime control. The runner also refuses an E3 resume
workspace containing a stale listener or T1 artifact.

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

The OS-level evidence is written beside it:

```text
$SYNCFUZZ_POC_WORKSPACE/root-cause-t0.json
$SYNCFUZZ_POC_WORKSPACE/root-cause-t1.json
$SYNCFUZZ_POC_WORKSPACE/root-cause-t2.json
$SYNCFUZZ_POC_WORKSPACE/root-cause-t3.json
$SYNCFUZZ_POC_WORKSPACE/branch-b-activation.json
$SYNCFUZZ_POC_WORKSPACE/root-cause.json
```

For E3, T2/T3 and `branch-b-activation.json` are in the resume workspace,
while the merged `root-cause.json` stays in the initial workspace. The evidence
correlates pathname metadata, `/proc/<listener-pid>/fd`, `/proc/net/unix`, and
`SO_PEERCRED`; it does not claim eBPF or full syscall causality.

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
