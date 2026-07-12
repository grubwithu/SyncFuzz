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

Future examples should include:

- testcase manifest;
- seed primitive;
- fault phase;
- expected mismatch signature;
- minimized reproduction command.
