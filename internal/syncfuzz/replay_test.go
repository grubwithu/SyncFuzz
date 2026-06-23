package syncfuzz

import (
	"context"
	"testing"
)

func TestReplayCorpusEntryReproducesSignature(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := tmp + "/corpus"
	suite := &SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []SuiteDiscovery{
			{
				Kind:      "new-signature",
				Key:       "<replay, after-external-commit, external-effect, duplicate-create, missing-receipt, forgotten-external-effect>",
				CaseName:  "action-replay",
				Iteration: 1,
				RunID:     "run-test",
				Signature: MismatchSignature{
					LifecycleEvent: "replay",
					FaultPhase:     "after-external-commit",
					StateClass:     "external-effect",
					Operation:      "duplicate-create",
					Relation:       "missing-receipt",
					Impact:         "forgotten-external-effect",
				},
				ArtifactDir: "runs/suite-test/run-test",
			},
		},
	}
	entries, err := WriteCorpus(corpusDir, suite)
	if err != nil {
		t.Fatalf("WriteCorpus failed: %v", err)
	}

	result, err := ReplayCorpusEntry(context.Background(), ReplayOptions{
		CorpusDir: corpusDir,
		EntryID:   entries[0].EntryID,
		OutDir:    tmp + "/runs",
	})
	if err != nil {
		t.Fatalf("ReplayCorpusEntry failed: %v", err)
	}

	if !result.Reproduced {
		t.Fatalf("expected replay to reproduce signature")
	}
	if !result.SignatureMatched {
		t.Fatalf("expected replay signature to match")
	}
	if result.RunArtifactDir == "" {
		t.Fatalf("expected run artifact directory")
	}
}
