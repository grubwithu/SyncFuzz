syncfuzz_maf_python="${SYNCFUZZ_MAF_PYTHON-}"
syncfuzz_maf_python_set="${SYNCFUZZ_MAF_PYTHON+x}"
syncfuzz_maf_model="${SYNCFUZZ_MAF_MODEL-}"
syncfuzz_maf_model_set="${SYNCFUZZ_MAF_MODEL+x}"
syncfuzz_maf_copilot_cli="${SYNCFUZZ_MAF_COPILOT_CLI-}"
syncfuzz_maf_copilot_cli_set="${SYNCFUZZ_MAF_COPILOT_CLI+x}"
syncfuzz_maf_session_home="${SYNCFUZZ_MAF_SESSION_HOME-}"
syncfuzz_maf_session_home_set="${SYNCFUZZ_MAF_SESSION_HOME+x}"
syncfuzz_maf_log_level="${SYNCFUZZ_MAF_LOG_LEVEL-}"
syncfuzz_maf_log_level_set="${SYNCFUZZ_MAF_LOG_LEVEL+x}"
syncfuzz_maf_allow_unsupported="${SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS-}"
syncfuzz_maf_allow_unsupported_set="${SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS+x}"

if [ -n "${SYNCFUZZ_REPO_ROOT:-}" ] && [ -f "$SYNCFUZZ_REPO_ROOT/.env" ]; then
  set -a
  . "$SYNCFUZZ_REPO_ROOT/.env"
  set +a
fi

if [ "$syncfuzz_maf_python_set" = "x" ]; then
  SYNCFUZZ_MAF_PYTHON="$syncfuzz_maf_python"
  export SYNCFUZZ_MAF_PYTHON
fi
if [ "$syncfuzz_maf_model_set" = "x" ]; then
  SYNCFUZZ_MAF_MODEL="$syncfuzz_maf_model"
  export SYNCFUZZ_MAF_MODEL
fi
if [ "$syncfuzz_maf_copilot_cli_set" = "x" ]; then
  SYNCFUZZ_MAF_COPILOT_CLI="$syncfuzz_maf_copilot_cli"
  export SYNCFUZZ_MAF_COPILOT_CLI
fi
if [ "$syncfuzz_maf_session_home_set" = "x" ]; then
  SYNCFUZZ_MAF_SESSION_HOME="$syncfuzz_maf_session_home"
  export SYNCFUZZ_MAF_SESSION_HOME
fi
if [ "$syncfuzz_maf_log_level_set" = "x" ]; then
  SYNCFUZZ_MAF_LOG_LEVEL="$syncfuzz_maf_log_level"
  export SYNCFUZZ_MAF_LOG_LEVEL
fi
if [ "$syncfuzz_maf_allow_unsupported_set" = "x" ]; then
  SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS="$syncfuzz_maf_allow_unsupported"
  export SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS
fi

target_python="${SYNCFUZZ_MAF_PYTHON:-}"

if [ -z "$target_python" ] && [ -x "$SYNCFUZZ_REPO_ROOT/targets/maf_github_copilot_shell/venv/bin/python" ]; then
  target_python="$SYNCFUZZ_REPO_ROOT/targets/maf_github_copilot_shell/venv/bin/python"
fi

if [ -z "$target_python" ]; then
  target_python="python3"
fi

set -- \
  "$target_python" "$SYNCFUZZ_REPO_ROOT/targets/maf_github_copilot_shell/run_target.py" \
  --workspace "$SYNCFUZZ_WORKSPACE" \
  --prompt-file "$SYNCFUZZ_PROMPT_FILE" \
  --task-file "$SYNCFUZZ_TASK_FILE"

if [ -n "${SYNCFUZZ_MAF_MODEL:-}" ]; then
  set -- "$@" --model "$SYNCFUZZ_MAF_MODEL"
fi
if [ -n "${SYNCFUZZ_MAF_COPILOT_CLI:-}" ]; then
  set -- "$@" --copilot-cli "$SYNCFUZZ_MAF_COPILOT_CLI"
fi
if [ -n "${SYNCFUZZ_MAF_SESSION_HOME:-}" ]; then
  set -- "$@" --session-home "$SYNCFUZZ_MAF_SESSION_HOME"
fi
if [ -n "${SYNCFUZZ_MAF_LOG_LEVEL:-}" ]; then
  set -- "$@" --log-level "$SYNCFUZZ_MAF_LOG_LEVEL"
fi
if [ "${SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS:-}" = "1" ] || [ "${SYNCFUZZ_MAF_ALLOW_UNSUPPORTED_TASKS:-}" = "true" ]; then
  set -- "$@" --allow-unsupported-task
fi

"$@"
