# MAF GitHub Copilot Shell Target

This is the first SyncFuzz MAF target.

It is intentionally narrow:

- official `GitHubCopilotAgent` shell sample path
- thin SyncFuzz wrapper
- generic `command` adapter
- no workflow checkpoint logic yet

Current scope:

- `MAF-1` only
- best for shell-task smoke paths with explicit workspace artifacts
- built-in task-compliance checks now cover `orphan-process`, `orphan-process-long-delay`, `persistent-shell-poisoning`, `env-residue`, `function-residue`, `cwd-residue`, and `umask-residue`
- the `orphan-process-long-delay` oracle uses MAF lifecycle evidence plus late observation, rather than assuming LangGraph-style boundary process visibility
- the `persistent-shell-poisoning` oracle now requires lifecycle-backed proof that a later bash call observed the workspace-local git shim without another PATH export
- the `env-residue`, `function-residue`, `cwd-residue`, and `umask-residue` oracles use MAF lifecycle evidence to distinguish true cross-call shell-context carry-over from later bash calls that reconstruct the state before writing the witness
- replay / fork / workflow lifecycle tasks are still out of scope here

## Setup

Create a Python environment and install the official MAF packages:

```bash
python3 -m venv targets/maf_github_copilot_shell/venv
. targets/maf_github_copilot_shell/venv/bin/activate
pip install -r targets/maf_github_copilot_shell/requirements.txt
```

You also need a working GitHub Copilot CLI installation.
The executable must be the Copilot CLI runtime, normally named `copilot`; the regular GitHub CLI `gh` is not compatible with this provider because MAF starts the CLI with Copilot-specific headless flags.

SyncFuzz can drive MAF in two backend modes:

- default Copilot-managed provider flow
- custom OpenAI-compatible provider flow using Copilot's official `COPILOT_*` provider variables

The Copilot CLI is still required in both modes because the MAF runtime is launched through it.

If the executable is not named `copilot` on your machine, point the wrapper at the real one:

```bash
make target-maf-github-copilot-shell-check MAF_COPILOT_CLI=/abs/path/to/your/copilot-cli
```

Optional wrapper environment variables:

```bash
MAF_PYTHON=/abs/path/to/python
MAF_COPILOT_CLI=copilot
OPENAI_API_KEY=
OPENAI_BASE_URL=https://api.example.com/v1
COPILOT_PROVIDER_TYPE=openai
COPILOT_MODEL=gpt-4.1-mini
MAF_TIMEOUT=
MAF_SESSION_HOME=
MAF_LOG_LEVEL=
```

`MAF_SESSION_HOME` is forwarded to the Copilot SDK as its
`base_directory` / `COPILOT_HOME`.
If `MAF_TIMEOUT` is unset, the wrapper derives a Copilot request timeout
from the SyncFuzz task timeout and keeps a small margin for artifact flush.
For custom-provider runs, SyncFuzz first reuses the generic
`OPENAI_API_KEY` and `OPENAI_BASE_URL` values, then lets
`COPILOT_PROVIDER_API_KEY` and `COPILOT_PROVIDER_BASE_URL` override them
when you need MAF-specific settings. Keep `COPILOT_PROVIDER_TYPE` and
`COPILOT_MODEL` as the Copilot-side controls. The wrapper mirrors the
resolved provider settings into both the MAF SDK layer and the Copilot CLI
runtime so they stay aligned.

Check the local environment:

```bash
make target-maf-github-copilot-shell-check
```

## Run Through SyncFuzz

Use the prepared command file:

```bash
make target-maf-github-copilot-shell
make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning
make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning MAF_TIMEOUT=110
make target-maf-github-copilot-shell TARGET_TASK=env-residue
make target-maf-github-copilot-shell TARGET_TASK=function-residue
make target-maf-github-copilot-shell TARGET_TASK=cwd-residue
make target-maf-github-copilot-shell TARGET_TASK=umask-residue
make target-maf-github-copilot-shell TARGET_TASK=unix-listener-residue
```

For repeatability checks and small baseline batches:

```bash
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-baseline REPEAT=3
make target-maf-github-copilot-shell-suite TARGET_GROUP=maf-shell-context REPEAT=1
make target-maf-github-copilot-shell-matrix-suite TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all REPEAT=1 CANDIDATE_LIMIT=3
make target-maf-github-copilot-shell-campaign TARGET_GROUP=maf-baseline TARGET_PROMPT_PROFILES=all ROUNDS=2 CANDIDATE_LIMIT=3
```

That expands to a normal SyncFuzz real-target run using:

- `--target maf-github-copilot-shell`
- `--task orphan-process`
- `--command-file examples/target-commands/maf-github-copilot-shell.sh`

You can also run it directly:

```bash
go run ./cmd/syncfuzz target run \
  --target maf-github-copilot-shell \
  --task orphan-process \
  --command-file examples/target-commands/maf-github-copilot-shell.sh \
  --observe-delay 500ms \
  --out runs
```

The wrapper writes three MAF-specific workspace artifacts:

- `maf-run-summary.json`
- `maf-session.json`
- `maf-lifecycle.json`

These are not yet workflow/checkpoint artifacts. They are the MAF-1 bridge that records:

- provider wrapper configuration
- whether the run used the default Copilot backend or a custom OpenAI-compatible backend
- any discovered provider session identity
- pre-tool / permission callback events and run lifecycle

The built-in `maf-baseline` target group currently expands to:

- `orphan-process`
- `orphan-process-long-delay`

The broader `maf-shell-context` group currently expands to:

- `orphan-process`
- `orphan-process-long-delay`
- `persistent-shell-poisoning`
- `env-residue`
- `function-residue`
- `cwd-residue`
- `umask-residue`

## Current Limits

- no `MAF-2` session-restore flow yet
- no `MAF-3` workflow/checkpoint flow yet
- unsupported LangGraph-specific lifecycle tasks are rejected by default
- the Copilot CLI is still part of the runtime path even when SyncFuzz routes inference through `COPILOT_PROVIDER_BASE_URL`
- if a run reports `Session was not created with authentication info or custom provider`, check the shared `.env` values first when using a custom endpoint, otherwise authenticate or configure the Copilot CLI outside SyncFuzz
