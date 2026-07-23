package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// MAFWorkflowForkAdapterID is intentionally distinct from the legacy
	// generic command adapter. It is registered only for an adapter that asks
	// the MAF Workflow API to restore an exact durable file checkpoint.
	MAFWorkflowForkAdapterID         = "maf-workflow"
	MAFWorkflowForkPlanSchema        = "syncfuzz.maf-workflow-fork-plan.v1"
	MAFWorkflowForkObservationSchema = "syncfuzz.maf-workflow-fork-observation.v1"
)

// MAFWorkflowForkPlan is the adapter-owned part of an opaque RecordedPlan.
// CheckpointBindings maps a V2 frontier coordinate to the exact native MAF
// file-checkpoint ID. It is deliberately separate from the legacy target-task
// and Scenario IR artifacts.
type MAFWorkflowForkPlan struct {
	SchemaVersion      string            `json:"schema_version"`
	RecordedPlanID     string            `json:"recorded_plan_id"`
	AdapterID          string            `json:"adapter_id"`
	TargetID           string            `json:"target_id"`
	TaskID             string            `json:"task_id"`
	PythonCommand      string            `json:"python_command"`
	RunnerPath         string            `json:"runner_path"`
	PreparedWorkspace  string            `json:"prepared_workspace"`
	RuntimeRoot        string            `json:"runtime_root"`
	CheckpointBindings map[string]string `json:"checkpoint_bindings"`
}

func (p MAFWorkflowForkPlan) ValidateFor(plan RecordedPlan) error {
	if p.SchemaVersion != MAFWorkflowForkPlanSchema {
		return fmt.Errorf("unsupported MAF workflow fork plan schema %q", p.SchemaVersion)
	}
	if p.RecordedPlanID != plan.RecordedPlanID || p.AdapterID != plan.AdapterID || p.TargetID != plan.TargetID {
		return fmt.Errorf("MAF workflow fork plan does not match recorded plan %q", plan.RecordedPlanID)
	}
	if p.AdapterID != MAFWorkflowForkAdapterID {
		return fmt.Errorf("MAF workflow fork plan requires adapter %q, got %q", MAFWorkflowForkAdapterID, p.AdapterID)
	}
	if strings.TrimSpace(p.TaskID) == "" || strings.TrimSpace(p.PythonCommand) == "" || strings.TrimSpace(p.RunnerPath) == "" || strings.TrimSpace(p.PreparedWorkspace) == "" || strings.TrimSpace(p.RuntimeRoot) == "" {
		return fmt.Errorf("MAF workflow fork plan requires task, Python command, runner, prepared workspace, and runtime root")
	}
	if len(p.CheckpointBindings) < 2 {
		return fmt.Errorf("MAF workflow fork plan requires at least two checkpoint bindings")
	}
	for coordinate, nativeID := range p.CheckpointBindings {
		if strings.TrimSpace(coordinate) == "" || strings.TrimSpace(nativeID) == "" {
			return fmt.Errorf("MAF workflow fork plan has an empty checkpoint binding")
		}
	}
	return nil
}

func ReadMAFWorkflowForkPlan(path string) (MAFWorkflowForkPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MAFWorkflowForkPlan{}, fmt.Errorf("read MAF workflow fork plan %s: %w", path, err)
	}
	var plan MAFWorkflowForkPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return MAFWorkflowForkPlan{}, fmt.Errorf("decode MAF workflow fork plan %s: %w", path, err)
	}
	return plan, nil
}

func WriteMAFWorkflowForkPlan(path string, plan MAFWorkflowForkPlan) error {
	if plan.SchemaVersion != MAFWorkflowForkPlanSchema {
		return fmt.Errorf("unsupported MAF workflow fork plan schema %q", plan.SchemaVersion)
	}
	return writeRecoveryJSON(path, plan)
}

type mafWorkflowForkObservationArtifact struct {
	SchemaVersion      string   `json:"schema_version"`
	TaskID             string   `json:"task_id"`
	CheckpointID       string   `json:"checkpoint_id"`
	RuntimeInstanceID  string   `json:"runtime_instance_id"`
	WorkflowRecreated  bool     `json:"workflow_object_recreated"`
	Restored           bool     `json:"restored"`
	AgentState         string   `json:"agent_state"`
	OSState            string   `json:"os_state"`
	OSStateOrigin      string   `json:"os_state_origin"`
	EffectMultiplicity string   `json:"effect_multiplicity"`
	Evidence           []string `json:"evidence"`
}

// MAFWorkflowForkExecutor starts a separate Python process per query. The
// runner clones the prepared initial workspace, recreates a MAF Workflow
// object, and calls its native checkpoint restore API with one exact ID.
type MAFWorkflowForkExecutor struct{}

func NewMAFWorkflowForkExecutor() MAFWorkflowForkExecutor {
	return MAFWorkflowForkExecutor{}
}

