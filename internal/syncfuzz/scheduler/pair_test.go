package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestControlRunStaysClean(t *testing.T) {
	result, err := cases.Run(context.Background(), core.RunOptions{
		CaseName: "action-replay",
		OutDir:   filepath.Join(t.TempDir(), "runs"),
		RunRole:  core.RunRoleControl,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.RunRole != core.RunRoleControl {
		t.Fatalf("unexpected run role %q", result.RunRole)
	}
	if result.Confirmed {
		t.Fatalf("expected control run to stay clean")
	}
}

func TestRunPairWritesDifferentialReport(t *testing.T) {
	result, err := RunPair(context.Background(), PairOptions{
		CaseName: "action-replay",
		OutDir:   filepath.Join(t.TempDir(), "runs"),
	})
	if err != nil {
		t.Fatalf("RunPair failed: %v", err)
	}
	if result.SchemaVersion != "syncfuzz.differential-report.v1" {
		t.Fatalf("unexpected schema version %q", result.SchemaVersion)
	}
	if result.Control.RunRole != core.RunRoleControl || result.Fault.RunRole != core.RunRoleFault {
		t.Fatalf("unexpected pair roles: control=%q fault=%q", result.Control.RunRole, result.Fault.RunRole)
	}
	if result.Control.Confirmed {
		t.Fatalf("expected control run to remain unconfirmed")
	}
	if !result.Fault.Confirmed {
		t.Fatalf("expected fault run to confirm")
	}
	if !result.Verdict.Differential || !result.Verdict.SecurityRelevant {
		t.Fatalf("expected security-relevant differential verdict: %#v", result.Verdict)
	}
	if len(result.ObservationCoverage) != 2 {
		t.Fatalf("expected two observation coverage entries")
	}
	if !fileExists(filepath.Join(result.ArtifactDir, differentialReportArtifact)) {
		t.Fatalf("expected differential report artifact")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
