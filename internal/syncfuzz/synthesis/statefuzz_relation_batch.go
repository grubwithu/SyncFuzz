package synthesis

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const StateFuzzRelationBatchReportSchema = "syncfuzz.statefuzz-relation-batch-report.v1"

type StateFuzzRelationBatchEntryStatus string

const (
	StateFuzzRelationBatchAggregated              StateFuzzRelationBatchEntryStatus = "aggregated"
	StateFuzzRelationBatchNotEligible             StateFuzzRelationBatchEntryStatus = "not-eligible"
	StateFuzzRelationBatchInvalidRelationArtifact StateFuzzRelationBatchEntryStatus = "invalid-relation-artifact"
)

func (s StateFuzzRelationBatchEntryStatus) Valid() bool {
	return s == StateFuzzRelationBatchAggregated || s == StateFuzzRelationBatchNotEligible || s == StateFuzzRelationBatchInvalidRelationArtifact
}

// StateFuzzRelationBatchScope contains only evidence dimensions that must
// agree before independent generated attempts are compared. It deliberately
// excludes task prose, result content, tool-call IDs, and contract verdicts.
type StateFuzzRelationBatchScope struct {
	ObjectiveID          string                 `json:"objective_id"`
	TargetID             string                 `json:"target_id"`
	AdapterID            string                 `json:"adapter_id"`
	FrontierID           string                 `json:"frontier_id"`
	EffectScope          []objective.EffectAtom `json:"effect_scope"`
	PassiveObservationID string                 `json:"passive_observation_id"`
	PassiveProbeMode     string                 `json:"passive_probe_mode,omitempty"`
	RuntimeImageID       string                 `json:"runtime_image_id,omitempty"`
}

func (s StateFuzzRelationBatchScope) Validate() error {
	if strings.TrimSpace(s.ObjectiveID) == "" || strings.TrimSpace(s.TargetID) == "" || strings.TrimSpace(s.AdapterID) == "" || strings.TrimSpace(s.FrontierID) == "" || strings.TrimSpace(s.PassiveObservationID) == "" || len(s.EffectScope) == 0 {
		return fmt.Errorf("StateFuzz relation batch scope is incomplete")
	}
	previous := ""
	for _, effect := range s.EffectScope {
		if effect.Family == "" || strings.TrimSpace(effect.Operation) == "" {
			return fmt.Errorf("StateFuzz relation batch scope has an incomplete effect atom")
		}
		key := string(effect.Family) + "\x00" + effect.Operation
		if previous != "" && key <= previous {
			return fmt.Errorf("StateFuzz relation batch scope effect atoms are not strictly ordered")
		}
		previous = key
	}
	return nil
}

func (s StateFuzzRelationBatchScope) key() string {
	effects := make([]string, 0, len(s.EffectScope))
	for _, effect := range s.EffectScope {
		effects = append(effects, string(effect.Family)+"/"+effect.Operation)
	}
	return strings.Join([]string{s.ObjectiveID, s.TargetID, s.AdapterID, s.FrontierID, strings.Join(effects, ","), s.PassiveObservationID, s.PassiveProbeMode, s.RuntimeImageID}, "\x1f")
}

type StateFuzzRelationVectorCount struct {
	Scope          StateFuzzRelationBatchScope `json:"scope"`
	BeforeRelation string                      `json:"before_relation"`
	AfterRelation  string                      `json:"after_relation"`
	HeadRelation   string                      `json:"head_relation"`
	Count          int                         `json:"count"`
}

func (v StateFuzzRelationVectorCount) Validate() error {
	if err := v.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(v.BeforeRelation) == "" || strings.TrimSpace(v.AfterRelation) == "" || strings.TrimSpace(v.HeadRelation) == "" || v.Count <= 0 {
		return fmt.Errorf("StateFuzz relation vector count is incomplete")
	}
	return nil
}

func (v StateFuzzRelationVectorCount) key() string {
	return strings.Join([]string{v.Scope.key(), v.BeforeRelation, v.AfterRelation, v.HeadRelation}, "\x1f")
}

