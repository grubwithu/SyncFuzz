package hazard

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

func TestBuildLangGraphTargetRecoveryHazardReportUsesSeparateCleanRuntime(t *testing.T) {
	workload, err := NewWorkload(WorkloadOptions{
		BaseProjectID: "langgraph-health-client", InitialPrompt: "implement the normal health client", ContinuationPrompt: "Run the standard health command.", RunnerConstraints: "unit-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect, EndpointPath: "agent.sock", InitialRole: "baseline", ActiveRole: "baseline", HolderLifetime: environment.HolderLifetimeChild,
	})
	if err != nil {
		t.Fatal(err)
	}
	tainted, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	taintedMaterialization := targetTestMaterialization(t, tainted, "baseline", "replacement", 10)
	cleanMaterialization := targetTestMaterialization(t, baseline, "baseline", "baseline", 20)
	taintedSeed, taintedSet, taintedExecution := targetTestRecovery(t, "tainted", workload.ContinuationPrompt, tainted, taintedMaterialization, true)
	cleanSeed, cleanSet, cleanExecution := targetTestRecovery(t, "clean", workload.ContinuationPrompt, baseline, cleanMaterialization, false)
	report, err := BuildLangGraphTargetRecoveryHazardReport(LangGraphTargetHazardInput{
		Workload:    workload,
		TaintedSeed: taintedSeed, TaintedRecoverySet: taintedSet, TaintedRecoveryExecution: taintedExecution, TaintedProgram: tainted, TaintedMaterialization: taintedMaterialization,
		CleanSeed: cleanSeed, CleanRecoverySet: cleanSet, CleanRecoveryExecution: cleanExecution, CleanProgram: baseline, CleanMaterialization: cleanMaterialization,
	})
	if err != nil {
		t.Fatalf("BuildLangGraphTargetRecoveryHazardReport: %v", err)
	}
	if report.Status != RecoveryHazardStatusRealized || report.Class != RecoveryHazardClassRebound || report.Calibration {
		t.Fatalf("expected realized target rebound report, got %#v", report)
	}
	controls := controlsByName(report.Controls)
	if controls[HazardControlRetentionAblation].CheckpointID == report.RecoveryEvidence.Before.CheckpointID || controls[HazardControlCleanBaseline].CheckpointID == report.RecoveryEvidence.Head.CheckpointID {
		t.Fatal("clean controls must retain their own raw checkpoint provenance")
	}
	if controls[HazardControlRetentionAblation].EffectiveLogicalCoordinateID() != report.RecoveryEvidence.Before.EffectiveLogicalCoordinateID() || controls[HazardControlCleanBaseline].EffectiveLogicalCoordinateID() != report.RecoveryEvidence.Head.EffectiveLogicalCoordinateID() {
		t.Fatal("clean controls lost structural coordinate equivalence")
	}
	artifact := filepath.Join(t.TempDir(), "recovery-hazard-report.json")
	if err := WriteRecoveryHazardReport(artifact, report); err != nil {
		t.Fatalf("WriteRecoveryHazardReport: %v", err)
	}
	loaded, err := ReadRecoveryHazardReport(artifact)
	if err != nil {
		t.Fatalf("ReadRecoveryHazardReport: %v", err)
	}
	if loaded.ReportID != report.ReportID || loaded.Controls[0].UsePlan.Request != "" {
		t.Fatalf("target report did not preserve digest-only use plan: %#v", loaded)
	}
	// A semantically equivalent clean control must still receive a different
	// report ID when it came from another fresh source run. Otherwise a clean
	// artifact could be silently substituted after the target experiment.
	secondCleanMaterialization := targetTestMaterialization(t, baseline, "baseline", "baseline", 40)
	secondCleanSeed, secondCleanSet, secondCleanExecution := targetTestRecovery(t, "clean-second", workload.ContinuationPrompt, baseline, secondCleanMaterialization, false)
	secondReport, err := BuildLangGraphTargetRecoveryHazardReport(LangGraphTargetHazardInput{
		Workload:    workload,
		TaintedSeed: taintedSeed, TaintedRecoverySet: taintedSet, TaintedRecoveryExecution: taintedExecution, TaintedProgram: tainted, TaintedMaterialization: taintedMaterialization,
		CleanSeed: secondCleanSeed, CleanRecoverySet: secondCleanSet, CleanRecoveryExecution: secondCleanExecution, CleanProgram: baseline, CleanMaterialization: secondCleanMaterialization,
	})
	if err != nil {
		t.Fatalf("BuildLangGraphTargetRecoveryHazardReport with second clean run: %v", err)
	}
	if secondReport.ReportID == report.ReportID {
		t.Fatal("report ID did not bind fresh clean-run provenance")
	}
}

