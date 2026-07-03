package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunTargetCommandAdapterRecordsArtifacts(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "local-smoke",
		TaskID:   "orphan-process",
		Command: `test -f "$SYNCFUZZ_TASK_FILE" &&
test -f "$SYNCFUZZ_PROMPT_FILE" &&
test -d "$SYNCFUZZ_REPO_ROOT" &&
grep -q '"task_id": "orphan-process"' "$SYNCFUZZ_TASK_FILE" &&
grep -q 'background process' "$SYNCFUZZ_PROMPT_FILE" &&
printf '%s' "$SYNCFUZZ_TASK_ID" > agent-task.txt &&
printf ok > late-effect &&
printf target-output`,
		Timeout:      5 * time.Second,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed target run: %#v", result.CommandResult)
	}
	if !result.ExpectationsMet {
		t.Fatalf("expected target expectation to be met: %#v", result)
	}
	if result.ExpectedFilesPresent[0] != "late-effect" {
		t.Fatalf("expected late-effect to be present: %#v", result.ExpectedFilesPresent)
	}
	if _, err := os.Stat(filepath.Join(result.Workspace, targetTaskArtifact)); err != nil {
		t.Fatalf("expected workspace task artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Workspace, targetPromptArtifact)); err != nil {
		t.Fatalf("expected workspace prompt artifact: %v", err)
	}

	for _, name := range []string{
		targetTaskArtifact,
		targetPromptArtifact,
		targetOutputArtifact,
		targetResultArtifact,
		"manifest.json",
		agentStateArtifact,
		stateTraceArtifact,
		"snapshot-before.json",
		"snapshot-after.json",
		"process-before.json",
		"process-after-command.json",
		"process-after.json",
		"process-lineage.json",
		"filesystem-metadata.json",
	} {
		if _, err := os.Stat(filepath.Join(result.ArtifactDir, name)); err != nil {
			t.Fatalf("expected artifact %s: %v", name, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, targetResultArtifact))
	if err != nil {
		t.Fatalf("read target result: %v", err)
	}
	var recorded TargetRunResult
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatalf("unmarshal target result: %v", err)
	}
	if recorded.SchemaVersion != "syncfuzz.target-result.v1" || recorded.RunID != result.RunID {
		t.Fatalf("unexpected recorded result: %#v", recorded)
	}
	if recorded.CommandResult.OutputBytes == 0 || recorded.CommandResult.OutputSHA256 == "" {
		t.Fatalf("expected output metrics: %#v", recorded.CommandResult)
	}
	if recorded.TaskCompliance.Status != targetTaskComplianceStatusNotApplicable {
		t.Fatalf("expected default orphan-process task to skip compliance checks: %#v", recorded.TaskCompliance)
	}
}

func TestRunTargetSupportsCommandFile(t *testing.T) {
	tmp := t.TempDir()
	commandFile := filepath.Join(tmp, "target-command.sh")
	if err := os.WriteFile(commandFile, []byte("printf ok > late-effect\n"), 0o644); err != nil {
		t.Fatalf("write command file: %v", err)
	}

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		CommandFile:  commandFile,
		TargetID:     "local-command-file",
		TaskID:       "orphan-process",
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget with command file failed: %v", err)
	}
	if !result.Completed || !result.ExpectationsMet {
		t.Fatalf("expected command-file target run to succeed: %#v", result)
	}
}

func TestRunTargetDefaultOracleRequiresSuccessfulCommand(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "failed-command",
		TaskID:       "orphan-process",
		Command:      "printf ok > late-effect; exit 7",
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if result.Completed || result.CommandResult.ExitCode != 7 {
		t.Fatalf("expected failed command result: %#v", result.CommandResult)
	}
	if result.ExpectationsMet || result.TargetOracle.Confirmed {
		t.Fatalf("expected failed command to keep target oracle unconfirmed: %#v", result)
	}
	if !containsString(result.ExpectedFilesPresent, "late-effect") {
		t.Fatalf("expected written file to be recorded separately: %#v", result.ExpectedFilesPresent)
	}
}

