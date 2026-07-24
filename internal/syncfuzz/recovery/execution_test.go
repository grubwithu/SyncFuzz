package recovery

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteForkPairUsesOnlyCheckpointAsVariable(t *testing.T) {
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
	requests := make([]ForkExecutionRequest, 0, 2)
	executor := ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		requests = append(requests, request)
		observation := RecoveryObservation{
			SchemaVersion:        ExecutionSchemaVersion,
			QueryID:              request.Query.QueryID,
			SeedID:               request.Query.SeedID,
			Boundary:             request.Query.Boundary,
			CheckpointID:         request.Query.CheckpointID,
			RecordedPlanID:       request.Plan.RecordedPlanID,
			PassiveObservationID: request.Query.PassiveObservationID,
			RuntimeInstanceID:    "runtime:" + request.Query.CheckpointID,
			EffectMultiplicity:   EffectMultiplicitySingle,
			Evidence:             []string{"deterministic-observation"},
		}
		if request.Query.CheckpointID == seed.BeforeCheckpointID {
			observation.AgentState = StatePresenceAbsent
			observation.OSState = StatePresencePresent
			observation.OSStateOrigin = StateOriginResidual
		} else {
			observation.AgentState = StatePresencePresent
			observation.OSState = StatePresencePresent
			observation.OSStateOrigin = StateOriginResidual
		}
		return observation, nil
	})
	execution, err := ExecuteForkPair(context.Background(), seed, *pair, plan, executor)
	if err != nil {
		t.Fatalf("ExecuteForkPair returned error: %v", err)
	}
	if len(requests) != 2 || requests[0].Query.CheckpointID != seed.BeforeCheckpointID || requests[1].Query.CheckpointID != seed.AfterCheckpointID {
		t.Fatalf("unexpected fork requests: %#v", requests)
	}
	for _, request := range requests {
		if request.Plan.RecordedPlanID != seed.RecordedPlanID || request.Plan.ExecutionArtifact != seed.RecordedPlanArtifact || request.Query.PassiveObservationID != plan.PassiveObservationID || request.Query.Boundary != BoundaryFork {
			t.Fatalf("executor received mutable execution condition: %#v", request)
		}
	}
	if execution.Classification.BeforeOutcome != "residual" || execution.Classification.AfterOutcome != "consistent" || execution.Classification.Outcome != "residual" {
		t.Fatalf("unexpected paired classification: %#v", execution.Classification)
	}
}

func TestExecuteForkRecoverySetRequiresConsistentHeadControl(t *testing.T) {
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
	requests := make([]ForkExecutionRequest, 0, 3)
	executor := ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		requests = append(requests, request)
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
			RuntimeInstanceID:     "runtime:" + request.Query.CheckpointID,
			EffectMultiplicity:    EffectMultiplicitySingle,
			Evidence:              []string{"deterministic-observation"},
		}
		switch request.Query.CheckpointID {
		case seed.BeforeCheckpointID:
			observation.AgentState = StatePresenceAbsent
			observation.OSState = StatePresencePresent
			observation.OSStateOrigin = StateOriginResidual
		case seed.AfterCheckpointID, seed.MaterializationHeadCheckpointID:
			observation.AgentState = StatePresencePresent
			observation.OSState = StatePresencePresent
			observation.OSStateOrigin = StateOriginResidual
		}
		return observation, nil
	})
	execution, err := ExecuteForkRecoverySet(context.Background(), seed, *set, plan, executor)
	if err != nil {
		t.Fatalf("ExecuteForkRecoverySet returned error: %v", err)
	}
	if len(requests) != 3 || requests[2].Query.CheckpointID != seed.MaterializationHeadCheckpointID || execution.Classification.BeforeOutcome != "residual" || execution.Classification.AfterOutcome != "consistent" || execution.Classification.HeadOutcome != "consistent" || execution.Classification.Outcome != "residual" {
		t.Fatalf("unexpected recovery set execution: %#v requests=%#v", execution, requests)
	}

	inconclusiveExecution, err := ExecuteForkRecoverySet(context.Background(), seed, *set, plan, ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		observation := RecoveryObservation{SchemaVersion: ExecutionSchemaVersion, QueryID: request.Query.QueryID, SeedID: request.Query.SeedID, Boundary: request.Query.Boundary, CheckpointID: request.Query.CheckpointID, RecordedPlanID: request.Plan.RecordedPlanID, PassiveObservationID: request.Query.PassiveObservationID, MaterializationHeadID: request.Query.MaterializationHeadID, RetentionPolicy: request.Query.RetentionPolicy, RuntimeInstanceID: "unique:" + request.Query.CheckpointID, AgentState: StatePresencePresent, OSState: StatePresencePresent, OSStateOrigin: StateOriginResidual, EffectMultiplicity: EffectMultiplicitySingle, Evidence: []string{"deterministic-observation"}}
		if request.Query.CheckpointID == seed.BeforeCheckpointID {
			observation.AgentState = StatePresenceAbsent
		}
		if request.Query.CheckpointID == seed.MaterializationHeadCheckpointID {
			observation.OSStateOrigin = StateOriginReconstructed
		}
		return observation, nil
	}))
	if err != nil {
		t.Fatalf("ExecuteForkRecoverySet with non-consistent head returned error: %v", err)
	}
	if inconclusiveExecution.Classification.Outcome != "inconclusive" || inconclusiveExecution.Classification.HeadOutcome != "reconstruction" {
		t.Fatalf("non-consistent head control must force an inconclusive result: %#v", inconclusiveExecution.Classification)
	}
}

