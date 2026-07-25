package recovery

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

const RecoveryRelationReportSchema = "syncfuzz.recovery-relation-report.v1"

// RecoveryEvidenceStatus distinguishes missing measurement from a relation
// whose framework contract has not yet been evaluated.
type RecoveryEvidenceStatus string

const (
	RecoveryEvidenceComplete   RecoveryEvidenceStatus = "complete"
	RecoveryEvidenceIncomplete RecoveryEvidenceStatus = "incomplete"
)

func (s RecoveryEvidenceStatus) Valid() bool {
	return s == RecoveryEvidenceComplete || s == RecoveryEvidenceIncomplete
}

// LogicalEffectPhase records only what the durable agent-side projection can
// prove today. It deliberately does not label an observation PRE_CALL or
// CALL_DURABLE until adapters emit durable tool-lifecycle provenance.
type LogicalEffectPhase string

const (
	LogicalEffectPhaseUnknown LogicalEffectPhase = "unknown"
	LogicalEffectNotCommitted LogicalEffectPhase = "effect-not-committed"
	LogicalEffectCommitted    LogicalEffectPhase = "effect-committed"
)

func (p LogicalEffectPhase) Valid() bool {
	return p == LogicalEffectPhaseUnknown || p == LogicalEffectNotCommitted || p == LogicalEffectCommitted
}

// ResourceOrigin normalizes an adapter's recovery-specific state origin into
// a relation dimension. "original" means retained from the materialized
// source state, not merely that a path with the same name exists.
type ResourceOrigin string

const (
	ResourceOriginOriginal      ResourceOrigin = "original"
	ResourceOriginReconstructed ResourceOrigin = "reconstructed"
	ResourceOriginNone          ResourceOrigin = "none"
	ResourceOriginUnknown       ResourceOrigin = "unknown"
)

func (o ResourceOrigin) Valid() bool {
	return o == ResourceOriginOriginal || o == ResourceOriginReconstructed || o == ResourceOriginNone || o == ResourceOriginUnknown
}

// ResourceActivity stays unknown until an adapter exposes a family-specific
// passive observation. Presence and multiplicity alone must not imply active
// reachability for every resource family.
type ResourceActivity string

const (
	ResourceActivityActive   ResourceActivity = "active"
	ResourceActivityInactive ResourceActivity = "inactive"
	ResourceActivityUnknown  ResourceActivity = "unknown"
)

func (a ResourceActivity) Valid() bool {
	return a == ResourceActivityActive || a == ResourceActivityInactive || a == ResourceActivityUnknown
}

type RecoveryRelationClass string

const (
	RecoveryRelationAligned                     RecoveryRelationClass = "aligned"
	RecoveryRelationUncommittedOriginalResidual RecoveryRelationClass = "uncommitted-original-residual"
	RecoveryRelationMissingCommittedEffect      RecoveryRelationClass = "missing-committed-effect"
	RecoveryRelationReconstruction              RecoveryRelationClass = "reconstruction"
	RecoveryRelationDuplicate                   RecoveryRelationClass = "duplicate"
	RecoveryRelationUnknown                     RecoveryRelationClass = "unknown"
)

func (c RecoveryRelationClass) Valid() bool {
	switch c {
	case RecoveryRelationAligned, RecoveryRelationUncommittedOriginalResidual, RecoveryRelationMissingCommittedEffect, RecoveryRelationReconstruction, RecoveryRelationDuplicate, RecoveryRelationUnknown:
		return true
	default:
		return false
	}
}

// ContractStatus is intentionally independent from evidence classification.
// The recovery fuzzer emits not-evaluated by default; no relation is promoted
// to a violation merely because it is unusual.
type ContractStatus string

const (
	ContractNotEvaluated ContractStatus = "not-evaluated"
	ContractUnspecified  ContractStatus = "unspecified"
	ContractAllowed      ContractStatus = "allowed"
	ContractViolated     ContractStatus = "violated"
)

func (s ContractStatus) Valid() bool {
	return s == ContractNotEvaluated || s == ContractUnspecified || s == ContractAllowed || s == ContractViolated
}

type ContractEvaluation struct {
	Status     ContractStatus `json:"status"`
	ContractID string         `json:"contract_id,omitempty"`
	Reason     string         `json:"reason"`
}

func (c ContractEvaluation) Validate() error {
	if !c.Status.Valid() || strings.TrimSpace(c.Reason) == "" {
		return fmt.Errorf("contract evaluation requires a status and reason")
	}
	if c.Status == ContractNotEvaluated && strings.TrimSpace(c.ContractID) != "" {
		return fmt.Errorf("not-evaluated contract result must not name a contract")
	}
	if c.Status != ContractNotEvaluated && strings.TrimSpace(c.ContractID) == "" {
		return fmt.Errorf("evaluated contract result requires a contract ID")
	}
	return nil
}