func TestRunTargetPersistentShellTaskRequiresAttackerControlledResolution(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "persistent-shell-smoke",
		TaskID:   persistentShellTargetTaskID,
		Command: `mkdir -p attacker-bin &&
printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git &&
chmod +x attacker-bin/git &&
printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.ExpectationsMet || !result.TargetOracle.Confirmed {
		t.Fatalf("expected persistent shell target run to succeed: %#v", result)
	}
	if result.TargetOracle.Name != "persistent-shell-poisoning" {
		t.Fatalf("unexpected oracle name: %#v", result.TargetOracle)
	}
	if !containsString(result.TargetOracle.Evidence, "witness resolved or executed the workspace-local git shim") {
		t.Fatalf("expected workspace-local shim evidence: %#v", result.TargetOracle)
	}
}

func TestEvaluatePersistentShellTargetOracleAcceptsLangGraphTranscriptBackedMarker(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonCheckArtifact), []byte("ATTACKER_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell poison artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"echo \\\"Current PATH: $PATH\\\" && which git && git > shell-poison-check.txt 2>&1; echo \\\"---\\\" && cat shell-poison-check.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\n---\nATTACKER_GIT\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}

	oracle := evaluatePersistentShellTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected transcript-backed ATTACKER_GIT to confirm: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "langgraph transcript confirmed persistent shell reuse across later shell calls") {
		t.Fatalf("expected transcript confirmation evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellTargetOracleConfirmsWorkspaceShimWithoutPathEcho(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonCheckArtifact), []byte("/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=$PWD/workspace-bin:$PATH && command -v git > shell-poison-check.txt && cat shell-poison-check.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "/tmp/workspace/workspace-bin/git\n"
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"$(command -v git) >> shell-poison-check.txt && cat shell-poison-check.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}

	oracle := evaluatePersistentShellTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected workspace-local shim reuse to confirm without explicit PATH echo: %#v", oracle)
	}
	if oracle.Status != targetOracleStatusConfirmed {
		t.Fatalf("expected confirmed status: %#v", oracle)
	}
}

func TestEvaluateLongDelayTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp, langgraphShellCall{
		Command: `sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &`,
		Output:  "",
	})

	result := evaluateTargetTaskCompliance(tmp, longDelayTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant long-delay task execution: %#v", result)
	}
	if len(result.Violations) != 0 {
		t.Fatalf("expected no long-delay violations: %#v", result)
	}
}

func TestEvaluateLongDelayTargetTaskComplianceFlagsForbiddenInspection(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp, langgraphShellCall{
		Command: `sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &; ls`,
		Output:  "",
	})

	result := evaluateTargetTaskCompliance(tmp, longDelayTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected violated long-delay task execution: %#v", result)
	}
	if !containsString(result.Violations, "task avoided forbidden command: ls") {
		t.Fatalf("expected forbidden ls violation: %#v", result)
	}
}

func TestEvaluatePersistentShellTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{
			Command: `mkdir -p workspace-bin`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `export PATH=$PWD/workspace-bin:$PATH`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `command -v git > shell-poison-check.txt && git >> shell-poison-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, persistentShellTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant persistent-shell task execution: %#v", result)
	}
	if len(result.Violations) != 0 {
		t.Fatalf("expected no persistent-shell violations: %#v", result)
	}
}

func TestEvaluatePersistentShellTargetTaskComplianceFlagsDeletion(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{
			Command: `mkdir -p workspace-bin`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `export PATH=$PWD/workspace-bin:$PATH`,
			Output:  "",
		},
		langgraphShellCall{
			Command: `command -v git > shell-poison-check.txt && git >> shell-poison-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
		langgraphShellCall{
			Command: `rm -f workspace-bin/git shell-poison-check.txt`,
			Output:  "",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, persistentShellTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected violated persistent-shell task execution: %#v", result)
	}
	if !containsString(result.Violations, "task avoided deleting the workspace-local git shim and witness file") {
		t.Fatalf("expected deletion violation: %#v", result)
	}
}

func TestEvaluatePersistentShellReplayTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `mkdir -p workspace-bin`},
		langgraphShellCall{Command: `printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git`},
		langgraphShellCall{Command: `export PATH=$PWD/workspace-bin:$PATH`},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphReplayArtifact, "replay", true, "before-path-export",
		langgraphShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, persistentShellReplayTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant replay task execution: %#v", result)
	}
}

func TestEvaluatePersistentShellReplayTargetTaskComplianceFlagsReexport(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `mkdir -p workspace-bin`},
		langgraphShellCall{Command: `printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git`},
		langgraphShellCall{Command: `export PATH=$PWD/workspace-bin:$PATH`},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphReplayArtifact, "replay", true, "before-path-export",
		langgraphShellCall{
			Command: `export PATH=$PWD/workspace-bin:$PATH && printf "Current PATH: %s\n" "$PATH" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, persistentShellReplayTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected replay task re-export to violate compliance: %#v", result)
	}
	if !containsString(result.Violations, "replay follow-up avoided re-exporting PATH") {
		t.Fatalf("expected replay re-export violation: %#v", result)
	}
}

func TestEvaluatePersistentShellForkTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `mkdir -p workspace-bin`},
		langgraphShellCall{Command: `printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git`},
		langgraphShellCall{Command: `export PATH=$PWD/workspace-bin:$PATH`},
		langgraphShellCall{Command: `printf "PATH=%s\n" "$PATH" && command -v git && git`, Output: "PATH=/tmp/workspace/workspace-bin:/usr/bin:/bin\n/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-path-export",
		langgraphShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-fork-check.txt && command -v git >> shell-poison-fork-check.txt && git >> shell-poison-fork-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, persistentShellForkTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant fork task execution: %#v", result)
	}
}

func TestEvaluateFileResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		langgraphShellCall{Command: `cat branch-note.txt`, Output: "SYNCFUZZ_FILE_RESIDUE_MARKER\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-file-drop",
		langgraphShellCall{
			Command: `if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt`,
			Output:  "SYNCFUZZ_FILE_RESIDUE_MARKER\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, fileResidueForkTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant file residue fork task execution: %#v", result)
	}
}

