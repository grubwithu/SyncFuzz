package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	LangGraphForkAdapterID            = "langgraph"
	LangGraphForkPlanSchema           = "syncfuzz.langgraph-fork-plan.v3"
	LangGraphNativeCoordinateSchema   = "syncfuzz.langgraph-native-coordinate.v1"
	LangGraphUnixSocketProbeSchema    = "syncfuzz.langgraph-unix-socket-probe.v1"
	LangGraphWorkspaceFileProbeSchema = "syncfuzz.langgraph-workspace-file-probe.v1"
	LangGraphResourceContractSchema   = "syncfuzz.langgraph-retained-resource-contract.v1"
	LangGraphWorkspaceTopologySchema  = "syncfuzz.langgraph-workspace-topology.v1"
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

// LangGraphRetainedResourceKind identifies the one source-workspace node that
// recovery may retain outside its clone. It is an execution-topology contract,
// not a task-success Oracle.
type LangGraphRetainedResourceKind string

const (
	LangGraphRetainedUnixSocket    LangGraphRetainedResourceKind = "unix-socket-listener"
	LangGraphRetainedWorkspaceFile LangGraphRetainedResourceKind = "workspace-regular-file"
)

// LangGraphRetainedResourceContract is the target-owned resource boundary
// shared by binding, snapshotting, and recovery. New plans persist it so the
// observed workspace topology is no longer inferred from Make variables.
type LangGraphRetainedResourceContract struct {
	SchemaVersion         string                        `json:"schema_version"`
	Kind                  LangGraphRetainedResourceKind `json:"kind"`
	WorkspaceRelativePath string                        `json:"workspace_relative_path"`
}

func NewLangGraphRetainedResourceContract(kind LangGraphRetainedResourceKind, workspaceRelativePath string) (LangGraphRetainedResourceContract, error) {
	contract := LangGraphRetainedResourceContract{
		SchemaVersion:         LangGraphResourceContractSchema,
		Kind:                  kind,
		WorkspaceRelativePath: filepath.Clean(strings.TrimSpace(workspaceRelativePath)),
	}
	if err := contract.Validate(); err != nil {
		return LangGraphRetainedResourceContract{}, err
	}
	return contract, nil
}

func (c LangGraphRetainedResourceContract) Validate() error {
	if c.SchemaVersion != LangGraphResourceContractSchema || (c.Kind != LangGraphRetainedUnixSocket && c.Kind != LangGraphRetainedWorkspaceFile) || strings.TrimSpace(c.WorkspaceRelativePath) == "" || filepath.IsAbs(c.WorkspaceRelativePath) {
		return fmt.Errorf("LangGraph retained resource contract is incomplete")
	}
	if _, err := workspaceChild("/workspace", c.WorkspaceRelativePath); err != nil {
		return fmt.Errorf("LangGraph retained resource contract path: %w", err)
	}
	return nil
}

// LangGraphWorkspaceTopology is a deterministic inventory of nodes which a
// recovery clone cannot safely reproduce. A clean inventory proves that the
// contract's single retained node is the only special workspace resource.
type LangGraphWorkspaceTopology struct {
	SchemaVersion    string                            `json:"schema_version"`
	SourceWorkspace  string                            `json:"source_workspace"`
	ResourceContract LangGraphRetainedResourceContract `json:"resource_contract"`
	UnexpectedNodes  []LangGraphWorkspaceTopologyNode  `json:"unexpected_nodes,omitempty"`
}

type LangGraphWorkspaceTopologyNode struct {
	WorkspaceRelativePath string `json:"workspace_relative_path"`
	Kind                  string `json:"kind"`
}

