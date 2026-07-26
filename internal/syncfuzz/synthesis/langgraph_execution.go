package synthesis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
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

type LangGraphDurableToolCall = recovery.LangGraphDurableToolCall
type LangGraphDurableToolLifecycle = recovery.LangGraphDurableToolLifecycle
type LangGraphToolEffectProvenance = recovery.LangGraphToolEffectProvenance

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
	// Nil means this legacy manifest did not record message-lifecycle
	// provenance. A non-nil empty value proves no complete tool IDs were
	// durable at this checkpoint.
	DurableToolLifecycle *LangGraphDurableToolLifecycle `json:"durable_tool_lifecycle,omitempty"`
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
		if checkpoint.DurableToolLifecycle != nil {
			if err := checkpoint.DurableToolLifecycle.Validate(); err != nil {
				return fmt.Errorf("LangGraph native checkpoint %q durable tool lifecycle: %w", checkpoint.CheckpointID, err)
			}
		}
	}
	return nil
}

// LangGraphExecutionConfig provides only execution-environment inputs. The
// generated candidate remains the sole prompt source. ProviderEnvironment is
// passed to the target process but intentionally is never serialized into any
// SyncFuzz artifact.
type LangGraphExecutionConfig struct {
	OutDir         string
	ContainerImage string
	Timeout        time.Duration
	ObserveDelay   time.Duration
	AllowNetwork   bool
	// RetainRuntime keeps the profiled source container alive so recovery can
	// observe the original listener process and socket namespace.
	RetainRuntime       bool
	ProviderEnvironment map[string]string
	// EnvironmentProgram, when present, is passed as a controller-written
	// workspace artifact. The Python target materializes it between the initial
	// task and ProfileFollowupQuery, at an exact durable native checkpoint.
	// This gives the trajectory both a pre-materialization logical state and a
	// later normal Agent turn that can observe the active binding.
	EnvironmentProgram *environment.EnvironmentProgram
	// ProfileFollowupQuery is a profiling-time normal user turn, never a
	// recovery continuation. It is required for EnvironmentProgram runs so the
	// adapter does not label a merely time-later checkpoint as Agent awareness.
	ProfileFollowupQuery string
}

// LangGraphCandidateExecution links the scheduler candidate to one real,
// profiled LangGraph run. It is not a StateSeed: a candidate still has to
// satisfy EvaluateProfile and PromoteStateSeed before entering the corpus.
type LangGraphCandidateExecution struct {
	SchemaVersion                    string                            `json:"schema_version"`
	CandidateID                      string                            `json:"candidate_id"`
	TargetRunID                      string                            `json:"target_run_id"`
	TargetRunArtifact                string                            `json:"target_run_artifact"`
	NativeCheckpointManifestArtifact string                            `json:"native_checkpoint_manifest_artifact"`
	NativeCheckpointRunID            string                            `json:"native_checkpoint_run_id"`
	RuntimeContract                  recovery.LangGraphRuntimeContract `json:"runtime_contract"`
	ProfileRun                       objective.ProfileRun              `json:"profile_run"`
}

