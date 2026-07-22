package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestTargetScheduleMatrixPropagatesScenarioViolationSignature(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.UnixListenerResidueForkTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.Candidates) == 0 {
		t.Fatal("expected schedule candidates")
	}
	for _, candidate := range matrix.Candidates {
		if candidate.TaskID != target.UnixListenerResidueForkTargetTaskID {
			continue
		}
		if candidate.ViolationSignature.SchemaVersion != target.TargetViolationSignatureSchemaVersion || candidate.ViolationSignature.LifecycleBoundary != target.TargetViolationBoundaryCheckpointFork || !schedulerTargetViolationContains(candidate.ViolationSignature.ResourceClasses, target.TargetViolationIPCEndpoint) {
			t.Fatalf("candidate did not preserve scenario violation signature: %#v", candidate)
		}
		return
	}
	t.Fatal("expected fork listener candidate")
}

func TestTargetViolationSignatureDimensionsContributeCoverage(t *testing.T) {
	signature, err := target.NormalizeTargetViolationSignature(target.TargetViolationSignature{
		Relations:             []target.TargetViolationRelation{target.TargetViolationResidual, target.TargetViolationRebound},
		ResourceClasses:       []target.TargetViolationResourceClass{target.TargetViolationIPCEndpoint},
		LifecycleBoundary:     target.TargetViolationBoundaryCheckpointFork,
		PersistenceMechanisms: []target.TargetViolationPersistenceMechanism{target.TargetViolationDescendantSurvival, target.TargetViolationSharedRuntime},
		Consequences:          []target.TargetViolationConsequence{target.TargetViolationCrossBranchInterference, target.TargetViolationServiceImpersonation},
	})
	if err != nil {
		t.Fatalf("normalize signature: %v", err)
	}
	candidate := TargetScheduleCandidate{
		CandidateID:        "target/socket-fork",
		TaskID:             "socket-fork",
		ViolationSignature: signature,
	}
	summaries := summarizeTargetDimensionCoverage([]TargetScheduleCandidate{candidate}, []TargetSuiteRunResult{{
		CandidateID:        candidate.CandidateID,
		TaskID:             candidate.TaskID,
		Confirmed:          true,
		ActivationStage:    TargetActivationStageActivationReached,
		ViolationSignature: signature,
	}})
	coverage := make(map[string]TargetDimensionCoverageSummary, len(summaries))
	for _, summary := range summaries {
		coverage[summary.Dimension] = summary
	}
	for _, dimension := range []string{
		"violation_relation",
		"violation_resource_class",
		"violation_lifecycle_boundary",
		"violation_persistence_mechanism",
		"violation_consequence",
		"violation_signature_id",
	} {
		summary, ok := coverage[dimension]
		if !ok || summary.ExecutedValues != summary.TotalValues || summary.ConfirmedValues != summary.TotalValues || summary.ActivationReachedValues != summary.TotalValues {
			t.Fatalf("unexpected %s coverage: %#v", dimension, summary)
		}
	}
}

func schedulerTargetViolationContains[T comparable](values []T, expected T) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