// StateFuzzRelationBatchEntry retains every source attempt. Only accepted
// attempts with a valid relation artifact enter vector aggregates; an absent
// or mixed relation artifact remains visible as a denominator result.
type StateFuzzRelationBatchEntry struct {
	ArtifactRoot            string                              `json:"artifact_root"`
	Attempt                 int                                 `json:"attempt"`
	StateFuzzStatus         StateFuzzBatchEntryStatus           `json:"statefuzz_status"`
	RelationStatus          StateFuzzRelationBatchEntryStatus   `json:"relation_status"`
	Reason                  string                              `json:"reason,omitempty"`
	CandidateID             string                              `json:"candidate_id,omitempty"`
	ProfileRunID            string                              `json:"profile_run_id,omitempty"`
	SeedID                  string                              `json:"seed_id,omitempty"`
	Scope                   *StateFuzzRelationBatchScope        `json:"scope,omitempty"`
	BeforeRelation          string                              `json:"before_relation,omitempty"`
	AfterRelation           string                              `json:"after_relation,omitempty"`
	HeadRelation            string                              `json:"head_relation,omitempty"`
	CompleteThreeControlSet bool                                `json:"complete_three_control_set,omitempty"`
	HeadConsistent          bool                                `json:"head_consistent,omitempty"`
	CausalEffectEvidence    recovery.CausalEffectEvidenceStatus `json:"causal_effect_evidence_status,omitempty"`
	ContractStatus          recovery.ContractStatus             `json:"contract_status,omitempty"`
	RuntimeImageID          string                              `json:"runtime_image_id,omitempty"`
}

// StateFuzzRelationBatchReport aggregates normalized before/after/head
// evidence from independent generated attempts. It does not evaluate a
// framework contract and never derives a relation from task-specific output.
type StateFuzzRelationBatchReport struct {
	SchemaVersion                string                         `json:"schema_version"`
	ObjectiveID                  string                         `json:"objective_id"`
	BatchRoot                    string                         `json:"batch_root"`
	AttemptCount                 int                            `json:"attempt_count"`
	AcceptedCount                int                            `json:"accepted_count"`
	AggregatedRelationCount      int                            `json:"aggregated_relation_count"`
	InvalidRelationArtifactCount int                            `json:"invalid_relation_artifact_count"`
	NotEligibleAttemptCount      int                            `json:"not_eligible_attempt_count"`
	CompleteThreeControlSetCount int                            `json:"complete_three_control_set_count"`
	HeadConsistentCount          int                            `json:"head_consistent_count"`
	AttemptStatusCounts          map[string]int                 `json:"attempt_status_counts"`
	RelationStatusCounts         map[string]int                 `json:"relation_status_counts"`
	CausalEffectEvidenceCounts   map[string]int                 `json:"causal_effect_evidence_counts"`
	ContractStatusCounts         map[string]int                 `json:"contract_status_counts"`
	RuntimeImageIDCounts         map[string]int                 `json:"runtime_image_id_counts"`
	RelationVectors              []StateFuzzRelationVectorCount `json:"relation_vectors"`
	Attempts                     []StateFuzzRelationBatchEntry  `json:"attempts"`
}

func BuildStateFuzzRelationBatchReport(stateObjective objective.StateObjective, root string) (StateFuzzRelationBatchReport, error) {
	batch, err := BuildStateFuzzBatchReport(stateObjective, root)
	if err != nil {
		return StateFuzzRelationBatchReport{}, err
	}
	report := StateFuzzRelationBatchReport{
		SchemaVersion:              StateFuzzRelationBatchReportSchema,
		ObjectiveID:                stateObjective.ObjectiveID,
		BatchRoot:                  batch.BatchRoot,
		AttemptStatusCounts:        make(map[string]int),
		RelationStatusCounts:       make(map[string]int),
		CausalEffectEvidenceCounts: make(map[string]int),
		ContractStatusCounts:       make(map[string]int),
		RuntimeImageIDCounts:       make(map[string]int),
		Attempts:                   make([]StateFuzzRelationBatchEntry, 0, len(batch.Attempts)),
	}
	for _, source := range batch.Attempts {
		entry := StateFuzzRelationBatchEntry{
			ArtifactRoot:    source.ArtifactRoot,
			Attempt:         source.Attempt,
			StateFuzzStatus: source.Status,
			CandidateID:     source.CandidateID,
			ProfileRunID:    source.ProfileRunID,
			SeedID:          source.SeedID,
		}
		if source.Status != StateFuzzBatchAccepted {
			entry.RelationStatus = StateFuzzRelationBatchNotEligible
			entry.Reason = source.Reason
		} else {
			entry = aggregateStateFuzzRelationAttempt(stateObjective, entry)
		}
		report.Attempts = append(report.Attempts, entry)
	}
	sort.Slice(report.Attempts, func(left, right int) bool {
		if report.Attempts[left].Attempt == report.Attempts[right].Attempt {
			return report.Attempts[left].ArtifactRoot < report.Attempts[right].ArtifactRoot
		}
		return report.Attempts[left].Attempt < report.Attempts[right].Attempt
	})
	if err := report.rebuildAggregates(); err != nil {
		return StateFuzzRelationBatchReport{}, err
	}
	if err := report.Validate(); err != nil {
		return StateFuzzRelationBatchReport{}, err
	}
	return report, nil
}

