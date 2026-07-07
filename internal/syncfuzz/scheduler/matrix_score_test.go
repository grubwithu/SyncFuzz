package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestSummarizeMatrixCandidatesRanksNovelConfirmedRuns(t *testing.T) {
	results := []SuiteCaseResult{
		{
			CandidateID:     "case-a/primitive-a/baseline",
			CaseName:        "case-a",
			PrimitiveID:     "primitive-a",
			TimingProfileID: "baseline",
			Confirmed:       true,
			Interesting:     true,
			Score:           18,
			DurationMillis:  25,
			ArtifactBytes:   4096,
			ArtifactFiles:   12,
			Signature: core.MismatchSignature{
				LifecycleEvent: "replay",
				FaultPhase:     "after-effect",
				StateClass:     "external-effect",
				Operation:      "duplicate-create",
				Relation:       "missing-receipt",
				Impact:         "forgotten-external-effect",
			},
		},
		{
			CandidateID:     "case-a/primitive-a/tight",
			CaseName:        "case-a",
			PrimitiveID:     "primitive-a",
			TimingProfileID: "tight",
			Confirmed:       true,
			DurationMillis:  10,
			ArtifactBytes:   1024,
			ArtifactFiles:   3,
		},
		{
			CandidateID:     "case-a/primitive-a/wide",
			CaseName:        "case-a",
			PrimitiveID:     "primitive-a",
			TimingProfileID: "wide",
			Error:           "boom",
		},
	}

	summaries := summarizeMatrixCandidates(results)
	if len(summaries) != 3 {
		t.Fatalf("expected three summaries, got %d", len(summaries))
	}
	if summaries[0].CandidateID != "case-a/primitive-a/baseline" || summaries[0].Rank != 1 {
		t.Fatalf("expected novel confirmed candidate first: %#v", summaries)
	}
	if summaries[0].Score <= summaries[1].Score {
		t.Fatalf("expected first candidate to outrank second: %#v", summaries)
	}
	if summaries[2].Status != "error" || summaries[2].Score >= summaries[1].Score {
		t.Fatalf("expected errored candidate to be penalized: %#v", summaries)
	}
	if len(summaries[0].Signatures) != 1 || summaries[0].StateClasses[0] != "external-effect" {
		t.Fatalf("expected signature dimensions to be retained: %#v", summaries[0])
	}
	if summaries[0].AvgDurationMillis != 25 || summaries[0].AvgArtifactBytes != 4096 || summaries[0].AvgArtifactFiles != 12 {
		t.Fatalf("expected cost metrics to be retained: %#v", summaries[0])
	}
}

func TestSummarizeMatrixCandidatesUsesCostAsTieBreaker(t *testing.T) {
	results := []SuiteCaseResult{
		{
			CandidateID:     "case-a/primitive-a/slow",
			CaseName:        "case-a",
			PrimitiveID:     "primitive-a",
			TimingProfileID: "slow",
			Confirmed:       true,
			DurationMillis:  2500,
			ArtifactBytes:   4 * 1024 * 1024,
			ArtifactFiles:   250,
		},
		{
			CandidateID:     "case-a/primitive-a/fast",
			CaseName:        "case-a",
			PrimitiveID:     "primitive-a",
			TimingProfileID: "fast",
			Confirmed:       true,
			DurationMillis:  100,
			ArtifactBytes:   1024,
			ArtifactFiles:   5,
		},
	}

	summaries := summarizeMatrixCandidates(results)
	if len(summaries) != 2 {
		t.Fatalf("expected two summaries, got %d", len(summaries))
	}
	if summaries[0].CandidateID != "case-a/primitive-a/fast" {
		t.Fatalf("expected lower-cost candidate first: %#v", summaries)
	}
	if summaries[1].CostPenalty <= summaries[0].CostPenalty {
		t.Fatalf("expected slow candidate to carry a larger cost penalty: %#v", summaries)
	}
}
