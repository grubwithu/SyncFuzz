package recovery

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

func TestNewForkPairChangesOnlyCheckpoint(t *testing.T) {
	seed := testSeed()
	plan := RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       seed.RecordedPlanID,
		AdapterID:            seed.AdapterID,
		TargetID:             seed.TargetID,
		ExecutionArtifact:    seed.RecordedPlanArtifact,
		PassiveObservationID: "observation:socket",
	}
	pair, err := NewForkPair(seed, plan)
	if err != nil {
		t.Fatalf("NewForkPair returned error: %v", err)
	}
	if pair.Before.CheckpointID != "C0" || pair.After.CheckpointID != "C1" || pair.Before.PassiveObservationID != pair.After.PassiveObservationID {
		t.Fatalf("pair did not preserve only-checkpoint invariant: %#v", pair)
	}
	pair.After.PassiveObservationID = "observation:changed"
	if err := pair.ValidateFor(seed); err == nil {
		t.Fatal("expected changed passive observation to invalidate pair")
	}
}

func testSeed() objective.StateSeed {
	return objective.StateSeed{
		SchemaVersion:        objective.SchemaVersion,
		SeedID:               "state-seed:profile-1:C0..C1",
		ObjectiveID:          "ipc.unix-listener.survival",
		ProfileRunID:         "profile-1",
		ProfileRunKind:       objective.ProfileRunKindSynthesisCandidate,
		SynthesisCandidateID: "synthesis-candidate:ipc-unix-listener:1",
		TargetID:             "langgraph-shell-react",
		AdapterID:            "langgraph-shell-react",
		RecordedPlanID:       "recorded-plan:profile-1",
		RecordedPlanArtifact: "recorded-plan.json",
		FrontierID:           "C0..C1",
		BeforeCheckpointID:   "C0",
		AfterCheckpointID:    "C1",
		ValidatedEffects:     []objective.EffectAtom{{Family: profiling.StateFamilyIPC, Operation: "bind"}},
		ResourceIDs:          []string{"unix-socket:socket:123"},
	}
}
