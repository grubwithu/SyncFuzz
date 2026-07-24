package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LangGraphForkExecutor clones a recorded durable store, then opens the exact
// source checkpoint in a fresh constrained container. It never reruns the
// candidate task while collecting a recovery observation.
type LangGraphForkExecutor struct{}

func NewLangGraphForkExecutor() LangGraphForkExecutor { return LangGraphForkExecutor{} }

type langGraphPassiveSocketMetadata struct {
	IsUnixSocket     bool                           `json:"is_unix_socket"`
	Device           uint64                         `json:"device"`
	Inode            uint64                         `json:"inode"`
	Mode             uint32                         `json:"mode"`
	ProbeMode        LangGraphPassiveProbeMode      `json:"probe_mode"`
	ProbeDurationNS  uint64                         `json:"probe_duration_ns"`
	ScannedProcesses int                            `json:"scanned_processes"`
	ScannedFDs       int                            `json:"scanned_fds"`
	KernelSocketID   string                         `json:"kernel_socket_id"`
	ListenerActive   bool                           `json:"listener_active"`
	ListenerCount    int                            `json:"listener_count"`
	ListenerHolders  []langGraphPassiveSocketHolder `json:"listener_holders"`
}

type langGraphPassiveSocketHolder struct {
	PID int   `json:"pid"`
	FDs []int `json:"fds"`
}

type langGraphRecoveryArtifact struct {
	RuntimeInstanceID              string   `json:"runtime_instance_id"`
	RuntimeRecreated               bool     `json:"runtime_recreated"`
	ThreadID                       string   `json:"thread_id"`
	RequestedCheckpointID          string   `json:"requested_checkpoint_id"`
	RestoredCheckpointID           string   `json:"restored_checkpoint_id"`
	RestoredCheckpointMessageCount int      `json:"restored_checkpoint_message_count"`
	RestoredCheckpointNext         []string `json:"restored_checkpoint_next"`
	PassiveUnixSocket              struct {
		BeforeFork           langGraphPassiveSocketMetadata `json:"before_fork"`
		AfterFork            langGraphPassiveSocketMetadata `json:"after_fork"`
		SameEndpointIdentity bool                           `json:"same_endpoint_identity"`
	} `json:"passive_unix_socket"`
}

