package target

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
)

func TestCompareTargetRunsReportsTargetOnlyCheckpointState(t *testing.T) {
	tmp := t.TempDir()
	controlDir := filepath.Join(tmp, "control")
	targetDir := filepath.Join(tmp, "target")
	writePairRunArtifact(t, controlDir, "control-run", core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}}}, core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 10, Name: "bash", RawCmdline: "bash agent"}}})
	writePairRunArtifact(t, targetDir, "target-run", core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}, {Path: "residue.txt", Type: "file", SHA256: "target"}}}, core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 11, Name: "bash", RawCmdline: "bash agent"}, {PID: 12, Name: "listener", RawCmdline: "listener --socket residue.sock"}}})

	result, err := CompareTargetRuns(TargetPairDifferentialOptions{ControlRunDir: controlDir, TargetRunDir: targetDir})
	if err != nil {
		t.Fatalf("CompareTargetRuns failed: %v", err)
	}
	if result.SchemaVersion != TargetPairDifferentialSchemaVersion || result.QueryID != "pair-query" || len(result.Checkpoints) != 3 {
		t.Fatalf("unexpected pair differential: %#v", result)
	}
	for _, checkpoint := range result.Checkpoints {
		if !targetContainsString(checkpoint.Filesystem.TargetOnly, "residue.txt") {
			t.Fatalf("expected target-only residue at %s: %#v", checkpoint.Point, checkpoint)
		}
		if len(checkpoint.Processes.TargetOnly) != 1 || checkpoint.Processes.TargetOnly[0].Name != "listener" {
			t.Fatalf("expected target-only listener at %s: %#v", checkpoint.Point, checkpoint)
		}
	}
	if len(result.Evidence) != 6 {
		t.Fatalf("expected filesystem and process evidence for each checkpoint: %#v", result.Evidence)
	}
	if _, err := os.Stat(filepath.Join(targetDir, TargetPairDifferentialArtifact)); err != nil {
		t.Fatalf("expected pair differential artifact: %v", err)
	}
}

func TestCompareTargetRunsRejectsDifferentQueries(t *testing.T) {
	tmp := t.TempDir()
	controlDir := filepath.Join(tmp, "control")
	targetDir := filepath.Join(tmp, "target")
	writePairRunArtifact(t, controlDir, "control-run", core.Snapshot{}, core.ProcessSnapshot{})
	writePairRunArtifact(t, targetDir, "target-run", core.Snapshot{}, core.ProcessSnapshot{})
	differentialPath := filepath.Join(targetDir, TargetCheckpointDifferentialArtifact)
	differential, err := readTargetPairJSON[TargetCheckpointDifferential](differentialPath)
	if err != nil {
		t.Fatalf("read differential: %v", err)
	}
	differential.QueryID = "different-query"
	if err := core.WriteJSON(differentialPath, differential); err != nil {
		t.Fatalf("write differential: %v", err)
	}
	if _, err := CompareTargetRuns(TargetPairDifferentialOptions{ControlRunDir: controlDir, TargetRunDir: targetDir}); err == nil {
		t.Fatal("expected different queries to be rejected")
	}
}

func writePairRunArtifact(t *testing.T, runDir string, runID string, snapshot core.Snapshot, processes core.ProcessSnapshot) {
	t.Helper()
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	if err := core.WriteJSON(filepath.Join(runDir, TargetResultArtifact), TargetRunResult{RunID: runID}); err != nil {
		t.Fatalf("write result: %v", err)
	}
	checkpoints := []TargetCheckpointState{
		{Point: observation.ObservationAfterPlant, RunnerPhase: "P4", FilesystemArtifact: "after-plant.json", ProcessArtifact: "after-plant-process.json"},
		{Point: observation.ObservationAfterRecovery, RunnerPhase: "P6", FilesystemArtifact: "after-recovery.json", ProcessArtifact: "after-recovery-process.json"},
		{Point: observation.ObservationAfterActivation, RunnerPhase: "P7", FilesystemArtifact: "after-activation.json", ProcessArtifact: "after-activation-process.json"},
	}
	differential := TargetCheckpointDifferential{
		SchemaVersion: TargetCheckpointDifferentialSchemaVersion,
		QueryID:       "pair-query",
		Checkpoints:   checkpoints,
	}
	if err := core.WriteJSON(filepath.Join(runDir, TargetCheckpointDifferentialArtifact), differential); err != nil {
		t.Fatalf("write differential: %v", err)
	}
	for _, checkpoint := range checkpoints {
		if err := core.WriteJSON(filepath.Join(runDir, checkpoint.FilesystemArtifact), snapshot); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		if err := core.WriteJSON(filepath.Join(runDir, checkpoint.ProcessArtifact), processes); err != nil {
			t.Fatalf("write processes: %v", err)
		}
	}
}