// RecoveryRelationEvidence is the evidence-only projection used for relation
// discovery and later coverage. Activity is allowed to remain unknown without
// making an otherwise identity-complete relation incomplete.
type RecoveryRelationEvidence struct {
	Status       RecoveryEvidenceStatus `json:"status"`
	LogicalPhase LogicalEffectPhase     `json:"logical_phase"`
	OSPresence   StatePresence          `json:"os_presence"`
	Origin       ResourceOrigin         `json:"origin"`
	Multiplicity EffectMultiplicity     `json:"multiplicity"`
	Activity     ResourceActivity       `json:"activity"`
}

func (e RecoveryRelationEvidence) Validate() error {
	if !e.Status.Valid() || !e.LogicalPhase.Valid() || !e.OSPresence.Valid() || !e.Origin.Valid() || !e.Multiplicity.Valid() || !e.Activity.Valid() {
		return fmt.Errorf("recovery relation evidence has an invalid dimension")
	}
	if e.Status == RecoveryEvidenceComplete && (e.LogicalPhase == LogicalEffectPhaseUnknown || e.OSPresence == StatePresenceUnknown || e.Origin == ResourceOriginUnknown || e.Multiplicity == EffectMultiplicityUnknown) {
		return fmt.Errorf("complete recovery relation evidence cannot have an unknown required dimension")
	}
	return nil
}

type RecoveryRelation struct {
	Class     RecoveryRelationClass `json:"class"`
	Signature string                `json:"signature"`
}

func (r RecoveryRelation) ValidateFor(evidence RecoveryRelationEvidence) error {
	if !r.Class.Valid() || strings.TrimSpace(r.Signature) == "" {
		return fmt.Errorf("recovery relation requires class and signature")
	}
	if evidence.Status == RecoveryEvidenceIncomplete && r.Class != RecoveryRelationUnknown {
		return fmt.Errorf("incomplete recovery relation evidence must classify as unknown")
	}
	if r.Signature != recoveryRelationSignature(evidence, r.Class) {
		return fmt.Errorf("recovery relation signature does not match its evidence")
	}
	return nil
}

type RecoveryRelationControl struct {
	Name         string                   `json:"name"`
	CheckpointID string                   `json:"checkpoint_id"`
	Evidence     RecoveryRelationEvidence `json:"evidence"`
	Relation     RecoveryRelation         `json:"relation"`
}

// RecoveryRelationReport preserves the evidence/relation/contract split for
// a complete before/after/head recovery set. Effect and resource scope are
// copied from the StateSeed rather than inferred from task prose.
type RecoveryRelationReport struct {
	SchemaVersion        string                    `json:"schema_version"`
	SeedID               string                    `json:"seed_id"`
	ObjectiveID          string                    `json:"objective_id"`
	ProfileRunID         string                    `json:"profile_run_id"`
	FrontierID           string                    `json:"frontier_id"`
	EffectScope          []objective.EffectAtom    `json:"effect_scope"`
	SeedResourceIDs      []string                  `json:"seed_resource_ids"`
	Controls             []RecoveryRelationControl `json:"controls"`
	CausalEffectEvidence *CausalEffectEvidence     `json:"causal_effect_evidence,omitempty"`
	Contract             ContractEvaluation        `json:"contract"`
}

// DeriveRecoveryRelation maps existing adapter observations into a generic
// relation. It does not inspect natural-language tasks or assert a framework
// contract.
func DeriveRecoveryRelation(observation RecoveryObservation) (RecoveryRelationEvidence, RecoveryRelation) {
	evidence := RecoveryRelationEvidence{
		LogicalPhase: logicalPhaseFromAgentState(observation.AgentState),
		OSPresence:   observation.OSState,
		Origin:       resourceOriginFromStateOrigin(observation.OSStateOrigin),
		Multiplicity: observation.EffectMultiplicity,
		Activity:     ResourceActivityUnknown,
	}
	if evidence.LogicalPhase == LogicalEffectPhaseUnknown || evidence.OSPresence == StatePresenceUnknown || evidence.Origin == ResourceOriginUnknown || evidence.Multiplicity == EffectMultiplicityUnknown || len(observation.Evidence) == 0 {
		evidence.Status = RecoveryEvidenceIncomplete
	} else {
		evidence.Status = RecoveryEvidenceComplete
	}
	relation := RecoveryRelation{Class: classifyRecoveryRelation(evidence)}
	relation.Signature = recoveryRelationSignature(evidence, relation.Class)
	return evidence, relation
}

