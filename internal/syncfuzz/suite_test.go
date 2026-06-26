package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestRunSuiteRunsSelectedCases(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunSuite(context.Background(), SuiteOptions{
		OutDir:    tmp,
		CorpusDir: tmp + "/corpus",
		Repeat:    2,
		Cases:     []string{"action-replay"},
	})
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}

	if result.TotalRuns != 2 {
		t.Fatalf("expected 2 total runs, got %d", result.TotalRuns)
	}
	if result.Confirmed != 2 {
		t.Fatalf("expected 2 confirmed runs, got %d", result.Confirmed)
	}
	if result.Errors != 0 {
		t.Fatalf("expected no errors, got %d", result.Errors)
	}
	if result.UniqueSignatures != 1 {
		t.Fatalf("expected one unique signature, got %d", result.UniqueSignatures)
	}
	if result.UniqueStateClasses != 1 {
		t.Fatalf("expected one unique state class, got %d", result.UniqueStateClasses)
	}
	if result.UniqueImpacts != 1 {
		t.Fatalf("expected one unique impact, got %d", result.UniqueImpacts)
	}
	if !result.Results[0].Interesting {
		t.Fatalf("expected first result to be interesting")
	}
	if result.Results[1].Interesting {
		t.Fatalf("expected repeated identical result not to be interesting")
	}
	if len(result.Discoveries) != 3 {
		t.Fatalf("expected three discoveries, got %d", len(result.Discoveries))
	}
	if len(result.CorpusEntries) != 3 {
		t.Fatalf("expected three corpus entries, got %d", len(result.CorpusEntries))
	}
	if result.ArtifactDir == "" {
		t.Fatalf("expected suite artifact directory")
	}
}

func TestRunSuiteDifferentialPairs(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunSuite(context.Background(), SuiteOptions{
		OutDir:       tmp,
		CorpusDir:    tmp + "/corpus",
		Cases:        []string{"action-replay"},
		Differential: true,
	})
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}

	if !result.Differential {
		t.Fatalf("expected differential suite")
	}
	if result.TotalRuns != 1 {
		t.Fatalf("expected one suite item, got %d", result.TotalRuns)
	}
	if result.Confirmed != 1 {
		t.Fatalf("expected one confirmed differential, got %d", result.Confirmed)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected one result")
	}
	item := result.Results[0]
	if item.PairID == "" || item.ControlRunID == "" || item.FaultRunID == "" {
		t.Fatalf("expected pair metadata: %#v", item)
	}
	if !item.Differential || !item.SecurityRelevant {
		t.Fatalf("expected security-relevant differential result: %#v", item)
	}
	if item.DifferentialReport == "" {
		t.Fatalf("expected differential report path")
	}
	if len(result.Discoveries) != 3 {
		t.Fatalf("expected three discoveries, got %d", len(result.Discoveries))
	}
	if result.Discoveries[0].PairID != item.PairID {
		t.Fatalf("expected discovery pair id %q, got %q", item.PairID, result.Discoveries[0].PairID)
	}
	if len(result.CorpusEntries) != 3 {
		t.Fatalf("expected three corpus entries, got %d", len(result.CorpusEntries))
	}
	if result.CorpusEntries[0].PairID != item.PairID || !result.CorpusEntries[0].SecurityRelevant {
		t.Fatalf("expected corpus pair metadata: %#v", result.CorpusEntries[0])
	}
}

