package scheduler

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
)

func TestWriteListShowCorpus(t *testing.T) {
	corpusDir := t.TempDir()
	suite := &SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []SuiteDiscovery{
			{
				Kind:               "new-signature",
				Key:                "<x>",
				CaseName:           "action-replay",
				Iteration:          1,
				RunID:              "run-test",
				PairID:             "pair-test",
				ControlRunID:       "control-run-test",
				FaultRunID:         "fault-run-test",
				CandidateID:        "action-replay/external-api-commit/baseline",
				Differential:       true,
				SecurityRelevant:   true,
				DifferentialReport: "runs/suite-test/pair-test/differential-report.json",
				PrimitiveID:        "external-api-commit",
				Signature: core.MismatchSignature{
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
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}

	listed, err := corpus.ListCorpus(corpusDir, 0)
	if err != nil {
		t.Fatalf("corpus.ListCorpus failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one listed entry, got %d", len(listed))
	}

	shown, err := corpus.ShowCorpusEntry(corpusDir, entries[0].EntryID)
	if err != nil {
		t.Fatalf("corpus.ShowCorpusEntry failed: %v", err)
	}
	if shown.EntryID != entries[0].EntryID {
		t.Fatalf("expected entry id %q, got %q", entries[0].EntryID, shown.EntryID)
	}
	if shown.PairID != "pair-test" || !shown.SecurityRelevant {
		t.Fatalf("expected pair metadata to round-trip: %#v", shown)
	}
	if shown.CandidateID != "action-replay/external-api-commit/baseline" || shown.PrimitiveID != "external-api-commit" {
		t.Fatalf("expected candidate metadata to round-trip: %#v", shown)
	}

	prefixShown, err := corpus.ShowCorpusEntry(corpusDir, "new-signature-action-replay")
	if err != nil {
		t.Fatalf("corpus.ShowCorpusEntry by prefix failed: %v", err)
	}
	if prefixShown.EntryID != entries[0].EntryID {
		t.Fatalf("expected prefix match %q, got %q", entries[0].EntryID, prefixShown.EntryID)
	}
}

func TestListCorpusMissingIndexReturnsEmpty(t *testing.T) {
	entries, err := corpus.ListCorpus(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("corpus.ListCorpus failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
}