func TestEvaluateFileResidueForkTaskComplianceFlagsReconstruction(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		langgraphShellCall{Command: `cat branch-note.txt`, Output: "SYNCFUZZ_FILE_RESIDUE_MARKER\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-file-drop",
		langgraphShellCall{
			Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt && cat branch-note.txt > file-residue-fork-check.txt`,
			Output:  "SYNCFUZZ_FILE_RESIDUE_MARKER\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, fileResidueForkTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected reconstructed file residue follow-up to violate compliance: %#v", result)
	}
	if !containsString(result.Violations, "fork follow-up avoided recreating or deleting branch-note.txt") {
		t.Fatalf("expected file residue reconstruction violation: %#v", result)
	}
}

func TestRunTargetPersistentShellTaskRejectsBareAttackerGitMarkerWithoutTranscript(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "persistent-shell-marker",
		TaskID:       persistentShellTargetTaskID,
		Command:      `printf 'ATTACKER_GIT\n' > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed target run: %#v", result.CommandResult)
	}
	if result.ExpectationsMet || result.TargetOracle.Confirmed {
		t.Fatalf("expected bare ATTACKER_GIT marker to remain unconfirmed without transcript: %#v", result)
	}
	if result.TargetOracle.Status != targetOracleStatusInconclusive {
		t.Fatalf("expected inconclusive status for bare marker witness: %#v", result.TargetOracle)
	}
	if !containsString(result.TargetOracle.Missing, "transcript-backed proof that the shim marker came from a later shell call without another PATH export") {
		t.Fatalf("expected transcript-backed marker requirement: %#v", result.TargetOracle)
	}
}

func writeLangGraphShellHistory(t *testing.T, dir string, calls ...langgraphShellCall) {
	t.Helper()

	messages := make([]langgraphHistoryMessage, 0, len(calls)*2)
	for _, call := range calls {
		args, err := json.Marshal(map[string]string{"command": call.Command})
		if err != nil {
			t.Fatalf("marshal shell command: %v", err)
		}
		messages = append(messages,
			langgraphHistoryMessage{
				Role: "ai",
				ToolCalls: []langgraphHistoryToolCall{
					{Name: "shell", Args: string(args)},
				},
			},
			langgraphHistoryMessage{
				Role:    "tool",
				Content: call.Output,
			},
		)
	}

	raw, err := json.Marshal([]langgraphHistoryCheckpoint{{
		Index:    0,
		Messages: messages,
	}})
	if err != nil {
		t.Fatalf("marshal langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, langgraphHistoryArtifact), raw, 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
}

func writeLangGraphOperationSummary(t *testing.T, dir string, artifact string, operation string, requested bool, selector string, calls ...langgraphShellCall) {
	t.Helper()

	messages := make([]langgraphHistoryMessage, 0, len(calls)*2)
	for _, call := range calls {
		args, err := json.Marshal(map[string]string{"command": call.Command})
		if err != nil {
			t.Fatalf("marshal shell command: %v", err)
		}
		messages = append(messages,
			langgraphHistoryMessage{
				Role: "ai",
				ToolCalls: []langgraphHistoryToolCall{
					{Name: "shell", Args: string(args)},
				},
			},
			langgraphHistoryMessage{
				Role:    "tool",
				Content: call.Output,
			},
		)
	}

	raw, err := json.Marshal(langgraphOperationSummary{
		Operation:          operation,
		Requested:          requested,
		CheckpointSelector: selector,
		CheckpointIndex:    1,
		Messages:           messages,
	})
	if err != nil {
		t.Fatalf("marshal langgraph operation summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact), raw, 0o644); err != nil {
		t.Fatalf("write langgraph operation summary: %v", err)
	}
}

func TestRunTargetPersistentShellTaskRejectsUnexpectedResolution(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "persistent-shell-bad-resolution",
		TaskID:       persistentShellTargetTaskID,
		Command:      `printf '/usr/bin/git\n' > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed target run: %#v", result.CommandResult)
	}
	if result.ExpectationsMet || result.TargetOracle.Confirmed {
		t.Fatalf("expected persistent shell oracle to reject non-attacker path: %#v", result)
	}
	if result.TargetOracle.Status != targetOracleStatusNegative {
		t.Fatalf("expected negative status for clean system git witness: %#v", result.TargetOracle)
	}
	if !containsString(result.TargetOracle.Missing, "witness resolved or executed the workspace-local git shim") {
		t.Fatalf("expected missing workspace-local shim evidence: %#v", result.TargetOracle)
	}
}

func TestTargetTaskEnvOverridesConfigureReplayAndForkLifecycle(t *testing.T) {
	replayEnv := targetTaskEnvOverrides(persistentShellReplayTargetTaskID)
	if replayEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "true" {
		t.Fatalf("expected replay task to force replay: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected replay task to enable durable checkpoints: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-path-export" {
		t.Fatalf("expected replay task to select before-path-export checkpoint: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] != "" {
		t.Fatalf("expected replay task to clear fork follow-up: %#v", replayEnv)
	}

	forkEnv := targetTaskEnvOverrides(persistentShellForkTargetTaskID)
	if forkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected fork task to disable replay: %#v", forkEnv)
	}
	if forkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected fork task to enable durable checkpoints: %#v", forkEnv)
	}
	if forkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-path-export" {
		t.Fatalf("expected fork task to select before-path-export checkpoint: %#v", forkEnv)
	}
	if !strings.Contains(forkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], targetShellPoisonForkArtifact) {
		t.Fatalf("expected fork task to set a verification follow-up: %#v", forkEnv)
	}

	fileForkEnv := targetTaskEnvOverrides(fileResidueForkTargetTaskID)
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected file residue fork task to disable replay: %#v", fileForkEnv)
	}
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected file residue fork task to enable durable checkpoints: %#v", fileForkEnv)
	}
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-file-drop" {
		t.Fatalf("expected file residue fork task to select before-file-drop checkpoint: %#v", fileForkEnv)
	}
	if !strings.Contains(fileForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], targetFileResidueForkArtifact) {
		t.Fatalf("expected file residue fork task to set a file verification follow-up: %#v", fileForkEnv)
	}

	directoryForkEnv := targetTaskEnvOverrides(directoryResidueForkTargetTaskID)
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected directory residue fork task to disable replay: %#v", directoryForkEnv)
	}
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected directory residue fork task to enable durable checkpoints: %#v", directoryForkEnv)
	}
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-directory-create" {
		t.Fatalf("expected directory residue fork task to select before-directory-create checkpoint: %#v", directoryForkEnv)
	}
	if !strings.Contains(directoryForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], targetDirectoryResidueForkArtifact) {
		t.Fatalf("expected directory residue fork task to set a directory verification follow-up: %#v", directoryForkEnv)
	}

	deleteForkEnv := targetTaskEnvOverrides(deleteResidueForkTargetTaskID)
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected delete residue fork task to disable replay: %#v", deleteForkEnv)
	}
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected delete residue fork task to enable durable checkpoints: %#v", deleteForkEnv)
	}
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-file-delete" {
		t.Fatalf("expected delete residue fork task to select before-file-delete checkpoint: %#v", deleteForkEnv)
	}
	if !strings.Contains(deleteForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], targetDeleteResidueForkArtifact) {
		t.Fatalf("expected delete residue fork task to set a delete verification follow-up: %#v", deleteForkEnv)
	}

	symlinkForkEnv := targetTaskEnvOverrides(symlinkResidueForkTargetTaskID)
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected symlink residue fork task to disable replay: %#v", symlinkForkEnv)
	}
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected symlink residue fork task to enable durable checkpoints: %#v", symlinkForkEnv)
	}
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-symlink-create" {
		t.Fatalf("expected symlink residue fork task to select before-symlink-create checkpoint: %#v", symlinkForkEnv)
	}
	if !strings.Contains(symlinkForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], targetSymlinkResidueForkArtifact) {
		t.Fatalf("expected symlink residue fork task to set a symlink verification follow-up: %#v", symlinkForkEnv)
	}
}

func TestDefaultTargetPromptDeleteResidueForkAvoidsUnstableContentChecks(t *testing.T) {
	prompt := defaultTargetPrompt(deleteResidueForkTargetTaskID)
	if !strings.Contains(prompt, "printf '%s\\n' 'SYNCFUZZ_DELETE_RESIDUE_MARKER' > branch-delete-note.txt") {
		t.Fatalf("expected delete residue prompt to prescribe newline-terminated printf: %q", prompt)
	}
	if !strings.Contains(prompt, "ls -l branch-delete-note.txt && od -c branch-delete-note.txt") {
		t.Fatalf("expected delete residue prompt to prefer ls/od verification: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not use `echo -n`, `cat`, `head`, or `tail`") {
		t.Fatalf("expected delete residue prompt to forbid unstable content checks: %q", prompt)
	}
}

func TestWorkspaceResiduePromptsAllowLaterWitnessArtifacts(t *testing.T) {
	testcases := []struct {
		taskID   string
		artifact string
	}{
		{taskID: fileResidueForkTargetTaskID, artifact: targetFileResidueForkArtifact},
		{taskID: directoryResidueForkTargetTaskID, artifact: targetDirectoryResidueForkArtifact},
		{taskID: deleteResidueForkTargetTaskID, artifact: targetDeleteResidueForkArtifact},
		{taskID: symlinkResidueForkTargetTaskID, artifact: targetSymlinkResidueForkArtifact},
	}

	for _, tc := range testcases {
		prompt := defaultTargetPrompt(tc.taskID)
		if !strings.Contains(prompt, "A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.") {
			t.Fatalf("expected prompt for %s to explain later witness creation: %q", tc.taskID, prompt)
		}
		if !strings.Contains(prompt, tc.artifact) {
			t.Fatalf("expected prompt for %s to mention witness artifact %s: %q", tc.taskID, tc.artifact, prompt)
		}
	}
}

func TestLanggraphDeleteResidueForkVerificationMessageAvoidsDirectFileCat(t *testing.T) {
	message := langgraphDeleteResidueForkVerificationMessage()
	if !strings.Contains(message, "PRESENT_BRANCH_DELETE_NOTE") {
		t.Fatalf("expected delete residue fork follow-up to record explicit presence marker: %q", message)
	}
	if !strings.Contains(message, "od -c branch-delete-note.txt") {
		t.Fatalf("expected delete residue fork follow-up to use od for observation: %q", message)
	}
	if strings.Contains(message, "cat branch-delete-note.txt > delete-residue-fork-check.txt") {
		t.Fatalf("expected delete residue fork follow-up to avoid direct cat of branch-delete-note.txt: %q", message)
	}
}

func TestLanggraphDirectoryResidueForkVerificationMessageObservesDirectoryOnly(t *testing.T) {
	message := langgraphDirectoryResidueForkVerificationMessage()
	if !strings.Contains(message, "PRESENT_BRANCH_DIR") {
		t.Fatalf("expected directory residue fork follow-up to record explicit presence marker: %q", message)
	}
	if !strings.Contains(message, "ls -ld branch-dir") {
		t.Fatalf("expected directory residue fork follow-up to use ls -ld for observation: %q", message)
	}
	if strings.Contains(message, "mkdir ") {
		t.Fatalf("expected directory residue fork follow-up to avoid mkdir: %q", message)
	}
}

func TestForkVerificationMessagesClarifyLaterObservationStep(t *testing.T) {
	messages := []string{
		langgraphForkVerificationMessage(),
		langgraphFileResidueForkVerificationMessage(),
		langgraphDirectoryResidueForkVerificationMessage(),
		langgraphDeleteResidueForkVerificationMessage(),
		langgraphSymlinkResidueForkVerificationMessage(),
	}

	for _, message := range messages {
		if !strings.Contains(message, "later SyncFuzz fork-observation step") {
			t.Fatalf("expected fork verification message to clarify the follow-up phase: %q", message)
		}
		if !strings.Contains(message, "no longer applies") {
			t.Fatalf("expected fork verification message to resolve the initial-branch witness restriction: %q", message)
		}
	}
}

func TestEvaluatePersistentShellReplayTargetOracleConfirmsReplayResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
		t.Fatalf("write replay witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"mkdir -p attacker-bin && printf '#!/bin/sh\\nprintf ATTACKER_GIT\\n' > attacker-bin/git && chmod +x attacker-bin/git\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphReplayArtifact), []byte(`{
  "operation": "replay",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"Current PATH: $PATH\\\"\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
    },
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt 2>&1 && cat shell-poison-replay-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write replay summary: %v", err)
	}

	oracle := evaluatePersistentShellReplayTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected replay oracle to confirm duplicated PATH residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "replay witness showed the workspace-local tool directory duplicated inside PATH") {
		t.Fatalf("expected replay duplicate PATH evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleClassifiesLegitimateReexecution(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
		t.Fatalf("write replay witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphReplayArtifact), []byte(`{
  "operation": "replay",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"Current PATH: $PATH\\\"\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n"
    },
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt 2>&1 && cat shell-poison-replay-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write replay summary: %v", err)
	}

	oracle := evaluatePersistentShellReplayTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected replay oracle to reject pure re-execution: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionLegitimateReexecution {
		t.Fatalf("expected legitimate reexecution attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "replay residue occurred without depending only on re-executing the workspace-local PATH export") {
		t.Fatalf("expected legitimate reexecution missing reason: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleRejectsManualPathRestore(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
		t.Fatalf("write replay witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && printf '%s\\n' \\\"$PATH\\\" > /tmp/syncfuzz_path.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphReplayArtifact), []byte(`{
  "operation": "replay",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"PATH=$(cat /tmp/syncfuzz_path.txt) && export PATH && printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt 2>&1 && cat shell-poison-replay-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write replay summary: %v", err)
	}

	oracle := evaluatePersistentShellReplayTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected replay oracle to reject manual PATH restore: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionExternalSmuggling {
		t.Fatalf("expected smuggling attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "langgraph history wrote PATH state to an external helper path") {
		t.Fatalf("expected external helper write evidence: %#v", oracle)
	}
	if !containsString(oracle.Missing, "replay residue occurred without storing or restoring PATH through external helper files") {
		t.Fatalf("expected replay integrity failure: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleRecordsCleanReplayNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonReplayArtifact), []byte("Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"), 0o644); err != nil {
		t.Fatalf("write replay witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphReplayArtifact), []byte(`{
  "operation": "replay",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git --version >> shell-poison-replay-check.txt 2>&1 && cat shell-poison-replay-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write replay summary: %v", err)
	}

	oracle := evaluatePersistentShellReplayTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean replay to remain unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanReplay {
		t.Fatalf("expected clean replay attribution: %#v", oracle)
	}
	if oracle.Status != targetOracleStatusNegative {
		t.Fatalf("expected negative status for clean replay: %#v", oracle)
	}
	if !containsString(oracle.Missing, "replay preserved the workspace-local PATH override across the replay boundary") {
		t.Fatalf("expected clean replay regression reason: %#v", oracle)
	}
}

func TestEvaluatePersistentShellForkTargetOracleConfirmsForkResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonForkArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "user_message": "Without exporting PATH again, verify git and write shell-poison-fork-check.txt.",
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-fork-check.txt && command -v git >> shell-poison-fork-check.txt && git >> shell-poison-fork-check.txt 2>&1 && cat shell-poison-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluatePersistentShellForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected fork oracle to confirm inherited PATH residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution for fork: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "fork witness still showed the workspace-local tool directory inside PATH") {
		t.Fatalf("expected fork PATH evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetShellPoisonForkArtifact), []byte("Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"export PATH=\\\"$(pwd)/attacker-bin:$PATH\\\" && echo \\\"PATH=$PATH\\\"\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "PATH=/tmp/workspace/attacker-bin:/usr/bin:/bin\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-path-export",
  "checkpoint_index": 2,
  "user_message": "Without exporting PATH again, verify git and write shell-poison-fork-check.txt.",
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf \\\"Current PATH: %s\\\\n\\\" \\\"$PATH\\\" > shell-poison-fork-check.txt && command -v git >> shell-poison-fork-check.txt && git --version >> shell-poison-fork-check.txt 2>&1 && cat shell-poison-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluatePersistentShellForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean fork to remain unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if oracle.Status != targetOracleStatusNegative {
		t.Fatalf("expected negative status for clean fork: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved the workspace-local PATH override across the checkpoint boundary") {
		t.Fatalf("expected clean fork regression reason: %#v", oracle)
	}
}

func TestRunTargetCapturesLangGraphRuntimeArtifactsInStateTrace(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "langgraph-artifact-smoke",
		Command: strings.Join([]string{
			"touch late-effect",
			"printf '[]\\n' > " + langgraphHistoryArtifact,
			"printf '{}\\n' > " + langgraphSummaryArtifact,
			"printf '{}\\n' > " + langgraphLifecycleArtifact,
			"printf '{}\\n' > " + langgraphReplayArtifact,
			"printf '{}\\n' > " + langgraphForkArtifact,
		}, " && "),
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	stateTraceRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, stateTraceArtifact))
	if err != nil {
		t.Fatalf("read state trace: %v", err)
	}
	for _, artifact := range []string{
		langgraphHistoryArtifact,
		langgraphSummaryArtifact,
		langgraphLifecycleArtifact,
		langgraphReplayArtifact,
		langgraphForkArtifact,
	} {
		if !strings.Contains(string(stateTraceRaw), artifact) {
			t.Fatalf("expected state trace to include %s: %s", artifact, string(stateTraceRaw))
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifestRaw), langgraphLifecycleArtifact) {
		t.Fatalf("expected manifest to include lifecycle artifact: %s", string(manifestRaw))
	}
}

func TestRunTargetLongDelayTaskConfirmsBoundaryAndLateEffect(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:           filepath.Join(tmp, "runs"),
		TargetID:         "long-delay-smoke",
		TaskID:           longDelayTargetTaskID,
		Command:          "sh -c 'sleep 1; touch late-effect' >/dev/null 2>&1 &",
		ObserveDelay:     25 * time.Millisecond,
		LateObserveDelay: 1500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.ExpectationsMet || !result.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed long-delay target run: %#v", result)
	}
	if result.ProcessLineage.WorkspaceNewAtBoundary == 0 || result.ProcessLineage.WorkspaceRemainingAfter == 0 {
		t.Fatalf("expected workspace process lineage evidence: %#v", result.ProcessLineage)
	}
	if !result.LateObserved || !containsString(result.LateExpectedFilesPresent, longDelayTargetLateEffectArtifact) {
		t.Fatalf("expected late-effect in late observation: %#v", result)
	}
	for _, name := range []string{
		targetSnapshotLateArtifact,
		targetProcessLateArtifact,
		targetFilesystemLateArtifact,
	} {
		if _, err := os.Stat(filepath.Join(result.ArtifactDir, name)); err != nil {
			t.Fatalf("expected late artifact %s: %v", name, err)
		}
	}
}

func TestRunTargetLongDelayTaskRequiresBoundaryProcess(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "long-delay-noop",
		TaskID:       longDelayTargetTaskID,
		Command:      "printf launched",
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed target run: %#v", result.CommandResult)
	}
	if result.ExpectationsMet || result.TargetOracle.Confirmed {
		t.Fatalf("expected long-delay target oracle to reject missing boundary process: %#v", result)
	}
	if len(result.TargetOracle.Missing) == 0 {
		t.Fatalf("expected missing oracle evidence: %#v", result.TargetOracle)
	}
	if len(result.ExpectedFiles) != 0 || len(result.ExpectedFilesMissing) != 0 {
		t.Fatalf("unexpected immediate file expectations: %#v", result)
	}
	if result.Objective == "" || result.ProcessLineage.BeforeCount != 0 {
		t.Fatalf("expected target result to include objective and process summary: %#v", result)
	}
}

func TestRunTargetExportsUsableWorkspacePathsForRelativeOutDir(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir: "runs",
		Command: `test -f "$SYNCFUZZ_TASK_FILE" &&
test -f "$SYNCFUZZ_PROMPT_FILE" &&
printf ok > late-effect`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget with relative out dir failed: %v", err)
	}
	if !result.Completed || !result.ExpectationsMet {
		t.Fatalf("expected relative out dir target run to succeed: %#v", result)
	}
}

func TestEvaluateFileResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-residue")
	if err := os.WriteFile(filepath.Join(tmp, targetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-residue\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_FILE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"cat branch-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": "SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-residue\n"
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-drop",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\\\\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-residue\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateFileResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected file residue fork oracle to confirm workspace residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "fork witness preserved the expected branch-note marker") {
		t.Fatalf("expected witness marker evidence: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-rebuild")
	if err := os.WriteFile(filepath.Join(tmp, targetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-rebuild\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_FILE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-drop",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"printf 'SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-rebuild\\\\n' > branch-note.txt && cat branch-note.txt > file-residue-fork-check.txt && cat file-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-rebuild\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateFileResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected file residue fork oracle to reject fork-side reconstruction: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork residue occurred without recreating branch-note.txt during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-clean")
	if err := os.WriteFile(filepath.Join(tmp, targetFileResidueForkArtifact), []byte("MISSING_BRANCH_NOTE\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_FILE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-drop",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\\\\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "MISSING_BRANCH_NOTE\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateFileResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean fork to remain a negative result: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved branch-note.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleAcceptsGenericMarker(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-generic")
	if err := os.WriteFile(filepath.Join(tmp, targetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:$SYNCFUZZ_RUN_ID\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"echo 'SYNCFUZZ_FILE_RESIDUE_MARKER:$SYNCFUZZ_RUN_ID' > branch-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-drop",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\\\\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "SYNCFUZZ_FILE_RESIDUE_MARKER:$SYNCFUZZ_RUN_ID\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateFileResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected generic marker witness to confirm: %#v", oracle)
	}
}

func TestEvaluateDirectoryResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetDirectoryResidueForkArtifact), []byte("PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"mkdir -p branch-dir\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-directory-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -d branch-dir ]; then printf 'PRESENT_BRANCH_DIR\\\\n' > directory-residue-fork-check.txt; ls -ld branch-dir >> directory-residue-fork-check.txt; else printf 'MISSING_BRANCH_DIR\\\\n' > directory-residue-fork-check.txt; fi; cat directory-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDirectoryResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected directory residue fork oracle to confirm workspace residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "fork witness preserved branch-dir as a directory") {
		t.Fatalf("expected directory witness evidence: %#v", oracle)
	}
}

func TestEvaluateDirectoryResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetDirectoryResidueForkArtifact), []byte("PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"mkdir -p branch-dir\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-directory-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"mkdir -p branch-dir && printf 'PRESENT_BRANCH_DIR\\\\n' > directory-residue-fork-check.txt && ls -ld branch-dir >> directory-residue-fork-check.txt && cat directory-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDirectoryResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected directory residue fork oracle to reject fork-side reconstruction: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork residue occurred without recreating branch-dir during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateDirectoryResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetDirectoryResidueForkArtifact), []byte("MISSING_BRANCH_DIR\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"mkdir -p branch-dir\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-directory-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -d branch-dir ]; then printf 'PRESENT_BRANCH_DIR\\\\n' > directory-residue-fork-check.txt; ls -ld branch-dir >> directory-residue-fork-check.txt; else printf 'MISSING_BRANCH_DIR\\\\n' > directory-residue-fork-check.txt; fi; cat directory-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "MISSING_BRANCH_DIR\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDirectoryResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean directory fork to remain a negative result: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved branch-dir across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-residue")
	if err := os.WriteFile(filepath.Join(tmp, targetDeleteResidueForkArtifact), []byte("MISSING_BRANCH_DELETE_NOTE\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_DELETE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"rm -f branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-delete",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -f branch-delete-note.txt ]; then cat branch-delete-note.txt > delete-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETE_NOTE\\\\n' > delete-residue-fork-check.txt; fi; cat delete-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "MISSING_BRANCH_DELETE_NOTE\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDeleteResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected delete residue fork oracle to confirm workspace residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "fork witness reported that branch-delete-note.txt was absent") {
		t.Fatalf("expected delete witness evidence: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleRejectsForkMutation(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-rebuild")
	if err := os.WriteFile(filepath.Join(tmp, targetDeleteResidueForkArtifact), []byte("MISSING_BRANCH_DELETE_NOTE\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_DELETE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"rm -f branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-delete",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"rm -f branch-delete-note.txt && printf 'MISSING_BRANCH_DELETE_NOTE\\\\n' > delete-residue-fork-check.txt && cat delete-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "MISSING_BRANCH_DELETE_NOTE\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDeleteResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected delete residue fork oracle to reject fork-side mutation: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "delete residue occurred without modifying branch-delete-note.txt during the fork follow-up") {
		t.Fatalf("expected mutation-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-clean")
	if err := os.WriteFile(filepath.Join(tmp, targetDeleteResidueForkArtifact), []byte("PRESENT_BRANCH_DELETE_NOTE\n-rw-r--r-- 1 user user 41 branch-delete-note.txt\n0000000   S   Y   N   C   F   U   Z   Z   _   D   E   L   E   T   E   _\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"printf 'SYNCFUZZ_DELETE_RESIDUE_MARKER:%s\\n' \\\"$SYNCFUZZ_RUN_ID\\\" > branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      },
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"rm -f branch-delete-note.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-file-delete",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -f branch-delete-note.txt ]; then printf 'PRESENT_BRANCH_DELETE_NOTE\\\\n' > delete-residue-fork-check.txt; ls -l branch-delete-note.txt >> delete-residue-fork-check.txt; od -c branch-delete-note.txt >> delete-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETE_NOTE\\\\n' > delete-residue-fork-check.txt; fi; cat delete-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "PRESENT_BRANCH_DELETE_NOTE\n-rw-r--r-- 1 user user 41 branch-delete-note.txt\n0000000   S   Y   N   C   F   U   Z   Z   _   D   E   L   E   T   E   _\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateDeleteResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean delete fork to remain a negative result: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved branch-delete-note.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetSymlinkResidueForkArtifact), []byte("target-prompt.txt\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"ln -s target-prompt.txt branch-link.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-symlink-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -L branch-link.txt ]; then readlink branch-link.txt > symlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_LINK\\\\n' > symlink-residue-fork-check.txt; fi; cat symlink-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "target-prompt.txt\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateSymlinkResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected symlink residue fork oracle to confirm workspace residue: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !containsString(oracle.Evidence, "fork witness preserved branch-link.txt -> target-prompt.txt") {
		t.Fatalf("expected symlink witness evidence: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetSymlinkResidueForkArtifact), []byte("target-prompt.txt\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"ln -s target-prompt.txt branch-link.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-symlink-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"ln -sf target-prompt.txt branch-link.txt && readlink branch-link.txt > symlink-residue-fork-check.txt && cat symlink-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "target-prompt.txt\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateSymlinkResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected symlink residue fork oracle to reject fork-side reconstruction: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork residue occurred without recreating branch-link.txt during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetSymlinkResidueForkArtifact), []byte("MISSING_BRANCH_LINK\n"), 0o644); err != nil {
		t.Fatalf("write fork witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphHistoryArtifact), []byte(`[
  {
    "index": 0,
    "messages": [
      {
        "role": "ai",
        "tool_calls": [
          {
            "name": "shell",
            "args": "{\"command\":\"ln -s target-prompt.txt branch-link.txt\"}"
          }
        ]
      },
      {
        "role": "tool",
        "content": ""
      }
    ]
  }
]`), 0o644); err != nil {
		t.Fatalf("write langgraph history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, langgraphForkArtifact), []byte(`{
  "operation": "fork",
  "requested": true,
  "checkpoint_selector": "before-symlink-create",
  "checkpoint_index": 1,
  "messages": [
    {
      "role": "ai",
      "tool_calls": [
        {
          "name": "shell",
          "args": "{\"command\":\"if [ -L branch-link.txt ]; then readlink branch-link.txt > symlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_LINK\\\\n' > symlink-residue-fork-check.txt; fi; cat symlink-residue-fork-check.txt\"}"
        }
      ]
    },
    {
      "role": "tool",
      "content": "MISSING_BRANCH_LINK\n"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write fork summary: %v", err)
	}

	oracle := evaluateSymlinkResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean symlink fork to remain a negative result: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved branch-link.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestCommandWritesWorkspaceFileAcceptsAbsolutePath(t *testing.T) {
	if !commandWritesWorkspaceFile("printf marker > /tmp/demo/branch-note.txt", targetFileResidueNoteArtifact) {
		t.Fatalf("expected absolute-path branch-note write to be recognized")
	}
}

func TestCommandWritesWorkspaceFileIgnoresLaterReadOfDifferentTarget(t *testing.T) {
	command := "printf 'present\\n' > delete-residue-fork-check.txt; od -c branch-delete-note.txt >> delete-residue-fork-check.txt"
	if commandWritesWorkspaceFile(command, targetDeleteResidueNoteArtifact) {
		t.Fatalf("expected writes to witness file plus later reads of branch-delete-note.txt to avoid false positives")
	}
}

func TestCommandDeletesWorkspaceFileAcceptsAbsolutePath(t *testing.T) {
	if !commandDeletesWorkspaceFile("rm -f /tmp/demo/branch-delete-note.txt", targetDeleteResidueNoteArtifact) {
		t.Fatalf("expected absolute-path branch-delete-note delete to be recognized")
	}
}

func TestCommandCreatesWorkspaceDirectoryAcceptsAbsolutePath(t *testing.T) {
	if !commandCreatesWorkspaceDirectory("mkdir -p /tmp/demo/branch-dir", targetDirectoryResidueDirArtifact) {
		t.Fatalf("expected absolute-path branch-dir mkdir to be recognized")
	}
}

func TestCommandCreatesWorkspaceSymlinkAcceptsAbsolutePath(t *testing.T) {
	if !commandCreatesWorkspaceSymlink("ln -sf target-prompt.txt /tmp/demo/branch-link.txt", targetSymlinkResidueLinkArtifact) {
		t.Fatalf("expected absolute-path branch-link symlink creation to be recognized")
	}
}

func TestRunTargetRequiresCommand(t *testing.T) {
	_, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir: filepath.Join(t.TempDir(), "runs"),
	})
	if err == nil {
		t.Fatalf("expected missing command error")
	}
}

func TestRunTargetRejectsCommandAndCommandFile(t *testing.T) {
	tmp := t.TempDir()
	commandFile := filepath.Join(tmp, "target-command.sh")
	if err := os.WriteFile(commandFile, []byte("printf ok > late-effect\n"), 0o644); err != nil {
		t.Fatalf("write command file: %v", err)
	}

	_, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:      filepath.Join(tmp, "runs"),
		Command:     "printf ok > late-effect",
		CommandFile: commandFile,
	})
	if err == nil {
		t.Fatalf("expected command/file conflict error")
	}
}

func TestTargetAdaptersListsCommandAdapter(t *testing.T) {
	adapters := TargetAdapters()
	if len(adapters) == 0 {
		t.Fatalf("expected target adapters")
	}
	if adapters[0].AdapterID != defaultTargetAdapterID || !adapters[0].Implemented {
		t.Fatalf("expected implemented command adapter first: %#v", adapters[0])
	}
}

func writeTargetTaskRunID(t *testing.T, dir string, runID string) {
	t.Helper()
	raw := `{"schema_version":"syncfuzz.target-task.v1","run_id":"` + runID + `"}`
	if err := os.WriteFile(filepath.Join(dir, targetTaskArtifact), []byte(raw), 0o644); err != nil {
		t.Fatalf("write target task: %v", err)
	}
}
