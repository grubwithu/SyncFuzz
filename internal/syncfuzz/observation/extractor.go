package observation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	targetTaskArtifactName         = "target-task.json"
	targetSnapshotLateArtifactName = "snapshot-late.json"
)

// These structures intentionally mirror only the persisted target-task JSON
// fields needed for footprint extraction. Keeping the artifact reader free of
// a target-package import lets the target runner later consume ObservationPlan
// without an import cycle.
type targetTaskArtifact struct {
	TaskID        string                  `json:"task_id"`
	ExpectedFiles []string                `json:"expected_files,omitempty"`
	Scenario      *targetScenarioArtifact `json:"scenario,omitempty"`
}

type targetScenarioArtifact struct {
	ScenarioID           string                            `json:"scenario_id"`
	TaskID               string                            `json:"task_id"`
	StateSurface         string                            `json:"state_surface,omitempty"`
	LifecycleEdge        string                            `json:"lifecycle_edge,omitempty"`
	PlantPrimitiveID     string                            `json:"plant_primitive_id,omitempty"`
	ActivationKindID     string                            `json:"activation_kind_id,omitempty"`
	OracleKindID         string                            `json:"oracle_kind_id,omitempty"`
	DefaultExpectedFiles []string                          `json:"default_expected_files,omitempty"`
	LateExpectedFiles    []string                          `json:"late_expected_files,omitempty"`
	Components           []targetScenarioComponentArtifact `json:"components,omitempty"`
	ExecutionPlan        *targetExecutionPlanArtifact      `json:"execution_plan,omitempty"`
}

type targetScenarioComponentArtifact struct {
	ComponentID string `json:"component_id"`
	Role        string `json:"role"`
	KindID      string `json:"kind_id"`
	Summary     string `json:"summary"`
}

type targetExecutionPlanArtifact struct {
	LifecycleOperationID string `json:"lifecycle_operation_id,omitempty"`
	CheckpointSelector   string `json:"checkpoint_selector,omitempty"`
	Replay               bool   `json:"replay,omitempty"`
	ForkFollowup         bool   `json:"fork_followup,omitempty"`
	CheckpointBackend    string `json:"checkpoint_backend,omitempty"`
	ProcessMode          string `json:"process_mode,omitempty"`
}

// ExtractTargetRunFootprint builds a deterministic resource footprint from a
// completed target run. It consumes normalized artifacts rather than parsing
// prompts, so it is safe to run over historical artifacts and can later merge
// a privileged OS-trace source.
func ExtractTargetRunFootprint(runDir string) (*ResourceFootprint, error) {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return nil, fmt.Errorf("target run directory is required")
	}
	task, err := readTargetTask(filepath.Join(runDir, targetTaskArtifactName))
	if err != nil {
		return nil, err
	}

	query, err := lifecycleQueryFromTargetTask(task)
	if err != nil {
		return nil, err
	}
	footprint := &ResourceFootprint{
		SchemaVersion: ResourceFootprintSchemaVersion,
		QueryID:       query.QueryID,
		Query:         query,
		ScenarioID:    query.ScenarioID,
		TaskID:        task.TaskID,
	}
	addScenarioEvidence(footprint, task)
	if err := addSnapshotEvidence(footprint, runDir); err != nil {
		return nil, err
	}
	if err := addProcessLineageEvidence(footprint, filepath.Join(runDir, "process-lineage.json")); err != nil {
		return nil, err
	}
	if err := addStateTraceEvidence(footprint, filepath.Join(runDir, core.StateTraceArtifact)); err != nil {
		return nil, err
	}
	if err := NormalizeFootprint(footprint); err != nil {
		return nil, err
	}
	return footprint, nil
}

func readTargetTask(path string) (targetTaskArtifact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return targetTaskArtifact{}, fmt.Errorf("read target task %s: %w", path, err)
	}
	var task targetTaskArtifact
	if err := json.Unmarshal(raw, &task); err != nil {
		return targetTaskArtifact{}, fmt.Errorf("decode target task %s: %w", path, err)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return targetTaskArtifact{}, fmt.Errorf("target task %s is missing task_id", path)
	}
	return task, nil
}

