package syncfuzz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunTargetSuiteRepeatsSingleTask(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "suite-smoke",
		Tasks:        []string{defaultTargetTaskID},
		Command:      `printf ok > late-effect`,
		ObserveDelay: 10 * time.Millisecond,
		Repeat:       2,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if result.TotalRuns != 2 || result.Confirmed != 2 || result.Unconfirmed != 0 || result.Errors != 0 {
		t.Fatalf("unexpected target suite summary: %#v", result)
	}
	if len(result.TaskSummaries) != 1 || result.TaskSummaries[0].TaskID != defaultTargetTaskID || result.TaskSummaries[0].Confirmed != 2 {
		t.Fatalf("unexpected target suite task summary: %#v", result.TaskSummaries)
	}
	if len(result.ComplianceSummaries) != 1 || result.ComplianceSummaries[0].Status != targetTaskComplianceStatusNotApplicable || result.ComplianceSummaries[0].TotalRuns != 2 {
		t.Fatalf("unexpected target suite compliance summary: %#v", result.ComplianceSummaries)
	}
	if len(result.TaskSummaries[0].ComplianceSummaries) != 1 || result.TaskSummaries[0].ComplianceSummaries[0].Status != targetTaskComplianceStatusNotApplicable || result.TaskSummaries[0].ComplianceSummaries[0].TotalRuns != 2 {
		t.Fatalf("unexpected per-task compliance summary: %#v", result.TaskSummaries[0].ComplianceSummaries)
	}
	for _, item := range result.Results {
		if item.TaskCompliance.Status != targetTaskComplianceStatusNotApplicable {
			t.Fatalf("expected suite result to carry task compliance status: %#v", item.TaskCompliance)
		}
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetSuiteResultArtifact)); err != nil {
		t.Fatalf("expected target suite artifact: %v", err)
	}
}

func TestRunTargetSuiteSupportsMultipleBuiltInTasks(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p attacker-bin && printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git && chmod +x attacker-bin/git && printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "suite-multi-task",
		Tasks:        []string{defaultTargetTaskID, persistentShellTargetTaskID},
		Command:      command,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if result.TotalRuns != 2 || result.Confirmed != 2 || result.Errors != 0 {
		t.Fatalf("unexpected target suite counts: %#v", result)
	}
	if len(result.TaskSummaries) != 2 {
		t.Fatalf("expected two target task summaries: %#v", result.TaskSummaries)
	}
	for _, item := range result.Results {
		if !item.Confirmed || item.RunID == "" || item.ArtifactDir == "" {
			t.Fatalf("expected confirmed target suite item: %#v", item)
		}
	}
}

func TestDefaultTargetLateObserveDelayMapsLongDelayTask(t *testing.T) {
	if got := defaultTargetLateObserveDelay(longDelayTargetTaskID); got != defaultLongDelayLateObserveDelay {
		t.Fatalf("expected default long-delay late observe delay %s, got %s", defaultLongDelayLateObserveDelay, got)
	}
	if got := defaultTargetLateObserveDelay(persistentShellTargetTaskID); got != 0 {
		t.Fatalf("expected no default late observe delay for persistent shell task, got %s", got)
	}
}