func TestBuildLangGraphTargetRecoveryHazardReportRejectsChangingRequestDigest(t *testing.T) {
	workload, err := NewWorkload(WorkloadOptions{BaseProjectID: "langgraph-health-client", InitialPrompt: "implement normal health client", ContinuationPrompt: "Run the standard health command.", RunnerConstraints: "unit-test"})
	if err != nil {
		t.Fatal(err)
	}
	clean, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect, EndpointPath: "agent.sock", InitialRole: "baseline", ActiveRole: "baseline", HolderLifetime: environment.HolderLifetimeChild})
	if err != nil {
		t.Fatal(err)
	}
	tainted, err := clean.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	taintedMaterialization := targetTestMaterialization(t, tainted, "baseline", "replacement", 10)
	cleanMaterialization := targetTestMaterialization(t, clean, "baseline", "baseline", 20)
	taintedSeed, taintedSet, taintedExecution := targetTestRecovery(t, "tainted", workload.ContinuationPrompt, tainted, taintedMaterialization, true)
	cleanSeed, cleanSet, cleanExecution := targetTestRecovery(t, "clean", workload.ContinuationPrompt, clean, cleanMaterialization, false)
	cleanExecution.After.EnvironmentUseEvidence.RequestSHA256 = digest("a-different-normal-request\n")
	_, err = BuildLangGraphTargetRecoveryHazardReport(LangGraphTargetHazardInput{
		Workload:    workload,
		TaintedSeed: taintedSeed, TaintedRecoverySet: taintedSet, TaintedRecoveryExecution: taintedExecution, TaintedProgram: tainted, TaintedMaterialization: taintedMaterialization,
		CleanSeed: cleanSeed, CleanRecoverySet: cleanSet, CleanRecoveryExecution: cleanExecution, CleanProgram: clean, CleanMaterialization: cleanMaterialization,
	})
	if err == nil {
		t.Fatal("expected changing request digest to be rejected")
	}
}

func targetTestMaterialization(t *testing.T, program environment.EnvironmentProgram, initialRole, activeRole string, base int) environment.TargetUnixSocketMaterialization {
	t.Helper()
	materialization := environment.TargetUnixSocketMaterialization{
		SchemaVersion: environment.TargetUnixSocketMaterializationSchemaVersion, ProgramID: program.ProgramID,
		SourceNativeCheckpointID: "native-before", SourceCheckpointMonotonicNS: 100,
		EffectWindowMonotonicNS: environment.TargetEffectWindow{Start: 100, End: 140},
		Family:                  environment.EnvironmentResourceFamilyUnixSocket, EndpointPath: "agent.sock", LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect,
		ResolutionSteps: []environment.ResolutionStep{
			{Kind: environment.ResolutionStepLogicalName, From: "agent-service", To: "agent.sock"},
			{Kind: environment.ResolutionStepPathname, From: "agent.sock", To: "unix-endpoint:agent.sock"},
		},
		UseEventArtifactPath: "environment-use-events.jsonl",
		Listeners: []environment.TargetUnixSocketListener{
			{PID: base + 1, Role: initialRole, Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: uint64(base + 101), FD: 3, SocketID: "socket:" + itoa(base+11), SocketDevice: 1, SocketInode: uint64(base + 11), ReadyMonotonicNS: 120},
			{PID: base + 2, Role: activeRole, Endpoint: "/workspace/agent.sock", EndpointDevice: 1, EndpointInode: uint64(base + 102), FD: 3, SocketID: "socket:" + itoa(base+12), SocketDevice: 1, SocketInode: uint64(base + 12), ReadyMonotonicNS: 130},
		},
	}
	materialization.ActiveListener = materialization.Listeners[1]
	if err := materialization.ValidateFor(program); err != nil {
		t.Fatalf("test materialization: %v", err)
	}
	return materialization
}

