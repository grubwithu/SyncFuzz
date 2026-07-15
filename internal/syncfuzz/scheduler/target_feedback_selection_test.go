package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestSelectTargetMatrixCandidatesPrefersDistinctTasksBeforeAlternatePromptProfiles(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileAuditID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{Limit: 3})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 3 {
		t.Fatalf("expected 3 selected candidates, got %d", len(selected.Candidates))
	}

	wantTasks := []string{"task-a", "task-b", "task-c"}
	for i, taskID := range wantTasks {
		if selected.Candidates[i].TaskID != taskID {
			t.Fatalf("candidate %d task mismatch: got %q want %q", i, selected.Candidates[i].TaskID, taskID)
		}
		if selected.Candidates[i].PromptProfileID != target.TargetPromptProfileBaselineID {
			t.Fatalf("candidate %d prompt mismatch: got %q want %q", i, selected.Candidates[i].PromptProfileID, target.TargetPromptProfileBaselineID)
		}
	}
}

func TestSelectTargetMatrixCandidatesAppendsDiverseUnseenCandidatesAfterFeedback(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID), Score: 9, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileWorkflowID), Score: 8, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileAuditID), Score: 7, ReproducibilityRate: 1, Confirmed: 1},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom: feedbackPath,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 5 {
		t.Fatalf("expected 5 selected candidates, got %d", len(selected.Candidates))
	}

	for i, profileID := range []string{target.TargetPromptProfileBaselineID, target.TargetPromptProfileWorkflowID, target.TargetPromptProfileAuditID} {
		if selected.Candidates[i].TaskID != "task-a" || selected.Candidates[i].PromptProfileID != profileID {
			t.Fatalf("ranked candidate %d mismatch: got task=%q profile=%q", i, selected.Candidates[i].TaskID, selected.Candidates[i].PromptProfileID)
		}
	}
	if selected.Candidates[3].TaskID != "task-b" || selected.Candidates[3].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected first unseen candidate to be baseline of a new task, got task=%q profile=%q", selected.Candidates[3].TaskID, selected.Candidates[3].PromptProfileID)
	}
	if selected.Candidates[4].TaskID != "task-c" || selected.Candidates[4].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected second unseen candidate to expand to another task baseline, got task=%q profile=%q", selected.Candidates[4].TaskID, selected.Candidates[4].PromptProfileID)
	}
}

func TestSelectTargetMatrixCandidatesPrefersNewSeedsBeforeSameSeedVariants(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScenarioCandidate("task-a", "seed-a", "primitive-a"),
			testTargetScenarioCandidate("task-b", "seed-a", "primitive-b"),
			testTargetScenarioCandidate("task-c", "seed-b", "primitive-a"),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{Limit: 2})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-a" {
		t.Fatalf("expected first candidate to preserve base ordering, got %q", selected.Candidates[0].TaskID)
	}
	if selected.Candidates[1].TaskID != "task-c" {
		t.Fatalf("expected second candidate to expand to a new seed, got %q", selected.Candidates[1].TaskID)
	}
}

func TestSelectTargetMatrixCandidatesPrefersNewExecutionTransitionBeforeDuplicatePath(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-a", "seed-shared", "primitive-shared", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.ActivationKindID = "activation-shared"
				item.OracleKindID = "oracle-shared"
				item.ExecutionPlan = &target.TargetScenarioExecutionPlan{
					LifecycleOperationID: "checkpoint-replay",
					CheckpointSelector:   "before-a",
					Replay:               true,
					CheckpointBackend:    "disk",
					ProcessMode:          "split-process",
				}
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-z", "seed-shared", "primitive-shared", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.ActivationKindID = "activation-shared"
				item.OracleKindID = "oracle-shared"
				item.ExecutionPlan = &target.TargetScenarioExecutionPlan{
					LifecycleOperationID: "checkpoint-replay",
					CheckpointSelector:   "before-b",
					Replay:               true,
					CheckpointBackend:    "disk",
					ProcessMode:          "split-process",
				}
				return item
			}(),
			func() TargetScheduleCandidate {
				item := testTargetScenarioCandidateWithProfile("task-b", "seed-shared", "primitive-shared", target.TargetPromptProfileBaselineID)
				item.LifecycleOperationID = "checkpoint-replay"
				item.ActivationKindID = "activation-shared"
				item.OracleKindID = "oracle-shared"
				item.ExecutionPlan = &target.TargetScenarioExecutionPlan{
					LifecycleOperationID: "checkpoint-replay",
					CheckpointSelector:   "before-a",
					Replay:               true,
					CheckpointBackend:    "disk",
					ProcessMode:          "split-process",
				}
				return item
			}(),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{Limit: 2})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-a" {
		t.Fatalf("expected first candidate to preserve base ordering, got %#v", selected.Candidates[0])
	}
	if selected.Candidates[1].TaskID != "task-z" {
		t.Fatalf("expected second candidate to prioritize a new execution transition, got %#v", selected.Candidates[1])
	}
}