func addScenarioEvidence(footprint *ResourceFootprint, task targetTaskArtifact) {
	scenario := task.Scenario
	if scenario == nil {
		return
	}
	evidence := FootprintEvidence{
		Source:   EvidenceScenarioIR,
		Artifact: targetTaskArtifactName,
		Phase:    "declared",
		Detail:   "scenario IR declaration",
	}
	for _, class := range resourceClassesForScenario(scenario) {
		addClass(footprint, class, evidence)
	}
	for _, path := range append(append([]string{}, scenario.DefaultExpectedFiles...), scenario.LateExpectedFiles...) {
		addPath(footprint, PathFootprint{
			Path:          filepath.ToSlash(path),
			ResourceClass: ResourceFilesystemNamespace,
			Operations:    []string{"activate"},
			OriginPhase:   "activation",
			Evidence:      []FootprintEvidence{evidence},
		})
	}
	for _, path := range task.ExpectedFiles {
		addPath(footprint, PathFootprint{
			Path:          filepath.ToSlash(path),
			ResourceClass: ResourceFilesystemNamespace,
			Operations:    []string{"activate"},
			OriginPhase:   "activation",
			Evidence:      []FootprintEvidence{evidence},
		})
	}
}

func resourceClassesForScenario(scenario *targetScenarioArtifact) []ResourceClass {
	if scenario == nil {
		return nil
	}
	var classes []ResourceClass
	primitive := scenario.PlantPrimitiveID
	surface := scenario.StateSurface
	switch {
	case strings.Contains(primitive, "unix-listener"), strings.Contains(surface, "unix-listener"), strings.Contains(surface, "communication"):
		classes = append(classes, ResourceUnixSocket, ResourceFilesystemNamespace, ResourceProcess, ResourceFileDescriptor)
	case strings.Contains(primitive, "fd"), strings.Contains(surface, "open-fd"), strings.Contains(surface, "inherited-fd"):
		classes = append(classes, ResourceFileDescriptor, ResourceProcess)
	case strings.Contains(primitive, "background-process"), strings.Contains(surface, "process"):
		classes = append(classes, ResourceProcess)
	case strings.HasPrefix(primitive, "shell-"), strings.Contains(surface, "shell-session"):
		classes = append(classes, ResourceExecutionContext)
	case strings.HasPrefix(primitive, "workspace-"), strings.Contains(surface, "workspace"):
		classes = append(classes, ResourceFilesystemNamespace)
	}
	return classes
}

func addSnapshotEvidence(footprint *ResourceFootprint, runDir string) error {
	type snapshotArtifact struct {
		name  string
		phase string
	}
	artifacts := []snapshotArtifact{
		{name: "snapshot-before.json", phase: "before-plant"},
		{name: "snapshot-after.json", phase: "after-recovery"},
		{name: targetSnapshotLateArtifactName, phase: "after-activation"},
	}
	var previous *core.Snapshot
	for _, item := range artifacts {
		path := filepath.Join(runDir, item.name)
		snapshot, found, err := readSnapshot(path)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if previous != nil {
			addSnapshotDelta(footprint, *previous, snapshot, item.name, item.phase)
		}
		copy := snapshot
		previous = &copy
	}
	return nil
}

func readSnapshot(path string) (core.Snapshot, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return core.Snapshot{}, false, nil
	}
	if err != nil {
		return core.Snapshot{}, false, fmt.Errorf("read snapshot %s: %w", path, err)
	}
	var snapshot core.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return core.Snapshot{}, false, fmt.Errorf("decode snapshot %s: %w", path, err)
	}
	return snapshot, true, nil
}

