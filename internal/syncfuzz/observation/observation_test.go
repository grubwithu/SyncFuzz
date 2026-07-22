package observation

import (
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestExtractTargetRunFootprintAndCompilePlanForUnixSocket(t *testing.T) {
	runDir := t.TempDir()
	task := target.TargetTask{
		TaskID: "unix-listener-residue-fork",
		Scenario: &target.TargetScenarioInfo{
			SchemaVersion:        target.TargetScenarioSchemaVersion,
			ScenarioID:           "unix-listener-residue-fork",
			TaskID:               "unix-listener-residue-fork",
			PlantPrimitiveID:     "workspace-unix-listener",
			StateSurface:         "runtime.unix-listener",
			LifecycleEdge:        "checkpoint->fork",
			ActivationKindID:     "unix-socket-connect",
			OracleKindID:         "unix-listener-residue",
			ExecutionPlan:        &target.TargetScenarioExecutionPlan{LifecycleOperationID: "checkpoint-fork"},
			DefaultExpectedFiles: []string{"unix-listener-residue-fork-check.txt"},
		},
	}
	writeObservationJSON(t, filepath.Join(runDir, target.TargetTaskArtifact), task)
	writeObservationJSON(t, filepath.Join(runDir, "snapshot-before.json"), core.Snapshot{Files: []core.FileEntry{}})
	writeObservationJSON(t, filepath.Join(runDir, "snapshot-after.json"), core.Snapshot{Files: []core.FileEntry{
		{Path: "branch-listener.sock", Type: "socket", Mode: "Srwxr-xr-x"},
		{Path: "unix-listener-residue-fork-check.txt", Type: "file", Mode: "-rw-r--r--", SHA256: "check"},
	}})
	writeObservationJSON(t, filepath.Join(runDir, "process-lineage.json"), core.ProcessLineageReport{
		NewAtBoundary: []core.ProcessEntry{{
			PID:              42,
			Name:             "python3",
			RawCmdline:       "python3 listener.py",
			OpenFDs:          []core.ProcessFDEntry{{FD: 7, Target: "socket:[1234]", Kind: "socket"}},
			WorkspaceRelated: true,
		}},
		RemainingAfter: []core.ProcessEntry{{
			PID:              42,
			Name:             "python3",
			RawCmdline:       "python3 listener.py",
			OpenFDs:          []core.ProcessFDEntry{{FD: 7, Target: "socket:[1234]", Kind: "socket"}},
			WorkspaceRelated: true,
		}},
	})
	writeObservationJSON(t, filepath.Join(runDir, core.StateTraceArtifact), core.CrossLayerTrace{
		Observations: []core.StateObservation{{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-after.json"}},
	})

	footprint, err := ExtractTargetRunFootprint(runDir)
	if err != nil {
		t.Fatalf("ExtractTargetRunFootprint failed: %v", err)
	}
	if footprint.QueryID != "unix-listener-residue-fork" {
		t.Fatalf("unexpected query id: %#v", footprint)
	}
	if footprint.Query == nil || footprint.Query.Boundary.LifecycleEdge != "checkpoint->fork" {
		t.Fatalf("expected typed lifecycle query: %#v", footprint.Query)
	}
	if !containsQueryComponentKind(footprint.Query.Plant.Components, "workspace-unix-listener") {
		t.Fatalf("expected plant query component: %#v", footprint.Query.Plant)
	}
	for _, class := range []ResourceClass{ResourceFilesystemNamespace, ResourceProcess, ResourceFileDescriptor, ResourceUnixSocket} {
		if !containsResourceClass(footprint.ResourceClasses, class) {
			t.Fatalf("footprint missing resource class %q: %#v", class, footprint.ResourceClasses)
		}
	}
	path := findFootprintPath(footprint.Paths, "branch-listener.sock", ResourceUnixSocket)
	if path.Path == "" || !containsString(path.Operations, "create") {
		t.Fatalf("expected socket create footprint: %#v", footprint.Paths)
	}
	if len(footprint.Processes) != 1 || footprint.Processes[0].Executable != "python3" {
		t.Fatalf("expected deduplicated listener process selector: %#v", footprint.Processes)
	}

	plan, err := CompilePlan(*footprint)
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if len(plan.Checkpoints) != 4 || plan.Checkpoints[0] != ObservationBeforePlant || plan.Checkpoints[3] != ObservationAfterActivation {
		t.Fatalf("unexpected checkpoints: %#v", plan.Checkpoints)
	}
	if plan.Query == nil || plan.Query.Hypothesis.Kind != "recovery-consistency" {
		t.Fatalf("expected query to propagate into plan: %#v", plan.Query)
	}
	for _, family := range []ProbeFamily{ProbeFilesystem, ProbeProcess, ProbeFD, ProbeUnixSocket} {
		probe, ok := findProbePlan(plan.ProbePlans, family)
		if !ok || !probe.Enabled {
			t.Fatalf("plan missing enabled probe family %q: %#v", family, plan.ProbePlans)
		}
	}
	socketProbe, _ := findProbePlan(plan.ProbePlans, ProbeUnixSocket)
	if !containsString(socketProbe.Paths, "branch-listener.sock") {
		t.Fatalf("socket probe missing listener path: %#v", socketProbe)
	}
}

func TestCompilePlanUsesDependencyClosureForFD(t *testing.T) {
	plan, err := CompilePlan(ResourceFootprint{
		QueryID:         "fd-query",
		ResourceClasses: []ResourceClass{ResourceFileDescriptor},
	})
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if _, ok := findProbePlan(plan.ProbePlans, ProbeFD); !ok {
		t.Fatalf("expected fd probe: %#v", plan.ProbePlans)
	}
	if _, ok := findProbePlan(plan.ProbePlans, ProbeProcess); !ok {
		t.Fatalf("expected process dependency for fd probe: %#v", plan.ProbePlans)
	}
}

func TestFootprintRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ResourceFootprintArtifact)
	want := &ResourceFootprint{QueryID: "orphan-process", ResourceClasses: []ResourceClass{ResourceProcess}}
	if err := WriteFootprint(path, want); err != nil {
		t.Fatalf("WriteFootprint failed: %v", err)
	}
	got, err := ReadFootprint(path)
	if err != nil {
		t.Fatalf("ReadFootprint failed: %v", err)
	}
	if got.SchemaVersion != ResourceFootprintSchemaVersion || got.QueryID != want.QueryID || len(got.ResourceClasses) != 1 {
		t.Fatalf("unexpected footprint round trip: %#v", got)
	}
}

func TestPlanRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ObservationPlanArtifact)
	want, err := CompilePlan(ResourceFootprint{
		QueryID:         "shell-query",
		ResourceClasses: []ResourceClass{ResourceExecutionContext},
	})
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if err := WritePlan(path, want); err != nil {
		t.Fatalf("WritePlan failed: %v", err)
	}
	got, err := ReadPlan(path)
	if err != nil {
		t.Fatalf("ReadPlan failed: %v", err)
	}
	if got.SchemaVersion != ObservationPlanSchemaVersion || got.QueryID != "shell-query" {
		t.Fatalf("unexpected plan round trip: %#v", got)
	}
	if _, ok := findProbePlan(got.ProbePlans, ProbeShellContext); !ok {
		t.Fatalf("expected shell-context plan: %#v", got.ProbePlans)
	}
}

func writeObservationJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := core.WriteJSON(path, value); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsResourceClass(values []ResourceClass, want ResourceClass) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsQueryComponentKind(values []QueryComponent, want string) bool {
	for _, value := range values {
		if value.KindID == want {
			return true
		}
	}
	return false
}

func findFootprintPath(paths []PathFootprint, wantPath string, wantClass ResourceClass) PathFootprint {
	for _, path := range paths {
		if path.Path == wantPath && path.ResourceClass == wantClass {
			return path
		}
	}
	return PathFootprint{}
}

func findProbePlan(plans []ProbePlan, family ProbeFamily) (ProbePlan, bool) {
	for _, plan := range plans {
		if plan.Family == family {
			return plan, true
		}
	}
	return ProbePlan{}, false
}
