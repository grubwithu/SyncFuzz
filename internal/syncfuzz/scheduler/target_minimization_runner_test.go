package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestRunTargetMinimizationReducesPromptAndPreservesOracle(t *testing.T) {
	tmp := t.TempDir()
	baseline, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:   filepath.Join(tmp, "baseline-runs"),
		TargetID: "minimizer-target",
		TaskID:   target.PersistentShellTargetTaskID,
		ExecutionPlan: &target.TargetScenarioExecutionPlan{
			LifecycleOperationID: "run-continue",
			CheckpointBackend:    "disk",
			ProcessMode:          "single",
		},
		Prompt:       "prepare workspace\nplant path state\nactivate later\npreserve witness",
		Command:      `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
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
	if item.MinimizedExecutionPlan.ProcessMode != "" || item.MinimizedExecutionPlan.CheckpointBackend != "" {
		t.Fatalf("expected minimized execution plan defaults: %#v", item.MinimizedExecutionPlan)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetMinimizationResultArtifact)); err != nil {
		t.Fatalf("expected minimization result artifact: %v", err)
	}
}

func TestRunTargetMinimizationRequiresSourcePath(t *testing.T) {
	if _, err := RunTargetMinimization(context.Background(), TargetMinimizationRunOptions{}); err == nil {
		t.Fatal("expected missing source path error")
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
	if !targetMinimizationPreserved(source, matching) {
		t.Fatal("expected matching trial to preserve source constraints")
	}

	oracleDrift := *matching
	oracleDrift.TargetOracle.Status = target.TargetOracleStatusNegative
	if targetMinimizationPreserved(source, &oracleDrift) {
		t.Fatal("expected oracle status drift to reject reduction")
	}
	complianceDrift := *matching
	complianceDrift.TaskCompliance.Status = target.TargetTaskComplianceStatusViolated
	if targetMinimizationPreserved(source, &complianceDrift) {
		t.Fatal("expected compliance drift to reject reduction")
	}
	signatureDrift := *matching
	signatureDrift.Signature.Impact = "different-impact"
	if targetMinimizationPreserved(source, &signatureDrift) {
		t.Fatal("expected signature drift to reject reduction")
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
	}, 1)
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
	}, 1)
	if item.Preserved || item.Error == "" {
		t.Fatalf("expected command failures to fail minimization: %#v", item)
	}
}