func (e LangGraphCandidateExecution) ValidateFor(stateObjective objective.StateObjective, candidate SynthesisCandidate) error {
	if e.SchemaVersion != LangGraphCandidateExecutionSchema || e.CandidateID != candidate.CandidateID || strings.TrimSpace(e.TargetRunID) == "" || strings.TrimSpace(e.TargetRunArtifact) == "" || strings.TrimSpace(e.NativeCheckpointManifestArtifact) == "" || strings.TrimSpace(e.NativeCheckpointRunID) == "" {
		return fmt.Errorf("LangGraph candidate execution lacks scheduler or runtime provenance")
	}
	if err := e.RuntimeContract.Validate(); err != nil {
		return err
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
	if e.ProfileRun.RetainedRuntime == nil || e.ProfileRun.RetainedRuntime.ContainerImageID != e.RuntimeContract.ImageID {
		return fmt.Errorf("LangGraph candidate execution profile did not retain the verified runtime image")
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
	if !config.RetainRuntime {
		return target.TargetRunOptions{}, fmt.Errorf("LangGraph synthesis execution requires retaining the profiled runtime for live OS-state recovery")
	}
	command := `python3 /opt/syncfuzz-langgraph/run_target.py --workspace "$SYNCFUZZ_WORKSPACE" --prompt-file "$SYNCFUZZ_PROMPT_FILE" --task-file "$SYNCFUZZ_TASK_FILE" --thread-id "$SYNCFUZZ_RUN_ID" --execution-policy host --checkpoint-backend disk --process-mode single --require-tool-use`
	if config.EnvironmentProgram != nil {
		if err := config.EnvironmentProgram.Validate(); err != nil {
			return target.TargetRunOptions{}, fmt.Errorf("validate LangGraph environment program: %w", err)
		}
		if strings.TrimSpace(config.ProfileFollowupQuery) == "" {
			return target.TargetRunOptions{}, fmt.Errorf("LangGraph environment program requires a profiling follow-up query")
		}
		command += ` --environment-program-file "$SYNCFUZZ_ENVIRONMENT_PROGRAM_FILE" --environment-materialization-artifact "$SYNCFUZZ_ENVIRONMENT_MATERIALIZATION_ARTIFACT"`
		command += ` --profile-followup-user-message "$SYNCFUZZ_PROFILE_FOLLOWUP_USER_MESSAGE" --require-profile-followup-tool-use`
	}
	return target.TargetRunOptions{
		AdapterID:               target.LangGraphTargetAdapterID,
		TargetID:                LangGraphSynthesisTargetID,
		TaskID:                  LangGraphCandidateTaskID,
		SynthesisCandidateID:    candidate.CandidateID,
		Objective:               stateObjective.ObjectiveID,
		Prompt:                  candidate.Task,
		Command:                 command,
		OutDir:                  config.OutDir,
		Timeout:                 timeout,
		ObserveDelay:            config.ObserveDelay,
		EnvKind:                 "container",
		ContainerImage:          image,
		EnableProcessProfiling:  true,
		EnableResourceProfiling: true,
		AllowNetwork:            config.AllowNetwork,
		RetainEnvironment:       config.RetainRuntime,
		CommandEnvironment:      copyNonEmptyEnvironment(config.ProviderEnvironment),
		EnvironmentProgram:      config.EnvironmentProgram,
		ProfileFollowupQuery:    strings.TrimSpace(config.ProfileFollowupQuery),
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
	runtimeContract, err := recovery.VerifyLangGraphRuntime(ctx, opts.ContainerImage)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	result, err := target.RunTarget(ctx, opts)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	if err := validateLangGraphCandidateProfilingEvidence(result); err != nil {
		return LangGraphCandidateExecution{}, err
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
	if config.EnvironmentProgram != nil {
		if err := validateLangGraphTargetEnvironmentMaterialization(result, manifest, *config.EnvironmentProgram); err != nil {
			return LangGraphCandidateExecution{}, err
		}
	}
	profileRun, err := objective.ImportTargetProfileRun(result.ArtifactDir, stateObjective.ObjectiveID, objective.ProfileRunKindSynthesisCandidate, candidate.CandidateID)
	if err != nil {
		return LangGraphCandidateExecution{}, err
	}
	profileRun.NativeCheckpointRunID = manifest.InitialRuntimeInstanceID
	if profileRun.RetainedRuntime == nil || profileRun.RetainedRuntime.ContainerImageID != runtimeContract.ImageID {
		if cleanupErr := ReleaseLangGraphRuntime(ctx, profileRun); cleanupErr != nil {
			return LangGraphCandidateExecution{}, fmt.Errorf("LangGraph retained source runtime does not match the verified image contract; cleanup failed: %w", cleanupErr)
		}
		return LangGraphCandidateExecution{}, fmt.Errorf("LangGraph retained source runtime does not match the verified image contract")
	}
	execution := LangGraphCandidateExecution{
		SchemaVersion:                    LangGraphCandidateExecutionSchema,
		CandidateID:                      candidate.CandidateID,
		TargetRunID:                      result.RunID,
		TargetRunArtifact:                filepath.Join(result.ArtifactDir, target.TargetResultArtifact),
		NativeCheckpointManifestArtifact: manifestPath,
		NativeCheckpointRunID:            manifest.InitialRuntimeInstanceID,
		RuntimeContract:                  runtimeContract,
		ProfileRun:                       profileRun,
	}
	if err := execution.ValidateFor(stateObjective, candidate); err != nil {
		return LangGraphCandidateExecution{}, err
	}
	return execution, nil
}

func validateLangGraphCandidateProfilingEvidence(result *target.TargetRunResult) error {
	if result == nil {
		return fmt.Errorf("LangGraph candidate target run returned no profiling result")
	}
	if !result.Completed {
		if result.CommandResult.TimedOut {
			return fmt.Errorf("LangGraph candidate target run %q timed out after %dms; incomplete profiling evidence was not retained", result.RunID, result.CommandResult.DurationMs)
		}
		return fmt.Errorf("LangGraph candidate target run %q did not complete (exit_code=%d); incomplete profiling evidence was not retained", result.RunID, result.CommandResult.ExitCode)
	}
	if result.ProfilingAnalysis == nil {
		return fmt.Errorf("LangGraph candidate target run %q completed without profiling analysis", result.RunID)
	}
	return nil
}

// validateLangGraphTargetEnvironmentMaterialization verifies only delivery,
// target-cgroup occurrence, and native-clock provenance for E. It intentionally
// does not call this a frontier/head admission: that gate additionally needs a
// target resource probe whose active listener identity is linked into the
// controller checkpoint map.
func validateLangGraphTargetEnvironmentMaterialization(result *target.TargetRunResult, manifest LangGraphNativeCheckpointManifest, program environment.EnvironmentProgram) error {
	if result == nil || strings.TrimSpace(result.ArtifactDir) == "" || result.ResourceProfiling == nil {
		return fmt.Errorf("LangGraph target environment materialization requires resource eBPF profiling")
	}
	artifact, err := environment.ReadTargetUnixSocketMaterialization(filepath.Join(result.ArtifactDir, target.TargetEnvironmentMaterializationArtifact))
	if err != nil {
		return err
	}
	if err := artifact.ValidateFor(program); err != nil {
		return fmt.Errorf("validate target environment materialization: %w", err)
	}
	followup, err := environment.ReadTargetEnvironmentProfileFollowup(filepath.Join(result.ArtifactDir, target.TargetEnvironmentProfileFollowupArtifact))
	if err != nil {
		return err
	}
	if err := followup.ValidateFor(program, artifact); err != nil {
		return fmt.Errorf("validate target environment profile follow-up: %w", err)
	}
	foundNative := false
	for _, checkpoint := range manifest.NativeCheckpoints {
		if checkpoint.CheckpointID == artifact.SourceNativeCheckpointID && checkpoint.PersistedMonotonicNS == uint64(artifact.SourceCheckpointMonotonicNS) {
			foundNative = true
			break
		}
	}
	if !foundNative {
		return fmt.Errorf("target environment materialization source checkpoint is absent from the native manifest")
	}
	activeSocketID := artifact.ActiveListener.SocketID
	activeEndpoint := "/workspace/" + filepath.ToSlash(program.UnixSocket.EndpointPath)
	seenBind, seenListen := false, false
	for _, event := range result.ResourceProfiling.Events {
		if event.MonotonicNS < uint64(artifact.EffectWindowMonotonicNS.Start) || event.MonotonicNS > uint64(artifact.EffectWindowMonotonicNS.End) || event.Resource.SocketID != activeSocketID {
			continue
		}
		switch event.Kind {
		case profiling.RawEventBind:
			if filepath.Clean(event.Resource.Path) == activeEndpoint {
				seenBind = true
			}
		case profiling.RawEventListen:
			seenListen = true
		}
	}
	if !seenBind || !seenListen {
		return fmt.Errorf("target environment materialization lacks cgroup-scoped bind(%q)/listen evidence for active socket %q", activeEndpoint, activeSocketID)
	}
	seenFollowupConnect := false
	for _, event := range result.ResourceProfiling.Events {
		if event.Kind != profiling.RawEventConnect || event.Result != 0 || filepath.Clean(event.Resource.Path) != activeEndpoint {
			continue
		}
		if event.MonotonicNS > uint64(artifact.EffectWindowMonotonicNS.End) && event.MonotonicNS <= uint64(followup.AfterNativeCheckpointMonotonicNS) {
			seenFollowupConnect = true
			break
		}
	}
	if !seenFollowupConnect {
		return fmt.Errorf("target environment profile follow-up lacks cgroup-scoped connect(%q) after materialization", activeEndpoint)
	}
	return nil
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
	if err := execution.RuntimeContract.Validate(); err != nil {
		return err
	}
	return writeJSON(path, execution)
}

// ReleaseLangGraphRuntime removes the explicitly retained source container
// after all recovery controls have completed. It verifies the immutable
// container ID first, so a replacement with the same name is never removed.
func ReleaseLangGraphRuntime(ctx context.Context, run objective.ProfileRun) error {
	if run.RetainedRuntime == nil {
		return fmt.Errorf("profile run %q has no retained LangGraph runtime", run.ProfileRunID)
	}
	if err := run.RetainedRuntime.Validate(); err != nil {
		return err
	}
	lease := run.RetainedRuntime
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.Config.Image}} {{.Image}}", lease.ContainerID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect retained LangGraph runtime %q: %w: %s", lease.ContainerID, err, strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 3 || fields[0] != lease.ContainerID || fields[1] != lease.ContainerImage || (lease.ContainerImageID != "" && fields[2] != lease.ContainerImageID) {
		return fmt.Errorf("retained LangGraph runtime %q no longer matches its recorded lease", lease.ContainerID)
	}
	output, err = exec.CommandContext(ctx, "docker", "rm", "-f", lease.ContainerID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove retained LangGraph runtime %q: %w: %s", lease.ContainerID, err, strings.TrimSpace(string(output)))
	}
	return nil
}
