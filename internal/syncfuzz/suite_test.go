package syncfuzz

import (
	"context"
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

func TestRunSuiteRejectsUnknownCase(t *testing.T) {
	_, err := RunSuite(context.Background(), SuiteOptions{
		OutDir: t.TempDir(),
		Cases:  []string{"not-a-case"},
	})
	if err == nil {
		t.Fatalf("expected unknown case error")
	}
}