// BuildRecoveryRelationReport transforms a completed recovery set into a
// relation artifact. Contract evaluation intentionally remains separate and
// defaults to not-evaluated.
func BuildRecoveryRelationReport(seed objective.StateSeed, execution ForkRecoverySetExecution) (RecoveryRelationReport, error) {
	return BuildRecoveryRelationReportWithCausalEffectEvidence(seed, execution, nil)
}

// BuildRecoveryRelationReportWithCausalEffectEvidence preserves immutable
// adapter proof alongside generic relation evidence. It does not alter logical
// phase, relation class, normalized signature, or contract evaluation.
func BuildRecoveryRelationReportWithCausalEffectEvidence(seed objective.StateSeed, execution ForkRecoverySetExecution, causalEvidence *CausalEffectEvidence) (RecoveryRelationReport, error) {
	if err := seed.Validate(); err != nil {
		return RecoveryRelationReport{}, err
	}
	if causalEvidence != nil {
		if err := causalEvidence.ValidateFor(seed); err != nil {
			return RecoveryRelationReport{}, err
		}
	}
	if execution.SchemaVersion != ExecutionSchemaVersion || execution.SeedID != seed.SeedID || execution.FrontierID != seed.FrontierID || execution.RecordedPlanID != seed.RecordedPlanID || execution.RetentionPolicy != RetentionPolicyRetainRelevantOSState {
		return RecoveryRelationReport{}, fmt.Errorf("recovery execution does not match state seed %q", seed.SeedID)
	}
	if err := execution.MaterializationHead.ValidateFor(seed); err != nil {
		return RecoveryRelationReport{}, err
	}
	if execution.Before.CheckpointID != seed.BeforeCheckpointID || execution.After.CheckpointID != seed.AfterCheckpointID || execution.Head.CheckpointID != seed.MaterializationHeadCheckpointID {
		return RecoveryRelationReport{}, fmt.Errorf("recovery execution does not preserve state seed checkpoint coordinates")
	}
	controls := make([]RecoveryRelationControl, 0, 3)
	for _, input := range []struct {
		name        string
		observation RecoveryObservation
	}{
		{name: "before", observation: execution.Before},
		{name: "after", observation: execution.After},
		{name: "head", observation: execution.Head},
	} {
		if err := validateRelationObservation(seed, execution, input.name, input.observation); err != nil {
			return RecoveryRelationReport{}, err
		}
		evidence, relation := DeriveRecoveryRelation(input.observation)
		controls = append(controls, RecoveryRelationControl{Name: input.name, CheckpointID: input.observation.CheckpointID, Evidence: evidence, Relation: relation})
	}
	report := RecoveryRelationReport{
		SchemaVersion:   RecoveryRelationReportSchema,
		SeedID:          seed.SeedID,
		ObjectiveID:     seed.ObjectiveID,
		ProfileRunID:    seed.ProfileRunID,
		FrontierID:      seed.FrontierID,
		EffectScope:     canonicalEffects(seed.ValidatedEffects),
		SeedResourceIDs: sortedStrings(seed.ResourceIDs),
		Controls:        controls,
		Contract: ContractEvaluation{
			Status: ContractNotEvaluated,
			Reason: "relation classification does not evaluate a framework contract",
		},
	}
	if causalEvidence != nil {
		clone := *causalEvidence
		if causalEvidence.LangGraphToolEffectProof != nil {
			proof := *causalEvidence.LangGraphToolEffectProof
			clone.LangGraphToolEffectProof = &proof
		}
		report.CausalEffectEvidence = &clone
	}
	if err := report.Validate(); err != nil {
		return RecoveryRelationReport{}, err
	}
	return report, nil
}

func validateRelationObservation(seed objective.StateSeed, execution ForkRecoverySetExecution, name string, observation RecoveryObservation) error {
	expectedCheckpoint := seed.BeforeCheckpointID
	if name == "after" {
		expectedCheckpoint = seed.AfterCheckpointID
	} else if name == "head" {
		expectedCheckpoint = seed.MaterializationHeadCheckpointID
	}
	if observation.SchemaVersion != ExecutionSchemaVersion || observation.SeedID != seed.SeedID || observation.Boundary != BoundaryFork || observation.CheckpointID != expectedCheckpoint || observation.RecordedPlanID != seed.RecordedPlanID || observation.MaterializationHeadID != execution.MaterializationHead.HeadID || observation.RetentionPolicy != execution.RetentionPolicy || strings.TrimSpace(observation.QueryID) == "" || strings.TrimSpace(observation.PassiveObservationID) == "" || strings.TrimSpace(observation.RuntimeInstanceID) == "" || !observation.AgentState.Valid() || !observation.OSState.Valid() || !observation.OSStateOrigin.Valid() || !observation.EffectMultiplicity.Valid() || len(observation.Evidence) == 0 {
		return fmt.Errorf("recovery relation %s observation does not preserve recovery-set evidence", name)
	}
	return nil
}

