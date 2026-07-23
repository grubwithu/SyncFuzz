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

// RecordedPlan fixes every execution condition shared by both members of a
// recovery pair. The execution adapter in V2.3 consumes this opaque record;
// it is intentionally not a legacy TargetScenarioExecutionPlan.
type RecordedPlan struct {
	SchemaVersion        string `json:"schema_version"`
	RecordedPlanID       string `json:"recorded_plan_id"`
	AdapterID            string `json:"adapter_id"`
	TargetID             string `json:"target_id"`
	ExecutionArtifact    string `json:"execution_artifact"`
	PassiveObservationID string `json:"passive_observation_id"`
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
	return nil
}

type RecoveryQuery struct {
	QueryID              string   `json:"query_id"`
	SeedID               string   `json:"seed_id"`
	Boundary             Boundary `json:"boundary"`
	CheckpointID         string   `json:"checkpoint_id"`
	RecordedPlanID       string   `json:"recorded_plan_id"`
	PassiveObservationID string   `json:"passive_observation_id"`
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
