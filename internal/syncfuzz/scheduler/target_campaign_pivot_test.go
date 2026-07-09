package scheduler

import (
	"testing"

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