func TestRunSuiteMatrixModeAnnotatesCandidates(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunSuite(context.Background(), SuiteOptions{
		OutDir:           tmp,
		CorpusDir:        tmp + "/corpus",
		Cases:            []string{"action-replay"},
		Matrix:           true,
		TimingProfileIDs: []string{"baseline", "tight"},
	})
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}

	if result.SchedulerMode != suiteSchedulerMatrix {
		t.Fatalf("expected matrix scheduler, got %q", result.SchedulerMode)
	}
	if result.TotalCandidates != 2 || result.TotalRuns != 2 {
		t.Fatalf("expected two matrix runs, got candidates=%d runs=%d", result.TotalCandidates, result.TotalRuns)
	}
	if result.ScheduleMatrix == "" || result.MatrixResult == "" {
		t.Fatalf("expected matrix artifacts: %#v", result)
	}
	if _, err := os.Stat(result.ScheduleMatrix); err != nil {
		t.Fatalf("expected schedule matrix artifact: %v", err)
	}
	if _, err := os.Stat(result.MatrixResult); err != nil {
		t.Fatalf("expected matrix result artifact: %v", err)
	}

	for _, item := range result.Results {
		if item.CandidateID == "" || item.PrimitiveID != "external-api-commit" {
			t.Fatalf("expected candidate metadata: %#v", item)
		}
		if item.FaultPlanID != "action-replay/p5-dropped-receipt" {
			t.Fatalf("expected fault plan to be carried by candidate, got %q", item.FaultPlanID)
		}
	}
	if len(result.Discoveries) == 0 || result.Discoveries[0].CandidateID == "" {
		t.Fatalf("expected discoveries to include candidate metadata")
	}
	if len(result.CorpusEntries) == 0 || result.CorpusEntries[0].PrimitiveID != "external-api-commit" {
		t.Fatalf("expected corpus entries to include primitive metadata")
	}
	if len(result.CandidateSummaries) != 2 {
		t.Fatalf("expected two candidate summaries, got %d", len(result.CandidateSummaries))
	}
	if result.CandidateSummaries[0].Rank != 1 || result.CandidateSummaries[0].Score <= result.CandidateSummaries[1].Score {
		t.Fatalf("expected ranked candidate summaries: %#v", result.CandidateSummaries)
	}
	if result.CandidateSummaries[0].Status != "confirmed" || result.CandidateSummaries[0].ReproducibilityRate != 1 {
		t.Fatalf("expected confirmed top candidate: %#v", result.CandidateSummaries[0])
	}
	if result.CandidateSummaries[0].AvgArtifactBytes == 0 || result.CandidateSummaries[0].AvgArtifactFiles == 0 {
		t.Fatalf("expected artifact cost metrics in top candidate: %#v", result.CandidateSummaries[0])
	}

	raw, err := os.ReadFile(result.MatrixResult)
	if err != nil {
		t.Fatalf("read matrix result: %v", err)
	}
	var matrixResult SuiteMatrixResult
	if err := json.Unmarshal(raw, &matrixResult); err != nil {
		t.Fatalf("decode matrix result: %v", err)
	}
	if matrixResult.SchemaVersion != "syncfuzz.matrix-result.v1" || len(matrixResult.Results) != 2 {
		t.Fatalf("unexpected matrix result: %#v", matrixResult)
	}
	if len(matrixResult.CandidateSummaries) != 2 || matrixResult.CandidateSummaries[0].Rank != 1 {
		t.Fatalf("expected matrix result candidate summaries: %#v", matrixResult.CandidateSummaries)
	}
}

func TestRunSuiteMatrixModeHonorsCandidateLimit(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunSuite(context.Background(), SuiteOptions{
		OutDir:           tmp,
		CorpusDir:        tmp + "/corpus",
		Cases:            []string{"action-replay"},
		Matrix:           true,
		TimingProfileIDs: []string{"baseline", "tight"},
		CandidateLimit:   1,
	})
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}

	if result.SchedulerMode != suiteSchedulerFeedback {
		t.Fatalf("expected feedback scheduler when candidate limit is set, got %q", result.SchedulerMode)
	}
	if result.OriginalCandidates != 2 || result.TotalCandidates != 1 || result.TotalRuns != 1 {
		t.Fatalf("expected limited matrix run, got original=%d candidates=%d runs=%d", result.OriginalCandidates, result.TotalCandidates, result.TotalRuns)
	}
	if len(result.Results) != 1 || result.Results[0].CandidateID == "" {
		t.Fatalf("expected one candidate result: %#v", result.Results)
	}
}

func TestRunSuiteRejectsUnknownCase(t *testing.T) {
	_, err := RunSuite(context.Background(), SuiteOptions{
		OutDir: t.TempDir(),
		Cases:  []string{"not-a-case"},
	})
	if err == nil {
		t.Fatalf("expected unknown case error")
	}
}