func aggregateStateFuzzRelationAttempt(stateObjective objective.StateObjective, entry StateFuzzRelationBatchEntry) StateFuzzRelationBatchEntry {
	invalid := func(reason string) StateFuzzRelationBatchEntry {
		entry.RelationStatus = StateFuzzRelationBatchInvalidRelationArtifact
		entry.Reason = reason
		return entry
	}
	seed, err := objective.ReadStateSeed(filepath.Join(entry.ArtifactRoot, "state-seed.json"))
	if err != nil || seed.ValidateFor(stateObjective) != nil || seed.SeedID != entry.SeedID || seed.ProfileRunID != entry.ProfileRunID || seed.SynthesisCandidateID != entry.CandidateID {
		return invalid("missing-or-invalid-state-seed")
	}
	profile, err := objective.ReadProfileRun(filepath.Join(entry.ArtifactRoot, "profile-run.json"))
	if err != nil || profile.ValidateFor(stateObjective) != nil || profile.ProfileRunID != seed.ProfileRunID || profile.SynthesisCandidateID != seed.SynthesisCandidateID || profile.TargetID != seed.TargetID || profile.AdapterID != seed.AdapterID {
		return invalid("missing-or-invalid-profile-lineage")
	}
	execution, err := recovery.ReadForkRecoverySetExecution(filepath.Join(entry.ArtifactRoot, "recovery-set-execution.json"))
	if err != nil || execution.SeedID != seed.SeedID || execution.FrontierID != seed.FrontierID || execution.RecordedPlanID != seed.RecordedPlanID || execution.MaterializationHead.ValidateFor(seed) != nil || strings.TrimSpace(execution.Classification.BeforeOutcome) == "" || strings.TrimSpace(execution.Classification.AfterOutcome) == "" || strings.TrimSpace(execution.Classification.HeadOutcome) == "" || strings.TrimSpace(execution.Classification.Outcome) == "" {
		return invalid("missing-or-invalid-recovery-execution")
	}
	relation, err := recovery.ReadRecoveryRelationReport(filepath.Join(entry.ArtifactRoot, "recovery-relation-report.json"))
	if err != nil || relation.SeedID != seed.SeedID || relation.ObjectiveID != stateObjective.ObjectiveID || relation.ProfileRunID != profile.ProfileRunID || relation.FrontierID != seed.FrontierID || !reflect.DeepEqual(relation.EffectScope, canonicalStateFuzzEffects(seed.ValidatedEffects)) {
		return invalid("missing-or-invalid-relation-report")
	}
	if relation.CausalEffectEvidence != nil {
		if err := relation.CausalEffectEvidence.ValidateFor(seed); err != nil {
			return invalid("relation-causal-evidence-does-not-match-seed")
		}
	}
	expectedRelation, err := recovery.BuildRecoveryRelationReportWithCausalEffectEvidence(seed, execution, relation.CausalEffectEvidence)
	if err != nil || !reflect.DeepEqual(relation, expectedRelation) {
		return invalid("relation-report-is-not-derived-from-recovery-execution")
	}
	passiveObservationID, passiveProbeMode, err := stateFuzzRelationObservationScope(execution)
	if err != nil {
		return invalid("recovery-controls-do-not-share-observation-scope")
	}
	if relation.Controls[0].CheckpointID != execution.Before.CheckpointID || relation.Controls[1].CheckpointID != execution.After.CheckpointID || relation.Controls[2].CheckpointID != execution.Head.CheckpointID {
		return invalid("relation-controls-do-not-match-recovery-execution")
	}
	runtimeImageID := ""
	if profile.RetainedRuntime != nil {
		runtimeImageID = profile.RetainedRuntime.ContainerImageID
	}
	scope := StateFuzzRelationBatchScope{
		ObjectiveID:          stateObjective.ObjectiveID,
		TargetID:             seed.TargetID,
		AdapterID:            seed.AdapterID,
		FrontierID:           seed.FrontierID,
		EffectScope:          canonicalStateFuzzEffects(seed.ValidatedEffects),
		PassiveObservationID: passiveObservationID,
		PassiveProbeMode:     passiveProbeMode,
		RuntimeImageID:       runtimeImageID,
	}
	if err := scope.Validate(); err != nil {
		return invalid("invalid-relation-scope")
	}
	causalStatus := recovery.CausalEffectEvidenceUnknown
	if relation.CausalEffectEvidence != nil {
		causalStatus = relation.CausalEffectEvidence.Status
	}
	entry.RelationStatus = StateFuzzRelationBatchAggregated
	entry.Scope = &scope
	entry.BeforeRelation = relation.Controls[0].Relation.Signature
	entry.AfterRelation = relation.Controls[1].Relation.Signature
	entry.HeadRelation = relation.Controls[2].Relation.Signature
	entry.CompleteThreeControlSet = relation.Controls[0].Evidence.Status == recovery.RecoveryEvidenceComplete && relation.Controls[1].Evidence.Status == recovery.RecoveryEvidenceComplete && relation.Controls[2].Evidence.Status == recovery.RecoveryEvidenceComplete
	entry.HeadConsistent = execution.Classification.HeadOutcome == "consistent"
	entry.CausalEffectEvidence = causalStatus
	entry.ContractStatus = relation.Contract.Status
	entry.RuntimeImageID = scope.RuntimeImageID
	return entry
}

