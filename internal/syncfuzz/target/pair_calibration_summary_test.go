package target

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
)

func TestSummarizeTargetPairCalibrationsAggregatesCoverageAndReasons(t *testing.T) {
	tmp := t.TempDir()
	calibratedPath := filepath.Join(tmp, "calibrated", TargetPairDifferentialArtifact)
	unresolvedPath := filepath.Join(tmp, "unresolved", TargetPairDifferentialArtifact)
	writePairCalibrationReport(t, calibratedPath, TargetPairDifferential{
		SchemaVersion: TargetPairDifferentialSchemaVersion,
		QueryID:       "unix-listener-residue-fork",
		ControlRunID:  "control-a",
		TargetRunID:   "target-a",
		Evidence: []TargetPairEvidenceCandidate{
			{Family: "filesystem", Kind: "target-only-path", Detail: "branch-listener.sock"},
			{Family: "process", Kind: "target-only-process", Detail: "listener"},
		},
		ContractCalibration: TargetPairContractCalibration{
			Status:            TargetPairContractCalibrationCalibrated,
			Reason:            "target contract violation is paired with a task-compliant, contract-consistent control",
			RootCauseEligible: true,
			Target: TargetPairContractReading{
				Available:        true,
				Status:           TargetContractStatusViolation,
				ProfileID:        "langgraph-shell-react.phase5b.v1",
				RuleID:           "communication-unix-listener-fork-boundary",
				StateSurface:     "runtime.unix-listener",
				LifecycleEdge:    "checkpoint->fork",
				SourceStrength:   TargetContractSourceStrengthImplicit,
				ComplianceStatus: TargetTaskComplianceStatusCompliant,
			},
		},
		RootCauseCandidates: []TargetPairRootCauseCandidate{{
			Point:             observation.ObservationAfterRecovery,
			StateSurface:      "filesystem-namespace",
			Mechanism:         "target-only namespace object at lifecycle checkpoint",
			Evidence:          "branch-listener.sock",
			ContractProfileID: "langgraph-shell-react.phase5b.v1",
			ContractRuleID:    "communication-unix-listener-fork-boundary",
		}},
	})
	writePairCalibrationReport(t, unresolvedPath, TargetPairDifferential{
		SchemaVersion: TargetPairDifferentialSchemaVersion,
		QueryID:       "orphan-process",
		ControlRunID:  "control-b",
		TargetRunID:   "target-b",
		Evidence:      []TargetPairEvidenceCandidate{{Family: "process", Kind: "target-only-process", Detail: "sh"}},
		ContractCalibration: TargetPairContractCalibration{
			Status:  TargetPairContractCalibrationUnresolved,
			Reason:  "a paired run does not provide a contract interpretation",
			Control: TargetPairContractReading{ComplianceStatus: TargetTaskComplianceStatusNotApplicable},
			Target:  TargetPairContractReading{ComplianceStatus: TargetTaskComplianceStatusNotApplicable},
		},
	})
	reviewPath := filepath.Join(tmp, "reviews", "review.json")
	writePairRootCauseReviewManifest(t, reviewPath, TargetPairRootCauseReviewManifest{
		SchemaVersion: "syncfuzz.target-pair-root-cause-review.v1",
		Reviews: []TargetPairRootCauseCandidateReview{{
			TargetRunID:       "target-a",
			Point:             observation.ObservationAfterRecovery,
			StateSurface:      "filesystem-namespace",
			Mechanism:         "target-only namespace object at lifecycle checkpoint",
			Evidence:          "branch-listener.sock",
			ContractProfileID: "langgraph-shell-react.phase5b.v1",
			ContractRuleID:    "communication-unix-listener-fork-boundary",
			Verdict:           TargetPairRootCauseReviewSupported,
		}},
	})

	output := filepath.Join(tmp, "summary", TargetPairCalibrationSummaryArtifact)
	result, err := SummarizeTargetPairCalibrations(TargetPairCalibrationSummaryOptions{
		Inputs:              []string{tmp, calibratedPath},
		ReviewManifestPaths: []string{reviewPath},
		OutputPath:          output,
	})
	if err != nil {
		t.Fatalf("SummarizeTargetPairCalibrations failed: %v", err)
	}
	if result.SchemaVersion != TargetPairCalibrationSummarySchemaVersion || result.TotalPairs != 2 || result.RootCauseEligiblePairs != 1 || result.CalibrationCoverage != 0.5 || result.EvidenceCandidates != 3 || result.RootCauseCandidates != 1 {
		t.Fatalf("unexpected calibration summary: %#v", result)
	}
	if len(result.Reports) != 2 || result.Reports[0].Artifact >= result.Reports[1].Artifact {
		t.Fatalf("expected deduplicated, sorted report references: %#v", result.Reports)
	}
	if len(result.CalibrationStatuses) != 2 || result.CalibrationStatuses[0].Status != TargetPairContractCalibrationCalibrated || result.CalibrationStatuses[0].PairCount != 1 || result.CalibrationStatuses[1].Status != TargetPairContractCalibrationUnresolved || result.CalibrationStatuses[1].PairCount != 1 {
		t.Fatalf("unexpected calibration status aggregation: %#v", result.CalibrationStatuses)
	}
	if len(result.UnresolvedReasons) != 1 || result.UnresolvedReasons[0].Reason != "a paired run does not provide a contract interpretation" || result.UnresolvedReasons[0].PairCount != 1 {
		t.Fatalf("unexpected unresolved-reason aggregation: %#v", result.UnresolvedReasons)
	}
	if len(result.ContractRules) != 1 || result.ContractRules[0].RuleID != "communication-unix-listener-fork-boundary" || result.ContractRules[0].PairCount != 1 || result.ContractRules[0].RootCauseEligiblePairs != 1 || result.ContractRules[0].RootCauseCandidates != 1 {
		t.Fatalf("unexpected contract-rule aggregation: %#v", result.ContractRules)
	}
	if result.HypothesisReview == nil || result.HypothesisReview.ReviewedCandidates != 1 || result.HypothesisReview.SupportedCandidates != 1 || result.HypothesisReview.PrecisionDenominator != 1 || !result.HypothesisReview.PrecisionAvailable || result.HypothesisReview.Precision != 1 {
		t.Fatalf("unexpected hypothesis review aggregation: %#v", result.HypothesisReview)
	}
	stored, err := readTargetPairJSON[TargetPairCalibrationSummary](output)
	if err != nil {
		t.Fatalf("read written summary: %v", err)
	}
	if stored.TotalPairs != result.TotalPairs || stored.CalibrationCoverage != result.CalibrationCoverage {
		t.Fatalf("written summary drifted: %#v", stored)
	}
}

