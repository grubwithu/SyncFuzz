# Examples

This directory is reserved for minimal PoC inputs and exported minimized testcases.

The first implemented testcase is generated directly by the Go runner:

```bash
go run ./cmd/syncfuzz run --case orphan-process --out runs
```

Phase 5 also includes target-command examples that can be fed to the real target adapter:

```bash
go run ./cmd/syncfuzz target run --command-file examples/target-commands/orphan-process.sh --expect-files late-effect --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
OPENAI_BASE_URL=https://api.example.com/v1 LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --command-file examples/target-commands/langgraph-shell-react.sh --expect-files late-effect --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --task orphan-process-long-delay --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --task persistent-shell-poisoning --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --task persistent-shell-poisoning-replay --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target run --target langgraph-shell-react --task persistent-shell-poisoning-fork --command-file examples/target-commands/langgraph-shell-react.sh --observe-delay 500ms --out runs
LANGCHAIN_MODEL=openai:gpt-4.1-mini go run ./cmd/syncfuzz target suite --target langgraph-shell-react --tasks orphan-process-long-delay,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork --command-file examples/target-commands/langgraph-shell-react.sh --repeat 2 --observe-delay 500ms --out runs --corpus corpus
go run ./cmd/syncfuzz target run --target maf-github-copilot-shell --task orphan-process --command-file examples/target-commands/maf-github-copilot-shell.sh --observe-delay 500ms --out runs
```

The `profiling/` fixture demonstrates the V2.1a evidence contract without requiring BPF privileges:

```bash
go run ./cmd/syncfuzz profile analyze \
  --checkpoints examples/profiling/unix-listener-checkpoints.example.json \
  --events examples/profiling/unix-listener-events.example.jsonl \
  --summaries examples/profiling/unix-listener-summaries.example.json \
  --out runs/profile-example
```

`target-commands/unix-socket-listener.sh` is a calibration fixture for the
privileged `make ebpf-unix-socket-smoke` path. It leaves a workspace-bound Unix
listener alive long enough for the process probe to resolve the endpoint path,
kernel socket ID, holder FD, and holder process as one dependency closure.

`objectives/unix-listener-survival.example.json` plus
`profiling/unix-listener-profile-run.example.json` demonstrate the V2.1b IR:
an explicit `StateObjective` may promote a `StateSeed` only when every required
effect atom has an evidence link at a persistent frontier. The supplied profile
run is explicitly a `calibration-fixture`, so the following command is expected
to reject it rather than create a coverage-bearing StateSeed.

```bash
go run ./cmd/syncfuzz profile promote-seed \
  --objective examples/objectives/unix-listener-survival.example.json \
  --profile-run examples/profiling/unix-listener-profile-run.example.json \
  --frontier C0..C1 \
  --out runs/unix-listener-state-seed.json
```

V2.4's synthesis scheduler assigns a canonical `synthesis-candidate` ID to
each generated natural-task attempt. Only a completed run carrying that exact
candidate ID may be imported and promoted:

```bash
go run ./cmd/syncfuzz profile promote-seed \
  --objective objective.json \
  --target-run runs/<synthesis-run-id> \
  --profile-kind synthesis-candidate \
  --synthesis-candidate runs/<candidate>.json \
  --frontier before-command..after-command \
  --out runs/<synthesis-run-id>/state-seed.json
```

`--profile-kind` is an explicit provenance contract: never label a calibration
or hand-authored smoke run as `synthesis-candidate`. The V2.4 flow starts with
`synthesis schedule`, runs a separately configured generator through
`synthesis generate`, then requires `synthesis evaluate` to observe every atom
at a linked persistent frontier before `synthesis promote` can retain it. The
importer reads only `target-result.json`, `target-task.json`, and
`checkpoint-effect-map.json`; it does not consume legacy scenario or mutation
fields.

For a scheduler candidate whose `target_id` is `langgraph-shell-react` and
whose adapter is `langgraph`, the real candidate profile path is
`synthesis execute-langgraph`. Build the dedicated target image first, then
run the command with host BPF privileges and an explicit model-provider network
exception. The resulting `ProfileRun` is still only profile evidence. A fresh
profile records `persisted_monotonic_ns` for each native checkpoint; use that
evidence to produce a binding only when it brackets the linked effect window:

`synthesis/langgraph-shell-react-scaffold.example.json` is the bounded
generator context for that route. It describes only the target-owned project
surface and prohibitions; it does not prescribe a testcase, an expected
effect, or a recovery query.

```bash
make langgraph-profile-image
make synthesis-langgraph-profile \
  LANGGRAPH_SYNTHESIS_OBJECTIVE=examples/objectives/unix-listener-survival.example.json \
  LANGGRAPH_SYNTHESIS_CANDIDATE=runs/<candidate>.json \
  LANGGRAPH_SYNTHESIS_ROOT=runs/<candidate-profile>

make synthesis-langgraph-bind-frontier \
  LANGGRAPH_SYNTHESIS_OBJECTIVE=examples/objectives/unix-listener-survival.example.json \
  LANGGRAPH_SYNTHESIS_CANDIDATE=runs/<candidate>.json \
  LANGGRAPH_SYNTHESIS_ROOT=runs/<candidate-profile> \
  LANGGRAPH_SYNTHESIS_FRONTIER=before-command..after-command \
  LANGGRAPH_SYNTHESIS_MANIFEST=runs/<candidate-profile>/<target-run-id>/langgraph-native-checkpoints.json \
  LANGGRAPH_SYNTHESIS_BINDING=runs/<candidate-profile>/langgraph-native-frontier-binding.json
```

The first command deliberately invokes the cgroup-scoped collectors through
`sudo --preserve-env` for only the provider variables required by the target.
The binding command is offline; it does not call the model. A binding is not a
recovery result—the fresh-runtime LangGraph fork executor is still pending.

On Linux with a local Docker image and BPF privileges, the real-target smoke
test also emits a checkpoint catalog, probe summaries, normalized effects, and
a checkpoint-effect map:

```bash
GOCACHE=/tmp/syncfuzz-go-cache go build -o /tmp/syncfuzz-ebpf ./cmd/syncfuzz
sudo /tmp/syncfuzz-ebpf target run --env container --profile-processes --profile-resources --command 'sh -c "sleep 1 &"' --out runs
```

The checkpoints above are SyncFuzz controller observation boundaries, not a
claim that the invoked command adapter has a durable Agent checkpoint API.

Future examples should include:

- testcase manifest;
- seed primitive;
- fault phase;
- expected mismatch signature;
- minimized reproduction command.
