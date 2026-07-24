package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLangGraphProbeFidelityReportAggregatesPairedTrials(t *testing.T) {
	input := testLangGraphProbeFidelityInput("runs/fidelity/trial-001")
	report, err := BuildLangGraphProbeFidelityReport([]LangGraphProbeFidelityTrialInput{input})
	if err != nil {
		t.Fatalf("BuildLangGraphProbeFidelityReport returned error: %v", err)
	}
	if report.SchemaVersion != LangGraphProbeFidelityReportSchema || len(report.Trials) != 1 {
		t.Fatalf("unexpected report identity: %#v", report)
	}
	if report.Full.Metrics.SampleCount != 3 || report.Full.Metrics.TotalDurationNS != 600 || report.Full.Metrics.MeanDurationNS != 200 || report.Full.Metrics.TotalScannedProcesses != 12 || report.Full.Metrics.MeanScannedFDs != 10 {
		t.Fatalf("unexpected full metrics: %#v", report.Full.Metrics)
	}
	if report.Pruned.Metrics.SampleCount != 3 || report.Pruned.Metrics.TotalDurationNS != 150 || report.Pruned.Metrics.MeanDurationNS != 50 || report.Pruned.Metrics.TotalScannedProcesses != 3 || report.Pruned.Metrics.MeanScannedFDs != 1 {
		t.Fatalf("unexpected pruned metrics: %#v", report.Pruned.Metrics)
	}
	if report.Full.RecoverySetOutcomes["residual"] != 1 || report.Pruned.RecoverySetOutcomes["inconclusive"] != 1 {
		t.Fatalf("unexpected mode outcomes: full=%#v pruned=%#v", report.Full.RecoverySetOutcomes, report.Pruned.RecoverySetOutcomes)
	}
	if report.Comparison.PairedTrials != 1 || report.Comparison.ExactLayerStateOriginMatches != 1 || report.Comparison.FinalOutcomeMatches != 0 || report.Comparison.FullMultiplicityProofs != 1 || report.Comparison.PrunedMultiplicityUnknownTrials != 1 {
		t.Fatalf("unexpected comparison summary: %#v", report.Comparison)
	}
	trial := report.Trials[0]
	if !trial.ExactLayerStateOriginMatch || !trial.FullMultiplicityProven || !trial.PrunedMultiplicityUnknown || trial.FullOutcome != "residual" || trial.PrunedOutcome != "inconclusive" {
		t.Fatalf("unexpected trial result: %#v", trial)
	}
}

func TestBuildLangGraphProbeFidelityReportRejectsUnpairedSource(t *testing.T) {
	input := testLangGraphProbeFidelityInput("runs/fidelity/trial-001")
	input.PrunedPlan.WorkspaceSnapshot.WorkspaceSHA256 = strings.Repeat("c", 64)
	if _, err := BuildLangGraphProbeFidelityReport([]LangGraphProbeFidelityTrialInput{input}); err == nil || !strings.Contains(err.Error(), "do not share source runtime") {
		t.Fatalf("expected source identity mismatch to be rejected, got %v", err)
	}
}

func TestBuildLangGraphProbeFidelityReportRequiresStructuredProbeMetrics(t *testing.T) {
	input := testLangGraphProbeFidelityInput("runs/fidelity/trial-001")
	input.PrunedExecution.Before.PassiveProbe = nil
	if _, err := BuildLangGraphProbeFidelityReport([]LangGraphProbeFidelityTrialInput{input}); err == nil || !strings.Contains(err.Error(), "lacks pruned passive probe metrics") {
		t.Fatalf("expected missing probe metrics to be rejected, got %v", err)
	}
}

