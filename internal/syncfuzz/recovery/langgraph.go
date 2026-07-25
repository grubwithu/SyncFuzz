package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	LangGraphForkAdapterID          = "langgraph"
	LangGraphForkPlanSchema         = "syncfuzz.langgraph-fork-plan.v3"
	LangGraphNativeCoordinateSchema = "syncfuzz.langgraph-native-coordinate.v1"
	LangGraphUnixSocketProbeSchema  = "syncfuzz.langgraph-unix-socket-probe.v1"
)

// LangGraphPassiveProbeMode controls how a recovery container establishes
// listener-holder multiplicity. Only the full mode can prove that no other
// process holds the profiled socket; pruned mode is intentionally weaker.
type LangGraphPassiveProbeMode string

const (
	LangGraphPassiveProbeFull   LangGraphPassiveProbeMode = "full"
	LangGraphPassiveProbePruned LangGraphPassiveProbeMode = "pruned"
)

func (m LangGraphPassiveProbeMode) Effective() LangGraphPassiveProbeMode {
	if m == "" {
		return LangGraphPassiveProbeFull
	}
	return m
}

func (m LangGraphPassiveProbeMode) Valid() bool {
	return m == LangGraphPassiveProbeFull || m == LangGraphPassiveProbePruned
}

// LangGraphNativeCheckpointCoordinate records source-checkpoint provenance.
// A snapshot fork restores SourceCheckpointID directly from a cloned durable
// store; the remaining shape guards against a malformed recorded plan.
type LangGraphNativeCheckpointCoordinate struct {
	SchemaVersion      string   `json:"schema_version"`
	SourceCheckpointID string   `json:"source_checkpoint_id"`
	HistoryIndex       int      `json:"history_index"`
	MessageCount       int      `json:"message_count"`
	Next               []string `json:"next"`
}

// LangGraphDurableToolCall identifies one tool invocation represented in the
// persisted LangGraph message state. It deliberately excludes tool arguments
// and result content: this is lifecycle provenance, not task evidence.
type LangGraphDurableToolCall struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
}

// LangGraphDurableToolLifecycle records complete tool-call and tool-result
// identities known by one durable checkpoint. A missing lifecycle object
// means an older target artifact did not record this evidence; an empty object
// means the target did record it and found no durable tool activity.
type LangGraphDurableToolLifecycle struct {
	ToolCalls     []LangGraphDurableToolCall `json:"tool_calls"`
	ToolResultIDs []string                   `json:"tool_result_ids"`
}

func (l LangGraphDurableToolLifecycle) Validate() error {
	seenCalls := make(map[string]struct{}, len(l.ToolCalls))
	for _, call := range l.ToolCalls {
		callID := strings.TrimSpace(call.ToolCallID)
		if callID == "" || strings.TrimSpace(call.ToolName) == "" {
			return fmt.Errorf("LangGraph durable tool lifecycle has an incomplete tool call")
		}
		if _, exists := seenCalls[callID]; exists {
			return fmt.Errorf("LangGraph durable tool lifecycle repeats tool call %q", callID)
		}
		seenCalls[callID] = struct{}{}
	}
	seenResults := make(map[string]struct{}, len(l.ToolResultIDs))
	for _, resultID := range l.ToolResultIDs {
		resultID = strings.TrimSpace(resultID)
		if resultID == "" {
			return fmt.Errorf("LangGraph durable tool lifecycle has an empty tool result ID")
		}
		if _, exists := seenResults[resultID]; exists {
			return fmt.Errorf("LangGraph durable tool lifecycle repeats tool result %q", resultID)
		}
		seenResults[resultID] = struct{}{}
	}
	return nil
}

func (l LangGraphDurableToolLifecycle) Clone() LangGraphDurableToolLifecycle {
	result := LangGraphDurableToolLifecycle{
		ToolCalls:     make([]LangGraphDurableToolCall, len(l.ToolCalls)),
		ToolResultIDs: make([]string, len(l.ToolResultIDs)),
	}
	copy(result.ToolCalls, l.ToolCalls)
	copy(result.ToolResultIDs, l.ToolResultIDs)
	return result
}

