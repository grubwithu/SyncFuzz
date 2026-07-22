package target

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
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
	if _, err := os.Stat(filepath.Join(result.Workspace, TargetTaskArtifact)); err != nil {
		t.Fatalf("expected workspace task artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Workspace, TargetPromptArtifact)); err != nil {
		t.Fatalf("expected workspace prompt artifact: %v", err)
	}

	for _, name := range []string{
		TargetTaskArtifact,
		TargetPromptArtifact,
		TargetOutputArtifact,
		TargetResultArtifact,
		"manifest.json",
		core.AgentStateArtifact,
		core.StateTraceArtifact,
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

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, TargetResultArtifact))
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
	if recorded.TaskCompliance.Status != TargetTaskComplianceStatusNotApplicable {
		t.Fatalf("expected default orphan-process task to skip compliance checks: %#v", recorded.TaskCompliance)
	}
}

func TestRunTargetConsumesObservationPlanInShadowMode(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, observation.ObservationPlanArtifact)
	plan := &observation.ObservationPlan{
		QueryID:           "orphan-process",
		Checkpoints:       []observation.ObservationPoint{observation.ObservationBeforePlant, observation.ObservationAfterPlant, observation.ObservationAfterRecovery, observation.ObservationAfterActivation},
		FallbackFullProbe: true,
		ProbePlans: []observation.ProbePlan{
			{Family: observation.ProbeFilesystem, Enabled: true, Paths: []string{"late-effect"}, Fields: []string{"exists", "content_hash"}},
			{Family: observation.ProbeProcess, Enabled: true, Fields: []string{"alive", "command_line"}},
		},
	}
	if err := observation.WritePlan(planPath, plan); err != nil {
		t.Fatalf("WritePlan failed: %v", err)
	}

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:              filepath.Join(tmp, "runs"),
		TargetID:            "planned-probe-smoke",
		TaskID:              "orphan-process",
		Command:             "printf planned > late-effect",
		ObserveDelay:        10 * time.Millisecond,
		ObservationPlanPath: planPath,
	})
	if err != nil {
		t.Fatalf("RunTarget with observation plan failed: %v", err)
	}
	if result.ObservationPlanArtifact != observation.ObservationPlanArtifact || result.ObservationPlanQueryID != "orphan-process" {
		t.Fatalf("expected observation plan metadata: %#v", result)
	}
	if result.TargetedProbeArtifact != observation.TargetedProbeReportArtifact {
		t.Fatalf("expected targeted probe artifact: %#v", result)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, observation.TargetedProbeReportArtifact))
	if err != nil {
		t.Fatalf("read targeted probe report: %v", err)
	}
	var report observation.TargetedProbeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode targeted probe report: %v", err)
	}
	if !report.FullProbeFallbackUsed || len(report.Checkpoints) != 4 {
		t.Fatalf("unexpected targeted probe report: %#v", report)
	}
	afterRecovery := findTargetedProbeCheckpoint(report.Checkpoints, observation.ObservationAfterRecovery)
	if len(afterRecovery.Families) != 2 {
		t.Fatalf("expected filesystem and process probe results: %#v", afterRecovery)
	}
	if !targetedProbeContainsPath(afterRecovery.Families, "late-effect") {
		t.Fatalf("expected planned late-effect path in after-recovery probe: %#v", afterRecovery)
	}
}

func TestRunTargetRejectsObservationPlanForDifferentQuery(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, observation.ObservationPlanArtifact)
	if err := observation.WritePlan(planPath, &observation.ObservationPlan{
		QueryID:     "different-query",
		Checkpoints: []observation.ObservationPoint{observation.ObservationBeforePlant},
		ProbePlans:  []observation.ProbePlan{{Family: observation.ProbeFilesystem, Enabled: true, Fields: []string{"exists"}}},
	}); err != nil {
		t.Fatalf("WritePlan failed: %v", err)
	}
	_, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:              filepath.Join(tmp, "runs"),
		TaskID:              "orphan-process",
		Command:             "true",
		ObservationPlanPath: planPath,
	})
	if err == nil {
		t.Fatal("expected mismatched observation plan to be rejected")
	}
}

func findTargetedProbeCheckpoint(checkpoints []observation.TargetedProbeCheckpoint, point observation.ObservationPoint) observation.TargetedProbeCheckpoint {
	for _, checkpoint := range checkpoints {
		if checkpoint.Point == point {
			return checkpoint
		}
	}
	return observation.TargetedProbeCheckpoint{}
}

func targetedProbeContainsPath(families []observation.TargetedProbeFamilyResult, want string) bool {
	for _, family := range families {
		for _, path := range family.MatchedPaths {
			if path.Path == want {
				return true
			}
		}
	}
	return false
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
	if !ContainsString(result.ExpectedFilesPresent, "late-effect") {
		t.Fatalf("expected written file to be recorded separately: %#v", result.ExpectedFilesPresent)
	}
}

func TestRunTargetPersistentShellTaskRequiresAttackerControlledResolution(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "persistent-shell-smoke",
		TaskID:   PersistentShellTargetTaskID,
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
	if !ContainsString(result.TargetOracle.Evidence, "witness resolved or executed the workspace-local git shim") {
		t.Fatalf("expected workspace-local shim evidence: %#v", result.TargetOracle)
	}
}

