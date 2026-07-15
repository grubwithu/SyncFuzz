package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
)

func TestSummarizeTargetCandidatesRecordsActivationProgress(t *testing.T) {
	summaries := summarizeTargetCandidates([]TargetSuiteRunResult{
		{
			CandidateID:     "target/task-a",
			TargetID:        "target",
			TaskID:          "task-a",
			OutcomeCategory: corpus.TargetObservationActivationNotTriggered,
			ActivationStage: TargetActivationStageActivationPending,
		},
		{
			CandidateID:     "target/task-b",
			TargetID:        "target",
			TaskID:          "task-b",
			OutcomeCategory: corpus.TargetObservationLifecycleNotTriggered,
			ActivationStage: TargetActivationStageLifecyclePending,
		},
	})
	if len(summaries) != 2 {
		t.Fatalf("expected two candidate summaries, got %#v", summaries)
	}
	if summaries[0].CandidateID != "target/task-a" {
		t.Fatalf("expected activation-pending candidate to rank first, got %#v", summaries)
	}
	if summaries[0].MaxActivationStage != TargetActivationStageActivationPending || summaries[0].ActivationProgressScore <= summaries[1].ActivationProgressScore {
		t.Fatalf("expected activation progress score to reflect furthest stage: %#v", summaries)
	}
}