func TestExecuteForkPairRejectsObservationForDifferentQuery(t *testing.T) {
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
	_, err = ExecuteForkPair(context.Background(), seed, *pair, plan, ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		return RecoveryObservation{
			SchemaVersion:        ExecutionSchemaVersion,
			QueryID:              "wrong-query",
			SeedID:               request.Query.SeedID,
			Boundary:             request.Query.Boundary,
			CheckpointID:         request.Query.CheckpointID,
			RecordedPlanID:       request.Plan.RecordedPlanID,
			PassiveObservationID: request.Query.PassiveObservationID,
			AgentState:           StatePresencePresent,
			OSState:              StatePresencePresent,
			OSStateOrigin:        StateOriginResidual,
			EffectMultiplicity:   EffectMultiplicitySingle,
			Evidence:             []string{"wrong identity"},
		}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "does not bind to query") {
		t.Fatalf("expected query-binding rejection, got %v", err)
	}
}

func TestExecuteForkPairRequiresIndependentRuntimeInstances(t *testing.T) {
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
	_, err = ExecuteForkPair(context.Background(), seed, *pair, plan, ForkExecutorFunc(func(_ context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
		return RecoveryObservation{
			SchemaVersion:        ExecutionSchemaVersion,
			QueryID:              request.Query.QueryID,
			SeedID:               request.Query.SeedID,
			Boundary:             request.Query.Boundary,
			CheckpointID:         request.Query.CheckpointID,
			RecordedPlanID:       request.Plan.RecordedPlanID,
			PassiveObservationID: request.Query.PassiveObservationID,
			RuntimeInstanceID:    "reused-container",
			AgentState:           StatePresencePresent,
			OSState:              StatePresencePresent,
			OSStateOrigin:        StateOriginResidual,
			EffectMultiplicity:   EffectMultiplicitySingle,
			Evidence:             []string{"same runtime"},
		}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "reused runtime instance") {
		t.Fatalf("expected independent-runtime rejection, got %v", err)
	}
}

func TestForkExecutorRegistryRejectsCommandAdapterWithoutDurableCheckpoint(t *testing.T) {
	seed := testSeed()
	plan := RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       seed.RecordedPlanID,
		AdapterID:            "command",
		TargetID:             seed.TargetID,
		ExecutionArtifact:    seed.RecordedPlanArtifact,
		PassiveObservationID: "observation:socket",
	}
	pair, err := NewForkPair(seed, RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       seed.RecordedPlanID,
		AdapterID:            seed.AdapterID,
		TargetID:             seed.TargetID,
		ExecutionArtifact:    seed.RecordedPlanArtifact,
		PassiveObservationID: "observation:socket",
	})
	if err != nil {
		t.Fatalf("NewForkPair returned error: %v", err)
	}
	_, err = NewForkExecutorRegistry().Execute(context.Background(), seed, *pair, plan)
	if err == nil || !strings.Contains(err.Error(), "does not expose a durable checkpoint fork executor") {
		t.Fatalf("expected command adapter rejection, got %v", err)
	}
}

func TestClassifyForkPairPrefersDuplicateOverOtherOutcomes(t *testing.T) {
	before := RecoveryObservation{
		AgentState:         StatePresenceAbsent,
		OSState:            StatePresencePresent,
		OSStateOrigin:      StateOriginResidual,
		EffectMultiplicity: EffectMultiplicitySingle,
		Evidence:           []string{"residue"},
	}
	after := RecoveryObservation{
		AgentState:         StatePresencePresent,
		OSState:            StatePresencePresent,
		OSStateOrigin:      StateOriginResidual,
		EffectMultiplicity: EffectMultiplicityDuplicate,
		Evidence:           []string{"duplicate effect"},
	}
	classification := ClassifyForkPair(before, after)
	if classification.BeforeOutcome != "residual" || classification.AfterOutcome != "duplicate" || classification.Outcome != "duplicate" {
		t.Fatalf("unexpected classification precedence: %#v", classification)
	}
}
