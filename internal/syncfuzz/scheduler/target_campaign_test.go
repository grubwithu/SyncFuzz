package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestRunTargetCampaignFeedsBackAcrossRounds(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`

	result, err := RunTargetCampaign(context.Background(), TargetCampaignOptions{
		OutDir:         tmp,
		CorpusDir:      filepath.Join(tmp, "corpus"),
		TargetID:       "campaign-smoke",
		Tasks:          []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:        command,
		ObserveDelay:   10 * time.Millisecond,
		Rounds:         2,
		CandidateLimit: 1,
	})
	if err != nil {
		t.Fatalf("RunTargetCampaign failed: %v", err)
	}

	if result.SchemaVersion != "syncfuzz.target-campaign-result.v1" {
		t.Fatalf("unexpected schema version %q", result.SchemaVersion)
	}
	if result.TotalSuites != 2 || len(result.RoundResults) != 2 {
		t.Fatalf("expected two target campaign rounds: %#v", result)
	}
	if result.TotalRuns != 2 {
		t.Fatalf("expected two budgeted target runs, got %d runs", result.TotalRuns)
	}
	if result.UniqueCandidates != 2 || result.RepeatedCandidates != 0 {
		t.Fatalf("expected cross-round target candidate de-duplication: unique=%d repeated=%d", result.UniqueCandidates, result.RepeatedCandidates)
	}
	if result.RoundResults[0].SchedulerMode != suiteSchedulerFeedback || result.RoundResults[0].TotalCandidates != 1 {
		t.Fatalf("expected first target round to run budgeted matrix: %#v", result.RoundResults[0])
	}
	if result.RoundResults[1].SchedulerMode != suiteSchedulerFeedback || result.RoundResults[1].TotalCandidates != 1 {
		t.Fatalf("expected second target round to run feedback budget without repeating first candidate: %#v", result.RoundResults[1])
	}
	if result.RoundResults[1].FeedbackFrom != result.RoundResults[0].MatrixResult {
		t.Fatalf("expected second target round to consume first round feedback")
	}
	if result.RoundResults[1].TopCandidate == nil {
		t.Fatalf("expected top target candidate summary")
	}
	if len(result.RoundResults[0].FrontierCandidates) != 1 {
		t.Fatalf("expected round 1 frontier to point at the remaining candidate: %#v", result.RoundResults[0].FrontierCandidates)
	}
	if result.RoundResults[0].FrontierCandidates[0].CandidateID == result.RoundResults[0].TopCandidate.CandidateID {
		t.Fatalf("expected round 1 frontier to exclude the already executed candidate: %#v", result.RoundResults[0].FrontierCandidates[0])
	}
	if len(result.OutcomeSummaries) == 0 || result.OutcomeSummaries[0].Category != corpus.TargetObservationResidueObserved {
		t.Fatalf("expected campaign outcome summaries: %#v", result.OutcomeSummaries)
	}
	if len(result.ActivationSummaries) == 0 || result.ActivationSummaries[0].Stage != TargetActivationStageActivationReached {
		t.Fatalf("expected campaign activation summaries: %#v", result.ActivationSummaries)
	}
	taskCoverage := findTargetDimensionCoverage(t, result.DimensionCoverage, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 2 || taskCoverage.ConfirmedValues != 2 || len(taskCoverage.MissingValues) != 0 {
		t.Fatalf("unexpected campaign task coverage: %#v", taskCoverage)
	}
	if len(result.RoundResults[0].OutcomeSummaries) == 0 || len(result.RoundResults[0].ActivationSummaries) == 0 {
		t.Fatalf("expected per-round outcome/activation summaries: %#v", result.RoundResults[0])
	}
	roundCoverage := findTargetDimensionCoverage(t, result.RoundResults[0].DimensionCoverage, "task_id")
	if roundCoverage.TotalValues != 2 || roundCoverage.ExecutedValues != 1 || roundCoverage.ConfirmedValues != 1 || len(roundCoverage.MissingValues) != 1 {
		t.Fatalf("unexpected per-round task coverage: %#v", roundCoverage)
	}
	round1Gain := findTargetDimensionCoverageGain(t, result.RoundResults[0].CoverageGain, "task_id")
	round2Gain := findTargetDimensionCoverageGain(t, result.RoundResults[1].CoverageGain, "task_id")
	if len(round1Gain.NewExecutedValues) != 1 || len(round2Gain.NewExecutedValues) != 1 {
		t.Fatalf("expected each round to contribute one new executed task: round1=%#v round2=%#v", round1Gain, round2Gain)
	}
	if round1Gain.NewExecutedValues[0] == round2Gain.NewExecutedValues[0] {
		t.Fatalf("expected rounds to expand different task coverage: round1=%#v round2=%#v", round1Gain, round2Gain)
	}
	if result.RoundResults[0].CoverageGainStats != summarizeTargetDimensionCoverageGainStats(result.RoundResults[0].CoverageGain) ||
		result.RoundResults[1].CoverageGainStats != summarizeTargetDimensionCoverageGainStats(result.RoundResults[1].CoverageGain) {
		t.Fatalf("expected per-round coverage gain stats to match gain summaries: round1=%#v round2=%#v", result.RoundResults[0].CoverageGainStats, result.RoundResults[1].CoverageGainStats)
	}
	if result.RoundResults[0].CoverageGainStats.WeightedScore <= 0 || result.RoundResults[1].CoverageGainStats.WeightedScore <= 0 {
		t.Fatalf("expected positive per-round coverage gain scores: round1=%#v round2=%#v", result.RoundResults[0].CoverageGainStats, result.RoundResults[1].CoverageGainStats)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetCampaignResultArtifact)); err != nil {
		t.Fatalf("expected target campaign result artifact: %v", err)
	}
}

func TestRunTargetCampaignStopsEarlyWhenCoverageGainStagnates(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`

	result, err := RunTargetCampaign(context.Background(), TargetCampaignOptions{
		OutDir:               tmp,
		CorpusDir:            filepath.Join(tmp, "corpus"),
		TargetID:             "campaign-stop-smoke",
		Tasks:                []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:              command,
		ObserveDelay:         10 * time.Millisecond,
		Rounds:               4,
		CandidateLimit:       1,
		MinCoverageGainScore: 0,
		MaxStagnantRounds:    1,
	})
	if err != nil {
		t.Fatalf("RunTargetCampaign failed: %v", err)
	}

	if !result.StoppedEarly {
		t.Fatalf("expected target campaign to stop early: %#v", result)
	}
	if result.StopReason == "" {
		t.Fatalf("expected stop reason when target campaign stops early")
	}
	if result.CatalogExhausted {
		t.Fatalf("expected pivot recommendations for narrow stopped campaign: %#v", result)
	}
	if len(result.PivotRecommendations) == 0 {
		t.Fatalf("expected pivot recommendations when target campaign stops early: %#v", result)
	}
	if result.TotalSuites != 3 || len(result.RoundResults) != 3 {
		t.Fatalf("expected campaign to stop after first stagnant repeat round: %#v", result.RoundResults)
	}
	if result.RoundResults[2].CoverageGainStats.WeightedScore != 0 {
		t.Fatalf("expected stagnant round to have zero coverage gain score: %#v", result.RoundResults[2].CoverageGainStats)
	}
}

