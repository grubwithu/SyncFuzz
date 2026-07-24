// Package recovery defines V2's checkpoint-coordinate recovery IR. The first
// boundary is fork only; replay and rewind are deliberately not represented as
// interchangeable query dimensions yet.
package recovery

import (
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

const SchemaVersion = "syncfuzz.recovery.v1"

type Boundary string

const BoundaryFork Boundary = "fork"

// RetentionPolicy records the OS-state condition under which a historical
// logical checkpoint is restored. Recovery mechanisms are only comparable
// when this condition is identical.
type RetentionPolicy string

const RetentionPolicyRetainRelevantOSState RetentionPolicy = "retain-relevant-os-state"

func (p RetentionPolicy) Valid() bool {
	return p == RetentionPolicyRetainRelevantOSState
}

// MaterializationHead is the independently observed post-frontier state on
// which every query in a recovery set is materialized before logical recovery.
// It is intentionally distinct from the frontier's before/after coordinates.
type MaterializationHead struct {
	HeadID              string   `json:"head_id"`
	ProfileRunID        string   `json:"profile_run_id"`
	CheckpointID        string   `json:"checkpoint_id"`
	MonotonicNS         uint64   `json:"monotonic_ns"`
	RetainedResourceIDs []string `json:"retained_resource_ids"`
}

func MaterializationHeadFor(seed objective.StateSeed) (MaterializationHead, error) {
	if err := seed.Validate(); err != nil {
		return MaterializationHead{}, err
	}
	if strings.TrimSpace(seed.MaterializationHeadCheckpointID) == "" || seed.MaterializationHeadMonotonicNS == 0 || len(seed.MaterializationHeadResourceIDs) == 0 {
		return MaterializationHead{}, fmt.Errorf("state seed %q has no materialization-head persistence evidence", seed.SeedID)
	}
	head := MaterializationHead{
		HeadID:              "materialization-head:" + seed.ProfileRunID + ":" + seed.MaterializationHeadCheckpointID,
		ProfileRunID:        seed.ProfileRunID,
		CheckpointID:        seed.MaterializationHeadCheckpointID,
		MonotonicNS:         seed.MaterializationHeadMonotonicNS,
		RetainedResourceIDs: append([]string(nil), seed.MaterializationHeadResourceIDs...),
	}
	if err := head.ValidateFor(seed); err != nil {
		return MaterializationHead{}, err
	}
	return head, nil
}

func (h MaterializationHead) ValidateFor(seed objective.StateSeed) error {
	if strings.TrimSpace(h.HeadID) == "" || h.ProfileRunID != seed.ProfileRunID || strings.TrimSpace(h.CheckpointID) == "" || h.MonotonicNS == 0 || len(h.RetainedResourceIDs) == 0 {
		return fmt.Errorf("materialization head is incomplete for state seed %q", seed.SeedID)
	}
	if h.CheckpointID != seed.MaterializationHeadCheckpointID || h.MonotonicNS != seed.MaterializationHeadMonotonicNS || h.CheckpointID == seed.BeforeCheckpointID || h.CheckpointID == seed.AfterCheckpointID {
		return fmt.Errorf("materialization head does not match state seed %q", seed.SeedID)
	}
	if len(h.RetainedResourceIDs) != len(seed.MaterializationHeadResourceIDs) {
		return fmt.Errorf("materialization head does not retain the state seed resource set")
	}
	seen := make(map[string]struct{}, len(h.RetainedResourceIDs))
	for _, resourceID := range h.RetainedResourceIDs {
		if strings.TrimSpace(resourceID) == "" {
			return fmt.Errorf("materialization head has an empty retained resource ID")
		}
		seen[resourceID] = struct{}{}
	}
	for _, resourceID := range seed.MaterializationHeadResourceIDs {
		if _, ok := seen[resourceID]; !ok {
			return fmt.Errorf("materialization head omits retained resource %q", resourceID)
		}
	}
	return nil
}

// RecordedPlan fixes every execution condition shared by both members of a
// recovery pair. The execution adapter in V2.3 consumes this opaque record;
// it is intentionally not a legacy TargetScenarioExecutionPlan.
type RecordedPlan struct {
	SchemaVersion         string          `json:"schema_version"`
	RecordedPlanID        string          `json:"recorded_plan_id"`
	AdapterID             string          `json:"adapter_id"`
	TargetID              string          `json:"target_id"`
	ExecutionArtifact     string          `json:"execution_artifact"`
	PassiveObservationID  string          `json:"passive_observation_id"`
	MaterializationHeadID string          `json:"materialization_head_id,omitempty"`
	RetentionPolicy       RetentionPolicy `json:"retention_policy,omitempty"`
}

func (p RecordedPlan) ValidateFor(seed objective.StateSeed) error {
	if err := seed.Validate(); err != nil {
		return err
	}
	if p.SchemaVersion != "" && p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported recorded plan schema %q", p.SchemaVersion)
	}
	if strings.TrimSpace(p.RecordedPlanID) == "" || strings.TrimSpace(p.ExecutionArtifact) == "" || strings.TrimSpace(p.PassiveObservationID) == "" {
		return fmt.Errorf("recorded plan requires ID, execution artifact, and passive observation")
	}
	if p.RecordedPlanID != seed.RecordedPlanID || p.ExecutionArtifact != seed.RecordedPlanArtifact || p.AdapterID != seed.AdapterID || p.TargetID != seed.TargetID {
		return fmt.Errorf("recorded plan %q does not match state seed execution identity", p.RecordedPlanID)
	}
	if (strings.TrimSpace(p.MaterializationHeadID) == "") != (p.RetentionPolicy == "") {
		return fmt.Errorf("recorded plan must record both materialization head and retention policy together")
	}
	if p.RetentionPolicy != "" && !p.RetentionPolicy.Valid() {
		return fmt.Errorf("recorded plan has invalid retention policy %q", p.RetentionPolicy)
	}
	return nil
}

