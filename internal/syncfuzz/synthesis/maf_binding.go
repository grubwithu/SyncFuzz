package synthesis

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const (
	MAFNativeCheckpointManifestSchema = "syncfuzz.maf-workflow-fork-manifest.v1"
	MAFNativeFrontierBindingSchema    = "syncfuzz.maf-native-frontier-binding.v1"
)

// MAFNativeCheckpointManifest is the framework-owned durable-checkpoint
// evidence emitted by the MAF prepare phase. It is intentionally much smaller
// than a target task and contains no legacy scenario metadata.
type MAFNativeCheckpointManifest struct {
	SchemaVersion            string                `json:"schema_version"`
	TaskID                   string                `json:"task_id"`
	InitialRuntimeInstanceID string                `json:"initial_runtime_instance_id"`
	NativeCheckpoints        []MAFNativeCheckpoint `json:"native_checkpoints"`
}

type MAFNativeCheckpoint struct {
	CheckpointID   string   `json:"checkpoint_id"`
	Coordinate     string   `json:"coordinate"`
	MessageTargets []string `json:"message_targets"`
	MonotonicNS    uint64   `json:"monotonic_ns,omitempty"`
}

func ReadMAFNativeCheckpointManifest(path string) (MAFNativeCheckpointManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MAFNativeCheckpointManifest{}, fmt.Errorf("read MAF native checkpoint manifest %s: %w", path, err)
	}
	var manifest MAFNativeCheckpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return MAFNativeCheckpointManifest{}, fmt.Errorf("decode MAF native checkpoint manifest %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return MAFNativeCheckpointManifest{}, err
	}
	return manifest, nil
}

func (m MAFNativeCheckpointManifest) Validate() error {
	if m.SchemaVersion != MAFNativeCheckpointManifestSchema || strings.TrimSpace(m.TaskID) == "" || strings.TrimSpace(m.InitialRuntimeInstanceID) == "" {
		return fmt.Errorf("MAF native checkpoint manifest lacks schema, task, or initial runtime identity")
	}
	seenIDs := make(map[string]struct{}, len(m.NativeCheckpoints))
	seenCoordinates := make(map[string]struct{}, len(m.NativeCheckpoints))
	for _, checkpoint := range m.NativeCheckpoints {
		if strings.TrimSpace(checkpoint.CheckpointID) == "" || strings.TrimSpace(checkpoint.Coordinate) == "" {
			return fmt.Errorf("MAF native checkpoint manifest has an incomplete checkpoint")
		}
		if _, exists := seenIDs[checkpoint.CheckpointID]; exists {
			return fmt.Errorf("MAF native checkpoint manifest repeats checkpoint %q", checkpoint.CheckpointID)
		}
		if _, exists := seenCoordinates[checkpoint.Coordinate]; exists {
			return fmt.Errorf("MAF native checkpoint manifest repeats coordinate %q", checkpoint.Coordinate)
		}
		seenIDs[checkpoint.CheckpointID] = struct{}{}
		seenCoordinates[checkpoint.Coordinate] = struct{}{}
	}
	if _, ok := seenCoordinates["before-effect"]; !ok {
		return fmt.Errorf("MAF native checkpoint manifest has no before-effect coordinate")
	}
	if _, ok := seenCoordinates["after-effect"]; !ok {
		return fmt.Errorf("MAF native checkpoint manifest has no after-effect coordinate")
	}
	return nil
}

// MAFNativeFrontierBinding proves how one validated profiling frontier maps to
// the two framework-native checkpoint files consumed by the recovery adapter.
type MAFNativeFrontierBinding struct {
	SchemaVersion             string `json:"schema_version"`
	CandidateID               string `json:"candidate_id"`
	ProfileRunID              string `json:"profile_run_id"`
	NativeCheckpointRunID     string `json:"native_checkpoint_run_id"`
	FrontierID                string `json:"frontier_id"`
	BeforeProfileCheckpointID string `json:"before_profile_checkpoint_id"`
	AfterProfileCheckpointID  string `json:"after_profile_checkpoint_id"`
	BeforeNativeCheckpointID  string `json:"before_native_checkpoint_id"`
	AfterNativeCheckpointID   string `json:"after_native_checkpoint_id"`
	ManifestArtifact          string `json:"manifest_artifact"`
}

// MAFBindingConfig fixes adapter-owned execution inputs shared by both
// recovery queries. The plan's only varying field remains the checkpoint
// coordinate supplied by RecoveryPair.
type MAFBindingConfig struct {
	PythonCommand     string
	RunnerPath        string
	PreparedWorkspace string
	RuntimeRoot       string
}

