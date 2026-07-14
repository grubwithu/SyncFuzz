# MAF Workflow Checkpoint Target

This target exercises the official Microsoft Agent Framework Workflow API without
calling an LLM. It builds a minimal two-executor graph, stores file checkpoints,
recreates the workflow object, and restores from the selected checkpoint.

Current task:

- `maf-workflow-checkpoint-continuity`
- `maf-workflow-external-effect-replay`
- `maf-workflow-http-effect-replay`
- `maf-workflow-resource-replay`
- `maf-workflow-partial-commit-replay`
- `maf-workflow-approval-pending-replay`
- `maf-workflow-rehydrate-divergence`

The wrapper writes:

- `maf-workflow-effect.txt`
- `maf-workflow-continuity-check.txt`
- `maf-workflow-summary.json`
- `maf-workflow-checkpoints/*.json`
- `maf-workflow-external-ledger.jsonl` for the external-effect replay task
- `maf-workflow-external-replay-check.txt` for the external-effect replay task
- `maf-workflow-http-ledger.jsonl` for the HTTP external-service replay task
- `maf-workflow-http-replay-check.txt` for the HTTP external-service replay task
- `maf-workflow-resource-ledger.jsonl` for the resource-service replay task
- `maf-workflow-resource-replay-check.txt` for the resource-service replay task
- `maf-workflow-partial-commit-check.txt` for the partial-commit replay task
- `maf-workflow-approval-pending-check.txt` for the approval-pending replay task
- `maf-workflow-rehydrate-divergence-check.txt` for the resume-vs-rehydrate divergence task

Run the import check:

```bash
make target-maf-workflow-checkpoint-check
```

Run the target:

```bash
make target-maf-workflow-checkpoint
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-external-effect-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-resource-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-partial-commit-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-approval-pending-replay
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-rehydrate-divergence
```

`maf-workflow-http-effect-replay` and `maf-workflow-resource-replay` can call a real mock service process instead
of the default in-process fallback. Start the mock server in one terminal:

```bash
make mock-build
SYNCFUZZ_MOCK_PORT=8910 SYNCFUZZ_MOCK_DB=/tmp/syncfuzz-maf-workflow.json make mock-start
```

Then run the workflow task against that service:

```bash
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-http-effect-replay MAF_WORKFLOW_EFFECT_SERVICE_URL=http://127.0.0.1:8910
make target-maf-workflow-checkpoint TARGET_TASK=maf-workflow-resource-replay MAF_WORKFLOW_EFFECT_SERVICE_URL=http://127.0.0.1:8910
```

When `MAF_WORKFLOW_EFFECT_SERVICE_URL` is unset, the target starts a local
in-process HTTP service so the task remains self-contained.

This target reuses `MAF_PYTHON` when set. Otherwise it falls back to the Python
environment under `targets/maf_github_copilot_shell/venv`, because that venv
already contains the official `agent-framework` package.