func TestSelectTargetMatrixCandidatesUsesCoverageGapsToPrioritizeUnseenCandidates(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScenarioCandidateWithProfile("task-a", "seed-a", "primitive-a", target.TargetPromptProfileBaselineID),
			testTargetScenarioCandidateWithProfile("task-b", "seed-b", "primitive-b", target.TargetPromptProfileBaselineID),
			testTargetScenarioCandidateWithProfile("task-z", "seed-z", "primitive-z", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID), Score: 9, ReproducibilityRate: 1, Confirmed: 1},
		},
		DimensionCoverage: []TargetDimensionCoverageSummary{
			{Dimension: "task_id", TotalValues: 3, ExecutedValues: 1, MissingValues: []string{"task-z"}},
			{Dimension: "seed_id", TotalValues: 3, ExecutedValues: 1, MissingValues: []string{"seed-z"}},
			{Dimension: "plant_primitive_id", TotalValues: 3, ExecutedValues: 1, MissingValues: []string{"primitive-z"}},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom:        feedbackPath,
		ExcludeCandidateIDs: []string{targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID)},
		Limit:               2,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-z" {
		t.Fatalf("expected coverage gap candidate first, got task=%q", selected.Candidates[0].TaskID)
	}
	if selected.Candidates[1].TaskID != "task-b" {
		t.Fatalf("expected remaining candidate second, got task=%q", selected.Candidates[1].TaskID)
	}
}

func TestSelectTargetMatrixCandidatesUsesPromptRepairBeforeNewTaskExpansion(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{
				CandidateID:      targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID),
				TaskID:           "task-a",
				PromptProfileID:  target.TargetPromptProfileBaselineID,
				OutcomeSummaries: []TargetSuiteOutcomeStats{{Category: corpus.TargetObservationExecutionNotReached, TotalRuns: 1}},
			},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom:        feedbackPath,
		ExcludeCandidateIDs: []string{targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID)},
		Limit:               2,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-a" || selected.Candidates[0].PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected prompt repair candidate first, got task=%q profile=%q", selected.Candidates[0].TaskID, selected.Candidates[0].PromptProfileID)
	}
	if selected.Candidates[1].TaskID != "task-b" || selected.Candidates[1].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected new task expansion second, got task=%q profile=%q", selected.Candidates[1].TaskID, selected.Candidates[1].PromptProfileID)
	}
}

func TestSelectTargetMatrixCandidatesSkipsPromptRepairOnceActivationReached(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{
				CandidateID:         targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID),
				TaskID:              "task-a",
				PromptProfileID:     target.TargetPromptProfileBaselineID,
				ActivationReached:   1,
				OutcomeSummaries:    []TargetSuiteOutcomeStats{{Category: corpus.TargetObservationCleanNegative, TotalRuns: 1}},
				ActivationSummaries: []TargetSuiteActivationStats{{Stage: TargetActivationStageActivationReached, TotalRuns: 1}},
			},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom:        feedbackPath,
		ExcludeCandidateIDs: []string{targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID)},
		Limit:               2,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-b" || selected.Candidates[0].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected new task expansion first after activation reached, got task=%q profile=%q", selected.Candidates[0].TaskID, selected.Candidates[0].PromptProfileID)
	}
	if selected.Candidates[1].TaskID != "task-a" || selected.Candidates[1].PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected alternate prompt profile second after new task, got task=%q profile=%q", selected.Candidates[1].TaskID, selected.Candidates[1].PromptProfileID)
	}
}

