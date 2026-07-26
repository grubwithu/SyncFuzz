package recovery

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNewForkRecoverySetWithContinuationFreezesOneInputAcrossControls(t *testing.T) {
	seed := testSeed()
	head, err := MaterializationHeadFor(seed)
	if err != nil {
		t.Fatalf("MaterializationHeadFor: %v", err)
	}
	continuation, err := NewContinuationQuery("Summarize the restored workspace without changing it.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	set, err := NewForkRecoverySetWithContinuation(seed, RecordedPlan{
		SchemaVersion:         SchemaVersion,
		RecordedPlanID:        seed.RecordedPlanID,
		AdapterID:             seed.AdapterID,
		TargetID:              seed.TargetID,
		ExecutionArtifact:     seed.RecordedPlanArtifact,
		PassiveObservationID:  "observation:socket",
		MaterializationHeadID: head.HeadID,
		RetentionPolicy:       RetentionPolicyRetainRelevantOSState,
	}, continuation)
	if err != nil {
		t.Fatalf("NewForkRecoverySetWithContinuation: %v", err)
	}
	if set.ContinuationQuery == nil || set.ContinuationQuery.ContinuationQueryID != continuation.ContinuationQueryID {
		t.Fatalf("recovery set did not retain frozen continuation: %#v", set.ContinuationQuery)
	}
	for _, query := range []RecoveryQuery{set.Before, set.After, set.Head} {
		if query.ContinuationQueryID != continuation.ContinuationQueryID {
			t.Fatalf("recovery query %q changed frozen continuation: %#v", query.QueryID, query)
		}
	}

	// The constructor copies the value so a caller cannot mutate the frozen
	// recovery plan through the object it originally supplied.
	continuation.Query = "different input"
	if err := set.ValidateFor(seed); err != nil {
		t.Fatalf("set changed after caller mutation: %v", err)
	}
	set.After.ContinuationQueryID = "continuation-query:other"
	if err := set.ValidateFor(seed); err == nil || !strings.Contains(err.Error(), "frozen continuation") {
		t.Fatalf("expected changed control continuation to fail validation, got %v", err)
	}
}

func TestRecoveryObservationRequiresBoundPreAndPostContinuationEvidence(t *testing.T) {
	continuation, err := NewContinuationQuery("Inspect the restored state.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	query := RecoveryQuery{
		QueryID:              "recovery-query:test:C0",
		SeedID:               "seed:test",
		Boundary:             BoundaryFork,
		CheckpointID:         "C0",
		RecordedPlanID:       "plan:test",
		PassiveObservationID: "observation:test",
		ContinuationQueryID:  continuation.ContinuationQueryID,
	}
	plan := RecordedPlan{RecordedPlanID: query.RecordedPlanID}
	observation := RecoveryObservation{
		SchemaVersion:        ExecutionSchemaVersion,
		QueryID:              query.QueryID,
		SeedID:               query.SeedID,
		Boundary:             query.Boundary,
		CheckpointID:         query.CheckpointID,
		RecordedPlanID:       query.RecordedPlanID,
		PassiveObservationID: query.PassiveObservationID,
		RuntimeInstanceID:    "runtime:test",
		AgentState:           StatePresencePresent,
		OSState:              StatePresencePresent,
		OSStateOrigin:        StateOriginResidual,
		EffectMultiplicity:   EffectMultiplicitySingle,
		Evidence:             []string{"passive-observation"},
	}
	if err := observation.ValidateFor(query, plan); err == nil || !strings.Contains(err.Error(), "requires pre/post continuation evidence") {
		t.Fatalf("expected missing continuation evidence rejection, got %v", err)
	}
	observation.ContinuationEvidence = &ContinuationEvidence{
		ContinuationQueryID: continuation.ContinuationQueryID,
		PreEvidence:         []string{"exact-checkpoint-restored"},
		PostEvidence:        []string{"continuation-completed"},
	}
	if err := observation.ValidateFor(query, plan); err != nil {
		t.Fatalf("complete continuation observation rejected: %v", err)
	}
	observation.ContinuationEvidence.PostEvidence = nil
	if err := observation.ValidateFor(query, plan); err == nil || !strings.Contains(err.Error(), "complete pre/post") {
		t.Fatalf("expected missing post evidence rejection, got %v", err)
	}
}

