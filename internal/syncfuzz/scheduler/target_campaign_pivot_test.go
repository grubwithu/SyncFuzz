package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestSummarizeTargetCampaignPivotRecommendationsSuggestsMissingProfilesAndSeeds(t *testing.T) {
	universe := &TargetScheduleMatrix{
		TargetID: "test-target",
		Candidates: []TargetScheduleCandidate{
			{
				CandidateID:      targetScheduleCandidateID("test-target", target.DefaultTargetTaskID, target.TargetPromptProfileBaselineID),
				TaskID:           target.DefaultTargetTaskID,
				SeedID:           "delayed-effect",
				PromptProfileID:  target.TargetPromptProfileBaselineID,
				StateSurface:     "workspace.file-effect",
				PlantPrimitiveID: "background-process",
				ActivationKindID: "workspace-file-appearance",
				OracleKindID:     "expected-file",
			},
		},
	}

	recommendations, catalogExhausted := summarizeTargetCampaignPivotRecommendations(universe)
	if catalogExhausted {
		t.Fatalf("expected pivot recommendations, got catalog exhausted")
	}

	promptRecommendation := findTargetPivotRecommendation(t, recommendations, "prompt_profile_id")
	if len(promptRecommendation.Values) == 0 {
		t.Fatalf("expected missing prompt profile recommendations: %#v", promptRecommendation)
	}
	for _, forbidden := range []string{target.TargetPromptProfileWorkflowID, target.TargetPromptProfileAuditID} {
		if !target.ContainsString(promptRecommendation.Values, forbidden) {
			t.Fatalf("expected prompt profile recommendation %q in %#v", forbidden, promptRecommendation.Values)
		}
	}

	seedRecommendation := findTargetPivotRecommendation(t, recommendations, "seed_id")
	if len(seedRecommendation.Values) == 0 || target.ContainsString(seedRecommendation.Values, "delayed-effect") {
		t.Fatalf("expected only missing seed recommendations: %#v", seedRecommendation)
	}
}

func findTargetPivotRecommendation(t *testing.T, recommendations []TargetCampaignPivotRecommendation, dimension string) TargetCampaignPivotRecommendation {
	t.Helper()
	for _, recommendation := range recommendations {
		if recommendation.Dimension == dimension {
			return recommendation
		}
	}
	t.Fatalf("pivot recommendation %q not found: %#v", dimension, recommendations)
	return TargetCampaignPivotRecommendation{}
}

func TestTargetCampaignBestPivotOptionChoosesSinglePromptProfileStep(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
*) exit 9 ;;
esac`

	suite, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:           filepath.Join(tmp, "runs"),
		CorpusDir:        filepath.Join(tmp, "corpus"),
		TargetID:         "pivot-option-smoke",
		Tasks:            []string{target.DefaultTargetTaskID},
		PromptProfileIDs: []string{target.TargetPromptProfileBaselineID},
		Command:          command,
		ObserveDelay:     10 * time.Millisecond,
		Matrix:           true,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}

	universe, err := buildTargetCampaignUniverse(TargetCampaignOptions{
		TargetID:         "pivot-option-smoke",
		Tasks:            []string{target.DefaultTargetTaskID},
		PromptProfileIDs: []string{target.TargetPromptProfileBaselineID},
	})
	if err != nil {
		t.Fatalf("buildTargetCampaignUniverse failed: %v", err)
	}
	recommendation := findTargetPivotRecommendation(t, mustTargetPivotRecommendations(t, universe), "prompt_profile_id")
	nextOpts, nextUniverse, event, ok, err := targetCampaignBestPivotOption(
		TargetCampaignOptions{
			TargetID:         "pivot-option-smoke",
			Tasks:            []string{target.DefaultTargetTaskID},
			PromptProfileIDs: []string{target.TargetPromptProfileBaselineID},
		},
		universe,
		suite.Results,
		recommendation,
		1,
	)
	if err != nil {
		t.Fatalf("targetCampaignBestPivotOption failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected prompt-profile pivot option")
	}
	if event.Dimension != "prompt_profile_id" || len(event.Values) != 1 {
		t.Fatalf("expected single-value prompt-profile pivot event, got %#v", event)
	}
	if event.Values[0] != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected workflow to be the first prompt-profile pivot step, got %#v", event)
	}
	if event.NewCandidateCount <= 0 || event.FrontierCandidate == "" {
		t.Fatalf("expected pivot event to explain why this step was chosen, got %#v", event)
	}
	if event.FrontierSelection == "" {
		t.Fatalf("expected pivot event to record frontier selection mode, got %#v", event)
	}
	if len(nextOpts.PromptProfileIDs) != 2 || !target.ContainsString(nextOpts.PromptProfileIDs, target.TargetPromptProfileWorkflowID) {
		t.Fatalf("expected next opts to add exactly one prompt profile, got %#v", nextOpts.PromptProfileIDs)
	}
	if len(nextUniverse.PromptProfiles) != 2 {
		t.Fatalf("expected next universe to carry exactly two prompt profiles, got %#v", nextUniverse.PromptProfiles)
	}
}

func mustTargetPivotRecommendations(t *testing.T, universe *TargetScheduleMatrix) []TargetCampaignPivotRecommendation {
	t.Helper()
	recommendations, catalogExhausted := summarizeTargetCampaignPivotRecommendations(universe)
	if catalogExhausted || len(recommendations) == 0 {
		t.Fatalf("expected pivot recommendations for %#v", universe)
	}
	return recommendations
}