func TestReadLangGraphProbeFidelityTrialUsesStandardArtifactLayout(t *testing.T) {
	root := t.TempDir()
	input := testLangGraphProbeFidelityInput(root)
	writeLangGraphProbeFidelityTrial(t, root, input)
	loaded, err := ReadLangGraphProbeFidelityTrial(root)
	if err != nil {
		t.Fatalf("ReadLangGraphProbeFidelityTrial returned error: %v", err)
	}
	if report, err := BuildLangGraphProbeFidelityReport([]LangGraphProbeFidelityTrialInput{loaded}); err != nil || report.Comparison.PairedTrials != 1 {
		t.Fatalf("loaded trial did not build a report: report=%#v err=%v", report, err)
	}
}

func TestBuildLangGraphProbeFidelityBatchReportKeepsRejectedAttempts(t *testing.T) {
	accepted := testLangGraphProbeFidelityInput("runs/fidelity/batch/attempt-001")
	report, err := BuildLangGraphProbeFidelityBatchReport(2, 4, []LangGraphProbeFidelityBatchAttemptInput{
		{
			Attempt: LangGraphProbeFidelityAttempt{
				SchemaVersion: LangGraphProbeFidelityAttemptSchema,
				AttemptIndex:  1,
				ArtifactRoot:  accepted.ArtifactRoot,
				Status:        LangGraphProbeFidelityAttemptAccepted,
			},
			Trial: &accepted,
		},
		{
			Attempt: LangGraphProbeFidelityAttempt{
				SchemaVersion: LangGraphProbeFidelityAttemptSchema,
				AttemptIndex:  2,
				ArtifactRoot:  "runs/fidelity/batch/attempt-002",
				Status:        LangGraphProbeFidelityAttemptRejectedSourceBaseline,
				Reason:        "invalid-unix-listener-baseline",
				FailureStage:  "fidelity",
				LogArtifact:   "attempt.log",
			},
		},
		{
			Attempt: LangGraphProbeFidelityAttempt{
				SchemaVersion: LangGraphProbeFidelityAttemptSchema,
				AttemptIndex:  3,
				ArtifactRoot:  "runs/fidelity/batch/attempt-003",
				Status:        LangGraphProbeFidelityAttemptExecutionFailed,
				Reason:        "child-target-failed",
				FailureStage:  "fidelity",
				LogArtifact:   "attempt.log",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildLangGraphProbeFidelityBatchReport returned error: %v", err)
	}
	if report.SchemaVersion != LangGraphProbeFidelityBatchReportSchema || report.AttemptCount != 3 || report.AcceptedTrialCount != 1 || report.RejectedSourceBaselineCount != 1 || report.ExecutionFailureCount != 1 || report.Complete {
		t.Fatalf("unexpected batch denominator: %#v", report)
	}
	if report.Fidelity == nil || report.Fidelity.Comparison.PairedTrials != 1 {
		t.Fatalf("accepted trial did not contribute fidelity evidence: %#v", report.Fidelity)
	}
}

func TestReadLangGraphProbeFidelityBatchAttemptsUsesAttemptRecords(t *testing.T) {
	root := t.TempDir()
	acceptedRoot := filepath.Join(root, "attempt-001")
	accepted := testLangGraphProbeFidelityInput(acceptedRoot)
	writeLangGraphProbeFidelityTrial(t, acceptedRoot, accepted)
	if err := WriteLangGraphProbeFidelityAttempt(filepath.Join(acceptedRoot, "attempt.json"), LangGraphProbeFidelityAttempt{
		SchemaVersion: LangGraphProbeFidelityAttemptSchema,
		AttemptIndex:  1,
		ArtifactRoot:  acceptedRoot,
		Status:        LangGraphProbeFidelityAttemptAccepted,
	}); err != nil {
		t.Fatalf("write accepted attempt: %v", err)
	}
	rejectedRoot := filepath.Join(root, "attempt-002")
	if err := os.MkdirAll(rejectedRoot, 0o755); err != nil {
		t.Fatalf("create rejected attempt directory: %v", err)
	}
	if err := WriteLangGraphProbeFidelityAttempt(filepath.Join(rejectedRoot, "attempt.json"), LangGraphProbeFidelityAttempt{
		SchemaVersion: LangGraphProbeFidelityAttemptSchema,
		AttemptIndex:  2,
		ArtifactRoot:  rejectedRoot,
		Status:        LangGraphProbeFidelityAttemptRejectedSourceBaseline,
		Reason:        "invalid-unix-listener-baseline",
		FailureStage:  "fidelity",
		LogArtifact:   "attempt.log",
	}); err != nil {
		t.Fatalf("write rejected attempt: %v", err)
	}
	inputs, err := ReadLangGraphProbeFidelityBatchAttempts(root)
	if err != nil {
		t.Fatalf("ReadLangGraphProbeFidelityBatchAttempts returned error: %v", err)
	}
	if len(inputs) != 2 || inputs[0].Trial == nil || inputs[1].Trial != nil || inputs[1].Attempt.Status != LangGraphProbeFidelityAttemptRejectedSourceBaseline {
		t.Fatalf("unexpected loaded batch attempts: %#v", inputs)
	}
}

func writeLangGraphProbeFidelityTrial(t *testing.T, root string, input LangGraphProbeFidelityTrialInput) {
	t.Helper()
	for _, entry := range []struct {
		path  string
		value any
	}{
		{filepath.Join(root, "full", "langgraph-fork-plan.json"), input.FullPlan},
		{filepath.Join(root, "pruned", "langgraph-fork-plan.json"), input.PrunedPlan},
		{filepath.Join(root, "full", "recovery-set-execution.json"), input.FullExecution},
		{filepath.Join(root, "pruned", "recovery-set-execution.json"), input.PrunedExecution},
	} {
		if err := os.MkdirAll(filepath.Dir(entry.path), 0o755); err != nil {
			t.Fatalf("create artifact directory: %v", err)
		}
		data, err := json.Marshal(entry.value)
		if err != nil {
			t.Fatalf("encode artifact: %v", err)
		}
		if err := os.WriteFile(entry.path, data, 0o644); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}
}

func testLangGraphProbeFidelityInput(root string) LangGraphProbeFidelityTrialInput {
	coordinates := map[string]LangGraphNativeCheckpointCoordinate{
		"C0": {SchemaVersion: LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-before", HistoryIndex: 0, MessageCount: 0},
		"C1": {SchemaVersion: LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-after", HistoryIndex: 1, MessageCount: 1, Next: []string{"agent"}},
		"C2": {SchemaVersion: LangGraphNativeCoordinateSchema, SourceCheckpointID: "native-head", HistoryIndex: 2, MessageCount: 2},
	}
	basePlan := LangGraphForkPlan{
		SchemaVersion:                   LangGraphForkPlanSchema,
		RecordedPlanID:                  "recorded-plan:profile-1",
		AdapterID:                       LangGraphForkAdapterID,
		TargetID:                        "langgraph-shell-react",
		CandidateID:                     "candidate-1",
		Task:                            "preserve listener",
		Model:                           "openai:test",
		ContainerImage:                  "syncfuzz-langgraph:test",
		RuntimeRoot:                     root + "/full/recovery-runtimes",
		PassiveUnixSocketPath:           "agent.sock",
		PassiveProbeMode:                LangGraphPassiveProbeFull,
		PassiveObservationID:            "unix-socket-listener-holder-v1:agent.sock",
		MaterializationHeadID:           "materialization-head:profile-1:C2",
		MaterializationHeadCheckpointID: "C2",
		SourceThreadID:                  "profile-thread",
		SourceRuntime:                   LangGraphSourceRuntime{SchemaVersion: "syncfuzz.target-runtime-lease.v1", Environment: "container", ContainerName: "syncfuzz-source", ContainerID: "source-id", ContainerImage: "syncfuzz-langgraph:test"},
		WorkspaceSnapshot:               LangGraphWorkspaceSnapshot{SourceWorkspace: "/profile/workspace", WorkspaceSHA256: strings.Repeat("a", 64), CheckpointStoreRelativePath: "langgraph-checkpoints", CheckpointStoreSHA256: strings.Repeat("b", 64), PassiveUnixSocketPath: "agent.sock", PassiveUnixSocketInode: 1},
		UnixSocketProbe:                 LangGraphUnixSocketProbe{SchemaVersion: LangGraphUnixSocketProbeSchema, SocketID: "socket:123", HolderPID: 7, HolderFD: 3, BindEffectID: "bind-effect", ListenEffectID: "listen-effect"},
		CheckpointCoordinates:           coordinates,
		AgentStateByCheckpoint:          map[string]StatePresence{"C0": StatePresenceAbsent, "C1": StatePresencePresent, "C2": StatePresencePresent},
	}
	prunedPlan := basePlan
	prunedPlan.RuntimeRoot = root + "/pruned/recovery-runtimes"
	prunedPlan.PassiveProbeMode = LangGraphPassiveProbePruned
	full := testFidelityExecution(basePlan, LangGraphPassiveProbeFull, EffectMultiplicitySingle, []uint64{100, 200, 300}, 4, 10, RecoverySetClassification{BeforeOutcome: "residual", AfterOutcome: "consistent", HeadOutcome: "consistent", Outcome: "residual"})
	pruned := testFidelityExecution(prunedPlan, LangGraphPassiveProbePruned, EffectMultiplicityUnknown, []uint64{25, 50, 75}, 1, 1, RecoverySetClassification{BeforeOutcome: "inconclusive", AfterOutcome: "inconclusive", HeadOutcome: "inconclusive", Outcome: "inconclusive"})
	return LangGraphProbeFidelityTrialInput{ArtifactRoot: root, FullPlan: basePlan, PrunedPlan: prunedPlan, FullExecution: full, PrunedExecution: pruned}
}

func testFidelityExecution(plan LangGraphForkPlan, mode LangGraphPassiveProbeMode, multiplicity EffectMultiplicity, durations []uint64, processes, fds int, classification RecoverySetClassification) ForkRecoverySetExecution {
	newObservation := func(checkpointID, runtimeID string, duration uint64) RecoveryObservation {
		state := plan.AgentStateByCheckpoint[checkpointID]
		return RecoveryObservation{
			SchemaVersion:         ExecutionSchemaVersion,
			QueryID:               "query:" + checkpointID,
			SeedID:                "state-seed:profile-1:C0..C1",
			Boundary:              BoundaryFork,
			CheckpointID:          checkpointID,
			RecordedPlanID:        plan.RecordedPlanID,
			PassiveObservationID:  plan.PassiveObservationID,
			MaterializationHeadID: plan.MaterializationHeadID,
			RetentionPolicy:       RetentionPolicyRetainRelevantOSState,
			RuntimeInstanceID:     runtimeID,
			AgentState:            state,
			OSState:               StatePresencePresent,
			OSStateOrigin:         StateOriginResidual,
			EffectMultiplicity:    multiplicity,
			PassiveProbe:          &PassiveProbeMetrics{Mode: mode, DurationNS: duration, ScannedProcesses: processes, ScannedFDs: fds},
			Evidence:              []string{"test observation"},
		}
	}
	return ForkRecoverySetExecution{
		SchemaVersion:  ExecutionSchemaVersion,
		RecoverySetID:  "historical-recovery-set:state-seed:profile-1:C0..C1",
		SeedID:         "state-seed:profile-1:C0..C1",
		FrontierID:     "C0..C1",
		RecordedPlanID: plan.RecordedPlanID,
		MaterializationHead: MaterializationHead{
			HeadID:              plan.MaterializationHeadID,
			ProfileRunID:        "profile-1",
			CheckpointID:        plan.MaterializationHeadCheckpointID,
			MonotonicNS:         300,
			RetainedResourceIDs: []string{"unix-socket:socket:123"},
		},
		RetentionPolicy: RetentionPolicyRetainRelevantOSState,
		Before:          newObservation("C0", "runtime-before", durations[0]),
		After:           newObservation("C1", "runtime-after", durations[1]),
		Head:            newObservation("C2", "runtime-head", durations[2]),
		Classification:  classification,
	}
}
