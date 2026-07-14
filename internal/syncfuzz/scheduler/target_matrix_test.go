package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
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
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 7 {
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
	if replay.Objective == "" {
		t.Fatalf("expected replay objective metadata: %#v", replay)
	}
	if replay.ExecutionPlan == nil || replay.ExecutionPlan.CheckpointSelector != "before-path-export" || !replay.ExecutionPlan.Replay {
		t.Fatalf("expected replay execution plan metadata: %#v", replay)
	}
	if len(replay.Components) < 4 {
		t.Fatalf("expected replay structured components: %#v", replay)
	}
	if replay.MutationFocusID != "lifecycle-splice.checkpoint-replay" || replay.MutationFocusKind != target.TargetScenarioMutationLifecycleSplice {
		t.Fatalf("unexpected replay mutation focus: %#v", replay)
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
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 7 {
		t.Fatalf("unexpected seed-expanded matrix size: %#v", matrix)
	}
}

func TestBuildTargetScheduleMatrixAddsMutationFocusDerivedCandidates(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.PersistentShellReplayTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 3 {
		t.Fatalf("expected base plus two derived candidates, got %#v", matrix)
	}
	want := map[string]string{
		"langgraph-shell-react/persistent-shell-poisoning-replay":                    target.TargetPromptVariantBaseID,
		"langgraph-shell-react/persistent-shell-poisoning-replay/lifecycle-boundary": target.TargetPromptVariantLifecycleBoundaryID,
		"langgraph-shell-react/persistent-shell-poisoning-replay/mutation-focus":     target.TargetPromptVariantMutationFocusID,
	}
	for _, candidate := range matrix.Candidates {
		wantVariant, ok := want[candidate.CandidateID]
		if !ok {
			t.Fatalf("unexpected candidate id: %#v", candidate)
		}
		if candidate.PromptVariantID != wantVariant {
			t.Fatalf("unexpected prompt variant for %q: %#v", candidate.CandidateID, candidate)
		}
		if wantVariant == target.TargetPromptVariantBaseID && candidate.Generated {
			t.Fatalf("base candidate should not be generated: %#v", candidate)
		}
		if wantVariant != target.TargetPromptVariantBaseID && !candidate.Generated {
			t.Fatalf("derived candidate should be marked generated: %#v", candidate)
		}
	}
}

func TestBuildTargetScheduleMatrixSupportsMAFBaselineGroup(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "maf-github-copilot-shell",
		TaskGroups: []string{"maf-baseline"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.TaskGroups) != 1 || matrix.TaskGroups[0] != "maf-baseline" {
		t.Fatalf("unexpected MAF task groups: %#v", matrix.TaskGroups)
	}
	if len(matrix.Tasks) != 2 {
		t.Fatalf("expected two MAF baseline tasks, got %#v", matrix.Tasks)
	}
	if matrix.TotalCandidates != 3 {
		t.Fatalf("expected base orphan-process plus long-delay base/mutation candidates, got %#v", matrix)
	}
	late, err := findTargetMatrixCandidate(matrix, target.LongDelayTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if late.TargetID != "maf-github-copilot-shell" || late.SeedID != "delayed-effect" {
		t.Fatalf("unexpected MAF long-delay candidate metadata: %#v", late)
	}
	if late.ContractProfileID != "" || late.ContractRuleID != "" {
		t.Fatalf("expected no contract metadata for current MAF baseline: %#v", late)
	}
}

func TestBuildTargetScheduleMatrixSupportsMAFShellContextGroup(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "maf-github-copilot-shell",
		TaskGroups: []string{"maf-shell-context"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.TaskGroups) != 1 || matrix.TaskGroups[0] != "maf-shell-context" {
		t.Fatalf("unexpected MAF shell-context groups: %#v", matrix.TaskGroups)
	}
	if len(matrix.Tasks) != 7 {
		t.Fatalf("expected seven MAF shell-context tasks, got %#v", matrix.Tasks)
	}
	if matrix.TotalCandidates != 12 {
		t.Fatalf("expected delayed-effect plus PATH/env/function/cwd/umask shell-context candidates, got %#v", matrix)
	}
	persistent, err := findTargetMatrixCandidate(matrix, target.PersistentShellTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if persistent.TargetID != "maf-github-copilot-shell" || persistent.SeedID != "shell-path-residue" {
		t.Fatalf("unexpected MAF persistent-shell candidate metadata: %#v", persistent)
	}
	cwd, err := findTargetMatrixCandidate(matrix, target.CWDResidueTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if cwd.TargetID != "maf-github-copilot-shell" || cwd.SeedID != "shell-execution-context-residue" {
		t.Fatalf("unexpected MAF cwd residue candidate metadata: %#v", cwd)
	}
	envResidue, err := findTargetMatrixCandidate(matrix, target.EnvResidueTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if envResidue.TargetID != "maf-github-copilot-shell" || envResidue.SeedID != "shell-execution-context-residue" {
		t.Fatalf("unexpected MAF env residue candidate metadata: %#v", envResidue)
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
	taskCoverage := findTargetDimensionCoverage(t, result.DimensionCoverage, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 2 || taskCoverage.ConfirmedValues != 2 {
		t.Fatalf("unexpected matrix task coverage: %#v", taskCoverage)
	}
	for _, item := range result.Results {
		if item.CandidateID == "" || item.TargetID != "matrix-smoke" {
			t.Fatalf("expected candidate-aware target run result: %#v", item)
		}
		if item.TaskID == target.PersistentShellTargetTaskID {
			if item.MinimizationPlan == nil || !item.MinimizationPlan.Applicable {
				t.Fatalf("expected applicable minimization plan for confirmed matrix item: %#v", item.MinimizationPlan)
			}
			if len(item.MinimizationPlan.Steps) == 0 {
				t.Fatalf("expected minimization steps: %#v", item.MinimizationPlan)
			}
			if !containsStringPrefix(item.MinimizationPlan.Preserve, "artifact="+target.TargetShellPoisonCheckArtifact) {
				t.Fatalf("expected minimization plan to preserve witness artifact: %#v", item.MinimizationPlan.Preserve)
			}
			if !targetMinimizationPlanHasStepKind(item.MinimizationPlan, "artifact-replay-check") {
				t.Fatalf("expected replay minimization step: %#v", item.MinimizationPlan)
			}
		}
	}
	if _, err := os.Stat(result.ScheduleMatrix); err != nil {
		t.Fatalf("expected target schedule matrix artifact: %v", err)
	}
	if _, err := os.Stat(result.MatrixResult); err != nil {
		t.Fatalf("expected target matrix result artifact: %v", err)
	}
}

func TestBuildTargetMinimizationPlanIncludesMutationAxes(t *testing.T) {
	plan := buildTargetMinimizationPlan(TargetScheduleCandidate{
		TargetID:             "langgraph-shell-react",
		TaskID:               target.PersistentShellReplayTargetTaskID,
		OracleKindID:         "replay-path-residue",
		DefaultExpectedFiles: []string{target.TargetShellPoisonReplayArtifact},
		Mutations: []target.TargetScenarioMutation{{
			MutationID: "lifecycle-splice.checkpoint-replay",
			Kind:       target.TargetScenarioMutationLifecycleSplice,
			Summary:    "resume from an earlier checkpoint",
		}},
	}, TargetSuiteRunResult{
		TargetID:     "langgraph-shell-react",
		TaskID:       target.PersistentShellReplayTargetTaskID,
		OracleKindID: "replay-path-residue",
		Confirmed:    true,
		TargetOracle: target.TargetOracleResult{Attribution: "runtime-preserved-residue"},
	}, corpus.TargetObservationDetails{
		Category:          corpus.TargetObservationResidueObserved,
		ActivationReached: true,
	})
	if plan == nil || !plan.Applicable {
		t.Fatalf("expected applicable minimization plan: %#v", plan)
	}
	if !targetMinimizationPlanHasStepKind(plan, "mutation-axis-check") {
		t.Fatalf("expected mutation-axis minimization step: %#v", plan)
	}
	if !containsStringPrefix(plan.Preserve, "artifact="+target.TargetShellPoisonReplayArtifact) {
		t.Fatalf("expected replay witness artifact in preserve list: %#v", plan.Preserve)
	}
}

func containsStringPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func targetMinimizationPlanHasStepKind(plan *TargetMinimizationPlan, kind string) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if step.Kind == kind {
			return true
		}
	}
	return false
}

func TestRunTargetSuiteFeedbackMatrixPreservesUniverseCoverageGaps(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:         filepath.Join(tmp, "runs"),
		TargetID:       "matrix-frontier-smoke",
		Tasks:          []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:        command,
		ObserveDelay:   10 * time.Millisecond,
		Matrix:         true,
		CandidateLimit: 1,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}

	taskCoverage := findTargetDimensionCoverage(t, result.DimensionCoverage, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 1 || len(taskCoverage.MissingValues) != 1 {
		t.Fatalf("expected universe-aware coverage gaps after limited matrix run: %#v", taskCoverage)
	}
	if len(result.FrontierCandidates) != 1 {
		t.Fatalf("expected exactly one next frontier candidate, got %#v", result.FrontierCandidates)
	}
	if result.FrontierCandidates[0].CandidateID == result.Results[0].CandidateID {
		t.Fatalf("expected frontier to point at the remaining candidate, got %#v", result.FrontierCandidates[0])
	}
	if result.FrontierCandidates[0].SelectionMode == "" {
		t.Fatalf("expected frontier recommendation metadata: %#v", result.FrontierCandidates[0])
	}
}
