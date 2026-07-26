package recovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
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

func TestParseLangGraphRuntimeContractRecognizesContinuationCapability(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("b", 64)
	contract, err := ParseLangGraphRuntimeContract([]byte(`{
  "schema_version":"syncfuzz.langgraph-runtime-contract.v1",
  "target_id":"langgraph-shell-react",
  "runner_protocol":"syncfuzz.langgraph-runner.v1",
  "capabilities":[
    "continuation-user-turn-v1",
    "durable-disk-checkpoints-v1",
    "exact-checkpoint-restore-v1",
    "passive-unix-socket-observer-v1",
    "passive-workspace-file-observer-v1"
  ]
}`), imageID)
	if err != nil {
		t.Fatalf("ParseLangGraphRuntimeContract: %v", err)
	}
	if !contract.SupportsContinuation() {
		t.Fatalf("continuation runtime capability was not preserved: %#v", contract)
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

func TestLangGraphContinuationDockerArgsUseOneRestoredRuntimeForPassiveAndFollowUp(t *testing.T) {
	plan := LangGraphForkPlan{
		Model:                 "openai:test",
		ContainerImage:        "syncfuzz-langgraph:test",
		PassiveUnixSocketPath: "agent.sock",
		SourceThreadID:        "profile-thread",
		SourceRuntime:         LangGraphSourceRuntime{ContainerName: "syncfuzz-profile-source"},
		WorkspaceSnapshot:     LangGraphWorkspaceSnapshot{SourceWorkspace: "/profile/workspace", PassiveUnixSocketPath: "agent.sock"},
		UnixSocketProbe:       LangGraphUnixSocketProbe{SocketID: "socket:123", HolderPID: 7, HolderFD: 3},
	}
	continuation, err := NewContinuationQuery("Inspect the restored workspace and report its files.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	args := langGraphRecoveryDockerArgsWithContinuation(plan, "/recovery/workspace", "runtime-1", 10001, 10001, "native-checkpoint-1", nil, continuation)
	if hasArgument(args, "--passive-fork-observe") {
		t.Fatalf("continuation recovery must not use the passive-only runner path: %#v", args)
	}
	if !hasArgumentPair(args, "--continuation-user-message", continuation.Query) || !hasArgumentPair(args, "--continuation-query-id", continuation.ContinuationQueryID) || !hasArgumentPair(args, "--continuation-observation-artifact", "/workspace/langgraph-continuation-observation.json") {
		t.Fatalf("continuation recovery must inject the frozen query and request its artifact: %#v", args)
	}
	if !hasArgumentPair(args, "--passive-unix-socket-path", "agent.sock") || !hasArgumentPair(args, "--runtime-instance-id", "runtime-1") {
		t.Fatalf("continuation must keep passive probing and the same recovered runtime identity: %#v", args)
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

	osState, origin, multiplicity, metrics, evidence, err := langGraphPassiveRecoveryState(plan, artifact, LangGraphPassiveProbeFull, false)
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

func TestLangGraphContinuationProjectsStaticRelationFromPreContinuationMetadata(t *testing.T) {
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
	artifact := langGraphRecoveryArtifact{}
	artifact.PassiveWorkspaceFile.BeforeFork = langGraphPassiveWorkspaceFileMetadata{
		IsRegularFile: true,
		Device:        42,
		Inode:         99,
		Mode:          0o640,
	}
	// The active follow-up is permitted to change the post state. The static
	// recovery relation must remain anchored to the observation immediately
	// before that one user turn.
	artifact.PassiveWorkspaceFile.AfterFork = langGraphPassiveWorkspaceFileMetadata{}

	osState, origin, multiplicity, _, evidence, err := langGraphPassiveRecoveryState(plan, artifact, LangGraphPassiveProbeFull, true)
	if err != nil {
		t.Fatalf("langGraphPassiveRecoveryState: %v", err)
	}
	if osState != StatePresencePresent || origin != StateOriginResidual || multiplicity != EffectMultiplicitySingle {
		t.Fatalf("pre-continuation state=%s origin=%s multiplicity=%s, want present/residual/single", osState, origin, multiplicity)
	}
	if len(evidence) == 0 || !strings.Contains(evidence[0], "pre-continuation") {
		t.Fatalf("static relation evidence must identify the pre-continuation probe: %#v", evidence)
	}
}

func TestReadLangGraphContinuationEvidenceBindsExactQueryAndRestore(t *testing.T) {
	continuation, err := NewContinuationQuery("Inspect the restored workspace and report its files.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	coordinate := LangGraphNativeCheckpointCoordinate{
		SourceCheckpointID: "native-checkpoint-1",
		MessageCount:       3,
		Next:               []string{"tools"},
	}
	artifact := langGraphContinuationArtifact{
		SchemaVersion:                   "syncfuzz.langgraph-continuation-observation.v1",
		ObservationKind:                 "continuation-user-turn",
		RuntimeInstanceID:               "runtime-1",
		RuntimeRecreated:                true,
		ThreadID:                        "profile-thread",
		RequestedCheckpointID:           coordinate.SourceCheckpointID,
		RestoredCheckpointID:            coordinate.SourceCheckpointID,
		RestoredCheckpointMessageCount:  coordinate.MessageCount,
		RestoredCheckpointNext:          coordinate.Next,
		ContinuationQueryID:             continuation.ContinuationQueryID,
		ContinuationQuerySHA256:         continuation.QuerySHA256,
		ContinuationUserMessage:         continuation.Query,
		ContinuationInvoked:             true,
		ContinuationUserTurnCount:       1,
		ContinuationAIToolCallCount:     1,
		ContinuationToolResultCount:     1,
		PostContinuationCheckpointCount: 5,
		PreEvidence:                     []string{"exact native checkpoint restored"},
		PostEvidence:                    []string{"one continuation invoke completed"},
	}
	path := filepath.Join(t.TempDir(), "continuation.json")
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal continuation artifact: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write continuation artifact: %v", err)
	}
	evidence, err := readLangGraphContinuationEvidence(path, "runtime-1", LangGraphForkPlan{SourceThreadID: "profile-thread"}, coordinate, RecoveryQuery{ContinuationQueryID: continuation.ContinuationQueryID}, continuation)
	if err != nil {
		t.Fatalf("readLangGraphContinuationEvidence: %v", err)
	}
	if evidence == nil || evidence.ContinuationQueryID != continuation.ContinuationQueryID || len(evidence.PreEvidence) < 2 || len(evidence.PostEvidence) < 5 {
		t.Fatalf("unexpected continuation evidence: %#v", evidence)
	}

	artifact.ContinuationUserMessage = "substituted message"
	data, err = json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal substituted continuation artifact: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write substituted continuation artifact: %v", err)
	}
	if _, err := readLangGraphContinuationEvidence(path, "runtime-1", LangGraphForkPlan{SourceThreadID: "profile-thread"}, coordinate, RecoveryQuery{ContinuationQueryID: continuation.ContinuationQueryID}, continuation); err == nil {
		t.Fatal("expected substituted continuation text to be rejected")
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

func TestLangGraphWorkspaceSnapshotExcludesAuthorizedEphemeralObserverLog(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "langgraph-checkpoints"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"agent-result.txt":                  "result\n",
		"environment-use-events.jsonl":      "initial\n",
		"langgraph-checkpoints/storage.pkl": "storage\n",
		"langgraph-checkpoints/writes.pkl":  "writes\n",
		"langgraph-checkpoints/blobs.pkl":   "blobs\n",
	} {
		if err := os.WriteFile(filepath.Join(source, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	contract, err := NewLangGraphRetainedResourceContract(LangGraphRetainedWorkspaceFile, "agent-result.txt")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := CaptureLangGraphWorkspaceSnapshotForContractWithEphemeralArtifacts(source, contract, []string{"environment-use-events.jsonl"})
	if err != nil {
		t.Fatalf("capture snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "environment-use-events.jsonl"), []byte("initial\nrecovery-use\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VerifySource(); err != nil {
		t.Fatalf("ephemeral observer update invalidated snapshot: %v", err)
	}
	destination := t.TempDir()
	if err := snapshot.CloneTo(destination); err != nil {
		t.Fatalf("clone snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "environment-use-events.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ephemeral observer log leaked into recovery clone: %v", err)
	}
}

func TestValidateLangGraphRecoveryEnvironmentUseRequiresConnectAndActiveRoleAccept(t *testing.T) {
	source := t.TempDir()
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect,
		EndpointPath: "agent.sock", InitialRole: "baseline", ActiveRole: "baseline", HolderLifetime: environment.HolderLifetimeChild,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram returned error: %v", err)
	}
	program, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatalf("MutateUnixSocket returned error: %v", err)
	}
	if err := environment.WriteEnvironmentProgram(filepath.Join(source, "environment-program.json"), program); err != nil {
		t.Fatalf("write program: %v", err)
	}
	materialization := environment.TargetUnixSocketMaterialization{
		SchemaVersion: environment.TargetUnixSocketMaterializationSchemaVersion, ProgramID: program.ProgramID,
		SourceNativeCheckpointID: "checkpoint-1", SourceCheckpointMonotonicNS: 100,
		EffectWindowMonotonicNS: environment.TargetEffectWindow{Start: 100, End: 140},
		Family:                  environment.EnvironmentResourceFamilyUnixSocket, EndpointPath: "agent.sock", LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect, UseEventArtifactPath: "environment-use-events.jsonl",
		ResolutionSteps: []environment.ResolutionStep{
			{Kind: environment.ResolutionStepLogicalName, From: "agent-service", To: "agent.sock"},
			{Kind: environment.ResolutionStepPathname, From: "agent.sock", To: "unix-endpoint:agent.sock"},
		},
		Listeners: []environment.TargetUnixSocketListener{
			{PID: 11, Role: "baseline", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 101, FD: 3, SocketID: "socket:11", SocketDevice: 1, SocketInode: 11, ReadyMonotonicNS: 120},
			{PID: 12, Role: "replacement", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 102, FD: 3, SocketID: "socket:12", SocketDevice: 1, SocketInode: 12, ReadyMonotonicNS: 130},
		},
		ActiveListener: environment.TargetUnixSocketListener{PID: 12, Role: "replacement", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 102, FD: 3, SocketID: "socket:12", SocketDevice: 1, SocketInode: 12, ReadyMonotonicNS: 130},
	}
	data, err := json.Marshal(materialization)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "environment-materialization.json"), data, 0o644); err != nil {
		t.Fatalf("write materialization: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "environment-use-events.jsonl"), []byte(`{"schema_version":"syncfuzz.environment-listener-use.v1","monotonic_ns":210,"role":"replacement","endpoint":"/workspace/agent.sock","peer_pid":77,"request_bytes":21,"request_sha256":"0000000000000000000000000000000000000000000000000000000000000000","response_sent":true,"response_acknowledged":true}`+"\n"), 0o644); err != nil {
		t.Fatalf("write use event: %v", err)
	}
	trace := &langGraphRecoveryResourceTrace{Events: []profiling.RawEvent{{EventID: "connect", MonotonicNS: 200, Kind: profiling.RawEventConnect, Result: 0, Resource: profiling.ResourceRef{Path: "/workspace/agent.sock"}}, {EventID: "after", MonotonicNS: 220, Kind: profiling.RawEventClose}}}
	evidence, typed, err := validateLangGraphRecoveryEnvironmentUse(LangGraphForkPlan{WorkspaceSnapshot: LangGraphWorkspaceSnapshot{SourceWorkspace: source}}, trace)
	if err != nil || len(evidence) != 2 || typed == nil || !typed.CompletedExchange || typed.RequestSHA256 == "" || len(typed.ConnectEventIDs) != 1 {
		t.Fatalf("validate recovery environment use: %#v %#v, %v", evidence, typed, err)
	}
	trace.Events[0].Resource.Path = "/workspace/other.sock"
	if _, _, err := validateLangGraphRecoveryEnvironmentUse(LangGraphForkPlan{WorkspaceSnapshot: LangGraphWorkspaceSnapshot{SourceWorkspace: source}}, trace); err == nil {
		t.Fatal("expected unmatched connect rejection")
	}
}

func TestLangGraphInternalUnixListenerRecordsAcknowledgedHealthExchange(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	runTarget := filepath.Clean(filepath.Join(filepath.Dir(testFile), "../../../targets/langgraph_shell_react/run_target.py"))
	if _, err := os.Stat(runTarget); err != nil {
		t.Fatalf("locate LangGraph target script: %v", err)
	}
	workspace := t.TempDir()
	endpoint := filepath.Join(workspace, "agent.sock")
	ready := filepath.Join(workspace, "ready.json")
	uses := filepath.Join(workspace, "environment-use-events.jsonl")
	command := exec.Command("python3", runTarget,
		"--internal-unix-socket-listener",
		"--internal-listener-endpoint", endpoint,
		"--internal-listener-role", "replacement",
		"--internal-listener-ready", ready,
		"--internal-listener-use-artifact", uses,
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start internal listener: %v", err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	listenerExited := false
	defer func() {
		if listenerExited {
			return
		}
		select {
		case <-finished:
			return
		default:
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-finished
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("internal listener did not write a readiness record")
		}
		select {
		case err := <-finished:
			listenerExited = true
			t.Skipf("internal listener is unavailable in this test environment: %v: %s", err, strings.TrimSpace(stderr.String()))
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	connection, err := net.DialTimeout("unix", endpoint, 2*time.Second)
	if err != nil {
		t.Fatalf("connect to internal listener: %v", err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("normal-health-request\n")); err != nil {
		t.Fatalf("write normal health request: %v", err)
	}
	response := make([]byte, 128)
	read, err := connection.Read(response)
	if err != nil {
		t.Fatalf("read listener response: %v", err)
	}
	if string(response[:read]) != "syncfuzz-listener-role:replacement\n" {
		t.Fatalf("unexpected listener response %q", string(response[:read]))
	}
	if _, err := connection.Write([]byte("syncfuzz-health-ack\n")); err != nil {
		t.Fatalf("acknowledge listener response: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection: %v", err)
	}
	for {
		data, readErr := os.ReadFile(uses)
		if readErr == nil && len(strings.TrimSpace(string(data))) > 0 {
			var use langGraphListenerUseEvent
			if err := json.Unmarshal(data, &use); err != nil {
				t.Fatalf("decode listener use event: %v", err)
			}
			if use.Role != "replacement" || use.Endpoint != endpoint || use.PeerPID <= 0 || use.RequestBytes != len("normal-health-request\n") || !validSHA256Hex(use.RequestSHA256) || !use.ResponseSent || !use.ResponseAcknowledged {
				t.Fatalf("listener did not record completed normal exchange: %#v", use)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("internal listener did not write completed use record: %v", readErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
