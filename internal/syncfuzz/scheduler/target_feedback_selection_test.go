package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestSelectTargetMatrixCandidatesPrefersDistinctTasksBeforeAlternatePromptProfiles(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileAuditID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{Limit: 3})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 3 {
		t.Fatalf("expected 3 selected candidates, got %d", len(selected.Candidates))
	}

	wantTasks := []string{"task-a", "task-b", "task-c"}
	for i, taskID := range wantTasks {
		if selected.Candidates[i].TaskID != taskID {
			t.Fatalf("candidate %d task mismatch: got %q want %q", i, selected.Candidates[i].TaskID, taskID)
		}
		if selected.Candidates[i].PromptProfileID != target.TargetPromptProfileBaselineID {
			t.Fatalf("candidate %d prompt mismatch: got %q want %q", i, selected.Candidates[i].PromptProfileID, target.TargetPromptProfileBaselineID)
		}
	}
}

func TestSelectTargetMatrixCandidatesAppendsDiverseUnseenCandidatesAfterFeedback(t *testing.T) {
	matrix := &TargetScheduleMatrix{
		SchemaVersion: "syncfuzz.target-schedule-matrix.v1",
		TargetID:      "test-target",
		Candidates: []TargetScheduleCandidate{
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-a", target.TargetPromptProfileAuditID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileBaselineID),
			testTargetScheduleCandidate("task-b", target.TargetPromptProfileWorkflowID),
			testTargetScheduleCandidate("task-c", target.TargetPromptProfileBaselineID),
		},
	}
	matrix.TotalCandidates = len(matrix.Candidates)

	tmp := t.TempDir()
	feedbackPath := filepath.Join(tmp, "feedback.json")
	if err := core.WriteJSON(feedbackPath, &TargetMatrixResult{
		SchemaVersion: "syncfuzz.target-matrix-result.v1",
		CandidateSummaries: []TargetCandidateSummary{
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileBaselineID), Score: 9, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileWorkflowID), Score: 8, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: targetScheduleCandidateID("test-target", "task-a", target.TargetPromptProfileAuditID), Score: 7, ReproducibilityRate: 1, Confirmed: 1},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
		FeedbackFrom: feedbackPath,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("selectTargetMatrixCandidates failed: %v", err)
	}
	if len(selected.Candidates) != 5 {
		t.Fatalf("expected 5 selected candidates, got %d", len(selected.Candidates))
	}

	for i, profileID := range []string{target.TargetPromptProfileBaselineID, target.TargetPromptProfileWorkflowID, target.TargetPromptProfileAuditID} {
		if selected.Candidates[i].TaskID != "task-a" || selected.Candidates[i].PromptProfileID != profileID {
			t.Fatalf("ranked candidate %d mismatch: got task=%q profile=%q", i, selected.Candidates[i].TaskID, selected.Candidates[i].PromptProfileID)
		}
	}
	if selected.Candidates[3].TaskID != "task-b" || selected.Candidates[3].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected first unseen candidate to be baseline of a new task, got task=%q profile=%q", selected.Candidates[3].TaskID, selected.Candidates[3].PromptProfileID)
	}
	if selected.Candidates[4].TaskID != "task-c" || selected.Candidates[4].PromptProfileID != target.TargetPromptProfileBaselineID {
		t.Fatalf("expected second unseen candidate to expand to another task baseline, got task=%q profile=%q", selected.Candidates[4].TaskID, selected.Candidates[4].PromptProfileID)
	}
}

func testTargetScheduleCandidate(taskID string, profileID string) TargetScheduleCandidate {
	return TargetScheduleCandidate{
		CandidateID:            targetScheduleCandidateID("test-target", taskID, profileID),
		TargetID:               "test-target",
		TaskID:                 taskID,
		PromptProfileID:        profileID,
		ContractRuleID:         "shared-rule",
		ContractProfileID:      "test-profile",
		ContractExpectation:    target.TargetContractExpectationReset,
		ContractSourceStrength: target.TargetContractSourceStrengthImplicit,
		StateSurface:           "workspace.file",
		LifecycleEdge:          "checkpoint->fork",
	}
}
