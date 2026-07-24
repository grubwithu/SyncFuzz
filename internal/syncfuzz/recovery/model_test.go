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

func TestNewForkRecoverySetFreezesHeadRetentionAndThreeCoordinates(t *testing.T) {
	seed := testSeed()
	head, err := MaterializationHeadFor(seed)
	if err != nil {
		t.Fatalf("MaterializationHeadFor returned error: %v", err)
	}
	plan := RecordedPlan{
		SchemaVersion:         SchemaVersion,
		RecordedPlanID:        seed.RecordedPlanID,
		AdapterID:             seed.AdapterID,
		TargetID:              seed.TargetID,
		ExecutionArtifact:     seed.RecordedPlanArtifact,
		PassiveObservationID:  "observation:socket",
		MaterializationHeadID: head.HeadID,
		RetentionPolicy:       RetentionPolicyRetainRelevantOSState,
	}
	set, err := NewForkRecoverySet(seed, plan)
	if err != nil {
		t.Fatalf("NewForkRecoverySet returned error: %v", err)
	}
	if set.Before.CheckpointID != "C0" || set.After.CheckpointID != "C1" || set.Head.CheckpointID != "C2" {
		t.Fatalf("recovery set did not preserve the expected coordinates: %#v", set)
	}
	for _, query := range []RecoveryQuery{set.Before, set.After, set.Head} {
		if query.MaterializationHeadID != head.HeadID || query.RetentionPolicy != RetentionPolicyRetainRelevantOSState || query.PassiveObservationID != plan.PassiveObservationID {
			t.Fatalf("query did not freeze recovery conditions: %#v", query)
		}
	}
	set.Head.RetentionPolicy = ""
	if err := set.ValidateFor(seed); err == nil {
		t.Fatal("expected changed head retention policy to invalidate recovery set")
	}
}

func testSeed() objective.StateSeed {
	return objective.StateSeed{
		SchemaVersion:                   objective.SchemaVersion,
		SeedID:                          "state-seed:profile-1:C0..C1",
		ObjectiveID:                     "ipc.unix-listener.survival",
		ProfileRunID:                    "profile-1",
		ProfileRunKind:                  objective.ProfileRunKindSynthesisCandidate,
		SynthesisCandidateID:            "synthesis-candidate:ipc-unix-listener:1",
		TargetID:                        "langgraph-shell-react",
		AdapterID:                       "langgraph-shell-react",
		RecordedPlanID:                  "recorded-plan:profile-1",
		RecordedPlanArtifact:            "recorded-plan.json",
		FrontierID:                      "C0..C1",
		BeforeCheckpointID:              "C0",
		AfterCheckpointID:               "C1",
		MaterializationHeadCheckpointID: "C2",
		MaterializationHeadMonotonicNS:  300,
		MaterializationHeadResourceIDs:  []string{"unix-socket:socket:123"},
		ValidatedEffects:                []objective.EffectAtom{{Family: profiling.StateFamilyIPC, Operation: "bind"}},
		ResourceIDs:                     []string{"unix-socket:socket:123"},
	}
}
