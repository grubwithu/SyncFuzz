package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestRunTargetMinimizationReducesPromptAndPreservesOracle(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{
		LifecycleOperationID: "run-continue",
		CheckpointBackend:    "disk",
		ProcessMode:          "single",
	}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "minimizer-persistent-shell",
		TaskID:           target.PersistentShellTargetTaskID,
		SeedID:           "shell-path-residue",
		Description:      "Persistent shell minimizer fixture with an optional setup component.",
		Objective:        "Confirm that a workspace-local git shim is still observed by the later check.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "setup.optional-helper",
				Role:        target.TargetScenarioComponentSetup,
				KindID:      "optional-helper",
				Summary:     "optional helper setup that is not needed for the witness",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "prepare workspace\nplant path state\nactivate later\npreserve witness",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{Order: 1, StepID: "m1", Kind: "prompt-reduction", Summary: "reduce prompt"},
						{
							Order:         2,
							StepID:        "m2",
							Kind:          "component-deletion",
							ComponentID:   "setup.optional-helper",
							ComponentKind: "optional-helper",
							ComponentRole: target.TargetScenarioComponentSetup,
							Summary:       "try deleting optional helper setup",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      8,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if result.ExecutedCandidates != 1 || result.TotalTrials == 0 || result.AcceptedReductions == 0 {
		t.Fatalf("expected executed prompt reductions: %#v", result)
	}
	if result.Fidelity != TargetMinimizationFidelityExact {
		t.Fatalf("expected default exact fidelity: %#v", result)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one minimization candidate: %#v", result.Candidates)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.MinimizedPromptLines >= item.OriginalPromptLines || item.MinimizedArtifactDir == "" {
		t.Fatalf("expected a smaller preserved prompt: %#v", item)
	}
	if item.AcceptedExecutionReductions != 2 || item.MinimizedExecutionPlan == nil {
		t.Fatalf("expected process mode and checkpoint backend reductions: %#v", item)
	}
	if item.AcceptedComponentReductions != 1 || item.MinimizedComponents >= item.OriginalComponents {
		t.Fatalf("expected an accepted Scenario IR component deletion: %#v", item)
	}
	if item.MinimizedExecutionPlan.ProcessMode != "" || item.MinimizedExecutionPlan.CheckpointBackend != "" {
		t.Fatalf("expected minimized execution plan defaults: %#v", item.MinimizedExecutionPlan)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	for _, component := range minimizedTask.Scenario.Components {
		if component.ComponentID == "setup.optional-helper" {
			t.Fatalf("expected optional setup component to be removed from minimized artifact: %#v", minimizedTask.Scenario.Components)
		}
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetMinimizationResultArtifact)); err != nil {
		t.Fatalf("expected minimization result artifact: %v", err)
	}
}

func readMinimizedTargetTaskForTest(t *testing.T, artifactDir string) target.TargetTask {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(artifactDir, target.TargetTaskArtifact))
	if err != nil {
		t.Fatalf("read minimized target task: %v", err)
	}
	var task target.TargetTask
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatalf("decode minimized target task: %v", err)
	}
	if task.Scenario == nil {
		t.Fatalf("expected minimized target task to preserve Scenario IR")
	}
	return task
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRunTargetMinimizationSemanticFidelityAcceptsPlantMetadataReduction(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{LifecycleOperationID: "run-continue"}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "semantic-plant-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible plant metadata.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "plant.required",
				Role:        target.TargetScenarioComponentPlant,
				KindID:      "shell-path-prepend",
				Summary:     "plant PATH shim metadata",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "semantic-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:         1,
							StepID:        "m1",
							Kind:          "primitive-minimization",
							ComponentID:   "plant.required",
							ComponentKind: "shell-path-prepend",
							ComponentRole: target.TargetScenarioComponentPlant,
							Summary:       "try reducing plant metadata",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      4,
		Fidelity:       TargetMinimizationFidelitySemantic,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if result.Fidelity != TargetMinimizationFidelitySemantic || len(result.Candidates) != 1 {
		t.Fatalf("expected one semantic minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedComponentReductions != 1 || !containsString(item.AcceptedSteps, "component-clear-plant-metadata:plant.required") {
		t.Fatalf("expected accepted plant metadata reduction: %#v", item)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if minimizedTask.Scenario.PlantPrimitiveID != "" {
		t.Fatalf("expected minimized plant primitive metadata to be empty: %#v", minimizedTask.Scenario)
	}
	for _, component := range minimizedTask.Scenario.Components {
		if component.ComponentID == "plant.required" {
			t.Fatalf("expected plant component to be removed from minimized artifact: %#v", minimizedTask.Scenario.Components)
		}
	}
}

func TestRunTargetMinimizationReducesCommandLines(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{LifecycleOperationID: "run-continue"}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "command-line-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with a removable command line.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
	}
	command := `printf 'unused helper\n' > unused-helper.txt
mkdir -p workspace-bin
printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git
chmod +x workspace-bin/git
printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       command,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "command-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{Order: 1, StepID: "m1", Kind: "prompt-reduction", Summary: "reduce prompt and command"},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      2,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedCommandReductions < 1 || !containsString(item.AcceptedSteps, "command-line-delete:1") {
		t.Fatalf("expected accepted command line reduction: %#v", item)
	}
	if item.MinimizedCommandLines >= item.OriginalCommandLines {
		t.Fatalf("expected command line count to shrink: %#v", item)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if strings.Contains(minimizedTask.Command, "unused-helper.txt") {
		t.Fatalf("expected minimized command to remove unused helper write: %q", minimizedTask.Command)
	}
}

func TestRunTargetMinimizationReducesMutationMetadata(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{LifecycleOperationID: "run-continue"}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "mutation-metadata-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible mutation metadata.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Mutations: []target.TargetScenarioMutation{
			{
				MutationID: "phase-shift.process-mode.single-process",
				Kind:       target.TargetScenarioMutationPhaseShift,
				Summary:    "metadata-only phase shift provenance",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "mutation-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:        1,
							StepID:       "m1",
							Kind:         "mutation-axis-check",
							MutationID:   "phase-shift.process-mode.single-process",
							MutationKind: target.TargetScenarioMutationPhaseShift,
							Summary:      "try reducing mutation provenance",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      2,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedMutationReductions != 1 || !containsString(item.AcceptedSteps, "mutation-delete:phase-shift.process-mode.single-process") {
		t.Fatalf("expected accepted mutation metadata reduction: %#v", item)
	}
	if item.MinimizedMutations >= item.OriginalMutations {
		t.Fatalf("expected mutation count to shrink: %#v", item)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if len(minimizedTask.Scenario.Mutations) != 0 {
		t.Fatalf("expected minimized artifact to remove mutation metadata: %#v", minimizedTask.Scenario.Mutations)
	}
}

func TestRunTargetMinimizationReducesActivationForkMessage(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{
		LifecycleOperationID: "checkpoint-fork",
		ForkFollowup:         true,
		ForkMessage:          "write the trusted response artifact\nalso explain the trusted response",
	}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "activation-command-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible activation message.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "activation.git-resolution",
				Role:        target.TargetScenarioComponentActivation,
				KindID:      "git-resolution",
				Summary:     "activation message can be shortened",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "activation-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:         1,
							StepID:        "m1",
							Kind:          "activation-minimization",
							ComponentID:   "activation.git-resolution",
							ComponentKind: "git-resolution",
							ComponentRole: target.TargetScenarioComponentActivation,
							Summary:       "try reducing activation fork message",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      2,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedActivationReductions != 1 || !containsString(item.AcceptedSteps, "activation-command-line-delete:1") {
		t.Fatalf("expected accepted activation command reduction: %#v", item)
	}
	if item.AcceptedComponentReductions != 1 || !containsString(item.AcceptedSteps, "component-clear-summary:activation.git-resolution") {
		t.Fatalf("expected accepted activation component summary reduction: %#v", item)
	}
	if item.MinimizedExecutionPlan == nil || item.MinimizedExecutionPlan.ForkMessage != "also explain the trusted response" {
		t.Fatalf("expected shortened fork message: %#v", item.MinimizedExecutionPlan)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if minimizedTask.Scenario.ExecutionPlan == nil || minimizedTask.Scenario.ExecutionPlan.ForkMessage != "also explain the trusted response" {
		t.Fatalf("expected minimized artifact to contain shortened fork message: %#v", minimizedTask.Scenario.ExecutionPlan)
	}
	for _, component := range minimizedTask.Scenario.Components {
		if component.ComponentID == "activation.git-resolution" && component.Summary != "" {
			t.Fatalf("expected minimized artifact to clear activation component summary: %#v", component)
		}
	}
}

func TestRunTargetMinimizationImpactFidelityAcceptsLifecycleMetadataReduction(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{
		LifecycleOperationID: "checkpoint-fork",
		ForkFollowup:         true,
		ForkMessage:          "check the trusted git resolution",
		CheckpointSelector:   "before-path-prepend",
		CheckpointBackend:    "disk",
		ProcessMode:          "split-process",
	}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "impact-lifecycle-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible lifecycle metadata.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		LifecycleEdge:    "checkpoint->fork",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "lifecycle.required",
				Role:        target.TargetScenarioComponentLifecycle,
				KindID:      "checkpoint-fork",
				Summary:     "lifecycle metadata can be removed in impact mode",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "impact-lifecycle-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:         1,
							StepID:        "m1",
							Kind:          "lifecycle-tightening",
							ComponentID:   "lifecycle.required",
							ComponentKind: "checkpoint-fork",
							ComponentRole: target.TargetScenarioComponentLifecycle,
							Summary:       "try reducing lifecycle metadata",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      1,
		Fidelity:       TargetMinimizationFidelityImpact,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if result.Fidelity != TargetMinimizationFidelityImpact || len(result.Candidates) != 1 {
		t.Fatalf("expected one impact minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedComponentReductions != 1 || !containsString(item.AcceptedSteps, "component-clear-lifecycle-metadata:lifecycle.required") {
		t.Fatalf("expected accepted lifecycle metadata reduction: %#v", item)
	}
	if item.MinimizedExecutionPlan == nil || item.MinimizedExecutionPlan.LifecycleOperationID != "" {
		t.Fatalf("expected minimized execution plan lifecycle metadata to be empty: %#v", item.MinimizedExecutionPlan)
	}
	if !item.MinimizedExecutionPlan.ForkFollowup || item.MinimizedExecutionPlan.ForkMessage != "check the trusted git resolution" {
		t.Fatalf("expected runtime fork activation fields to remain intact: %#v", item.MinimizedExecutionPlan)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if minimizedTask.Scenario.LifecycleEdge != "" || minimizedTask.Scenario.ExecutionPlan == nil || minimizedTask.Scenario.ExecutionPlan.LifecycleOperationID != "" {
		t.Fatalf("expected minimized lifecycle metadata to be empty: %#v", minimizedTask.Scenario)
	}
	for _, component := range minimizedTask.Scenario.Components {
		if component.Role == target.TargetScenarioComponentLifecycle || component.ComponentID == "lifecycle.required" {
			t.Fatalf("expected lifecycle component to be removed from minimized artifact: %#v", minimizedTask.Scenario.Components)
		}
	}
}

func TestRunTargetMinimizationImpactFidelityAcceptsOracleMetadataReduction(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{LifecycleOperationID: "run-continue"}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "impact-oracle-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible oracle metadata.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "oracle.required",
				Role:        target.TargetScenarioComponentOracle,
				KindID:      "persistent-shell-path",
				Summary:     "oracle metadata can be removed in impact mode",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "impact-oracle-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:         1,
							StepID:        "m1",
							Kind:          "oracle-preservation",
							ComponentID:   "oracle.required",
							ComponentKind: "persistent-shell-path",
							ComponentRole: target.TargetScenarioComponentOracle,
							Summary:       "try reducing oracle metadata",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      1,
		Fidelity:       TargetMinimizationFidelityImpact,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if result.Fidelity != TargetMinimizationFidelityImpact || len(result.Candidates) != 1 {
		t.Fatalf("expected one impact minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedComponentReductions != 1 || !containsString(item.AcceptedSteps, "component-clear-oracle-metadata:oracle.required") {
		t.Fatalf("expected accepted oracle metadata reduction: %#v", item)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if minimizedTask.Scenario.OracleKindID != "" || minimizedTask.Scenario.ActivationKindID != "git-resolution" {
		t.Fatalf("expected oracle metadata to be empty while impact remains intact: %#v", minimizedTask.Scenario)
	}
	for _, component := range minimizedTask.Scenario.Components {
		if component.Role == target.TargetScenarioComponentOracle || component.ComponentID == "oracle.required" {
			t.Fatalf("expected oracle component to be removed from minimized artifact: %#v", minimizedTask.Scenario.Components)
		}
	}
}

func TestRunTargetMinimizationImpactFidelityAcceptsActivationMetadataReduction(t *testing.T) {
	tmp := t.TempDir()
	plan := &target.TargetScenarioExecutionPlan{LifecycleOperationID: "run-continue"}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "impact-activation-reduction",
		TaskID:           target.PersistentShellTargetTaskID,
		Description:      "Persistent shell minimizer fixture with reducible activation metadata.",
		Objective:        "Confirm that a workspace-local git shim is still observed.",
		PlantPrimitiveID: "shell-path-prepend",
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		ExecutionPlan:    plan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "activation.required",
				Role:        target.TargetScenarioComponentActivation,
				KindID:      "git-resolution",
			},
		},
	}
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:        filepath.Join(tmp, "baseline-runs"),
		TargetID:      "minimizer-target",
		TaskID:        target.PersistentShellTargetTaskID,
		Scenario:      scenario,
		ExecutionPlan: plan,
		Prompt:        "plant path state",
		Command:       `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget baseline failed: %v", err)
	}
	if !baseline.TargetOracle.Confirmed {
		t.Fatalf("expected confirmed baseline: %#v", baseline.TargetOracle)
	}

	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "impact-activation-minimizer-suite",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:            "minimizer-target/persistent-shell-poisoning",
				RunID:                  baseline.RunID,
				TargetID:               baseline.TargetID,
				TaskID:                 baseline.TaskID,
				Confirmed:              true,
				OutcomeCategory:        corpus.TargetObservationResidueObserved,
				ActivationStage:        TargetActivationStageActivationReached,
				TargetOracle:           baseline.TargetOracle,
				TaskCompliance:         baseline.TaskCompliance,
				ContractInterpretation: baseline.ContractInterpretation,
				Signature:              baseline.Signature,
				ArtifactDir:            baseline.ArtifactDir,
				MinimizationPlan: &TargetMinimizationPlan{
					SchemaVersion: "syncfuzz.target-minimization-plan.v1",
					Applicable:    true,
					Steps: []TargetMinimizationStep{
						{
							Order:         1,
							StepID:        "m1",
							Kind:          "activation-minimization",
							ComponentID:   "activation.required",
							ComponentKind: "git-resolution",
							ComponentRole: target.TargetScenarioComponentActivation,
							Summary:       "try reducing activation metadata",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}

	result, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath:     sourcePath,
		OutDir:         filepath.Join(tmp, "minimized-runs"),
		CandidateLimit: 1,
		MaxTrials:      1,
		Fidelity:       TargetMinimizationFidelityImpact,
	})
	if err != nil {
		t.Fatalf("RunTargetMinimization failed: %v", err)
	}
	if result.Fidelity != TargetMinimizationFidelityImpact || len(result.Candidates) != 1 {
		t.Fatalf("expected one impact minimization candidate: %#v", result)
	}
	item := result.Candidates[0]
	if !item.Preserved || item.AcceptedComponentReductions != 1 || !containsString(item.AcceptedSteps, "component-clear-activation-metadata:activation.required") {
		t.Fatalf("expected accepted activation metadata reduction: %#v", item)
	}
	minimizedTask := readMinimizedTargetTaskForTest(t, item.MinimizedArtifactDir)
	if minimizedTask.Scenario.ActivationKindID != "" || minimizedTask.Scenario.OracleKindID != "persistent-shell-path" {
		t.Fatalf("expected activation metadata to be empty while oracle metadata remains intact: %#v", minimizedTask.Scenario)
	}
	for _, component := range minimizedTask.Scenario.Components {
		if component.Role == target.TargetScenarioComponentActivation || component.ComponentID == "activation.required" {
			t.Fatalf("expected activation component to be removed from minimized artifact: %#v", minimizedTask.Scenario.Components)
		}
	}
}