type RecoveryQuery struct {
	QueryID               string          `json:"query_id"`
	SeedID                string          `json:"seed_id"`
	Boundary              Boundary        `json:"boundary"`
	CheckpointID          string          `json:"checkpoint_id"`
	RecordedPlanID        string          `json:"recorded_plan_id"`
	PassiveObservationID  string          `json:"passive_observation_id"`
	MaterializationHeadID string          `json:"materialization_head_id,omitempty"`
	RetentionPolicy       RetentionPolicy `json:"retention_policy,omitempty"`
}

// RecoveryPair holds exactly two observations whose checkpoint coordinate is
// the only permitted difference.
type RecoveryPair struct {
	SchemaVersion        string        `json:"schema_version"`
	ComparisonPairID     string        `json:"comparison_pair_id"`
	SeedID               string        `json:"seed_id"`
	FrontierID           string        `json:"frontier_id"`
	Boundary             Boundary      `json:"boundary"`
	RecordedPlanID       string        `json:"recorded_plan_id"`
	PassiveObservationID string        `json:"passive_observation_id"`
	Before               RecoveryQuery `json:"before"`
	After                RecoveryQuery `json:"after"`
}

// HistoricalRecoverySet is the complete V2 recovery experiment. Before,
// after, and head use the same materialized OS state, recorded plan, passive
// observation, and retention policy; the restored logical coordinate is the
// only discovery variable.
type HistoricalRecoverySet struct {
	SchemaVersion        string              `json:"schema_version"`
	RecoverySetID        string              `json:"recovery_set_id"`
	SeedID               string              `json:"seed_id"`
	FrontierID           string              `json:"frontier_id"`
	Boundary             Boundary            `json:"boundary"`
	RecordedPlanID       string              `json:"recorded_plan_id"`
	PassiveObservationID string              `json:"passive_observation_id"`
	MaterializationHead  MaterializationHead `json:"materialization_head"`
	RetentionPolicy      RetentionPolicy     `json:"retention_policy"`
	Before               RecoveryQuery       `json:"before"`
	After                RecoveryQuery       `json:"after"`
	Head                 RecoveryQuery       `json:"head"`
}

