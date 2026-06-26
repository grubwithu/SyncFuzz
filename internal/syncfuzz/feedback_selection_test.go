package syncfuzz

import (
	"path/filepath"
	"testing"
)

func TestSelectScheduleMatrixCandidatesUsesFeedbackRankingAndLimit(t *testing.T) {
	matrix := &ScheduleMatrix{
		SchemaVersion:   "syncfuzz.schedule-matrix.v1",
		Cases:           []string{"case-a"},
		TimingProfiles:  []string{"baseline", "tight", "wide"},
		TotalCandidates: 3,
		Candidates: []ScheduleCandidate{
			{CandidateID: "case-a/primitive-a/baseline"},
			{CandidateID: "case-a/primitive-a/tight"},
			{CandidateID: "case-a/primitive-a/wide"},
		},
	}
	feedbackPath := filepath.Join(t.TempDir(), "matrix-result.json")
	if err := writeJSON(feedbackPath, SuiteMatrixResult{
		SchemaVersion: "syncfuzz.matrix-result.v1",
		CandidateSummaries: []MatrixCandidateSummary{
			{CandidateID: "case-a/primitive-a/tight", Score: 5, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: "case-a/primitive-a/wide", Score: 20, ReproducibilityRate: 1, Confirmed: 1},
			{CandidateID: "case-a/primitive-a/baseline", Score: 1, ReproducibilityRate: 1, Confirmed: 1},
		},
	}); err != nil {
		t.Fatalf("write feedback: %v", err)
	}

	selected, err := selectScheduleMatrixCandidates(matrix, FeedbackSelectionOptions{
		FeedbackFrom: feedbackPath,
		Limit:        2,
	})
	if err != nil {
		t.Fatalf("selectScheduleMatrixCandidates failed: %v", err)
	}
	if selected.TotalCandidates != 2 || len(selected.Candidates) != 2 {
		t.Fatalf("expected two selected candidates: %#v", selected)
	}
	if selected.Candidates[0].CandidateID != "case-a/primitive-a/wide" {
		t.Fatalf("expected highest feedback score first: %#v", selected.Candidates)
	}
	if selected.Candidates[1].CandidateID != "case-a/primitive-a/tight" {
		t.Fatalf("expected second feedback score next: %#v", selected.Candidates)
	}
}

func TestSelectScheduleMatrixCandidatesRejectsMissingFeedback(t *testing.T) {
	_, err := selectScheduleMatrixCandidates(&ScheduleMatrix{}, FeedbackSelectionOptions{
		FeedbackFrom: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err == nil {
		t.Fatalf("expected missing feedback error")
	}
}