func targetTestRecovery(t *testing.T, name, continuationText string, program environment.EnvironmentProgram, materialization environment.TargetUnixSocketMaterialization, tainted bool) (objective.StateSeed, recovery.HistoricalRecoverySet, recovery.ForkRecoverySetExecution) {
	t.Helper()
	root := t.TempDir()
	planPath := filepath.Join(root, "langgraph-fork-plan.json")
	seed := objective.StateSeed{
		SchemaVersion: objective.SchemaVersion, SeedID: "seed-" + name, ObjectiveID: "ipc-unix-listener", ProfileRunID: "profile-" + name, ProfileRunKind: objective.ProfileRunKindSynthesisCandidate,
		SynthesisCandidateID: "synthesis-candidate:shared", NativeCheckpointRunID: "native-run-" + name, TargetID: "langgraph-shell-react", AdapterID: recovery.LangGraphForkAdapterID,
		RecordedPlanID: "recorded-plan-" + name, RecordedPlanArtifact: planPath, FrontierID: "before..after", BeforeCheckpointID: "before-" + name, AfterCheckpointID: "after-" + name,
		MaterializationHeadCheckpointID: "head-" + name, MaterializationHeadMonotonicNS: 300, MaterializationHeadResourceIDs: []string{"unix-socket:agent.sock"},
		ValidatedEffects: []objective.EffectAtom{{Family: profiling.StateFamilyIPC, Operation: "bind"}, {Family: profiling.StateFamilyIPC, Operation: "listen"}}, ResourceIDs: []string{"unix-socket:agent.sock"},
	}
	continuation, err := recovery.NewContinuationQuery(continuationText)
	if err != nil {
		t.Fatal(err)
	}
	head, err := recovery.MaterializationHeadFor(seed)
	if err != nil {
		t.Fatal(err)
	}
	recorded := recovery.RecordedPlan{SchemaVersion: recovery.SchemaVersion, RecordedPlanID: seed.RecordedPlanID, AdapterID: seed.AdapterID, TargetID: seed.TargetID, ExecutionArtifact: seed.RecordedPlanArtifact, PassiveObservationID: "unix-socket-listener-holder-v1:agent.sock", MaterializationHeadID: head.HeadID, RetentionPolicy: recovery.RetentionPolicyRetainRelevantOSState}
	set, err := recovery.NewForkRecoverySetWithContinuation(seed, recorded, continuation)
	if err != nil {
		t.Fatal(err)
	}
	forkPlan := recovery.LangGraphForkPlan{SchemaVersion: recovery.LangGraphForkPlanSchema, CheckpointCoordinates: map[string]recovery.LangGraphNativeCheckpointCoordinate{
		seed.BeforeCheckpointID:              {SchemaVersion: recovery.LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-before-" + name, HistoryIndex: 1, MessageCount: 2, Next: []string{"tools"}},
		seed.AfterCheckpointID:               {SchemaVersion: recovery.LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-after-" + name, HistoryIndex: 2, MessageCount: 4, Next: []string{"agent"}},
		seed.MaterializationHeadCheckpointID: {SchemaVersion: recovery.LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-head-" + name, HistoryIndex: 3, MessageCount: 6, Next: []string{}},
	}}
	if err := recovery.WriteLangGraphForkPlan(planPath, forkPlan); err != nil {
		t.Fatal(err)
	}
	execution, err := recovery.ExecuteForkRecoverySet(context.Background(), seed, *set, recorded, recovery.ForkExecutorFunc(func(_ context.Context, request recovery.ForkExecutionRequest) (recovery.RecoveryObservation, error) {
		active := materialization.ActiveListener
		use := &recovery.EnvironmentUseEvidence{SchemaVersion: recovery.EnvironmentUseEvidenceSchemaVersion, Family: "unix-socket", ProgramID: program.ProgramID, LogicalName: program.UnixSocket.LogicalName, ResolvedEndpointPath: "/workspace/agent.sock", ConnectEventIDs: []string{"connect-" + request.Query.CheckpointID}, RequestSHA256: digest("normal-health-request\n"), CompletedExchange: true, ListenerRole: active.Role, ListenerPID: active.PID, ListenerFD: active.FD, ListenerSocketID: active.SocketID, ListenerEndpointDevice: active.EndpointDevice, ListenerEndpointInode: active.EndpointInode, ListenerSocketDevice: active.SocketDevice, ListenerSocketInode: active.SocketInode}
		agent := recovery.StatePresencePresent
		if request.Query.CheckpointID == seed.BeforeCheckpointID {
			agent = recovery.StatePresenceAbsent
		}
		return recovery.RecoveryObservation{SchemaVersion: recovery.ExecutionSchemaVersion, QueryID: request.Query.QueryID, SeedID: request.Query.SeedID, Boundary: request.Query.Boundary, CheckpointID: request.Query.CheckpointID, RecordedPlanID: request.Query.RecordedPlanID, PassiveObservationID: request.Query.PassiveObservationID, MaterializationHeadID: request.Query.MaterializationHeadID, RetentionPolicy: request.Query.RetentionPolicy, RuntimeInstanceID: "runtime-" + name + "-" + request.Query.CheckpointID, AgentState: agent, OSState: recovery.StatePresencePresent, OSStateOrigin: recovery.StateOriginResidual, EffectMultiplicity: recovery.EffectMultiplicitySingle, ContinuationEvidence: &recovery.ContinuationEvidence{ContinuationQueryID: continuation.ContinuationQueryID, PreEvidence: []string{"pre"}, PostEvidence: []string{"post"}}, EnvironmentUseEvidence: use, Evidence: []string{"target test"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !tainted {
		// The clean execution may have any static relation; its two selected
		// controls are explicitly marked not-applicable by the target builder.
	}
	return seed, *set, *execution
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