func NewForkRecoverySet(seed objective.StateSeed, plan RecordedPlan) (*HistoricalRecoverySet, error) {
	if err := plan.ValidateFor(seed); err != nil {
		return nil, err
	}
	head, err := MaterializationHeadFor(seed)
	if err != nil {
		return nil, err
	}
	if plan.MaterializationHeadID != head.HeadID || plan.RetentionPolicy != RetentionPolicyRetainRelevantOSState {
		return nil, fmt.Errorf("recorded plan must freeze materialization head %q and retention policy %q", head.HeadID, RetentionPolicyRetainRelevantOSState)
	}
	newQuery := func(checkpointID string) RecoveryQuery {
		return RecoveryQuery{
			QueryID:               "recovery-query:" + seed.SeedID + ":" + checkpointID,
			SeedID:                seed.SeedID,
			Boundary:              BoundaryFork,
			CheckpointID:          checkpointID,
			RecordedPlanID:        plan.RecordedPlanID,
			PassiveObservationID:  plan.PassiveObservationID,
			MaterializationHeadID: head.HeadID,
			RetentionPolicy:       plan.RetentionPolicy,
		}
	}
	set := &HistoricalRecoverySet{
		SchemaVersion:        SchemaVersion,
		RecoverySetID:        "historical-recovery-set:" + seed.SeedID + ":" + seed.FrontierID,
		SeedID:               seed.SeedID,
		FrontierID:           seed.FrontierID,
		Boundary:             BoundaryFork,
		RecordedPlanID:       plan.RecordedPlanID,
		PassiveObservationID: plan.PassiveObservationID,
		MaterializationHead:  head,
		RetentionPolicy:      plan.RetentionPolicy,
		Before:               newQuery(seed.BeforeCheckpointID),
		After:                newQuery(seed.AfterCheckpointID),
		Head:                 newQuery(head.CheckpointID),
	}
	if err := set.ValidateFor(seed); err != nil {
		return nil, err
	}
	return set, nil
}

func (s HistoricalRecoverySet) ValidateFor(seed objective.StateSeed) error {
	if err := seed.Validate(); err != nil {
		return err
	}
	if s.SchemaVersion != "" && s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported historical recovery set schema %q", s.SchemaVersion)
	}
	if strings.TrimSpace(s.RecoverySetID) == "" || s.SeedID != seed.SeedID || s.FrontierID != seed.FrontierID || s.Boundary != BoundaryFork || strings.TrimSpace(s.RecordedPlanID) == "" || strings.TrimSpace(s.PassiveObservationID) == "" || s.RetentionPolicy != RetentionPolicyRetainRelevantOSState {
		return fmt.Errorf("historical recovery set does not preserve the shared recovery identity")
	}
	if err := s.MaterializationHead.ValidateFor(seed); err != nil {
		return err
	}
	if err := validateSetQuery(s.Before, seed, s, seed.BeforeCheckpointID); err != nil {
		return fmt.Errorf("before query: %w", err)
	}
	if err := validateSetQuery(s.After, seed, s, seed.AfterCheckpointID); err != nil {
		return fmt.Errorf("after query: %w", err)
	}
	if err := validateSetQuery(s.Head, seed, s, s.MaterializationHead.CheckpointID); err != nil {
		return fmt.Errorf("head query: %w", err)
	}
	if s.Before.QueryID == s.After.QueryID || s.Before.QueryID == s.Head.QueryID || s.After.QueryID == s.Head.QueryID {
		return fmt.Errorf("historical recovery set requires three distinct query coordinates")
	}
	return nil
}

func validateSetQuery(query RecoveryQuery, seed objective.StateSeed, set HistoricalRecoverySet, expectedCheckpoint string) error {
	if strings.TrimSpace(query.QueryID) == "" || query.SeedID != seed.SeedID || query.Boundary != BoundaryFork || query.CheckpointID != expectedCheckpoint {
		return fmt.Errorf("query lacks the shared seed/fork/checkpoint identity")
	}
	if query.RecordedPlanID != set.RecordedPlanID || query.PassiveObservationID != set.PassiveObservationID || query.MaterializationHeadID != set.MaterializationHead.HeadID || query.RetentionPolicy != set.RetentionPolicy {
		return fmt.Errorf("query changes a recovery condition other than checkpoint")
	}
	return nil
}