func stateFuzzRelationObservationScope(execution recovery.ForkRecoverySetExecution) (string, string, error) {
	observations := []recovery.RecoveryObservation{execution.Before, execution.After, execution.Head}
	passiveObservationID := strings.TrimSpace(observations[0].PassiveObservationID)
	if passiveObservationID == "" {
		return "", "", fmt.Errorf("missing passive observation ID")
	}
	probeMode := ""
	hasProbeMode := false
	for _, observation := range observations {
		if observation.PassiveObservationID != passiveObservationID {
			return "", "", fmt.Errorf("mixed passive observation IDs")
		}
		mode := ""
		if observation.PassiveProbe != nil {
			mode = string(observation.PassiveProbe.Mode)
		}
		if hasProbeMode && mode != probeMode {
			return "", "", fmt.Errorf("mixed passive probe modes")
		}
		probeMode = mode
		hasProbeMode = true
	}
	return passiveObservationID, probeMode, nil
}

func canonicalStateFuzzEffects(effects []objective.EffectAtom) []objective.EffectAtom {
	result := append([]objective.EffectAtom(nil), effects...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Family == result[right].Family {
			return result[left].Operation < result[right].Operation
		}
		return result[left].Family < result[right].Family
	})
	return result
}

func (r *StateFuzzRelationBatchReport) rebuildAggregates() error {
	r.AttemptCount = len(r.Attempts)
	r.AcceptedCount = 0
	r.AggregatedRelationCount = 0
	r.InvalidRelationArtifactCount = 0
	r.NotEligibleAttemptCount = 0
	r.CompleteThreeControlSetCount = 0
	r.HeadConsistentCount = 0
	r.AttemptStatusCounts = make(map[string]int)
	r.RelationStatusCounts = make(map[string]int)
	r.CausalEffectEvidenceCounts = make(map[string]int)
	r.ContractStatusCounts = make(map[string]int)
	r.RuntimeImageIDCounts = make(map[string]int)
	vectors := make(map[string]StateFuzzRelationVectorCount)
	for _, entry := range r.Attempts {
		if !validStateFuzzBatchStatus(entry.StateFuzzStatus) || !entry.RelationStatus.Valid() {
			return fmt.Errorf("StateFuzz relation batch has an unsupported attempt status")
		}
		r.AttemptStatusCounts[string(entry.StateFuzzStatus)]++
		r.RelationStatusCounts[string(entry.RelationStatus)]++
		if entry.StateFuzzStatus == StateFuzzBatchAccepted {
			r.AcceptedCount++
		}
		switch entry.RelationStatus {
		case StateFuzzRelationBatchNotEligible:
			r.NotEligibleAttemptCount++
		case StateFuzzRelationBatchInvalidRelationArtifact:
			r.InvalidRelationArtifactCount++
		case StateFuzzRelationBatchAggregated:
			if entry.Scope == nil {
				return fmt.Errorf("aggregated StateFuzz relation batch attempt lacks scope")
			}
			r.AggregatedRelationCount++
			if entry.CompleteThreeControlSet {
				r.CompleteThreeControlSetCount++
			}
			if entry.HeadConsistent {
				r.HeadConsistentCount++
			}
			r.CausalEffectEvidenceCounts[string(entry.CausalEffectEvidence)]++
			r.ContractStatusCounts[string(entry.ContractStatus)]++
			if entry.RuntimeImageID != "" {
				r.RuntimeImageIDCounts[entry.RuntimeImageID]++
			}
			vector := StateFuzzRelationVectorCount{
				Scope:          *entry.Scope,
				BeforeRelation: entry.BeforeRelation,
				AfterRelation:  entry.AfterRelation,
				HeadRelation:   entry.HeadRelation,
				Count:          1,
			}
			key := vector.key()
			if existing, ok := vectors[key]; ok {
				existing.Count++
				vectors[key] = existing
			} else {
				vectors[key] = vector
			}
		}
	}
	r.RelationVectors = make([]StateFuzzRelationVectorCount, 0, len(vectors))
	for _, vector := range vectors {
		r.RelationVectors = append(r.RelationVectors, vector)
	}
	sort.Slice(r.RelationVectors, func(left, right int) bool {
		return r.RelationVectors[left].key() < r.RelationVectors[right].key()
	})
	return nil
}

