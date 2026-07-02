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
