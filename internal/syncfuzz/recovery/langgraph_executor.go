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
	"sync"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

// LangGraphForkExecutor clones a recorded durable store, then opens the exact
// source checkpoint in a fresh constrained container. Static recovery
// classification is derived before any optional continuation turn; that turn
// runs only as separately recorded experiment stimulus.
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

type langGraphPassiveWorkspaceFileMetadata struct {
	IsRegularFile   bool   `json:"is_regular_file"`
	Device          uint64 `json:"device"`
	Inode           uint64 `json:"inode"`
	Mode            uint32 `json:"mode"`
	ProbeDurationNS uint64 `json:"probe_duration_ns"`
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
	PassiveWorkspaceFile struct {
		BeforeFork       langGraphPassiveWorkspaceFileMetadata `json:"before_fork"`
		AfterFork        langGraphPassiveWorkspaceFileMetadata `json:"after_fork"`
		SameFileIdentity bool                                  `json:"same_file_identity"`
	} `json:"passive_workspace_file"`
}

// langGraphContinuationArtifact is written by run_target.py after one user
// message is injected into a freshly restored durable checkpoint. It is kept
// separate from langGraphRecoveryArtifact: a continuation can use tools and
// must never turn the passive observer itself into an active operation.
type langGraphContinuationArtifact struct {
	SchemaVersion                   string   `json:"schema_version"`
	ObservationKind                 string   `json:"observation_kind"`
	RuntimeInstanceID               string   `json:"runtime_instance_id"`
	RuntimeRecreated                bool     `json:"runtime_recreated"`
	ThreadID                        string   `json:"thread_id"`
	RequestedCheckpointID           string   `json:"requested_checkpoint_id"`
	RestoredCheckpointID            string   `json:"restored_checkpoint_id"`
	RestoredCheckpointMessageCount  int      `json:"restored_checkpoint_message_count"`
	RestoredCheckpointNext          []string `json:"restored_checkpoint_next"`
	ContinuationQueryID             string   `json:"continuation_query_id"`
	ContinuationQuerySHA256         string   `json:"continuation_query_sha256"`
	ContinuationUserMessage         string   `json:"continuation_user_message"`
	ContinuationInvoked             bool     `json:"continuation_invoked"`
	ContinuationUserTurnCount       int      `json:"continuation_user_turn_count"`
	ContinuationAIToolCallCount     int      `json:"continuation_ai_tool_call_count"`
	ContinuationToolResultCount     int      `json:"continuation_tool_result_count"`
	PostContinuationCheckpointCount int      `json:"post_continuation_checkpoint_count"`
	PreEvidence                     []string `json:"pre_evidence"`
	PostEvidence                    []string `json:"post_evidence"`
}

type langGraphRecoveryWorkspace struct {
	Path                 string
	RuntimeID            string
	PassiveObservation   string
	ContinuationArtifact string
	SandboxUID           int
	SandboxGID           int
}

