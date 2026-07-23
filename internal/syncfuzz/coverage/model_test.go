package coverage

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

func TestUniqueTuplesDoNotInflateCoverageBySeed(t *testing.T) {
	record := CoverageRecord{
		SchemaVersion:      SchemaVersion,
		SeedID:             "seed-a",
		ObjectiveID:        "ipc.unix-listener.survival",
		Family:             profiling.StateFamilyIPC,
		Operation:          "listen",
		Lifetime:           "survive-tool-return",
		ResourceRelation:   "fixed-path-served-by-descendant",
		Boundary:           "fork",
		CheckpointRelation: "before-after",
		Outcome:            OutcomeResidual,
	}
	second := record
	second.SeedID = "seed-b"
	unique, err := UniqueTuples([]CoverageRecord{second, record})
	if err != nil {
		t.Fatalf("UniqueTuples returned error: %v", err)
	}
	if len(unique) != 1 || unique[0].SeedID != "seed-a" {
		t.Fatalf("expected one deterministic tuple representative, got %#v", unique)
	}
}
