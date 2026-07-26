package environment_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func TestUnixSocketProgramCanonicalMutationLineage(t *testing.T) {
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:            "agent-service",
		ResolutionMode:         environment.UnixSocketResolutionConfig,
		ResolutionKey:          "agent_socket",
		ResolutionArtifactPath: "service.json",
		EndpointPath:           "agent.sock",
		InitialRole:            "benign",
		ActiveRole:             "benign",
		HolderLifetime:         environment.HolderLifetimeForeground,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram returned error: %v", err)
	}
	if err := baseline.Validate(); err != nil {
		t.Fatalf("baseline program did not validate: %v", err)
	}
	if len(baseline.Nodes) != 6 || len(baseline.Edges) != 5 {
		t.Fatalf("unexpected canonical config topology: nodes=%#v edges=%#v", baseline.Nodes, baseline.Edges)
	}
	mutated, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{
		Operator:   environment.MutationOperatorRebind,
		ActiveRole: "replacement",
	})
	if err != nil {
		t.Fatalf("MutateUnixSocket returned error: %v", err)
	}
	if mutated.Mutation.ParentProgramID != baseline.ProgramID || mutated.Mutation.Operator != environment.MutationOperatorRebind || mutated.UnixSocket.ActiveRole != "replacement" || mutated.ProgramID == baseline.ProgramID {
		t.Fatalf("mutation lineage was not preserved: %#v", mutated)
	}
	if err := mutated.Validate(); err != nil {
		t.Fatalf("mutated program did not validate: %v", err)
	}
	if _, err := mutated.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"}); err == nil {
		t.Fatal("expected no-op rebind mutation rejection")
	}
}

func TestUnixSocketProgramRejectsPathEscapeAndInvalidRebind(t *testing.T) {
	_, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:    "agent-service",
		ResolutionMode: environment.UnixSocketResolutionDirect,
		EndpointPath:   "../agent.sock",
		InitialRole:    "benign",
		ActiveRole:     "benign",
		HolderLifetime: environment.HolderLifetimeForeground,
	})
	if err == nil {
		t.Fatal("expected unsafe endpoint path rejection")
	}
	_, err = environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		ParentProgramID:  "environment-program:parent",
		MutationOperator: environment.MutationOperatorRebind,
		LogicalName:      "agent-service",
		ResolutionMode:   environment.UnixSocketResolutionDirect,
		EndpointPath:     "agent.sock",
		InitialRole:      "benign",
		ActiveRole:       "benign",
		HolderLifetime:   environment.HolderLifetimeForeground,
	})
	if err == nil {
		t.Fatal("expected no-op rebind rejection")
	}
}

func TestEnvironmentProgramRoundTripRejectsUnknownFields(t *testing.T) {
	program, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect,
		EndpointPath: "agent.sock", InitialRole: "baseline", ActiveRole: "baseline", HolderLifetime: environment.HolderLifetimeChild,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram returned error: %v", err)
	}
	path := filepath.Join(t.TempDir(), "program.json")
	if err := environment.WriteEnvironmentProgram(path, program); err != nil {
		t.Fatalf("WriteEnvironmentProgram returned error: %v", err)
	}
	loaded, err := environment.ReadEnvironmentProgram(path)
	if err != nil || loaded.ProgramID != program.ProgramID {
		t.Fatalf("environment program round trip failed: %#v, %v", loaded, err)
	}
	if err := os.WriteFile(path, append(mustReadFile(t, path)[:len(mustReadFile(t, path))-2], []byte(",\n  \"unknown\": true\n}\n")...), 0o644); err != nil {
		t.Fatalf("write malformed program: %v", err)
	}
	if _, err := environment.ReadEnvironmentProgram(path); err == nil {
		t.Fatal("expected unknown environment-program field rejection")
	}
}

