package hazard_test

import (
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/hazard"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

func TestRecoveryHazardClassifierRequiresUseAndControls(t *testing.T) {
	input := syntheticHazardInput(t, "benign")
	report, err := hazard.BuildRecoveryHazardReport(input)
	if err != nil {
		t.Fatalf("BuildRecoveryHazardReport returned error: %v", err)
	}
	if report.Status != hazard.RecoveryHazardStatusRealizedCalibration || report.Class != hazard.RecoveryHazardClassRebound {
		t.Fatalf("expected complete fixture calibration rebound, got %#v", report)
	}

	for index := range input.Controls {
		if input.Controls[index].Name == hazard.HazardControlTreatment {
			input.Controls[index].UseEvidence = nil
		}
	}
	missingUse, err := hazard.BuildRecoveryHazardReport(input)
	if err != nil {
		t.Fatalf("BuildRecoveryHazardReport with missing use returned error: %v", err)
	}
	if missingUse.Status != hazard.RecoveryHazardStatusInconclusive || missingUse.Class != hazard.RecoveryHazardClassNone {
		t.Fatalf("static residual must not upgrade without U': %#v", missingUse)
	}
}

func TestRecoveryHazardClassifierDoesNotCallKnownRoleARebound(t *testing.T) {
	input := syntheticHazardInput(t, "replacement")
	report, err := hazard.BuildRecoveryHazardReport(input)
	if err != nil {
		t.Fatalf("BuildRecoveryHazardReport returned error: %v", err)
	}
	if report.Status != hazard.RecoveryHazardStatusNotRealized || report.Class != hazard.RecoveryHazardClassNone {
		t.Fatalf("known replacement role must remain non-hazard: %#v", report)
	}
}

func TestRecoveryHazardRejectsCrossScopeEvidenceModes(t *testing.T) {
	input := syntheticHazardInput(t, "benign")
	input.WriteEvidence.Mode = hazard.WriteEvidenceModeProfileEBPF
	if _, err := hazard.BuildRecoveryHazardReport(input); err == nil {
		t.Fatal("fixture recovery evidence must reject target eBPF write mode")
	}

	input = syntheticHazardInput(t, "benign")
	for index := range input.Controls {
		if input.Controls[index].UseEvidence != nil {
			input.Controls[index].UseEvidence.EvidenceMode = hazard.UseEvidenceModeEBPFResolved
		}
	}
	if _, err := hazard.BuildRecoveryHazardReport(input); err == nil {
		t.Fatal("fixture recovery evidence must reject target eBPF use mode")
	}
}

func TestTargetRecoveryUseEvidenceConvertsCompletedExchangeWithoutPayload(t *testing.T) {
	workload, err := hazard.NewWorkload(hazard.WorkloadOptions{
		BaseProjectID: "target-health-client", InitialPrompt: "implement normal health client", ContinuationPrompt: "run the normal health command", RunnerConstraints: "target-unit-test",
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
	program, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	materialization := environment.TargetUnixSocketMaterialization{
		SchemaVersion: environment.TargetUnixSocketMaterializationSchemaVersion, ProgramID: program.ProgramID,
		SourceNativeCheckpointID: "checkpoint-1", SourceCheckpointMonotonicNS: 100,
		EffectWindowMonotonicNS: environment.TargetEffectWindow{Start: 100, End: 140},
		Family:                  environment.EnvironmentResourceFamilyUnixSocket, EndpointPath: "agent.sock", LogicalName: "agent-service", ResolutionMode: environment.UnixSocketResolutionDirect,
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
	if err := materialization.ValidateFor(program); err != nil {
		t.Fatal(err)
	}
	plan, err := hazard.NewUnixSocketRecoveryUsePlan(workload, program, "normal-health-request")
	if err != nil {
		t.Fatal(err)
	}
	observed := recovery.EnvironmentUseEvidence{
		SchemaVersion: recovery.EnvironmentUseEvidenceSchemaVersion, Family: "unix-socket", ProgramID: program.ProgramID, LogicalName: "agent-service", ResolvedEndpointPath: "/workspace/agent.sock", ConnectEventIDs: []string{"connect-1"}, RequestSHA256: plan.RequestSHA256, CompletedExchange: true,
		ListenerRole: "replacement", ListenerPID: 12, ListenerFD: 3, ListenerSocketID: "socket:12", ListenerEndpointDevice: 1, ListenerEndpointInode: 102, ListenerSocketDevice: 1, ListenerSocketInode: 12,
	}
	evidence, err := hazard.NewUnixSocketUseEvidenceFromTargetRecovery(workload, plan, program, materialization, observed)
	if err != nil {
		t.Fatalf("NewUnixSocketUseEvidenceFromTargetRecovery: %v", err)
	}
	if evidence.EvidenceMode != hazard.UseEvidenceModeEBPFResolved || !evidence.IOObserved || evidence.ListenerSemantic.Creator != environment.TargetUnixSocketMaterializerCreator || evidence.ListenerLocal.HolderPID != 12 {
		t.Fatalf("target evidence lost completed-exchange identity: %#v", evidence)
	}
	instance := hazard.HazardEnvironmentInstance{InstanceID: "tainted", Program: program, TargetMaterialization: &materialization}
	if err := instance.Validate(); err != nil {
		t.Fatalf("target environment instance validation: %v", err)
	}
}

func syntheticHazardInput(t *testing.T, treatmentExpectedRole string) hazard.RecoveryHazardReportInput {
	t.Helper()
	workload, err := hazard.NewWorkload(hazard.WorkloadOptions{
		BaseProjectID:      "synthetic-calibration",
		InitialPrompt:      "prepare local service",
		ContinuationPrompt: "continue normal health workflow",
		RunnerConstraints:  "unit-test",
	})
	if err != nil {
		t.Fatalf("NewWorkload: %v", err)
	}
	baseline, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:    "agent-service",
		ResolutionMode: environment.UnixSocketResolutionDirect,
		EndpointPath:   "agent.sock",
		InitialRole:    "benign",
		ActiveRole:     "benign",
		HolderLifetime: environment.HolderLifetimeForeground,
	})
	if err != nil {
		t.Fatalf("NewUnixSocketProgram baseline: %v", err)
	}
	tainted, err := baseline.MutateUnixSocket(environment.UnixSocketMutation{Operator: environment.MutationOperatorRebind, ActiveRole: "replacement"})
	if err != nil {
		t.Fatalf("MutateUnixSocket: %v", err)
	}
	taintedMaterialization := syntheticMaterialization(t, tainted, 10, 20)
	cleanAMaterialization := syntheticMaterialization(t, baseline, 30, 30)
	cleanBMaterialization := syntheticMaterialization(t, baseline, 40, 40)
	taintedPlan, err := hazard.NewUnixSocketRecoveryUsePlan(workload, tainted, "normal-health-request")
	if err != nil {
		t.Fatalf("NewUnixSocketRecoveryUsePlan tainted: %v", err)
	}
	cleanPlan, err := hazard.NewUnixSocketRecoveryUsePlan(workload, baseline, "normal-health-request")
	if err != nil {
		t.Fatalf("NewUnixSocketRecoveryUsePlan clean: %v", err)
	}
	recoveryEvidence, err := hazard.NewFixtureHistoricalRecoveryEvidence("synthetic-profile", "before-bind..after-bind", "before-bind", "after-bind", "head")
	if err != nil {
		t.Fatalf("NewFixtureHistoricalRecoveryEvidence: %v", err)
	}
	treatmentUse := syntheticUseEvidence(taintedPlan, tainted, taintedMaterialization.ActiveBinding)
	afterUse := syntheticUseEvidence(taintedPlan, tainted, taintedMaterialization.ActiveBinding)
	headUse := syntheticUseEvidence(taintedPlan, tainted, taintedMaterialization.ActiveBinding)
	ablationUse := syntheticUseEvidence(cleanPlan, baseline, cleanAMaterialization.ActiveBinding)
	baselineUse := syntheticUseEvidence(cleanPlan, baseline, cleanBMaterialization.ActiveBinding)
	return hazard.RecoveryHazardReportInput{
		Calibration:      true,
		Workload:         workload,
		RecoveryEvidence: recoveryEvidence,
		WriteEvidence: hazard.MaterializationWriteEvidence{
			Mode:       hazard.WriteEvidenceModeFixtureTelemetry,
			FrontierID: recoveryEvidence.FrontierID,
			Operations: []string{"bind", "listen", "rebind"},
		},
		Environments: []hazard.HazardEnvironmentInstance{
			{InstanceID: "tainted", Program: tainted, Materialization: taintedMaterialization},
			{InstanceID: "clean-a", Program: baseline, Materialization: cleanAMaterialization},
			{InstanceID: "clean-b", Program: baseline, Materialization: cleanBMaterialization},
		},
		Controls: []hazard.RecoveryHazardControl{
			{Name: hazard.HazardControlTreatment, CheckpointID: recoveryEvidence.Before.CheckpointID, RuntimeInstanceID: recoveryEvidence.Before.RuntimeInstanceID, StaticOutcome: recoveryEvidence.Before.StaticOutcome, EnvironmentInstanceID: "tainted", ExpectedRole: treatmentExpectedRole, UsePlan: taintedPlan, UseEvidence: &treatmentUse},
			{Name: hazard.HazardControlFrontierLocal, CheckpointID: recoveryEvidence.After.CheckpointID, RuntimeInstanceID: recoveryEvidence.After.RuntimeInstanceID, StaticOutcome: recoveryEvidence.After.StaticOutcome, EnvironmentInstanceID: "tainted", ExpectedRole: "replacement", UsePlan: taintedPlan, UseEvidence: &afterUse},
			{Name: hazard.HazardControlHead, CheckpointID: recoveryEvidence.Head.CheckpointID, RuntimeInstanceID: recoveryEvidence.Head.RuntimeInstanceID, StaticOutcome: recoveryEvidence.Head.StaticOutcome, EnvironmentInstanceID: "tainted", ExpectedRole: "replacement", UsePlan: taintedPlan, UseEvidence: &headUse},
			{Name: hazard.HazardControlRetentionAblation, CheckpointID: recoveryEvidence.Before.CheckpointID, RuntimeInstanceID: "synthetic-runtime:retention-ablation", StaticOutcome: hazard.HazardStaticOutcomeNotApplicable, EnvironmentInstanceID: "clean-a", ExpectedRole: "benign", UsePlan: cleanPlan, UseEvidence: &ablationUse},
			{Name: hazard.HazardControlCleanBaseline, CheckpointID: recoveryEvidence.Head.CheckpointID, RuntimeInstanceID: "synthetic-runtime:clean-baseline", StaticOutcome: hazard.HazardStaticOutcomeNotApplicable, EnvironmentInstanceID: "clean-b", ExpectedRole: "benign", UsePlan: cleanPlan, UseEvidence: &baselineUse},
		},
	}
}

func syntheticMaterialization(t *testing.T, program environment.EnvironmentProgram, initialSocketInode, activeSocketInode uint64) environment.EnvironmentMaterialization {
	t.Helper()
	steps := []environment.ResolutionStep{
		{Kind: environment.ResolutionStepLogicalName, From: program.UnixSocket.LogicalName, To: program.UnixSocket.EndpointPath},
		{Kind: environment.ResolutionStepPathname, From: program.UnixSocket.EndpointPath, To: "unix-endpoint:" + program.UnixSocket.EndpointPath},
	}
	resolutionDigest := environment.ResolutionStepsDigest(steps)
	newBinding := func(role string, socketInode uint64, endpointInode uint64) environment.MaterializedUnixSocketBinding {
		return environment.MaterializedUnixSocketBinding{
			Semantic:  environment.SemanticIdentity{ProgramID: program.ProgramID, LogicalName: program.UnixSocket.LogicalName, Role: role, ResolutionSHA256: resolutionDigest, Creator: "synthetic-test"},
			Local:     environment.RunLocalIdentity{EndpointDevice: 1, EndpointInode: endpointInode, SocketDevice: 1, SocketInode: socketInode, HolderPID: 100, HolderFD: int(socketInode)},
			Listening: true,
		}
	}
	initial := newBinding(program.UnixSocket.InitialRole, initialSocketInode, initialSocketInode+100)
	active := newBinding(program.UnixSocket.ActiveRole, activeSocketInode, activeSocketInode+100)
	events := []environment.MaterializationEvent{
		{Sequence: 1, Operation: "bind", Role: initial.Semantic.Role, Binding: &initial},
		{Sequence: 2, Operation: "listen", Role: initial.Semantic.Role, Binding: &initial},
	}
	if program.Mutation.Operator == environment.MutationOperatorRebind {
		events = append(events,
			environment.MaterializationEvent{Sequence: 3, Operation: "unlink", Role: initial.Semantic.Role},
			environment.MaterializationEvent{Sequence: 4, Operation: "bind", Role: active.Semantic.Role, Binding: &active},
			environment.MaterializationEvent{Sequence: 5, Operation: "listen", Role: active.Semantic.Role, Binding: &active},
			environment.MaterializationEvent{Sequence: 6, Operation: "rebind", Role: active.Semantic.Role, Binding: &active},
		)
	}
	materialization := environment.EnvironmentMaterialization{
		SchemaVersion:   environment.EnvironmentMaterializationSchemaVersion,
		ProgramID:       program.ProgramID,
		Family:          program.Family,
		EndpointPath:    program.UnixSocket.EndpointPath,
		ResolutionSteps: steps,
		InitialBinding:  initial,
		ActiveBinding:   active,
		Events:          events,
	}
	if err := materialization.ValidateFor(program); err != nil {
		t.Fatalf("synthetic materialization did not validate: %v", err)
	}
	return materialization
}

func syntheticUseEvidence(plan hazard.RecoveryUsePlan, program environment.EnvironmentProgram, binding environment.MaterializedUnixSocketBinding) hazard.UnixSocketUseEvidence {
	return hazard.UnixSocketUseEvidence{
		SchemaVersion:        hazard.UnixSocketUseEvidenceSchemaVersion,
		EvidenceMode:         hazard.UseEvidenceModeFixtureRoundTrip,
		RecoveryUsePlanID:    plan.RecoveryUsePlanID,
		LogicalName:          plan.LogicalName,
		ResolvedEndpointPath: program.UnixSocket.EndpointPath,
		ResolutionSteps: []environment.ResolutionStep{
			{Kind: environment.ResolutionStepLogicalName, From: plan.LogicalName, To: program.UnixSocket.EndpointPath},
			{Kind: environment.ResolutionStepPathname, From: program.UnixSocket.EndpointPath, To: "unix-endpoint:" + program.UnixSocket.EndpointPath},
		},
		ConnectObserved:  true,
		IOObserved:       true,
		RequestSHA256:    plan.RequestSHA256,
		ResponseSHA256:   strings.Repeat("b", 64),
		ListenerRole:     binding.Semantic.Role,
		ListenerSemantic: binding.Semantic,
		ListenerLocal:    binding.Local,
	}
}