func TestPromptRepairPrefersActivationVariantAfterActivationPendingOutcome(t *testing.T) {
	feedback := newTargetPromptRepairFeedback(map[string]TargetCandidateSummary{
		"test-target/task-a": {
			CandidateID:     "test-target/task-a",
			TaskID:          "task-a",
			PromptProfileID: target.TargetPromptProfileBaselineID,
			PromptVariantID: target.TargetPromptVariantBaseID,
			OutcomeSummaries: []TargetSuiteOutcomeStats{
				{Category: corpus.TargetObservationActivationNotTriggered, TotalRuns: 1},
			},
		},
	})
	if feedback == nil {
		t.Fatal("expected activation-pending outcome to create prompt repair feedback")
	}

	lifecycle := testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID)
	lifecycle.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
	activation := lifecycle
	activation.PromptVariantID = target.TargetPromptVariantActivationFocusID

	activationScore := targetPromptRepairScore(activation, feedback)
	lifecycleScore := targetPromptRepairScore(lifecycle, feedback)
	if activationScore <= lifecycleScore {
		t.Fatalf("expected activation-focused repair to score higher: activation=%d lifecycle=%d", activationScore, lifecycleScore)
	}
}

func TestPromptRepairUsesActivationStageWhenOutcomeSummaryIsUnavailable(t *testing.T) {
	feedback := newTargetPromptRepairFeedback(map[string]TargetCandidateSummary{
		"test-target/task-a": {
			CandidateID:     "test-target/task-a",
			TaskID:          "task-a",
			PromptProfileID: target.TargetPromptProfileBaselineID,
			PromptVariantID: target.TargetPromptVariantBaseID,
			Runs:            2,
			ActivationSummaries: []TargetSuiteActivationStats{
				{Stage: TargetActivationStageStateNotPlanted, TotalRuns: 2},
			},
		},
	})
	if feedback == nil {
		t.Fatal("expected stage-only feedback to create a prompt repair signal")
	}

	mutation := testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID)
	mutation.PromptVariantID = target.TargetPromptVariantMutationFocusID
	lifecycle := mutation
	lifecycle.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
	if targetPromptRepairScore(mutation, feedback) <= targetPromptRepairScore(lifecycle, feedback) {
		t.Fatal("expected state-not-planted progress to prefer mutation-focused repair")
	}
}

func TestPromptRepairFromResultsUsesActivationStageWhenOutcomeIsUnavailable(t *testing.T) {
	base := testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID)
	candidates := map[string]TargetScheduleCandidate{base.CandidateID: base}
	feedback := newTargetPromptRepairFeedbackFromResults(candidates, []TargetSuiteRunResult{
		{
			CandidateID:     base.CandidateID,
			ActivationStage: TargetActivationStageActivationPending,
		},
	})
	if feedback == nil {
		t.Fatal("expected stage-only run result to create prompt repair feedback")
	}

	activation := base
	activation.PromptVariantID = target.TargetPromptVariantActivationFocusID
	lifecycle := base
	lifecycle.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
	if targetPromptRepairScore(activation, feedback) <= targetPromptRepairScore(lifecycle, feedback) {
		t.Fatal("expected activation-pending stage to prefer activation-focused repair")
	}
}

func TestSelectTargetMatrixCandidatesPrefersSeedExpansionAfterConfirmedHit(t *testing.T) {
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

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{
				CandidateID:          matrix.Candidates[0].CandidateID,
				TaskID:               "task-a",
				PromptProfileID:      target.TargetPromptProfileBaselineID,
				SeedID:               "seed-shared",
				PlantPrimitiveID:     "primitive-shared",
				LifecycleOperationID: "checkpoint-fork",
				ActivationKindID:     "activation-a",
				OracleKindID:         "oracle-a",
				Confirmed:            1,
				Mutations:            []target.TargetScenarioMutation{{MutationID: "mutation-a"}},
				OutcomeSummaries:     []TargetSuiteOutcomeStats{{Category: corpus.TargetObservationResidueObserved, TotalRuns: 1}},
				ActivationSummaries:  []TargetSuiteActivationStats{{Stage: TargetActivationStageActivationReached, TotalRuns: 1}},
				ContractViolations:   1,
				ComplianceCompliant:  1,
				ReproducibilityRate:  1,
			},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom:        feedbackPath,
		ExcludeCandidateIDs: []string{matrix.Candidates[0].CandidateID},
		Limit:               2,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-a2" {
		t.Fatalf("expected shared-seed expansion candidate first, got %#v", selected.Candidates[0])
	}
	if selected.Candidates[1].TaskID != "task-b" {
		t.Fatalf("expected unrelated candidate second, got %#v", selected.Candidates[1])
	}
}