func addSnapshotDelta(footprint *ResourceFootprint, before core.Snapshot, after core.Snapshot, artifact string, phase string) {
	beforePaths := before.Paths()
	afterPaths := after.Paths()
	for path, entry := range afterPaths {
		beforeEntry, existed := beforePaths[path]
		operations := []string{}
		if !existed {
			operations = append(operations, "create")
		} else {
			if beforeEntry.Type != entry.Type || beforeEntry.SymlinkTarget != entry.SymlinkTarget {
				operations = append(operations, "rebind")
			}
			if beforeEntry.SHA256 != entry.SHA256 || beforeEntry.Mode != entry.Mode || beforeEntry.Size != entry.Size {
				operations = append(operations, "modify")
			}
		}
		if len(operations) == 0 {
			continue
		}
		class := ResourceFilesystemNamespace
		if entry.Type == "socket" {
			class = ResourceUnixSocket
			addClass(footprint, ResourceProcess, FootprintEvidence{Source: EvidenceFilesystem, Artifact: artifact, Phase: phase, Detail: "socket pathname requires owner process evidence"})
			addClass(footprint, ResourceFileDescriptor, FootprintEvidence{Source: EvidenceFilesystem, Artifact: artifact, Phase: phase, Detail: "socket pathname requires listener FD evidence"})
		}
		addPath(footprint, PathFootprint{
			Path:          path,
			ResourceClass: class,
			Operations:    operations,
			OriginPhase:   phase,
			Evidence: []FootprintEvidence{{
				Source:   EvidenceFilesystem,
				Artifact: artifact,
				Phase:    phase,
				Detail:   strings.Join(operations, ","),
			}},
		})
	}
	for path := range beforePaths {
		if _, exists := afterPaths[path]; exists {
			continue
		}
		addPath(footprint, PathFootprint{
			Path:          path,
			ResourceClass: ResourceFilesystemNamespace,
			Operations:    []string{"delete"},
			OriginPhase:   phase,
			Evidence: []FootprintEvidence{{
				Source:   EvidenceFilesystem,
				Artifact: artifact,
				Phase:    phase,
				Detail:   "path removed",
			}},
		})
	}
}

func addProcessLineageEvidence(footprint *ResourceFootprint, path string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read process lineage %s: %w", path, err)
	}
	var report core.ProcessLineageReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("decode process lineage %s: %w", path, err)
	}
	for _, process := range append(append([]core.ProcessEntry{}, report.NewAtBoundary...), report.RemainingAfter...) {
		evidence := FootprintEvidence{Source: EvidenceProcessLineage, Artifact: filepath.Base(path), Phase: "after-recovery", Detail: "workspace process lineage"}
		addClass(footprint, ResourceProcess, evidence)
		addProcess(footprint, ProcessFootprint{
			Executable:  process.Name,
			CommandLine: process.RawCmdline,
			OriginPhase: "after-recovery",
			Evidence:    []FootprintEvidence{evidence},
		})
		for _, fd := range process.OpenFDs {
			addClass(footprint, ResourceFileDescriptor, evidence)
			if fd.Kind == "socket" {
				addClass(footprint, ResourceUnixSocket, evidence)
			}
		}
	}
	return nil
}

func addStateTraceEvidence(footprint *ResourceFootprint, path string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state trace %s: %w", path, err)
	}
	var trace core.CrossLayerTrace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return fmt.Errorf("decode state trace %s: %w", path, err)
	}
	for _, item := range trace.Observations {
		class, ok := resourceClassForStateObservation(item)
		if !ok {
			continue
		}
		addClass(footprint, class, FootprintEvidence{
			Source:   EvidenceStateTrace,
			Artifact: item.Artifact,
			Phase:    item.Phase,
			Detail:   item.StateClass,
		})
	}
	return nil
}

func resourceClassForStateObservation(item core.StateObservation) (ResourceClass, bool) {
	stateClass := strings.ToLower(item.StateClass)
	switch {
	case strings.Contains(stateClass, "process"):
		return ResourceProcess, true
	case strings.Contains(stateClass, "shell"):
		return ResourceExecutionContext, true
	case strings.Contains(stateClass, "filesystem"):
		return ResourceFilesystemNamespace, true
	default:
		return "", false
	}
}

func addClass(footprint *ResourceFootprint, class ResourceClass, evidence FootprintEvidence) {
	if class == "" {
		return
	}
	footprint.ResourceClasses = append(footprint.ResourceClasses, class)
	footprint.Evidence = append(footprint.Evidence, evidence)
}

func addPath(footprint *ResourceFootprint, path PathFootprint) {
	if path.Path == "" || path.ResourceClass == "" {
		return
	}
	footprint.Paths = append(footprint.Paths, path)
	footprint.ResourceClasses = append(footprint.ResourceClasses, path.ResourceClass)
	footprint.Operations = append(footprint.Operations, path.Operations...)
}

func addProcess(footprint *ResourceFootprint, process ProcessFootprint) {
	if process.Executable == "" && process.CommandLine == "" {
		return
	}
	footprint.Processes = append(footprint.Processes, process)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