func TestTargetActivationCommandReducersDeleteForkMessageLines(t *testing.T) {
	plan := &target.TargetScenarioExecutionPlan{
		ForkFollowup: true,
		ForkMessage:  "first activation line\nsecond activation line",
	}
	reducers := targetActivationCommandReducers(TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "activation-minimization", ComponentID: "activation"},
		},
	}, plan)
	if len(reducers) != 2 {
		t.Fatalf("expected one reducer per activation line, got %#v", reducers)
	}
	trial := cloneTargetExecutionPlan(plan)
	if !reducers[0].apply(trial) {
		t.Fatal("expected activation line reducer to apply")
	}
	if trial.ForkMessage != "second activation line" {
		t.Fatalf("unexpected reduced fork message: %#v", trial)
	}
	if reducers[0].apply(trial) {
		t.Fatal("expected reducer to stop before deleting the final activation line")
	}
}

func TestTargetScenarioMutationReducersDeleteMutationMetadata(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion: target.TargetScenarioSchemaVersion,
		ScenarioID:    "mutation-reducer",
		TaskID:        target.PersistentShellTargetTaskID,
		Mutations: []target.TargetScenarioMutation{
			{
				MutationID: "phase-shift.process-mode.single-process",
				Kind:       target.TargetScenarioMutationPhaseShift,
			},
			{
				MutationID: "activation.trusted-action",
				Kind:       target.TargetScenarioMutationActivationSubstitution,
			},
		},
	}
	reducers := targetScenarioMutationReducers(TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{
				Kind:         "mutation-axis-check",
				MutationID:   "phase-shift.process-mode.single-process",
				MutationKind: target.TargetScenarioMutationPhaseShift,
			},
		},
	}, scenario)
	if len(reducers) != 1 || reducers[0].stepID != "mutation-delete:phase-shift.process-mode.single-process" {
		t.Fatalf("expected one mutation reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial) {
		t.Fatal("expected mutation reducer to apply")
	}
	if len(trial.Mutations) != 1 || trial.Mutations[0].MutationID != "activation.trusted-action" {
		t.Fatalf("expected only selected mutation metadata to be removed: %#v", trial.Mutations)
	}
}

