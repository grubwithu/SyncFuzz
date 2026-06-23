package syncfuzz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyCorpusReportsReproductionAndSignatureDrift(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")
	suite := &SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []SuiteDiscovery{
			{
				Kind:      "new-signature",
				Key:       "<replay, after-external-commit, external-effect, duplicate-create, missing-receipt, forgotten-external-effect>",
				CaseName:  "action-replay",
				Iteration: 1,
				RunID:     "run-good",
				Signature: MismatchSignature{
					LifecycleEvent: "replay",
					FaultPhase:     "after-external-commit",
					StateClass:     "external-effect",
					Operation:      "duplicate-create",
					Relation:       "missing-receipt",
					Impact:         "forgotten-external-effect",
				},
				ArtifactDir: "runs/suite-test/run-good",
			},
			{
				Kind:      "new-impact",
				Key:       "wrong-impact",
				CaseName:  "action-replay",
				Iteration: 1,
				RunID:     "run-drift",
				Signature: MismatchSignature{
					LifecycleEvent: "replay",
					FaultPhase:     "after-external-commit",
					StateClass:     "external-effect",
					Operation:      "duplicate-create",
					Relation:       "missing-receipt",
					Impact:         "wrong-impact",
				},
				ArtifactDir: "runs/suite-test/run-drift",
			},
		},
	}
	if _, err := WriteCorpus(corpusDir, suite); err != nil {
		t.Fatalf("WriteCorpus failed: %v", err)
	}

	result, err := VerifyCorpus(context.Background(), VerifyOptions{
		CorpusDir: corpusDir,
		OutDir:    filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("VerifyCorpus failed: %v", err)
	}

	if result.TotalEntries != 2 {
		t.Fatalf("expected 2 entries, got %d", result.TotalEntries)
	}
	if result.Reproduced != 1 {
		t.Fatalf("expected 1 reproduced entry, got %d", result.Reproduced)
	}
	if result.SignatureDrift != 1 {
		t.Fatalf("expected 1 signature drift, got %d", result.SignatureDrift)
	}
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed entry, got %d", result.Failed)
	}
	if result.Errors != 0 {
		t.Fatalf("expected no errors, got %d", result.Errors)
	}
	if result.ReproducibilityRate != 0.5 {
		t.Fatalf("expected 0.5 reproducibility rate, got %f", result.ReproducibilityRate)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, "verification-result.json")); err != nil {
		t.Fatalf("expected verification-result.json: %v", err)
	}
}

func TestVerifyCorpusEmptyCorpusWritesReport(t *testing.T) {
	tmp := t.TempDir()
	result, err := VerifyCorpus(context.Background(), VerifyOptions{
		CorpusDir: filepath.Join(tmp, "empty-corpus"),
		OutDir:    filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("VerifyCorpus failed: %v", err)
	}
	if result.TotalEntries != 0 {
		t.Fatalf("expected empty corpus, got %d entries", result.TotalEntries)
	}
	if result.ReproducibilityRate != 0 {
		t.Fatalf("expected zero reproducibility rate, got %f", result.ReproducibilityRate)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, "verification-result.json")); err != nil {
		t.Fatalf("expected verification-result.json: %v", err)
	}
}
