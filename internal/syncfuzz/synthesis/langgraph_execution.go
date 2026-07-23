package synthesis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

const (
	LangGraphSynthesisTargetID                = "langgraph-shell-react"
	LangGraphSynthesisAdapterID               = target.LangGraphTargetAdapterID
	LangGraphCandidateTaskID                  = "synthesis-candidate"
	LangGraphNativeCheckpointManifestArtifact = "langgraph-native-checkpoints.json"
	LangGraphNativeCheckpointManifestSchema   = "syncfuzz.langgraph-native-checkpoint-manifest.v1"
	LangGraphCandidateExecutionSchema         = "syncfuzz.langgraph-candidate-execution.v1"
	DefaultLangGraphProfileImage              = "syncfuzz-langgraph:dev"
)

// LangGraphNativeCheckpointManifest is target-owned evidence for one initial
// LangGraph runtime. It deliberately names LangGraph checkpoint IDs separately
// from controller profiling checkpoints. A later native-frontier binder must
// prove the mapping rather than passing a controller C_i directly to LangGraph.
type LangGraphNativeCheckpointManifest struct {
	SchemaVersion            string                      `json:"schema_version"`
	InitialRuntimeInstanceID string                      `json:"initial_runtime_instance_id"`
	ThreadID                 string                      `json:"thread_id"`
	CheckpointBackend        string                      `json:"checkpoint_backend"`
	Durable                  bool                        `json:"durable"`
	ClockDomain              string                      `json:"clock_domain,omitempty"`
	CheckpointDir            string                      `json:"checkpoint_dir"`
	NativeCheckpoints        []LangGraphNativeCheckpoint `json:"native_checkpoints"`
}

type LangGraphNativeCheckpoint struct {
	CheckpointID         string   `json:"checkpoint_id"`
	HistoryIndex         int      `json:"history_index"`
	MessageCount         int      `json:"message_count"`
	Next                 []string `json:"next"`
	PersistedMonotonicNS uint64   `json:"persisted_monotonic_ns,omitempty"`
}

func ReadLangGraphNativeCheckpointManifest(path string) (LangGraphNativeCheckpointManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LangGraphNativeCheckpointManifest{}, fmt.Errorf("read LangGraph native checkpoint manifest %s: %w", path, err)
	}
	var manifest LangGraphNativeCheckpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return LangGraphNativeCheckpointManifest{}, fmt.Errorf("decode LangGraph native checkpoint manifest %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return LangGraphNativeCheckpointManifest{}, err
	}
	return manifest, nil
}

func (m LangGraphNativeCheckpointManifest) Validate() error {
	if m.SchemaVersion != LangGraphNativeCheckpointManifestSchema || strings.TrimSpace(m.InitialRuntimeInstanceID) == "" || strings.TrimSpace(m.ThreadID) == "" {
		return fmt.Errorf("LangGraph native checkpoint manifest lacks schema or runtime identity")
	}
	if m.CheckpointBackend != "disk" || !m.Durable || strings.TrimSpace(m.CheckpointDir) == "" {
		return fmt.Errorf("LangGraph native checkpoint manifest does not prove a durable disk backend")
	}
	if len(m.NativeCheckpoints) == 0 {
		return fmt.Errorf("LangGraph native checkpoint manifest contains no exact checkpoint IDs")
	}
	seen := make(map[string]struct{}, len(m.NativeCheckpoints))
	for _, checkpoint := range m.NativeCheckpoints {
		if strings.TrimSpace(checkpoint.CheckpointID) == "" || checkpoint.HistoryIndex < 0 || checkpoint.MessageCount < 0 {
			return fmt.Errorf("LangGraph native checkpoint manifest has an incomplete checkpoint")
		}
		if _, exists := seen[checkpoint.CheckpointID]; exists {
			return fmt.Errorf("LangGraph native checkpoint manifest repeats checkpoint %q", checkpoint.CheckpointID)
		}
		seen[checkpoint.CheckpointID] = struct{}{}
	}
	return nil
}

