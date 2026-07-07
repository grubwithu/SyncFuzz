package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestRunTargetSuiteRepeatsSingleTask(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "suite-smoke",
		Tasks:        []string{target.DefaultTargetTaskID},
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
	if len(result.TaskSummaries) != 1 || result.TaskSummaries[0].TaskID != target.DefaultTargetTaskID || result.TaskSummaries[0].Confirmed != 2 {
		t.Fatalf("unexpected target suite task summary: %#v", result.TaskSummaries)
	}
	if len(result.ComplianceSummaries) != 1 || result.ComplianceSummaries[0].Status != target.TargetTaskComplianceStatusNotApplicable || result.ComplianceSummaries[0].TotalRuns != 2 {
		t.Fatalf("unexpected target suite compliance summary: %#v", result.ComplianceSummaries)
	}
	if len(result.TaskSummaries[0].ComplianceSummaries) != 1 || result.TaskSummaries[0].ComplianceSummaries[0].Status != target.TargetTaskComplianceStatusNotApplicable || result.TaskSummaries[0].ComplianceSummaries[0].TotalRuns != 2 {
		t.Fatalf("unexpected per-task compliance summary: %#v", result.TaskSummaries[0].ComplianceSummaries)
	}
	for _, item := range result.Results {
		if item.TaskCompliance.Status != target.TargetTaskComplianceStatusNotApplicable {
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
		Tasks:        []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
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
	if got := target.DefaultTargetLateObserveDelay(target.LongDelayTargetTaskID); got != target.DefaultLongDelayLateObserveDelay {
		t.Fatalf("expected default long-delay late observe delay %s, got %s", target.DefaultLongDelayLateObserveDelay, got)
	}
	if got := target.DefaultTargetLateObserveDelay(target.PersistentShellTargetTaskID); got != 0 {
		t.Fatalf("expected no default late observe delay for persistent shell task, got %s", got)
	}
}

func TestTargetTasksIncludesPersistentShellTask(t *testing.T) {
	tasks := target.TargetTasks()
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
		if task.TaskID == target.PersistentShellTargetTaskID {
			foundPersistent = true
			if len(task.DefaultExpectedFiles) != 1 || task.DefaultExpectedFiles[0] != "shell-poison-check.txt" {
				t.Fatalf("unexpected persistent shell task metadata: %#v", task)
			}
		}
		if task.TaskID == target.PersistentShellReplayTargetTaskID {
			foundReplay = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetShellPoisonReplayArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphReplayArtifact) {
				t.Fatalf("unexpected replay task metadata: %#v", task)
			}
		}
		if task.TaskID == target.PersistentShellForkTargetTaskID {
			foundFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetShellPoisonForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.DirectoryResidueForkTargetTaskID {
			foundDirectoryFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetDirectoryResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected directory fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.DeleteResidueForkTargetTaskID {
			foundDeleteFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetDeleteResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected delete fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.SymlinkResidueForkTargetTaskID {
			foundSymlinkFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetSymlinkResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected symlink fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.RenameResidueForkTargetTaskID {
			foundRenameFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetRenameResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected rename fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.ModeResidueForkTargetTaskID {
			foundModeFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetModeResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected mode fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.AppendResidueForkTargetTaskID {
			foundAppendFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetAppendResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected append fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.HardlinkResidueForkTargetTaskID {
			foundHardlinkFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetHardlinkResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected hardlink fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.FifoResidueForkTargetTaskID {
			foundFIFOFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetFIFOResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected fifo fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.OpenFDResidueForkTargetTaskID {
			foundOpenFDFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetOpenFDResidueForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected open-fd fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.DeletedOpenFDForkTargetTaskID {
			foundDeletedOpenFDFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetDeletedOpenFDForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected deleted-open-fd fork task metadata: %#v", task)
			}
		}
		if task.TaskID == target.InheritedFDLeakTargetTaskID {
			foundInheritedFDLeak = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetInheritedFDLeakForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected inherited-fd branch leakage task metadata: %#v", task)
			}
		}
		if task.TaskID == target.UnixListenerResidueForkTargetTaskID {
			foundUnixListenerFork = true
			if !target.ContainsString(task.DefaultExpectedFiles, target.TargetUnixListenerForkArtifact) || !target.ContainsString(task.DefaultExpectedFiles, target.LanggraphForkArtifact) {
				t.Fatalf("unexpected unix listener fork task metadata: %#v", task)
			}
		}
	}
	if !foundPersistent || !foundReplay || !foundFork || !foundDirectoryFork || !foundDeleteFork || !foundSymlinkFork || !foundRenameFork || !foundModeFork || !foundAppendFork || !foundHardlinkFork || !foundFIFOFork || !foundOpenFDFork || !foundDeletedOpenFDFork || !foundInheritedFDLeak || !foundUnixListenerFork {
		t.Fatalf("expected persistent shell replay/fork tasks plus workspace residue fork tasks in catalog: %#v", tasks)
	}
}

func TestExpandTargetTasksIncludesGroupsAndDeduplicates(t *testing.T) {
	tasks, groups, err := target.ExpandTargetTasks(
		[]string{target.DeleteResidueForkTargetTaskID, target.SymlinkResidueForkTargetTaskID},
		[]string{"workspace-residue", "workspace-residue"},
	)
	if err != nil {
		t.Fatalf("target.ExpandTargetTasks failed: %v", err)
	}
	if len(groups) != 1 || groups[0] != "workspace-residue" {
		t.Fatalf("unexpected normalized groups: %#v", groups)
	}
	expected := []string{
		target.FileResidueForkTargetTaskID,
		target.DirectoryResidueForkTargetTaskID,
		target.DeleteResidueForkTargetTaskID,
		target.SymlinkResidueForkTargetTaskID,
		target.RenameResidueForkTargetTaskID,
		target.ModeResidueForkTargetTaskID,
		target.AppendResidueForkTargetTaskID,
		target.HardlinkResidueForkTargetTaskID,
		target.FifoResidueForkTargetTaskID,
		target.OpenFDResidueForkTargetTaskID,
		target.DeletedOpenFDForkTargetTaskID,
		target.InheritedFDLeakTargetTaskID,
		target.UnixListenerResidueForkTargetTaskID,
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
	if _, _, err := target.ExpandTargetTasks(nil, []string{"missing-group"}); err == nil {
		t.Fatalf("expected unknown target task group to fail")
	}
}

func TestTargetScenarioSeedsExposeShellPathFamily(t *testing.T) {
	seeds := target.TargetScenarioSeeds()
	var shellSeed target.TargetScenarioSeedInfo
	found := false
	for _, seed := range seeds {
		if seed.SeedID == "shell-path-residue" {
			shellSeed = seed
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected shell-path-residue seed in catalog: %#v", seeds)
	}
	if len(shellSeed.Tasks) != 3 {
		t.Fatalf("expected three shell-path tasks, got %#v", shellSeed)
	}
	if !target.ContainsString(shellSeed.PlantPrimitives, "shell-path-prepend") {
		t.Fatalf("expected shell primitive in seed: %#v", shellSeed)
	}
	if !target.ContainsString(shellSeed.LifecycleOperations, "run-continue") || !target.ContainsString(shellSeed.LifecycleOperations, "checkpoint-replay") || !target.ContainsString(shellSeed.LifecycleOperations, "checkpoint-fork") {
		t.Fatalf("expected shell lifecycle variants in seed: %#v", shellSeed)
	}
}

func TestExpandTargetSelectionIncludesSeedsAndDeduplicates(t *testing.T) {
	tasks, groups, seeds, err := target.ExpandTargetSelection(
		[]string{target.PersistentShellForkTargetTaskID},
		[]string{"shell-lifecycle"},
		[]string{"shell-path-residue", "shell-path-residue"},
	)
	if err != nil {
		t.Fatalf("target.ExpandTargetSelection failed: %v", err)
	}
	if len(groups) != 1 || groups[0] != "shell-lifecycle" {
		t.Fatalf("unexpected normalized groups: %#v", groups)
	}
	if len(seeds) != 1 || seeds[0] != "shell-path-residue" {
		t.Fatalf("unexpected normalized seeds: %#v", seeds)
	}
	expected := []string{
		target.PersistentShellTargetTaskID,
		target.PersistentShellReplayTargetTaskID,
		target.PersistentShellForkTargetTaskID,
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

func TestTargetSuiteAttributionStatsCountsAndSorts(t *testing.T) {
	stats := make(map[string]*TargetSuiteAttributionStats)
	recordTargetSuiteAttribution(stats, target.TargetOracleAttributionRuntimeResidue, true)
	recordTargetSuiteAttribution(stats, target.TargetOracleAttributionRuntimeResidue, false)
	recordTargetSuiteAttribution(stats, target.TargetOracleAttributionCleanFork, false)
	recordTargetSuiteAttribution(stats, "", true)

	got := targetSuiteAttributionStats(stats)
	if len(got) != 2 {
		t.Fatalf("unexpected attribution summary length: %#v", got)
	}
	if got[0].Attribution != target.TargetOracleAttributionCleanFork || got[0].TotalRuns != 1 || got[0].Confirmed != 0 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected clean-fork attribution summary: %#v", got[0])
	}
	if got[1].Attribution != target.TargetOracleAttributionRuntimeResidue || got[1].TotalRuns != 2 || got[1].Confirmed != 1 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected runtime residue attribution summary: %#v", got[1])
	}
}

func TestTargetSuiteComplianceStatsCountsAndSorts(t *testing.T) {
	stats := make(map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
	recordTargetSuiteCompliance(stats, target.TargetTaskComplianceStatusViolated, false)
	recordTargetSuiteCompliance(stats, target.TargetTaskComplianceStatusCompliant, true)
	recordTargetSuiteCompliance(stats, target.TargetTaskComplianceStatusCompliant, false)
	recordTargetSuiteCompliance(stats, target.TargetTaskComplianceStatusNotApplicable, true)
	recordTargetSuiteCompliance(stats, "", true)

	got := targetSuiteComplianceStats(stats)
	if len(got) != 3 {
		t.Fatalf("unexpected compliance summary length: %#v", got)
	}
	if got[0].Status != target.TargetTaskComplianceStatusCompliant || got[0].TotalRuns != 2 || got[0].Confirmed != 1 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected compliant summary: %#v", got[0])
	}
	if got[1].Status != target.TargetTaskComplianceStatusViolated || got[1].TotalRuns != 1 || got[1].Confirmed != 0 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected violated summary: %#v", got[1])
	}
	if got[2].Status != target.TargetTaskComplianceStatusNotApplicable || got[2].TotalRuns != 1 || got[2].Confirmed != 1 || got[2].Unconfirmed != 0 {
		t.Fatalf("unexpected not-applicable summary: %#v", got[2])
	}
}