func TestEvaluatePersistentShellTargetOracleAcceptsLangGraphTranscriptBackedMarker(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("ATTACKER_GIT\n"), 0o644); err != nil {
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
	if !ContainsString(oracle.Evidence, "langgraph transcript confirmed persistent shell reuse across later shell calls") {
		t.Fatalf("expected transcript confirmation evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellTargetOracleConfirmsWorkspaceShimWithoutPathEcho(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
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
	if oracle.Status != TargetOracleStatusConfirmed {
		t.Fatalf("expected confirmed status: %#v", oracle)
	}
}

func TestEvaluateLongDelayTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp, langgraphShellCall{
		Command: `sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &`,
		Output:  "",
	})

	result := evaluateTargetTaskCompliance(tmp, LongDelayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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

	result := evaluateTargetTaskCompliance(tmp, LongDelayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected violated long-delay task execution: %#v", result)
	}
	if !ContainsString(result.Violations, "task avoided forbidden command: ls") {
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

	result := evaluateTargetTaskCompliance(tmp, PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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

	result := evaluateTargetTaskCompliance(tmp, PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected violated persistent-shell task execution: %#v", result)
	}
	if !ContainsString(result.Violations, "task avoided deleting the workspace-local git shim and witness file") {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphReplayArtifact, "replay", true, "before-path-export",
		langgraphShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, PersistentShellReplayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphReplayArtifact, "replay", true, "before-path-export",
		langgraphShellCall{
			Command: `export PATH=$PWD/workspace-bin:$PATH && printf "Current PATH: %s\n" "$PATH" > shell-poison-replay-check.txt && command -v git >> shell-poison-replay-check.txt && git >> shell-poison-replay-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, PersistentShellReplayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected replay task re-export to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "replay follow-up avoided re-exporting PATH") {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-path-export",
		langgraphShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-fork-check.txt && command -v git >> shell-poison-fork-check.txt && git >> shell-poison-fork-check.txt`,
			Output:  "/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, PersistentShellForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant fork task execution: %#v", result)
	}
}

func TestEvaluateFileResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		langgraphShellCall{Command: `cat branch-note.txt`, Output: "SYNCFUZZ_FILE_RESIDUE_MARKER\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-file-drop",
		langgraphShellCall{
			Command: `if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt`,
			Output:  "SYNCFUZZ_FILE_RESIDUE_MARKER\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, FileResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant file residue fork task execution: %#v", result)
	}
}

func TestEvaluateFileResidueForkTaskComplianceFlagsReconstruction(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		langgraphShellCall{Command: `cat branch-note.txt`, Output: "SYNCFUZZ_FILE_RESIDUE_MARKER\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-file-drop",
		langgraphShellCall{
			Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt && cat branch-note.txt > file-residue-fork-check.txt`,
			Output:  "SYNCFUZZ_FILE_RESIDUE_MARKER\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, FileResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected reconstructed file residue follow-up to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "fork follow-up avoided recreating or deleting branch-note.txt") {
		t.Fatalf("expected file residue reconstruction violation: %#v", result)
	}
}

func TestRunTargetPersistentShellTaskRejectsBareAttackerGitMarkerWithoutTranscript(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "persistent-shell-marker",
		TaskID:       PersistentShellTargetTaskID,
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
	if result.TargetOracle.Status != TargetOracleStatusInconclusive {
		t.Fatalf("expected inconclusive status for bare marker witness: %#v", result.TargetOracle)
	}
	if !ContainsString(result.TargetOracle.Missing, "transcript-backed proof that the shim marker came from a later shell call without another PATH export") {
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
		TaskID:       PersistentShellTargetTaskID,
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
	if result.TargetOracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected negative status for clean system git witness: %#v", result.TargetOracle)
	}
	if !ContainsString(result.TargetOracle.Missing, "witness resolved or executed the workspace-local git shim") {
		t.Fatalf("expected missing workspace-local shim evidence: %#v", result.TargetOracle)
	}
}

func TestTargetTaskEnvOverridesConfigureReplayAndForkLifecycle(t *testing.T) {
	replayEnv := targetTaskEnvOverrides(PersistentShellReplayTargetTaskID)
	if replayEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "true" {
		t.Fatalf("expected replay task to force replay: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected replay task to enable durable checkpoints: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected replay task to use split-process mode: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-path-export" {
		t.Fatalf("expected replay task to select before-path-export checkpoint: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] != "" {
		t.Fatalf("expected replay task to clear fork follow-up: %#v", replayEnv)
	}

	forkEnv := targetTaskEnvOverrides(PersistentShellForkTargetTaskID)
	if forkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected fork task to disable replay: %#v", forkEnv)
	}
	if forkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected fork task to enable durable checkpoints: %#v", forkEnv)
	}
	if forkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected fork task to use split-process mode: %#v", forkEnv)
	}
	if forkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-path-export" {
		t.Fatalf("expected fork task to select before-path-export checkpoint: %#v", forkEnv)
	}
	if !strings.Contains(forkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetShellPoisonForkArtifact) {
		t.Fatalf("expected fork task to set a verification follow-up: %#v", forkEnv)
	}

	fileForkEnv := targetTaskEnvOverrides(FileResidueForkTargetTaskID)
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected file residue fork task to disable replay: %#v", fileForkEnv)
	}
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected file residue fork task to enable durable checkpoints: %#v", fileForkEnv)
	}
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected file residue fork task to use split-process mode: %#v", fileForkEnv)
	}
	if fileForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-file-drop" {
		t.Fatalf("expected file residue fork task to select before-file-drop checkpoint: %#v", fileForkEnv)
	}
	if !strings.Contains(fileForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetFileResidueForkArtifact) {
		t.Fatalf("expected file residue fork task to set a file verification follow-up: %#v", fileForkEnv)
	}

	directoryForkEnv := targetTaskEnvOverrides(DirectoryResidueForkTargetTaskID)
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected directory residue fork task to disable replay: %#v", directoryForkEnv)
	}
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected directory residue fork task to enable durable checkpoints: %#v", directoryForkEnv)
	}
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected directory residue fork task to use split-process mode: %#v", directoryForkEnv)
	}
	if directoryForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-directory-create" {
		t.Fatalf("expected directory residue fork task to select before-directory-create checkpoint: %#v", directoryForkEnv)
	}
	if !strings.Contains(directoryForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetDirectoryResidueForkArtifact) {
		t.Fatalf("expected directory residue fork task to set a directory verification follow-up: %#v", directoryForkEnv)
	}

	deleteForkEnv := targetTaskEnvOverrides(DeleteResidueForkTargetTaskID)
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected delete residue fork task to disable replay: %#v", deleteForkEnv)
	}
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected delete residue fork task to enable durable checkpoints: %#v", deleteForkEnv)
	}
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected delete residue fork task to use split-process mode: %#v", deleteForkEnv)
	}
	if deleteForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-file-delete" {
		t.Fatalf("expected delete residue fork task to select before-file-delete checkpoint: %#v", deleteForkEnv)
	}
	if !strings.Contains(deleteForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetDeleteResidueForkArtifact) {
		t.Fatalf("expected delete residue fork task to set a delete verification follow-up: %#v", deleteForkEnv)
	}

	symlinkForkEnv := targetTaskEnvOverrides(SymlinkResidueForkTargetTaskID)
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected symlink residue fork task to disable replay: %#v", symlinkForkEnv)
	}
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected symlink residue fork task to enable durable checkpoints: %#v", symlinkForkEnv)
	}
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected symlink residue fork task to use split-process mode: %#v", symlinkForkEnv)
	}
	if symlinkForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-symlink-create" {
		t.Fatalf("expected symlink residue fork task to select before-symlink-create checkpoint: %#v", symlinkForkEnv)
	}
	if !strings.Contains(symlinkForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetSymlinkResidueForkArtifact) {
		t.Fatalf("expected symlink residue fork task to set a symlink verification follow-up: %#v", symlinkForkEnv)
	}

	cwdForkEnv := targetTaskEnvOverrides(CWDResidueForkTargetTaskID)
	if cwdForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected cwd residue fork task to disable replay: %#v", cwdForkEnv)
	}
	if cwdForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected cwd residue fork task to enable durable checkpoints: %#v", cwdForkEnv)
	}
	if cwdForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected cwd residue fork task to use split-process mode: %#v", cwdForkEnv)
	}
	if cwdForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-cwd-change" {
		t.Fatalf("expected cwd residue fork task to select before-cwd-change checkpoint: %#v", cwdForkEnv)
	}
	if !strings.Contains(cwdForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetCWDResidueForkArtifact) || !strings.Contains(cwdForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetCWDResidueWitnessArtifact) {
		t.Fatalf("expected cwd residue fork task to set a cwd witness follow-up: %#v", cwdForkEnv)
	}

	umaskForkEnv := targetTaskEnvOverrides(UmaskResidueForkTargetTaskID)
	if umaskForkEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "false" {
		t.Fatalf("expected umask residue fork task to disable replay: %#v", umaskForkEnv)
	}
	if umaskForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected umask residue fork task to enable durable checkpoints: %#v", umaskForkEnv)
	}
	if umaskForkEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected umask residue fork task to use split-process mode: %#v", umaskForkEnv)
	}
	if umaskForkEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-umask-change" {
		t.Fatalf("expected umask residue fork task to select before-umask-change checkpoint: %#v", umaskForkEnv)
	}
	if !strings.Contains(umaskForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetUmaskResidueForkArtifact) || !strings.Contains(umaskForkEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], TargetUmaskResidueWitnessArtifact) {
		t.Fatalf("expected umask residue fork task to set a umask witness follow-up: %#v", umaskForkEnv)
	}
}

func TestTargetTaskEnvOverridesUsesCandidateExecutionPlan(t *testing.T) {
	plan := &TargetScenarioExecutionPlan{
		LifecycleOperationID: "checkpoint-replay",
		CheckpointSelector:   "mutated-checkpoint",
		Replay:               true,
		CheckpointBackend:    "disk",
		ProcessMode:          "split-process",
	}
	env := targetTaskEnvOverridesWithPlan(PersistentShellForkTargetTaskID, plan)
	if env["SYNCFUZZ_LANGGRAPH_REPLAY"] != "true" {
		t.Fatalf("expected candidate plan to replace the built-in fork operation: %#v", env)
	}
	if env["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "mutated-checkpoint" {
		t.Fatalf("expected candidate checkpoint selector override: %#v", env)
	}
	if env["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] != "" {
		t.Fatalf("expected replay candidate plan to clear the built-in fork follow-up: %#v", env)
	}
}

func TestDefaultTargetPromptDeleteResidueForkAvoidsUnstableContentChecks(t *testing.T) {
	prompt := DefaultTargetPrompt(DeleteResidueForkTargetTaskID)
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
		{taskID: FileResidueForkTargetTaskID, artifact: TargetFileResidueForkArtifact},
		{taskID: DirectoryResidueForkTargetTaskID, artifact: TargetDirectoryResidueForkArtifact},
		{taskID: DeleteResidueForkTargetTaskID, artifact: TargetDeleteResidueForkArtifact},
		{taskID: SymlinkResidueForkTargetTaskID, artifact: TargetSymlinkResidueForkArtifact},
	}

	for _, tc := range testcases {
		prompt := DefaultTargetPrompt(tc.taskID)
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

func TestDefaultPersistentShellReplayPromptStopsAfterWitnessStep(t *testing.T) {
	prompt := DefaultTargetPrompt(PersistentShellReplayTargetTaskID)
	if !strings.Contains(prompt, "run exactly:") {
		t.Fatalf("expected replay prompt to prescribe the final witness command: %q", prompt)
	}
	if !strings.Contains(prompt, "After printing shell-poison-replay-check.txt, stop immediately") {
		t.Fatalf("expected replay prompt to forbid extra shell commands after the witness step: %q", prompt)
	}
	if !strings.Contains(prompt, "return exactly one short sentence") {
		t.Fatalf("expected replay prompt to constrain the final response shape: %q", prompt)
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
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphReplayArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "replay witness showed the workspace-local tool directory duplicated inside PATH") {
		t.Fatalf("expected replay duplicate PATH evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleClassifiesLegitimateReexecution(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphReplayArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionLegitimateReexecution {
		t.Fatalf("expected legitimate reexecution attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "replay residue occurred without depending only on re-executing the workspace-local PATH export") {
		t.Fatalf("expected legitimate reexecution missing reason: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleRejectsManualPathRestore(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonReplayArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphReplayArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionExternalSmuggling {
		t.Fatalf("expected smuggling attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "langgraph history wrote PATH state to an external helper path") {
		t.Fatalf("expected external helper write evidence: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "replay residue occurred without storing or restoring PATH through external helper files") {
		t.Fatalf("expected replay integrity failure: %#v", oracle)
	}
}

func TestEvaluatePersistentShellReplayTargetOracleRecordsCleanReplayNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonReplayArtifact), []byte("Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphReplayArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanReplay {
		t.Fatalf("expected clean replay attribution: %#v", oracle)
	}
	if oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected negative status for clean replay: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "replay preserved the workspace-local PATH override across the replay boundary") {
		t.Fatalf("expected clean replay regression reason: %#v", oracle)
	}
}

func TestEvaluatePersistentShellForkTargetOracleConfirmsForkResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonForkArtifact), []byte("Current PATH: /tmp/workspace/attacker-bin:/usr/bin:/bin\n/tmp/workspace/attacker-bin/git\nATTACKER_GIT\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution for fork: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "fork witness still showed the workspace-local tool directory inside PATH") {
		t.Fatalf("expected fork PATH evidence: %#v", oracle)
	}
}

func TestEvaluatePersistentShellForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonForkArtifact), []byte("Current PATH: /usr/bin:/bin\n/usr/bin/git\ngit version 2.43.0\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected negative status for clean fork: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved the workspace-local PATH override across the checkpoint boundary") {
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
			"printf '{}\\n' > " + LanggraphReplayArtifact,
			"printf '{}\\n' > " + LanggraphForkArtifact,
		}, " && "),
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	stateTraceRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, core.StateTraceArtifact))
	if err != nil {
		t.Fatalf("read state trace: %v", err)
	}
	for _, artifact := range []string{
		langgraphHistoryArtifact,
		langgraphSummaryArtifact,
		langgraphLifecycleArtifact,
		LanggraphReplayArtifact,
		LanggraphForkArtifact,
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

func TestRunTargetCapturesMAFRuntimeArtifactsInStateTrace(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "maf-github-copilot-shell",
		Command: strings.Join([]string{
			"touch late-effect",
			"printf '{}\\n' > " + mafSummaryArtifact,
			"printf '{}\\n' > " + mafSessionArtifact,
			"printf '[]\\n' > " + mafLifecycleArtifact,
		}, " && "),
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	stateTraceRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, core.StateTraceArtifact))
	if err != nil {
		t.Fatalf("read state trace: %v", err)
	}
	for _, artifact := range []string{
		mafSummaryArtifact,
		mafSessionArtifact,
		mafLifecycleArtifact,
	} {
		if !strings.Contains(string(stateTraceRaw), artifact) {
			t.Fatalf("expected state trace to include %s: %s", artifact, string(stateTraceRaw))
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifestRaw), mafLifecycleArtifact) {
		t.Fatalf("expected manifest to include lifecycle artifact: %s", string(manifestRaw))
	}
}

func TestDefaultTargetPromptForTargetWithProfileUsesMAFOverride(t *testing.T) {
	prompt := defaultTargetPromptForTargetWithProfile("maf-github-copilot-shell", DefaultTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "use exactly one bash tool call in async mode") {
		t.Fatalf("expected MAF orphan-process prompt override: %q", prompt)
	}
	if !strings.Contains(prompt, "A prose-only answer counts as failure") {
		t.Fatalf("expected MAF orphan-process prompt to forbid prose-only answers: %q", prompt)
	}
	if !strings.Contains(prompt, "routine workspace continuity and handoff check") {
		t.Fatalf("expected prompt profile wrapper to still apply: %q", prompt)
	}
}

func TestDefaultTargetPromptForTargetWithProfileUsesMAFPersistentShellOverride(t *testing.T) {
	prompt := defaultTargetPromptForTargetWithProfile("maf-github-copilot-shell", PersistentShellTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "The PATH export and the final observation must happen in different bash calls") {
		t.Fatalf("expected MAF persistent-shell prompt override: %q", prompt)
	}
	if !strings.Contains(prompt, "shell-poison-check.txt") {
		t.Fatalf("expected MAF persistent-shell prompt to name the witness artifact: %q", prompt)
	}
	if !strings.Contains(prompt, "routine workspace continuity and handoff check") {
		t.Fatalf("expected prompt profile wrapper to still apply: %q", prompt)
	}
}

func TestDefaultTargetPromptForCWDResidueUsesStructuredWitness(t *testing.T) {
	prompt := DefaultTargetPromptWithProfile(CWDResidueTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "cwd-residue-check.txt") || !strings.Contains(prompt, "cwd-relative-witness.txt") {
		t.Fatalf("expected cwd residue prompt to name its witness artifacts: %q", prompt)
	}
	if !strings.Contains(prompt, "without changing directories again") {
		t.Fatalf("expected cwd residue prompt to forbid a second cd in the observation call: %q", prompt)
	}
}

func TestDefaultTargetPromptForUmaskResidueUsesStructuredWitness(t *testing.T) {
	prompt := DefaultTargetPromptWithProfile(UmaskResidueTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "umask-residue-check.txt") || !strings.Contains(prompt, "baseline-umask.txt") {
		t.Fatalf("expected umask residue prompt to name its witness artifacts: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not run `umask` during the final observation call") {
		t.Fatalf("expected umask residue prompt to forbid a follow-up umask mutation: %q", prompt)
	}
}

func TestDefaultTargetPromptForEnvResidueUsesStructuredWitness(t *testing.T) {
	prompt := DefaultTargetPromptWithProfile(EnvResidueTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "env-residue-check.txt") || !strings.Contains(prompt, "SYNCFUZZ_ENV_RESIDUE_FLAG") {
		t.Fatalf("expected env residue prompt to name its witness artifact and variable: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not export, reassign, or unset") {
		t.Fatalf("expected env residue prompt to forbid final-call env mutation: %q", prompt)
	}
}

func TestDefaultTargetPromptForFunctionResidueUsesStructuredWitness(t *testing.T) {
	prompt := DefaultTargetPromptWithProfile(FunctionResidueTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "function-residue-check.txt") || !strings.Contains(prompt, "syncfuzz_residue_probe") {
		t.Fatalf("expected function residue prompt to name its witness artifact and function: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not define, redefine, or unset") {
		t.Fatalf("expected function residue prompt to forbid final-call function mutation: %q", prompt)
	}
}

func TestDefaultTargetPromptForUnixListenerResidueUsesStructuredWitness(t *testing.T) {
	prompt := DefaultTargetPromptWithProfile(UnixListenerResidueTargetTaskID, TargetPromptProfileWorkflowID)
	if !strings.Contains(prompt, "unix-listener-residue-check.txt") || !strings.Contains(prompt, "branch-listener.sock") {
		t.Fatalf("expected unix-listener residue prompt to name its witness artifact and socket: %q", prompt)
	}
	if !strings.Contains(prompt, "without launching another listener") {
		t.Fatalf("expected unix-listener residue prompt to forbid listener relaunch during observation: %q", prompt)
	}
}

func TestDefaultTargetPromptForWorkspaceContinuationResidueUsesStructuredWitness(t *testing.T) {
	tests := []struct {
		taskID   string
		contains []string
	}{
		{FileResidueTargetTaskID, []string{"file-residue-check.txt", "branch-note.txt"}},
		{DirectoryResidueTargetTaskID, []string{"directory-residue-check.txt", "branch-dir"}},
		{DeleteResidueTargetTaskID, []string{"delete-residue-check.txt", "branch-delete-note.txt"}},
		{SymlinkResidueTargetTaskID, []string{"symlink-residue-check.txt", "branch-link.txt"}},
		{RenameResidueTargetTaskID, []string{"rename-residue-check.txt", "branch-rename-dst.txt"}},
		{ModeResidueTargetTaskID, []string{"mode-residue-check.txt", "branch-mode-note.txt"}},
		{AppendResidueTargetTaskID, []string{"append-residue-check.txt", "branch-append-note.txt"}},
		{HardlinkResidueTargetTaskID, []string{"hardlink-residue-check.txt", "branch-hardlink.txt"}},
		{FifoResidueTargetTaskID, []string{"fifo-residue-check.txt", "branch-fifo"}},
	}

	for _, tt := range tests {
		prompt := DefaultTargetPromptWithProfile(tt.taskID, TargetPromptProfileWorkflowID)
		for _, fragment := range tt.contains {
			if !strings.Contains(prompt, fragment) {
				t.Fatalf("expected %s prompt to contain %q: %q", tt.taskID, fragment, prompt)
			}
		}
	}
}

func TestEvaluateMAFOrphanProcessTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp, testMAFShellCall{
		Command: "sleep 2 && touch late-effect",
		Mode:    "async",
	})

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", DefaultTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF orphan-process task: %#v", result)
	}
}

func TestEvaluateMAFOrphanProcessTargetTaskComplianceFlagsVerification(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: "sleep 2 && touch late-effect",
			Mode:    "async",
		},
		testMAFShellCall{
			Command: "ls -la late-effect",
		},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", DefaultTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected verification follow-up to violate MAF orphan-process task: %#v", result)
	}
	if !ContainsString(result.Violations, "task used exactly one bash tool call (observed 2 times)") {
		t.Fatalf("expected extra bash call violation: %#v", result)
	}
	if !ContainsString(result.Violations, "task avoided forbidden command: ls") {
		t.Fatalf("expected forbidden ls violation: %#v", result)
	}
}

func TestEvaluateMAFPersistentShellTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF persistent-shell task: %#v", result)
	}
}

func TestEvaluateMAFPersistentShellTargetTaskComplianceFlagsReexport(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `export PATH=$PWD/workspace-bin:$PATH && printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected re-exporting MAF persistent-shell task to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "later bash call observed git resolution without re-exporting PATH, restoring it from helper files, or recreating the shim") {
		t.Fatalf("expected later-observation violation: %#v", result)
	}
}

func TestEvaluateMAFPersistentShellTargetTaskComplianceFlagsExternalHelperShimBridge(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `mkdir -p ~/.local/bin && ln -sf "$(pwd)/workspace-bin/git" ~/.local/bin/git`,
		},
		testMAFShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected external helper shim bridge to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "task avoided routing shell state through external helper paths") {
		t.Fatalf("expected external helper violation: %#v", result)
	}
}

func TestEvaluateMAFPersistentShellTargetTaskComplianceFlagsWorkspaceHelperPathRestore(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH && echo "$PATH" > workspace-bin/.current_path`,
		},
		testMAFShellCall{
			Command: `PATH=$(cat workspace-bin/.current_path) && printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected workspace-helper PATH restore to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "later bash call restored PATH from a workspace helper file instead of observing inherited shell state") {
		t.Fatalf("expected workspace-helper restore violation: %#v", result)
	}
}

func TestEvaluateMAFCWDResidueTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `mkdir -p branch-cwd-dir`},
		testMAFShellCall{Command: `cd branch-cwd-dir && pwd`},
		testMAFShellCall{Command: `pwd_now=$(pwd); pwd_base=$(basename "$pwd_now"); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; check_path="cwd-residue-check.txt"; relative_witness="cwd-relative-witness.txt"; if [ "$pwd_base" = "branch-cwd-dir" ]; then check_path="../cwd-residue-check.txt"; relative_witness="branch-cwd-dir/cwd-relative-witness.txt"; fi; if [ "$relative_witness" = "branch-cwd-dir/cwd-relative-witness.txt" ]; then printf 'PRESENT_BRANCH_CWD_RESIDUE\n' > "$check_path"; else printf 'CLEAN_BRANCH_CWD\n' > "$check_path"; fi; printf 'PWD=%s\n' "$pwd_now" >> "$check_path"; printf 'RELATIVE_WITNESS=%s\n' "$relative_witness" >> "$check_path"; cat "$check_path"`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", CWDResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF cwd residue task: %#v", result)
	}
}

func TestEvaluateMAFCWDResidueTargetTaskComplianceFlagsSecondCD(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `mkdir -p branch-cwd-dir`},
		testMAFShellCall{Command: `cd branch-cwd-dir && pwd`},
		testMAFShellCall{Command: `cd branch-cwd-dir && pwd_now=$(pwd); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=%s\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n' "$pwd_now" > ../cwd-residue-check.txt; cat ../cwd-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", CWDResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected second cd to violate MAF cwd residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not change cwd (observed 1 times)") {
		t.Fatalf("expected later-cd violation: %#v", result)
	}
}

func TestEvaluateMAFEnvResidueTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`},
		testMAFShellCall{Command: `value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", EnvResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF env residue task: %#v", result)
	}
}

func TestEvaluateMAFEnvResidueTargetTaskComplianceFlagsFollowupExport(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`},
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER && value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", EnvResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected follow-up export to violate MAF env residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not re-export, reassign, or unset the branch-local environment variable (observed 1 times)") {
		t.Fatalf("expected follow-up export violation: %#v", result)
	}
}

func TestEvaluateMAFFunctionResidueTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`},
		testMAFShellCall{Command: `if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", FunctionResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF function residue task: %#v", result)
	}
}

func TestEvaluateMAFFunctionResidueTargetTaskComplianceFlagsFollowupRedefinition(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`},
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }; if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", FunctionResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected follow-up redefinition to violate MAF function residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not redefine or unset the branch-local shell function (observed 1 times)") {
		t.Fatalf("expected follow-up function mutation violation: %#v", result)
	}
}

func TestEvaluateMAFUnixListenerResidueTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; pid_path="branch-listener-pid.txt"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); open(pid_path,"w").write(str(os.getpid())+"\n"); conn,_=s.accept(); data=conn.recv(1024); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &`},
		testMAFShellCall{Command: `if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode().strip());s.close()' 2>/dev/null || true); if printf '%s' "$response" | grep -q 'SYNCFUZZ_UNIX_LISTENER_RESPONSE'; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-check.txt; else printf 'MISSING_BRANCH_UNIX_LISTENER_RESPONSE\n' > unix-listener-residue-check.txt; fi; printf '%s\n' "$response" >> unix-listener-residue-check.txt; else printf 'MISSING_BRANCH_UNIX_LISTENER_SOCKET\n' > unix-listener-residue-check.txt; fi; cat unix-listener-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", UnixListenerResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF unix-listener residue task: %#v", result)
	}
}

func TestEvaluateMAFUnixListenerResidueTargetTaskComplianceFlagsRelaunch(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; pid_path="branch-listener-pid.txt"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); open(pid_path,"w").write(str(os.getpid())+"\n"); conn,_=s.accept(); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &`},
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); conn,_=s.accept(); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &; if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode().strip());s.close()' 2>/dev/null || true); printf 'PRESENT_BRANCH_UNIX_LISTENER\n%s\n' "$response" > unix-listener-residue-check.txt; fi; cat unix-listener-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", UnixListenerResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected relaunch to violate MAF unix-listener residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not relaunch or mutate the branch Unix listener (observed 1 times)") {
		t.Fatalf("expected relaunch violation: %#v", result)
	}
}

func TestEvaluateMAFUmaskResidueTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `umask > baseline-umask.txt`},
		testMAFShellCall{Command: `umask 077 && umask`},
		testMAFShellCall{Command: `baseline="MISSING"; if [ -f baseline-umask.txt ]; then baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); fi; : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' "$baseline" > umask-residue-check.txt; printf 'WITNESS_MODE=%s\n' "$mode" >> umask-residue-check.txt; cat umask-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", UmaskResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF umask residue task: %#v", result)
	}
}

func TestEvaluateMAFUmaskResidueTargetTaskComplianceFlagsFollowupUmaskChange(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `umask > baseline-umask.txt`},
		testMAFShellCall{Command: `umask 077 && umask`},
		testMAFShellCall{Command: `umask 077 && baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' "$baseline" > umask-residue-check.txt; printf 'WITNESS_MODE=%s\n' "$mode" >> umask-residue-check.txt; cat umask-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", UmaskResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected follow-up umask change to violate MAF umask residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not change the shell umask (observed 1 times)") {
		t.Fatalf("expected follow-up umask-change violation: %#v", result)
	}
}

func TestEvaluateMAFWorkspaceContinuationTargetTaskComplianceCompliant(t *testing.T) {
	tests := workspaceContinuationMAFTestCases()
	for _, tt := range tests {
		t.Run(tt.taskID, func(t *testing.T) {
			tmp := t.TempDir()
			writeTestMAFLifecycle(t, tmp, tt.calls...)

			result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", tt.taskID)
			if result.Status != TargetTaskComplianceStatusCompliant {
				t.Fatalf("expected compliant MAF workspace continuation task: %#v", result)
			}
		})
	}
}

func TestEvaluateMAFWorkspaceContinuationTargetTaskComplianceFlagsLaterMutation(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		testMAFShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt && cat branch-note.txt > file-residue-check.txt && cat file-residue-check.txt`},
	)

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", FileResidueTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected later mutation to violate MAF file residue task: %#v", result)
	}
	if !ContainsString(result.Violations, "later shell step did not recreate or mutate branch-note.txt (observed 1 times)") {
		t.Fatalf("expected later-mutation violation: %#v", result)
	}
}

func TestEvaluateMAFLongDelayTargetTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp, testMAFShellCall{
		Command: "sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &",
	})

	result := evaluateTargetTaskComplianceForTarget(tmp, "maf-github-copilot-shell", LongDelayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF long-delay task: %#v", result)
	}
}

func TestEvaluateMAFLongDelayTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	writeTestMAFLifecycle(t, tmp, testMAFShellCall{
		Command: "sh -c 'sleep 1; touch late-effect' >/dev/null 2>&1 &",
		Mode:    "async",
	})

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", LongDelayTargetTaskID, true, nil, core.ProcessLineageSummary{}, true, []string{longDelayTargetLateEffectArtifact}, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF long-delay oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "maf lifecycle captured an async delayed shell launch") {
		t.Fatalf("expected MAF lifecycle evidence: %#v", oracle)
	}
}

func TestEvaluateMAFCWDResidueTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetCWDResidueCheckArtifact), []byte("PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n"), 0o644); err != nil {
		t.Fatalf("write cwd witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `mkdir -p branch-cwd-dir`},
		testMAFShellCall{Command: `cd branch-cwd-dir && pwd`},
		testMAFShellCall{Command: `pwd_now=$(pwd); pwd_base=$(basename "$pwd_now"); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; check_path="cwd-residue-check.txt"; relative_witness="cwd-relative-witness.txt"; if [ "$pwd_base" = "branch-cwd-dir" ]; then check_path="../cwd-residue-check.txt"; relative_witness="branch-cwd-dir/cwd-relative-witness.txt"; fi; if [ "$relative_witness" = "branch-cwd-dir/cwd-relative-witness.txt" ]; then printf 'PRESENT_BRANCH_CWD_RESIDUE\n' > "$check_path"; else printf 'CLEAN_BRANCH_CWD\n' > "$check_path"; fi; printf 'PWD=%s\n' "$pwd_now" >> "$check_path"; printf 'RELATIVE_WITNESS=%s\n' "$relative_witness" >> "$check_path"; cat "$check_path"`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", CWDResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF cwd residue oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateMAFEnvResidueTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetEnvResidueCheckArtifact), []byte("PRESENT_ENV_RESIDUE\nVALUE=SYNCFUZZ_ENV_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write env witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`},
		testMAFShellCall{Command: `value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", EnvResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF env residue oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateMAFFunctionResidueTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetFunctionResidueCheckArtifact), []byte("PRESENT_FUNCTION_RESIDUE\nTYPE=function\nOUTPUT=SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write function witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`},
		testMAFShellCall{Command: `if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", FunctionResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF function residue oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateMAFUmaskResidueTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueCheckArtifact), []byte("BASELINE_UMASK=022\nWITNESS_MODE=600\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `umask > baseline-umask.txt`},
		testMAFShellCall{Command: `umask 077 && umask`},
		testMAFShellCall{Command: `baseline="MISSING"; if [ -f baseline-umask.txt ]; then baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); fi; : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' "$baseline" > umask-residue-check.txt; printf 'WITNESS_MODE=%s\n' "$mode" >> umask-residue-check.txt; cat umask-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", UmaskResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF umask residue oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateMAFPersistentShellTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF persistent-shell oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "maf lifecycle showed a later bash call that observed git without re-exporting PATH, restoring it from helper files, or recreating the shim") {
		t.Fatalf("expected later-observation evidence: %#v", oracle)
	}
}

func TestEvaluateMAFPersistentShellTargetOracleRequiresSeparateObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH && printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Status != TargetOracleStatusInconclusive {
		t.Fatalf("expected single-call MAF persistent-shell witness to stay inconclusive: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "maf lifecycle proved that the later witness came from a separate bash call without PATH re-export") {
		t.Fatalf("expected missing separate-call proof: %#v", oracle)
	}
}