// LangGraphExecutionConfig provides only execution-environment inputs. The
// generated candidate remains the sole prompt source. ProviderEnvironment is
// passed to the target process but intentionally is never serialized into any
// SyncFuzz artifact.
type LangGraphExecutionConfig struct {
	OutDir              string
	ContainerImage      string
	Timeout             time.Duration
	ObserveDelay        time.Duration
	AllowNetwork        bool
	ProviderEnvironment map[string]string
}

// LangGraphCandidateExecution links the scheduler candidate to one real,
// profiled LangGraph run. It is not a StateSeed: a candidate still has to
// satisfy EvaluateProfile and PromoteStateSeed before entering the corpus.
type LangGraphCandidateExecution struct {
	SchemaVersion                    string               `json:"schema_version"`
	CandidateID                      string               `json:"candidate_id"`
	TargetRunID                      string               `json:"target_run_id"`
	TargetRunArtifact                string               `json:"target_run_artifact"`
	NativeCheckpointManifestArtifact string               `json:"native_checkpoint_manifest_artifact"`
	NativeCheckpointRunID            string               `json:"native_checkpoint_run_id"`
	ProfileRun                       objective.ProfileRun `json:"profile_run"`
}

func (e LangGraphCandidateExecution) ValidateFor(stateObjective objective.StateObjective, candidate SynthesisCandidate) error {
	if e.SchemaVersion != LangGraphCandidateExecutionSchema || e.CandidateID != candidate.CandidateID || strings.TrimSpace(e.TargetRunID) == "" || strings.TrimSpace(e.TargetRunArtifact) == "" || strings.TrimSpace(e.NativeCheckpointManifestArtifact) == "" || strings.TrimSpace(e.NativeCheckpointRunID) == "" {
		return fmt.Errorf("LangGraph candidate execution lacks scheduler or runtime provenance")
	}
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return err
	}
	if err := e.ProfileRun.ValidateFor(stateObjective); err != nil {
		return err
	}
	if e.ProfileRun.SynthesisCandidateID != candidate.CandidateID || e.ProfileRun.TargetID != candidate.TargetID || e.ProfileRun.AdapterID != candidate.AdapterID || e.ProfileRun.NativeCheckpointRunID != e.NativeCheckpointRunID {
		return fmt.Errorf("LangGraph candidate execution profile provenance does not match its candidate")
	}
	return nil
}

// NewLangGraphSynthesisTargetRunOptions makes a real, eBPF-profiled target
// invocation for one scheduler candidate. It deliberately requires the
// dedicated container image and an explicit network opt-in in the caller;
// generic command runs and historical target tasks cannot use this path.
func NewLangGraphSynthesisTargetRunOptions(stateObjective objective.StateObjective, candidate SynthesisCandidate, config LangGraphExecutionConfig) (target.TargetRunOptions, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return target.TargetRunOptions{}, err
	}
	if candidate.TargetID != LangGraphSynthesisTargetID || candidate.AdapterID != LangGraphSynthesisAdapterID {
		return target.TargetRunOptions{}, fmt.Errorf("LangGraph synthesis execution requires target %q and adapter %q", LangGraphSynthesisTargetID, LangGraphSynthesisAdapterID)
	}
	if strings.TrimSpace(config.OutDir) == "" {
		return target.TargetRunOptions{}, fmt.Errorf("LangGraph synthesis execution requires an output directory")
	}
	image := strings.TrimSpace(config.ContainerImage)
	if image == "" {
		image = DefaultLangGraphProfileImage
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if config.ObserveDelay < 0 {
		return target.TargetRunOptions{}, fmt.Errorf("LangGraph synthesis execution requires a non-negative observation delay")
	}
	if !config.AllowNetwork {
		return target.TargetRunOptions{}, fmt.Errorf("LangGraph synthesis execution requires explicit network permission for its model provider")
	}
	return target.TargetRunOptions{
		AdapterID:               target.LangGraphTargetAdapterID,
		TargetID:                LangGraphSynthesisTargetID,
		TaskID:                  LangGraphCandidateTaskID,
		SynthesisCandidateID:    candidate.CandidateID,
		Objective:               stateObjective.ObjectiveID,
		Prompt:                  candidate.Task,
		Command:                 `python3 /opt/syncfuzz-langgraph/run_target.py --workspace "$SYNCFUZZ_WORKSPACE" --prompt-file "$SYNCFUZZ_PROMPT_FILE" --task-file "$SYNCFUZZ_TASK_FILE" --thread-id "$SYNCFUZZ_RUN_ID" --execution-policy host --checkpoint-backend disk --process-mode single --require-tool-use`,
		OutDir:                  config.OutDir,
		Timeout:                 timeout,
		ObserveDelay:            config.ObserveDelay,
		EnvKind:                 "container",
		ContainerImage:          image,
		EnableProcessProfiling:  true,
		EnableResourceProfiling: true,
		AllowNetwork:            config.AllowNetwork,
		CommandEnvironment:      copyNonEmptyEnvironment(config.ProviderEnvironment),
	}, nil
}

