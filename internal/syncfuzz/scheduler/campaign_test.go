package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCampaignFeedsBackAcrossRounds(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunCampaign(context.Background(), CampaignOptions{
		OutDir:           tmp,
		CorpusDir:        filepath.Join(tmp, "corpus"),
		Rounds:           2,
		CandidateLimit:   1,
		Cases:            []string{"action-replay"},
		TimingProfileIDs: []string{"baseline", "tight"},
	})
	if err != nil {
		t.Fatalf("RunCampaign failed: %v", err)
	}

	if result.SchemaVersion != "syncfuzz.campaign-result.v1" {
		t.Fatalf("unexpected schema version %q", result.SchemaVersion)
	}
	if result.TotalSuites != 2 || len(result.RoundResults) != 2 {
		t.Fatalf("expected two campaign rounds: %#v", result)
	}
	if result.TotalRuns != 2 {
		t.Fatalf("expected two budgeted campaign runs, got %d runs", result.TotalRuns)
	}
	if result.UniqueCandidates != 2 || result.RepeatedCandidates != 0 {
		t.Fatalf("expected cross-round candidate de-duplication: unique=%d repeated=%d", result.UniqueCandidates, result.RepeatedCandidates)
	}
	if result.RoundResults[0].SchedulerMode != suiteSchedulerFeedback || result.RoundResults[0].TotalCandidates != 1 {
		t.Fatalf("expected first round to run budgeted matrix: %#v", result.RoundResults[0])
	}
	if result.RoundResults[1].SchedulerMode != suiteSchedulerFeedback || result.RoundResults[1].TotalCandidates != 1 {
		t.Fatalf("expected second round to run feedback budget without repeating first candidate: %#v", result.RoundResults[1])
	}
	if result.RoundResults[1].FeedbackFrom != result.RoundResults[0].MatrixResult {
		t.Fatalf("expected second round to consume first round feedback")
	}
	if result.RoundResults[1].TopCandidate == nil {
		t.Fatalf("expected top candidate summary")
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, campaignResultArtifact)); err != nil {
		t.Fatalf("expected campaign result artifact: %v", err)
	}
}