func (MAFWorkflowForkExecutor) ExecuteFork(ctx context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
	if request.Plan.AdapterID != MAFWorkflowForkAdapterID {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow executor cannot execute adapter %q", request.Plan.AdapterID)
	}
	forkPlan, err := ReadMAFWorkflowForkPlan(request.Plan.ExecutionArtifact)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := forkPlan.ValidateFor(request.Plan); err != nil {
		return RecoveryObservation{}, err
	}
	nativeCheckpointID, ok := forkPlan.CheckpointBindings[request.Query.CheckpointID]
	if !ok {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow fork plan has no native checkpoint binding for query coordinate %q", request.Query.CheckpointID)
	}
	if err := os.MkdirAll(forkPlan.RuntimeRoot, 0o755); err != nil {
		return RecoveryObservation{}, fmt.Errorf("create MAF workflow runtime root: %w", err)
	}
	runtimeWorkspace, err := allocateMAFWorkflowRuntimeWorkspace(forkPlan.RuntimeRoot)
	if err != nil {
		return RecoveryObservation{}, err
	}
	runtimeInstanceID := "maf-workflow-fork-" + filepath.Base(runtimeWorkspace)
	observationPath := filepath.Join(runtimeWorkspace, "maf-workflow-fork-observation.json")
	command := exec.CommandContext(
		ctx,
		forkPlan.PythonCommand,
		forkPlan.RunnerPath,
		"--mode", "fork-observe",
		"--source-workspace", forkPlan.PreparedWorkspace,
		"--workspace", runtimeWorkspace,
		"--task-id", forkPlan.TaskID,
		"--checkpoint-id", nativeCheckpointID,
		"--runtime-instance-id", runtimeInstanceID,
		"--observation-out", observationPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("run MAF workflow fork checkpoint %q: %w: %s", nativeCheckpointID, err, strings.TrimSpace(string(output)))
	}
	artifact, err := readMAFWorkflowForkObservation(observationPath)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if artifact.SchemaVersion != MAFWorkflowForkObservationSchema || artifact.CheckpointID != nativeCheckpointID || artifact.TaskID != forkPlan.TaskID || !artifact.WorkflowRecreated || !artifact.Restored {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow fork observation does not prove native restore for checkpoint %q", nativeCheckpointID)
	}
	if artifact.RuntimeInstanceID != runtimeInstanceID {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow fork observation runtime %q does not match fresh runtime %q", artifact.RuntimeInstanceID, runtimeInstanceID)
	}
	agentState, err := parseStatePresence(artifact.AgentState)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow agent state: %w", err)
	}
	osState, err := parseStatePresence(artifact.OSState)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow OS state: %w", err)
	}
	origin, err := parseStateOrigin(artifact.OSStateOrigin)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow OS state origin: %w", err)
	}
	multiplicity, err := parseEffectMultiplicity(artifact.EffectMultiplicity)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("MAF workflow effect multiplicity: %w", err)
	}
	evidence := append([]string{}, artifact.Evidence...)
	evidence = append(evidence, "adapter observation artifact: "+observationPath, "native checkpoint binding: "+nativeCheckpointID)
	return RecoveryObservation{
		SchemaVersion:        ExecutionSchemaVersion,
		QueryID:              request.Query.QueryID,
		SeedID:               request.Query.SeedID,
		Boundary:             request.Query.Boundary,
		CheckpointID:         request.Query.CheckpointID,
		RecordedPlanID:       request.Query.RecordedPlanID,
		PassiveObservationID: request.Query.PassiveObservationID,
		RuntimeInstanceID:    artifact.RuntimeInstanceID,
		AgentState:           agentState,
		OSState:              osState,
		OSStateOrigin:        origin,
		EffectMultiplicity:   multiplicity,
		Evidence:             evidence,
	}, nil
}

func allocateMAFWorkflowRuntimeWorkspace(runtimeRoot string) (string, error) {
	dir, err := os.MkdirTemp(runtimeRoot, "syncfuzz-maf-fork-")
	if err != nil {
		return "", fmt.Errorf("allocate MAF workflow runtime workspace: %w", err)
	}
	if err := os.Remove(dir); err != nil {
		return "", fmt.Errorf("prepare MAF workflow runtime workspace: %w", err)
	}
	return dir, nil
}

func readMAFWorkflowForkObservation(path string) (mafWorkflowForkObservationArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return mafWorkflowForkObservationArtifact{}, fmt.Errorf("read MAF workflow fork observation %s: %w", path, err)
	}
	var observation mafWorkflowForkObservationArtifact
	if err := json.Unmarshal(data, &observation); err != nil {
		return mafWorkflowForkObservationArtifact{}, fmt.Errorf("decode MAF workflow fork observation %s: %w", path, err)
	}
	return observation, nil
}

func parseStatePresence(value string) (StatePresence, error) {
	presence := StatePresence(value)
	if !presence.Valid() {
		return "", fmt.Errorf("invalid presence %q", value)
	}
	return presence, nil
}

func parseStateOrigin(value string) (StateOrigin, error) {
	origin := StateOrigin(value)
	if !origin.Valid() {
		return "", fmt.Errorf("invalid origin %q", value)
	}
	return origin, nil
}

func parseEffectMultiplicity(value string) (EffectMultiplicity, error) {
	multiplicity := EffectMultiplicity(value)
	if !multiplicity.Valid() {
		return "", fmt.Errorf("invalid multiplicity %q", value)
	}
	return multiplicity, nil
}

// DefaultForkExecutorRegistry contains only framework-native recovery
// adapters. The legacy command adapter is intentionally omitted.
func DefaultForkExecutorRegistry() *ForkExecutorRegistry {
	registry := NewForkExecutorRegistry()
	if err := registry.Register(MAFWorkflowForkAdapterID, NewMAFWorkflowForkExecutor()); err != nil {
		panic(err)
	}
	if err := registry.Register(LangGraphForkAdapterID, NewLangGraphForkExecutor()); err != nil {
		panic(err)
	}
	return registry
}

// Compile-time guard: the adapter must continue to satisfy the fork-only
// execution contract without inheriting any legacy target command behaviour.
var _ ForkExecutor = MAFWorkflowForkExecutor{}