func (LangGraphForkExecutor) ExecuteFork(ctx context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
	if request.Plan.AdapterID != LangGraphForkAdapterID {
		return RecoveryObservation{}, fmt.Errorf("LangGraph executor cannot execute adapter %q", request.Plan.AdapterID)
	}
	forkPlan, err := ReadLangGraphForkPlan(request.Plan.ExecutionArtifact)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := forkPlan.ValidateFor(request.Plan); err != nil {
		return RecoveryObservation{}, err
	}
	if request.Query.MaterializationHeadID != "" && request.Query.MaterializationHeadID != forkPlan.MaterializationHeadID {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery query materialization head does not match the recorded plan")
	}
	if request.Query.RetentionPolicy != "" && request.Query.RetentionPolicy != RetentionPolicyRetainRelevantOSState {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery query has unsupported retention policy %q", request.Query.RetentionPolicy)
	}
	coordinate, ok := forkPlan.CheckpointCoordinates[request.Query.CheckpointID]
	if !ok {
		return RecoveryObservation{}, fmt.Errorf("LangGraph fork plan has no coordinate for query checkpoint %q", request.Query.CheckpointID)
	}
	if err := verifyLangGraphSourceRuntime(ctx, forkPlan.SourceRuntime); err != nil {
		return RecoveryObservation{}, err
	}
	runtimeRoot, err := filepath.Abs(forkPlan.RuntimeRoot)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("resolve LangGraph runtime root: %w", err)
	}
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return RecoveryObservation{}, fmt.Errorf("create LangGraph runtime root: %w", err)
	}
	workspace, err := os.MkdirTemp(runtimeRoot, "syncfuzz-langgraph-fork-")
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("allocate LangGraph runtime workspace: %w", err)
	}
	if err := forkPlan.WorkspaceSnapshot.CloneTo(workspace); err != nil {
		return RecoveryObservation{}, fmt.Errorf("clone LangGraph recovery source snapshot: %w", err)
	}
	socketMountTarget, err := workspaceChild(workspace, forkPlan.PassiveUnixSocketPath)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := os.MkdirAll(filepath.Dir(socketMountTarget), 0o755); err != nil {
		return RecoveryObservation{}, err
	}
	if err := os.WriteFile(socketMountTarget, nil, 0o600); err != nil {
		return RecoveryObservation{}, err
	}
	encodedSnapshot, err := json.Marshal(forkPlan.WorkspaceSnapshot)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := os.WriteFile(filepath.Join(workspace, "langgraph-recovery-source-snapshot.json"), append(encodedSnapshot, '\n'), 0o644); err != nil {
		return RecoveryObservation{}, err
	}
	runtimeID := "langgraph-fork-" + filepath.Base(workspace)
	observationPath := filepath.Join(workspace, "langgraph-recovery-observation.json")
	sandboxUID, sandboxGID := langGraphSandboxUserIDs()
	if err := chownLangGraphRecoveryWorkspace(workspace, sandboxUID, sandboxGID); err != nil {
		return RecoveryObservation{}, err
	}
	args := langGraphRecoveryDockerArgs(forkPlan, workspace, runtimeID, sandboxUID, sandboxGID, coordinate.SourceCheckpointID, langGraphProviderEnvironment())
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("run LangGraph recovery container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(observationPath)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("read LangGraph recovery observation: %w", err)
	}
	var artifact langGraphRecoveryArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return RecoveryObservation{}, fmt.Errorf("decode LangGraph recovery observation: %w", err)
	}
	if artifact.RuntimeInstanceID != runtimeID || !artifact.RuntimeRecreated || artifact.ThreadID != forkPlan.SourceThreadID || artifact.RequestedCheckpointID != coordinate.SourceCheckpointID || artifact.RestoredCheckpointID != coordinate.SourceCheckpointID {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation does not prove fresh native checkpoint restore")
	}
	if artifact.RestoredCheckpointMessageCount != coordinate.MessageCount || !sameStrings(artifact.RestoredCheckpointNext, coordinate.Next) {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation did not restore the planned native state shape")
	}
	probeMode := forkPlan.PassiveProbeMode.Effective()
	if artifact.PassiveUnixSocket.BeforeFork.ProbeMode != probeMode || artifact.PassiveUnixSocket.AfterFork.ProbeMode != probeMode {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation did not use the planned %s passive probe", probeMode)
	}
	osState := StatePresenceAbsent
	listenerIdentityMatches := matchesUnixSocketIdentity(artifact.PassiveUnixSocket.AfterFork, forkPlan.UnixSocketProbe)
	listenerMatches := matchesUnixSocketProbe(artifact.PassiveUnixSocket.AfterFork, forkPlan.UnixSocketProbe)
	if socketPresent(artifact.PassiveUnixSocket.AfterFork) && listenerIdentityMatches {
		osState = StatePresencePresent
	}
	origin := StateOriginNone
	if osState == StatePresencePresent && artifact.PassiveUnixSocket.SameEndpointIdentity && matchesSnapshotSocket(artifact.PassiveUnixSocket.BeforeFork, forkPlan.WorkspaceSnapshot) && matchesSnapshotSocket(artifact.PassiveUnixSocket.AfterFork, forkPlan.WorkspaceSnapshot) {
		origin = StateOriginResidual
	} else if osState == StatePresencePresent {
		origin = StateOriginUnknown
	}
	multiplicity := EffectMultiplicityUnknown
	if origin == StateOriginResidual && probeMode == LangGraphPassiveProbeFull && listenerMatches {
		multiplicity = EffectMultiplicitySingle
	}
	agentState := forkPlan.AgentStateByCheckpoint[request.Query.CheckpointID]
	return RecoveryObservation{SchemaVersion: ExecutionSchemaVersion, QueryID: request.Query.QueryID, SeedID: request.Query.SeedID, Boundary: request.Query.Boundary, CheckpointID: request.Query.CheckpointID, RecordedPlanID: request.Query.RecordedPlanID, PassiveObservationID: request.Query.PassiveObservationID, MaterializationHeadID: request.Query.MaterializationHeadID, RetentionPolicy: request.Query.RetentionPolicy, RuntimeInstanceID: runtimeID, AgentState: agentState, OSState: osState, OSStateOrigin: origin, EffectMultiplicity: multiplicity, PassiveProbe: &PassiveProbeMetrics{Mode: probeMode, DurationNS: artifact.PassiveUnixSocket.AfterFork.ProbeDurationNS, ScannedProcesses: artifact.PassiveUnixSocket.AfterFork.ScannedProcesses, ScannedFDs: artifact.PassiveUnixSocket.AfterFork.ScannedFDs}, Evidence: []string{"LangGraph fresh container: " + runtimeID, "retained source runtime verified: " + forkPlan.SourceRuntime.ContainerID, "source snapshot verified: " + forkPlan.WorkspaceSnapshot.WorkspaceSHA256, "native checkpoint restored by exact ID: " + artifact.RestoredCheckpointID, "timestamp-validated logical state: " + string(agentState), "passive probe mode: " + string(probeMode), "passive probe scan counts: processes=" + strconv.Itoa(artifact.PassiveUnixSocket.AfterFork.ScannedProcesses) + ",fds=" + strconv.Itoa(artifact.PassiveUnixSocket.AfterFork.ScannedFDs), "eBPF-linked listener effects: " + forkPlan.UnixSocketProbe.BindEffectID + "," + forkPlan.UnixSocketProbe.ListenEffectID, "passive observation artifact: " + observationPath}}, nil
}