func TestSummarizeTargetPairCalibrationsRejectsStaleRootCauseReview(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, TargetPairDifferentialArtifact)
	writePairCalibrationReport(t, reportPath, TargetPairDifferential{
		SchemaVersion: TargetPairDifferentialSchemaVersion,
		QueryID:       "orphan-process",
		TargetRunID:   "target-a",
		ContractCalibration: TargetPairContractCalibration{
			Status:            TargetPairContractCalibrationCalibrated,
			Reason:            "target contract violation is paired with a task-compliant, contract-consistent control",
			RootCauseEligible: true,
		},
		RootCauseCandidates: []TargetPairRootCauseCandidate{{
			Point:             observation.ObservationAfterRecovery,
			StateSurface:      "process",
			Mechanism:         "target-only process persisted at lifecycle checkpoint",
			Evidence:          "listener",
			ContractProfileID: "profile",
			ContractRuleID:    "rule",
		}},
	})
	reviewPath := filepath.Join(tmp, "review.json")
	writePairRootCauseReviewManifest(t, reviewPath, TargetPairRootCauseReviewManifest{
		SchemaVersion: "syncfuzz.target-pair-root-cause-review.v1",
		Reviews: []TargetPairRootCauseCandidateReview{{
			TargetRunID:       "target-a",
			Point:             observation.ObservationAfterRecovery,
			StateSurface:      "process",
			Mechanism:         "target-only process persisted at lifecycle checkpoint",
			Evidence:          "different-listener",
			ContractProfileID: "profile",
			ContractRuleID:    "rule",
			Verdict:           TargetPairRootCauseReviewSupported,
		}},
	})
	if _, err := SummarizeTargetPairCalibrations(TargetPairCalibrationSummaryOptions{
		Inputs:              []string{reportPath},
		ReviewManifestPaths: []string{reviewPath},
		OutputPath:          filepath.Join(tmp, "summary.json"),
	}); err == nil || !strings.Contains(err.Error(), "does not match a candidate") {
		t.Fatalf("expected stale root-cause review to be rejected, got %v", err)
	}
}

func TestSummarizeTargetPairCalibrationsRejectsUnsupportedPairSchema(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, TargetPairDifferentialArtifact)
	writePairCalibrationReport(t, reportPath, TargetPairDifferential{
		SchemaVersion: "syncfuzz.target-pair-differential.v1",
		QueryID:       "orphan-process",
	})
	if _, err := SummarizeTargetPairCalibrations(TargetPairCalibrationSummaryOptions{
		Inputs:     []string{reportPath},
		OutputPath: filepath.Join(tmp, "summary.json"),
	}); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("expected unsupported pair schema to be rejected, got %v", err)
	}
}

func TestSummarizeTargetPairCalibrationsRequiresOutputPath(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, TargetPairDifferentialArtifact)
	writePairCalibrationReport(t, reportPath, TargetPairDifferential{
		SchemaVersion: TargetPairDifferentialSchemaVersion,
		QueryID:       "orphan-process",
		ContractCalibration: TargetPairContractCalibration{
			Status: TargetPairContractCalibrationUnresolved,
			Reason: "a paired run does not provide a contract interpretation",
		},
	})
	if _, err := SummarizeTargetPairCalibrations(TargetPairCalibrationSummaryOptions{Inputs: []string{reportPath}}); err == nil || !strings.Contains(err.Error(), "output path is required") {
		t.Fatalf("expected output requirement, got %v", err)
	}
}

func writePairCalibrationReport(t *testing.T, path string, report TargetPairDifferential) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create pair calibration report directory: %v", err)
	}
	if err := core.WriteJSON(path, report); err != nil {
		t.Fatalf("write pair calibration report: %v", err)
	}
}

func writePairRootCauseReviewManifest(t *testing.T, path string, manifest TargetPairRootCauseReviewManifest) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create root-cause review manifest directory: %v", err)
	}
	if err := core.WriteJSON(path, manifest); err != nil {
		t.Fatalf("write root-cause review manifest: %v", err)
	}
}
