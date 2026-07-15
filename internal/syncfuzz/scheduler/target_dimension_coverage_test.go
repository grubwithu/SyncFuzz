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
			PromptVariantID:      target.TargetPromptVariantBaseID,
			StateSurface:         "workspace.file",
			LifecycleEdge:        "checkpoint->fork",
			ContractRuleID:       "rule-a",
			LifecycleOperationID: "checkpoint-fork",
			PlantPrimitiveID:     "primitive-a",
			ActivationKindID:     "activation-a",
			OracleKindID:         "oracle-a",
			ExecutionPlan: &target.TargetScenarioExecutionPlan{
				LifecycleOperationID: "checkpoint-fork",
				CheckpointSelector:   "before-a",
				ForkFollowup:         true,
				CheckpointBackend:    "disk",
				ProcessMode:          "split-process",
			},
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
			PromptVariantID:      target.TargetPromptVariantMutationFocusID,
			StateSurface:         "workspace.socket",
			LifecycleEdge:        "checkpoint->replay",
			ContractRuleID:       "rule-b",
			LifecycleOperationID: "checkpoint-replay",
			PlantPrimitiveID:     "primitive-b",
			ActivationKindID:     "activation-b",
			OracleKindID:         "oracle-b",
			ExecutionPlan: &target.TargetScenarioExecutionPlan{
				LifecycleOperationID: "checkpoint-replay",
				CheckpointSelector:   "before-b",
				Replay:               true,
				CheckpointBackend:    "disk",
				ProcessMode:          "split-process",
			},
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

	mutationCoverage := findTargetDimensionCoverage(t, summaries, "mutation_focus_id")
	if mutationCoverage.TotalValues != 2 || mutationCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected mutation coverage summary: %#v", mutationCoverage)
	}
	if len(mutationCoverage.MissingValues) != 1 || mutationCoverage.MissingValues[0] != "mutation-b" {
		t.Fatalf("unexpected mutation coverage gaps: %#v", mutationCoverage.MissingValues)
	}

	transitionCoverage := findTargetDimensionCoverage(t, summaries, "plant_to_activation")
	if transitionCoverage.TotalValues != 2 || transitionCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected transition coverage summary: %#v", transitionCoverage)
	}
	if len(transitionCoverage.MissingValues) != 1 || transitionCoverage.MissingValues[0] != "primitive-b->activation-b" {
		t.Fatalf("unexpected transition coverage gaps: %#v", transitionCoverage.MissingValues)
	}

	selectorCoverage := findTargetDimensionCoverage(t, summaries, "checkpoint_selector")
	if selectorCoverage.TotalValues != 2 || selectorCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected checkpoint selector coverage summary: %#v", selectorCoverage)
	}
	if len(selectorCoverage.MissingValues) != 1 || selectorCoverage.MissingValues[0] != "before-b" {
		t.Fatalf("unexpected checkpoint selector gaps: %#v", selectorCoverage.MissingValues)
	}

	signatureCoverage := findTargetDimensionCoverage(t, summaries, "transition_signature")
	if signatureCoverage.TotalValues != 2 || signatureCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected transition signature coverage summary: %#v", signatureCoverage)
	}
	if len(signatureCoverage.MissingValues) != 1 || signatureCoverage.MissingValues[0] != "workspace.socket=>replay|before-b|disk|split-process=>activation-b=>oracle-b" {
		t.Fatalf("unexpected transition signature gaps: %#v", signatureCoverage.MissingValues)
	}

	promptVariantCoverage := findTargetDimensionCoverage(t, summaries, "prompt_variant_id")
	if promptVariantCoverage.TotalValues != 2 || promptVariantCoverage.ExecutedValues != 1 {
		t.Fatalf("unexpected prompt variant coverage summary: %#v", promptVariantCoverage)
	}
	if len(promptVariantCoverage.MissingValues) != 1 || promptVariantCoverage.MissingValues[0] != target.TargetPromptVariantMutationFocusID {
		t.Fatalf("unexpected prompt variant coverage gaps: %#v", promptVariantCoverage.MissingValues)
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
			PromptVariantID:      target.TargetPromptVariantBaseID,
			StateSurface:         "workspace.file",
			LifecycleEdge:        "checkpoint->fork",
			ContractRuleID:       "rule-a",
			LifecycleOperationID: "checkpoint-fork",
			PlantPrimitiveID:     "primitive-a",
			ActivationKindID:     "activation-a",
			OracleKindID:         "oracle-a",
			ExecutionPlan: &target.TargetScenarioExecutionPlan{
				LifecycleOperationID: "checkpoint-fork",
				CheckpointSelector:   "before-a",
				ForkFollowup:         true,
				CheckpointBackend:    "disk",
				ProcessMode:          "split-process",
			},
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
			PromptVariantID:      target.TargetPromptVariantMutationFocusID,
			StateSurface:         "workspace.socket",
			LifecycleEdge:        "checkpoint->replay",
			ContractRuleID:       "rule-b",
			LifecycleOperationID: "checkpoint-replay",
			PlantPrimitiveID:     "primitive-b",
			ActivationKindID:     "activation-b",
			OracleKindID:         "oracle-b",
			ExecutionPlan: &target.TargetScenarioExecutionPlan{
				LifecycleOperationID: "checkpoint-replay",
				CheckpointSelector:   "before-b",
				Replay:               true,
				CheckpointBackend:    "disk",
				ProcessMode:          "split-process",
			},
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

	transitionGain := findTargetDimensionCoverageGain(t, gains, "activation_to_oracle")
	if len(transitionGain.NewExecutedValues) != 1 || transitionGain.NewExecutedValues[0] != "activation-b->oracle-b" {
		t.Fatalf("unexpected transition execution gain: %#v", transitionGain)
	}
	if len(transitionGain.NewConfirmedValues) != 1 || transitionGain.NewConfirmedValues[0] != "activation-a->oracle-a" {
		t.Fatalf("unexpected transition confirmation gain: %#v", transitionGain)
	}

	selectorGain := findTargetDimensionCoverageGain(t, gains, "checkpoint_selector")
	if len(selectorGain.NewExecutedValues) != 1 || selectorGain.NewExecutedValues[0] != "before-b" {
		t.Fatalf("unexpected selector execution gain: %#v", selectorGain)
	}
	if len(selectorGain.NewConfirmedValues) != 1 || selectorGain.NewConfirmedValues[0] != "before-a" {
		t.Fatalf("unexpected selector confirmation gain: %#v", selectorGain)
	}

	signatureGain := findTargetDimensionCoverageGain(t, gains, "transition_signature")
	if len(signatureGain.NewExecutedValues) != 1 || signatureGain.NewExecutedValues[0] != "workspace.socket=>replay|before-b|disk|split-process=>activation-b=>oracle-b" {
		t.Fatalf("unexpected transition signature execution gain: %#v", signatureGain)
	}
	if len(signatureGain.NewConfirmedValues) != 1 || signatureGain.NewConfirmedValues[0] != "workspace.file=>fork-followup|before-a|disk|split-process=>activation-a=>oracle-a" {
		t.Fatalf("unexpected transition signature confirmation gain: %#v", signatureGain)
	}
}