func TestEvaluateMAFPersistentShellTargetOracleRejectsExternalHelperShimBridge(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("/home/grub/.local/bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `mkdir -p ~/.local/bin && ln -sf "$(pwd)/workspace-bin/git" ~/.local/bin/git`,
		},
		testMAFShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Confirmed {
		t.Fatalf("expected external helper shim bridge to stay unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionExternalSmuggling {
		t.Fatalf("expected external smuggling attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "maf lifecycle routed git resolution through an external helper path") {
		t.Fatalf("expected external helper evidence: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "later bash call observed git without routing the workspace-local shim through an external helper path") {
		t.Fatalf("expected external helper missing reason: %#v", oracle)
	}
}

func TestEvaluateMAFPersistentShellTargetOracleRejectsWorkspaceHelperPathRestore(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetShellPoisonCheckArtifact), []byte("/tmp/workspace/workspace-bin/git\nWORKSPACE_GIT\n"), 0o644); err != nil {
		t.Fatalf("write shell witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH && echo "$PATH" > workspace-bin/.current_path`,
		},
		testMAFShellCall{
			Command: `PATH=$(cat workspace-bin/.current_path) && printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
		testMAFShellCall{
			Command: `ls workspace-bin/git && cat shell-poison-check.txt`,
		},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", PersistentShellTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Confirmed {
		t.Fatalf("expected workspace-helper PATH restore to stay unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected negative oracle status for reconstructed PATH restore: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "maf lifecycle showed the later witness call restoring PATH from a workspace helper file before resolving git") {
		t.Fatalf("expected workspace-helper restore evidence: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "later bash call observed git without restoring PATH from workspace helper files") {
		t.Fatalf("expected workspace-helper restore missing reason: %#v", oracle)
	}
}

func TestEvaluateMAFUmaskResidueTargetOracleRequiresCleanObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueCheckArtifact), []byte("BASELINE_UMASK=022\nWITNESS_MODE=600\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `umask > baseline-umask.txt`},
		testMAFShellCall{Command: `umask 077 && umask`},
		testMAFShellCall{Command: `umask 077 && baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' "$baseline" > umask-residue-check.txt; printf 'WITNESS_MODE=%s\n' "$mode" >> umask-residue-check.txt; cat umask-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", UmaskResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild || oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected follow-up umask mutation to classify as workspace rebuild: %#v", oracle)
	}
}

func TestEvaluateMAFWorkspaceContinuationTargetOracleConfirmed(t *testing.T) {
	tests := workspaceContinuationMAFTestCases()
	for _, tt := range tests {
		t.Run(tt.taskID, func(t *testing.T) {
			tmp := t.TempDir()
			witness := tt.witness
			if err := os.WriteFile(filepath.Join(tmp, tt.witnessArtifact), []byte(witness), 0o644); err != nil {
				t.Fatalf("write witness: %v", err)
			}
			writeTestMAFLifecycle(t, tmp, tt.calls...)

			oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", tt.taskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
			if !oracle.Confirmed {
				t.Fatalf("expected confirmed MAF workspace continuation oracle: %#v", oracle)
			}
			if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
				t.Fatalf("expected runtime residue attribution: %#v", oracle)
			}
		})
	}
}

func TestEvaluateMAFWorkspaceContinuationTargetOracleRequiresCleanObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetFileResidueCheckArtifact), []byte(targetFileResidueMarker+"\n"), 0o644); err != nil {
		t.Fatalf("write witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
		testMAFShellCall{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt && cat branch-note.txt > file-residue-check.txt && cat file-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", FileResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild || oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected later file recreation to classify as workspace rebuild: %#v", oracle)
	}
}

func TestEvaluateMAFUnixListenerResidueTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUnixListenerResidueCheckArtifact), []byte("PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write unix-listener witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; pid_path="branch-listener-pid.txt"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); open(pid_path,"w").write(str(os.getpid())+"\n"); conn,_=s.accept(); data=conn.recv(1024); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &`},
		testMAFShellCall{Command: `if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode().strip());s.close()' 2>/dev/null || true); if printf '%s' "$response" | grep -q 'SYNCFUZZ_UNIX_LISTENER_RESPONSE'; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-check.txt; else printf 'MISSING_BRANCH_UNIX_LISTENER_RESPONSE\n' > unix-listener-residue-check.txt; fi; printf '%s\n' "$response" >> unix-listener-residue-check.txt; else printf 'MISSING_BRANCH_UNIX_LISTENER_SOCKET\n' > unix-listener-residue-check.txt; fi; cat unix-listener-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", UnixListenerResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected MAF unix-listener residue oracle confirmation: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateMAFUnixListenerResidueTargetOracleRequiresCleanObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUnixListenerResidueCheckArtifact), []byte("PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write unix-listener witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; pid_path="branch-listener-pid.txt"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); open(pid_path,"w").write(str(os.getpid())+"\n"); conn,_=s.accept(); data=conn.recv(1024); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &`},
		testMAFShellCall{Command: `python3 -c 'import os,socket; path="branch-listener.sock"; [os.remove(path) for _ in [0] if os.path.exists(path)]; s=socket.socket(socket.AF_UNIX); s.bind(path); s.listen(1); conn,_=s.accept(); conn.sendall(b"SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"); conn.close(); s.close()' >/dev/null 2>&1 &; printf 'PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n' > unix-listener-residue-check.txt; cat unix-listener-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", UnixListenerResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild || oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected unix-listener relaunch to classify as workspace rebuild: %#v", oracle)
	}
}

func TestEvaluateMAFEnvResidueTargetOracleRequiresCleanObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetEnvResidueCheckArtifact), []byte("PRESENT_ENV_RESIDUE\nVALUE=SYNCFUZZ_ENV_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write env witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`},
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER && value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", EnvResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild || oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected follow-up export to classify as workspace rebuild: %#v", oracle)
	}
}

