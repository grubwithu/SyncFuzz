package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestSummarizeTargetCoverageFrontierPrefersGapFillingCandidates(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-a", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID)
				item.ScenarioID = "scenario-a"
				item.Mutations = []target.TargetScenarioMutation{{MutationID: "mutation-a"}}
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-b", "seed-b", "primitive-b", target.TargetPromptProfileBaselineID)
				item.ScenarioID = "scenario-b"
				item.Mutations = []target.TargetScenarioMutation{{MutationID: "mutation-b"}}
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-z", "seed-z", "primitive-z", target.TargetPromptProfileBaselineID)
				item.ScenarioID = "scenario-z"
				item.LifecycleOperationID = "checkpoint-replay"
				item.ActivationKindID = "socket-connect"
				item.OracleKindID = "unix-listener-residue"
				item.Mutations = []target.TargetScenarioMutation{{MutationID: "mutation-z"}}
				return item
			}(),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, []TargetSuiteRunResult{
		{
			CandidateID:      matrix.Candidates[0].CandidateID,
			Confirmed:        true,
			ActivationStage:  TargetActivationStageActivationReached,
			TargetOracle:     target.TargetOracleResult{Status: target.TargetOracleStatusConfirmed},
			TaskCompliance:   target.TargetTaskComplianceResult{Status: target.TargetTaskComplianceStatusCompliant},
			PromptProfileID:  target.TargetPromptProfileBaselineID,
			PlantPrimitiveID: "primitive-a",
		},
	}, nil, 2)
	if len(frontier) != 2 {
		t.Fatalf("expected 2 frontier candidates, got %#v", frontier)
	}
	if frontier[0].TaskID != "task-z" {
		t.Fatalf("expected strongest gap-filling candidate first, got %#v", frontier[0])
	}
	if frontier[0].SelectionMode != targetFrontierSelectionCoverageGap {
		t.Fatalf("expected coverage-gap selection mode, got %#v", frontier[0])
	}
	if frontier[0].GapScore <= frontier[1].GapScore {
		t.Fatalf("expected first frontier candidate to cover more gaps: first=%#v second=%#v", frontier[0], frontier[1])
	}
	if !target.ContainsString(frontier[0].CoveredGaps, "task_id=task-z") ||
		!target.ContainsString(frontier[0].CoveredGaps, "seed_id=seed-z") ||
		!target.ContainsString(frontier[0].CoveredGaps, "mutation_focus_id=mutation-z") {
		t.Fatalf("expected frontier candidate to explain covered gaps: %#v", frontier[0])
	}
}

func TestSummarizeTargetCoverageFrontierHonorsExcludedCandidates(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScenarioCandidate("task-a", "seed-a", "primitive-a"),
			testTargetScenarioCandidate("task-b", "seed-b", "primitive-b"),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, nil, []string{matrix.Candidates[0].CandidateID}, 5)
	if len(frontier) != 1 {
		t.Fatalf("expected only one frontier candidate after exclusion, got %#v", frontier)
	}
	if frontier[0].CandidateID != matrix.Candidates[1].CandidateID {
		t.Fatalf("expected excluded candidate to disappear from frontier, got %#v", frontier)
	}
}

func TestSummarizeTargetCoverageFrontierUsesPromptRepairForUnactivatedTask(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, []TargetSuiteRunResult{
		{
			CandidateID:     matrix.Candidates[0].CandidateID,
			TaskID:          "task-a",
			PromptProfileID: target.TargetPromptProfileBaselineID,
			OutcomeCategory: corpus.TargetObservationExecutionNotReached,
			ActivationStage: TargetActivationStagePreActivation,
		},
	}, nil, 2)
	if len(frontier) != 2 {
		t.Fatalf("expected 2 frontier candidates, got %#v", frontier)
	}
	if frontier[0].TaskID != "task-a" || frontier[0].PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected prompt-repair frontier candidate first, got %#v", frontier[0])
	}
	if frontier[0].SelectionMode != targetFrontierSelectionPromptRepair {
		t.Fatalf("expected prompt-repair selection mode, got %#v", frontier[0])
	}
	if frontier[1].TaskID != "task-b" {
		t.Fatalf("expected remaining candidate second, got %#v", frontier[1])
	}
}

