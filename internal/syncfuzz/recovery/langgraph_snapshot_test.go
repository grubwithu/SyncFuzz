package recovery

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLangGraphWorkspaceSnapshotClonesDurableStoreAndRetainsSocketIdentity(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "langgraph-checkpoints"), 0o755); err != nil {
		t.Fatalf("create checkpoint store: %v", err)
	}
	for path, content := range map[string]string{
		"target-prompt.txt":                 "profiled prompt\n",
		"target-task.json":                  "{}\n",
		"langgraph-checkpoints/storage.pkl": "storage\n",
		"langgraph-checkpoints/writes.pkl":  "writes\n",
		"langgraph-checkpoints/blobs.pkl":   "blobs\n",
		"unix-listener-result.txt":          "verified\n",
	} {
		if err := os.WriteFile(filepath.Join(source, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write source artifact %s: %v", path, err)
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(source, "agent.sock"), Net: "unix"})
	if err != nil {
		t.Skipf("Unix sockets are unavailable in this test sandbox: %v", err)
	}
	defer listener.Close()

	snapshot, err := CaptureLangGraphWorkspaceSnapshot(source, "agent.sock")
	if err != nil {
		t.Fatalf("CaptureLangGraphWorkspaceSnapshot returned error: %v", err)
	}
	if err := snapshot.VerifySource(); err != nil {
		t.Fatalf("VerifySource returned error: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "clone")
	if err := snapshot.CloneTo(destination); err != nil {
		t.Fatalf("CloneTo returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "langgraph-checkpoints", "storage.pkl"))
	if err != nil || string(content) != "storage\n" {
		t.Fatalf("clone did not preserve checkpoint store: content=%q err=%v", content, err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "agent.sock")); !os.IsNotExist(err) {
		t.Fatalf("clone must not reconstruct the retained socket: %v", err)
	}

	if err := os.WriteFile(filepath.Join(source, "langgraph-checkpoints", "storage.pkl"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("mutate source checkpoint store: %v", err)
	}
	if err := snapshot.VerifySource(); err == nil {
		t.Fatal("expected source snapshot mutation to be rejected")
	}
}

func TestLangGraphWorkspaceFileSnapshotClonesWithoutCopyingRetainedFile(t *testing.T) {
	source := t.TempDir()
	for _, artifact := range []string{"target-prompt.txt", "target-task.json", "langgraph-checkpoints/storage.pkl"} {
		path := filepath.Join(source, artifact)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create source artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(artifact+"\n"), 0o644); err != nil {
			t.Fatalf("write source artifact: %v", err)
		}
	}
	retainedPath := filepath.Join(source, "agent-result.txt")
	if err := os.WriteFile(retainedPath, []byte("source identity only\n"), 0o640); err != nil {
		t.Fatalf("write retained workspace file: %v", err)
	}

	snapshot, err := CaptureLangGraphWorkspaceFileSnapshot(source, "agent-result.txt")
	if err != nil {
		t.Fatalf("CaptureLangGraphWorkspaceFileSnapshot returned error: %v", err)
	}
	if snapshot.PassiveWorkspaceFilePath != "agent-result.txt" || snapshot.PassiveWorkspaceFileInode == 0 || snapshot.PassiveUnixSocketPath != "" {
		t.Fatalf("unexpected workspace file snapshot: %#v", snapshot)
	}
	destination := filepath.Join(t.TempDir(), "clone")
	if err := snapshot.CloneTo(destination); err != nil {
		t.Fatalf("CloneTo returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "agent-result.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retained workspace file must be excluded from clone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "langgraph-checkpoints", "storage.pkl")); err != nil {
		t.Fatalf("durable store was not copied: %v", err)
	}
	if snapshot.SourcePassiveResourcePath() != retainedPath {
		t.Fatalf("unexpected retained source path %q", snapshot.SourcePassiveResourcePath())
	}
}

func TestLangGraphWorkspaceTopologyReportsUnmodelledSocketForFileContract(t *testing.T) {
	source := t.TempDir()
	for _, artifact := range []string{"target-task.json", "langgraph-checkpoints/storage.pkl", "agent-result.txt"} {
		path := filepath.Join(source, artifact)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create source artifact directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(artifact+"\n"), 0o644); err != nil {
			t.Fatalf("write source artifact: %v", err)
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(source, "agent.sock"), Net: "unix"})
	if err != nil {
		t.Skipf("Unix sockets are unavailable in this test sandbox: %v", err)
	}
	defer listener.Close()

	contract, err := NewLangGraphRetainedResourceContract(LangGraphRetainedWorkspaceFile, "agent-result.txt")
	if err != nil {
		t.Fatalf("NewLangGraphRetainedResourceContract: %v", err)
	}
	_, topology, err := CaptureLangGraphWorkspaceSnapshotForContract(source, contract)
	var topologyError *LangGraphWorkspaceTopologyError
	if !errors.As(err, &topologyError) {
		t.Fatalf("expected a structured topology error, got topology=%#v err=%v", topology, err)
	}
	if len(topology.UnexpectedNodes) != 1 || topology.UnexpectedNodes[0].WorkspaceRelativePath != "agent.sock" || topology.UnexpectedNodes[0].Kind != "unix-socket" {
		t.Fatalf("unexpected workspace topology inventory: %#v", topology)
	}
	if !reflect.DeepEqual(topologyError.Topology, topology) {
		t.Fatalf("topology error must preserve the captured inventory: %#v", topologyError)
	}
}

func TestParseLangGraphRuntimeContractRequiresRecoveryCapabilities(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	contract, err := ParseLangGraphRuntimeContract([]byte(`{
  "schema_version":"syncfuzz.langgraph-runtime-contract.v1",
  "target_id":"langgraph-shell-react",
  "runner_protocol":"syncfuzz.langgraph-runner.v1",
  "capabilities":[
    "passive-workspace-file-observer-v1",
    "exact-checkpoint-restore-v1",
    "durable-disk-checkpoints-v1",
    "passive-unix-socket-observer-v1"
  ]
}`), imageID)
	if err != nil {
		t.Fatalf("ParseLangGraphRuntimeContract: %v", err)
	}
	if contract.ImageID != imageID || len(contract.Capabilities) != 4 || contract.Capabilities[0] != "durable-disk-checkpoints-v1" {
		t.Fatalf("runtime contract was not canonicalized: %#v", contract)
	}
	if _, err := ParseLangGraphRuntimeContract([]byte(`{
  "schema_version":"syncfuzz.langgraph-runtime-contract.v1",
  "target_id":"langgraph-shell-react",
  "runner_protocol":"syncfuzz.langgraph-runner.v1",
  "capabilities":["durable-disk-checkpoints-v1"]
}`), imageID); err == nil {
		t.Fatal("expected incomplete runtime capability set to be rejected")
	}
}

func TestLangGraphWorkspaceSnapshotRejectsSymlinks(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "langgraph-checkpoints"), 0o755); err != nil {
		t.Fatalf("create checkpoint store: %v", err)
	}
	if err := os.Symlink("/tmp", filepath.Join(source, "escape")); err != nil {
		t.Fatalf("create source symlink: %v", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(source, "agent.sock"), Net: "unix"})
	if err != nil {
		t.Skipf("Unix sockets are unavailable in this test sandbox: %v", err)
	}
	defer listener.Close()
	if _, err := CaptureLangGraphWorkspaceSnapshot(source, "agent.sock"); err == nil {
		t.Fatal("expected symlinked source workspace to be rejected")
	}
}

func TestLangGraphWorkspaceSnapshotRejectsEscapingPassiveSocketPath(t *testing.T) {
	snapshot := LangGraphWorkspaceSnapshot{
		SourceWorkspace:             t.TempDir(),
		WorkspaceSHA256:             strings.Repeat("a", 64),
		CheckpointStoreRelativePath: langGraphCheckpointStoreRelativePath,
		CheckpointStoreSHA256:       strings.Repeat("b", 64),
		PassiveUnixSocketPath:       "../agent.sock",
		PassiveUnixSocketInode:      1,
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("expected snapshot with an escaping passive socket path to be rejected")
	}
}

func TestLangGraphRecoveryDockerArgsUseExactCheckpointIDAndSourceNamespaces(t *testing.T) {
	plan := LangGraphForkPlan{
		Model:                 "openai:test",
		ContainerImage:        "syncfuzz-langgraph:test",
		PassiveUnixSocketPath: "sockets/agent.sock",
		SourceThreadID:        "profile-thread",
		SourceRuntime: LangGraphSourceRuntime{
			ContainerName: "syncfuzz-profile-source",
		},
		WorkspaceSnapshot: LangGraphWorkspaceSnapshot{
			SourceWorkspace:       "/profile/workspace",
			PassiveUnixSocketPath: "sockets/agent.sock",
		},
		UnixSocketProbe: LangGraphUnixSocketProbe{SocketID: "socket:123", HolderPID: 7, HolderFD: 3},
	}
	args := langGraphRecoveryDockerArgs(plan, "/recovery/workspace", "runtime-1", 10001, 10001, "native-checkpoint-1", map[string]string{"OPENAI_BASE_URL": "https://provider.example/v1"})
	if !hasArgumentPair(args, "--checkpoint-id", "native-checkpoint-1") {
		t.Fatalf("recovery invocation must restore the exact source checkpoint ID: %#v", args)
	}
	if hasArgument(args, "--checkpoint-coordinate-file") {
		t.Fatalf("recovery invocation must not fall back to runtime-local coordinate matching: %#v", args)
	}
	if !hasArgumentPair(args, "--network", "container:syncfuzz-profile-source") || !hasArgumentPair(args, "--pid", "container:syncfuzz-profile-source") {
		t.Fatalf("recovery invocation must join the retained source namespaces: %#v", args)
	}
	if !hasArgumentPair(args, "-v", "/profile/workspace/sockets/agent.sock:/workspace/sockets/agent.sock:ro") {
		t.Fatalf("recovery invocation must bind-mount the retained socket read-only: %#v", args)
	}
	if !hasArgumentPair(args, "-v", "/recovery/workspace:/workspace") {
		t.Fatalf("recovery invocation must use a distinct cloned workspace: %#v", args)
	}
	if !hasArgumentPair(args, "--passive-unix-socket-probe-mode", "full") || !hasArgumentPair(args, "--passive-unix-socket-expected-id", "socket:123") {
		t.Fatalf("full recovery invocation must carry the recorded passive probe identity: %#v", args)
	}
}

func TestLangGraphRecoveryDockerArgsPinsVerifiedRuntimeImage(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	plan := LangGraphForkPlan{
		ContainerImage: "syncfuzz-langgraph:mutable",
		RuntimeContract: LangGraphRuntimeContract{
			SchemaVersion: LangGraphRuntimeContractSchema,
			ImageID:       imageID,
		},
	}
	args := langGraphRecoveryDockerArgs(plan, "/recovery/workspace", "runtime-1", 10001, 10001, "native-checkpoint-1", nil)
	if !hasArgument(args, imageID) || hasArgument(args, plan.ContainerImage) {
		t.Fatalf("recovery invocation must use the verified immutable image ID: %#v", args)
	}
}

func TestLangGraphRecoveryDockerArgsPassesPrunedProbeIdentity(t *testing.T) {
	plan := LangGraphForkPlan{
		Model:                 "openai:test",
		ContainerImage:        "syncfuzz-langgraph:test",
		PassiveUnixSocketPath: "agent.sock",
		PassiveProbeMode:      LangGraphPassiveProbePruned,
		SourceThreadID:        "profile-thread",
		SourceRuntime:         LangGraphSourceRuntime{ContainerName: "syncfuzz-profile-source"},
		WorkspaceSnapshot:     LangGraphWorkspaceSnapshot{SourceWorkspace: "/profile/workspace", PassiveUnixSocketPath: "agent.sock"},
		UnixSocketProbe:       LangGraphUnixSocketProbe{SocketID: "socket:123", HolderPID: 7, HolderFD: 3},
	}
	args := langGraphRecoveryDockerArgs(plan, "/recovery/workspace", "runtime-1", 10001, 10001, "native-checkpoint-1", nil)
	if !hasArgumentPair(args, "--passive-unix-socket-probe-mode", "pruned") || !hasArgumentPair(args, "--passive-unix-socket-expected-id", "socket:123") || !hasArgumentPair(args, "--passive-unix-socket-expected-holder-pid", "7") || !hasArgumentPair(args, "--passive-unix-socket-expected-holder-fd", "3") {
		t.Fatalf("pruned recovery invocation must constrain the observer to the recorded listener identity: %#v", args)
	}
}

func TestLangGraphRecoveryDockerArgsBindMountsRetainedWorkspaceFile(t *testing.T) {
	plan := LangGraphForkPlan{
		Model:                    "openai:test",
		ContainerImage:           "syncfuzz-langgraph:test",
		PassiveWorkspaceFilePath: "agent-result.txt",
		SourceThreadID:           "profile-thread",
		SourceRuntime:            LangGraphSourceRuntime{ContainerName: "syncfuzz-profile-source"},
		WorkspaceSnapshot: LangGraphWorkspaceSnapshot{
			SourceWorkspace:             "/profile/workspace",
			PassiveWorkspaceFilePath:    "agent-result.txt",
			CheckpointStoreRelativePath: "langgraph-checkpoints",
			WorkspaceSHA256:             strings.Repeat("a", 64),
			CheckpointStoreSHA256:       strings.Repeat("b", 64),
			PassiveWorkspaceFileDevice:  42,
			PassiveWorkspaceFileInode:   99,
		},
		WorkspaceFileProbe: &LangGraphWorkspaceFileProbe{
			SchemaVersion: LangGraphWorkspaceFileProbeSchema,
			ResourceID:    "workspace:agent-result.txt",
			CanonicalPath: "/workspace/agent-result.txt",
			OpenEffectIDs: []string{"open-effect"},
		},
	}
	args := langGraphRecoveryDockerArgs(plan, "/recovery/workspace", "runtime-1", 10001, 10001, "native-checkpoint-1", nil)
	if !hasArgumentPair(args, "-v", "/profile/workspace/agent-result.txt:/workspace/agent-result.txt:ro") {
		t.Fatalf("recovery invocation must bind-mount the retained workspace file read-only: %#v", args)
	}
	if !hasArgumentPair(args, "--passive-workspace-file-path", "agent-result.txt") || !hasArgumentPair(args, "--passive-workspace-file-expected-device", "42") || !hasArgumentPair(args, "--passive-workspace-file-expected-inode", "99") {
		t.Fatalf("recovery invocation must carry exact workspace file identity: %#v", args)
	}
	if hasArgument(args, "--passive-unix-socket-path") {
		t.Fatalf("workspace file recovery must not configure a Unix socket observer: %#v", args)
	}
}

func TestLangGraphPassiveRecoveryStateRecognizesRetainedWorkspaceFile(t *testing.T) {
	plan := LangGraphForkPlan{
		PassiveWorkspaceFilePath: "agent-result.txt",
		WorkspaceSnapshot: LangGraphWorkspaceSnapshot{
			PassiveWorkspaceFilePath:   "agent-result.txt",
			PassiveWorkspaceFileDevice: 42,
			PassiveWorkspaceFileInode:  99,
			PassiveWorkspaceFileMode:   0o640,
		},
		WorkspaceFileProbe: &LangGraphWorkspaceFileProbe{
			SchemaVersion: LangGraphWorkspaceFileProbeSchema,
			ResourceID:    "workspace:agent-result.txt",
			CanonicalPath: "/workspace/agent-result.txt",
			OpenEffectIDs: []string{"open-effect"},
		},
	}
	metadata := langGraphPassiveWorkspaceFileMetadata{
		IsRegularFile:   true,
		Device:          42,
		Inode:           99,
		Mode:            0o640,
		ProbeDurationNS: 17,
	}
	artifact := langGraphRecoveryArtifact{}
	artifact.PassiveWorkspaceFile.BeforeFork = metadata
	artifact.PassiveWorkspaceFile.AfterFork = metadata
	artifact.PassiveWorkspaceFile.SameFileIdentity = true

	osState, origin, multiplicity, metrics, evidence, err := langGraphPassiveRecoveryState(plan, artifact, LangGraphPassiveProbeFull)
	if err != nil {
		t.Fatalf("langGraphPassiveRecoveryState returned error: %v", err)
	}
	if osState != StatePresencePresent || origin != StateOriginResidual || multiplicity != EffectMultiplicitySingle {
		t.Fatalf("workspace file recovery state=%s origin=%s multiplicity=%s, want present/residual/single", osState, origin, multiplicity)
	}
	if metrics == nil || metrics.Mode != LangGraphPassiveProbeFull || metrics.DurationNS != 17 {
		t.Fatalf("workspace file recovery probe metrics=%#v", metrics)
	}
	if len(evidence) != 2 || !strings.Contains(evidence[1], "open-effect") {
		t.Fatalf("workspace file recovery evidence=%#v", evidence)
	}
}

func hasArgument(args []string, expected string) bool {
	for _, value := range args {
		if value == expected {
			return true
		}
	}
	return false
}

func hasArgumentPair(args []string, flag string, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func TestLangGraphForkPlanRequiresSourceSnapshot(t *testing.T) {
	seed := testSeed()
	recordedPlan := RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       seed.RecordedPlanID,
		AdapterID:            LangGraphForkAdapterID,
		TargetID:             seed.TargetID,
		ExecutionArtifact:    seed.RecordedPlanArtifact,
		PassiveObservationID: "unix-socket-metadata:agent.sock",
	}
	coordinate := LangGraphNativeCheckpointCoordinate{
		SchemaVersion:      LangGraphNativeCoordinateSchema,
		SourceCheckpointID: "native-checkpoint",
		HistoryIndex:       0,
		MessageCount:       1,
		Next:               []string{},
	}
	plan := LangGraphForkPlan{
		SchemaVersion:         LangGraphForkPlanSchema,
		RecordedPlanID:        seed.RecordedPlanID,
		AdapterID:             LangGraphForkAdapterID,
		TargetID:              seed.TargetID,
		CandidateID:           "candidate-1",
		Task:                  "do the recorded task",
		Model:                 "openai:test",
		ContainerImage:        "syncfuzz-langgraph:test",
		RuntimeRoot:           "/tmp/langgraph-forks",
		PassiveUnixSocketPath: "agent.sock",
		PassiveObservationID:  recordedPlan.PassiveObservationID,
		SourceThreadID:        "source-thread",
		SourceRuntime: LangGraphSourceRuntime{
			SchemaVersion:  "syncfuzz.target-runtime-lease.v1",
			Environment:    "container",
			ContainerName:  "syncfuzz-profile-source",
			ContainerID:    "container-id",
			ContainerImage: "syncfuzz-langgraph:test",
		},
		WorkspaceSnapshot: LangGraphWorkspaceSnapshot{
			SourceWorkspace:             "/tmp/profile-workspace",
			WorkspaceSHA256:             strings.Repeat("a", 64),
			CheckpointStoreRelativePath: "langgraph-checkpoints",
			CheckpointStoreSHA256:       strings.Repeat("b", 64),
			PassiveUnixSocketPath:       "agent.sock",
			PassiveUnixSocketInode:      1,
		},
		UnixSocketProbe: LangGraphUnixSocketProbe{
			SchemaVersion:  LangGraphUnixSocketProbeSchema,
			SocketID:       "socket:123",
			HolderPID:      7,
			HolderFD:       3,
			BindEffectID:   "bind-effect",
			ListenEffectID: "listen-effect",
		},
		CheckpointCoordinates: map[string]LangGraphNativeCheckpointCoordinate{
			seed.BeforeCheckpointID: coordinate,
			seed.AfterCheckpointID:  coordinate,
		},
		AgentStateByCheckpoint: map[string]StatePresence{
			seed.BeforeCheckpointID: StatePresenceAbsent,
			seed.AfterCheckpointID:  StatePresencePresent,
		},
	}
	plan.CheckpointCoordinates[seed.AfterCheckpointID] = LangGraphNativeCheckpointCoordinate{
		SchemaVersion:      LangGraphNativeCoordinateSchema,
		SourceCheckpointID: "native-checkpoint-after",
		HistoryIndex:       1,
		MessageCount:       2,
		Next:               []string{"model"},
	}
	if err := plan.ValidateFor(recordedPlan); err != nil {
		t.Fatalf("expected complete snapshot plan to validate: %v", err)
	}
	plan.WorkspaceSnapshot = LangGraphWorkspaceSnapshot{}
	if err := plan.ValidateFor(recordedPlan); err == nil {
		t.Fatal("expected LangGraph fork plan without a source snapshot to be rejected")
	}
}

func TestMatchesUnixSocketProbeRequiresOneExactHolder(t *testing.T) {
	probe := LangGraphUnixSocketProbe{
		SchemaVersion:  LangGraphUnixSocketProbeSchema,
		SocketID:       "socket:123",
		HolderPID:      7,
		HolderFD:       3,
		BindEffectID:   "bind-effect",
		ListenEffectID: "listen-effect",
	}
	observation := langGraphPassiveSocketMetadata{
		KernelSocketID: "socket:123",
		ListenerActive: true,
		ListenerCount:  1,
		ListenerHolders: []langGraphPassiveSocketHolder{{
			PID: 7,
			FDs: []int{3},
		}},
	}
	if !matchesUnixSocketProbe(observation, probe) {
		t.Fatalf("expected exact live listener holder to match: %#v", observation)
	}
	observation.ListenerHolders[0].FDs = []int{4}
	if matchesUnixSocketProbe(observation, probe) {
		t.Fatalf("unexpected match for wrong holder FD: %#v", observation)
	}
	observation.ListenerHolders[0].FDs = []int{3}
	observation.ListenerCount = 2
	if matchesUnixSocketProbe(observation, probe) {
		t.Fatalf("unexpected match for duplicate listeners: %#v", observation)
	}
	observation.ListenerCount = 1
	observation.ListenerHolders = append(observation.ListenerHolders, langGraphPassiveSocketHolder{PID: 11, FDs: []int{5}})
	if matchesUnixSocketProbe(observation, probe) {
		t.Fatalf("unexpected full match with multiple listener holders: %#v", observation)
	}
	if !matchesUnixSocketIdentity(observation, probe) {
		t.Fatalf("pruned identity evidence must still recognize the recorded holder: %#v", observation)
	}
}

func TestLangGraphPassiveProbeModeDefaultsToFull(t *testing.T) {
	if got := (LangGraphPassiveProbeMode("")).Effective(); got != LangGraphPassiveProbeFull {
		t.Fatalf("empty passive probe mode = %q, want full", got)
	}
	if !LangGraphPassiveProbeFull.Valid() || !LangGraphPassiveProbePruned.Valid() || LangGraphPassiveProbeMode("unsafe").Valid() {
		t.Fatal("unexpected passive probe mode validation")
	}
}
