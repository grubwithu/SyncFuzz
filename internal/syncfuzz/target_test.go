package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