// LangGraphToolEffectProvenance proves that one shell-tool command span
// completely contained the timestamped eBPF effect interval. It is optional:
// legacy lifecycle artifacts lack command-span monotonic timestamps, and an
// ambiguous overlap must remain absent rather than being guessed.
type LangGraphToolEffectProvenance struct {
	ToolCallID                 string `json:"tool_call_id"`
	ToolName                   string `json:"tool_name"`
	ShellSessionID             string `json:"shell_session_id"`
	CommandSHA256              string `json:"command_sha256"`
	CommandStartedMonotonicNS  uint64 `json:"command_started_monotonic_ns"`
	CommandFinishedMonotonicNS uint64 `json:"command_finished_monotonic_ns"`
	FirstEffectMonotonicNS     uint64 `json:"first_effect_monotonic_ns"`
	LastEffectMonotonicNS      uint64 `json:"last_effect_monotonic_ns"`
}

func (p LangGraphToolEffectProvenance) Validate() error {
	if strings.TrimSpace(p.ToolCallID) == "" || strings.TrimSpace(p.ToolName) == "" || strings.TrimSpace(p.ShellSessionID) == "" || !isSHA256(p.CommandSHA256) {
		return fmt.Errorf("LangGraph tool-effect provenance is incomplete")
	}
	if p.CommandStartedMonotonicNS == 0 || p.CommandFinishedMonotonicNS < p.CommandStartedMonotonicNS || p.FirstEffectMonotonicNS == 0 || p.LastEffectMonotonicNS < p.FirstEffectMonotonicNS {
		return fmt.Errorf("LangGraph tool-effect provenance has invalid monotonic timestamps")
	}
	if p.FirstEffectMonotonicNS < p.CommandStartedMonotonicNS || p.LastEffectMonotonicNS > p.CommandFinishedMonotonicNS {
		return fmt.Errorf("LangGraph tool-effect provenance does not contain its effect interval")
	}
	return nil
}

// LangGraphSourceRuntime pins the original, still-running container whose
// network and PID namespaces hold the retained OS effect. The immutable ID
// prevents a replacement container with the same name from supplying state.
type LangGraphSourceRuntime struct {
	SchemaVersion  string `json:"schema_version"`
	Environment    string `json:"environment"`
	ContainerName  string `json:"container_name"`
	ContainerID    string `json:"container_id"`
	ContainerImage string `json:"container_image"`
}

func (r LangGraphSourceRuntime) Validate() error {
	if r.SchemaVersion != "syncfuzz.target-runtime-lease.v1" || r.Environment != "container" || strings.TrimSpace(r.ContainerName) == "" || strings.TrimSpace(r.ContainerID) == "" || strings.TrimSpace(r.ContainerImage) == "" {
		return fmt.Errorf("LangGraph source runtime is incomplete")
	}
	return nil
}

// LangGraphUnixSocketProbe binds profile-time eBPF effects to one listener
// holder in the retained source runtime. Recovery reads this namespace and
// never connects to the service.
type LangGraphUnixSocketProbe struct {
	SchemaVersion  string `json:"schema_version"`
	SocketID       string `json:"socket_id"`
	HolderPID      uint32 `json:"holder_pid"`
	HolderFD       int    `json:"holder_fd"`
	BindEffectID   string `json:"bind_effect_id"`
	ListenEffectID string `json:"listen_effect_id"`
}

func (p LangGraphUnixSocketProbe) Validate() error {
	if p.SchemaVersion != LangGraphUnixSocketProbeSchema || !strings.HasPrefix(p.SocketID, "socket:") || p.HolderPID == 0 || p.HolderFD < 0 || strings.TrimSpace(p.BindEffectID) == "" || strings.TrimSpace(p.ListenEffectID) == "" || p.BindEffectID == p.ListenEffectID {
		return fmt.Errorf("LangGraph Unix socket probe is incomplete")
	}
	return nil
}