func TestTargetScenarioComponentReducersClearRequiredComponentSummary(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion: target.TargetScenarioSchemaVersion,
		ScenarioID:    "component-summary-reducer",
		TaskID:        target.PersistentShellTargetTaskID,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "activation.required",
				Role:        target.TargetScenarioComponentActivation,
				KindID:      "git-resolution",
				Summary:     "activation summary metadata",
			},
		},
	}
	reducers := targetScenarioComponentReducers(TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "activation-minimization", ComponentID: "activation.required"},
		},
	}, scenario, TargetMinimizationFidelityExact)
	if len(reducers) != 1 || reducers[0].stepID != "component-clear-summary:activation.required" {
		t.Fatalf("expected required activation summary reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial, nil) {
		t.Fatal("expected component summary reducer to apply")
	}
	if len(trial.Components) != 1 || trial.Components[0].Summary != "" || trial.Components[0].KindID != "git-resolution" {
		t.Fatalf("expected only component summary to be cleared: %#v", trial.Components)
	}
}

func TestTargetScenarioComponentReducersAllowMultipleReducersForOneComponent(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion: target.TargetScenarioSchemaVersion,
		ScenarioID:    "multi-stage-component-reducer",
		TaskID:        target.PersistentShellTargetTaskID,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "setup.optional",
				Role:        target.TargetScenarioComponentSetup,
				KindID:      "optional-helper",
				Summary:     "optional helper summary",
			},
		},
	}
	reducers := targetScenarioComponentReducers(TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "component-deletion", ComponentID: "setup.optional"},
		},
	}, scenario, TargetMinimizationFidelityExact)
	if len(reducers) != 2 || reducers[0].stepID != "component-delete:setup.optional" || reducers[1].stepID != "component-clear-summary:setup.optional" {
		t.Fatalf("expected deletion and summary reducers for one component, got %#v", reducers)
	}
	deleteTrial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(deleteTrial, nil) || len(deleteTrial.Components) != 0 {
		t.Fatalf("expected first reducer to delete component: %#v", deleteTrial.Components)
	}
	summaryTrial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[1].apply(summaryTrial, nil) {
		t.Fatal("expected second reducer to clear summary")
	}
	if len(summaryTrial.Components) != 1 || summaryTrial.Components[0].Summary != "" {
		t.Fatalf("expected summary reducer to preserve component identity: %#v", summaryTrial.Components)
	}
}

