package synthesis

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/coverage"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestScheduleObjectivesPrioritizesUncoveredAtoms(t *testing.T) {
	ipc := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	handle := testObjective("handle.dup", profiling.StateFamilyHandle, "dup")
	ledger := []coverage.CoverageRecord{{
		SchemaVersion:      coverage.SchemaVersion,
		SeedID:             "seed-a",
		ObjectiveID:        ipc.ObjectiveID,
		Family:             profiling.StateFamilyIPC,
		Operation:          "listen",
		Lifetime:           "survive-tool-return",
		ResourceRelation:   "fixed-resource",
		Boundary:           "fork",
		CheckpointRelation: "before-after",
		Outcome:            coverage.OutcomeConsistent,
	}}
	schedule, err := ScheduleObjectives([]objective.StateObjective{ipc, handle}, ledger, 0)
	if err != nil {
		t.Fatalf("ScheduleObjectives returned error: %v", err)
	}
	if len(schedule.Selections) != 2 || schedule.Selections[0].ObjectiveID != handle.ObjectiveID || len(schedule.Selections[0].UncoveredEffects) != 1 {
		t.Fatalf("expected uncovered handle objective first, got %#v", schedule)
	}
}

func TestEvaluateProfileRequiresSchedulerCandidateAndLinkedEffects(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	request, err := NewGeneratorRequest(stateObjective, "maf-workflow-checkpoint", "maf-workflow", "scaffolds/maf", 0, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	candidate, err := NewCandidate(request, "test-generator", GeneratorResponse{SchemaVersion: SchemaVersion, Task: "Start the local integration service and leave it available for the next check."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	run := profileForCandidate(stateObjective, candidate)
	evaluation, err := EvaluateProfile(stateObjective, candidate, run)
	if err != nil {
		t.Fatalf("EvaluateProfile returned error: %v", err)
	}
	if !evaluation.EligibleForRetention || len(evaluation.MissingEffects) != 0 || len(evaluation.ValidatedFrontiers) != 1 {
		t.Fatalf("expected a retained candidate evaluation, got %#v", evaluation)
	}
	feedback, err := evaluation.FeedbackForObjective(stateObjective)
	if err != nil || len(feedback) != 1 || !feedback[0].Observed {
		t.Fatalf("expected canonical evaluation feedback, got %#v err=%v", feedback, err)
	}
	request, err = NewGeneratorRequest(stateObjective, "maf-workflow-checkpoint", "maf-workflow", "scaffolds/maf", 1, feedback)
	if err != nil || len(request.Feedback) != 1 || !request.Feedback[0].Observed {
		t.Fatalf("expected feedback-bearing generator request, got %#v err=%v", request, err)
	}
	invalidEvaluation := evaluation
	invalidEvaluation.Feedback[0].Observed = false
	if _, err := invalidEvaluation.FeedbackForObjective(stateObjective); err == nil {
		t.Fatal("expected inconsistent evaluation feedback to be rejected")
	}
	run.SynthesisCandidateID = "synthesis-candidate:wrong"
	if _, err := EvaluateProfile(stateObjective, candidate, run); err == nil {
		t.Fatal("expected candidate/profile identity mismatch")
	}
}

func TestCandidateIDCannotBeProvidedByGenerator(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	request, err := NewGeneratorRequest(stateObjective, "maf-workflow-checkpoint", "maf-workflow", "scaffolds/maf", 1, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	first, err := NewCandidate(request, "test-generator", GeneratorResponse{Task: "Run the local integration service."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	second, err := NewCandidate(request, "test-generator", GeneratorResponse{Task: "Run the local integration service."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	if first.CandidateID != second.CandidateID {
		t.Fatalf("expected scheduler-assigned deterministic candidate ID, got %q and %q", first.CandidateID, second.CandidateID)
	}
}

func TestStateFuzzAttemptValidatesOutcomeEvidence(t *testing.T) {
	eligible := true
	attempt := StateFuzzAttempt{
		SchemaVersion:        StateFuzzAttemptSchema,
		Attempt:              3,
		ArtifactRoot:         "runs/statefuzz/attempt-003",
		CandidateID:          "synthesis-candidate:test",
		ProfileRunID:         "target-profile:test",
		EligibleForRetention: &eligible,
		Status:               StateFuzzAttemptAccepted,
	}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("accepted StateFuzz attempt should validate: %v", err)
	}
	attempt.Status = StateFuzzAttemptRejectedSourceBaseline
	attempt.Reason = "multiple-listener-holders"
	if err := attempt.Validate(); err != nil {
		t.Fatalf("source-baseline rejection should validate: %v", err)
	}
	attempt.Status = StateFuzzAttemptRejectedEvaluation
	if err := attempt.Validate(); err == nil {
		t.Fatal("expected eligible evaluation rejection to fail validation")
	}
	attempt = StateFuzzAttempt{
		SchemaVersion: StateFuzzAttemptSchema,
		Attempt:       4,
		ArtifactRoot:  "runs/statefuzz/attempt-004",
		CandidateID:   "synthesis-candidate:test",
		Status:        StateFuzzAttemptExecutionFailed,
		Reason:        "candidate-profile-timeout",
	}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("early execution failure should validate without profile evidence: %v", err)
	}
}

func TestBuildStateFuzzBatchReportKeepsLegacyRejectionsAndMixedRoots(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	root := t.TempDir()

	writeAttempt := func(index int, retain bool) (SynthesisCandidate, objective.ProfileRun, objective.StateSeed) {
		t.Helper()
		request, err := NewGeneratorRequest(stateObjective, "target", "adapter", "scaffold.json", index, nil)
		if err != nil {
			t.Fatalf("NewGeneratorRequest: %v", err)
		}
		candidate, err := NewCandidate(request, "test-generator", GeneratorResponse{SchemaVersion: SchemaVersion, Task: "Start the local service and leave it available."})
		if err != nil {
			t.Fatalf("NewCandidate: %v", err)
		}
		attemptRoot := filepath.Join(root, fmt.Sprintf("attempt-%03d", index))
		if err := os.MkdirAll(attemptRoot, 0o755); err != nil {
			t.Fatalf("mkdir attempt root: %v", err)
		}
		if err := WriteCandidate(filepath.Join(attemptRoot, "candidate.json"), candidate); err != nil {
			t.Fatalf("WriteCandidate: %v", err)
		}
		run := profileForCandidate(stateObjective, candidate)
		if !retain {
			run.CheckpointMap.Intervals[0].EvidenceLinks = nil
		}
		if err := objective.WriteProfileRun(filepath.Join(attemptRoot, "profile-run.json"), run); err != nil {
			t.Fatalf("WriteProfileRun: %v", err)
		}
		evaluation, err := EvaluateProfile(stateObjective, candidate, run)
		if err != nil {
			t.Fatalf("EvaluateProfile: %v", err)
		}
		if err := WriteEvaluation(filepath.Join(attemptRoot, "evaluation.json"), evaluation); err != nil {
			t.Fatalf("WriteEvaluation: %v", err)
		}
		if !retain {
			return candidate, run, objective.StateSeed{}
		}
		seed, err := objective.PromoteStateSeed(stateObjective, run, "C0..C1")
		if err != nil {
			t.Fatalf("PromoteStateSeed: %v", err)
		}
		return candidate, run, *seed
	}
	writeRecovery := func(index int, seed objective.StateSeed) {
		t.Helper()
		head, err := recovery.MaterializationHeadFor(seed)
		if err != nil {
			t.Fatalf("MaterializationHeadFor: %v", err)
		}
		execution := recovery.ForkRecoverySetExecution{
			SchemaVersion:       recovery.ExecutionSchemaVersion,
			RecoverySetID:       "recovery-set:" + seed.SeedID,
			SeedID:              seed.SeedID,
			FrontierID:          seed.FrontierID,
			RecordedPlanID:      seed.RecordedPlanID,
			MaterializationHead: head,
			RetentionPolicy:     recovery.RetentionPolicyRetainRelevantOSState,
			Classification: recovery.RecoverySetClassification{
				BeforeOutcome: "residual",
				AfterOutcome:  "consistent",
				HeadOutcome:   "consistent",
				Outcome:       "residual",
			},
		}
		attemptRoot := filepath.Join(root, fmt.Sprintf("attempt-%03d", index))
		if err := objective.WriteStateSeed(filepath.Join(attemptRoot, "state-seed.json"), seed); err != nil {
			t.Fatalf("WriteStateSeed: %v", err)
		}
		if err := writeJSON(filepath.Join(attemptRoot, "recovery-set-execution.json"), execution); err != nil {
			t.Fatalf("write recovery execution: %v", err)
		}
	}

	// attempt-000 is a legacy retention rejection with no StateSeed.
	writeAttempt(0, false)
	_, _, acceptedSeed := writeAttempt(1, true)
	writeRecovery(1, acceptedSeed)
	// attempt-002 has top-level candidate/profile evidence from a later run but
	// retains a prior seed and recovery execution, so it must be invalidated.
	_, _, mixedSeed := writeAttempt(2, true)
	_, _, staleSeed := writeAttempt(3, true)
	writeRecovery(2, staleSeed)
	if mixedSeed.SynthesisCandidateID == staleSeed.SynthesisCandidateID {
		t.Fatal("expected independently generated candidates for mixed-root audit")
	}

	// attempt-004 models an early failure before an evaluation artifact exists.
	failureRequest, err := NewGeneratorRequest(stateObjective, "target", "adapter", "scaffold.json", 4, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest failure attempt: %v", err)
	}
	failureCandidate, err := NewCandidate(failureRequest, "test-generator", GeneratorResponse{SchemaVersion: SchemaVersion, Task: "Start the local service and leave it available."})
	if err != nil {
		t.Fatalf("NewCandidate failure attempt: %v", err)
	}
	failureRoot := filepath.Join(root, "attempt-004")
	if err := os.MkdirAll(failureRoot, 0o755); err != nil {
		t.Fatalf("mkdir failure root: %v", err)
	}
	if err := WriteCandidate(filepath.Join(failureRoot, "candidate.json"), failureCandidate); err != nil {
		t.Fatalf("WriteCandidate failure attempt: %v", err)
	}
	if err := WriteStateFuzzAttempt(filepath.Join(failureRoot, "statefuzz-attempt.json"), StateFuzzAttempt{
		SchemaVersion: StateFuzzAttemptSchema,
		Attempt:       4,
		ArtifactRoot:  failureRoot,
		CandidateID:   failureCandidate.CandidateID,
		Status:        StateFuzzAttemptExecutionFailed,
		Reason:        "candidate-profile-timeout",
	}); err != nil {
		t.Fatalf("WriteStateFuzzAttempt failure attempt: %v", err)
	}

	report, err := BuildStateFuzzBatchReport(stateObjective, root)
	if err != nil {
		t.Fatalf("BuildStateFuzzBatchReport: %v", err)
	}
	if report.AttemptCount != 5 || report.AcceptedCount != 1 || report.RejectedEvaluationCount != 1 || report.ExecutionFailureCount != 1 || report.InvalidArtifactRootCount != 2 || report.RecoveryOutcomeCounts["residual"] != 1 {
		t.Fatalf("unexpected StateFuzz batch report: %#v", report)
	}
	if report.Attempts[2].Status != StateFuzzBatchInvalidArtifactRoot || report.Attempts[2].Reason != "candidate-profile-seed-lineage-mismatch" {
		t.Fatalf("expected mixed root to remain invalid, got %#v", report.Attempts[2])
	}
}

func TestGenerateUsesStrictJSONResponseContract(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	scaffoldPath := filepath.Join(t.TempDir(), "scaffold.json")
	if err := os.WriteFile(scaffoldPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write scaffold: %v", err)
	}
	request, err := NewGeneratorRequest(stateObjective, "maf-workflow-checkpoint", "maf-workflow", scaffoldPath, 0, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	candidate, err := Generate(context.Background(), `printf '%s' '{"schema_version":"syncfuzz.synthesis.v1","task":"Start the local service for the next integration check."}'`, request, "test-generator")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if candidate.Task == "" || candidate.CandidateID == "" {
		t.Fatalf("unexpected generated candidate: %#v", candidate)
	}
	if _, err := Generate(context.Background(), `printf '%s' '{"task":"one"}{"task":"two"}'`, request, "test-generator"); err == nil {
		t.Fatal("expected multiple JSON generator response to be rejected")
	}
}

func TestBindMAFNativeFrontierRequiresSameInitialRuntime(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	request, err := NewGeneratorRequest(stateObjective, "maf-workflow-checkpoint", "maf-workflow", "scaffolds/maf", 0, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	candidate, err := NewCandidate(request, "test-generator", GeneratorResponse{Task: "Start the local integration service."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	run := profileForCandidate(stateObjective, candidate)
	run.NativeCheckpointRunID = "maf-initial-1"
	manifest := MAFNativeCheckpointManifest{
		SchemaVersion:            MAFNativeCheckpointManifestSchema,
		TaskID:                   "maf-workflow-checkpoint-continuity",
		InitialRuntimeInstanceID: "maf-initial-1",
		NativeCheckpoints: []MAFNativeCheckpoint{
			{CheckpointID: "native-before", Coordinate: "before-effect", MessageTargets: []string{"v2-start"}},
			{CheckpointID: "native-after", Coordinate: "after-effect", MessageTargets: []string{"v2-plant"}},
		},
	}
	binding, plan, err := BindMAFNativeFrontier(stateObjective, candidate, run, "C0..C1", "prepared/maf-workflow-fork-manifest.json", manifest, MAFBindingConfig{PythonCommand: "python3", RunnerPath: "targets/maf_workflow_checkpoint/run_target.py", PreparedWorkspace: "prepared", RuntimeRoot: "forks"})
	if err != nil {
		t.Fatalf("BindMAFNativeFrontier returned error: %v", err)
	}
	if binding.BeforeNativeCheckpointID != "native-before" || binding.AfterNativeCheckpointID != "native-after" || plan.CheckpointBindings["C0"] != "native-before" || plan.CheckpointBindings["C1"] != "native-after" {
		t.Fatalf("unexpected native binding: %#v %#v", binding, plan)
	}
	manifest.InitialRuntimeInstanceID = "other-initial"
	if _, _, err := BindMAFNativeFrontier(stateObjective, candidate, run, "C0..C1", "prepared/maf-workflow-fork-manifest.json", manifest, MAFBindingConfig{PythonCommand: "python3", RunnerPath: "targets/maf_workflow_checkpoint/run_target.py", PreparedWorkspace: "prepared", RuntimeRoot: "forks"}); err == nil {
		t.Fatal("expected unrelated MAF native manifest to be rejected")
	}
}

func TestLangGraphSynthesisTargetRunRequiresExplicitNetworkAndUsesCandidatePrompt(t *testing.T) {
	stateObjective := testObjective("ipc.listen", profiling.StateFamilyIPC, "listen")
	request, err := NewGeneratorRequest(stateObjective, LangGraphSynthesisTargetID, LangGraphSynthesisAdapterID, "scaffolds/langgraph", 0, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	candidate, err := NewCandidate(request, "test-generator", GeneratorResponse{Task: "Start the local development listener needed by the integration check and leave it available."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	if _, err := NewLangGraphSynthesisTargetRunOptions(stateObjective, candidate, LangGraphExecutionConfig{OutDir: t.TempDir()}); err == nil {
		t.Fatal("expected network opt-in to be required")
	}
	opts, err := NewLangGraphSynthesisTargetRunOptions(stateObjective, candidate, LangGraphExecutionConfig{
		OutDir:              t.TempDir(),
		AllowNetwork:        true,
		RetainRuntime:       true,
		ProviderEnvironment: map[string]string{"LANGCHAIN_MODEL": "openai:test", "OPENAI_API_KEY": "not-written"},
	})
	if err != nil {
		t.Fatalf("NewLangGraphSynthesisTargetRunOptions returned error: %v", err)
	}
	if opts.AdapterID != LangGraphSynthesisAdapterID || opts.TargetID != LangGraphSynthesisTargetID || opts.TaskID != LangGraphCandidateTaskID || opts.SynthesisCandidateID != candidate.CandidateID || opts.Prompt != candidate.Task || opts.EnvKind != "container" || !opts.EnableProcessProfiling || !opts.EnableResourceProfiling || !opts.AllowNetwork || !opts.RetainEnvironment {
		t.Fatalf("unexpected LangGraph candidate options: %#v", opts)
	}
	if opts.CommandEnvironment["OPENAI_API_KEY"] != "not-written" || !strings.Contains(opts.Command, "/opt/syncfuzz-langgraph/run_target.py") {
		t.Fatalf("expected ephemeral provider environment and image-owned runner: %#v", opts)
	}
}

func TestValidateLangGraphCandidateProfilingEvidenceReportsTimeout(t *testing.T) {
	err := validateLangGraphCandidateProfilingEvidence(&target.TargetRunResult{
		RunID: "timed-out-run",
		CommandResult: target.TargetCommandResult{
			TimedOut:   true,
			DurationMs: 120000,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out after 120000ms") || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("expected a specific incomplete-profile timeout error, got %v", err)
	}
}

func TestReadLangGraphNativeCheckpointManifestRequiresDurableExactIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "langgraph-native-checkpoints.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version":"syncfuzz.langgraph-native-checkpoint-manifest.v1",
  "initial_runtime_instance_id":"langgraph-native-runtime:run-1",
  "thread_id":"run-1",
  "checkpoint_backend":"disk",
  "durable":true,
  "checkpoint_dir":"/workspace/langgraph-checkpoints",
  "native_checkpoints":[{
    "checkpoint_id":"checkpoint-1",
    "history_index":0,
    "message_count":3,
    "next":[],
    "durable_tool_lifecycle":{
      "tool_calls":[{"tool_call_id":"call-1","tool_name":"shell"}],
      "tool_result_ids":["call-1"]
    }
  }]
}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := ReadLangGraphNativeCheckpointManifest(path)
	if err != nil {
		t.Fatalf("ReadLangGraphNativeCheckpointManifest returned error: %v", err)
	}
	if manifest.InitialRuntimeInstanceID != "langgraph-native-runtime:run-1" || len(manifest.NativeCheckpoints) != 1 || manifest.NativeCheckpoints[0].DurableToolLifecycle == nil || manifest.NativeCheckpoints[0].DurableToolLifecycle.ToolCalls[0].ToolName != "shell" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	manifest.Durable = false
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected non-durable manifest rejection")
	}
	manifest.Durable = true
	manifest.NativeCheckpoints[0].DurableToolLifecycle.ToolResultIDs = []string{"call-1", "call-1"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected duplicate durable tool result rejection")
	}
}

func TestLangGraphNativeCheckpointManifestPathUsesHostWorkspace(t *testing.T) {
	hostWorkspace := t.TempDir()
	path, err := langGraphNativeCheckpointManifestPath(&target.TargetRunResult{
		Workspace:     "/workspace",
		HostWorkspace: hostWorkspace,
	})
	if err != nil {
		t.Fatalf("resolve host manifest path: %v", err)
	}
	if want := filepath.Join(hostWorkspace, LangGraphNativeCheckpointManifestArtifact); path != want {
		t.Fatalf("manifest path = %q, want %q", path, want)
	}
	if _, err := langGraphNativeCheckpointManifestPath(&target.TargetRunResult{Workspace: "/workspace"}); err == nil {
		t.Fatal("expected missing host workspace to be rejected")
	}
}

func TestInferLangGraphNativeCheckpointManifestPathUsesRecordedPlanDirectory(t *testing.T) {
	artifactDir := t.TempDir()
	path, err := InferLangGraphNativeCheckpointManifestPath(objective.ProfileRun{
		RecordedPlanArtifact: filepath.Join(artifactDir, "target-task.json"),
	})
	if err != nil {
		t.Fatalf("InferLangGraphNativeCheckpointManifestPath returned error: %v", err)
	}
	if want := filepath.Join(artifactDir, LangGraphNativeCheckpointManifestArtifact); path != want {
		t.Fatalf("manifest path = %q, want %q", path, want)
	}
	if _, err := InferLangGraphNativeCheckpointManifestPath(objective.ProfileRun{}); err == nil {
		t.Fatal("expected profile run without a recorded target plan to be rejected")
	}
}

func TestBindLangGraphNativeFrontierUsesEffectBracketingNativeCheckpoints(t *testing.T) {
	stateObjective := objective.StateObjective{
		SchemaVersion: objective.SchemaVersion,
		ObjectiveID:   "ipc.unix-listener.survival",
		Effects: []objective.EffectAtom{
			{Family: profiling.StateFamilyIPC, Operation: "bind"},
			{Family: profiling.StateFamilyIPC, Operation: "listen"},
		},
		Lifetime:         "survive-tool-return",
		ResourceRelation: "fixed-path-served-by-descendant",
		Persistence:      "across-checkpoint",
	}
	request, err := NewGeneratorRequest(stateObjective, LangGraphSynthesisTargetID, LangGraphSynthesisAdapterID, "scaffolds/langgraph", 0, nil)
	if err != nil {
		t.Fatalf("NewGeneratorRequest returned error: %v", err)
	}
	candidate, err := NewCandidate(request, "test-generator", GeneratorResponse{Task: "Start the local socket service needed for the integration check."})
	if err != nil {
		t.Fatalf("NewCandidate returned error: %v", err)
	}
	run := profileForCandidate(stateObjective, candidate)
	run.NativeCheckpointRunID = "langgraph-native-runtime:run-1"
	run.RetainedRuntime = &objective.RetainedRuntime{
		SchemaVersion:  objective.RetainedRuntimeSchema,
		Environment:    "container",
		ContainerName:  "syncfuzz-profile-source",
		ContainerID:    "container-id",
		ContainerImage: "syncfuzz-langgraph:dev",
	}
	frontier := &run.CheckpointMap.Intervals[0]
	frontier.StartMonotonicNS = 100
	frontier.EndMonotonicNS = 200
	frontier.Effects[0].MonotonicNS = 140
	frontier.Effects[0].Resource = profiling.ResourceRef{Family: profiling.StateFamilyIPC, SocketID: "socket:123", HolderPID: 7, FD: 3}
	frontier.EvidenceLinks[0] = profiling.EvidenceLink{LinkID: "bind-resource", EffectID: "effect-1", ResourceID: "unix-socket:socket:123", Relation: profiling.EvidenceLinkExactSocketID}
	frontier.Effects = append(frontier.Effects, profiling.NormalizedEffect{
		EffectID: "effect-2", MonotonicNS: 150, Family: profiling.StateFamilyIPC, Operation: "listen", Resource: profiling.ResourceRef{Family: profiling.StateFamilyIPC, SocketID: "socket:123", HolderPID: 7, FD: 3}, PersistencePotential: true,
	})
	frontier.EvidenceLinks = append(frontier.EvidenceLinks, profiling.EvidenceLink{
		LinkID: "listen-resource", EffectID: "effect-2", ResourceID: "unix-socket:socket:123", Relation: profiling.EvidenceLinkExactSocketID,
	})
	listenerResources := []profiling.PersistentResource{
		{Observed: true, Resource: profiling.ResourceRef{ResourceID: "unix-socket:socket:123", Family: profiling.StateFamilyIPC, Kind: "unix-listener", SocketID: "socket:123"}},
		{Observed: true, Resource: profiling.ResourceRef{ResourceID: "container-fd:7:3:socket:123", Family: profiling.StateFamilyHandle, Kind: "socket", SocketID: "socket:123", HolderPID: 7, FD: 3}},
	}
	run.CheckpointSummaries[1].Resources = append([]profiling.PersistentResource{}, listenerResources...)
	run.CheckpointSummaries[2].Resources = append([]profiling.PersistentResource{}, listenerResources...)
	manifest := LangGraphNativeCheckpointManifest{
		SchemaVersion:            LangGraphNativeCheckpointManifestSchema,
		InitialRuntimeInstanceID: "langgraph-native-runtime:run-1",
		ThreadID:                 "run-1",
		CheckpointBackend:        "disk",
		Durable:                  true,
		ClockDomain:              "CLOCK_MONOTONIC",
		CheckpointDir:            "/workspace/langgraph-checkpoints",
		NativeCheckpoints: []LangGraphNativeCheckpoint{
			{CheckpointID: "too-early", HistoryIndex: 3, MessageCount: 1, PersistedMonotonicNS: 90, DurableToolLifecycle: &LangGraphDurableToolLifecycle{}},
			{CheckpointID: "before-native", HistoryIndex: 2, MessageCount: 2, PersistedMonotonicNS: 130, DurableToolLifecycle: &LangGraphDurableToolLifecycle{ToolCalls: []LangGraphDurableToolCall{{ToolCallID: "call-1", ToolName: "shell"}}}},
			{CheckpointID: "inside-effect", HistoryIndex: 1, MessageCount: 3, PersistedMonotonicNS: 145, DurableToolLifecycle: &LangGraphDurableToolLifecycle{ToolCalls: []LangGraphDurableToolCall{{ToolCallID: "call-1", ToolName: "shell"}}}},
			{CheckpointID: "after-native", HistoryIndex: 0, MessageCount: 4, PersistedMonotonicNS: 170, DurableToolLifecycle: &LangGraphDurableToolLifecycle{ToolCalls: []LangGraphDurableToolCall{{ToolCallID: "call-1", ToolName: "shell"}}, ToolResultIDs: []string{"call-1"}}},
			{CheckpointID: "head-native", HistoryIndex: 0, MessageCount: 5, PersistedMonotonicNS: 180, DurableToolLifecycle: &LangGraphDurableToolLifecycle{ToolCalls: []LangGraphDurableToolCall{{ToolCallID: "call-1", ToolName: "shell"}}, ToolResultIDs: []string{"call-1"}}},
		},
	}
	manifestPath := filepath.Join(t.TempDir(), "langgraph-native-checkpoints.json")
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode LangGraph native manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, manifestRaw, 0o644); err != nil {
		t.Fatalf("write LangGraph native manifest: %v", err)
	}
	lifecycle := &LangGraphLifecycleArtifact{
		SchemaVersion: LangGraphLifecycleArtifactSchema,
		ThreadID:      "run-1",
		ClockDomain:   "CLOCK_MONOTONIC",
		Events: []LangGraphLifecycleEvent{
			{Index: 0, Event: "shell_command_started", MonotonicNS: 135, ToolCallID: "call-1", ShellSessionID: "shell-1", CommandSHA256: strings.Repeat("a", 64)},
			{Index: 1, Event: "shell_command_finished", MonotonicNS: 165, ToolCallID: "call-1"},
		},
	}
	binding, err := BindLangGraphNativeFrontierWithLifecycle(stateObjective, candidate, run, "C0..C1", manifestPath, manifest, lifecycle)
	if err != nil {
		t.Fatalf("BindLangGraphNativeFrontier returned error: %v", err)
	}
	if binding.BeforeNativeCheckpointID != "before-native" || binding.AfterNativeCheckpointID != "after-native" || binding.FirstEffectMonotonicNS != 140 || binding.LastEffectMonotonicNS != 150 {
		t.Fatalf("unexpected LangGraph native frontier binding: %#v", binding)
	}
	if binding.BeforeNativeCoordinate.SourceCheckpointID != "before-native" || binding.BeforeNativeCoordinate.HistoryIndex != 2 || binding.BeforeNativeCoordinate.MessageCount != 2 || binding.AfterNativeCoordinate.SourceCheckpointID != "after-native" || binding.AfterNativeCoordinate.HistoryIndex != 0 || binding.AfterNativeCoordinate.MessageCount != 4 {
		t.Fatalf("binding did not preserve fresh-runtime native coordinates: %#v", binding)
	}
	if binding.BeforeNativeToolLifecycle == nil || binding.AfterNativeToolLifecycle == nil || len(binding.BeforeNativeToolLifecycle.ToolResultIDs) != 0 || len(binding.AfterNativeToolLifecycle.ToolResultIDs) != 1 {
		t.Fatalf("binding did not preserve durable tool lifecycle evidence: %#v", binding)
	}
	if binding.ToolEffectProvenance == nil || binding.ToolEffectProvenance.ToolCallID != "call-1" || binding.ToolEffectProvenance.FirstEffectMonotonicNS != 140 || binding.ToolEffectProvenance.LastEffectMonotonicNS != 150 {
		t.Fatalf("binding did not preserve exact tool-effect provenance: %#v", binding.ToolEffectProvenance)
	}
	withoutResult := manifest
	withoutResult.NativeCheckpoints = append([]LangGraphNativeCheckpoint(nil), manifest.NativeCheckpoints...)
	afterLifecycle := manifest.NativeCheckpoints[3].DurableToolLifecycle.Clone()
	afterLifecycle.ToolResultIDs = nil
	withoutResult.NativeCheckpoints[3].DurableToolLifecycle = &afterLifecycle
	withoutResultBinding, err := BindLangGraphNativeFrontierWithLifecycle(stateObjective, candidate, run, "C0..C1", manifestPath, withoutResult, lifecycle)
	if err != nil {
		t.Fatalf("BindLangGraphNativeFrontierWithLifecycle without durable result returned error: %v", err)
	}
	if withoutResultBinding.ToolEffectProvenance != nil {
		t.Fatalf("binding without an after-checkpoint durable result must keep provenance unknown: %#v", withoutResultBinding.ToolEffectProvenance)
	}
	sourceArtifactDir := t.TempDir()
	sourceWorkspace := filepath.Join(sourceArtifactDir, "workspace")
	if err := os.MkdirAll(filepath.Join(sourceWorkspace, "langgraph-checkpoints"), 0o755); err != nil {
		t.Fatalf("create source checkpoint store: %v", err)
	}
	for _, artifact := range []string{"target-prompt.txt", "target-task.json", "langgraph-checkpoints/storage.pkl", "langgraph-checkpoints/writes.pkl", "langgraph-checkpoints/blobs.pkl"} {
		if err := os.WriteFile(filepath.Join(sourceWorkspace, artifact), []byte(artifact+"\n"), 0o644); err != nil {
			t.Fatalf("write source workspace artifact %s: %v", artifact, err)
		}
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(sourceWorkspace, "agent.sock"), Net: "unix"})
	if err != nil {
		t.Skipf("Unix sockets are unavailable in this test sandbox: %v", err)
	}
	defer listener.Close()
	if err := os.WriteFile(filepath.Join(sourceArtifactDir, "target-task.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write source target task: %v", err)
	}
	run.RecordedPlanArtifact = filepath.Join(sourceArtifactDir, "target-task.json")
	forkPlan, err := PrepareLangGraphForkPlan(stateObjective, candidate, run, binding, LangGraphForkPlanConfig{
		Model:                 "openai:gpt-4.1-mini",
		ContainerImage:        "syncfuzz-langgraph:dev",
		RuntimeRoot:           "runs/langgraph-forks",
		PassiveUnixSocketPath: "agent.sock",
	})
	if err != nil {
		t.Fatalf("PrepareLangGraphForkPlan returned error: %v", err)
	}
	if err := forkPlan.ValidateFor(recovery.RecordedPlan{
		SchemaVersion:        recovery.SchemaVersion,
		RecordedPlanID:       run.RecordedPlanID,
		AdapterID:            recovery.LangGraphForkAdapterID,
		TargetID:             run.TargetID,
		ExecutionArtifact:    "runs/langgraph-fork-plan.json",
		PassiveObservationID: "unix-socket-listener-holder-v1:agent.sock",
	}); err != nil {
		t.Fatalf("LangGraph fork plan validation failed: %v", err)
	}
	if forkPlan.MaterializationHeadID != "materialization-head:profile-1:C2" || forkPlan.MaterializationHeadCheckpointID != "C2" || forkPlan.CheckpointCoordinates["C2"].SourceCheckpointID != "head-native" || forkPlan.AgentStateByCheckpoint["C2"] != recovery.StatePresencePresent {
		t.Fatalf("LangGraph fork plan did not preserve a distinct materialization head: %#v", forkPlan)
	}
	if forkPlan.SourceThreadID != "run-1" || forkPlan.WorkspaceSnapshot.SourceWorkspace != sourceWorkspace || forkPlan.WorkspaceSnapshot.PassiveUnixSocketInode == 0 {
		t.Fatalf("LangGraph fork plan did not preserve a source workspace snapshot: %#v", forkPlan.WorkspaceSnapshot)
	}
	if len(forkPlan.ToolLifecycleByCheckpoint) != 3 || len(forkPlan.ToolLifecycleByCheckpoint["C0"].ToolResultIDs) != 0 || len(forkPlan.ToolLifecycleByCheckpoint["C1"].ToolResultIDs) != 1 || len(forkPlan.ToolLifecycleByCheckpoint["C2"].ToolCalls) != 1 {
		t.Fatalf("LangGraph fork plan did not preserve durable tool lifecycle evidence: %#v", forkPlan.ToolLifecycleByCheckpoint)
	}
	if forkPlan.ToolEffectProvenance == nil || forkPlan.ToolEffectProvenance.ToolCallID != "call-1" {
		t.Fatalf("LangGraph fork plan did not preserve tool-effect provenance: %#v", forkPlan.ToolEffectProvenance)
	}
	headCoordinate, ok := forkPlan.CheckpointCoordinates["C2"]
	if !ok || headCoordinate.Next == nil {
		t.Fatalf("terminal LangGraph coordinate must retain an empty next array: %#v", headCoordinate)
	}
	encodedHeadCoordinate, err := json.Marshal(headCoordinate)
	if err != nil {
		t.Fatalf("encode terminal LangGraph coordinate: %v", err)
	}
	var rawHeadCoordinate map[string]any
	if err := json.Unmarshal(encodedHeadCoordinate, &rawHeadCoordinate); err != nil {
		t.Fatalf("decode terminal LangGraph coordinate: %v", err)
	}
	if next, ok := rawHeadCoordinate["next"].([]any); !ok || len(next) != 0 {
		t.Fatalf("terminal LangGraph coordinate serialized next incorrectly: %#v", rawHeadCoordinate)
	}
	legacyPlan := forkPlan
	legacyPlan.MaterializationHeadID = ""
	legacyPlan.MaterializationHeadCheckpointID = ""
	legacyPlan.CheckpointCoordinates = map[string]recovery.LangGraphNativeCheckpointCoordinate{
		"C0": forkPlan.CheckpointCoordinates["C0"],
		"C1": forkPlan.CheckpointCoordinates["C1"],
	}
	legacyPlan.AgentStateByCheckpoint = map[string]recovery.StatePresence{
		"C0": forkPlan.AgentStateByCheckpoint["C0"],
		"C1": forkPlan.AgentStateByCheckpoint["C1"],
	}
	legacyPlan.ToolLifecycleByCheckpoint = nil
	legacyPlan.ToolEffectProvenance = nil
	if err := legacyPlan.ValidateFor(recovery.RecordedPlan{
		SchemaVersion:        recovery.SchemaVersion,
		RecordedPlanID:       run.RecordedPlanID,
		AdapterID:            recovery.LangGraphForkAdapterID,
		TargetID:             run.TargetID,
		ExecutionArtifact:    "runs/legacy-langgraph-fork-plan.json",
		PassiveObservationID: "unix-socket-listener-holder-v1:agent.sock",
	}); err != nil {
		t.Fatalf("legacy before/after LangGraph plan no longer validates: %v", err)
	}
	manifest.ClockDomain = ""
	if _, err := BindLangGraphNativeFrontier(stateObjective, candidate, run, "C0..C1", "runs/run-1/langgraph-native-checkpoints.json", manifest); err == nil {
		t.Fatal("expected non-monotonic native manifest clock domain to be rejected")
	}
	manifest.ClockDomain = "CLOCK_MONOTONIC"
	manifest.NativeCheckpoints[1].PersistedMonotonicNS = 0
	if _, err := BindLangGraphNativeFrontier(stateObjective, candidate, run, "C0..C1", "runs/run-1/langgraph-native-checkpoints.json", manifest); err == nil {
		t.Fatal("expected native binding without a timestamped before checkpoint to be rejected")
	}
}

func TestLangGraphUnixSocketProbeRejectsMultipleLiveLinkedEndpoints(t *testing.T) {
	endpoint := func(socketID string, pid uint32) []profiling.PersistentResource {
		return []profiling.PersistentResource{
			{Observed: true, Resource: profiling.ResourceRef{
				ResourceID: "unix-socket:" + socketID,
				Family:     profiling.StateFamilyIPC,
				Kind:       "unix-listener",
				SocketID:   socketID,
			}},
			{Observed: true, Resource: profiling.ResourceRef{
				ResourceID: "container-fd:" + socketID,
				Family:     profiling.StateFamilyHandle,
				Kind:       "socket",
				SocketID:   socketID,
				HolderPID:  pid,
				FD:         3,
			}},
		}
	}
	resources := append(endpoint("socket:one", 60), endpoint("socket:two", 107)...)
	newEffect := func(id, operation, socketID string) profiling.NormalizedEffect {
		return profiling.NormalizedEffect{
			EffectID:  id,
			Family:    profiling.StateFamilyIPC,
			Operation: operation,
			Resource: profiling.ResourceRef{
				SocketID: socketID,
			},
		}
	}
	run := objective.ProfileRun{
		CheckpointSummaries: []profiling.CheckpointStateSummary{{
			CheckpointID: "C2",
			Resources:    resources,
		}},
		CheckpointMap: profiling.CheckpointEffectMap{Intervals: []profiling.CheckpointInterval{{
			FrontierID: "C0..C1",
			Effects: []profiling.NormalizedEffect{
				newEffect("bind-one", "bind", "socket:one"),
				newEffect("listen-one", "listen", "socket:one"),
				newEffect("bind-two", "bind", "socket:two"),
				newEffect("listen-two", "listen", "socket:two"),
			},
			EvidenceLinks: []profiling.EvidenceLink{
				{EffectID: "bind-one", ResourceID: "unix-socket:socket:one", Relation: profiling.EvidenceLinkExactSocketID},
				{EffectID: "listen-one", ResourceID: "unix-socket:socket:one", Relation: profiling.EvidenceLinkExactSocketID},
				{EffectID: "bind-two", ResourceID: "unix-socket:socket:two", Relation: profiling.EvidenceLinkExactSocketID},
				{EffectID: "listen-two", ResourceID: "unix-socket:socket:two", Relation: profiling.EvidenceLinkExactSocketID},
			},
		}}},
	}
	_, err := langGraphUnixSocketProbe(run, LangGraphNativeFrontierBinding{FrontierID: "C0..C1"}, "C2")
	if err == nil {
		t.Fatal("expected multiple live linked Unix endpoints to be rejected")
	}
	if !strings.Contains(err.Error(), "multiple linked Unix listener endpoints") || !strings.Contains(err.Error(), "socket:one,socket:two") {
		t.Fatalf("unexpected multiple-listener error: %v", err)
	}
}

func testObjective(id string, family profiling.StateFamily, operation string) objective.StateObjective {
	return objective.StateObjective{
		SchemaVersion:    objective.SchemaVersion,
		ObjectiveID:      id,
		Effects:          []objective.EffectAtom{{Family: family, Operation: operation}},
		Lifetime:         "survive-tool-return",
		ResourceRelation: "fixed-resource",
		Persistence:      "across-checkpoint",
	}
}

func profileForCandidate(stateObjective objective.StateObjective, candidate SynthesisCandidate) objective.ProfileRun {
	atom := stateObjective.Effects[0]
	effectID := "effect-1"
	return objective.ProfileRun{
		SchemaVersion:        objective.SchemaVersion,
		ProfileRunID:         "profile-1",
		Kind:                 objective.ProfileRunKindSynthesisCandidate,
		SynthesisCandidateID: candidate.CandidateID,
		ObjectiveID:          stateObjective.ObjectiveID,
		TargetID:             candidate.TargetID,
		AdapterID:            candidate.AdapterID,
		RecordedPlanID:       "recorded-plan:profile-1",
		RecordedPlanArtifact: "recorded-plan.json",
		CheckpointCatalog: profiling.CheckpointCatalog{
			SchemaVersion: profiling.SchemaVersion,
			RunID:         "run-1",
			Checkpoints: []profiling.Checkpoint{
				{CheckpointID: "C0", MonotonicNS: 100},
				{CheckpointID: "C1", MonotonicNS: 200},
				{CheckpointID: "C2", MonotonicNS: 300},
			},
		},
		CheckpointSummaries: []profiling.CheckpointStateSummary{
			{CheckpointID: "C0", MonotonicNS: 100},
			{CheckpointID: "C1", MonotonicNS: 200, Resources: []profiling.PersistentResource{{Observed: true, Resource: profiling.ResourceRef{ResourceID: "resource-1", Family: atom.Family}}}},
			{CheckpointID: "C2", MonotonicNS: 300, Resources: []profiling.PersistentResource{{Observed: true, Resource: profiling.ResourceRef{ResourceID: "resource-1", Family: atom.Family}}}},
		},
		CheckpointMap: profiling.CheckpointEffectMap{
			SchemaVersion: profiling.SchemaVersion,
			RunID:         "run-1",
			Intervals: []profiling.CheckpointInterval{{
				FrontierID:         "C0..C1",
				BeforeCheckpointID: "C0",
				AfterCheckpointID:  "C1",
				StartMonotonicNS:   100,
				EndMonotonicNS:     200,
				Effects: []profiling.NormalizedEffect{{
					EffectID: effectID, Family: atom.Family, Operation: atom.Operation, PersistencePotential: true,
				}},
				PersistentDelta: profiling.StateDelta{Added: []profiling.PersistentResource{{
					Observed: true, Resource: profiling.ResourceRef{ResourceID: "resource-1", Family: atom.Family},
				}}},
				EvidenceLinks: []profiling.EvidenceLink{{LinkID: "effect-resource", EffectID: effectID, ResourceID: "resource-1", Relation: profiling.EvidenceLinkExactPath}},
				IsFrontier:    true,
			}},
		},
	}
}