func (c LangGraphNativeCheckpointCoordinate) Validate() error {
	if c.SchemaVersion != LangGraphNativeCoordinateSchema || strings.TrimSpace(c.SourceCheckpointID) == "" || c.HistoryIndex < 0 || c.MessageCount < 0 {
		return fmt.Errorf("LangGraph native checkpoint coordinate is incomplete")
	}
	for _, node := range c.Next {
		if strings.TrimSpace(node) == "" {
			return fmt.Errorf("LangGraph native checkpoint coordinate has an empty next node")
		}
	}
	return nil
}

// LangGraphWorkspaceSnapshot freezes the durable LangGraph store and exactly
// one retained passive resource from the profiled workspace. The source is
// verified before every fork, then cloned into a new workspace; the retained
// node is excluded from the clone and bind-mounted read-only at the same path
// so a recovery probe cannot mistake a copied replacement for source state.
type LangGraphWorkspaceSnapshot struct {
	SourceWorkspace             string `json:"source_workspace"`
	WorkspaceSHA256             string `json:"workspace_sha256"`
	CheckpointStoreRelativePath string `json:"checkpoint_store_relative_path"`
	CheckpointStoreSHA256       string `json:"checkpoint_store_sha256"`
	PassiveUnixSocketPath       string `json:"passive_unix_socket_path"`
	PassiveUnixSocketDevice     uint64 `json:"passive_unix_socket_device"`
	PassiveUnixSocketInode      uint64 `json:"passive_unix_socket_inode"`
	PassiveUnixSocketMode       uint32 `json:"passive_unix_socket_mode"`
	PassiveWorkspaceFilePath    string `json:"passive_workspace_file_path,omitempty"`
	PassiveWorkspaceFileDevice  uint64 `json:"passive_workspace_file_device,omitempty"`
	PassiveWorkspaceFileInode   uint64 `json:"passive_workspace_file_inode,omitempty"`
	PassiveWorkspaceFileMode    uint32 `json:"passive_workspace_file_mode,omitempty"`
}

func (s LangGraphWorkspaceSnapshot) Validate() error {
	if strings.TrimSpace(s.SourceWorkspace) == "" || !filepath.IsAbs(s.SourceWorkspace) || !isSHA256(s.WorkspaceSHA256) || s.CheckpointStoreRelativePath != "langgraph-checkpoints" || !isSHA256(s.CheckpointStoreSHA256) {
		return fmt.Errorf("LangGraph workspace snapshot is incomplete")
	}
	hasSocket := strings.TrimSpace(s.PassiveUnixSocketPath) != ""
	hasFile := strings.TrimSpace(s.PassiveWorkspaceFilePath) != ""
	if hasSocket == hasFile {
		return fmt.Errorf("LangGraph workspace snapshot requires exactly one passive retained resource")
	}
	if hasSocket {
		if filepath.IsAbs(s.PassiveUnixSocketPath) || s.PassiveUnixSocketInode == 0 {
			return fmt.Errorf("LangGraph workspace snapshot has incomplete passive socket metadata")
		}
		if _, err := workspaceChild(s.SourceWorkspace, s.PassiveUnixSocketPath); err != nil {
			return fmt.Errorf("LangGraph workspace snapshot passive socket path: %w", err)
		}
	}
	if hasFile {
		if filepath.IsAbs(s.PassiveWorkspaceFilePath) || s.PassiveWorkspaceFileInode == 0 {
			return fmt.Errorf("LangGraph workspace snapshot has incomplete passive workspace file metadata")
		}
		if _, err := workspaceChild(s.SourceWorkspace, s.PassiveWorkspaceFilePath); err != nil {
			return fmt.Errorf("LangGraph workspace snapshot passive workspace file path: %w", err)
		}
	}
	return nil
}