func TestSummarizeTargetDimensionCoverageGainStatsWeightsProgress(t *testing.T) {
	gains := []TargetDimensionCoverageGainSummary{
		{
			Dimension:                   "seed_id",
			NewExecutedValues:           []string{"seed-a"},
			NewConfirmedValues:          []string{"seed-a"},
			NewActivationProgressValues: []string{"seed-a"},
			NewActivationReachedValues:  []string{"seed-a"},
		},
		{
			Dimension:         "task_id",
			NewExecutedValues: []string{"task-b"},
		},
	}

	stats := summarizeTargetDimensionCoverageGainStats(gains)
	if stats.NewExecutedCount != 2 || stats.NewConfirmedCount != 1 || stats.NewActivationProgressCount != 1 || stats.NewActivationReachedCount != 1 {
		t.Fatalf("unexpected coverage gain counts: %#v", stats)
	}
	wantScore := targetDimensionGapWeight("seed_id")*8 + targetDimensionGapWeight("task_id")
	if stats.WeightedScore != wantScore {
		t.Fatalf("unexpected weighted score: got %d want %d", stats.WeightedScore, wantScore)
	}
}

func TestSummarizeTargetDimensionCoverageGainTracksIntermediateActivationProgress(t *testing.T) {
	universe := []TargetScheduleCandidate{
		{
			CandidateID:      "cand-a",
			TaskID:           "task-a",
			SeedID:           "seed-a",
			PlantPrimitiveID: "primitive-a",
			ActivationKindID: "activation-a",
			OracleKindID:     "oracle-a",
		},
	}
	previous := []TargetSuiteRunResult{
		{
			CandidateID:     "cand-a",
			ActivationStage: TargetActivationStageLifecyclePending,
		},
	}
	current := []TargetSuiteRunResult{
		{
			CandidateID:     "cand-a",
			ActivationStage: TargetActivationStageActivationPending,
		},
	}

	gains := summarizeTargetDimensionCoverageGain(universe, previous, current)
	taskGain := findTargetDimensionCoverageGain(t, gains, "task_id")
	if len(taskGain.NewExecutedValues) != 0 || len(taskGain.NewActivationReachedValues) != 0 {
		t.Fatalf("expected only intermediate progress gain, got %#v", taskGain)
	}
	if len(taskGain.NewActivationProgressValues) != 1 || taskGain.NewActivationProgressValues[0] != "task-a" {
		t.Fatalf("unexpected activation progress gain: %#v", taskGain)
	}
	stats := summarizeTargetDimensionCoverageGainStats(gains)
	if stats.NewActivationProgressCount == 0 || stats.WeightedScore <= 0 {
		t.Fatalf("expected activation progress to contribute to gain stats: %#v", stats)
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