func TestTargetScenarioComponentReducersSkipRequiredComponents(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion: target.TargetScenarioSchemaVersion,
		ScenarioID:    "component-reducer",
		TaskID:        target.PersistentShellTargetTaskID,
		Components: []target.TargetScenarioComponent{
			{ComponentID: "setup.optional", Role: target.TargetScenarioComponentSetup, KindID: "optional"},
			{ComponentID: "plant.required", Role: target.TargetScenarioComponentPlant, KindID: "shell-path-prepend"},
		},
	}
	reducers := targetScenarioComponentReducers(TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "component-deletion", ComponentID: "setup.optional"},
			{Kind: "component-deletion", ComponentID: "plant.required"},
		},
	}, scenario, TargetMinimizationFidelityExact)
	if len(reducers) != 1 || reducers[0].stepID != "component-delete:setup.optional" {
		t.Fatalf("expected only optional setup deletion reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial, nil) {
		t.Fatal("expected setup component deletion to apply")
	}
	for _, component := range trial.Components {
		if component.ComponentID == "setup.optional" {
			t.Fatalf("expected setup component to be deleted: %#v", trial.Components)
		}
		if component.ComponentID == "plant.required" && component.Role != target.TargetScenarioComponentPlant {
			t.Fatalf("expected required plant component to remain intact: %#v", trial.Components)
		}
	}
	if len(trial.Components) != 1 || trial.Components[0].ComponentID != "plant.required" {
		t.Fatalf("expected required plant component to remain: %#v", trial.Components)
	}
}