func TestSummarizeTargetCoverageFrontierUsesActivationRepairForActivationPendingTask(t *testing.T) {
	base := testTargetScenarioCandidateWithProfile("task-a", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID)
	base.PromptVariantID = target.TargetPromptVariantBaseID
	base.CandidateID = targetScheduleCandidateIDWithVariant("test-target", base.TaskID, base.PromptProfileID, base.PromptVariantID)
	lifecycle := base
	lifecycle.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
	lifecycle.CandidateID = targetScheduleCandidateIDWithVariant("test-target", lifecycle.TaskID, lifecycle.PromptProfileID, lifecycle.PromptVariantID)
	activation := base
	activation.PromptVariantID = target.TargetPromptVariantActivationFocusID
	activation.CandidateID = targetScheduleCandidateIDWithVariant("test-target", activation.TaskID, activation.PromptProfileID, activation.PromptVariantID)
	other := testTargetScenarioCandidateWithProfile("task-b", "seed-b", "primitive-b", target.TargetPromptProfileBaselineID)

	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates:    []TargetScheduleCandidate{base, lifecycle, activation, other},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, []TargetSuiteRunResult{
		{
			CandidateID:     base.CandidateID,
			TaskID:          base.TaskID,
			PromptProfileID: base.PromptProfileID,
			PromptVariantID: base.PromptVariantID,
			OutcomeCategory: corpus.TargetObservationActivationNotTriggered,
			ActivationStage: TargetActivationStageActivationPending,
		},
	}, nil, 1)
	if len(frontier) != 1 {
		t.Fatalf("expected one frontier candidate, got %#v", frontier)
	}
	if frontier[0].CandidateID != activation.CandidateID {
		t.Fatalf("expected activation-focused candidate, got %#v", frontier[0])
	}
	if frontier[0].SelectionMode != targetFrontierSelectionActivationRepair {
		t.Fatalf("expected activation-repair selection mode, got %#v", frontier[0])
	}
}

func TestSummarizeTargetCoverageFrontierUsesSeedExpansionAfterConfirmedHit(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-a", "seed-shared", "primitive-shared", target.TargetPromptProfileBaselineID)
				item.ActivationKindID = "activation-a"
				item.OracleKindID = "oracle-a"
				item.Mutations = []target.TargetScenarioMutation{{MutationID: "mutation-a"}}
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-a2", "seed-shared", "primitive-shared", target.TargetPromptProfileBaselineID)
				item.ActivationKindID = "activation-b"
				item.OracleKindID = "oracle-b"
				item.Mutations = []target.TargetScenarioMutation{{MutationID: "mutation-b"}}
				return item
			}(),
			testTargetScenarioCandidateWithProfile("task-b", "seed-other", "primitive-other", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, []TargetSuiteRunResult{
		{
			CandidateID:     matrix.Candidates[0].CandidateID,
			Confirmed:       true,
			ActivationStage: TargetActivationStageActivationReached,
			TaskID:          "task-a",
			PromptProfileID: target.TargetPromptProfileBaselineID,
		},
	}, nil, 2)
	if len(frontier) != 2 {
		t.Fatalf("expected 2 frontier candidates, got %#v", frontier)
	}
	if frontier[0].TaskID != "task-a2" {
		t.Fatalf("expected shared-seed expansion frontier candidate first, got %#v", frontier[0])
	}
	if frontier[0].SelectionMode != targetFrontierSelectionSeedExpand {
		t.Fatalf("expected seed-expansion selection mode, got %#v", frontier[0])
	}
	if frontier[1].TaskID != "task-b" {
		t.Fatalf("expected unrelated candidate second, got %#v", frontier[1])
	}
}

func TestSummarizeTargetCoverageFrontierUsesVariantExpansionForCheckpointFamilies(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-checkpoint", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.PromptVariantID = target.TargetPromptVariantBaseID
				item.CandidateID = targetScheduleCandidateIDWithVariant("test-target", item.TaskID, item.PromptProfileID, item.PromptVariantID)
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-checkpoint", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
				item.CandidateID = targetScheduleCandidateIDWithVariant("test-target", item.TaskID, item.PromptProfileID, item.PromptVariantID)
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-checkpoint", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.PromptVariantID = target.TargetPromptVariantMutationFocusID
				item.MutationFocusID = "mutation-focus-a"
				item.CandidateID = targetScheduleCandidateIDWithVariant("test-target", item.TaskID, item.PromptProfileID, item.PromptVariantID)
				return item
			}(),
			testTargetScenarioCandidateWithProfile("task-other", "seed-b", "primitive-b", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	frontier := summarizeTargetCoverageFrontier(matrix, []TargetSuiteRunResult{
		{
			CandidateID:     matrix.Candidates[0].CandidateID,
			Confirmed:       true,
			ActivationStage: TargetActivationStageActivationReached,
			TaskID:          "task-checkpoint",
			PromptProfileID: target.TargetPromptProfileBaselineID,
			PromptVariantID: target.TargetPromptVariantBaseID,
			TargetOracle:    target.TargetOracleResult{Status: target.TargetOracleStatusConfirmed},
			TaskCompliance:  target.TargetTaskComplianceResult{Status: target.TargetTaskComplianceStatusCompliant},
		},
	}, nil, 2)
	if len(frontier) != 2 {
		t.Fatalf("expected 2 frontier candidates, got %#v", frontier)
	}
	if frontier[0].TaskID != "task-checkpoint" || frontier[0].SelectionMode != targetFrontierSelectionVariantExpand {
		t.Fatalf("expected checkpoint sibling variant first, got %#v", frontier[0])
	}
	if frontier[1].TaskID != "task-checkpoint" || frontier[1].SelectionMode != targetFrontierSelectionVariantExpand {
		t.Fatalf("expected second checkpoint sibling variant next, got %#v", frontier[1])
	}
}