func TestSelectTargetMatrixCandidatesPrefersVariantExpansionForCheckpointFamilies(t *testing.T) {
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
				item.MutationFocusID = "mutation-focus-a"
				item.PromptVariantID = target.TargetPromptVariantMutationFocusID
				item.CandidateID = targetScheduleCandidateIDWithVariant("test-target", item.TaskID, item.PromptProfileID, item.PromptVariantID)
				return item
			}(),
			testTargetScenarioCandidateWithProfile("task-other", "seed-b", "primitive-b", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{
				CandidateID:          matrix.Candidates[0].CandidateID,
				TaskID:               "task-checkpoint",
				PromptProfileID:      target.TargetPromptProfileBaselineID,
				PromptVariantID:      target.TargetPromptVariantBaseID,
				LifecycleOperationID: "checkpoint-replay",
				Confirmed:            1,
				OutcomeSummaries:     []TargetSuiteOutcomeStats{{Category: corpus.TargetObservationResidueObserved, TotalRuns: 1}},
				ActivationSummaries:  []TargetSuiteActivationStats{{Stage: TargetActivationStageActivationReached, TotalRuns: 1}},
				ReproducibilityRate:  1,
			},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom:        feedbackPath,
		ExcludeCandidateIDs: []string{matrix.Candidates[0].CandidateID},
		Limit:               2,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 2 {
		t.Fatalf("expected 2 selected candidates, got %d", len(selected.Candidates))
	}
	if selected.Candidates[0].TaskID != "task-checkpoint" || target.NormalizeTargetPromptVariantID(selected.Candidates[0].PromptVariantID) == target.TargetPromptVariantBaseID {
		t.Fatalf("expected sibling checkpoint variant first, got %#v", selected.Candidates[0])
	}
	if selected.Candidates[1].TaskID != "task-checkpoint" || target.NormalizeTargetPromptVariantID(selected.Candidates[1].PromptVariantID) == target.TargetPromptVariantBaseID {
		t.Fatalf("expected second sibling checkpoint variant next, got %#v", selected.Candidates[1])
	}
}

func TestSelectTargetMatrixCandidatesAllowsFullExclusionToReturnEmptyMatrix(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		ExcludeCandidateIDs: []string{matrix.Candidates[0].CandidateID},
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if selected.TotalCandidates != 0 || len(selected.Candidates) != 0 {
		t.Fatalf("expected full exclusion to produce an empty candidate set, got %#v", selected)
	}
}

func testTargetScheduleCandidate(taskID string, profileID string) TargetScheduleCandidate {
	return TargetScheduleCandidate{
		CandidateID:            targetScheduleCandidateID("test-target", taskID, profileID),
		TargetID:               "test-target",
		TaskID:                 taskID,
		PromptProfileID:        profileID,
		ContractRuleID:         "shared-rule",
		ContractProfileID:      "test-profile",
		ContractExpectation:    target.TargetContractExpectationReset,
		ContractSourceStrength: target.TargetContractSourceStrengthImplicit,
		StateSurface:           "workspace.file",
		LifecycleEdge:          "checkpoint->fork",
	}
}

func testTargetScenarioCandidate(taskID string, seedID string, primitiveID string) TargetScheduleCandidate {
	return testTargetScenarioCandidateWithProfile(taskID, seedID, primitiveID, target.TargetPromptProfileBaselineID)
}

func testTargetScenarioCandidateWithProfile(taskID string, seedID string, primitiveID string, profileID string) TargetScheduleCandidate {
	item := testTargetScheduleCandidate(taskID, profileID)
	item.SeedID = seedID
	item.PlantPrimitiveID = primitiveID
	item.LifecycleOperationID = "checkpoint-fork"
	item.ActivationKindID = "file-presence-check"
	item.OracleKindID = "workspace-file-residue"
	return item
}
