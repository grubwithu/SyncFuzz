package target

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunTargetRuntimePairExecutesFreshControlAndTarget(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTargetRuntimePair(context.Background(), TargetRuntimePairOptions{
		PairID:      "fresh-runtime",
		ControlKind: TargetPairControlFreshRuntime,
		OutDir:      filepath.Join(tmp, "runs"),
		Control: TargetRunOptions{
			TargetID: "local-runtime-pair",
			TaskID:   DefaultTargetTaskID,
			Command:  "printf control > control-only.txt",
			Timeout:  5 * time.Second,
		},
		Target: TargetRunOptions{
			TargetID: "local-runtime-pair",
			TaskID:   DefaultTargetTaskID,
			Command:  "printf target > target-only.txt; printf ok > late-effect",
			Timeout:  5 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("RunTargetRuntimePair failed: %v", err)
	}
	if result.SchemaVersion != TargetRuntimePairSchemaVersion || result.ControlKind != TargetPairControlFreshRuntime || result.QueryID != DefaultTargetTaskID {
		t.Fatalf("unexpected runtime pair result: %#v", result)
	}
	if result.CounterfactualLabel != TargetPairCounterfactualTargetOnly {
		t.Fatalf("expected target-only counterfactual label, got %#v", result)
	}
	if result.QueryStratum.RootQueryID != DefaultTargetTaskID || result.QueryStratum.ViolationSignature == nil {
		t.Fatalf("expected query stratum from freshly executed target run: %#v", result.QueryStratum)
	}
	for _, path := range []string{
		result.ControlRunDir,
		result.TargetRunDir,
		filepath.Join(result.ArtifactDir, result.PairDifferential),
		filepath.Join(result.ArtifactDir, TargetRuntimePairArtifact),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected runtime pair artifact %s: %v", path, err)
		}
	}
	campaign, err := RunTargetPairCampaign(TargetPairCampaignOptions{
		RuntimePairPaths: []string{filepath.Join(result.ArtifactDir, TargetRuntimePairArtifact)},
		OutDir:           filepath.Join(tmp, "campaign"),
	})
	if err != nil {
		t.Fatalf("RunTargetPairCampaign over fresh runtime pair failed: %v", err)
	}
	if campaign.TotalPairs != 1 || len(campaign.RuntimePairArtifacts) != 1 || len(campaign.CounterfactualLabels) != 1 || campaign.CounterfactualLabels[0].Label != TargetPairCounterfactualTargetOnly {
		t.Fatalf("fresh runtime pair did not feed pair campaign labels: %#v", campaign)
	}
}

func TestRunTargetRuntimePairRejectsMismatchedTargetAndCustomWithoutDescription(t *testing.T) {
	base := TargetRunOptions{Command: "true"}
	if _, err := RunTargetRuntimePair(context.Background(), TargetRuntimePairOptions{
		ControlKind: TargetPairControlCustom,
		OutDir:      t.TempDir(),
		Control:     base,
		Target:      base,
	}); err == nil {
		t.Fatal("expected custom control without description to fail")
	}
	if _, err := RunTargetRuntimePair(context.Background(), TargetRuntimePairOptions{
		ControlKind: TargetPairControlBaseline,
		OutDir:      t.TempDir(),
		Control: TargetRunOptions{
			TargetID: "control-target",
			TaskID:   DefaultTargetTaskID,
			Command:  "true",
		},
		Target: TargetRunOptions{
			TargetID: "target-target",
			TaskID:   DefaultTargetTaskID,
			Command:  "true",
		},
	}); err == nil {
		t.Fatal("expected mismatched target ids to fail before running either side")
	}
	pairOut := t.TempDir()
	if err := os.Mkdir(filepath.Join(pairOut, "existing-pair"), 0o755); err != nil {
		t.Fatalf("create existing pair directory: %v", err)
	}
	if _, err := RunTargetRuntimePair(context.Background(), TargetRuntimePairOptions{
		PairID:      "existing-pair",
		ControlKind: TargetPairControlBaseline,
		OutDir:      pairOut,
		Control:     base,
		Target:      base,
	}); err == nil {
		t.Fatal("expected existing pair directory to be rejected")
	}
}