func (r StateFuzzRelationBatchReport) Validate() error {
	if r.SchemaVersion != StateFuzzRelationBatchReportSchema || strings.TrimSpace(r.ObjectiveID) == "" || strings.TrimSpace(r.BatchRoot) == "" || r.AttemptStatusCounts == nil || r.RelationStatusCounts == nil || r.CausalEffectEvidenceCounts == nil || r.ContractStatusCounts == nil || r.RuntimeImageIDCounts == nil {
		return fmt.Errorf("StateFuzz relation batch report is incomplete")
	}
	expected := r
	if err := expected.rebuildAggregates(); err != nil {
		return err
	}
	for _, entry := range r.Attempts {
		if strings.TrimSpace(entry.ArtifactRoot) == "" || entry.Attempt < -1 {
			return fmt.Errorf("StateFuzz relation batch attempt is incomplete")
		}
		switch entry.RelationStatus {
		case StateFuzzRelationBatchAggregated:
			if entry.StateFuzzStatus != StateFuzzBatchAccepted || strings.TrimSpace(entry.Reason) != "" || entry.Scope == nil || !entry.CausalEffectEvidence.Valid() || !entry.ContractStatus.Valid() || strings.TrimSpace(entry.BeforeRelation) == "" || strings.TrimSpace(entry.AfterRelation) == "" || strings.TrimSpace(entry.HeadRelation) == "" {
				return fmt.Errorf("aggregated StateFuzz relation batch attempt is incomplete")
			}
			if err := entry.Scope.Validate(); err != nil {
				return err
			}
		case StateFuzzRelationBatchInvalidRelationArtifact:
			if entry.StateFuzzStatus != StateFuzzBatchAccepted || strings.TrimSpace(entry.Reason) == "" || entry.Scope != nil {
				return fmt.Errorf("invalid StateFuzz relation artifact attempt is inconsistent")
			}
		case StateFuzzRelationBatchNotEligible:
			if entry.StateFuzzStatus == StateFuzzBatchAccepted || entry.Scope != nil {
				return fmt.Errorf("ineligible StateFuzz relation attempt is inconsistent")
			}
		default:
			return fmt.Errorf("unsupported StateFuzz relation batch status %q", entry.RelationStatus)
		}
	}
	for _, vector := range r.RelationVectors {
		if err := vector.Validate(); err != nil {
			return err
		}
	}
	if r.AttemptCount != expected.AttemptCount || r.AcceptedCount != expected.AcceptedCount || r.AggregatedRelationCount != expected.AggregatedRelationCount || r.InvalidRelationArtifactCount != expected.InvalidRelationArtifactCount || r.NotEligibleAttemptCount != expected.NotEligibleAttemptCount || r.CompleteThreeControlSetCount != expected.CompleteThreeControlSetCount || r.HeadConsistentCount != expected.HeadConsistentCount || !reflect.DeepEqual(r.AttemptStatusCounts, expected.AttemptStatusCounts) || !reflect.DeepEqual(r.RelationStatusCounts, expected.RelationStatusCounts) || !reflect.DeepEqual(r.CausalEffectEvidenceCounts, expected.CausalEffectEvidenceCounts) || !reflect.DeepEqual(r.ContractStatusCounts, expected.ContractStatusCounts) || !reflect.DeepEqual(r.RuntimeImageIDCounts, expected.RuntimeImageIDCounts) || !reflect.DeepEqual(r.RelationVectors, expected.RelationVectors) {
		return fmt.Errorf("StateFuzz relation batch report aggregates do not match attempt entries")
	}
	return nil
}

func validStateFuzzBatchStatus(status StateFuzzBatchEntryStatus) bool {
	switch status {
	case StateFuzzBatchAccepted, StateFuzzBatchRejectedEvaluation, StateFuzzBatchRejectedSourceBaseline, StateFuzzBatchRejectedResourceTopology, StateFuzzBatchExecutionFailed, StateFuzzBatchInvalidArtifactRoot:
		return true
	default:
		return false
	}
}
