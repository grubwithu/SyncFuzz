syncfuzz_maf_python="${MAF_PYTHON-}"
syncfuzz_maf_python_set="${MAF_PYTHON+x}"
syncfuzz_maf_workflow_pre_timeout="${MAF_WORKFLOW_PRE_TIMEOUT-}"
syncfuzz_maf_workflow_pre_timeout_set="${MAF_WORKFLOW_PRE_TIMEOUT+x}"
syncfuzz_maf_workflow_restore_timeout="${MAF_WORKFLOW_RESTORE_TIMEOUT-}"
syncfuzz_maf_workflow_restore_timeout_set="${MAF_WORKFLOW_RESTORE_TIMEOUT+x}"

if [ -n "${SYNCFUZZ_REPO_ROOT:-}" ] && [ -f "$SYNCFUZZ_REPO_ROOT/.env" ]; then
  set -a
  . "$SYNCFUZZ_REPO_ROOT/.env"
  set +a
fi

if [ "$syncfuzz_maf_python_set" = "x" ]; then
  MAF_PYTHON="$syncfuzz_maf_python"
  export MAF_PYTHON
fi
if [ "$syncfuzz_maf_workflow_pre_timeout_set" = "x" ]; then
  MAF_WORKFLOW_PRE_TIMEOUT="$syncfuzz_maf_workflow_pre_timeout"
  export MAF_WORKFLOW_PRE_TIMEOUT
fi
if [ "$syncfuzz_maf_workflow_restore_timeout_set" = "x" ]; then
  MAF_WORKFLOW_RESTORE_TIMEOUT="$syncfuzz_maf_workflow_restore_timeout"
  export MAF_WORKFLOW_RESTORE_TIMEOUT
fi

target_python="${MAF_PYTHON:-}"

if [ -z "$target_python" ] && [ -x "$SYNCFUZZ_REPO_ROOT/targets/maf_github_copilot_shell/venv/bin/python" ]; then
  target_python="$SYNCFUZZ_REPO_ROOT/targets/maf_github_copilot_shell/venv/bin/python"
fi

if [ -z "$target_python" ]; then
  target_python="python3"
fi

set -- \
  "$target_python" "$SYNCFUZZ_REPO_ROOT/targets/maf_workflow_checkpoint/run_target.py" \
  --workspace "$SYNCFUZZ_WORKSPACE" \
  --prompt-file "$SYNCFUZZ_PROMPT_FILE" \
  --task-file "$SYNCFUZZ_TASK_FILE"

"$@"
