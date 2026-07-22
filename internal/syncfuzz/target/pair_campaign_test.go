package target

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestRunTargetPairCampaignWritesCounterfactualReportsAndSummary(t *testing.T) {
	tmp := t.TempDir()
	calibratedControl := filepath.Join(tmp, "runs", "calibrated-control")
	calibratedTarget := filepath.Join(tmp, "runs", "calibrated-target")
	writePairRunArtifact(t, calibratedControl, "control-a", core.Snapshot{}, core.ProcessSnapshot{})
	writePairRunArtifact(t, calibratedTarget, "target-a", core.Snapshot{Files: []core.FileEntry{{Path: "branch-listener.sock", Type: "socket"}}}, core.ProcessSnapshot{})
	writePairRunContractCalibration(t, calibratedControl, false, TargetContractStatusConsistent)
	writePairRunContractCalibration(t, calibratedTarget, true, TargetContractStatusViolation)

	unresolvedControl := filepath.Join(tmp, "runs", "unresolved-control")
	unresolvedTarget := filepath.Join(tmp, "runs", "unresolved-target")
	writePairRunArtifact(t, unresolvedControl, "control-b", core.Snapshot{}, core.ProcessSnapshot{})
	writePairRunArtifact(t, unresolvedTarget, "target-b", core.Snapshot{Files: []core.FileEntry{{Path: "late-effect", Type: "file", SHA256: "effect"}}}, core.ProcessSnapshot{})
	writePairRunOracle(t, unresolvedControl, false)
	writePairRunOracle(t, unresolvedTarget, true)

	manifestPath := filepath.Join(tmp, "manifest.json")
	writeTargetPairCampaignManifest(t, manifestPath, TargetPairCampaignManifest{
		SchemaVersion: TargetPairCampaignManifestSchemaVersion,
		CampaignID:    "counterfactual-control-test",
		Pairs: []TargetPairCampaignPair{
			{
				PairID:        "fresh-runtime",
				ControlKind:   TargetPairControlFreshRuntime,
				ControlRunDir: filepath.Join("runs", "calibrated-control"),
				TargetRunDir:  filepath.Join("runs", "calibrated-target"),
			},
			{
				PairID:        "branch-cleanup",
				ControlKind:   TargetPairControlBranchCleanup,
				ControlRunDir: filepath.Join("runs", "unresolved-control"),
				TargetRunDir:  filepath.Join("runs", "unresolved-target"),
			},
		},
	})

	campaignDir := filepath.Join(tmp, "campaign")
	result, err := RunTargetPairCampaign(TargetPairCampaignOptions{ManifestPath: manifestPath, OutDir: campaignDir})
	if err != nil {
		t.Fatalf("RunTargetPairCampaign failed: %v", err)
	}
	if result.SchemaVersion != TargetPairCampaignResultSchemaVersion || result.CampaignID != "counterfactual-control-test" || result.TotalPairs != 2 || result.RootCauseEligiblePairs != 1 || result.CalibrationCoverage != 0.5 {
		t.Fatalf("unexpected pair campaign result: %#v", result)
	}
	if len(result.Pairs) != 2 || result.Pairs[0].PairID != "fresh-runtime" || !result.Pairs[0].RootCauseEligible || result.Pairs[1].PairID != "branch-cleanup" || result.Pairs[1].RootCauseEligible {
		t.Fatalf("unexpected pair campaign pair results: %#v", result.Pairs)
	}
	if len(result.ControlKinds) != 2 || result.ControlKinds[0].ControlKind != TargetPairControlBranchCleanup || result.ControlKinds[0].PairCount != 1 || result.ControlKinds[1].ControlKind != TargetPairControlFreshRuntime || result.ControlKinds[1].RootCauseEligiblePairs != 1 {
		t.Fatalf("unexpected control-kind summary: %#v", result.ControlKinds)
	}
	if len(result.CounterfactualLabels) != 1 || result.CounterfactualLabels[0].Label != TargetPairCounterfactualTargetOnly || result.CounterfactualLabels[0].PairCount != 2 {
		t.Fatalf("expected deterministic counterfactual labels: %#v", result.CounterfactualLabels)
	}
	if len(result.QueryStrata) != 2 {
		t.Fatalf("expected one query stratum per control kind: %#v", result.QueryStrata)
	}
	for _, artifact := range []string{
		filepath.Join(campaignDir, TargetPairCampaignManifestArtifact),
		filepath.Join(campaignDir, TargetPairCampaignResultArtifact),
		filepath.Join(campaignDir, TargetPairCalibrationSummaryArtifact),
		filepath.Join(campaignDir, "fresh-runtime", TargetPairDifferentialArtifact),
		filepath.Join(campaignDir, "branch-cleanup", TargetPairDifferentialArtifact),
	} {
		if _, err := os.Stat(artifact); err != nil {
			t.Fatalf("expected campaign artifact %s: %v", artifact, err)
		}
	}
	summary, err := readTargetPairJSON[TargetPairCalibrationSummary](filepath.Join(campaignDir, TargetPairCalibrationSummaryArtifact))
	if err != nil {
		t.Fatalf("read calibration summary: %v", err)
	}
	if summary.TotalPairs != 2 || summary.RootCauseEligiblePairs != 1 || summary.CalibrationCoverage != 0.5 {
		t.Fatalf("unexpected campaign calibration summary: %#v", summary)
	}
}

func TestRunTargetPairCampaignRejectsUnsupportedControlKind(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "manifest.json")
	writeTargetPairCampaignManifest(t, manifestPath, TargetPairCampaignManifest{
		SchemaVersion: TargetPairCampaignManifestSchemaVersion,
		Pairs: []TargetPairCampaignPair{{
			PairID:        "bad-control",
			ControlKind:   "unknown-control",
			ControlRunDir: "control",
			TargetRunDir:  "target",
		}},
	})
	if _, err := RunTargetPairCampaign(TargetPairCampaignOptions{ManifestPath: manifestPath, OutDir: filepath.Join(tmp, "campaign")}); err == nil {
		t.Fatal("expected unsupported control kind to be rejected")
	}
}

func writeTargetPairCampaignManifest(t *testing.T, path string, manifest TargetPairCampaignManifest) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create pair campaign manifest directory: %v", err)
	}
	if err := core.WriteJSON(path, manifest); err != nil {
		t.Fatalf("write pair campaign manifest: %v", err)
	}
}