type langGraphRecoveryResourceTrace struct {
	ScopeArtifact  string
	EventsArtifact string
	Scope          profiling.ProfilingScope
	Events         []profiling.RawEvent
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
	if request.ContinuationQuery == nil {
		if request.Query.ContinuationQueryID != "" {
			return RecoveryObservation{}, fmt.Errorf("LangGraph recovery query binds continuation %q without a frozen query", request.Query.ContinuationQueryID)
		}
	} else {
		if err := request.ContinuationQuery.Validate(); err != nil {
			return RecoveryObservation{}, err
		}
		if request.Query.ContinuationQueryID != request.ContinuationQuery.ContinuationQueryID {
			return RecoveryObservation{}, fmt.Errorf("LangGraph recovery query continuation does not match the frozen query")
		}
	}
	if !sameContinuationQuery(forkPlan.ContinuationQuery, request.ContinuationQuery) {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery set continuation does not match the frozen fork plan")
	}
	coordinate, ok := forkPlan.CheckpointCoordinates[request.Query.CheckpointID]
	if !ok {
		return RecoveryObservation{}, fmt.Errorf("LangGraph fork plan has no coordinate for query checkpoint %q", request.Query.CheckpointID)
	}
	if err := verifyLangGraphSourceRuntime(ctx, forkPlan.SourceRuntime); err != nil {
		return RecoveryObservation{}, err
	}
	if forkPlan.RuntimeContract.SchemaVersion != "" {
		actualContract, err := VerifyLangGraphRuntime(ctx, forkPlan.RuntimeContract.ImageID)
		if err != nil {
			return RecoveryObservation{}, err
		}
		if !actualContract.Matches(forkPlan.RuntimeContract) {
			return RecoveryObservation{}, fmt.Errorf("LangGraph recovery image no longer matches the profiled runtime contract")
		}
	}
	if request.ContinuationQuery != nil && !forkPlan.RuntimeContract.SupportsContinuation() {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery plan runtime does not advertise continuation-user-turn-v1")
	}
	passiveWorkspace, err := prepareLangGraphRecoveryWorkspace(forkPlan)
	if err != nil {
		return RecoveryObservation{}, err
	}
	args := langGraphRecoveryDockerArgsWithContinuation(forkPlan, passiveWorkspace.Path, passiveWorkspace.RuntimeID, passiveWorkspace.SandboxUID, passiveWorkspace.SandboxGID, coordinate.SourceCheckpointID, langGraphProviderEnvironment(), request.ContinuationQuery)
	var recoveryTrace *langGraphRecoveryResourceTrace
	var output []byte
	if request.ContinuationQuery != nil && langGraphForkPlanHasEnvironmentProgram(forkPlan) {
		output, recoveryTrace, err = runProfiledLangGraphRecoveryContainer(ctx, forkPlan, passiveWorkspace, args)
	} else {
		output, err = exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	}
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("run LangGraph recovery container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(passiveWorkspace.PassiveObservation)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("read LangGraph recovery observation: %w", err)
	}
	var artifact langGraphRecoveryArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return RecoveryObservation{}, fmt.Errorf("decode LangGraph recovery observation: %w", err)
	}
	if artifact.RuntimeInstanceID != passiveWorkspace.RuntimeID || !artifact.RuntimeRecreated || artifact.ThreadID != forkPlan.SourceThreadID || artifact.RequestedCheckpointID != coordinate.SourceCheckpointID || artifact.RestoredCheckpointID != coordinate.SourceCheckpointID {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation does not prove fresh native checkpoint restore")
	}
	if artifact.RestoredCheckpointMessageCount != coordinate.MessageCount || !sameStrings(artifact.RestoredCheckpointNext, coordinate.Next) {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation did not restore the planned native state shape")
	}
	probeMode := forkPlan.PassiveProbeMode.Effective()
	if artifact.PassiveUnixSocket.BeforeFork.ProbeMode != probeMode || artifact.PassiveUnixSocket.AfterFork.ProbeMode != probeMode {
		if forkPlan.PassiveUnixSocketPath != "" {
			return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation did not use the planned %s passive probe", probeMode)
		}
	}
	osState, origin, multiplicity, passiveMetrics, passiveEvidence, err := langGraphPassiveRecoveryState(forkPlan, artifact, probeMode, request.ContinuationQuery != nil)
	if err != nil {
		return RecoveryObservation{}, err
	}
	continuationEvidence, err := readLangGraphContinuationEvidence(passiveWorkspace.ContinuationArtifact, passiveWorkspace.RuntimeID, forkPlan, coordinate, request.Query, request.ContinuationQuery)
	if err != nil {
		return RecoveryObservation{}, err
	}
	agentState := forkPlan.AgentStateByCheckpoint[request.Query.CheckpointID]
	evidence := []string{"LangGraph fresh container: " + passiveWorkspace.RuntimeID, "retained source runtime verified: " + forkPlan.SourceRuntime.ContainerID, "source snapshot verified: " + forkPlan.WorkspaceSnapshot.WorkspaceSHA256, "native checkpoint restored by exact ID: " + artifact.RestoredCheckpointID, "timestamp-validated logical state: " + string(agentState), "passive probe mode: " + string(probeMode)}
	evidence = append(evidence, passiveEvidence...)
	var environmentUseEvidence *EnvironmentUseEvidence
	if recoveryTrace != nil {
		useEvidence, typedUseEvidence, err := validateLangGraphRecoveryEnvironmentUse(forkPlan, recoveryTrace)
		if err != nil {
			return RecoveryObservation{}, err
		}
		environmentUseEvidence = typedUseEvidence
		evidence = append(evidence, "recovery cgroup-scoped resource events: "+recoveryTrace.EventsArtifact, "recovery cgroup scope: "+recoveryTrace.ScopeArtifact)
		evidence = append(evidence, useEvidence...)
		if continuationEvidence != nil {
			continuationEvidence.PostEvidence = append(continuationEvidence.PostEvidence, useEvidence...)
		}
	}
	evidence = append(evidence, "passive observation artifact: "+passiveWorkspace.PassiveObservation)
	return RecoveryObservation{SchemaVersion: ExecutionSchemaVersion, QueryID: request.Query.QueryID, SeedID: request.Query.SeedID, Boundary: request.Query.Boundary, CheckpointID: request.Query.CheckpointID, RecordedPlanID: request.Query.RecordedPlanID, PassiveObservationID: request.Query.PassiveObservationID, MaterializationHeadID: request.Query.MaterializationHeadID, RetentionPolicy: request.Query.RetentionPolicy, RuntimeInstanceID: passiveWorkspace.RuntimeID, AgentState: agentState, OSState: osState, OSStateOrigin: origin, EffectMultiplicity: multiplicity, PassiveProbe: passiveMetrics, ContinuationEvidence: continuationEvidence, EnvironmentUseEvidence: environmentUseEvidence, Evidence: evidence}, nil
}

