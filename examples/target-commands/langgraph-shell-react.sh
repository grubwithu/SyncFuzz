syncfuzz_openai_api_key="${OPENAI_API_KEY-}"
syncfuzz_openai_api_key_set="${OPENAI_API_KEY+x}"
syncfuzz_openai_admin_key="${OPENAI_ADMIN_KEY-}"
syncfuzz_openai_admin_key_set="${OPENAI_ADMIN_KEY+x}"
syncfuzz_openai_base_url="${OPENAI_BASE_URL-}"
syncfuzz_openai_base_url_set="${OPENAI_BASE_URL+x}"
syncfuzz_anthropic_api_key="${ANTHROPIC_API_KEY-}"
syncfuzz_anthropic_api_key_set="${ANTHROPIC_API_KEY+x}"
syncfuzz_langchain_model="${LANGCHAIN_MODEL-}"
syncfuzz_langchain_model_set="${LANGCHAIN_MODEL+x}"

if [ -n "${SYNCFUZZ_REPO_ROOT:-}" ] && [ -f "$SYNCFUZZ_REPO_ROOT/.env" ]; then
  set -a
  . "$SYNCFUZZ_REPO_ROOT/.env"
  set +a
fi

if [ "$syncfuzz_openai_api_key_set" = "x" ]; then
  OPENAI_API_KEY="$syncfuzz_openai_api_key"
  export OPENAI_API_KEY
fi
if [ "$syncfuzz_openai_admin_key_set" = "x" ]; then
  OPENAI_ADMIN_KEY="$syncfuzz_openai_admin_key"
  export OPENAI_ADMIN_KEY
fi
if [ "$syncfuzz_openai_base_url_set" = "x" ]; then
  OPENAI_BASE_URL="$syncfuzz_openai_base_url"
  export OPENAI_BASE_URL
fi
if [ "$syncfuzz_anthropic_api_key_set" = "x" ]; then
  ANTHROPIC_API_KEY="$syncfuzz_anthropic_api_key"
  export ANTHROPIC_API_KEY
fi
if [ "$syncfuzz_langchain_model_set" = "x" ]; then
  LANGCHAIN_MODEL="$syncfuzz_langchain_model"
  export LANGCHAIN_MODEL
fi

policy="${SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY:-host}"
target_python="${SYNCFUZZ_LANGGRAPH_PYTHON:-}"

if [ -z "$target_python" ] && [ -x "$SYNCFUZZ_REPO_ROOT/targets/langgraph_shell_react/venv/bin/python" ]; then
  target_python="$SYNCFUZZ_REPO_ROOT/targets/langgraph_shell_react/venv/bin/python"
fi

if [ -z "$target_python" ]; then
  target_python="python3"
fi

set -- \
  "$target_python" "$SYNCFUZZ_REPO_ROOT/targets/langgraph_shell_react/run_target.py" \
  --workspace "$SYNCFUZZ_WORKSPACE" \
  --prompt-file "$SYNCFUZZ_PROMPT_FILE" \
  --task-file "$SYNCFUZZ_TASK_FILE" \
  --thread-id "$SYNCFUZZ_RUN_ID" \
  --execution-policy "$policy"

if [ "$policy" = "docker" ] && [ -n "${SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE:-}" ]; then
  set -- "$@" --docker-image "$SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE"
fi

"$@"
