package hazard_test

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/hazard"
)

func TestRunUnixSocketCalibrationProducesFixtureOnlyReboundClosure(t *testing.T) {
	result := runCalibrationOrSkip(t)
	if err := result.Validate(); err != nil {
		t.Fatalf("calibration result failed validation: %v", err)
	}
	if result.Scope != "fixture-only" || result.HazardReport.Status != hazard.RecoveryHazardStatusRealizedCalibration || result.HazardReport.Class != hazard.RecoveryHazardClassRebound {
		t.Fatalf("unexpected calibration result: %#v", result.HazardReport)
	}
	controls := controlsByName(result.HazardReport.Controls)
	if controls[hazard.HazardControlTreatment].ExpectedRole != "benign" || controls[hazard.HazardControlTreatment].UseEvidence == nil || controls[hazard.HazardControlTreatment].UseEvidence.ListenerRole != "replacement" {
		t.Fatalf("treatment did not demonstrate before-state expectation over rebound binding: %#v", controls[hazard.HazardControlTreatment])
	}
	for _, name := range []hazard.HazardControlName{hazard.HazardControlFrontierLocal, hazard.HazardControlHead} {
		control := controls[name]
		if control.UseEvidence == nil || control.ExpectedRole != "replacement" || control.UseEvidence.ListenerRole != "replacement" {
			t.Fatalf("%s control did not retain after/head role awareness: %#v", name, control)
		}
	}
	for _, name := range []hazard.HazardControlName{hazard.HazardControlRetentionAblation, hazard.HazardControlCleanBaseline} {
		control := controls[name]
		if control.UseEvidence == nil || control.ExpectedRole != "benign" || control.UseEvidence.ListenerRole != "benign" {
			t.Fatalf("%s control did not establish clean benign baseline: %#v", name, control)
		}
	}
	if result.HazardReport.RecoveryEvidence.Source != hazard.HistoricalRecoveryEvidenceCalibrationFixture {
		t.Fatalf("calibration must not masquerade as a StateSeed: %#v", result.HazardReport.RecoveryEvidence)
	}
	frontiers := result.FixtureProfile.CheckpointMap.HotFrontiers()
	if len(frontiers) != 1 || frontiers[0].FrontierID != "before-bind..after-bind" || len(frontiers[0].EvidenceLinks) == 0 {
		t.Fatalf("unexpected fixture frontier evidence: %#v", frontiers)
	}
	instances := instancesByName(result.HazardReport.Environments)
	tainted := instances["tainted-head"].Materialization
	if tainted.InitialBinding.Local.SocketID() == tainted.ActiveBinding.Local.SocketID() {
		t.Fatalf("rebind must use a distinct active socket identity: %#v", tainted)
	}
	if instances["clean-ablation"].Materialization.ActiveBinding.Local.SocketID() == instances["clean-baseline"].Materialization.ActiveBinding.Local.SocketID() {
		t.Fatalf("fresh clean controls unexpectedly reused a local socket identity")
	}

	artifact := filepath.Join(t.TempDir(), "calibration.json")
	if err := hazard.WriteUnixSocketCalibrationResult(artifact, result); err != nil {
		t.Fatalf("WriteUnixSocketCalibrationResult returned error: %v", err)
	}
	loaded, err := hazard.ReadUnixSocketCalibrationResult(artifact)
	if err != nil {
		t.Fatalf("ReadUnixSocketCalibrationResult returned error: %v", err)
	}
	if loaded.CalibrationID != result.CalibrationID || loaded.HazardReport.ReportID != result.HazardReport.ReportID {
		t.Fatalf("calibration JSON round-trip changed identity: %#v", loaded)
	}
}

func TestRecoveryHazardDoesNotPromoteStaticResidualWithoutUseEvidence(t *testing.T) {
	result := runCalibrationOrSkip(t)
	controls := append([]hazard.RecoveryHazardControl(nil), result.HazardReport.Controls...)
	for index := range controls {
		if controls[index].Name == hazard.HazardControlTreatment {
			controls[index].UseEvidence = nil
		}
	}
	report, err := hazard.BuildRecoveryHazardReport(hazard.RecoveryHazardReportInput{
		Calibration:      true,
		Workload:         result.Workload,
		RecoveryEvidence: result.HazardReport.RecoveryEvidence,
		WriteEvidence:    result.HazardReport.WriteEvidence,
		Environments:     result.HazardReport.Environments,
		Controls:         controls,
	})
	if err != nil {
		t.Fatalf("BuildRecoveryHazardReport returned error: %v", err)
	}
	if report.Status != hazard.RecoveryHazardStatusInconclusive || report.Class != hazard.RecoveryHazardClassNone {
		t.Fatalf("static residual without typed use must stay inconclusive: %#v", report)
	}
}

func TestRecoveryHazardDoesNotCallKnownRoleARebound(t *testing.T) {
	result := runCalibrationOrSkip(t)
	controls := append([]hazard.RecoveryHazardControl(nil), result.HazardReport.Controls...)
	for index := range controls {
		if controls[index].Name == hazard.HazardControlTreatment {
			controls[index].ExpectedRole = "replacement"
		}
	}
	report, err := hazard.BuildRecoveryHazardReport(hazard.RecoveryHazardReportInput{
		Calibration:      true,
		Workload:         result.Workload,
		RecoveryEvidence: result.HazardReport.RecoveryEvidence,
		WriteEvidence:    result.HazardReport.WriteEvidence,
		Environments:     result.HazardReport.Environments,
		Controls:         controls,
	})
	if err != nil {
		t.Fatalf("BuildRecoveryHazardReport returned error: %v", err)
	}
	if report.Status != hazard.RecoveryHazardStatusNotRealized || report.Class != hazard.RecoveryHazardClassNone {
		t.Fatalf("known replacement role must not become rebound: %#v", report)
	}
}

func controlsByName(controls []hazard.RecoveryHazardControl) map[hazard.HazardControlName]hazard.RecoveryHazardControl {
	result := make(map[hazard.HazardControlName]hazard.RecoveryHazardControl, len(controls))
	for _, control := range controls {
		result[control.Name] = control
	}
	return result
}

func instancesByName(instances []hazard.HazardEnvironmentInstance) map[string]hazard.HazardEnvironmentInstance {
	result := make(map[string]hazard.HazardEnvironmentInstance, len(instances))
	for _, instance := range instances {
		result[instance.InstanceID] = instance
	}
	return result
}

func runCalibrationOrSkip(t *testing.T) hazard.UnixSocketCalibrationResult {
	t.Helper()
	result, err := hazard.RunUnixSocketCalibration(context.Background(), t.TempDir())
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("test sandbox does not permit Unix-domain sockets: %v", err)
		}
		t.Fatalf("RunUnixSocketCalibration returned error: %v", err)
	}
	return result
}