func (t LangGraphWorkspaceTopology) Validate() error {
	if t.SchemaVersion != LangGraphWorkspaceTopologySchema || strings.TrimSpace(t.SourceWorkspace) == "" || !filepath.IsAbs(t.SourceWorkspace) {
		return fmt.Errorf("LangGraph workspace topology is incomplete")
	}
	if err := t.ResourceContract.Validate(); err != nil {
		return err
	}
	previousPath := ""
	for _, node := range t.UnexpectedNodes {
		if strings.TrimSpace(node.WorkspaceRelativePath) == "" || strings.TrimSpace(node.Kind) == "" {
			return fmt.Errorf("LangGraph workspace topology has an incomplete unexpected node")
		}
		if _, err := workspaceChild(t.SourceWorkspace, node.WorkspaceRelativePath); err != nil {
			return fmt.Errorf("LangGraph workspace topology unexpected node: %w", err)
		}
		if previousPath != "" && node.WorkspaceRelativePath <= previousPath {
			return fmt.Errorf("LangGraph workspace topology unexpected nodes are not strictly ordered")
		}
		previousPath = node.WorkspaceRelativePath
	}
	return nil
}

func (t LangGraphWorkspaceTopology) ValidateForRecovery() error {
	if err := t.Validate(); err != nil {
		return err
	}
	if len(t.UnexpectedNodes) != 0 {
		return &LangGraphWorkspaceTopologyError{Topology: t}
	}
	return nil
}

// LangGraphWorkspaceTopologyError preserves an inspectable inventory for a
// generated candidate that creates an unmodelled workspace node.
type LangGraphWorkspaceTopologyError struct {
	Topology LangGraphWorkspaceTopology
}

func (e *LangGraphWorkspaceTopologyError) Error() string {
	if len(e.Topology.UnexpectedNodes) == 0 {
		return "LangGraph workspace topology violates retained-resource contract"
	}
	first := e.Topology.UnexpectedNodes[0]
	return fmt.Sprintf("LangGraph workspace topology violates retained-resource contract %s at %q: unexpected %s", e.Topology.ResourceContract.Kind, first.WorkspaceRelativePath, first.Kind)
}

func WriteLangGraphWorkspaceTopology(path string, topology LangGraphWorkspaceTopology) error {
	if err := topology.Validate(); err != nil {
		return err
	}
	return writeRecoveryJSON(path, topology)
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
	SchemaVersion    string `json:"schema_version"`
	Environment      string `json:"environment"`
	ContainerName    string `json:"container_name"`
	ContainerID      string `json:"container_id"`
	ContainerImage   string `json:"container_image"`
	ContainerImageID string `json:"container_image_id,omitempty"`
}

