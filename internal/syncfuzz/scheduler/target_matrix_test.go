package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestBuildTargetScheduleMatrixExpandsGroupsAndContracts(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "langgraph-shell-react",
		TaskGroups: []string{"shell-lifecycle"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.SchemaVersion != "syncfuzz.target-schedule-matrix.v1" {
		t.Fatalf("unexpected schema version %q", matrix.SchemaVersion)
	}
	if matrix.TargetID != "langgraph-shell-react" {
		t.Fatalf("unexpected target id %q", matrix.TargetID)
	}
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 3 {
		t.Fatalf("unexpected target matrix size: %#v", matrix)
	}
	replay, err := findTargetMatrixCandidate(matrix, target.PersistentShellReplayTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if replay.ContractRuleID != "shell-path-replay-boundary" || replay.ContractExpectation != target.TargetContractExpectationReset {
		t.Fatalf("unexpected replay candidate contract metadata: %#v", replay)
	}
	if replay.SeedID != "shell-path-residue" || replay.PlantPrimitiveID != "shell-path-prepend" {
		t.Fatalf("unexpected replay scenario seed metadata: %#v", replay)
	}
	if replay.LifecycleOperationID != "checkpoint-replay" || replay.ActivationKindID != "git-resolution" || replay.OracleKindID != "replay-path-residue" {
		t.Fatalf("unexpected replay scenario execution metadata: %#v", replay)
	}
	if len(replay.Mutations) == 0 || replay.Mutations[0].Kind != target.TargetScenarioMutationLifecycleSplice {
		t.Fatalf("expected replay candidate mutations: %#v", replay)
	}
}

func TestBuildTargetScheduleMatrixExpandsSeeds(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		SeedIDs:  []string{"shell-path-residue"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.SeedIDs) != 1 || matrix.SeedIDs[0] != "shell-path-residue" {
		t.Fatalf("unexpected seed ids: %#v", matrix)
	}
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 3 {
		t.Fatalf("unexpected seed-expanded matrix size: %#v", matrix)
	}
}

func TestRunTargetSuiteMatrixWritesArtifacts(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "matrix-smoke",
		Tasks:        []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:      command,
		ObserveDelay: 10 * time.Millisecond,
		Matrix:       true,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if result.SchedulerMode != suiteSchedulerMatrix {
		t.Fatalf("expected target matrix scheduler, got %q", result.SchedulerMode)
	}
	if result.TotalCandidates != 2 || result.TotalRuns != 2 {
		t.Fatalf("unexpected target matrix counts: %#v", result)
	}
	if result.ScheduleMatrix == "" || result.MatrixResult == "" {
		t.Fatalf("expected target matrix artifacts: %#v", result)
	}
	if len(result.CandidateSummaries) != 2 {
		t.Fatalf("expected two target candidate summaries: %#v", result.CandidateSummaries)
	}
	for _, item := range result.Results {
		if item.CandidateID == "" || item.TargetID != "matrix-smoke" {
			t.Fatalf("expected candidate-aware target run result: %#v", item)
		}
	}
	if _, err := os.Stat(result.ScheduleMatrix); err != nil {
		t.Fatalf("expected target schedule matrix artifact: %v", err)
	}
	if _, err := os.Stat(result.MatrixResult); err != nil {
		t.Fatalf("expected target matrix result artifact: %v", err)
	}
}