// langGraphRecoveryDockerArgs is kept separate from execution so the V3
// recovery contract can be asserted without a Docker daemon or model provider.
func langGraphRecoveryDockerArgs(plan LangGraphForkPlan, workspace, runtimeID string, sandboxUID, sandboxGID int, checkpointID string, providerEnvironment map[string]string) []string {
	args := []string{"run", "--rm", "--name", "syncfuzz-" + runtimeID, "--pids-limit", "128", "--memory", "256m", "--cpus", "1", "--security-opt", "no-new-privileges", "--cap-drop", "ALL", "--network", "container:" + plan.SourceRuntime.ContainerName, "--pid", "container:" + plan.SourceRuntime.ContainerName, "--user", strconv.Itoa(sandboxUID) + ":" + strconv.Itoa(sandboxGID), "-v", workspace + ":/workspace", "-v", plan.WorkspaceSnapshot.SourceSocketPath() + ":/workspace/" + plan.PassiveUnixSocketPath + ":ro", "-w", "/workspace", "-e", "LANGCHAIN_MODEL=" + plan.Model}
	for _, key := range []string{"OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY"} {
		if value := providerEnvironment[key]; value != "" {
			args = append(args, "-e", key+"="+value)
		}
	}
	return append(args, plan.ContainerImage, "python3", "/opt/syncfuzz-langgraph/run_target.py", "--workspace", "/workspace", "--prompt-file", "/workspace/target-prompt.txt", "--task-file", "/workspace/target-task.json", "--thread-id", plan.SourceThreadID, "--execution-policy", "host", "--checkpoint-backend", "disk", "--internal-phase", "resume", "--checkpoint-id", checkpointID, "--passive-fork-observe", "--passive-unix-socket-path", plan.PassiveUnixSocketPath, "--passive-unix-socket-probe-mode", string(plan.PassiveProbeMode.Effective()), "--passive-unix-socket-expected-id", plan.UnixSocketProbe.SocketID, "--passive-unix-socket-expected-holder-pid", strconv.FormatUint(uint64(plan.UnixSocketProbe.HolderPID), 10), "--passive-unix-socket-expected-holder-fd", strconv.Itoa(plan.UnixSocketProbe.HolderFD), "--runtime-instance-id", runtimeID, "--recovery-observation-artifact", "/workspace/langgraph-recovery-observation.json")
}

func langGraphProviderEnvironment() map[string]string {
	values := make(map[string]string, 4)
	for _, key := range []string{"OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	return values
}

// The controller can run through sudo to access eBPF and Docker, while each
// recovery container deliberately runs as the original unprivileged user.
// Make the private clone writable before the container starts; the source
// snapshot remains immutable and is verified before every clone.
func chownLangGraphRecoveryWorkspace(workspace string, uid, gid int) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("assign LangGraph recovery workspace %s to sandbox user: %w", path, err)
		}
		return nil
	})
}

func socketPresent(value langGraphPassiveSocketMetadata) bool {
	return value.IsUnixSocket
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func matchesSnapshotSocket(observation langGraphPassiveSocketMetadata, snapshot LangGraphWorkspaceSnapshot) bool {
	return socketPresent(observation) && observation.Device == snapshot.PassiveUnixSocketDevice && observation.Inode == snapshot.PassiveUnixSocketInode && observation.Mode == snapshot.PassiveUnixSocketMode
}

func matchesUnixSocketProbe(observation langGraphPassiveSocketMetadata, probe LangGraphUnixSocketProbe) bool {
	if !observation.ListenerActive || observation.ListenerCount != 1 || observation.KernelSocketID != probe.SocketID || len(observation.ListenerHolders) != 1 || observation.ListenerHolders[0].PID != int(probe.HolderPID) {
		return false
	}
	for _, fd := range observation.ListenerHolders[0].FDs {
		if fd == probe.HolderFD {
			return true
		}
	}
	return false
}

func matchesUnixSocketIdentity(observation langGraphPassiveSocketMetadata, probe LangGraphUnixSocketProbe) bool {
	if !observation.ListenerActive || observation.ListenerCount != 1 || observation.KernelSocketID != probe.SocketID {
		return false
	}
	for _, holder := range observation.ListenerHolders {
		if holder.PID != int(probe.HolderPID) {
			continue
		}
		for _, fd := range holder.FDs {
			if fd == probe.HolderFD {
				return true
			}
		}
	}
	return false
}

func verifyLangGraphSourceRuntime(ctx context.Context, runtime LangGraphSourceRuntime) error {
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Running}} {{.Config.Image}}", runtime.ContainerName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect retained LangGraph source runtime %q: %w: %s", runtime.ContainerName, err, strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 3 || fields[0] != runtime.ContainerID || fields[1] != "true" || fields[2] != runtime.ContainerImage {
		return fmt.Errorf("retained LangGraph source runtime %q no longer matches its recorded lease", runtime.ContainerName)
	}
	return nil
}

func langGraphSandboxUserIDs() (int, int) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid != 0 {
		return uid, gid
	}
	sudoUID, uidErr := strconv.Atoi(strings.TrimSpace(os.Getenv("SUDO_UID")))
	sudoGID, gidErr := strconv.Atoi(strings.TrimSpace(os.Getenv("SUDO_GID")))
	if uidErr != nil || gidErr != nil || sudoUID < 0 || sudoGID < 0 {
		return uid, gid
	}
	return sudoUID, sudoGID
}

var _ ForkExecutor = LangGraphForkExecutor{}
