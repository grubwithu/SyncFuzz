package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestSummarizeTargetDimensionCoverageTracksMissingValuesAndProgress(t *testing.T) {
	universe := []TargetScheduleCandidate{
		{
			CandidateID:          "cand-a",
			ScenarioID:           "scenario-a",
			SeedID:               "seed-a",
			TaskID:               "task-a",
			PromptProfileID:      target.TargetPromptProfileBaselineID,
			StateSurface:         "workspace.file",
			LifecycleEdge:        "checkpoint->fork",
			ContractRuleID:       "rule-a",
			LifecycleOperationID: "checkpoint-fork",
			PlantPrimitiveID:     "primitive-a",
			ActivationKindID:     "activation-a",
			OracleKindID:         "oracle-a",
			Mutations: []target.TargetScenarioMutation{
				{MutationID: "mutation-a"},
			},
		},
		{
			CandidateID:          "cand-b",
			ScenarioID:           "scenario-b",
			SeedID:               "seed-b",
			TaskID:               "task-b",
			PromptProfileID:      target.TargetPromptProfileWorkflowID,
			StateSurface:         "workspace.socket",
			LifecycleEdge:        "checkpoint->replay",
			ContractRuleID:       "rule-b",
			LifecycleOperationID: "checkpoint-replay",
			PlantPrimitiveID:     "primitive-b",
			ActivationKindID:     "activation-b",
			OracleKindID:         "oracle-b",
			Mutations: []target.TargetScenarioMutation{
				{MutationID: "mutation-b"},
			},
		},
	}
	results := []TargetSuiteRunResult{
		{
			CandidateID:     "cand-a",
			Confirmed:       true,
			ActivationStage: TargetActivationStageActivationReached,
		},
	}

	summaries := summarizeTargetDimensionCoverage(universe, results)
	if len(summaries) == 0 {
		t.Fatalf("expected dimension coverage summaries")
	}

	taskCoverage := findTargetDimensionCoverage(t, summaries, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 1 || taskCoverage.ConfirmedValues != 1 || taskCoverage.ActivationReachedValues != 1 {
		t.Fatalf("unexpected task coverage summary: %#v", taskCoverage)
	}
	if len(taskCoverage.MissingValues) != 1 || taskCoverage.MissingValues[0] != "task-b" {
		t.Fatalf("unexpected task coverage gaps: %#v", taskCoverage.MissingValues)
	}

	mutationCoverage := findTargetDimensionCoverage(t, summaries, "mutation_id")
	if mutationCoverage.TotalValues != 2 || mutationCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected mutation coverage summary: %#v", mutationCoverage)
	}
	if len(mutationCoverage.MissingValues) != 1 || mutationCoverage.MissingValues[0] != "mutation-b" {
		t.Fatalf("unexpected mutation coverage gaps: %#v", mutationCoverage.MissingValues)
	}
}

func TestSummarizeTargetDimensionCoverageGainTracksNewValuesAndTransitions(t *testing.T) {
	universe := []TargetScheduleCandidate{
		{
			CandidateID:          "cand-a",
			ScenarioID:           "scenario-a",
			SeedID:               "seed-a",
			TaskID:               "task-a",
			PromptProfileID:      target.TargetPromptProfileBaselineID,
			StateSurface:         "workspace.file",
			LifecycleEdge:        "checkpoint->fork",
			ContractRuleID:       "rule-a",
			LifecycleOperationID: "checkpoint-fork",
			PlantPrimitiveID:     "primitive-a",
			ActivationKindID:     "activation-a",
			OracleKindID:         "oracle-a",
			Mutations: []target.TargetScenarioMutation{
				{MutationID: "mutation-a"},
			},
		},
		{
			CandidateID:          "cand-b",
			ScenarioID:           "scenario-b",
			SeedID:               "seed-b",
			TaskID:               "task-b",
			PromptProfileID:      target.TargetPromptProfileWorkflowID,
			StateSurface:         "workspace.socket",
			LifecycleEdge:        "checkpoint->replay",
			ContractRuleID:       "rule-b",
			LifecycleOperationID: "checkpoint-replay",
			PlantPrimitiveID:     "primitive-b",
			ActivationKindID:     "activation-b",
			OracleKindID:         "oracle-b",
			Mutations: []target.TargetScenarioMutation{
				{MutationID: "mutation-b"},
			},
		},
	}
	previous := []TargetSuiteRunResult{
		{
			CandidateID: "cand-a",
		},
	}
	current := []TargetSuiteRunResult{
		{
			CandidateID:     "cand-a",
			Confirmed:       true,
			ActivationStage: TargetActivationStageActivationReached,
		},
		{
			CandidateID: "cand-b",
		},
	}

	gains := summarizeTargetDimensionCoverageGain(universe, previous, current)
	if len(gains) == 0 {
		t.Fatalf("expected dimension coverage gains")
	}

	taskGain := findTargetDimensionCoverageGain(t, gains, "task_id")
	if len(taskGain.NewExecutedValues) != 1 || taskGain.NewExecutedValues[0] != "task-b" {
		t.Fatalf("unexpected task execution gain: %#v", taskGain)
	}
	if len(taskGain.NewConfirmedValues) != 1 || taskGain.NewConfirmedValues[0] != "task-a" {
		t.Fatalf("unexpected task confirmation gain: %#v", taskGain)
	}
	if len(taskGain.NewActivationReachedValues) != 1 || taskGain.NewActivationReachedValues[0] != "task-a" {
		t.Fatalf("unexpected task activation gain: %#v", taskGain)
	}
}

func TestSummarizeTargetDimensionCoverageGainStatsWeightsProgress(t *testing.T) {
	gains := []TargetDimensionCoverageGainSummary{
		{
			Dimension:                  "seed_id",
			NewExecutedValues:          []string{"seed-a"},
			NewConfirmedValues:         []string{"seed-a"},
			NewActivationReachedValues: []string{"seed-a"},
		},
		{
			Dimension:         "task_id",
			NewExecutedValues: []string{"task-b"},
		},
	}

	stats := summarizeTargetDimensionCoverageGainStats(gains)
	if stats.NewExecutedCount != 2 || stats.NewConfirmedCount != 1 || stats.NewActivationReachedCount != 1 {
		t.Fatalf("unexpected coverage gain counts: %#v", stats)
	}
	wantScore := targetDimensionGapWeight("seed_id")*6 + targetDimensionGapWeight("task_id")
	if stats.WeightedScore != wantScore {
		t.Fatalf("unexpected weighted score: got %d want %d", stats.WeightedScore, wantScore)
	}
}

func findTargetDimensionCoverage(t *testing.T, summaries []TargetDimensionCoverageSummary, dimension string) TargetDimensionCoverageSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.Dimension == dimension {
			return summary
		}
	}
	t.Fatalf("dimension coverage %q not found: %#v", dimension, summaries)
	return TargetDimensionCoverageSummary{}
}

func findTargetDimensionCoverageGain(t *testing.T, summaries []TargetDimensionCoverageGainSummary, dimension string) TargetDimensionCoverageGainSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.Dimension == dimension {
			return summary
		}
	}
	t.Fatalf("dimension coverage gain %q not found: %#v", dimension, summaries)
	return TargetDimensionCoverageGainSummary{}
}
