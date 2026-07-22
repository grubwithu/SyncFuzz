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
	writePairRunArtifact(t, controlDir, "control-run", core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}, {Path: TargetTaskArtifact, Type: "file", SHA256: "control-task"}}}, core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 10, Name: "bash", RawCmdline: "bash agent"}}})
	writePairRunArtifact(t, targetDir, "target-run", core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}, {Path: TargetTaskArtifact, Type: "file", SHA256: "target-task"}, {Path: "residue.txt", Type: "file", SHA256: "target"}}}, core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 11, Name: "bash", RawCmdline: "bash agent"}, {PID: 12, Name: "listener", RawCmdline: "listener --socket residue.sock"}}})
	writePairRunOracle(t, targetDir, true)

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
	if result.ContractCalibration.Status != TargetPairContractCalibrationUnresolved || result.ContractCalibration.RootCauseEligible {
		t.Fatalf("expected uncalibrated report without contract readings: %#v", result.ContractCalibration)
	}
	if len(result.RootCauseCandidates) != 0 {
		t.Fatalf("expected target-only evidence to remain unpromoted without contract calibration: %#v", result.RootCauseCandidates)
	}
	if _, err := os.Stat(filepath.Join(targetDir, TargetPairDifferentialArtifact)); err != nil {
		t.Fatalf("expected pair differential artifact: %v", err)
	}
}

func TestCompareTargetRunsEmitsRootCauseCandidatesForCalibratedContractViolation(t *testing.T) {
	tmp := t.TempDir()
	controlDir := filepath.Join(tmp, "control")
	targetDir := filepath.Join(tmp, "target")
	controlSnapshot := core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}}}
	targetSnapshot := core.Snapshot{Files: []core.FileEntry{{Path: "shared.txt", Type: "file", SHA256: "same"}, {Path: "residue.txt", Type: "file", SHA256: "target"}}}
	controlProcesses := core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 10, Name: "bash", RawCmdline: "bash agent"}}}
	targetProcesses := core.ProcessSnapshot{Processes: []core.ProcessEntry{{PID: 11, Name: "bash", RawCmdline: "bash agent"}, {PID: 12, Name: "listener", RawCmdline: "listener --socket residue.sock"}}}
	writePairRunArtifact(t, controlDir, "control-run", controlSnapshot, controlProcesses)
	writePairRunArtifact(t, targetDir, "target-run", targetSnapshot, targetProcesses)
	writePairRunContractCalibration(t, controlDir, false, TargetContractStatusConsistent)
	writePairRunContractCalibration(t, targetDir, true, TargetContractStatusViolation)

	result, err := CompareTargetRuns(TargetPairDifferentialOptions{ControlRunDir: controlDir, TargetRunDir: targetDir})
	if err != nil {
		t.Fatalf("CompareTargetRuns failed: %v", err)
	}
	if result.ContractCalibration.Status != TargetPairContractCalibrationCalibrated || !result.ContractCalibration.RootCauseEligible {
		t.Fatalf("expected calibrated contract pairing: %#v", result.ContractCalibration)
	}
	if len(result.RootCauseCandidates) != 6 {
		t.Fatalf("expected one calibrated hypothesis for each target-only evidence candidate: %#v", result.RootCauseCandidates)
	}
	for _, candidate := range result.RootCauseCandidates {
		if candidate.Confidence != "contract-calibrated-evidence-hypothesis" || candidate.ContractProfileID != "langgraph-shell-react.phase5b.v1" || candidate.ContractRuleID != "shell-path-fork-boundary" || candidate.ContractSourceStrength != TargetContractSourceStrengthImplicit {
			t.Fatalf("expected traceable calibrated hypothesis: %#v", candidate)
		}
	}
}

