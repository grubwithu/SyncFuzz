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
	if len(tasks) < 7 {
		t.Fatalf("expected built-in target tasks: %#v", tasks)
	}
	foundPersistent := false
	foundReplay := false
	foundFork := false
	foundDeleteFork := false
	foundSymlinkFork := false
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
	}
	if !foundPersistent || !foundReplay || !foundFork || !foundDeleteFork || !foundSymlinkFork {
		t.Fatalf("expected persistent shell replay/fork tasks plus filesystem fork tasks in catalog: %#v", tasks)
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
		deleteResidueForkTargetTaskID,
		symlinkResidueForkTargetTaskID,
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
