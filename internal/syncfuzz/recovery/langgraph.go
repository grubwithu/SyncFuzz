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

// LangGraphWorkspaceSnapshot freezes the durable LangGraph store and the
// retained passive Unix socket metadata from the profiled workspace. The
// source is verified before every fork, then cloned into a new workspace; the
// socket itself is bind-mounted read-only because Unix socket nodes cannot be
// copied without creating a new endpoint.
type LangGraphWorkspaceSnapshot struct {
	SourceWorkspace             string `json:"source_workspace"`
	WorkspaceSHA256             string `json:"workspace_sha256"`
	CheckpointStoreRelativePath string `json:"checkpoint_store_relative_path"`
	CheckpointStoreSHA256       string `json:"checkpoint_store_sha256"`
	PassiveUnixSocketPath       string `json:"passive_unix_socket_path"`
	PassiveUnixSocketDevice     uint64 `json:"passive_unix_socket_device"`
	PassiveUnixSocketInode      uint64 `json:"passive_unix_socket_inode"`
	PassiveUnixSocketMode       uint32 `json:"passive_unix_socket_mode"`
}

func (s LangGraphWorkspaceSnapshot) Validate() error {
	if strings.TrimSpace(s.SourceWorkspace) == "" || !filepath.IsAbs(s.SourceWorkspace) || !isSHA256(s.WorkspaceSHA256) || s.CheckpointStoreRelativePath != "langgraph-checkpoints" || !isSHA256(s.CheckpointStoreSHA256) || strings.TrimSpace(s.PassiveUnixSocketPath) == "" || filepath.IsAbs(s.PassiveUnixSocketPath) || s.PassiveUnixSocketInode == 0 {
		return fmt.Errorf("LangGraph workspace snapshot is incomplete")
	}
	if _, err := workspaceChild(s.SourceWorkspace, s.PassiveUnixSocketPath); err != nil {
		return fmt.Errorf("LangGraph workspace snapshot passive socket path: %w", err)
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
	PassiveObservationID            string                                         `json:"passive_observation_id"`
	MaterializationHeadID           string                                         `json:"materialization_head_id"`
	MaterializationHeadCheckpointID string                                         `json:"materialization_head_checkpoint_id"`
	SourceThreadID                  string                                         `json:"source_thread_id"`
	SourceRuntime                   LangGraphSourceRuntime                         `json:"source_runtime"`
	WorkspaceSnapshot               LangGraphWorkspaceSnapshot                     `json:"workspace_snapshot"`
	UnixSocketProbe                 LangGraphUnixSocketProbe                       `json:"unix_socket_probe"`
	CheckpointCoordinates           map[string]LangGraphNativeCheckpointCoordinate `json:"checkpoint_coordinates"`
	AgentStateByCheckpoint          map[string]StatePresence                       `json:"agent_state_by_checkpoint"`
}

func (p LangGraphForkPlan) ValidateFor(plan RecordedPlan) error {
	if p.SchemaVersion != LangGraphForkPlanSchema || p.RecordedPlanID != plan.RecordedPlanID || p.AdapterID != plan.AdapterID || p.TargetID != plan.TargetID {
		return fmt.Errorf("LangGraph fork plan does not match recorded plan %q", plan.RecordedPlanID)
	}
	if p.AdapterID != LangGraphForkAdapterID || strings.TrimSpace(p.CandidateID) == "" || strings.TrimSpace(p.Task) == "" || strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.ContainerImage) == "" || strings.TrimSpace(p.RuntimeRoot) == "" || strings.TrimSpace(p.PassiveUnixSocketPath) == "" || p.PassiveObservationID != plan.PassiveObservationID || strings.TrimSpace(p.SourceThreadID) == "" {
		return fmt.Errorf("LangGraph fork plan requires candidate, task, model, image, runtime root, source thread, and passive Unix socket path")
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