func (r RecoveryRelationReport) Validate() error {
	if r.SchemaVersion != RecoveryRelationReportSchema || strings.TrimSpace(r.SeedID) == "" || strings.TrimSpace(r.ObjectiveID) == "" || strings.TrimSpace(r.ProfileRunID) == "" || strings.TrimSpace(r.FrontierID) == "" || len(r.EffectScope) == 0 || len(r.SeedResourceIDs) == 0 || len(r.Controls) != 3 {
		return fmt.Errorf("recovery relation report is incomplete")
	}
	if err := r.Contract.Validate(); err != nil {
		return err
	}
	if r.CausalEffectEvidence != nil {
		if err := r.CausalEffectEvidence.Validate(); err != nil {
			return fmt.Errorf("recovery relation report causal effect evidence: %w", err)
		}
	}
	expectedNames := []string{"before", "after", "head"}
	for index, control := range r.Controls {
		if control.Name != expectedNames[index] || strings.TrimSpace(control.CheckpointID) == "" {
			return fmt.Errorf("recovery relation report has invalid control ordering")
		}
		if err := control.Evidence.Validate(); err != nil {
			return fmt.Errorf("recovery relation control %q: %w", control.Name, err)
		}
		if err := control.Relation.ValidateFor(control.Evidence); err != nil {
			return fmt.Errorf("recovery relation control %q: %w", control.Name, err)
		}
	}
	return nil
}

func logicalPhaseFromAgentState(state StatePresence) LogicalEffectPhase {
	switch state {
	case StatePresenceAbsent:
		return LogicalEffectNotCommitted
	case StatePresencePresent:
		return LogicalEffectCommitted
	default:
		return LogicalEffectPhaseUnknown
	}
}

func resourceOriginFromStateOrigin(origin StateOrigin) ResourceOrigin {
	switch origin {
	case StateOriginResidual:
		return ResourceOriginOriginal
	case StateOriginReconstructed:
		return ResourceOriginReconstructed
	case StateOriginNone:
		return ResourceOriginNone
	default:
		return ResourceOriginUnknown
	}
}

func classifyRecoveryRelation(evidence RecoveryRelationEvidence) RecoveryRelationClass {
	if evidence.Status != RecoveryEvidenceComplete {
		return RecoveryRelationUnknown
	}
	if evidence.Multiplicity == EffectMultiplicityDuplicate {
		return RecoveryRelationDuplicate
	}
	if evidence.OSPresence == StatePresencePresent && evidence.Origin == ResourceOriginReconstructed {
		return RecoveryRelationReconstruction
	}
	if evidence.LogicalPhase == LogicalEffectNotCommitted && evidence.OSPresence == StatePresencePresent && evidence.Origin == ResourceOriginOriginal {
		return RecoveryRelationUncommittedOriginalResidual
	}
	if evidence.LogicalPhase == LogicalEffectCommitted && evidence.OSPresence == StatePresenceAbsent {
		return RecoveryRelationMissingCommittedEffect
	}
	if (evidence.LogicalPhase == LogicalEffectNotCommitted && evidence.OSPresence == StatePresenceAbsent) || (evidence.LogicalPhase == LogicalEffectCommitted && evidence.OSPresence == StatePresencePresent && (evidence.Origin == ResourceOriginOriginal || evidence.Origin == ResourceOriginNone)) {
		return RecoveryRelationAligned
	}
	return RecoveryRelationUnknown
}

func recoveryRelationSignature(evidence RecoveryRelationEvidence, class RecoveryRelationClass) string {
	return strings.Join([]string{string(evidence.LogicalPhase), string(evidence.OSPresence), string(evidence.Origin), string(evidence.Multiplicity), string(evidence.Activity), string(class)}, "|")
}

func canonicalEffects(effects []objective.EffectAtom) []objective.EffectAtom {
	result := append([]objective.EffectAtom(nil), effects...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Family == result[right].Family {
			return result[left].Operation < result[right].Operation
		}
		return result[left].Family < result[right].Family
	})
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
