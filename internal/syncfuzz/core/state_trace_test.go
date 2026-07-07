package core_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestBuildCrossLayerTraceSummarizesCoreLayers(t *testing.T) {
	run := &core.RunContext{
		RunID:       "run-1",
		CaseName:    "trace-test",
		Environment: "local",
	}
	trace := core.BuildCrossLayerTrace(run, core.MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-external-commit",
		StateClass:     "external-effect",
		Operation:      "duplicate-create",
		Relation:       "missing-receipt",
		Impact:         "forgotten-external-effect",
	}, true, "2026-01-01T00:00:00Z", []core.StateObservation{
		{Layer: "external", StateClass: "external-effect", Phase: "P8", Artifact: "external-after.json", Kind: "external-state-snapshot"},
		{Layer: "agent", StateClass: "agent-logical", Phase: "oracle", Artifact: "agent-state.json", Kind: "agent-state-projection"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
	})

	if trace.SchemaVersion != "syncfuzz.state-trace.v1" {
		t.Fatalf("unexpected schema version %q", trace.SchemaVersion)
	}
	if !layerPresent(trace.Layers, "agent") || !layerPresent(trace.Layers, "os") || !layerPresent(trace.Layers, "external") {
		t.Fatalf("expected agent, os, and external layers, got %#v", trace.Layers)
	}
	if layerPresent(trace.Layers, "authority") {
		t.Fatalf("did not expect authority layer to be present: %#v", trace.Layers)
	}
	if trace.Observations[0].Phase != "P0" || trace.Observations[len(trace.Observations)-1].Phase != "oracle" {
		t.Fatalf("expected observations to be lifecycle-sorted, got %#v", trace.Observations)
	}
}

func TestFastSeedsWritePhase2Artifacts(t *testing.T) {
	tmp := t.TempDir()
	for _, caseName := range []string{"action-replay", "authority-resurrection", "partial-filesystem-rollback"} {
		t.Run(caseName, func(t *testing.T) {
			result, err := cases.Run(context.Background(), core.RunOptions{
				CaseName: caseName,
				OutDir:   filepath.Join(tmp, "runs"),
			})
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if !result.Confirmed {
				t.Fatalf("expected confirmed result")
			}
			if result.FaultPlanID == "" {
				t.Fatalf("expected run result to include fault plan id")
			}
			for _, artifact := range []string{core.AgentStateArtifact, core.StateTraceArtifact, core.FaultPlanArtifact} {
				if !fileExists(filepath.Join(result.ArtifactDir, artifact)) {
					t.Fatalf("expected phase 2 artifact %s", artifact)
				}
			}
		})
	}
}

func layerPresent(layers []core.StateLayerSummary, name string) bool {
	for _, layer := range layers {
		if layer.Layer == name {
			return layer.Present
		}
	}
	return false
}