func TestTargetTasksIncludesPersistentShellTask(t *testing.T) {
	tasks := TargetTasks()
	if len(tasks) < 17 {
		t.Fatalf("expected built-in target tasks: %#v", tasks)
	}
	foundPersistent := false
	foundReplay := false
	foundFork := false
	foundDirectoryFork := false
	foundDeleteFork := false
	foundSymlinkFork := false
	foundRenameFork := false
	foundModeFork := false
	foundAppendFork := false
	foundHardlinkFork := false
	foundFIFOFork := false
	foundOpenFDFork := false
	foundDeletedOpenFDFork := false
	foundInheritedFDLeak := false
	foundUnixListenerFork := false
	for _, task := range tasks {
		if task.TaskID == persistentShellTargetTaskID {
			foundPersistent = true
			if len(task.DefaultExpectedFiles) != 1 || task.DefaultExpectedFiles[0] != "shell-poison-check.txt" {
				t.Fatalf("unexpected persistent shell task metadata: %#v", task)
			}
		}
		if task.TaskID == persistentShellReplayTargetTaskID {
			foundReplay = true
			if !containsString(task.DefaultExpectedFiles, targetShellPoisonReplayArtifact) || !containsString(task.DefaultExpectedFiles, langgraphReplayArtifact) {
				t.Fatalf("unexpected replay task metadata: %#v", task)
			}
		}
		if task.TaskID == persistentShellForkTargetTaskID {
			foundFork = true
			if !containsString(task.DefaultExpectedFiles, targetShellPoisonForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected fork task metadata: %#v", task)
			}
		}
		if task.TaskID == directoryResidueForkTargetTaskID {
			foundDirectoryFork = true
			if !containsString(task.DefaultExpectedFiles, targetDirectoryResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected directory fork task metadata: %#v", task)
			}
		}
		if task.TaskID == deleteResidueForkTargetTaskID {
			foundDeleteFork = true
			if !containsString(task.DefaultExpectedFiles, targetDeleteResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected delete fork task metadata: %#v", task)
			}
		}
		if task.TaskID == symlinkResidueForkTargetTaskID {
			foundSymlinkFork = true
			if !containsString(task.DefaultExpectedFiles, targetSymlinkResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected symlink fork task metadata: %#v", task)
			}
		}
		if task.TaskID == renameResidueForkTargetTaskID {
			foundRenameFork = true
			if !containsString(task.DefaultExpectedFiles, targetRenameResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected rename fork task metadata: %#v", task)
			}
		}
		if task.TaskID == modeResidueForkTargetTaskID {
			foundModeFork = true
			if !containsString(task.DefaultExpectedFiles, targetModeResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected mode fork task metadata: %#v", task)
			}
		}
		if task.TaskID == appendResidueForkTargetTaskID {
			foundAppendFork = true
			if !containsString(task.DefaultExpectedFiles, targetAppendResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected append fork task metadata: %#v", task)
			}
		}
		if task.TaskID == hardlinkResidueForkTargetTaskID {
			foundHardlinkFork = true
			if !containsString(task.DefaultExpectedFiles, targetHardlinkResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected hardlink fork task metadata: %#v", task)
			}
		}
		if task.TaskID == fifoResidueForkTargetTaskID {
			foundFIFOFork = true
			if !containsString(task.DefaultExpectedFiles, targetFIFOResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected fifo fork task metadata: %#v", task)
			}
		}
		if task.TaskID == openFDResidueForkTargetTaskID {
			foundOpenFDFork = true
			if !containsString(task.DefaultExpectedFiles, targetOpenFDResidueForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected open-fd fork task metadata: %#v", task)
			}
		}
		if task.TaskID == deletedOpenFDForkTargetTaskID {
			foundDeletedOpenFDFork = true
			if !containsString(task.DefaultExpectedFiles, targetDeletedOpenFDForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected deleted-open-fd fork task metadata: %#v", task)
			}
		}
		if task.TaskID == inheritedFDLeakTargetTaskID {
			foundInheritedFDLeak = true
			if !containsString(task.DefaultExpectedFiles, targetInheritedFDLeakForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected inherited-fd branch leakage task metadata: %#v", task)
			}
		}
		if task.TaskID == unixListenerResidueForkTargetTaskID {
			foundUnixListenerFork = true
			if !containsString(task.DefaultExpectedFiles, targetUnixListenerForkArtifact) || !containsString(task.DefaultExpectedFiles, langgraphForkArtifact) {
				t.Fatalf("unexpected unix listener fork task metadata: %#v", task)
			}
		}
	}
	if !foundPersistent || !foundReplay || !foundFork || !foundDirectoryFork || !foundDeleteFork || !foundSymlinkFork || !foundRenameFork || !foundModeFork || !foundAppendFork || !foundHardlinkFork || !foundFIFOFork || !foundOpenFDFork || !foundDeletedOpenFDFork || !foundInheritedFDLeak || !foundUnixListenerFork {
		t.Fatalf("expected persistent shell replay/fork tasks plus workspace residue fork tasks in catalog: %#v", tasks)
	}
}

