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

You also need a working GitHub Copilot CLI installation and sign-in outside SyncFuzz.
The executable must be the Copilot CLI runtime, normally named `copilot`; the regular GitHub CLI `gh` is not compatible with this provider because MAF starts the CLI with Copilot-specific headless flags.

If the executable is not named `copilot` on your machine, point the wrapper at the real one:

```bash
make target-maf-github-copilot-shell-check MAF_COPILOT_CLI=/abs/path/to/your/copilot-cli
```

Optional wrapper environment variables:

```bash
SYNCFUZZ_MAF_PYTHON=/abs/path/to/python
SYNCFUZZ_MAF_COPILOT_CLI=copilot
SYNCFUZZ_MAF_MODEL=
SYNCFUZZ_MAF_SESSION_HOME=
SYNCFUZZ_MAF_LOG_LEVEL=
```

`SYNCFUZZ_MAF_SESSION_HOME` is forwarded to the Copilot SDK as its
`base_directory` / `COPILOT_HOME`.

Check the local environment:

```bash
make target-maf-github-copilot-shell-check
```

## Run Through SyncFuzz

Use the prepared command file:

```bash
make target-maf-github-copilot-shell
make target-maf-github-copilot-shell TARGET_TASK=persistent-shell-poisoning
make target-maf-github-copilot-shell TARGET_TASK=env-residue
make target-maf-github-copilot-shell TARGET_TASK=function-residue
make target-maf-github-copilot-shell TARGET_TASK=cwd-residue
make target-maf-github-copilot-shell TARGET_TASK=umask-residue
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
- provider authentication is owned by the Copilot CLI; if a run reports `Session was not created with authentication info or custom provider`, authenticate or configure the CLI outside SyncFuzz first
