package recovery

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

func TestCausalEffectEvidenceForRecordedLangGraphPlan(t *testing.T) {
	seed := testSeed()
	seed.AdapterID = LangGraphForkAdapterID
	plan := RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       seed.RecordedPlanID,
		AdapterID:            seed.AdapterID,
		TargetID:             seed.TargetID,
		ExecutionArtifact:    filepath.Join(t.TempDir(), "langgraph-fork-plan.json"),
		PassiveObservationID: "unix-socket-metadata:agent.sock",
	}
	forkPlan := validLangGraphPlanForCausalEvidence(seed, plan)
	if err := WriteLangGraphForkPlan(plan.ExecutionArtifact, forkPlan); err != nil {
		t.Fatalf("WriteLangGraphForkPlan without proof: %v", err)
	}
	unknown, err := CausalEffectEvidenceForRecordedPlan(plan)
	if err != nil {
		t.Fatalf("CausalEffectEvidenceForRecordedPlan without proof: %v", err)
	}
	if unknown.Status != CausalEffectEvidenceUnknown || unknown.LangGraphToolEffectProof != nil {
		t.Fatalf("legacy/unknown plan evidence must stay unknown: %#v", unknown)
	}

	forkPlan.ToolEffectProvenance = &LangGraphToolEffectProvenance{
		ToolCallID:                 "call-1",
		ToolName:                   "shell",
		ShellSessionID:             "shell-1",
		CommandSHA256:              strings.Repeat("a", 64),
		CommandStartedMonotonicNS:  100,
		CommandFinishedMonotonicNS: 300,
		FirstEffectMonotonicNS:     150,
		LastEffectMonotonicNS:      160,
	}
	if err := WriteLangGraphForkPlan(plan.ExecutionArtifact, forkPlan); err != nil {
		t.Fatalf("WriteLangGraphForkPlan with proof: %v", err)
	}
	proven, err := CausalEffectEvidenceForRecordedPlan(plan)
	if err != nil {
		t.Fatalf("CausalEffectEvidenceForRecordedPlan with proof: %v", err)
	}
	if proven.Status != CausalEffectEvidenceProven || proven.LangGraphToolEffectProof == nil || proven.LangGraphToolEffectProof.ToolCallID != "call-1" {
		t.Fatalf("expected immutable LangGraph proof in causal evidence: %#v", proven)
	}
	if err := proven.ValidateFor(seed); err != nil {
		t.Fatalf("proven causal evidence does not bind to state seed: %v", err)
	}
}

func TestCausalEffectEvidenceMissingPlanRemainsUnknown(t *testing.T) {
	evidence, err := CausalEffectEvidenceForRecordedPlan(RecordedPlan{
		RecordedPlanID:    "recorded-plan:missing",
		AdapterID:         LangGraphForkAdapterID,
		ExecutionArtifact: filepath.Join(t.TempDir(), "missing-plan.json"),
	})
	if err != nil {
		t.Fatalf("missing legacy plan should not block classification: %v", err)
	}
	if evidence.Status != CausalEffectEvidenceUnknown || evidence.LangGraphToolEffectProof != nil {
		t.Fatalf("missing plan must produce unknown evidence: %#v", evidence)
	}
}

func validLangGraphPlanForCausalEvidence(seed objective.StateSeed, plan RecordedPlan) LangGraphForkPlan {
	return LangGraphForkPlan{
		SchemaVersion:         LangGraphForkPlanSchema,
		RecordedPlanID:        plan.RecordedPlanID,
		AdapterID:             plan.AdapterID,
		TargetID:              plan.TargetID,
		CandidateID:           "synthesis-candidate:test",
		Task:                  "run the recorded task",
		Model:                 "openai:test",
		ContainerImage:        "syncfuzz-langgraph:test",
		RuntimeRoot:           "/tmp/langgraph-forks",
		PassiveUnixSocketPath: "agent.sock",
		PassiveObservationID:  plan.PassiveObservationID,
		SourceThreadID:        "thread-1",
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
			seed.BeforeCheckpointID: {
				SchemaVersion:      LangGraphNativeCoordinateSchema,
				SourceCheckpointID: "native-before",
				HistoryIndex:       0,
				MessageCount:       1,
				Next:               []string{},
			},
			seed.AfterCheckpointID: {
				SchemaVersion:      LangGraphNativeCoordinateSchema,
				SourceCheckpointID: "native-after",
				HistoryIndex:       1,
				MessageCount:       2,
				Next:               []string{"model"},
			},
		},
		AgentStateByCheckpoint: map[string]StatePresence{
			seed.BeforeCheckpointID: StatePresenceAbsent,
			seed.AfterCheckpointID:  StatePresencePresent,
		},
	}
}