func copyNonEmptyEnvironment(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func ExecuteLangGraphCandidate(ctx context.Context, stateObjective objective.StateObjective, candidate SynthesisCandidate, config LangGraphExecutionConfig) (LangGraphCandidateExecution, error) {
	opts, err := NewLangGraphSynthesisTargetRunOptions(stateObjective, candidate, config)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	result, err := target.RunTarget(ctx, opts)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	if !result.Completed || result.ProfilingAnalysis == nil {
		return LangGraphCandidateExecution{}, fmt.Errorf("LangGraph candidate target run %q did not produce completed profiling evidence", result.RunID)
	}
	workspaceManifestPath, err := langGraphNativeCheckpointManifestPath(result)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	manifest, err := ReadLangGraphNativeCheckpointManifest(workspaceManifestPath)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	manifestPath := filepath.Join(result.ArtifactDir, LangGraphNativeCheckpointManifestArtifact)
	if err := copyExecutionArtifact(workspaceManifestPath, manifestPath); err != nil {
		return LangGraphCandidateExecution{}, err
	}
	profileRun, err := objective.ImportTargetProfileRun(result.ArtifactDir, stateObjective.ObjectiveID, objective.ProfileRunKindSynthesisCandidate, candidate.CandidateID)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	profileRun.NativeCheckpointRunID = manifest.InitialRuntimeInstanceID
	execution := LangGraphCandidateExecution{
		SchemaVersion:                    LangGraphCandidateExecutionSchema,
		CandidateID:                      candidate.CandidateID,
		TargetRunID:                      result.RunID,
		TargetRunArtifact:                filepath.Join(result.ArtifactDir, target.TargetResultArtifact),
		NativeCheckpointManifestArtifact: manifestPath,
		NativeCheckpointRunID:            manifest.InitialRuntimeInstanceID,
		ProfileRun:                       profileRun,
	}
	if err := execution.ValidateFor(stateObjective, candidate); err != nil {
		return LangGraphCandidateExecution{}, err
	}
	return execution, nil
}

// langGraphNativeCheckpointManifestPath uses the controller-visible workspace
// rather than TargetRunResult.Workspace. Container results intentionally
// expose /workspace as target provenance, but that path does not exist on the
// controller after the container has stopped.
func langGraphNativeCheckpointManifestPath(result *target.TargetRunResult) (string, error) {
	if result == nil || strings.TrimSpace(result.HostWorkspace) == "" {
		return "", fmt.Errorf("LangGraph target run did not expose its host workspace for native checkpoint import")
	}
	return filepath.Join(result.HostWorkspace, LangGraphNativeCheckpointManifestArtifact), nil
}

func copyExecutionArtifact(source string, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read execution artifact %s: %w", source, err)
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return fmt.Errorf("persist execution artifact %s: %w", destination, err)
	}
	return nil
}

func WriteLangGraphCandidateExecution(path string, execution LangGraphCandidateExecution) error {
	if execution.SchemaVersion != LangGraphCandidateExecutionSchema {
		return fmt.Errorf("unsupported LangGraph candidate execution schema %q", execution.SchemaVersion)
	}
	return writeJSON(path, execution)
}