func TestTargetScenarioComponentReducersClearPlantMetadataInSemanticMode(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "plant-metadata-reducer",
		TaskID:           target.PersistentShellTargetTaskID,
		PlantPrimitiveID: "shell-path-prepend",
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "plant.required",
				Role:        target.TargetScenarioComponentPlant,
				KindID:      "shell-path-prepend",
			},
		},
	}
	plan := TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "primitive-minimization", ComponentID: "plant.required"},
		},
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityExact); len(reducers) != 0 {
		t.Fatalf("expected exact fidelity to skip required plant metadata reducers, got %#v", reducers)
	}
	reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelitySemantic)
	if len(reducers) != 1 || reducers[0].stepID != "component-clear-plant-metadata:plant.required" {
		t.Fatalf("expected semantic plant metadata reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial, nil) {
		t.Fatal("expected plant metadata reducer to apply")
	}
	if trial.PlantPrimitiveID != "" {
		t.Fatalf("expected plant primitive metadata to be cleared: %#v", trial)
	}
	if len(trial.Components) != 0 {
		t.Fatalf("expected plant component to be removed: %#v", trial.Components)
	}
}

func TestTargetScenarioComponentReducersClearLifecycleMetadataInImpactMode(t *testing.T) {
	executionPlan := &target.TargetScenarioExecutionPlan{
		LifecycleOperationID: "checkpoint-fork",
		ForkFollowup:         true,
		ForkMessage:          "trusted follow-up command",
		CheckpointSelector:   "before-plant",
		CheckpointBackend:    "disk",
		ProcessMode:          "split-process",
	}
	scenario := &target.TargetScenarioInfo{
		SchemaVersion: target.TargetScenarioSchemaVersion,
		ScenarioID:    "lifecycle-metadata-reducer",
		TaskID:        target.PersistentShellTargetTaskID,
		LifecycleEdge: "checkpoint->fork",
		ExecutionPlan: executionPlan,
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "lifecycle.required",
				Role:        target.TargetScenarioComponentLifecycle,
				KindID:      "checkpoint-fork",
			},
		},
	}
	plan := TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "lifecycle-tightening", ComponentID: "lifecycle.required"},
		},
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityExact); len(reducers) != 0 {
		t.Fatalf("expected exact fidelity to skip lifecycle metadata reducers, got %#v", reducers)
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelitySemantic); len(reducers) != 0 {
		t.Fatalf("expected semantic fidelity to skip lifecycle metadata reducers, got %#v", reducers)
	}
	reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityImpact)
	if len(reducers) != 1 || reducers[0].stepID != "component-clear-lifecycle-metadata:lifecycle.required" {
		t.Fatalf("expected impact lifecycle metadata reducer, got %#v", reducers)
	}
	trialScenario := target.CloneTargetScenarioInfo(scenario)
	trialPlan := cloneTargetExecutionPlan(executionPlan)
	if !reducers[0].apply(trialScenario, trialPlan) {
		t.Fatal("expected lifecycle metadata reducer to apply")
	}
	if trialScenario.LifecycleEdge != "" || trialScenario.ExecutionPlan.LifecycleOperationID != "" || trialPlan.LifecycleOperationID != "" {
		t.Fatalf("expected lifecycle metadata to be cleared: scenario=%#v plan=%#v", trialScenario, trialPlan)
	}
	if !trialPlan.ForkFollowup || trialPlan.ForkMessage != "trusted follow-up command" || trialPlan.CheckpointSelector != "before-plant" {
		t.Fatalf("expected runtime execution fields to remain intact: %#v", trialPlan)
	}
	if len(trialScenario.Components) != 0 {
		t.Fatalf("expected lifecycle component to be removed: %#v", trialScenario.Components)
	}
}

