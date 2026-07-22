package observation

import (
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestExtractTargetRunFootprintAndCompilePlanForUnixSocket(t *testing.T) {
	runDir := t.TempDir()
	task := targetTaskArtifact{
		TaskID: "unix-listener-residue-fork",
		Scenario: &targetScenarioArtifact{
			ScenarioID:           "unix-listener-residue-fork",
			TaskID:               "unix-listener-residue-fork",
			PlantPrimitiveID:     "workspace-unix-listener",
			StateSurface:         "runtime.unix-listener",
			LifecycleEdge:        "checkpoint->fork",
			ActivationKindID:     "unix-socket-connect",
			OracleKindID:         "unix-listener-residue",
			ExecutionPlan:        &targetExecutionPlanArtifact{LifecycleOperationID: "checkpoint-fork"},
			DefaultExpectedFiles: []string{"unix-listener-residue-fork-check.txt"},
		},
	}
	writeObservationJSON(t, filepath.Join(runDir, targetTaskArtifactName), task)
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
	if path.Path == "" || path.OriginPhase != "after-recovery" || !containsString(path.Operations, "create") {
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

func TestCaptureTargetedProbeCheckpointSelectsOnlyPlannedObjects(t *testing.T) {
	plan := ObservationPlan{
		QueryID:     "probe-query",
		Checkpoints: []ObservationPoint{ObservationAfterRecovery},
		ProbePlans: []ProbePlan{
			{Family: ProbeFilesystem, Enabled: true, Paths: []string{"wanted.txt", "missing.txt"}, Fields: []string{"exists"}},
			{Family: ProbeUnixSocket, Enabled: true, Paths: []string{"listener.sock"}, Fields: []string{"exists"}},
			{Family: ProbeProcess, Enabled: true, ProcessSelectors: []ProcessFootprint{{Executable: "listener", CommandLine: "listener --socket listener.sock"}}, Fields: []string{"alive"}},
		},
	}
	snapshot := core.Snapshot{Files: []core.FileEntry{
		{Path: "wanted.txt", Type: "file"},
		{Path: "listener.sock", Type: "socket"},
		{Path: "unplanned.txt", Type: "file"},
	}}
	processes := core.ProcessSnapshot{Processes: []core.ProcessEntry{
		{PID: 10, Name: "listener", RawCmdline: "listener --socket listener.sock", WorkspaceRelated: true},
		{PID: 11, Name: "other", RawCmdline: "other", WorkspaceRelated: true},
	}}

	checkpoint, err := CaptureTargetedProbeCheckpoint(plan, ObservationAfterRecovery, "P6", &snapshot, &processes, "test")
	if err != nil {
		t.Fatalf("CaptureTargetedProbeCheckpoint failed: %v", err)
	}
	filesystem, ok := findTargetedProbeFamily(checkpoint.Families, ProbeFilesystem)
	if !ok || len(filesystem.MatchedPaths) != 1 || filesystem.MatchedPaths[0].Path != "wanted.txt" || !containsString(filesystem.MissingPaths, "missing.txt") {
		t.Fatalf("unexpected filesystem targeted probe: %#v", filesystem)
	}
	socket, ok := findTargetedProbeFamily(checkpoint.Families, ProbeUnixSocket)
	if !ok || len(socket.MatchedPaths) != 1 || socket.MatchedPaths[0].Path != "listener.sock" {
		t.Fatalf("unexpected socket targeted probe: %#v", socket)
	}
	process, ok := findTargetedProbeFamily(checkpoint.Families, ProbeProcess)
	if !ok || len(process.MatchedProcesses) != 1 || process.MatchedProcesses[0].PID != 10 {
		t.Fatalf("unexpected process targeted probe: %#v", process)
	}
}

func TestProcessSelectorsForPlanMergesProcessAndFDSelectors(t *testing.T) {
	plan := ObservationPlan{
		QueryID:     "selector-query",
		Checkpoints: []ObservationPoint{ObservationAfterRecovery},
		ProbePlans: []ProbePlan{
			{Family: ProbeProcess, Enabled: true, ProcessSelectors: []ProcessFootprint{{Executable: "listener", CommandLine: "listener --serve"}}, Fields: []string{"alive"}},
			{Family: ProbeFD, Enabled: true, ProcessSelectors: []ProcessFootprint{{Executable: "holder"}, {Executable: "listener", CommandLine: "listener --serve"}}, Fields: []string{"fd_number"}},
		},
	}
	selectors, err := ProcessSelectorsForPlan(plan)
	if err != nil {
		t.Fatalf("ProcessSelectorsForPlan failed: %v", err)
	}
	if len(selectors) != 2 || selectors[0].Executable != "holder" || selectors[1].CommandLine != "listener --serve" {
		t.Fatalf("unexpected canonical selectors: %#v", selectors)
	}
}

func TestCaptureTargetedProbeCheckpointUsesSelectedProcessScope(t *testing.T) {
	plan := ObservationPlan{
		QueryID:     "selected-scope-query",
		Checkpoints: []ObservationPoint{ObservationAfterRecovery},
		ProbePlans: []ProbePlan{{
			Family:           ProbeProcess,
			Enabled:          true,
			ProcessSelectors: []ProcessFootprint{{Executable: "listener", CommandLine: "listener --serve"}},
			Fields:           []string{"alive"},
		}},
	}
	processes := core.ProcessSnapshot{
		CollectionScope: "selected",
		Processes: []core.ProcessEntry{{
			PID:        11,
			Name:       "listener",
			RawCmdline: "listener --serve",
		}},
	}
	checkpoint, err := CaptureTargetedProbeCheckpoint(plan, ObservationAfterRecovery, "P6", nil, &processes, "selected process")
	if err != nil {
		t.Fatalf("CaptureTargetedProbeCheckpoint failed: %v", err)
	}
	process, ok := findTargetedProbeFamily(checkpoint.Families, ProbeProcess)
	if !ok || len(process.MatchedProcesses) != 1 || process.MatchedProcesses[0].PID != 11 {
		t.Fatalf("selected process should not require workspace cwd/FD evidence: %#v", process)
	}
}

func TestRefinePlanFromFallbackExpandsOnceWithSocketDependencies(t *testing.T) {
	plan := ObservationPlan{
		QueryID:                 "refine-query",
		Checkpoints:             []ObservationPoint{ObservationBeforePlant, ObservationAfterRecovery},
		FallbackFullProbe:       true,
		UnplannedResourcePolicy: "expand-once-then-full-probe",
		ProbePlans:              []ProbePlan{{Family: ProbeFilesystem, Enabled: true, Paths: []string{"known.txt"}, Fields: []string{"exists"}}},
	}
	report := TargetedProbeReport{
		SchemaVersion:                    TargetedProbeReportSchemaVersion,
		QueryID:                          "refine-query",
		FullProbeFallbackUsed:            true,
		FallbackFilesystemArtifact:       "snapshot-full-fallback.json",
		UnplannedFallbackFilesystemPaths: []string{"listener.sock", "unexpected.txt"},
	}
	fallback := core.Snapshot{Files: []core.FileEntry{
		{Path: "known.txt", Type: "file"},
		{Path: "listener.sock", Type: "socket"},
		{Path: "unexpected.txt", Type: "file"},
	}}
	refined, err := RefinePlanFromFallback(plan, report, fallback)
	if err != nil {
		t.Fatalf("RefinePlanFromFallback failed: %v", err)
	}
	if refined.ExpansionCount != 1 || refined.LastExpansionSource != "snapshot-full-fallback.json" || !containsString(refined.LastExpansionPaths, "listener.sock") || !containsString(refined.LastExpansionPaths, "unexpected.txt") {
		t.Fatalf("unexpected refined plan metadata: %#v", refined)
	}
	filesystem, _ := findProbePlan(refined.ProbePlans, ProbeFilesystem)
	if !containsString(filesystem.Paths, "listener.sock") || !containsString(filesystem.Paths, "unexpected.txt") {
		t.Fatalf("expected filesystem expansion: %#v", filesystem)
	}
	socket, socketOK := findProbePlan(refined.ProbePlans, ProbeUnixSocket)
	if !socketOK || !containsString(socket.Paths, "listener.sock") {
		t.Fatalf("expected socket expansion: %#v", refined.ProbePlans)
	}
	if _, ok := findProbePlan(refined.ProbePlans, ProbeProcess); !ok {
		t.Fatalf("expected process dependency: %#v", refined.ProbePlans)
	}
	if _, ok := findProbePlan(refined.ProbePlans, ProbeFD); !ok {
		t.Fatalf("expected fd dependency: %#v", refined.ProbePlans)
	}
	if _, err := RefinePlanFromFallback(*refined, report, fallback); err == nil {
		t.Fatal("expected a second fallback expansion to be rejected")
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

func findTargetedProbeFamily(families []TargetedProbeFamilyResult, want ProbeFamily) (TargetedProbeFamilyResult, bool) {
	for _, family := range families {
		if family.Family == want {
			return family, true
		}
	}
	return TargetedProbeFamilyResult{}, false
}