// LangGraphForkPlan freezes the source task, model identity, container image,
// passive probe and structural native coordinates. Credentials deliberately do
// not appear here: the future executor receives them only from its process
// environment.
type LangGraphForkPlan struct {
	SchemaVersion                   string                                         `json:"schema_version"`
	RecordedPlanID                  string                                         `json:"recorded_plan_id"`
	AdapterID                       string                                         `json:"adapter_id"`
	TargetID                        string                                         `json:"target_id"`
	CandidateID                     string                                         `json:"candidate_id"`
	Task                            string                                         `json:"task"`
	Model                           string                                         `json:"model"`
	ContainerImage                  string                                         `json:"container_image"`
	RuntimeRoot                     string                                         `json:"runtime_root"`
	PassiveUnixSocketPath           string                                         `json:"passive_unix_socket_path"`
	PassiveProbeMode                LangGraphPassiveProbeMode                      `json:"passive_probe_mode,omitempty"`
	PassiveObservationID            string                                         `json:"passive_observation_id"`
	MaterializationHeadID           string                                         `json:"materialization_head_id"`
	MaterializationHeadCheckpointID string                                         `json:"materialization_head_checkpoint_id"`
	SourceThreadID                  string                                         `json:"source_thread_id"`
	SourceRuntime                   LangGraphSourceRuntime                         `json:"source_runtime"`
	WorkspaceSnapshot               LangGraphWorkspaceSnapshot                     `json:"workspace_snapshot"`
	UnixSocketProbe                 LangGraphUnixSocketProbe                       `json:"unix_socket_probe"`
	CheckpointCoordinates           map[string]LangGraphNativeCheckpointCoordinate `json:"checkpoint_coordinates"`
	AgentStateByCheckpoint          map[string]StatePresence                       `json:"agent_state_by_checkpoint"`
	ToolLifecycleByCheckpoint       map[string]LangGraphDurableToolLifecycle       `json:"tool_lifecycle_by_checkpoint,omitempty"`
	ToolEffectProvenance            *LangGraphToolEffectProvenance                 `json:"tool_effect_provenance,omitempty"`
}

