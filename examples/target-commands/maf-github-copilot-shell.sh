syncfuzz_maf_python="${MAF_PYTHON-}"
syncfuzz_maf_python_set="${MAF_PYTHON+x}"
syncfuzz_copilot_model="${COPILOT_MODEL-}"
syncfuzz_copilot_model_set="${COPILOT_MODEL+x}"
syncfuzz_copilot_provider_base_url="${COPILOT_PROVIDER_BASE_URL-}"
syncfuzz_copilot_provider_base_url_set="${COPILOT_PROVIDER_BASE_URL+x}"
syncfuzz_copilot_provider_type="${COPILOT_PROVIDER_TYPE-}"
syncfuzz_copilot_provider_type_set="${COPILOT_PROVIDER_TYPE+x}"
syncfuzz_copilot_provider_api_key="${COPILOT_PROVIDER_API_KEY-}"
syncfuzz_copilot_provider_api_key_set="${COPILOT_PROVIDER_API_KEY+x}"
syncfuzz_maf_copilot_cli="${MAF_COPILOT_CLI-}"
syncfuzz_maf_copilot_cli_set="${MAF_COPILOT_CLI+x}"
syncfuzz_maf_session_home="${MAF_SESSION_HOME-}"
syncfuzz_maf_session_home_set="${MAF_SESSION_HOME+x}"
syncfuzz_maf_log_level="${MAF_LOG_LEVEL-}"
syncfuzz_maf_log_level_set="${MAF_LOG_LEVEL+x}"
syncfuzz_maf_allow_unsupported="${MAF_ALLOW_UNSUPPORTED_TASKS-}"
syncfuzz_maf_allow_unsupported_set="${MAF_ALLOW_UNSUPPORTED_TASKS+x}"

if [ -n "${SYNCFUZZ_REPO_ROOT:-}" ] && [ -f "$SYNCFUZZ_REPO_ROOT/.env" ]; then
  set -a
  . "$SYNCFUZZ_REPO_ROOT/.env"
  set +a
fi

if [ "$syncfuzz_maf_python_set" = "x" ]; then
  MAF_PYTHON="$syncfuzz_maf_python"
  export MAF_PYTHON
fi
if [ "$syncfuzz_copilot_model_set" = "x" ]; then
  COPILOT_MODEL="$syncfuzz_copilot_model"
  export COPILOT_MODEL
fi
if [ "$syncfuzz_copilot_provider_base_url_set" = "x" ]; then
  COPILOT_PROVIDER_BASE_URL="$syncfuzz_copilot_provider_base_url"
  export COPILOT_PROVIDER_BASE_URL
fi
if [ "$syncfuzz_copilot_provider_type_set" = "x" ]; then
  COPILOT_PROVIDER_TYPE="$syncfuzz_copilot_provider_type"
  export COPILOT_PROVIDER_TYPE
fi
if [ "$syncfuzz_copilot_provider_api_key_set" = "x" ]; then
  COPILOT_PROVIDER_API_KEY="$syncfuzz_copilot_provider_api_key"
  export COPILOT_PROVIDER_API_KEY
fi
if [ "$syncfuzz_maf_copilot_cli_set" = "x" ]; then
  MAF_COPILOT_CLI="$syncfuzz_maf_copilot_cli"
  export MAF_COPILOT_CLI
fi
if [ "$syncfuzz_maf_session_home_set" = "x" ]; then
  MAF_SESSION_HOME="$syncfuzz_maf_session_home"
  export MAF_SESSION_HOME
fi
if [ "$syncfuzz_maf_log_level_set" = "x" ]; then
  MAF_LOG_LEVEL="$syncfuzz_maf_log_level"
  export MAF_LOG_LEVEL
fi
if [ "$syncfuzz_maf_allow_unsupported_set" = "x" ]; then
  MAF_ALLOW_UNSUPPORTED_TASKS="$syncfuzz_maf_allow_unsupported"
  export MAF_ALLOW_UNSUPPORTED_TASKS
fi

target_python="${MAF_PYTHON:-}"

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

if [ -n "${COPILOT_MODEL:-}" ]; then
  set -- "$@" --model "$COPILOT_MODEL"
fi
if [ -n "${MAF_COPILOT_CLI:-}" ]; then
  set -- "$@" --copilot-cli "$MAF_COPILOT_CLI"
fi
if [ -n "${MAF_SESSION_HOME:-}" ]; then
  set -- "$@" --session-home "$MAF_SESSION_HOME"
fi
if [ -n "${MAF_LOG_LEVEL:-}" ]; then
  set -- "$@" --log-level "$MAF_LOG_LEVEL"
fi
if [ "${MAF_ALLOW_UNSUPPORTED_TASKS:-}" = "1" ] || [ "${MAF_ALLOW_UNSUPPORTED_TASKS:-}" = "true" ]; then
  set -- "$@" --allow-unsupported-task
fi

"$@"