func TestCompareTargetRunsDoesNotPromoteContractConsistentTargetEvidence(t *testing.T) {
	tmp := t.TempDir()
	controlDir := filepath.Join(tmp, "control")
	targetDir := filepath.Join(tmp, "target")
	writePairRunArtifact(t, controlDir, "control-run", core.Snapshot{}, core.ProcessSnapshot{})
	writePairRunArtifact(t, targetDir, "target-run", core.Snapshot{Files: []core.FileEntry{{Path: "residue.txt", Type: "file", SHA256: "target"}}}, core.ProcessSnapshot{})
	writePairRunContractCalibration(t, controlDir, false, TargetContractStatusConsistent)
	writePairRunContractCalibration(t, targetDir, true, TargetContractStatusConsistent)

	result, err := CompareTargetRuns(TargetPairDifferentialOptions{ControlRunDir: controlDir, TargetRunDir: targetDir})
	if err != nil {
		t.Fatalf("CompareTargetRuns failed: %v", err)
	}
	if result.ContractCalibration.Status != TargetPairContractCalibrationUnresolved || result.ContractCalibration.RootCauseEligible {
		t.Fatalf("expected contract-consistent target to remain unpromoted: %#v", result.ContractCalibration)
	}
	if len(result.RootCauseCandidates) != 0 {
		t.Fatalf("expected no root-cause hypotheses for contract-consistent target evidence: %#v", result.RootCauseCandidates)
	}
}

func TestCompareTargetRunsRequiresMatchingContractRule(t *testing.T) {
	tmp := t.TempDir()
	controlDir := filepath.Join(tmp, "control")
	targetDir := filepath.Join(tmp, "target")
	writePairRunArtifact(t, controlDir, "control-run", core.Snapshot{}, core.ProcessSnapshot{})
	writePairRunArtifact(t, targetDir, "target-run", core.Snapshot{Files: []core.FileEntry{{Path: "residue.txt", Type: "file", SHA256: "target"}}}, core.ProcessSnapshot{})
	writePairRunContractCalibration(t, controlDir, false, TargetContractStatusConsistent)
	writePairRunContractCalibration(t, targetDir, true, TargetContractStatusViolation)
	updatePairRunResult(t, controlDir, func(result *TargetRunResult) {
		result.ContractInterpretation.RuleID = "different-contract-rule"
	})

	result, err := CompareTargetRuns(TargetPairDifferentialOptions{ControlRunDir: controlDir, TargetRunDir: targetDir})
	if err != nil {
		t.Fatalf("CompareTargetRuns failed: %v", err)
	}
	if result.ContractCalibration.Status != TargetPairContractCalibrationUnresolved || result.ContractCalibration.RootCauseEligible || result.ContractCalibration.Reason != "paired runs do not resolve to the same contract profile and rule" {
		t.Fatalf("expected mismatched contract rules to remain unpromoted: %#v", result.ContractCalibration)
	}
	if len(result.RootCauseCandidates) != 0 {
		t.Fatalf("expected no root-cause hypotheses for mismatched contract rules: %#v", result.RootCauseCandidates)
	}
}

func writePairRunOracle(t *testing.T, runDir string, confirmed bool) {
	t.Helper()
	updatePairRunResult(t, runDir, func(result *TargetRunResult) {
		result.TargetOracle.Confirmed = confirmed
		if confirmed {
			result.TargetOracle.Status = TargetOracleStatusConfirmed
		} else {
			result.TargetOracle.Status = TargetOracleStatusNegative
		}
	})
}

func writePairRunContractCalibration(t *testing.T, runDir string, confirmed bool, status TargetContractInterpretationStatus) {
	t.Helper()
	updatePairRunResult(t, runDir, func(result *TargetRunResult) {
		result.TargetID = "langgraph-shell-react"
		result.TaskID = PersistentShellForkTargetTaskID
		result.TargetOracle.Confirmed = confirmed
		if confirmed {
			result.TargetOracle.Status = TargetOracleStatusConfirmed
		} else {
			result.TargetOracle.Status = TargetOracleStatusNegative
		}
		result.TaskCompliance = TargetTaskComplianceResult{Status: TargetTaskComplianceStatusCompliant}
		result.ContractInterpretation = &TargetContractInterpretation{
			Status:         status,
			ProfileID:      "langgraph-shell-react.phase5b.v1",
			RuleID:         "shell-path-fork-boundary",
			StateSurface:   "shell-session.path",
			LifecycleEdge:  "checkpoint->fork",
			Expectation:    TargetContractExpectationReset,
			SourceStrength: TargetContractSourceStrengthImplicit,
		}
	})
}

func updatePairRunResult(t *testing.T, runDir string, update func(*TargetRunResult)) {
	t.Helper()
	resultPath := filepath.Join(runDir, TargetResultArtifact)
	result, err := readTargetPairJSON[TargetRunResult](resultPath)
	if err != nil {
		t.Fatalf("read target result: %v", err)
	}
	update(&result)
	if err := core.WriteJSON(resultPath, result); err != nil {
		t.Fatalf("write target result: %v", err)
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
