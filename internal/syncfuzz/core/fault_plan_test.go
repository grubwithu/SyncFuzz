package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestFaultPlansCoverCases(t *testing.T) {
	for _, testCase := range core.Cases() {
		plan, err := core.ResolveFaultPlan(testCase.Name, "")
		if err != nil {
			t.Fatalf("expected default fault plan for %s: %v", testCase.Name, err)
		}
		if plan.CaseName != testCase.Name {
			t.Fatalf("fault plan case mismatch: got %s want %s", plan.CaseName, testCase.Name)
		}
		if plan.SchemaVersion != "syncfuzz.fault-plan.v1" {
			t.Fatalf("unexpected schema version %q", plan.SchemaVersion)
		}
		if plan.InjectPhase == "" || plan.Fault == "" || plan.ExpectedImpact == "" {
			t.Fatalf("incomplete fault plan: %#v", plan)
		}
	}
}

func TestResolveFaultPlanRejectsWrongCase(t *testing.T) {
	_, err := core.ResolveFaultPlan("action-replay", "orphan-process/p5-delayed-child")
	if err == nil {
		t.Fatalf("expected mismatched fault plan to be rejected")
	}
}

func TestRunRejectsUnknownCaseBeforeFaultPlanResolution(t *testing.T) {
	_, err := cases.Run(context.Background(), core.RunOptions{
		CaseName: "not-a-case",
		OutDir:   t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown case") {
		t.Fatalf("expected unknown case error, got %v", err)
	}
}

func TestRunWritesFaultPlanArtifact(t *testing.T) {
	tmp := t.TempDir()
	result, err := cases.Run(context.Background(), core.RunOptions{
		CaseName: "action-replay",
		OutDir:   filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.FaultPlanID != "action-replay/p5-dropped-receipt" {
		t.Fatalf("unexpected fault plan id %q", result.FaultPlanID)
	}
	if !fileExists(filepath.Join(result.ArtifactDir, core.FaultPlanArtifact)) {
		t.Fatalf("expected fault plan artifact")
	}
}
