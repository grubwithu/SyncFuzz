package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestReadTargetMaterializationForSeedDefaultsBesideRecordedPlan(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, target.TargetEnvironmentMaterializationArtifact), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	materialization, err := readTargetMaterializationForSeed(objective.StateSeed{
		SeedID:                "state-seed:test",
		RecordedPlanArtifact: filepath.Join(directory, target.TargetTaskArtifact),
	}, "")
	if err != nil {
		t.Fatalf("read default target materialization: %v", err)
	}
	if materialization.SchemaVersion != "" {
		t.Fatalf("unexpected materialization read: %#v", materialization)
	}
}