func NewForkPair(seed objective.StateSeed, plan RecordedPlan) (*RecoveryPair, error) {
	if err := plan.ValidateFor(seed); err != nil {
		return nil, err
	}
	pair := &RecoveryPair{
		SchemaVersion:        SchemaVersion,
		ComparisonPairID:     "recovery-pair:" + seed.SeedID + ":" + seed.FrontierID,
		SeedID:               seed.SeedID,
		FrontierID:           seed.FrontierID,
		Boundary:             BoundaryFork,
		RecordedPlanID:       plan.RecordedPlanID,
		PassiveObservationID: plan.PassiveObservationID,
		Before: RecoveryQuery{
			QueryID:              "recovery-query:" + seed.SeedID + ":" + seed.BeforeCheckpointID,
			SeedID:               seed.SeedID,
			Boundary:             BoundaryFork,
			CheckpointID:         seed.BeforeCheckpointID,
			RecordedPlanID:       plan.RecordedPlanID,
			PassiveObservationID: plan.PassiveObservationID,
		},
		After: RecoveryQuery{
			QueryID:              "recovery-query:" + seed.SeedID + ":" + seed.AfterCheckpointID,
			SeedID:               seed.SeedID,
			Boundary:             BoundaryFork,
			CheckpointID:         seed.AfterCheckpointID,
			RecordedPlanID:       plan.RecordedPlanID,
			PassiveObservationID: plan.PassiveObservationID,
		},
	}
	if err := pair.ValidateFor(seed); err != nil {
		return nil, err
	}
	return pair, nil
}

func (p RecoveryPair) ValidateFor(seed objective.StateSeed) error {
	if err := seed.Validate(); err != nil {
		return err
	}
	if p.SchemaVersion != "" && p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported recovery pair schema %q", p.SchemaVersion)
	}
	if p.Boundary != BoundaryFork {
		return fmt.Errorf("recovery pair %q must use fork in V2.1b", p.ComparisonPairID)
	}
	if strings.TrimSpace(p.ComparisonPairID) == "" || p.SeedID != seed.SeedID || p.FrontierID != seed.FrontierID {
		return fmt.Errorf("recovery pair does not match state seed %q and frontier %q", seed.SeedID, seed.FrontierID)
	}
	if p.RecordedPlanID != seed.RecordedPlanID || strings.TrimSpace(p.PassiveObservationID) == "" {
		return fmt.Errorf("recovery pair %q does not preserve the recorded plan and passive observation", p.ComparisonPairID)
	}
	if err := validatePairQuery(p.Before, seed, p, seed.BeforeCheckpointID); err != nil {
		return fmt.Errorf("before query: %w", err)
	}
	if err := validatePairQuery(p.After, seed, p, seed.AfterCheckpointID); err != nil {
		return fmt.Errorf("after query: %w", err)
	}
	if p.Before.CheckpointID == p.After.CheckpointID || p.Before.QueryID == p.After.QueryID {
		return fmt.Errorf("recovery pair %q does not vary the checkpoint coordinate", p.ComparisonPairID)
	}
	return nil
}

func validatePairQuery(query RecoveryQuery, seed objective.StateSeed, pair RecoveryPair, expectedCheckpoint string) error {
	if strings.TrimSpace(query.QueryID) == "" || query.SeedID != seed.SeedID || query.Boundary != BoundaryFork {
		return fmt.Errorf("query lacks the shared seed/fork identity")
	}
	if query.CheckpointID != expectedCheckpoint {
		return fmt.Errorf("checkpoint %q does not equal required coordinate %q", query.CheckpointID, expectedCheckpoint)
	}
	if query.RecordedPlanID != pair.RecordedPlanID || query.PassiveObservationID != pair.PassiveObservationID {
		return fmt.Errorf("query changes an execution condition other than checkpoint")
	}
	return nil
}