func (r LangGraphSourceRuntime) Validate() error {
	if r.SchemaVersion != "syncfuzz.target-runtime-lease.v1" || r.Environment != "container" || strings.TrimSpace(r.ContainerName) == "" || strings.TrimSpace(r.ContainerID) == "" || strings.TrimSpace(r.ContainerImage) == "" {
		return fmt.Errorf("LangGraph source runtime is incomplete")
	}
	if r.ContainerImageID != "" && !strings.HasPrefix(r.ContainerImageID, "sha256:") {
		return fmt.Errorf("LangGraph source runtime has an invalid container image ID")
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

// LangGraphWorkspaceFileProbe binds one or more eBPF open effects to one
// regular workspace file. It deliberately records only path/identity
// provenance, never file contents or a target-specific success condition.
type LangGraphWorkspaceFileProbe struct {
	SchemaVersion string   `json:"schema_version"`
	ResourceID    string   `json:"resource_id"`
	CanonicalPath string   `json:"canonical_path"`
	OpenEffectIDs []string `json:"open_effect_ids"`
}

func (p LangGraphWorkspaceFileProbe) Validate() error {
	if p.SchemaVersion != LangGraphWorkspaceFileProbeSchema || strings.TrimSpace(p.ResourceID) == "" || strings.TrimSpace(p.CanonicalPath) == "" || !filepath.IsAbs(p.CanonicalPath) || len(p.OpenEffectIDs) == 0 {
		return fmt.Errorf("LangGraph workspace file probe is incomplete")
	}
	seen := make(map[string]struct{}, len(p.OpenEffectIDs))
	for _, effectID := range p.OpenEffectIDs {
		effectID = strings.TrimSpace(effectID)
		if effectID == "" {
			return fmt.Errorf("LangGraph workspace file probe has an empty open effect ID")
		}
		if _, exists := seen[effectID]; exists {
			return fmt.Errorf("LangGraph workspace file probe repeats open effect %q", effectID)
		}
		seen[effectID] = struct{}{}
	}
	return nil
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
	// EphemeralObserverArtifacts are target-owned, append-only observation
	// channels (for example a retained listener accept log). They are excluded
	// from source immutability and clone digests so observation itself cannot
	// invalidate a later historical recovery control.
	EphemeralObserverArtifacts []string `json:"ephemeral_observer_artifacts,omitempty"`
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
	previous := ""
	for _, artifact := range s.EphemeralObserverArtifacts {
		if strings.TrimSpace(artifact) == "" || filepath.IsAbs(artifact) || filepath.Clean(artifact) != artifact || artifact <= previous {
			return fmt.Errorf("LangGraph workspace snapshot has invalid ephemeral observer artifacts")
		}
		if _, err := workspaceChild(s.SourceWorkspace, artifact); err != nil {
			return fmt.Errorf("LangGraph workspace snapshot ephemeral observer artifact: %w", err)
		}
		if artifact == s.PassiveResourcePath() {
			return fmt.Errorf("LangGraph workspace snapshot observer artifact overlaps the retained resource")
		}
		previous = artifact
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
	RuntimeContract                 LangGraphRuntimeContract                       `json:"runtime_contract,omitempty"`
	RuntimeRoot                     string                                         `json:"runtime_root"`
	PassiveUnixSocketPath           string                                         `json:"passive_unix_socket_path"`
	PassiveWorkspaceFilePath        string                                         `json:"passive_workspace_file_path,omitempty"`
	PassiveProbeMode                LangGraphPassiveProbeMode                      `json:"passive_probe_mode,omitempty"`
	PassiveObservationID            string                                         `json:"passive_observation_id"`
	ContinuationQuery               *ContinuationQuery                             `json:"continuation_query,omitempty"`
	MaterializationHeadID           string                                         `json:"materialization_head_id"`
	MaterializationHeadCheckpointID string                                         `json:"materialization_head_checkpoint_id"`
	SourceThreadID                  string                                         `json:"source_thread_id"`
	SourceRuntime                   LangGraphSourceRuntime                         `json:"source_runtime"`
	ResourceContract                LangGraphRetainedResourceContract              `json:"resource_contract,omitempty"`
	WorkspaceTopology               *LangGraphWorkspaceTopology                    `json:"workspace_topology,omitempty"`
	WorkspaceSnapshot               LangGraphWorkspaceSnapshot                     `json:"workspace_snapshot"`
	UnixSocketProbe                 LangGraphUnixSocketProbe                       `json:"unix_socket_probe"`
	WorkspaceFileProbe              *LangGraphWorkspaceFileProbe                   `json:"workspace_file_probe,omitempty"`
	CheckpointCoordinates           map[string]LangGraphNativeCheckpointCoordinate `json:"checkpoint_coordinates"`
	AgentStateByCheckpoint          map[string]StatePresence                       `json:"agent_state_by_checkpoint"`
	ToolLifecycleByCheckpoint       map[string]LangGraphDurableToolLifecycle       `json:"tool_lifecycle_by_checkpoint,omitempty"`
	ToolEffectProvenance            *LangGraphToolEffectProvenance                 `json:"tool_effect_provenance,omitempty"`
}

func (p LangGraphForkPlan) ValidateFor(plan RecordedPlan) error {
	if p.SchemaVersion != LangGraphForkPlanSchema || p.RecordedPlanID != plan.RecordedPlanID || p.AdapterID != plan.AdapterID || p.TargetID != plan.TargetID {
		return fmt.Errorf("LangGraph fork plan does not match recorded plan %q", plan.RecordedPlanID)
	}
	if p.AdapterID != LangGraphForkAdapterID || strings.TrimSpace(p.CandidateID) == "" || strings.TrimSpace(p.Task) == "" || strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.ContainerImage) == "" || strings.TrimSpace(p.RuntimeRoot) == "" || p.PassiveObservationID != plan.PassiveObservationID || strings.TrimSpace(p.SourceThreadID) == "" {
		return fmt.Errorf("LangGraph fork plan requires candidate, task, model, image, runtime root, source thread, and passive observation")
	}
	hasSocket := strings.TrimSpace(p.PassiveUnixSocketPath) != ""
	hasWorkspaceFile := strings.TrimSpace(p.PassiveWorkspaceFilePath) != ""
	if hasSocket == hasWorkspaceFile {
		return fmt.Errorf("LangGraph fork plan requires exactly one passive resource path")
	}
	if !p.PassiveProbeMode.Effective().Valid() {
		return fmt.Errorf("LangGraph fork plan has unsupported passive probe mode %q", p.PassiveProbeMode)
	}
	if p.ContinuationQuery != nil {
		if err := p.ContinuationQuery.Validate(); err != nil {
			return fmt.Errorf("LangGraph fork plan continuation query: %w", err)
		}
		if p.RuntimeContract.SchemaVersion == "" || !p.RuntimeContract.SupportsContinuation() {
			return fmt.Errorf("LangGraph fork plan continuation query requires the continuation-user-turn runtime capability")
		}
	}
	if p.ResourceContract.SchemaVersion != "" {
		if err := p.ResourceContract.Validate(); err != nil {
			return err
		}
		expectedKind := LangGraphRetainedUnixSocket
		expectedPath := p.PassiveUnixSocketPath
		if hasWorkspaceFile {
			expectedKind = LangGraphRetainedWorkspaceFile
			expectedPath = p.PassiveWorkspaceFilePath
		}
		if p.ResourceContract.Kind != expectedKind || p.ResourceContract.WorkspaceRelativePath != filepath.Clean(expectedPath) {
			return fmt.Errorf("LangGraph retained resource contract does not match the fork plan passive resource")
		}
		if p.WorkspaceTopology == nil {
			return fmt.Errorf("LangGraph fork plan has no workspace topology inventory")
		}
		if p.WorkspaceTopology.SourceWorkspace != p.WorkspaceSnapshot.SourceWorkspace || p.WorkspaceTopology.ResourceContract != p.ResourceContract {
			return fmt.Errorf("LangGraph workspace topology does not match the fork plan resource contract")
		}
		if err := p.WorkspaceTopology.ValidateForRecovery(); err != nil {
			return err
		}
	}
	if err := p.SourceRuntime.Validate(); err != nil {
		return err
	}
	if p.SourceRuntime.ContainerImage != p.ContainerImage {
		return fmt.Errorf("LangGraph source runtime image does not match the fork plan")
	}
	if p.RuntimeContract.SchemaVersion != "" {
		if err := p.RuntimeContract.Validate(); err != nil {
			return err
		}
		if p.SourceRuntime.ContainerImageID == "" || p.SourceRuntime.ContainerImageID != p.RuntimeContract.ImageID {
			return fmt.Errorf("LangGraph runtime contract image does not match the retained source runtime")
		}
	}
	if err := p.WorkspaceSnapshot.Validate(); err != nil {
		return err
	}
	if hasSocket {
		if err := p.UnixSocketProbe.Validate(); err != nil {
			return err
		}
		if p.WorkspaceFileProbe != nil || p.WorkspaceSnapshot.PassiveUnixSocketPath != p.PassiveUnixSocketPath {
			return fmt.Errorf("LangGraph workspace snapshot socket path does not match the fork plan")
		}
	} else {
		if p.PassiveProbeMode.Effective() != LangGraphPassiveProbeFull {
			return fmt.Errorf("LangGraph workspace file recovery supports only the full passive probe")
		}
		if p.WorkspaceFileProbe == nil {
			return fmt.Errorf("LangGraph fork plan has no workspace file probe")
		}
		if err := p.WorkspaceFileProbe.Validate(); err != nil {
			return err
		}
		if p.WorkspaceSnapshot.PassiveWorkspaceFilePath != p.PassiveWorkspaceFilePath {
			return fmt.Errorf("LangGraph workspace snapshot file path does not match the fork plan")
		}
		expectedCanonicalPath := "/workspace/" + filepath.ToSlash(filepath.Clean(p.PassiveWorkspaceFilePath))
		if p.WorkspaceFileProbe.CanonicalPath != expectedCanonicalPath {
			return fmt.Errorf("LangGraph workspace file probe path does not match the fork plan")
		}
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
