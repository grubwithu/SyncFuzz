package scheduler

import (
	"testing"

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
	}, 2)
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
		!target.ContainsString(frontier[0].CoveredGaps, "mutation_id=mutation-z") {
		t.Fatalf("expected frontier candidate to explain covered gaps: %#v", frontier[0])
	}
}
