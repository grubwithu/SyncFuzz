package environment

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// TargetUnixSocketMaterializationSchemaVersion identifies target-owned
// provenance. It is deliberately not EnvironmentMaterialization: its child
// processes live inside a retained target container and their raw PID/socket
// identities cannot be treated as portable local-fixture identities.
const TargetUnixSocketMaterializationSchemaVersion = "syncfuzz.target-environment-materialization.v1"
const TargetUnixSocketMaterializerCreator = "langgraph-target-environment-materializer-v1"
const TargetEnvironmentProfileFollowupSchemaVersion = "syncfuzz.target-environment-profile-followup.v1"

type TargetEffectWindow struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type TargetUnixSocketListener struct {
	PID              int    `json:"pid"`
	Role             string `json:"role"`
	Endpoint         string `json:"endpoint"`
	EndpointDevice   uint64 `json:"endpoint_device"`
	EndpointInode    uint64 `json:"endpoint_inode"`
	FD               int    `json:"fd"`
	SocketID         string `json:"socket_id"`
	SocketDevice     uint64 `json:"socket_device"`
	SocketInode      uint64 `json:"socket_inode"`
	ReadyMonotonicNS int64  `json:"ready_monotonic_ns"`
}

// TargetUnixSocketMaterialization is evidence emitted by the image-owned
// adapter after a controller-approved program is materialized in its cgroup.
// It proves neither eBPF W admission nor recovery-time U' use; those are
// intentionally separate gates.
type TargetUnixSocketMaterialization struct {
	SchemaVersion               string                     `json:"schema_version"`
	ProgramID                   string                     `json:"program_id"`
	SourceNativeCheckpointID    string                     `json:"source_native_checkpoint_id"`
	SourceCheckpointMonotonicNS int64                      `json:"source_checkpoint_monotonic_ns"`
	EffectWindowMonotonicNS     TargetEffectWindow         `json:"effect_window_monotonic_ns"`
	Family                      EnvironmentResourceFamily  `json:"family"`
	EndpointPath                string                     `json:"endpoint_path"`
	LogicalName                 string                     `json:"logical_name"`
	ResolutionMode              UnixSocketResolutionMode   `json:"resolution_mode"`
	ResolutionArtifactPath      string                     `json:"resolution_artifact_path"`
	ResolutionSteps             []ResolutionStep           `json:"resolution_steps"`
	UseEventArtifactPath        string                     `json:"use_event_artifact_path"`
	Listeners                   []TargetUnixSocketListener `json:"listeners"`
	ActiveListener              TargetUnixSocketListener   `json:"active_listener"`
}

// TargetEnvironmentProfileFollowup is target-owned evidence that a normal
// Agent turn occurred after E materialization and reached a later durable
// checkpoint. It intentionally records no prompt or tool-result payload;
// eBPF connect evidence and the listener-side use log are joined separately.
type TargetEnvironmentProfileFollowup struct {
	SchemaVersion                     string `json:"schema_version"`
	ProgramID                         string `json:"program_id"`
	MaterializationNativeCheckpointID string `json:"materialization_native_checkpoint_id"`
	FollowupQuerySHA256               string `json:"followup_query_sha256"`
	FollowupInvoked                   bool   `json:"followup_invoked"`
	FollowupToolResultCount           int    `json:"followup_tool_result_count"`
	AfterNativeCheckpointID           string `json:"after_native_checkpoint_id"`
	AfterNativeCheckpointMonotonicNS  int64  `json:"after_native_checkpoint_monotonic_ns"`
}