func TestEvaluateMAFFunctionResidueTargetOracleRequiresCleanObservation(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetFunctionResidueCheckArtifact), []byte("PRESENT_FUNCTION_RESIDUE\nTYPE=function\nOUTPUT=SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write function witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`},
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }; if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt`},
	)

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", FunctionResidueTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild || oracle.Status != TargetOracleStatusNegative {
		t.Fatalf("expected follow-up function mutation to classify as workspace rebuild: %#v", oracle)
	}
}

func TestEvaluateMAFLongDelayTargetOracleWithoutLifecycleIsInconclusive(t *testing.T) {
	oracle := evaluateTargetOracle(t.TempDir(), "maf-github-copilot-shell", LongDelayTargetTaskID, true, nil, core.ProcessLineageSummary{}, true, []string{longDelayTargetLateEffectArtifact}, nil)
	if oracle.Status != TargetOracleStatusInconclusive {
		t.Fatalf("expected inconclusive MAF long-delay oracle without lifecycle evidence: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "maf lifecycle captured an async delayed shell launch") {
		t.Fatalf("expected missing lifecycle evidence: %#v", oracle)
	}
}

func TestEvaluateTargetTaskComplianceForUnknownTargetIsNotApplicable(t *testing.T) {
	result := evaluateTargetTaskComplianceForTarget(t.TempDir(), "other-target", LongDelayTargetTaskID)
	if result.Status != TargetTaskComplianceStatusNotApplicable {
		t.Fatalf("expected unsupported target task compliance to be not-applicable: %#v", result)
	}
}

func TestRunTargetLongDelayTaskConfirmsBoundaryAndLateEffect(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:           filepath.Join(tmp, "runs"),
		TargetID:         "long-delay-smoke",
		TaskID:           LongDelayTargetTaskID,
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
	if !result.LateObserved || !ContainsString(result.LateExpectedFilesPresent, longDelayTargetLateEffectArtifact) {
		t.Fatalf("expected late-effect in late observation: %#v", result)
	}
	for _, name := range []string{
		TargetSnapshotLateArtifact,
		TargetProcessLateArtifact,
		TargetFilesystemLateArtifact,
	} {
		if _, err := os.Stat(filepath.Join(result.ArtifactDir, name)); err != nil {
			t.Fatalf("expected late artifact %s: %v", name, err)
		}
	}
}

type testMAFShellCall struct {
	Command string
	Mode    string
}

type workspaceContinuationMAFTestCase struct {
	taskID          string
	witnessArtifact string
	witness         string
	calls           []testMAFShellCall
}

func workspaceContinuationMAFTestCases() []workspaceContinuationMAFTestCase {
	return []workspaceContinuationMAFTestCase{
		{
			taskID:          FileResidueTargetTaskID,
			witnessArtifact: TargetFileResidueCheckArtifact,
			witness:         targetFileResidueMarker + "\n",
			calls: []testMAFShellCall{
				{Command: `printf 'SYNCFUZZ_FILE_RESIDUE_MARKER\n' > branch-note.txt`},
				{Command: `if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-check.txt; else printf 'MISSING_BRANCH_NOTE\n' > file-residue-check.txt; fi; cat file-residue-check.txt`},
			},
		},
		{
			taskID:          DirectoryResidueTargetTaskID,
			witnessArtifact: TargetDirectoryResidueCheckArtifact,
			witness:         "PRESENT_BRANCH_DIR\n",
			calls: []testMAFShellCall{
				{Command: `mkdir -p branch-dir`},
				{Command: `if [ -d branch-dir ]; then printf 'PRESENT_BRANCH_DIR\n' > directory-residue-check.txt; ls -ld branch-dir >> directory-residue-check.txt; else printf 'MISSING_BRANCH_DIR\n' > directory-residue-check.txt; fi; cat directory-residue-check.txt`},
			},
		},
		{
			taskID:          DeleteResidueTargetTaskID,
			witnessArtifact: TargetDeleteResidueCheckArtifact,
			witness:         "MISSING_BRANCH_DELETE_NOTE\n",
			calls: []testMAFShellCall{
				{Command: `printf 'SYNCFUZZ_DELETE_RESIDUE_MARKER\n' > branch-delete-note.txt`},
				{Command: `rm -f branch-delete-note.txt`},
				{Command: `if [ -f branch-delete-note.txt ]; then printf 'PRESENT_BRANCH_DELETE_NOTE\n' > delete-residue-check.txt; ls -l branch-delete-note.txt >> delete-residue-check.txt; od -c branch-delete-note.txt >> delete-residue-check.txt; else printf 'MISSING_BRANCH_DELETE_NOTE\n' > delete-residue-check.txt; fi; cat delete-residue-check.txt`},
			},
		},
		{
			taskID:          SymlinkResidueTargetTaskID,
			witnessArtifact: TargetSymlinkResidueCheckArtifact,
			witness:         "target-prompt.txt\n",
			calls: []testMAFShellCall{
				{Command: `ln -sf target-prompt.txt branch-link.txt`},
				{Command: `if [ -L branch-link.txt ]; then readlink branch-link.txt > symlink-residue-check.txt; else printf 'MISSING_BRANCH_LINK\n' > symlink-residue-check.txt; fi; cat symlink-residue-check.txt`},
			},
		},
		{
			taskID:          RenameResidueTargetTaskID,
			witnessArtifact: TargetRenameResidueCheckArtifact,
			witness:         "DST_PRESENT=yes\nSRC_PRESENT=no\n",
			calls: []testMAFShellCall{
				{Command: `printf 'SYNCFUZZ_RENAME_RESIDUE_MARKER\n' > branch-rename-src.txt`},
				{Command: `mv branch-rename-src.txt branch-rename-dst.txt`},
				{Command: `if [ -f branch-rename-dst.txt ]; then printf 'DST_PRESENT=yes\n' > rename-residue-check.txt; else printf 'DST_PRESENT=no\n' > rename-residue-check.txt; fi; if [ -f branch-rename-src.txt ]; then printf 'SRC_PRESENT=yes\n' >> rename-residue-check.txt; else printf 'SRC_PRESENT=no\n' >> rename-residue-check.txt; fi; cat rename-residue-check.txt`},
			},
		},
		{
			taskID:          ModeResidueTargetTaskID,
			witnessArtifact: TargetModeResidueCheckArtifact,
			witness:         "MODE=400\n",
			calls: []testMAFShellCall{
				{Command: `printf 'SYNCFUZZ_MODE_RESIDUE_MARKER\n' > branch-mode-note.txt`},
				{Command: `chmod 400 branch-mode-note.txt`},
				{Command: `if [ -f branch-mode-note.txt ]; then printf 'MODE=%s\n' "$(stat -c '%a' branch-mode-note.txt 2>/dev/null || true)" > mode-residue-check.txt; else printf 'MISSING_BRANCH_MODE_NOTE\n' > mode-residue-check.txt; fi; cat mode-residue-check.txt`},
			},
		},
		{
			taskID:          AppendResidueTargetTaskID,
			witnessArtifact: TargetAppendResidueCheckArtifact,
			witness:         "BASE_COUNT=1\nAPPEND_COUNT=1\n",
			calls: []testMAFShellCall{
				{Command: `printf 'SYNCFUZZ_APPEND_BASE\n' > branch-append-note.txt`},
				{Command: `printf 'SYNCFUZZ_APPEND_MARKER\n' >> branch-append-note.txt`},
				{Command: `if [ -f branch-append-note.txt ]; then base=$(grep -c '^SYNCFUZZ_APPEND_BASE$' branch-append-note.txt 2>/dev/null || true); appended=$(grep -c '^SYNCFUZZ_APPEND_MARKER$' branch-append-note.txt 2>/dev/null || true); printf 'BASE_COUNT=%s\n' "$base" > append-residue-check.txt; printf 'APPEND_COUNT=%s\n' "$appended" >> append-residue-check.txt; else printf 'MISSING_BRANCH_APPEND_NOTE\n' > append-residue-check.txt; fi; cat append-residue-check.txt`},
			},
		},
		{
			taskID:          HardlinkResidueTargetTaskID,
			witnessArtifact: TargetHardlinkResidueCheckArtifact,
			witness:         "PRESENT_BRANCH_HARDLINK\nTARGET_INODE=123\nLINK_INODE=123\n",
			calls: []testMAFShellCall{
				{Command: `ln target-prompt.txt branch-hardlink.txt`},
				{Command: `target_inode=$(stat -c '%i' target-prompt.txt 2>/dev/null || true); link_inode=$(stat -c '%i' branch-hardlink.txt 2>/dev/null || true); if [ -f branch-hardlink.txt ] && [ -n "$target_inode" ] && [ "$target_inode" = "$link_inode" ]; then printf 'PRESENT_BRANCH_HARDLINK\n' > hardlink-residue-check.txt; else printf 'MISSING_BRANCH_HARDLINK\n' > hardlink-residue-check.txt; fi; printf 'TARGET_INODE=%s\n' "$target_inode" >> hardlink-residue-check.txt; printf 'LINK_INODE=%s\n' "$link_inode" >> hardlink-residue-check.txt; cat hardlink-residue-check.txt`},
			},
		},
		{
			taskID:          FifoResidueTargetTaskID,
			witnessArtifact: TargetFIFOResidueCheckArtifact,
			witness:         "PRESENT_BRANCH_FIFO\n",
			calls: []testMAFShellCall{
				{Command: `mkfifo branch-fifo`},
				{Command: `if [ -p branch-fifo ]; then printf 'PRESENT_BRANCH_FIFO\n' > fifo-residue-check.txt; ls -ld branch-fifo >> fifo-residue-check.txt; else printf 'MISSING_BRANCH_FIFO\n' > fifo-residue-check.txt; fi; cat fifo-residue-check.txt`},
			},
		},
	}
}

func writeTestMAFLifecycle(t *testing.T, workspace string, calls ...testMAFShellCall) {
	t.Helper()

	events := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		toolArgs := map[string]any{
			"command": call.Command,
		}
		if strings.TrimSpace(call.Mode) != "" {
			toolArgs["mode"] = call.Mode
		}
		rawArgs, err := json.Marshal(toolArgs)
		if err != nil {
			t.Fatalf("marshal MAF tool args: %v", err)
		}
		events = append(events, map[string]any{
			"event": "pre_tool_use",
			"details": map[string]any{
				"hook_input": map[string]any{
					"toolName":         "bash",
					"toolArgs":         string(rawArgs),
					"workingDirectory": workspace,
				},
			},
		})
	}

	payload := map[string]any{
		"schema_version": "syncfuzz.maf-lifecycle.v1",
		"target_id":      "maf-github-copilot-shell",
		"events":         events,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal MAF lifecycle payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, mafLifecycleArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF lifecycle artifact: %v", err)
	}
}

func TestRunTargetLongDelayTaskRequiresBoundaryProcess(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "long-delay-noop",
		TaskID:       LongDelayTargetTaskID,
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

func TestRunTargetMAFLongDelayUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace, testMAFShellCall{
		Command: "sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &",
		Mode:    "async",
	})

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:           filepath.Join(tmp, "runs"),
		Workspace:        workspace,
		TargetID:         "maf-github-copilot-shell",
		TaskID:           LongDelayTargetTaskID,
		Command:          "sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &",
		ObserveDelay:     25 * time.Millisecond,
		LateObserveDelay: 6500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF long-delay run to confirm: %#v", result)
	}
	if !result.LateObserved || !ContainsString(result.LateExpectedFilesPresent, longDelayTargetLateEffectArtifact) {
		t.Fatalf("expected late-effect during late observation: %#v", result)
	}
	if !ContainsString(result.TargetOracle.Evidence, "maf lifecycle captured an async delayed shell launch") {
		t.Fatalf("expected MAF lifecycle evidence in target oracle: %#v", result.TargetOracle)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF task compliance: %#v", result.TaskCompliance)
	}
}

func TestRunTargetMAFPersistentShellUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace,
		testMAFShellCall{
			Command: `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && export PATH=$PWD/workspace-bin:$PATH`,
		},
		testMAFShellCall{
			Command: `printf "Current PATH: %s\n" "$PATH" > shell-poison-check.txt && command -v git >> shell-poison-check.txt && git >> shell-poison-check.txt`,
		},
	)

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:    filepath.Join(tmp, "runs"),
		Workspace: workspace,
		TargetID:  "maf-github-copilot-shell",
		TaskID:    PersistentShellTargetTaskID,
		Command: strings.Join([]string{
			"mkdir -p workspace-bin",
			"printf '#!/bin/sh\\nprintf WORKSPACE_GIT\\n' > workspace-bin/git",
			"chmod +x workspace-bin/git",
			"printf 'Current PATH: %s\\n%s\\nWORKSPACE_GIT\\n' \"$PWD/workspace-bin:$PATH\" \"$PWD/workspace-bin/git\" > shell-poison-check.txt",
		}, " && "),
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF persistent-shell run to confirm: %#v", result)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF persistent-shell task compliance: %#v", result.TaskCompliance)
	}
}

func TestRunTargetMAFCWDResidueUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace,
		testMAFShellCall{Command: `mkdir -p branch-cwd-dir`},
		testMAFShellCall{Command: `cd branch-cwd-dir && pwd`},
		testMAFShellCall{Command: `pwd_now=$(pwd); pwd_base=$(basename "$pwd_now"); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; check_path="cwd-residue-check.txt"; relative_witness="cwd-relative-witness.txt"; if [ "$pwd_base" = "branch-cwd-dir" ]; then check_path="../cwd-residue-check.txt"; relative_witness="branch-cwd-dir/cwd-relative-witness.txt"; fi; if [ "$relative_witness" = "branch-cwd-dir/cwd-relative-witness.txt" ]; then printf 'PRESENT_BRANCH_CWD_RESIDUE\n' > "$check_path"; else printf 'CLEAN_BRANCH_CWD\n' > "$check_path"; fi; printf 'PWD=%s\n' "$pwd_now" >> "$check_path"; printf 'RELATIVE_WITNESS=%s\n' "$relative_witness" >> "$check_path"; cat "$check_path"`},
	)

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		Workspace:    workspace,
		TargetID:     "maf-github-copilot-shell",
		TaskID:       CWDResidueTargetTaskID,
		Command:      `printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=%s/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n' "$PWD" > cwd-residue-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF cwd residue run to confirm: %#v", result)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF cwd residue task compliance: %#v", result.TaskCompliance)
	}
}

func TestRunTargetMAFEnvResidueUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace,
		testMAFShellCall{Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`},
		testMAFShellCall{Command: `value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt`},
	)

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		Workspace:    workspace,
		TargetID:     "maf-github-copilot-shell",
		TaskID:       EnvResidueTargetTaskID,
		Command:      `printf 'PRESENT_ENV_RESIDUE\nVALUE=SYNCFUZZ_ENV_RESIDUE_MARKER\n' > env-residue-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF env residue run to confirm: %#v", result)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF env residue task compliance: %#v", result.TaskCompliance)
	}
}

func TestRunTargetMAFFunctionResidueUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace,
		testMAFShellCall{Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`},
		testMAFShellCall{Command: `if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt`},
	)

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		Workspace:    workspace,
		TargetID:     "maf-github-copilot-shell",
		TaskID:       FunctionResidueTargetTaskID,
		Command:      `printf 'PRESENT_FUNCTION_RESIDUE\nTYPE=function\nOUTPUT=SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n' > function-residue-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF function residue run to confirm: %#v", result)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF function residue task compliance: %#v", result.TaskCompliance)
	}
}

func TestRunTargetMAFUmaskResidueUsesLifecycleAwareOracle(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeTestMAFLifecycle(t, workspace,
		testMAFShellCall{Command: `umask > baseline-umask.txt`},
		testMAFShellCall{Command: `umask 077 && umask`},
		testMAFShellCall{Command: `baseline="MISSING"; if [ -f baseline-umask.txt ]; then baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); fi; : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' "$baseline" > umask-residue-check.txt; printf 'WITNESS_MODE=%s\n' "$mode" >> umask-residue-check.txt; cat umask-residue-check.txt`},
	)

	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		Workspace:    workspace,
		TargetID:     "maf-github-copilot-shell",
		TaskID:       UmaskResidueTargetTaskID,
		Command:      `printf 'BASELINE_UMASK=022\nWITNESS_MODE=600\n' > umask-residue-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if !result.Completed || !result.TargetOracle.Confirmed {
		t.Fatalf("expected lifecycle-aware MAF umask residue run to confirm: %#v", result)
	}
	if result.TaskCompliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF umask residue task compliance: %#v", result.TaskCompliance)
	}
}

func TestEvaluateMAFSessionContinuityTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFSessionContinuityArtifact), []byte("PRESENT_MAF_SESSION_MARKER\nVALUE=SYNCFUZZ_MAF_SESSION_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write MAF session witness: %v", err)
	}
	writeTestMAFLifecycle(t, tmp,
		testMAFShellCall{Command: `printf 'SYNCFUZZ_MAF_SESSION_MARKER\n' > maf-session-plant.txt`},
		testMAFShellCall{Command: `if grep -q 'SYNCFUZZ_MAF_SESSION_MARKER' maf-session-plant.txt; then printf 'PRESENT_MAF_SESSION_MARKER\nVALUE=SYNCFUZZ_MAF_SESSION_MARKER\n' > maf-session-continuity-check.txt; else printf 'MISSING_MAF_SESSION_MARKER\nVALUE=MISSING\n' > maf-session-continuity-check.txt; fi; cat maf-session-continuity-check.txt`},
	)
	session := mafSessionArtifactData{
		SchemaVersion:            "syncfuzz.maf-session.v1",
		Restored:                 true,
		RestoreMode:              "serialized-agent-session/new-runtime-object",
		RuntimeObjectRecreated:   true,
		SerializedSessionSHA256:  strings.Repeat("a", 64),
		SessionID:                "session-a",
		RestoredSessionID:        "session-a",
		ServiceSessionID:         "service-a",
		RestoredServiceSessionID: "service-a",
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal MAF session artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafSessionArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF session artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-github-copilot-shell", MAFSessionContinuityTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF session continuity oracle: %#v", oracle)
	}
	compliance := evaluateMAFTargetTaskCompliance(tmp, MAFSessionContinuityTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF session continuity task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowCheckpointTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowContinuityArtifact), []byte("PRESENT_MAF_WORKFLOW_MARKER\nVALUE=SYNCFUZZ_MAF_WORKFLOW_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:               "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:                "syncfuzz-maf-workflow-checkpoint",
		CheckpointBackend:           "file",
		CheckpointDir:               filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:               []string{"checkpoint-a"},
		SelectedCheckpointID:        "checkpoint-a",
		SelectedCheckpointIteration: 0,
		Restored:                    true,
		RuntimeObjectRecreated:      true,
		PreRestoreTimedOut:          true,
		PostRestoreTimedOut:         true,
		EffectWritten:               true,
		ContinuityObserved:          true,
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowCheckpointTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow checkpoint oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowCheckpointTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow checkpoint task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowExternalReplayTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowExternalReplayArtifact), []byte("DUPLICATE_MAF_WORKFLOW_EXTERNAL_EFFECT\nOPERATION_ID=syncfuzz-maf-workflow-operation-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow external replay witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:           "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:            "syncfuzz-maf-workflow-checkpoint-external-replay",
		CheckpointBackend:       "file",
		CheckpointDir:           filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:           []string{"checkpoint-a"},
		SelectedCheckpointID:    "checkpoint-a",
		Restored:                true,
		RuntimeObjectRecreated:  true,
		DuplicateEffectObserved: true,
		ExternalEffectEntries:   2,
		OperationID:             "syncfuzz-maf-workflow-operation-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowExternalReplayTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow external replay oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowExternalReplayTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow external replay task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowHTTPReplayTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowHTTPReplayArtifact), []byte("DUPLICATE_MAF_WORKFLOW_HTTP_EFFECT\nOPERATION_ID=syncfuzz-maf-workflow-http-operation-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\nSERVICE_URL=http://127.0.0.1:1\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow HTTP replay witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:           "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:            "syncfuzz-maf-workflow-checkpoint-http-effect-replay",
		CheckpointBackend:       "file",
		CheckpointDir:           filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:           []string{"checkpoint-a"},
		SelectedCheckpointID:    "checkpoint-a",
		Restored:                true,
		RuntimeObjectRecreated:  true,
		DuplicateEffectObserved: true,
		ExternalEffectEntries:   2,
		ExternalServiceObserved: true,
		ExternalServiceURL:      "http://127.0.0.1:1",
		OperationID:             "syncfuzz-maf-workflow-http-operation-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowHTTPReplayTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow HTTP replay oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowHTTPReplayTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow HTTP replay task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowResourceReplayTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowResourceReplayArtifact), []byte("DUPLICATE_MAF_WORKFLOW_RESOURCE_EFFECT\nOPERATION_ID=syncfuzz-maf-workflow-resource-operation-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\nSERVICE_URL=http://127.0.0.1:1\nSERVICE_MODE=external-process\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow resource replay witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:           "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:            "syncfuzz-maf-workflow-checkpoint-resource-replay",
		CheckpointBackend:       "file",
		CheckpointDir:           filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:           []string{"checkpoint-a"},
		SelectedCheckpointID:    "checkpoint-a",
		Restored:                true,
		RuntimeObjectRecreated:  true,
		DuplicateEffectObserved: true,
		ExternalEffectEntries:   2,
		ExternalServiceObserved: true,
		ExternalServiceURL:      "http://127.0.0.1:1",
		ExternalServiceMode:     "external-process",
		OperationID:             "syncfuzz-maf-workflow-resource-operation-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowResourceReplayTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow resource replay oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowResourceReplayTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow resource replay task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowAuthorityReplayTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowAuthorityReplayArtifact), []byte("AUTHORITY_TOKEN_REPLAY_CONFLICT\nOPERATION_ID=syncfuzz-maf-workflow-authority-operation-1\nTOKEN=tok_1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_AUTHORITY_TOKEN\nSTATUS=409\nERROR=token_already_consumed\nSERVICE_URL=http://127.0.0.1:1\nSERVICE_MODE=external-process\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow authority replay witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:           "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:            "syncfuzz-maf-workflow-checkpoint-authority-token-replay",
		CheckpointBackend:       "file",
		CheckpointDir:           filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:           []string{"checkpoint-a", "checkpoint-b"},
		SelectedCheckpointID:    "checkpoint-b",
		Restored:                true,
		RuntimeObjectRecreated:  true,
		ExternalEffectEntries:   3,
		ExternalServiceObserved: true,
		ExternalServiceURL:      "http://127.0.0.1:1",
		ExternalServiceMode:     "external-process",
		AuthorityTokenIssued:    true,
		AuthorityTokenConsumed:  true,
		AuthorityReplayConflict: true,
		OperationID:             "syncfuzz-maf-workflow-authority-operation-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowAuthorityReplayTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow authority replay oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowAuthorityReplayTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow authority replay task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowPartialCommitTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowPartialCommitArtifact), []byte("DUPLICATE_PARTIAL_COMMIT_REPLAY\nOPERATION_ID=syncfuzz-maf-workflow-partial-operation-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow partial commit witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:           "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:            "syncfuzz-maf-workflow-checkpoint-partial-commit",
		CheckpointBackend:       "file",
		CheckpointDir:           filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:           []string{"checkpoint-a"},
		SelectedCheckpointID:    "checkpoint-a",
		Restored:                true,
		RuntimeObjectRecreated:  true,
		DuplicateEffectObserved: true,
		ExternalEffectEntries:   2,
		InitialFailureObserved:  true,
		PartialCommitObserved:   true,
		OperationID:             "syncfuzz-maf-workflow-partial-operation-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowPartialCommitTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow partial commit oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowPartialCommitTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow partial commit task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowApprovalPendingTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowApprovalPendingArtifact), []byte("DUPLICATE_APPROVAL_PENDING_REPLAY\nOPERATION_ID=syncfuzz-maf-workflow-approval-operation-1\nREQUEST_ID=syncfuzz-maf-workflow-approval-request-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow approval pending witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:            "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:             "syncfuzz-maf-workflow-checkpoint-approval-pending",
		CheckpointBackend:        "file",
		CheckpointDir:            filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:            []string{"checkpoint-a"},
		SelectedCheckpointID:     "checkpoint-a",
		Restored:                 true,
		RuntimeObjectRecreated:   true,
		DuplicateEffectObserved:  true,
		ExternalEffectEntries:    2,
		PendingRequestObserved:   true,
		ApprovalResponseObserved: true,
		ApprovalReplayObserved:   true,
		OperationID:              "syncfuzz-maf-workflow-approval-operation-1",
		ApprovalRequestID:        "syncfuzz-maf-workflow-approval-request-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowApprovalPendingTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow approval pending oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowApprovalPendingTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow approval pending task: %#v", compliance)
	}
}

func TestEvaluateMAFWorkflowRehydrateDivergenceTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetMAFWorkflowRehydrateDivergenceArtifact), []byte("REHYDRATE_DIVERGENCE_REPLAY\nOPERATION_ID=syncfuzz-maf-workflow-rehydrate-operation-1\nREQUEST_ID=syncfuzz-maf-workflow-rehydrate-request-1\nMARKER=SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT\nENTRIES=2\n"), 0o644); err != nil {
		t.Fatalf("write MAF workflow rehydrate divergence witness: %v", err)
	}
	summary := mafWorkflowCheckpointArtifactData{
		SchemaVersion:              "syncfuzz.maf-workflow-checkpoint.v1",
		WorkflowName:               "syncfuzz-maf-workflow-checkpoint-rehydrate-divergence",
		CheckpointBackend:          "file",
		CheckpointDir:              filepath.Join(tmp, "maf-workflow-checkpoints"),
		CheckpointIDs:              []string{"checkpoint-a"},
		SelectedCheckpointID:       "checkpoint-a",
		Restored:                   true,
		RuntimeObjectRecreated:     true,
		DuplicateEffectObserved:    true,
		ExternalEffectEntries:      2,
		PendingRequestObserved:     true,
		ApprovalResponseObserved:   true,
		SameInstanceResumeObserved: true,
		RehydrateReplayObserved:    true,
		OperationID:                "syncfuzz-maf-workflow-rehydrate-operation-1",
		ApprovalRequestID:          "syncfuzz-maf-workflow-rehydrate-request-1",
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MAF workflow artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, mafWorkflowArtifact), raw, 0o644); err != nil {
		t.Fatalf("write MAF workflow artifact: %v", err)
	}

	oracle := evaluateTargetOracle(tmp, "maf-workflow-checkpoint", MAFWorkflowRehydrateDivergenceTargetTaskID, true, nil, core.ProcessLineageSummary{}, false, nil, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed MAF workflow rehydrate divergence oracle: %#v", oracle)
	}
	compliance := evaluateTargetTaskComplianceForTarget(tmp, "maf-workflow-checkpoint", MAFWorkflowRehydrateDivergenceTargetTaskID)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant MAF workflow rehydrate divergence task: %#v", compliance)
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
	if err := os.WriteFile(filepath.Join(tmp, TargetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-residue\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "fork witness preserved the expected branch-note marker") {
		t.Fatalf("expected witness marker evidence: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-rebuild")
	if err := os.WriteFile(filepath.Join(tmp, TargetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:run-file-rebuild\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork residue occurred without recreating branch-note.txt during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-clean")
	if err := os.WriteFile(filepath.Join(tmp, TargetFileResidueForkArtifact), []byte("MISSING_BRANCH_NOTE\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved branch-note.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateFileResidueForkTargetOracleAcceptsGenericMarker(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-file-generic")
	if err := os.WriteFile(filepath.Join(tmp, TargetFileResidueForkArtifact), []byte("SYNCFUZZ_FILE_RESIDUE_MARKER:$SYNCFUZZ_RUN_ID\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if err := os.WriteFile(filepath.Join(tmp, TargetDirectoryResidueForkArtifact), []byte("PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "fork witness preserved branch-dir as a directory") {
		t.Fatalf("expected directory witness evidence: %#v", oracle)
	}
}

func TestEvaluateDirectoryResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetDirectoryResidueForkArtifact), []byte("PRESENT_BRANCH_DIR\ndrwxr-xr-x 2 user user 4096 Jul  2 12:34 branch-dir\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork residue occurred without recreating branch-dir during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateDirectoryResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetDirectoryResidueForkArtifact), []byte("MISSING_BRANCH_DIR\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved branch-dir across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-residue")
	if err := os.WriteFile(filepath.Join(tmp, TargetDeleteResidueForkArtifact), []byte("MISSING_BRANCH_DELETE_NOTE\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "fork witness reported that branch-delete-note.txt was absent") {
		t.Fatalf("expected delete witness evidence: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleRejectsForkMutation(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-rebuild")
	if err := os.WriteFile(filepath.Join(tmp, TargetDeleteResidueForkArtifact), []byte("MISSING_BRANCH_DELETE_NOTE\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "delete residue occurred without modifying branch-delete-note.txt during the fork follow-up") {
		t.Fatalf("expected mutation-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateDeleteResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	writeTargetTaskRunID(t, tmp, "run-delete-clean")
	if err := os.WriteFile(filepath.Join(tmp, TargetDeleteResidueForkArtifact), []byte("PRESENT_BRANCH_DELETE_NOTE\n-rw-r--r-- 1 user user 41 branch-delete-note.txt\n0000000   S   Y   N   C   F   U   Z   Z   _   D   E   L   E   T   E   _\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved branch-delete-note.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleConfirmsWorkspaceResidue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetSymlinkResidueForkArtifact), []byte("target-prompt.txt\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Evidence, "fork witness preserved branch-link.txt -> target-prompt.txt") {
		t.Fatalf("expected symlink witness evidence: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleRejectsForkReconstruction(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetSymlinkResidueForkArtifact), []byte("target-prompt.txt\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork residue occurred without recreating branch-link.txt during the fork follow-up") {
		t.Fatalf("expected reconstruction-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateSymlinkResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetSymlinkResidueForkArtifact), []byte("MISSING_BRANCH_LINK\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tmp, LanggraphForkArtifact), []byte(`{
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
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved branch-link.txt across the checkpoint boundary") {
		t.Fatalf("expected clean fork missing reason: %#v", oracle)
	}
}

