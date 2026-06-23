package syncfuzz

import "testing"

func TestWriteListShowCorpus(t *testing.T) {
	corpusDir := t.TempDir()
	suite := &SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []SuiteDiscovery{
			{
				Kind:      "new-signature",
				Key:       "<x>",
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
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}

	listed, err := ListCorpus(corpusDir, 0)
	if err != nil {
		t.Fatalf("ListCorpus failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one listed entry, got %d", len(listed))
	}

	shown, err := ShowCorpusEntry(corpusDir, entries[0].EntryID)
	if err != nil {
		t.Fatalf("ShowCorpusEntry failed: %v", err)
	}
	if shown.EntryID != entries[0].EntryID {
		t.Fatalf("expected entry id %q, got %q", entries[0].EntryID, shown.EntryID)
	}

	prefixShown, err := ShowCorpusEntry(corpusDir, "new-signature-action-replay")
	if err != nil {
		t.Fatalf("ShowCorpusEntry by prefix failed: %v", err)
	}
	if prefixShown.EntryID != entries[0].EntryID {
		t.Fatalf("expected prefix match %q, got %q", entries[0].EntryID, prefixShown.EntryID)
	}
}

func TestListCorpusMissingIndexReturnsEmpty(t *testing.T) {
	entries, err := ListCorpus(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("ListCorpus failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
}