func TestTargetUnixSocketMaterializationBindsApprovedProgram(t *testing.T) {
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
	artifact := environment.TargetUnixSocketMaterialization{
		SchemaVersion:               environment.TargetUnixSocketMaterializationSchemaVersion,
		ProgramID:                   program.ProgramID,
		SourceNativeCheckpointID:    "checkpoint-1",
		SourceCheckpointMonotonicNS: 100,
		EffectWindowMonotonicNS:     environment.TargetEffectWindow{Start: 100, End: 140},
		Family:                      environment.EnvironmentResourceFamilyUnixSocket,
		EndpointPath:                "agent.sock",
		LogicalName:                 "agent-service",
		ResolutionMode:              environment.UnixSocketResolutionDirect,
		ResolutionSteps: []environment.ResolutionStep{
			{Kind: environment.ResolutionStepLogicalName, From: "agent-service", To: "agent.sock"},
			{Kind: environment.ResolutionStepPathname, From: "agent.sock", To: "unix-endpoint:agent.sock"},
		},
		UseEventArtifactPath: "environment-use-events.jsonl",
		Listeners: []environment.TargetUnixSocketListener{
			{PID: 11, Role: "baseline", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 101, FD: 3, SocketID: "socket:11", SocketDevice: 1, SocketInode: 11, ReadyMonotonicNS: 120},
			{PID: 12, Role: "replacement", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 102, FD: 3, SocketID: "socket:12", SocketDevice: 1, SocketInode: 12, ReadyMonotonicNS: 130},
		},
		ActiveListener: environment.TargetUnixSocketListener{PID: 12, Role: "replacement", Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: 102, FD: 3, SocketID: "socket:12", SocketDevice: 1, SocketInode: 12, ReadyMonotonicNS: 130},
	}
	if err := artifact.ValidateFor(program); err != nil {
		t.Fatalf("ValidateFor returned error: %v", err)
	}
	active := artifact.ActiveBinding()
	if err := active.ValidateFor(program); err != nil || active.Semantic.Creator != environment.TargetUnixSocketMaterializerCreator || active.Local.HolderPID != 12 {
		t.Fatalf("target active binding translation is invalid: %#v, %v", active, err)
	}
	artifact.ActiveListener.Role = "baseline"
	if err := artifact.ValidateFor(program); err == nil {
		t.Fatal("expected active listener mismatch rejection")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestUnixSocketMaterializationResolvesConfigAndRebindsPathname(t *testing.T) {
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:            "agent-service",
		ResolutionMode:         environment.UnixSocketResolutionAlias,
		ResolutionKey:          "agent_alias",
		ResolutionArtifactPath: "service.alias",
		EndpointPath:           "agent.sock",
		InitialRole:            "benign",
		ActiveRole:             "benign",
		HolderLifetime:         environment.HolderLifetimeForeground,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram returned error: %v", err)
	}
	program, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatalf("MutateUnixSocket returned error: %v", err)
	}
	materialization, err := environment.MaterializeUnixSocketProgram(context.Background(), program, t.TempDir())
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("test sandbox does not permit Unix-domain sockets: %v", err)
		}
		t.Fatalf("MaterializeUnixSocketProgram returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := materialization.Close(); err != nil {
			t.Errorf("close materialization: %v", err)
		}
	})
	artifact := materialization.Artifact()
	if err := artifact.ValidateFor(program); err != nil {
		t.Fatalf("materialization artifact failed validation: %v", err)
	}
	if artifact.InitialBinding.Semantic.Role != "benign" || artifact.ActiveBinding.Semantic.Role != "replacement" || artifact.InitialBinding.Local.SocketID() == artifact.ActiveBinding.Local.SocketID() {
		t.Fatalf("materialization did not retain distinct initial/active listener identities: %#v", artifact)
	}
	resolved, err := materialization.ResolveLogicalName(context.Background(), "agent-service")
	if err != nil {
		t.Fatalf("ResolveLogicalName returned error: %v", err)
	}
	if resolved.EndpointPath != "agent.sock" || len(resolved.ResolutionSteps) != 3 {
		t.Fatalf("unexpected alias resolution: %#v", resolved)
	}
}

func TestUnixSocketMaterializationRefusesToOverwriteExistingEndpoint(t *testing.T) {
	program, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:    "agent-service",
		ResolutionMode: environment.UnixSocketResolutionDirect,
		EndpointPath:   "agent.sock",
		InitialRole:    "benign",
		ActiveRole:     "benign",
		HolderLifetime: environment.HolderLifetimeForeground,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram returned error: %v", err)
	}
	workspace := t.TempDir()
	endpoint := filepath.Join(workspace, "agent.sock")
	if err := os.WriteFile(endpoint, []byte("must-not-be-overwritten"), 0o600); err != nil {
		t.Fatalf("write existing endpoint fixture: %v", err)
	}
	if _, err := environment.MaterializeUnixSocketProgram(context.Background(), program, workspace); err == nil {
		t.Fatal("expected materializer to reject an existing endpoint")
	}
	contents, err := os.ReadFile(endpoint)
	if err != nil {
		t.Fatalf("read existing endpoint fixture: %v", err)
	}
	if string(contents) != "must-not-be-overwritten" {
		t.Fatalf("materializer altered an existing endpoint: %q", contents)
	}
}
