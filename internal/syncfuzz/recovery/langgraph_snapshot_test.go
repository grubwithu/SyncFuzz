package recovery

import (
	"net"
	"os"
	"path/filepath"
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
}
