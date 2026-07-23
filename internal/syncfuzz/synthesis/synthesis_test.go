package synthesis

import (
	"context"
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
		ProviderEnvironment: map[string]string{"LANGCHAIN_MODEL": "openai:test", "OPENAI_API_KEY": "not-written"},
	})
	if err != nil {
		t.Fatalf("NewLangGraphSynthesisTargetRunOptions returned error: %v", err)
	}
	if opts.AdapterID != LangGraphSynthesisAdapterID || opts.TargetID != LangGraphSynthesisTargetID || opts.TaskID != LangGraphCandidateTaskID || opts.SynthesisCandidateID != candidate.CandidateID || opts.Prompt != candidate.Task || opts.EnvKind != "container" || !opts.EnableProcessProfiling || !opts.EnableResourceProfiling || !opts.AllowNetwork {
		t.Fatalf("unexpected LangGraph candidate options: %#v", opts)
	}
	if opts.CommandEnvironment["OPENAI_API_KEY"] != "not-written" || !strings.Contains(opts.Command, "/opt/syncfuzz-langgraph/run_target.py") {
		t.Fatalf("expected ephemeral provider environment and image-owned runner: %#v", opts)
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
  "native_checkpoints":[{"checkpoint_id":"checkpoint-1","history_index":0,"message_count":3,"next":[]}]
}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := ReadLangGraphNativeCheckpointManifest(path)
	if err != nil {
		t.Fatalf("ReadLangGraphNativeCheckpointManifest returned error: %v", err)
	}
	if manifest.InitialRuntimeInstanceID != "langgraph-native-runtime:run-1" || len(manifest.NativeCheckpoints) != 1 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	manifest.Durable = false
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected non-durable manifest rejection")
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
	frontier := &run.CheckpointMap.Intervals[0]
	frontier.StartMonotonicNS = 100
	frontier.EndMonotonicNS = 200
	frontier.Effects[0].MonotonicNS = 140
	frontier.Effects = append(frontier.Effects, profiling.NormalizedEffect{
		EffectID: "effect-2", MonotonicNS: 150, Family: profiling.StateFamilyIPC, Operation: "listen", PersistencePotential: true,
	})
	frontier.EvidenceLinks = append(frontier.EvidenceLinks, profiling.EvidenceLink{
		LinkID: "listen-resource", EffectID: "effect-2", ResourceID: "resource-1", Relation: profiling.EvidenceLinkExactPath,
	})
	manifest := LangGraphNativeCheckpointManifest{
		SchemaVersion:            LangGraphNativeCheckpointManifestSchema,
		InitialRuntimeInstanceID: "langgraph-native-runtime:run-1",
		ThreadID:                 "run-1",
		CheckpointBackend:        "disk",
		Durable:                  true,
		ClockDomain:              "CLOCK_MONOTONIC",
		CheckpointDir:            "/workspace/langgraph-checkpoints",
		NativeCheckpoints: []LangGraphNativeCheckpoint{
			{CheckpointID: "too-early", HistoryIndex: 3, MessageCount: 1, PersistedMonotonicNS: 90},
			{CheckpointID: "before-native", HistoryIndex: 2, MessageCount: 2, PersistedMonotonicNS: 130},
			{CheckpointID: "inside-effect", HistoryIndex: 1, MessageCount: 3, PersistedMonotonicNS: 145},
			{CheckpointID: "after-native", HistoryIndex: 0, MessageCount: 4, PersistedMonotonicNS: 170},
		},
	}
	binding, err := BindLangGraphNativeFrontier(stateObjective, candidate, run, "C0..C1", "runs/run-1/langgraph-native-checkpoints.json", manifest)
	if err != nil {
		t.Fatalf("BindLangGraphNativeFrontier returned error: %v", err)
	}
	if binding.BeforeNativeCheckpointID != "before-native" || binding.AfterNativeCheckpointID != "after-native" || binding.FirstEffectMonotonicNS != 140 || binding.LastEffectMonotonicNS != 150 {
		t.Fatalf("unexpected LangGraph native frontier binding: %#v", binding)
	}
	if binding.BeforeNativeCoordinate.SourceCheckpointID != "before-native" || binding.BeforeNativeCoordinate.HistoryIndex != 2 || binding.BeforeNativeCoordinate.MessageCount != 2 || binding.AfterNativeCoordinate.SourceCheckpointID != "after-native" || binding.AfterNativeCoordinate.HistoryIndex != 0 || binding.AfterNativeCoordinate.MessageCount != 4 {
		t.Fatalf("binding did not preserve fresh-runtime native coordinates: %#v", binding)
	}
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
		PassiveObservationID: "unix-socket-metadata:agent.sock",
	}); err != nil {
		t.Fatalf("LangGraph fork plan validation failed: %v", err)
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
		CheckpointMap: profiling.CheckpointEffectMap{
			SchemaVersion: profiling.SchemaVersion,
			RunID:         "run-1",
			Intervals: []profiling.CheckpointInterval{{
				FrontierID:         "C0..C1",
				BeforeCheckpointID: "C0",
				AfterCheckpointID:  "C1",
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