func TestTargetScenarioComponentReducersClearActivationMetadataInImpactMode(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "activation-metadata-reducer",
		TaskID:           target.PersistentShellTargetTaskID,
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "activation.required",
				Role:        target.TargetScenarioComponentActivation,
				KindID:      "git-resolution",
			},
		},
	}
	plan := TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "activation-minimization", ComponentID: "activation.required"},
		},
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityExact); len(reducers) != 0 {
		t.Fatalf("expected exact fidelity to skip activation metadata reducers, got %#v", reducers)
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelitySemantic); len(reducers) != 0 {
		t.Fatalf("expected semantic fidelity to skip activation metadata reducers, got %#v", reducers)
	}
	reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityImpact)
	if len(reducers) != 1 || reducers[0].stepID != "component-clear-activation-metadata:activation.required" {
		t.Fatalf("expected impact activation metadata reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial, nil) {
		t.Fatal("expected activation metadata reducer to apply")
	}
	if trial.ActivationKindID != "" || trial.OracleKindID != "persistent-shell-path" {
		t.Fatalf("expected only activation metadata to be cleared: %#v", trial)
	}
	if len(trial.Components) != 0 {
		t.Fatalf("expected activation component to be removed: %#v", trial.Components)
	}
}

func TestTargetScenarioComponentReducersClearOracleMetadataInImpactMode(t *testing.T) {
	scenario := &target.TargetScenarioInfo{
		SchemaVersion:    target.TargetScenarioSchemaVersion,
		ScenarioID:       "oracle-metadata-reducer",
		TaskID:           target.PersistentShellTargetTaskID,
		ActivationKindID: "git-resolution",
		OracleKindID:     "persistent-shell-path",
		Components: []target.TargetScenarioComponent{
			{
				ComponentID: "oracle.required",
				Role:        target.TargetScenarioComponentOracle,
				KindID:      "persistent-shell-path",
			},
		},
	}
	plan := TargetMinimizationPlan{
		Steps: []TargetMinimizationStep{
			{Kind: "oracle-preservation", ComponentID: "oracle.required"},
		},
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityExact); len(reducers) != 0 {
		t.Fatalf("expected exact fidelity to skip oracle metadata reducers, got %#v", reducers)
	}
	if reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelitySemantic); len(reducers) != 0 {
		t.Fatalf("expected semantic fidelity to skip oracle metadata reducers, got %#v", reducers)
	}
	reducers := targetScenarioComponentReducers(plan, scenario, TargetMinimizationFidelityImpact)
	if len(reducers) != 1 || reducers[0].stepID != "component-clear-oracle-metadata:oracle.required" {
		t.Fatalf("expected impact oracle metadata reducer, got %#v", reducers)
	}
	trial := target.CloneTargetScenarioInfo(scenario)
	if !reducers[0].apply(trial, nil) {
		t.Fatal("expected oracle metadata reducer to apply")
	}
	if trial.OracleKindID != "" || trial.ActivationKindID != "git-resolution" {
		t.Fatalf("expected only oracle metadata to be cleared: %#v", trial)
	}
	if len(trial.Components) != 0 {
		t.Fatalf("expected oracle component to be removed: %#v", trial.Components)
	}
}