type langGraphListenerUseEvent struct {
	SchemaVersion        string `json:"schema_version"`
	MonotonicNS          uint64 `json:"monotonic_ns"`
	Role                 string `json:"role"`
	Endpoint             string `json:"endpoint"`
	PeerPID              int    `json:"peer_pid"`
	RequestBytes         int    `json:"request_bytes"`
	RequestSHA256        string `json:"request_sha256"`
	ResponseSent         bool   `json:"response_sent"`
	ResponseAcknowledged bool   `json:"response_acknowledged"`
}

// validateLangGraphRecoveryEnvironmentUse joins an active recovery-cgroup
// AF_UNIX connect to the retained listener's own role-tagged accept record.
// The listener log deliberately stores no request payload; role and endpoint
// are enough to establish typed dependence on the active E binding.
func validateLangGraphRecoveryEnvironmentUse(plan LangGraphForkPlan, trace *langGraphRecoveryResourceTrace) ([]string, *EnvironmentUseEvidence, error) {
	if trace == nil || len(trace.Events) == 0 {
		return nil, nil, fmt.Errorf("recovery environment use has no cgroup-scoped resource events")
	}
	program, err := environment.ReadEnvironmentProgram(filepath.Join(plan.WorkspaceSnapshot.SourceWorkspace, "environment-program.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read recovery environment program: %w", err)
	}
	materialization, err := environment.ReadTargetUnixSocketMaterialization(filepath.Join(plan.WorkspaceSnapshot.SourceWorkspace, "environment-materialization.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read recovery environment materialization: %w", err)
	}
	if err := materialization.ValidateFor(program); err != nil {
		return nil, nil, fmt.Errorf("validate recovery environment materialization: %w", err)
	}
	expectedPath := "/workspace/" + filepath.ToSlash(program.UnixSocket.EndpointPath)
	var (
		firstNS      uint64
		lastNS       uint64
		connectCount int
	)
	for _, event := range trace.Events {
		if firstNS == 0 || event.MonotonicNS < firstNS {
			firstNS = event.MonotonicNS
		}
		if event.MonotonicNS > lastNS {
			lastNS = event.MonotonicNS
		}
		if event.Kind == profiling.RawEventConnect && event.Result == 0 && filepath.Clean(event.Resource.Path) == expectedPath {
			connectCount++
		}
	}
	if connectCount == 0 {
		return nil, nil, fmt.Errorf("recovery cgroup did not connect to environment endpoint %q", expectedPath)
	}
	data, err := os.ReadFile(filepath.Join(plan.WorkspaceSnapshot.SourceWorkspace, materialization.UseEventArtifactPath))
	if err != nil {
		return nil, nil, fmt.Errorf("read retained listener use events: %w", err)
	}
	matchedAccepts := 0
	var matchedUse *langGraphListenerUseEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event langGraphListenerUseEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, nil, fmt.Errorf("decode retained listener use event: %w", err)
		}
		if event.SchemaVersion == "syncfuzz.environment-listener-use.v1" && event.MonotonicNS >= firstNS && event.MonotonicNS <= lastNS && event.Role == materialization.ActiveListener.Role && event.Endpoint == expectedPath && event.PeerPID > 0 && event.RequestBytes > 0 && validSHA256Hex(event.RequestSHA256) && event.ResponseSent && event.ResponseAcknowledged {
			matchedAccepts++
			matched := event
			matchedUse = &matched
		}
	}
	if matchedAccepts == 0 {
		return nil, nil, fmt.Errorf("recovery endpoint connect has no active-listener role-tagged completed exchange record")
	}
	connectEventIDs := make([]string, 0, connectCount)
	for _, event := range trace.Events {
		if event.Kind == profiling.RawEventConnect && event.Result == 0 && filepath.Clean(event.Resource.Path) == expectedPath {
			connectEventIDs = append(connectEventIDs, event.EventID)
		}
	}
	if matchedUse == nil {
		return nil, nil, fmt.Errorf("recovery endpoint connect has no completed listener use record")
	}
	active := materialization.ActiveListener
	typed := &EnvironmentUseEvidence{
		SchemaVersion:          EnvironmentUseEvidenceSchemaVersion,
		Family:                 "unix-socket",
		ProgramID:              program.ProgramID,
		LogicalName:            program.UnixSocket.LogicalName,
		ResolvedEndpointPath:   expectedPath,
		ConnectEventIDs:        connectEventIDs,
		RequestSHA256:          matchedUse.RequestSHA256,
		CompletedExchange:      true,
		ListenerRole:           active.Role,
		ListenerPID:            active.PID,
		ListenerFD:             active.FD,
		ListenerSocketID:       active.SocketID,
		ListenerEndpointDevice: active.EndpointDevice,
		ListenerEndpointInode:  active.EndpointInode,
		ListenerSocketDevice:   active.SocketDevice,
		ListenerSocketInode:    active.SocketInode,
	}
	if err := typed.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate recovery environment use: %w", err)
	}
	return []string{
		"recovery environment use: cgroup AF_UNIX connect to " + expectedPath,
		"recovery environment use: active listener role " + materialization.ActiveListener.Role + " completed " + strconv.Itoa(matchedAccepts) + " acknowledged request/response exchange(s)",
	}, typed, nil
}

func validSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func langGraphForkPlanHasEnvironmentProgram(plan LangGraphForkPlan) bool {
	_, err := os.Stat(filepath.Join(plan.WorkspaceSnapshot.SourceWorkspace, "environment-program.json"))
	return err == nil
}

// runProfiledLangGraphRecoveryContainer creates a gated recovery container so
// the host collector is attached to the recovery cgroup before the restored
// agent can execute its continuation. This is intentionally distinct from
// profile-time W collection: these events are U' candidate evidence.
func runProfiledLangGraphRecoveryContainer(ctx context.Context, plan LangGraphForkPlan, workspace langGraphRecoveryWorkspace, runArgs []string) ([]byte, *langGraphRecoveryResourceTrace, error) {
	if len(runArgs) == 0 || runArgs[0] != "run" {
		return nil, nil, fmt.Errorf("profiled LangGraph recovery requires docker run arguments")
	}
	gate := filepath.Join(workspace.Path, ".syncfuzz-recovery-start")
	if err := os.Remove(gate); err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("clear LangGraph recovery start gate: %w", err)
	}
	imageIndex := -1
	image := langGraphRecoveryContainerImage(plan)
	for index, value := range runArgs {
		if value == image {
			imageIndex = index
			break
		}
	}
	if imageIndex < 0 {
		return nil, nil, fmt.Errorf("profiled LangGraph recovery could not locate its container image")
	}
	command := append([]string(nil), runArgs[imageIndex+1:]...)
	gatedRunArgs := append([]string{}, runArgs[:imageIndex+1]...)
	gatedRunArgs = append(gatedRunArgs, "/bin/sh", "-c", "while [ ! -f /workspace/.syncfuzz-recovery-start ]; do sleep 0.02; done; exec \"$@\"", "syncfuzz-recovery-gate")
	gatedRunArgs = append(gatedRunArgs, command...)
	createArgs := []string{"create"}
	for _, value := range gatedRunArgs[1:] {
		if value != "--rm" {
			createArgs = append(createArgs, value)
		}
	}
	containerName := "syncfuzz-" + workspace.RuntimeID
	created := false
	defer func() {
		if created {
			_ = exec.CommandContext(context.Background(), "docker", "rm", "-f", containerName).Run()
		}
	}()
	if output, err := exec.CommandContext(ctx, "docker", createArgs...).CombinedOutput(); err != nil {
		return output, nil, fmt.Errorf("create gated LangGraph recovery container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	created = true
	if output, err := exec.CommandContext(ctx, "docker", "start", containerName).CombinedOutput(); err != nil {
		return output, nil, fmt.Errorf("start gated LangGraph recovery container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	scope, err := environment.ResolveContainerProfilingScope(ctx, containerName, workspace.RuntimeID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve recovery eBPF cgroup scope: %w", err)
	}
	collector, err := profiling.StartResourceCollector(scope)
	if err != nil {
		return nil, nil, fmt.Errorf("start recovery eBPF resource collector: %w", err)
	}
	var (
		mu      sync.Mutex
		events  []profiling.RawEvent
		readErr error
		done    = make(chan struct{})
	)
	go func() {
		defer close(done)
		for {
			event, readErrValue := collector.Read()
			if readErrValue != nil {
				if !profiling.IsResourceCollectorClosed(readErrValue) {
					mu.Lock()
					readErr = readErrValue
					mu.Unlock()
				}
				return
			}
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
	}()
	if err := os.WriteFile(gate, []byte("release\n"), 0o600); err != nil {
		_ = collector.Close()
		<-done
		return nil, nil, fmt.Errorf("release LangGraph recovery start gate: %w", err)
	}
	output, waitErr := exec.CommandContext(ctx, "docker", "wait", containerName).CombinedOutput()
	if closeErr := collector.Close(); closeErr != nil && waitErr == nil {
		waitErr = fmt.Errorf("close recovery eBPF resource collector: %w", closeErr)
	}
	<-done
	mu.Lock()
	captured := append([]profiling.RawEvent(nil), events...)
	collectorReadErr := readErr
	mu.Unlock()
	if collectorReadErr != nil && waitErr == nil {
		waitErr = fmt.Errorf("read recovery eBPF resource collector: %w", collectorReadErr)
	}
	trace := &langGraphRecoveryResourceTrace{
		ScopeArtifact:  filepath.Join(workspace.Path, "ebpf-recovery-resource-scope.json"),
		EventsArtifact: filepath.Join(workspace.Path, "ebpf-recovery-resource-events.jsonl"),
		Scope:          scope,
		Events:         captured,
	}
	encodedScope, marshalErr := json.MarshalIndent(scope, "", "  ")
	if marshalErr != nil {
		return output, nil, marshalErr
	}
	if err := os.WriteFile(trace.ScopeArtifact, append(encodedScope, '\n'), 0o644); err != nil {
		return output, nil, fmt.Errorf("write recovery eBPF scope: %w", err)
	}
	if err := profiling.WriteRawEventsJSONL(trace.EventsArtifact, captured); err != nil {
		return output, nil, fmt.Errorf("write recovery eBPF events: %w", err)
	}
	if waitErr != nil {
		return output, trace, fmt.Errorf("wait for gated LangGraph recovery container: %w: %s", waitErr, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "0" {
		return output, trace, fmt.Errorf("gated LangGraph recovery container exited with status %s", strings.TrimSpace(string(output)))
	}
	return output, trace, nil
}

func prepareLangGraphRecoveryWorkspace(forkPlan LangGraphForkPlan) (langGraphRecoveryWorkspace, error) {
	runtimeRoot, err := filepath.Abs(forkPlan.RuntimeRoot)
	if err != nil {
		return langGraphRecoveryWorkspace{}, fmt.Errorf("resolve LangGraph runtime root: %w", err)
	}
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return langGraphRecoveryWorkspace{}, fmt.Errorf("create LangGraph runtime root: %w", err)
	}
	workspace, err := os.MkdirTemp(runtimeRoot, "syncfuzz-langgraph-fork-")
	if err != nil {
		return langGraphRecoveryWorkspace{}, fmt.Errorf("allocate LangGraph runtime workspace: %w", err)
	}
	if err := forkPlan.WorkspaceSnapshot.CloneTo(workspace); err != nil {
		return langGraphRecoveryWorkspace{}, fmt.Errorf("clone LangGraph recovery source snapshot: %w", err)
	}
	passiveMountTarget, err := workspaceChild(workspace, forkPlan.WorkspaceSnapshot.PassiveResourcePath())
	if err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	if err := os.MkdirAll(filepath.Dir(passiveMountTarget), 0o755); err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	if err := os.WriteFile(passiveMountTarget, nil, 0o600); err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	encodedSnapshot, err := json.Marshal(forkPlan.WorkspaceSnapshot)
	if err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	if err := os.WriteFile(filepath.Join(workspace, "langgraph-recovery-source-snapshot.json"), append(encodedSnapshot, '\n'), 0o644); err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	runtimeID := "langgraph-fork-" + filepath.Base(workspace)
	sandboxUID, sandboxGID := langGraphSandboxUserIDs()
	if err := chownLangGraphRecoveryWorkspace(workspace, sandboxUID, sandboxGID); err != nil {
		return langGraphRecoveryWorkspace{}, err
	}
	return langGraphRecoveryWorkspace{
		Path:                 workspace,
		RuntimeID:            runtimeID,
		PassiveObservation:   filepath.Join(workspace, "langgraph-recovery-observation.json"),
		ContinuationArtifact: filepath.Join(workspace, "langgraph-continuation-observation.json"),
		SandboxUID:           sandboxUID,
		SandboxGID:           sandboxGID,
	}, nil
}

func readLangGraphContinuationEvidence(path, runtimeID string, plan LangGraphForkPlan, coordinate LangGraphNativeCheckpointCoordinate, query RecoveryQuery, continuation *ContinuationQuery) (*ContinuationEvidence, error) {
	if continuation == nil {
		if query.ContinuationQueryID != "" {
			return nil, fmt.Errorf("LangGraph recovery query binds continuation %q without a frozen query", query.ContinuationQueryID)
		}
		return nil, nil
	}
	if err := continuation.Validate(); err != nil {
		return nil, err
	}
	if query.ContinuationQueryID != continuation.ContinuationQueryID {
		return nil, fmt.Errorf("LangGraph recovery query continuation does not match the frozen query")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read LangGraph continuation observation: %w", err)
	}
	var artifact langGraphContinuationArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("decode LangGraph continuation observation: %w", err)
	}
	if artifact.SchemaVersion != "syncfuzz.langgraph-continuation-observation.v1" || artifact.ObservationKind != "continuation-user-turn" || artifact.RuntimeInstanceID != runtimeID || !artifact.RuntimeRecreated || artifact.ThreadID != plan.SourceThreadID || artifact.RequestedCheckpointID != coordinate.SourceCheckpointID || artifact.RestoredCheckpointID != coordinate.SourceCheckpointID {
		return nil, fmt.Errorf("LangGraph continuation observation does not prove an exact fresh restore")
	}
	if artifact.RestoredCheckpointMessageCount != coordinate.MessageCount || !sameStrings(artifact.RestoredCheckpointNext, coordinate.Next) {
		return nil, fmt.Errorf("LangGraph continuation observation did not start from the planned native state shape")
	}
	if artifact.ContinuationQueryID != continuation.ContinuationQueryID || artifact.ContinuationQuerySHA256 != continuation.QuerySHA256 || artifact.ContinuationUserMessage != continuation.Query || !artifact.ContinuationInvoked {
		return nil, fmt.Errorf("LangGraph continuation observation does not prove the frozen follow-up query was invoked")
	}
	if artifact.ContinuationUserTurnCount != 1 || artifact.ContinuationAIToolCallCount < 0 || artifact.ContinuationToolResultCount < 0 || artifact.PostContinuationCheckpointCount <= 0 || len(artifact.PreEvidence) == 0 || len(artifact.PostEvidence) == 0 {
		return nil, fmt.Errorf("LangGraph continuation observation has incomplete completion evidence")
	}
	pre := append([]string(nil), artifact.PreEvidence...)
	pre = append(pre, "continuation observation artifact: "+path)
	post := append([]string(nil), artifact.PostEvidence...)
	post = append(post,
		"continuation invoked: true",
		"continuation user-turn count: 1",
		"continuation AI tool-call count: "+strconv.Itoa(artifact.ContinuationAIToolCallCount),
		"continuation tool-result count: "+strconv.Itoa(artifact.ContinuationToolResultCount),
		"post-continuation durable checkpoint count: "+strconv.Itoa(artifact.PostContinuationCheckpointCount),
	)
	return &ContinuationEvidence{ContinuationQueryID: continuation.ContinuationQueryID, PreEvidence: pre, PostEvidence: post}, nil
}

func sameContinuationQuery(left, right *ContinuationQuery) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.ContinuationQueryID == right.ContinuationQueryID && left.Query == right.Query && left.QuerySHA256 == right.QuerySHA256
}

func langGraphPassiveRecoveryState(plan LangGraphForkPlan, artifact langGraphRecoveryArtifact, probeMode LangGraphPassiveProbeMode, usePreContinuationObservation bool) (StatePresence, StateOrigin, EffectMultiplicity, *PassiveProbeMetrics, []string, error) {
	if plan.PassiveUnixSocketPath != "" {
		metadata := artifact.PassiveUnixSocket.AfterFork
		if usePreContinuationObservation {
			metadata = artifact.PassiveUnixSocket.BeforeFork
		}
		listenerIdentityMatches := matchesUnixSocketIdentity(metadata, plan.UnixSocketProbe)
		listenerMatches := matchesUnixSocketProbe(metadata, plan.UnixSocketProbe)
		osState := StatePresenceAbsent
		if socketPresent(metadata) && listenerIdentityMatches {
			osState = StatePresencePresent
		}
		origin := StateOriginNone
		if osState == StatePresencePresent && usePreContinuationObservation && matchesSnapshotSocket(metadata, plan.WorkspaceSnapshot) {
			origin = StateOriginResidual
		} else if osState == StatePresencePresent && artifact.PassiveUnixSocket.SameEndpointIdentity && matchesSnapshotSocket(artifact.PassiveUnixSocket.BeforeFork, plan.WorkspaceSnapshot) && matchesSnapshotSocket(artifact.PassiveUnixSocket.AfterFork, plan.WorkspaceSnapshot) {
			origin = StateOriginResidual
		} else if osState == StatePresencePresent {
			origin = StateOriginUnknown
		}
		multiplicity := EffectMultiplicityUnknown
		if origin == StateOriginResidual && probeMode == LangGraphPassiveProbeFull && listenerMatches {
			multiplicity = EffectMultiplicitySingle
		}
		metrics := &PassiveProbeMetrics{Mode: probeMode, DurationNS: metadata.ProbeDurationNS, ScannedProcesses: metadata.ScannedProcesses, ScannedFDs: metadata.ScannedFDs}
		phase := "post-continuation"
		if usePreContinuationObservation {
			phase = "pre-continuation"
		}
		evidence := []string{"passive " + phase + " probe scan counts: processes=" + strconv.Itoa(metadata.ScannedProcesses) + ",fds=" + strconv.Itoa(metadata.ScannedFDs), "eBPF-linked listener effects: " + plan.UnixSocketProbe.BindEffectID + "," + plan.UnixSocketProbe.ListenEffectID}
		return osState, origin, multiplicity, metrics, evidence, nil
	}
	if plan.WorkspaceFileProbe == nil {
		return StatePresenceUnknown, StateOriginUnknown, EffectMultiplicityUnknown, nil, nil, fmt.Errorf("LangGraph recovery plan has no passive workspace file probe")
	}
	after := artifact.PassiveWorkspaceFile.AfterFork
	if usePreContinuationObservation {
		after = artifact.PassiveWorkspaceFile.BeforeFork
	}
	osState := StatePresenceAbsent
	if matchesSnapshotWorkspaceFile(after, plan.WorkspaceSnapshot) {
		osState = StatePresencePresent
	}
	origin := StateOriginNone
	if osState == StatePresencePresent && usePreContinuationObservation && matchesSnapshotWorkspaceFile(after, plan.WorkspaceSnapshot) {
		origin = StateOriginResidual
	} else if osState == StatePresencePresent && artifact.PassiveWorkspaceFile.SameFileIdentity && matchesSnapshotWorkspaceFile(artifact.PassiveWorkspaceFile.BeforeFork, plan.WorkspaceSnapshot) {
		origin = StateOriginResidual
	} else if osState == StatePresencePresent {
		origin = StateOriginUnknown
	}
	multiplicity := EffectMultiplicityUnknown
	if origin == StateOriginResidual {
		multiplicity = EffectMultiplicitySingle
	}
	metrics := &PassiveProbeMetrics{Mode: probeMode, DurationNS: after.ProbeDurationNS}
	phase := "post-continuation"
	if usePreContinuationObservation {
		phase = "pre-continuation"
	}
	evidence := []string{"passive " + phase + " workspace file identity: " + plan.WorkspaceFileProbe.CanonicalPath, "eBPF-linked workspace file open effects: " + strings.Join(plan.WorkspaceFileProbe.OpenEffectIDs, ",")}
	return osState, origin, multiplicity, metrics, evidence, nil
}

// langGraphRecoveryDockerArgs is kept separate from execution so the V3
// recovery contract can be asserted without a Docker daemon or model provider.
func langGraphRecoveryDockerArgs(plan LangGraphForkPlan, workspace, runtimeID string, sandboxUID, sandboxGID int, checkpointID string, providerEnvironment map[string]string) []string {
	return langGraphRecoveryDockerArgsWithContinuation(plan, workspace, runtimeID, sandboxUID, sandboxGID, checkpointID, providerEnvironment, nil)
}

func langGraphRecoveryDockerArgsWithContinuation(plan LangGraphForkPlan, workspace, runtimeID string, sandboxUID, sandboxGID int, checkpointID string, providerEnvironment map[string]string, continuation *ContinuationQuery) []string {
	passivePath := plan.WorkspaceSnapshot.PassiveResourcePath()
	args := []string{"run", "--rm", "--name", "syncfuzz-" + runtimeID, "--pids-limit", "128", "--memory", "256m", "--cpus", "1", "--security-opt", "no-new-privileges", "--cap-drop", "ALL", "--network", "container:" + plan.SourceRuntime.ContainerName, "--pid", "container:" + plan.SourceRuntime.ContainerName, "--user", strconv.Itoa(sandboxUID) + ":" + strconv.Itoa(sandboxGID), "-v", workspace + ":/workspace", "-v", plan.WorkspaceSnapshot.SourcePassiveResourcePath() + ":/workspace/" + passivePath + ":ro", "-w", "/workspace", "-e", "LANGCHAIN_MODEL=" + plan.Model}
	for _, key := range []string{"OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY"} {
		if value := providerEnvironment[key]; value != "" {
			args = append(args, "-e", key+"="+value)
		}
	}
	command := []string{langGraphRecoveryContainerImage(plan), "python3", "/opt/syncfuzz-langgraph/run_target.py", "--workspace", "/workspace", "--prompt-file", "/workspace/target-prompt.txt", "--task-file", "/workspace/target-task.json", "--thread-id", plan.SourceThreadID, "--execution-policy", "host", "--checkpoint-backend", "disk", "--internal-phase", "resume", "--checkpoint-id", checkpointID, "--runtime-instance-id", runtimeID, "--recovery-observation-artifact", "/workspace/langgraph-recovery-observation.json"}
	if continuation == nil {
		command = append(command, "--passive-fork-observe")
	} else {
		command = append(command,
			"--continuation-user-message", continuation.Query,
			"--continuation-query-id", continuation.ContinuationQueryID,
			"--continuation-observation-artifact", "/workspace/langgraph-continuation-observation.json",
		)
	}
	if plan.PassiveUnixSocketPath != "" {
		command = append(command, "--passive-unix-socket-path", plan.PassiveUnixSocketPath, "--passive-unix-socket-probe-mode", string(plan.PassiveProbeMode.Effective()), "--passive-unix-socket-expected-id", plan.UnixSocketProbe.SocketID, "--passive-unix-socket-expected-holder-pid", strconv.FormatUint(uint64(plan.UnixSocketProbe.HolderPID), 10), "--passive-unix-socket-expected-holder-fd", strconv.Itoa(plan.UnixSocketProbe.HolderFD))
	} else {
		command = append(command, "--passive-workspace-file-path", plan.PassiveWorkspaceFilePath, "--passive-workspace-file-expected-device", strconv.FormatUint(plan.WorkspaceSnapshot.PassiveWorkspaceFileDevice, 10), "--passive-workspace-file-expected-inode", strconv.FormatUint(plan.WorkspaceSnapshot.PassiveWorkspaceFileInode, 10))
	}
	return append(args, command...)
}

func langGraphRecoveryContainerImage(plan LangGraphForkPlan) string {
	if plan.RuntimeContract.SchemaVersion != "" {
		return plan.RuntimeContract.ImageID
	}
	return plan.ContainerImage
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

func matchesSnapshotWorkspaceFile(observation langGraphPassiveWorkspaceFileMetadata, snapshot LangGraphWorkspaceSnapshot) bool {
	return observation.IsRegularFile && observation.Device == snapshot.PassiveWorkspaceFileDevice && observation.Inode == snapshot.PassiveWorkspaceFileInode && observation.Mode == snapshot.PassiveWorkspaceFileMode
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
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Running}} {{.Config.Image}} {{.Image}}", runtime.ContainerName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect retained LangGraph source runtime %q: %w: %s", runtime.ContainerName, err, strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 4 || fields[0] != runtime.ContainerID || fields[1] != "true" || fields[2] != runtime.ContainerImage || (runtime.ContainerImageID != "" && fields[3] != runtime.ContainerImageID) {
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