// BindMAFNativeFrontier produces the exact recovery adapter plan for one
// synthesis profile frontier. The profile and manifest must carry the same
// initial runtime identity; this rejects accidental binding of an unrelated
// controller profile to a convenient fixture checkpoint.
func BindMAFNativeFrontier(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, frontierID string, manifestPath string, manifest MAFNativeCheckpointManifest, config MAFBindingConfig) (MAFNativeFrontierBinding, recovery.MAFWorkflowForkPlan, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, err
	}
	if err := run.ValidateFor(stateObjective); err != nil {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, err
	}
	if run.SynthesisCandidateID != candidate.CandidateID || run.AdapterID != recovery.MAFWorkflowForkAdapterID || run.TargetID != candidate.TargetID || candidate.AdapterID != recovery.MAFWorkflowForkAdapterID || candidate.TargetID != "maf-workflow-checkpoint" {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, fmt.Errorf("MAF native binding requires a matching maf-workflow synthesis candidate and profile run")
	}
	if err := manifest.Validate(); err != nil {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, err
	}
	if run.NativeCheckpointRunID != manifest.InitialRuntimeInstanceID {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, fmt.Errorf("profile native checkpoint runtime %q does not match MAF manifest runtime %q", run.NativeCheckpointRunID, manifest.InitialRuntimeInstanceID)
	}
	frontier, ok := profileFrontier(run, frontierID)
	if !ok || !frontier.IsFrontier || !frontier.PersistentDelta.Changed() || len(frontier.EvidenceLinks) == 0 {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, fmt.Errorf("profile run has no validated persistent frontier %q", frontierID)
	}
	before, after := nativeCoordinates(manifest)
	if !contains(before.MessageTargets, "v2-start") || !contains(after.MessageTargets, "v2-plant") {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, fmt.Errorf("MAF manifest does not prove before/after active-message coordinates")
	}
	if strings.TrimSpace(config.PythonCommand) == "" || strings.TrimSpace(config.RunnerPath) == "" || strings.TrimSpace(config.PreparedWorkspace) == "" || strings.TrimSpace(config.RuntimeRoot) == "" {
		return MAFNativeFrontierBinding{}, recovery.MAFWorkflowForkPlan{}, fmt.Errorf("MAF native binding requires Python command, runner, prepared workspace, and runtime root")
	}
	binding := MAFNativeFrontierBinding{
		SchemaVersion:             MAFNativeFrontierBindingSchema,
		CandidateID:               candidate.CandidateID,
		ProfileRunID:              run.ProfileRunID,
		NativeCheckpointRunID:     run.NativeCheckpointRunID,
		FrontierID:                frontier.FrontierID,
		BeforeProfileCheckpointID: frontier.BeforeCheckpointID,
		AfterProfileCheckpointID:  frontier.AfterCheckpointID,
		BeforeNativeCheckpointID:  before.CheckpointID,
		AfterNativeCheckpointID:   after.CheckpointID,
		ManifestArtifact:          manifestPath,
	}
	plan := recovery.MAFWorkflowForkPlan{
		SchemaVersion:     recovery.MAFWorkflowForkPlanSchema,
		RecordedPlanID:    run.RecordedPlanID,
		AdapterID:         recovery.MAFWorkflowForkAdapterID,
		TargetID:          run.TargetID,
		TaskID:            manifest.TaskID,
		PythonCommand:     config.PythonCommand,
		RunnerPath:        config.RunnerPath,
		PreparedWorkspace: config.PreparedWorkspace,
		RuntimeRoot:       config.RuntimeRoot,
		CheckpointBindings: map[string]string{
			frontier.BeforeCheckpointID: before.CheckpointID,
			frontier.AfterCheckpointID:  after.CheckpointID,
		},
	}
	return binding, plan, nil
}

func profileFrontier(run objective.ProfileRun, frontierID string) (profiling.CheckpointInterval, bool) {
	for _, frontier := range run.CheckpointMap.Intervals {
		if frontier.FrontierID == frontierID {
			return frontier, true
		}
	}
	return profiling.CheckpointInterval{}, false
}

func nativeCoordinates(manifest MAFNativeCheckpointManifest) (MAFNativeCheckpoint, MAFNativeCheckpoint) {
	var before MAFNativeCheckpoint
	var after MAFNativeCheckpoint
	for _, checkpoint := range manifest.NativeCheckpoints {
		switch checkpoint.Coordinate {
		case "before-effect":
			before = checkpoint
		case "after-effect":
			after = checkpoint
		}
	}
	return before, after
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