func ReadTargetEnvironmentProfileFollowup(path string) (TargetEnvironmentProfileFollowup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetEnvironmentProfileFollowup{}, fmt.Errorf("read target environment profile follow-up %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var artifact TargetEnvironmentProfileFollowup
	if err := decoder.Decode(&artifact); err != nil {
		return TargetEnvironmentProfileFollowup{}, fmt.Errorf("decode target environment profile follow-up %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return TargetEnvironmentProfileFollowup{}, fmt.Errorf("decode target environment profile follow-up %s: trailing JSON value", path)
		}
		return TargetEnvironmentProfileFollowup{}, fmt.Errorf("decode target environment profile follow-up %s: trailing data: %w", path, err)
	}
	return artifact, nil
}

func (f TargetEnvironmentProfileFollowup) ValidateFor(program EnvironmentProgram, materialization TargetUnixSocketMaterialization) error {
	if err := materialization.ValidateFor(program); err != nil {
		return err
	}
	if f.SchemaVersion != TargetEnvironmentProfileFollowupSchemaVersion || f.ProgramID != program.ProgramID || f.MaterializationNativeCheckpointID != materialization.SourceNativeCheckpointID || len(f.FollowupQuerySHA256) != 64 || !f.FollowupInvoked || f.FollowupToolResultCount <= 0 || strings.TrimSpace(f.AfterNativeCheckpointID) == "" || f.AfterNativeCheckpointMonotonicNS <= materialization.EffectWindowMonotonicNS.End {
		return fmt.Errorf("target environment profile follow-up does not prove a post-materialization normal Agent turn")
	}
	return nil
}

func ReadTargetUnixSocketMaterialization(path string) (TargetUnixSocketMaterialization, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetUnixSocketMaterialization{}, fmt.Errorf("read target Unix socket materialization %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var artifact TargetUnixSocketMaterialization
	if err := decoder.Decode(&artifact); err != nil {
		return TargetUnixSocketMaterialization{}, fmt.Errorf("decode target Unix socket materialization %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return TargetUnixSocketMaterialization{}, fmt.Errorf("decode target Unix socket materialization %s: trailing JSON value", path)
		}
		return TargetUnixSocketMaterialization{}, fmt.Errorf("decode target Unix socket materialization %s: trailing data: %w", path, err)
	}
	return artifact, nil
}

func (m TargetUnixSocketMaterialization) ValidateFor(program EnvironmentProgram) error {
	if err := program.Validate(); err != nil {
		return fmt.Errorf("validate environment program: %w", err)
	}
	binding := program.UnixSocket
	if m.SchemaVersion != TargetUnixSocketMaterializationSchemaVersion || m.ProgramID != program.ProgramID || m.Family != EnvironmentResourceFamilyUnixSocket || m.EndpointPath != binding.EndpointPath || m.LogicalName != binding.LogicalName || m.ResolutionMode != binding.ResolutionMode || m.ResolutionArtifactPath != binding.ResolutionArtifactPath || m.UseEventArtifactPath != "environment-use-events.jsonl" {
		return fmt.Errorf("target Unix socket materialization does not bind the approved environment program")
	}
	if strings.TrimSpace(m.SourceNativeCheckpointID) == "" || m.SourceCheckpointMonotonicNS <= 0 || m.EffectWindowMonotonicNS.Start < m.SourceCheckpointMonotonicNS || m.EffectWindowMonotonicNS.End < m.EffectWindowMonotonicNS.Start {
		return fmt.Errorf("target Unix socket materialization lacks an ordered native checkpoint effect window")
	}
	if err := validateTargetUnixSocketResolution(program, m.ResolutionSteps); err != nil {
		return err
	}
	if len(m.Listeners) != 2 || m.Listeners[0].Role != binding.InitialRole || m.Listeners[1].Role != binding.ActiveRole || m.ActiveListener != m.Listeners[1] {
		return fmt.Errorf("target Unix socket materialization lacks the declared initial/active listener transition")
	}
	for _, listener := range m.Listeners {
		if listener.PID <= 0 || listener.Endpoint == "" || listener.EndpointInode == 0 || listener.FD < 0 || listener.SocketInode == 0 || listener.SocketID != "socket:"+strconv.FormatUint(listener.SocketInode, 10) || listener.ReadyMonotonicNS < m.SourceCheckpointMonotonicNS || listener.ReadyMonotonicNS > m.EffectWindowMonotonicNS.End {
			return fmt.Errorf("target Unix socket materialization has invalid child listener provenance")
		}
	}
	return nil
}

// ActiveBinding translates target-owned listener provenance into the same
// semantic/run-local identity split used by the local calibration IR. The raw
// kernel socket ID remains target-specific provenance; cross-runtime
// comparison must use SemanticIdentity instead.
func (m TargetUnixSocketMaterialization) ActiveBinding() MaterializedUnixSocketBinding {
	listener := m.ActiveListener
	return MaterializedUnixSocketBinding{
		Semantic: SemanticIdentity{
			ProgramID:        m.ProgramID,
			LogicalName:      m.LogicalName,
			Role:             listener.Role,
			ResolutionSHA256: ResolutionStepsDigest(m.ResolutionSteps),
			Creator:          TargetUnixSocketMaterializerCreator,
		},
		Local: RunLocalIdentity{
			EndpointDevice: listener.EndpointDevice,
			EndpointInode:  listener.EndpointInode,
			SocketDevice:   listener.SocketDevice,
			SocketInode:    listener.SocketInode,
			HolderPID:      listener.PID,
			HolderFD:       listener.FD,
		},
		Listening: true,
	}
}

func (m TargetUnixSocketMaterialization) InitialBinding() MaterializedUnixSocketBinding {
	listener := m.Listeners[0]
	return MaterializedUnixSocketBinding{
		Semantic: SemanticIdentity{
			ProgramID:        m.ProgramID,
			LogicalName:      m.LogicalName,
			Role:             listener.Role,
			ResolutionSHA256: ResolutionStepsDigest(m.ResolutionSteps),
			Creator:          TargetUnixSocketMaterializerCreator,
		},
		Local: RunLocalIdentity{
			EndpointDevice: listener.EndpointDevice,
			EndpointInode:  listener.EndpointInode,
			SocketDevice:   listener.SocketDevice,
			SocketInode:    listener.SocketInode,
			HolderPID:      listener.PID,
			HolderFD:       listener.FD,
		},
		Listening: true,
	}
}

func validateTargetUnixSocketResolution(program EnvironmentProgram, steps []ResolutionStep) error {
	binding := program.UnixSocket
	if len(steps) < 2 {
		return fmt.Errorf("target Unix socket materialization lacks resolution steps")
	}
	for _, step := range steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("target Unix socket materialization resolution: %w", err)
		}
	}
	first := steps[0]
	last := steps[len(steps)-1]
	if first.Kind != ResolutionStepLogicalName || first.From != binding.LogicalName || last.Kind != ResolutionStepPathname || last.From != binding.EndpointPath || last.To != "unix-endpoint:"+binding.EndpointPath {
		return fmt.Errorf("target Unix socket materialization resolution does not bind the declared logical name and endpoint")
	}
	switch binding.ResolutionMode {
	case UnixSocketResolutionDirect:
		if len(steps) != 2 || first.To != binding.EndpointPath {
			return fmt.Errorf("target Unix socket direct resolution is not canonical")
		}
	case UnixSocketResolutionConfig:
		if len(steps) != 3 || first.To != string(UnixSocketResolutionConfig) || steps[1].Kind != ResolutionStepConfig || steps[1].From != binding.ResolutionKey || steps[1].To != binding.EndpointPath || steps[1].ArtifactPath != binding.ResolutionArtifactPath || len(steps[1].ValueSHA256) != 64 {
			return fmt.Errorf("target Unix socket config resolution is not canonical")
		}
	case UnixSocketResolutionAlias:
		if len(steps) != 3 || first.To != string(UnixSocketResolutionAlias) || steps[1].Kind != ResolutionStepAlias || steps[1].From != binding.ResolutionKey || steps[1].To != binding.EndpointPath || steps[1].ArtifactPath != binding.ResolutionArtifactPath || len(steps[1].ValueSHA256) != 64 {
			return fmt.Errorf("target Unix socket alias resolution is not canonical")
		}
	default:
		return fmt.Errorf("target Unix socket materialization does not support resolution mode %q", binding.ResolutionMode)
	}
	return nil
}
