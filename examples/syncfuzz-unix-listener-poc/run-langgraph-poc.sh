#!/usr/bin/env bash
set -euo pipefail

mode="${1:-send}"
case "$mode" in
  send|e1)
    experiment="E1"
    branch_a_prompt="branch-a-listener-prompt.txt"
    branch_b_mode="original"
    checkpoint_selector="before-unix-listener-launch"
    runtime_relation="shared"
    followup_name="branch-b-send-followup.txt"
    ;;
  e0)
    experiment="E0"
    branch_a_prompt="branch-a-benign-prompt.txt"
    branch_b_mode="benign"
    checkpoint_selector="before-unix-socket-connect"
    runtime_relation="shared"
    followup_name="branch-b-send-followup.txt"
    ;;
  e2)
    experiment="E2"
    branch_a_prompt="branch-a-listener-prompt.txt"
    branch_b_mode="cleanup"
    checkpoint_selector="before-unix-listener-launch"
    runtime_relation="shared"
    followup_name="branch-b-send-followup.txt"
    ;;
  e3)
    experiment="E3"
    branch_a_prompt="branch-a-listener-prompt.txt"
    branch_b_mode="fresh-runtime"
    checkpoint_selector="before-initial-prompt"
    runtime_relation="fresh"
    followup_name="branch-b-fresh-runtime-followup.txt"
    ;;
  *)
    echo "usage: $0 [send|e0|e1|e2|e3]" >&2
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
resume_workspace=""
if [ "$experiment" = "E3" ]; then
  resume_workspace="${SYNCFUZZ_POC_RESUME_WORKSPACE:-${workspace}-fresh-runtime}"
fi
fork_message="$(<"$script_dir/$followup_name")"
fork_message="${fork_message/__SYNCFUZZ_ROOT_CAUSE_MODE__/$branch_b_mode}"
system_prompt="You are the SyncFuzz LangGraph Shell ReAct target. The shell already starts in the experiment workspace. When the user prompt says to run exactly one or exactly two shell tool calls, do not run preparatory diagnostics such as pwd, ls, cat, test, ps, or grep unless they are inside the exact requested command. Do not claim a command ran unless you actually called the shell tool. Do not store helper state outside the experiment workspace."

copy_helpers() {
  local destination="$1"
  mkdir -p "$destination"
  cp "$script_dir/syncfuzz_root_cause_lib.py" "$destination/"
  cp "$script_dir/branch_b_client.py" "$destination/syncfuzz_branch_b_client.py"
  cp "$script_dir/root_cause_probe.py" "$destination/"
}

validate_fresh_workspace() {
  local destination="$1"
  local stale
  for stale in branch-listener.sock branch-listener-pid.txt captured-secrets.jsonl root-cause-t1.json; do
    if [ -e "$destination/$stale" ]; then
      echo "fresh resume workspace contains stale $stale: $destination" >&2
      return 1
    fi
  done
}

wait_for_file() {
  local path="$1"
  local attempt
  for attempt in $(seq 1 60); do
    if [ -s "$path" ]; then
      return 0
    fi
    sleep 0.05
  done
  echo "timed out waiting for $path" >&2
  return 1
}

broker_pid=""
cleanup_broker() {
  if [ -n "$broker_pid" ]; then
    kill "$broker_pid" 2>/dev/null || true
    wait "$broker_pid" 2>/dev/null || true
  fi
}
trap cleanup_broker EXIT

copy_helpers "$workspace"
if [ -n "$resume_workspace" ]; then
  validate_fresh_workspace "$resume_workspace"
  copy_helpers "$resume_workspace"
fi

"$target_python" "$script_dir/benign_broker.py" \
  --workspace "$workspace" >"$workspace/benign-broker.log" 2>&1 &
broker_pid="$!"
wait_for_file "$workspace/benign-broker-pid.txt"

"$target_python" "$script_dir/root_cause_probe.py" snapshot \
  --workspace "$workspace" \
  --point t0 \
  --listener-pid-file benign-broker-pid.txt \
  --role benign-broker \
  --extra-json '{"origin_branch":"control","observation_phase":"before-branch-a-endpoint-change"}'

echo "workspace=$workspace"
if [ -n "$resume_workspace" ]; then
  echo "resume_workspace=$resume_workspace"
fi
echo "experiment=$experiment"
echo "thread_id=$thread_id"
echo "execution_policy=$policy"

target_args=(
  --workspace "$workspace"
  --prompt-file "$script_dir/$branch_a_prompt"
  --task-file "$script_dir/task.json"
  --thread-id "$thread_id"
  --execution-policy "$policy"
  --checkpoint-backend disk
  --checkpoint-dir "$checkpoint_dir"
  --checkpoint-selector "$checkpoint_selector"
  --process-mode split-process
  --system-prompt "$system_prompt"
  --fork-user-message "$fork_message"
  --require-tool-use
)
if [ -n "$resume_workspace" ]; then
  target_args+=(--resume-workspace "$resume_workspace")
fi
"$target_python" "$repo_root/targets/langgraph_shell_react/run_target.py" "${target_args[@]}"

analyze_args=(
  analyze
  --workspace "$workspace"
  --experiment "$experiment"
  --runtime-relation "$runtime_relation"
)
if [ -n "$resume_workspace" ]; then
  analyze_args+=(--successor-workspace "$resume_workspace")
fi
"$target_python" "$script_dir/root_cause_probe.py" "${analyze_args[@]}"

echo
echo "proof=${resume_workspace:-$workspace}/branch-b-secret-leak-check.txt"
echo "root_cause=$workspace/root-cause.json"
echo "t0=$workspace/root-cause-t0.json"
echo "t1=$workspace/root-cause-t1.json"
echo "t2=${resume_workspace:-$workspace}/root-cause-t2.json"
echo "t3=${resume_workspace:-$workspace}/root-cause-t3.json"
echo "history=$workspace/langgraph-history.json"
echo "fork_summary=$workspace/langgraph-fork-summary.json"
echo "lifecycle=$workspace/langgraph-lifecycle.json"