func TestRunTargetCampaignAutoPivotsPromptProfilesAfterStagnation(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
*) exit 9 ;;
esac`

	result, err := RunTargetCampaign(context.Background(), TargetCampaignOptions{
		OutDir:               tmp,
		CorpusDir:            filepath.Join(tmp, "corpus"),
		TargetID:             "campaign-autopivot-smoke",
		Tasks:                []string{target.DefaultTargetTaskID},
		PromptProfileID:      target.TargetPromptProfileBaselineID,
		Command:              command,
		ObserveDelay:         10 * time.Millisecond,
		Rounds:               4,
		CandidateLimit:       1,
		MinCoverageGainScore: 0,
		MaxStagnantRounds:    1,
		AutoPivot:            true,
	})
	if err != nil {
		t.Fatalf("RunTargetCampaign failed: %v", err)
	}

	if result.StoppedEarly {
		t.Fatalf("expected auto pivot to continue campaign instead of stopping early: %#v", result)
	}
	if len(result.PivotHistory) != 1 {
		t.Fatalf("expected exactly one pivot event: %#v", result.PivotHistory)
	}
	pivot := result.PivotHistory[0]
	if pivot.Dimension != "prompt_profile_id" {
		t.Fatalf("expected prompt-profile pivot, got %#v", pivot)
	}
	if !target.ContainsString(pivot.Values, target.TargetPromptProfileWorkflowID) || !target.ContainsString(pivot.Values, target.TargetPromptProfileAuditID) {
		t.Fatalf("expected workflow/audit pivot values: %#v", pivot)
	}
	if len(result.PromptProfiles) != 3 {
		t.Fatalf("expected prompt profiles to expand after pivot: %#v", result.PromptProfiles)
	}
	if result.TotalSuites != 4 || len(result.RoundResults) != 4 {
		t.Fatalf("expected campaign to consume remaining rounds after pivot: %#v", result.RoundResults)
	}
	if result.RoundResults[2].CoverageGainStats.WeightedScore <= 0 || result.RoundResults[3].CoverageGainStats.WeightedScore <= 0 {
		t.Fatalf("expected post-pivot rounds to regain positive coverage: round3=%#v round4=%#v", result.RoundResults[2].CoverageGainStats, result.RoundResults[3].CoverageGainStats)
	}
}