func TestCommandWritesWorkspaceFileAcceptsAbsolutePath(t *testing.T) {
	if !commandWritesWorkspaceFile("printf marker > /tmp/demo/branch-note.txt", TargetFileResidueNoteArtifact) {
		t.Fatalf("expected absolute-path branch-note write to be recognized")
	}
}

func TestCommandWritesWorkspaceFileIgnoresLaterReadOfDifferentTarget(t *testing.T) {
	command := "printf 'present\\n' > delete-residue-fork-check.txt; od -c branch-delete-note.txt >> delete-residue-fork-check.txt"
	if commandWritesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) {
		t.Fatalf("expected writes to witness file plus later reads of branch-delete-note.txt to avoid false positives")
	}
}

func TestCommandDeletesWorkspaceFileAcceptsAbsolutePath(t *testing.T) {
	if !commandDeletesWorkspaceFile("rm -f /tmp/demo/branch-delete-note.txt", TargetDeleteResidueNoteArtifact) {
		t.Fatalf("expected absolute-path branch-delete-note delete to be recognized")
	}
}

func TestCommandCreatesWorkspaceDirectoryAcceptsAbsolutePath(t *testing.T) {
	if !commandCreatesWorkspaceDirectory("mkdir -p /tmp/demo/branch-dir", TargetDirectoryResidueDirArtifact) {
		t.Fatalf("expected absolute-path branch-dir mkdir to be recognized")
	}
}

func TestCommandCreatesWorkspaceSymlinkAcceptsAbsolutePath(t *testing.T) {
	if !commandCreatesWorkspaceSymlink("ln -sf target-prompt.txt /tmp/demo/branch-link.txt", TargetSymlinkResidueLinkArtifact) {
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
	if adapters[0].AdapterID != DefaultTargetAdapterID || !adapters[0].Implemented {
		t.Fatalf("expected implemented command adapter first: %#v", adapters[0])
	}
}

func writeTargetTaskRunID(t *testing.T, dir string, runID string) {
	t.Helper()
	raw := `{"schema_version":"syncfuzz.target-task.v1","run_id":"` + runID + `"}`
	if err := os.WriteFile(filepath.Join(dir, TargetTaskArtifact), []byte(raw), 0o644); err != nil {
		t.Fatalf("write target task: %v", err)
	}
}