func TestRunTargetMinimizationRequiresSourcePath(t *testing.T) {
	if _, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{}); err == nil {
		t.Fatal("expected missing source path error")
	}
}

func TestRunTargetMinimizationRejectsUnknownFidelity(t *testing.T) {
	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "target-suite-result.json")
	if err := core.WriteJSON(sourcePath, TargetSuiteResult{SchemaVersion: "syncfuzz.target-suite-result.v1"}); err != nil {
		t.Fatalf("write minimization source: %v", err)
	}
	_, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{
		SourcePath: sourcePath,
		OutDir:     filepath.Join(tmp, "runs"),
		Fidelity:   TargetMinimizationFidelity("loose"),
	})
	if err == nil {
		t.Fatal("expected unknown fidelity to be rejected")
	}
}

func TestTargetPromptReductionLinesPreservesLineContent(t *testing.T) {
	lines := targetPromptReductionLines("\n  keep indentation\n`quoted command`  \n\n")
	if len(lines) != 2 || lines[0] != "  keep indentation" || lines[1] != "`quoted command`  " {
		t.Fatalf("unexpected prompt line normalization: %#v", lines)
	}
}

func TestTargetMinimizationPreservedRejectsOracleAndComplianceDrift(t *testing.T) {
	signature := core.MismatchSignature{LifecycleEvent: "fork", Impact: "communication-residue"}
	source := TargetSuiteRunResult{
		Confirmed:      true,
		TargetOracle:   target.TargetOracleResult{Status: target.TargetOracleStatusConfirmed, Attribution: target.TargetOracleAttributionRuntimeResidue},
		TaskCompliance: target.TargetTaskComplianceResult{Status: target.TargetTaskComplianceStatusCompliant},
		Signature:      signature,
	}
	matching := &target.TargetRunResult{
		Completed:      true,
		TargetOracle:   source.TargetOracle,
		TaskCompliance: source.TaskCompliance,
		Signature:      signature,
	}
	if !targetMinimizationPreserved(source, matching, TargetMinimizationFidelityExact) {
		t.Fatal("expected matching trial to preserve source constraints")
	}

	oracleDrift := *matching
	oracleDrift.TargetOracle.Status = target.TargetOracleStatusNegative
	if targetMinimizationPreserved(source, &oracleDrift, TargetMinimizationFidelityExact) {
		t.Fatal("expected oracle status drift to reject reduction")
	}
	complianceDrift := *matching
	complianceDrift.TaskCompliance.Status = target.TargetTaskComplianceStatusViolated
	if targetMinimizationPreserved(source, &complianceDrift, TargetMinimizationFidelityExact) {
		t.Fatal("expected compliance drift to reject reduction")
	}
	signatureDrift := *matching
	signatureDrift.Signature.Impact = "different-impact"
	if targetMinimizationPreserved(source, &signatureDrift, TargetMinimizationFidelityExact) {
		t.Fatal("expected signature drift to reject reduction")
	}
}

