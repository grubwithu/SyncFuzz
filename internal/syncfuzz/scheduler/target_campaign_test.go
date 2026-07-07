package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetCampaignResultArtifact)); err != nil {
		t.Fatalf("expected target campaign result artifact: %v", err)
	}
}
