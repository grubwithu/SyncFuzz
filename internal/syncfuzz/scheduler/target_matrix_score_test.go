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

func TestTargetCandidateScorePrefersCompliantActivationOverTaskDrift(t *testing.T) {
	compliant := TargetCandidateSummary{
		Confirmed:               1,
		ContractViolations:      1,
		ComplianceCompliant:     1,
		ActivationProgressScore: 5,
		ActivationReached:       1,
	}
	taskDrift := TargetCandidateSummary{
		Confirmed:          3,
		ComplianceViolated: 3,
		OutcomeSummaries: []TargetSuiteOutcomeStats{
			{Category: corpus.TargetObservationTaskNoncompliant, TotalRuns: 3},
		},
	}

	if targetCandidateScore(compliant) <= targetCandidateScore(taskDrift) {
		t.Fatalf("expected compliant activation to outrank task drift: compliant=%d task_drift=%d", targetCandidateScore(compliant), targetCandidateScore(taskDrift))
	}
}

func TestTargetCandidateScorePenalizesExecutionNotReached(t *testing.T) {
	completed := TargetCandidateSummary{
		Confirmed:               1,
		ComplianceCompliant:     1,
		ActivationProgressScore: 5,
		ActivationReached:       1,
	}
	timeoutProne := TargetCandidateSummary{
		Confirmed:           3,
		ComplianceCompliant: 3,
		OutcomeSummaries: []TargetSuiteOutcomeStats{
			{Category: corpus.TargetObservationExecutionNotReached, TotalRuns: 2},
		},
	}

	if targetCandidateScore(completed) <= targetCandidateScore(timeoutProne) {
		t.Fatalf("expected completed activation to outrank timeout-prone candidate: completed=%d timeout_prone=%d", targetCandidateScore(completed), targetCandidateScore(timeoutProne))
	}
}