func (p LangGraphForkPlan) ValidateFor(plan RecordedPlan) error {
	if p.SchemaVersion != LangGraphForkPlanSchema || p.RecordedPlanID != plan.RecordedPlanID || p.AdapterID != plan.AdapterID || p.TargetID != plan.TargetID {
		return fmt.Errorf("LangGraph fork plan does not match recorded plan %q", plan.RecordedPlanID)
	}
	if p.AdapterID != LangGraphForkAdapterID || strings.TrimSpace(p.CandidateID) == "" || strings.TrimSpace(p.Task) == "" || strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.ContainerImage) == "" || strings.TrimSpace(p.RuntimeRoot) == "" || strings.TrimSpace(p.PassiveUnixSocketPath) == "" || p.PassiveObservationID != plan.PassiveObservationID || strings.TrimSpace(p.SourceThreadID) == "" {
		return fmt.Errorf("LangGraph fork plan requires candidate, task, model, image, runtime root, source thread, and passive Unix socket path")
	}
	if !p.PassiveProbeMode.Effective().Valid() {
		return fmt.Errorf("LangGraph fork plan has unsupported passive probe mode %q", p.PassiveProbeMode)
	}
	if err := p.SourceRuntime.Validate(); err != nil {
		return err
	}
	if p.SourceRuntime.ContainerImage != p.ContainerImage {
		return fmt.Errorf("LangGraph source runtime image does not match the fork plan")
	}
	if err := p.UnixSocketProbe.Validate(); err != nil {
		return err
	}
	if err := p.WorkspaceSnapshot.Validate(); err != nil {
		return err
	}
	if p.WorkspaceSnapshot.PassiveUnixSocketPath != p.PassiveUnixSocketPath {
		return fmt.Errorf("LangGraph workspace snapshot socket path does not match the fork plan")
	}
	hasMaterializationHead := strings.TrimSpace(p.MaterializationHeadID) != "" || strings.TrimSpace(p.MaterializationHeadCheckpointID) != ""
	if hasMaterializationHead {
		if strings.TrimSpace(p.MaterializationHeadID) == "" || strings.TrimSpace(p.MaterializationHeadCheckpointID) == "" {
			return fmt.Errorf("LangGraph fork plan has incomplete materialization-head identity")
		}
		if plan.MaterializationHeadID != "" && p.MaterializationHeadID != plan.MaterializationHeadID {
			return fmt.Errorf("LangGraph fork plan materialization head does not match recorded plan")
		}
		if len(p.CheckpointCoordinates) != 3 {
			return fmt.Errorf("LangGraph fork plan requires before, after, and materialization-head coordinates")
		}
	} else {
		if plan.MaterializationHeadID != "" || len(p.CheckpointCoordinates) != 2 {
			return fmt.Errorf("legacy LangGraph fork plan requires exactly two coordinates and no materialization-head contract")
		}
	}
	if len(p.AgentStateByCheckpoint) != len(p.CheckpointCoordinates) {
		return fmt.Errorf("LangGraph fork plan requires one logical-state projection per checkpoint")
	}
	for profileCheckpoint, coordinate := range p.CheckpointCoordinates {
		if strings.TrimSpace(profileCheckpoint) == "" {
			return fmt.Errorf("LangGraph fork plan has an empty profile checkpoint coordinate")
		}
		if err := coordinate.Validate(); err != nil {
			return err
		}
		state, ok := p.AgentStateByCheckpoint[profileCheckpoint]
		if !ok || (state != StatePresenceAbsent && state != StatePresencePresent) {
			return fmt.Errorf("LangGraph fork plan has no deterministic logical-state projection for checkpoint %q", profileCheckpoint)
		}
	}
	if len(p.ToolLifecycleByCheckpoint) != 0 {
		if len(p.ToolLifecycleByCheckpoint) != len(p.CheckpointCoordinates) {
			return fmt.Errorf("LangGraph fork plan requires one durable tool lifecycle projection per checkpoint when lifecycle evidence is present")
		}
		for profileCheckpoint := range p.CheckpointCoordinates {
			lifecycle, ok := p.ToolLifecycleByCheckpoint[profileCheckpoint]
			if !ok {
				return fmt.Errorf("LangGraph fork plan has no durable tool lifecycle projection for checkpoint %q", profileCheckpoint)
			}
			if err := lifecycle.Validate(); err != nil {
				return fmt.Errorf("LangGraph fork plan durable tool lifecycle for checkpoint %q: %w", profileCheckpoint, err)
			}
		}
	}
	if p.ToolEffectProvenance != nil {
		if err := p.ToolEffectProvenance.Validate(); err != nil {
			return fmt.Errorf("LangGraph fork plan tool-effect provenance: %w", err)
		}
	}
	if hasMaterializationHead {
		headCoordinate, ok := p.CheckpointCoordinates[p.MaterializationHeadCheckpointID]
		if !ok || p.AgentStateByCheckpoint[p.MaterializationHeadCheckpointID] != StatePresencePresent {
			return fmt.Errorf("LangGraph fork plan requires a present logical state at its materialization head")
		}
		for profileCheckpoint, coordinate := range p.CheckpointCoordinates {
			if profileCheckpoint != p.MaterializationHeadCheckpointID && coordinate.SourceCheckpointID == headCoordinate.SourceCheckpointID {
				return fmt.Errorf("LangGraph fork plan materialization head must use a distinct native coordinate")
			}
		}
	}
	return nil
}

func ReadLangGraphForkPlan(path string) (LangGraphForkPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LangGraphForkPlan{}, fmt.Errorf("read LangGraph fork plan %s: %w", path, err)
	}
	var plan LangGraphForkPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return LangGraphForkPlan{}, fmt.Errorf("decode LangGraph fork plan %s: %w", path, err)
	}
	return plan, nil
}

func WriteLangGraphForkPlan(path string, plan LangGraphForkPlan) error {
	if plan.SchemaVersion != LangGraphForkPlanSchema {
		return fmt.Errorf("unsupported LangGraph fork plan schema %q", plan.SchemaVersion)
	}
	return writeRecoveryJSON(path, plan)
}