func TestTargetMinimizationPreservedSupportsFidelityModes(t *testing.T) {
	source := TargetSuiteRunResult{
		Confirmed: true,
		TargetOracle: target.TargetOracleResult{
			Status:      target.TargetOracleStatusConfirmed,
			Attribution: target.TargetOracleAttributionRuntimeResidue,
		},
		TaskCompliance: target.TargetTaskComplianceResult{Status: target.TargetTaskComplianceStatusCompliant},
		ContractInterpretation: &target.TargetContractInterpretation{
			Status: target.TargetContractStatusViolation,
		},
		Signature: core.MismatchSignature{
			LifecycleEvent: "checkpoint-fork",
			FaultPhase:     "after-checkpoint",
			StateClass:     "socket",
			Operation:      "workspace-unix-listener",
			Relation:       "trusted-action-execution",
			Impact:         "trusted-action-effect",
		},
	}
	trial := &target.TargetRunResult{
		Completed: true,
		TargetOracle: target.TargetOracleResult{
			Status:      target.TargetOracleStatusConfirmed,
			Attribution: target.TargetOracleAttributionWorkspaceRebuild,
		},
		TaskCompliance: source.TaskCompliance,
		ContractInterpretation: &target.TargetContractInterpretation{
			Status: target.TargetContractStatusViolation,
		},
		Signature: core.MismatchSignature{
			LifecycleEvent: "checkpoint-fork",
			FaultPhase:     "after-checkpoint",
			StateClass:     "socket",
			Operation:      "smaller-unix-listener-plant",
			Relation:       "trusted-action-execution",
			Impact:         "trusted-action-effect",
		},
	}
	if targetMinimizationPreserved(source, trial, TargetMinimizationFidelityExact) {
		t.Fatal("expected exact fidelity to reject attribution and operation drift")
	}
	if !targetMinimizationPreserved(source, trial, TargetMinimizationFidelitySemantic) {
		t.Fatal("expected semantic fidelity to allow attribution and operation drift")
	}

	semanticDrift := *trial
	semanticDrift.Signature.Relation = "different-relation"
	if targetMinimizationPreserved(source, &semanticDrift, TargetMinimizationFidelitySemantic) {
		t.Fatal("expected semantic fidelity to reject mismatch relation drift")
	}

	impactOnly := *trial
	impactOnly.Signature.Relation = "different-relation"
	impactOnly.ContractInterpretation = &target.TargetContractInterpretation{Status: target.TargetContractStatusConsistent}
	impactOnly.TaskCompliance.Status = target.TargetTaskComplianceStatusViolated
	if !targetMinimizationPreserved(source, &impactOnly, TargetMinimizationFidelityImpact) {
		t.Fatal("expected impact fidelity to preserve only oracle status and impact")
	}
	impactDrift := impactOnly
	impactDrift.Signature.Impact = "different-impact"
	if targetMinimizationPreserved(source, &impactDrift, TargetMinimizationFidelityImpact) {
		t.Fatal("expected impact fidelity to reject impact drift")
	}
	sourceWithOracle := source
	sourceWithOracle.TargetOracle.Name = "trusted-action"
	impactMetadataDrop := *trial
	impactMetadataDrop.Signature.Impact = ""
	impactMetadataDrop.TargetOracle.Name = "trusted-action"
	if !targetMinimizationPreserved(sourceWithOracle, &impactMetadataDrop, TargetMinimizationFidelityImpact) {
		t.Fatal("expected impact fidelity to allow empty impact when oracle name is preserved")
	}
	impactWrongOracle := impactMetadataDrop
	impactWrongOracle.TargetOracle.Name = "different-oracle"
	if targetMinimizationPreserved(sourceWithOracle, &impactWrongOracle, TargetMinimizationFidelityImpact) {
		t.Fatal("expected impact fidelity to reject empty impact when oracle name drifts")
	}
}

func TestRunTargetPromptMinimizationReportsExhaustedTrialErrors(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "source-run")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create source artifact directory: %v", err)
	}
	if err := core.WriteJSON(filepath.Join(artifactDir, target.TargetTaskArtifact), target.TargetTask{
		SchemaVersion: "syncfuzz.target-task.v1",
		AdapterID:     "unsupported-adapter",
		TargetID:      "minimizer-error-target",
		TaskID:        target.DefaultTargetTaskID,
		Prompt:        "line one\nline two",
		Command:       "true",
		Environment:   "local",
	}); err != nil {
		t.Fatalf("write source target task: %v", err)
	}

	item := runTargetPromptMinimization(context.Background(), filepath.Join(tmp, "runs"), TargetSuiteRunResult{
		RunID:       "source-run",
		TargetID:    "minimizer-error-target",
		TaskID:      target.DefaultTargetTaskID,
		Confirmed:   true,
		ArtifactDir: artifactDir,
		TargetOracle: target.TargetOracleResult{
			Status: target.TargetOracleStatusConfirmed,
		},
	}, 1, TargetMinimizationFidelityExact)
	if item.Preserved || item.Error == "" {
		t.Fatalf("expected exhausted execution errors to fail minimization: %#v", item)
	}
}

func TestRunTargetPromptMinimizationReportsCommandFailures(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "source-run")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create source artifact directory: %v", err)
	}
	if err := core.WriteJSON(filepath.Join(artifactDir, target.TargetTaskArtifact), target.TargetTask{
		SchemaVersion: "syncfuzz.target-task.v1",
		AdapterID:     target.DefaultTargetAdapterID,
		TargetID:      "minimizer-command-error-target",
		TaskID:        target.DefaultTargetTaskID,
		Prompt:        "line one\nline two",
		Command:       "exit 7",
		Environment:   "local",
	}); err != nil {
		t.Fatalf("write source target task: %v", err)
	}

	item := runTargetPromptMinimization(context.Background(), filepath.Join(tmp, "runs"), TargetSuiteRunResult{
		RunID:       "source-run",
		TargetID:    "minimizer-command-error-target",
		TaskID:      target.DefaultTargetTaskID,
		Confirmed:   true,
		ArtifactDir: artifactDir,
		TargetOracle: target.TargetOracleResult{
			Status: target.TargetOracleStatusConfirmed,
		},
	}, 1, TargetMinimizationFidelityExact)
	if item.Preserved || item.Error == "" {
		t.Fatalf("expected command failures to fail minimization: %#v", item)
	}
}
