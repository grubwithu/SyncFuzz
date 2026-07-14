package scheduler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestBuildTargetMinimizationBatchFromSuiteResult(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target-suite-result.json")
	plan := &TargetMinimizationPlan{
		SchemaVersion: "syncfuzz.target-minimization-plan.v1",
		Applicable:    true,
		Strategy:      "test",
		Preserve:      []string{"artifact=" + target.TargetShellPoisonCheckArtifact},
		Steps: []TargetMinimizationStep{{
			Order:   1,
			StepID:  "m1",
			Kind:    "artifact-replay-check",
			Summary: "rerun",
		}},
	}
	if err := core.WriteJSON(source, TargetSuiteResult{
		SchemaVersion: "syncfuzz.target-suite-result.v1",
		SuiteID:       "suite-test",
		Results: []TargetSuiteRunResult{
			{
				CandidateID:      "target/persistent-shell-poisoning",
				RunID:            "run-confirmed",
				TargetID:         "target",
				TaskID:           target.PersistentShellTargetTaskID,
				PromptProfileID:  target.TargetPromptProfileBaselineID,
				Confirmed:        true,
				OutcomeCategory:  corpus.TargetObservationResidueObserved,
				ActivationStage:  TargetActivationStageActivationReached,
				TargetOracle:     target.TargetOracleResult{Attribution: target.TargetOracleAttributionRuntimeResidue},
				ArtifactDir:      filepath.Join(tmp, "run-confirmed"),
				MinimizationPlan: plan,
			},
			{
				CandidateID:     "target/orphan-process",
				RunID:           "run-unconfirmed",
				TargetID:        "target",
				TaskID:          target.DefaultTargetTaskID,
				Confirmed:       false,
				OutcomeCategory: corpus.TargetObservationExecutionNotReached,
				TargetOracle:    target.TargetOracleResult{},
			},
		},
	}); err != nil {
		t.Fatalf("write source suite result: %v", err)
	}

	batch, err := BuildTargetMinimizationBatch(TargetMinimizationBatchOptions{
		SourcePath: source,
		OutDir:     filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("BuildTargetMinimizationBatch failed: %v", err)
	}
	if batch.SourceSchemaVersion != "syncfuzz.target-suite-result.v1" {
		t.Fatalf("unexpected source schema: %#v", batch)
	}
	if batch.TotalResults != 2 || batch.ApplicablePlans != 1 || batch.SkippedPlans != 1 {
		t.Fatalf("unexpected batch counts: %#v", batch)
	}
	if len(batch.Plans) != 2 || !batch.Plans[0].MinimizationPlan.Applicable || batch.Plans[1].MinimizationPlan.Applicable {
		t.Fatalf("unexpected batch plans: %#v", batch.Plans)
	}
	if _, err := os.Stat(filepath.Join(batch.ArtifactDir, targetMinimizationBatchArtifact)); err != nil {
		t.Fatalf("expected minimization batch artifact: %v", err)
	}
}

func TestBuildTargetMinimizationBatchRejectsUnsupportedSource(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "unknown.json")
	if err := core.WriteJSON(source, map[string]string{"schema_version": "syncfuzz.unknown.v1"}); err != nil {
		t.Fatalf("write unknown source: %v", err)
	}
	if _, err := BuildTargetMinimizationBatch(TargetMinimizationBatchOptions{
		SourcePath: source,
		OutDir:     filepath.Join(tmp, "runs"),
	}); err == nil {
		t.Fatalf("expected unsupported source error")
	}
}
