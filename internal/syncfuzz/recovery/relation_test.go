package recovery

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeriveRecoveryRelationSeparatesCompleteEvidenceFromContract(t *testing.T) {
	evidence, relation := DeriveRecoveryRelation(RecoveryObservation{
		AgentState:         StatePresenceAbsent,
		OSState:            StatePresencePresent,
		OSStateOrigin:      StateOriginResidual,
		EffectMultiplicity: EffectMultiplicitySingle,
		Evidence:           []string{"exact source listener identity"},
	})
	if evidence.Status != RecoveryEvidenceComplete || evidence.LogicalPhase != LogicalEffectNotCommitted || evidence.Origin != ResourceOriginOriginal || evidence.Activity != ResourceActivityUnknown || relation.Class != RecoveryRelationUncommittedOriginalResidual {
		t.Fatalf("unexpected complete relation derivation: evidence=%#v relation=%#v", evidence, relation)
	}
	if relation.Signature == "" {
		t.Fatal("expected a normalized relation signature")
	}

	incompleteEvidence, incompleteRelation := DeriveRecoveryRelation(RecoveryObservation{
		AgentState:         StatePresenceAbsent,
		OSState:            StatePresencePresent,
		OSStateOrigin:      StateOriginResidual,
		EffectMultiplicity: EffectMultiplicityUnknown,
		Evidence:           []string{"holder identity without multiplicity proof"},
	})
	if incompleteEvidence.Status != RecoveryEvidenceIncomplete || incompleteRelation.Class != RecoveryRelationUnknown {
		t.Fatalf("incomplete measurement must not become a relation claim: evidence=%#v relation=%#v", incompleteEvidence, incompleteRelation)
	}
}

func TestBuildRecoveryRelationReportKeepsContractNotEvaluated(t *testing.T) {
	seed := testSeed()
	seed.AdapterID = LangGraphForkAdapterID
	head, err := MaterializationHeadFor(seed)
	if err != nil {
		t.Fatalf("MaterializationHeadFor: %v", err)
	}
	observation := func(checkpointID string, agentState StatePresence) RecoveryObservation {
		return RecoveryObservation{
			SchemaVersion:         ExecutionSchemaVersion,
			QueryID:               "query:" + checkpointID,
			SeedID:                seed.SeedID,
			Boundary:              BoundaryFork,
			CheckpointID:          checkpointID,
			RecordedPlanID:        seed.RecordedPlanID,
			PassiveObservationID:  "passive:socket",
			MaterializationHeadID: head.HeadID,
			RetentionPolicy:       RetentionPolicyRetainRelevantOSState,
			RuntimeInstanceID:     "runtime:" + checkpointID,
			AgentState:            agentState,
			OSState:               StatePresencePresent,
			OSStateOrigin:         StateOriginResidual,
			EffectMultiplicity:    EffectMultiplicitySingle,
			Evidence:              []string{"exact resource identity"},
		}
	}
	causalEvidence := CausalEffectEvidence{
		Status:         CausalEffectEvidenceProven,
		AdapterID:      seed.AdapterID,
		RecordedPlanID: seed.RecordedPlanID,
		LangGraphToolEffectProof: &LangGraphToolEffectProvenance{
			ToolCallID:                 "call-1",
			ToolName:                   "shell",
			ShellSessionID:             "shell-1",
			CommandSHA256:              "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			CommandStartedMonotonicNS:  100,
			CommandFinishedMonotonicNS: 300,
			FirstEffectMonotonicNS:     150,
			LastEffectMonotonicNS:      160,
		},
	}
	report, err := BuildRecoveryRelationReportWithCausalEffectEvidence(seed, ForkRecoverySetExecution{
		SchemaVersion:       ExecutionSchemaVersion,
		RecoverySetID:       "recovery-set:" + seed.SeedID,
		SeedID:              seed.SeedID,
		FrontierID:          seed.FrontierID,
		RecordedPlanID:      seed.RecordedPlanID,
		MaterializationHead: head,
		RetentionPolicy:     RetentionPolicyRetainRelevantOSState,
		Before:              observation(seed.BeforeCheckpointID, StatePresenceAbsent),
		After:               observation(seed.AfterCheckpointID, StatePresencePresent),
		Head:                observation(seed.MaterializationHeadCheckpointID, StatePresencePresent),
	}, &causalEvidence)
	if err != nil {
		t.Fatalf("BuildRecoveryRelationReport: %v", err)
	}
	if report.Contract.Status != ContractNotEvaluated || report.Controls[0].Relation.Class != RecoveryRelationUncommittedOriginalResidual || report.Controls[1].Relation.Class != RecoveryRelationAligned || report.Controls[2].Relation.Class != RecoveryRelationAligned {
		t.Fatalf("unexpected recovery relation report: %#v", report)
	}
	if report.CausalEffectEvidence == nil || report.CausalEffectEvidence.Status != CausalEffectEvidenceProven || report.CausalEffectEvidence.LangGraphToolEffectProof == nil || report.CausalEffectEvidence.LangGraphToolEffectProof.ToolCallID != "call-1" {
		t.Fatalf("relation report did not retain immutable causal evidence: %#v", report.CausalEffectEvidence)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("recovery relation report did not validate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "recovery-relation-report.json")
	if err := WriteRecoveryRelationReport(path, report); err != nil {
		t.Fatalf("WriteRecoveryRelationReport: %v", err)
	}
	decoded, err := ReadRecoveryRelationReport(path)
	if err != nil {
		t.Fatalf("ReadRecoveryRelationReport: %v", err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("relation report round trip mismatch: got=%#v want=%#v", decoded, report)
	}
}