func TestExpandTargetTasksIncludesGroupsAndDeduplicates(t *testing.T) {
	tasks, groups, err := expandTargetTasks(
		[]string{deleteResidueForkTargetTaskID, symlinkResidueForkTargetTaskID},
		[]string{"workspace-residue", "workspace-residue"},
	)
	if err != nil {
		t.Fatalf("expandTargetTasks failed: %v", err)
	}
	if len(groups) != 1 || groups[0] != "workspace-residue" {
		t.Fatalf("unexpected normalized groups: %#v", groups)
	}
	expected := []string{
		fileResidueForkTargetTaskID,
		directoryResidueForkTargetTaskID,
		deleteResidueForkTargetTaskID,
		symlinkResidueForkTargetTaskID,
		renameResidueForkTargetTaskID,
		modeResidueForkTargetTaskID,
		appendResidueForkTargetTaskID,
		hardlinkResidueForkTargetTaskID,
		fifoResidueForkTargetTaskID,
		openFDResidueForkTargetTaskID,
		deletedOpenFDForkTargetTaskID,
		inheritedFDLeakTargetTaskID,
		unixListenerResidueForkTargetTaskID,
	}
	if len(tasks) != len(expected) {
		t.Fatalf("unexpected expanded task count: got %d want %d (%#v)", len(tasks), len(expected), tasks)
	}
	for i := range expected {
		if tasks[i] != expected[i] {
			t.Fatalf("unexpected expanded tasks: got %#v want %#v", tasks, expected)
		}
	}
}

func TestExpandTargetTasksRejectsUnknownGroup(t *testing.T) {
	if _, _, err := expandTargetTasks(nil, []string{"missing-group"}); err == nil {
		t.Fatalf("expected unknown target task group to fail")
	}
}

func TestTargetSuiteAttributionStatsCountsAndSorts(t *testing.T) {
	stats := make(map[string]*TargetSuiteAttributionStats)
	recordTargetSuiteAttribution(stats, targetOracleAttributionRuntimeResidue, true)
	recordTargetSuiteAttribution(stats, targetOracleAttributionRuntimeResidue, false)
	recordTargetSuiteAttribution(stats, targetOracleAttributionCleanFork, false)
	recordTargetSuiteAttribution(stats, "", true)

	got := targetSuiteAttributionStats(stats)
	if len(got) != 2 {
		t.Fatalf("unexpected attribution summary length: %#v", got)
	}
	if got[0].Attribution != targetOracleAttributionCleanFork || got[0].TotalRuns != 1 || got[0].Confirmed != 0 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected clean-fork attribution summary: %#v", got[0])
	}
	if got[1].Attribution != targetOracleAttributionRuntimeResidue || got[1].TotalRuns != 2 || got[1].Confirmed != 1 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected runtime residue attribution summary: %#v", got[1])
	}
}

func TestTargetSuiteComplianceStatsCountsAndSorts(t *testing.T) {
	stats := make(map[TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
	recordTargetSuiteCompliance(stats, targetTaskComplianceStatusViolated, false)
	recordTargetSuiteCompliance(stats, targetTaskComplianceStatusCompliant, true)
	recordTargetSuiteCompliance(stats, targetTaskComplianceStatusCompliant, false)
	recordTargetSuiteCompliance(stats, targetTaskComplianceStatusNotApplicable, true)
	recordTargetSuiteCompliance(stats, "", true)

	got := targetSuiteComplianceStats(stats)
	if len(got) != 3 {
		t.Fatalf("unexpected compliance summary length: %#v", got)
	}
	if got[0].Status != targetTaskComplianceStatusCompliant || got[0].TotalRuns != 2 || got[0].Confirmed != 1 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected compliant summary: %#v", got[0])
	}
	if got[1].Status != targetTaskComplianceStatusViolated || got[1].TotalRuns != 1 || got[1].Confirmed != 0 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected violated summary: %#v", got[1])
	}
	if got[2].Status != targetTaskComplianceStatusNotApplicable || got[2].TotalRuns != 1 || got[2].Confirmed != 1 || got[2].Unconfirmed != 0 {
		t.Fatalf("unexpected not-applicable summary: %#v", got[2])
	}
}
