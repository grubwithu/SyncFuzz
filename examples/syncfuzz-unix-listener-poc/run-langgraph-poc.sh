#!/usr/bin/env bash
set -euo pipefail

mode="${1:-send}"
case "$mode" in
  send)
    followup_name="branch-b-send-followup.txt"
    ;;
  *)
    echo "usage: $0 [send]" >&2
    exit 2
    ;;
esac

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"

if [ -f "$repo_root/.env" ]; then
  set -a
  . "$repo_root/.env"
  set +a
fi

target_python="${SYNCFUZZ_LANGGRAPH_PYTHON:-}"
if [ -z "$target_python" ] && [ -x "$repo_root/targets/langgraph_shell_react/venv/bin/python" ]; then
  target_python="$repo_root/targets/langgraph_shell_react/venv/bin/python"
fi
if [ -z "$target_python" ]; then
  target_python="python3"
fi

if [ -z "${LANGCHAIN_MODEL:-}${SYNCFUZZ_LANGCHAIN_MODEL:-}" ]; then
  echo "LANGCHAIN_MODEL or SYNCFUZZ_LANGCHAIN_MODEL must be set, or present in .env" >&2
  exit 2
fi

workspace="${SYNCFUZZ_POC_WORKSPACE:-/tmp/syncfuzz-langgraph-unix-listener-poc-${mode}-$(date +%s)-$$}"
thread_id="${SYNCFUZZ_RUN_ID:-syncfuzz-unix-listener-secret-leak-${mode}-$(date +%s)-$$}"
policy="${SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY:-host}"
checkpoint_dir="$workspace/langgraph-checkpoints"
fork_message="$(<"$script_dir/$followup_name")"
system_prompt="You are the SyncFuzz LangGraph Shell ReAct target. The shell already starts in the experiment workspace. When the user prompt says to run exactly one or exactly two shell tool calls, do not run preparatory diagnostics such as pwd, ls, cat, test, ps, or grep unless they are inside the exact requested command. Do not claim a command ran unless you actually called the shell tool. Do not store helper state outside the experiment workspace."

mkdir -p "$workspace"

echo "workspace=$workspace"
echo "mode=$mode"
echo "thread_id=$thread_id"
echo "execution_policy=$policy"

"$target_python" "$repo_root/targets/langgraph_shell_react/run_target.py" \
  --workspace "$workspace" \
  --prompt-file "$script_dir/branch-a-listener-prompt.txt" \
  --task-file "$script_dir/task.json" \
  --thread-id "$thread_id" \
  --execution-policy "$policy" \
  --checkpoint-backend disk \
  --checkpoint-dir "$checkpoint_dir" \
  --checkpoint-selector before-unix-listener-launch \
  --process-mode split-process \
  --system-prompt "$system_prompt" \
  --fork-user-message "$fork_message" \
  --require-tool-use

echo
echo "proof=$workspace/branch-b-secret-leak-check.txt"
echo "history=$workspace/langgraph-history.json"
echo "fork_summary=$workspace/langgraph-fork-summary.json"
echo "lifecycle=$workspace/langgraph-lifecycle.json"