func TestContinuationRoundTripsThroughRecoverySetAndExecutionArtifacts(t *testing.T) {
	seed := testSeed()
	head, err := MaterializationHeadFor(seed)
	if err != nil {
		t.Fatalf("MaterializationHeadFor: %v", err)
	}
	continuation, err := NewContinuationQuery("List the files after restore.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
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
	set, err := NewForkRecoverySetWithContinuation(seed, plan, continuation)
	if err != nil {
		t.Fatalf("NewForkRecoverySetWithContinuation: %v", err)
	}
	setPath := t.TempDir() + "/recovery-set.json"
	if err := WriteHistoricalRecoverySet(setPath, *set); err != nil {
		t.Fatalf("WriteHistoricalRecoverySet: %v", err)
	}
	decodedSet, err := ReadHistoricalRecoverySet(setPath)
	if err != nil {
		t.Fatalf("ReadHistoricalRecoverySet: %v", err)
	}
	if !reflect.DeepEqual(decodedSet, *set) || decodedSet.ValidateFor(seed) != nil {
		t.Fatalf("continuation recovery set did not round-trip: %#v", decodedSet)
	}

	execution, err := ExecuteForkRecoverySet(context.Background(), seed, *set, plan, ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		if request.ContinuationQuery == nil || request.ContinuationQuery.ContinuationQueryID != continuation.ContinuationQueryID {
			t.Fatalf("executor did not receive frozen continuation: %#v", request)
		}
		return continuationTestObservation(request), nil
	}))
	if err != nil {
		t.Fatalf("ExecuteForkRecoverySet: %v", err)
	}
	executionPath := t.TempDir() + "/recovery-set-execution.json"
	if err := WriteForkRecoverySetExecution(executionPath, *execution); err != nil {
		t.Fatalf("WriteForkRecoverySetExecution: %v", err)
	}
	decodedExecution, err := ReadForkRecoverySetExecution(executionPath)
	if err != nil {
		t.Fatalf("ReadForkRecoverySetExecution: %v", err)
	}
	if !reflect.DeepEqual(decodedExecution, *execution) {
		t.Fatalf("continuation recovery execution did not round-trip: %#v", decodedExecution)
	}
}

func TestLangGraphForkPlanAndRecoverySetMustUseTheSameContinuation(t *testing.T) {
	frozen, err := NewContinuationQuery("Continue the restored task and report progress.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	same, err := NewContinuationQuery("Continue the restored task and report progress.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	different, err := NewContinuationQuery("Inspect the restored task and list any remaining work.")
	if err != nil {
		t.Fatalf("NewContinuationQuery: %v", err)
	}
	if !sameContinuationQuery(frozen, same) {
		t.Fatal("identical frozen continuation queries must match")
	}
	if sameContinuationQuery(frozen, different) {
		t.Fatal("different continuation queries must not match")
	}
	if sameContinuationQuery(frozen, nil) || sameContinuationQuery(nil, frozen) || !sameContinuationQuery(nil, nil) {
		t.Fatal("nil continuation matching is not fail-closed")
	}
}

func continuationTestObservation(request ForkExecutionRequest) RecoveryObservation {
	observation := RecoveryObservation{
		SchemaVersion:         ExecutionSchemaVersion,
		QueryID:               request.Query.QueryID,
		SeedID:                request.Query.SeedID,
		Boundary:              request.Query.Boundary,
		CheckpointID:          request.Query.CheckpointID,
		RecordedPlanID:        request.Plan.RecordedPlanID,
		PassiveObservationID:  request.Query.PassiveObservationID,
		MaterializationHeadID: request.Query.MaterializationHeadID,
		RetentionPolicy:       request.Query.RetentionPolicy,
		RuntimeInstanceID:     "continuation-runtime:" + request.Query.CheckpointID,
		AgentState:            StatePresencePresent,
		OSState:               StatePresencePresent,
		OSStateOrigin:         StateOriginResidual,
		EffectMultiplicity:    EffectMultiplicitySingle,
		ContinuationEvidence: &ContinuationEvidence{
			ContinuationQueryID: request.Query.ContinuationQueryID,
			PreEvidence:         []string{"exact-checkpoint-restored"},
			PostEvidence:        []string{"continuation-completed"},
		},
		Evidence: []string{"passive-observation"},
	}
	if request.Query.CheckpointID == "C0" {
		observation.AgentState = StatePresenceAbsent
	}
	return observation
}
