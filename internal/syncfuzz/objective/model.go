// Package objective defines the V2 State Fuzzer's stable, non-query IR.
// It deliberately does not import target scenarios, mutation records, or
// prompt variants: objectives describe desired OS state relations, while a
// later synthesis layer is responsible for producing natural tasks.
package objective

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const SchemaVersion = "syncfuzz.objective.v1"

type EffectAtom struct {
	Family    profiling.StateFamily `json:"family"`
	Operation string                `json:"operation"`
}

// StateObjective is an intentionally small declaration of a state relation
// the State Fuzzer may try to form. It contains no concrete path, daemon
// story, prompt, query parent, or target-matrix mutation.
type StateObjective struct {
	SchemaVersion    string       `json:"schema_version"`
	ObjectiveID      string       `json:"objective_id"`
	Effects          []EffectAtom `json:"effects"`
	Lifetime         string       `json:"lifetime"`
	ResourceRelation string       `json:"resource_relation"`
	Persistence      string       `json:"persistence"`
}

func (o StateObjective) Validate() error {
	if o.SchemaVersion != "" && o.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported state objective schema %q", o.SchemaVersion)
	}
	if strings.TrimSpace(o.ObjectiveID) == "" {
		return fmt.Errorf("state objective ID is required")
	}
	if len(o.Effects) == 0 {
		return fmt.Errorf("state objective %q requires at least one effect atom", o.ObjectiveID)
	}
	seen := make(map[string]struct{}, len(o.Effects))
	for _, effect := range o.Effects {
		if !effect.Family.Valid() {
			return fmt.Errorf("state objective %q has invalid effect family %q", o.ObjectiveID, effect.Family)
		}
		if strings.TrimSpace(effect.Operation) == "" {
			return fmt.Errorf("state objective %q has an effect atom with no operation", o.ObjectiveID)
		}
		key := string(effect.Family) + "\x00" + effect.Operation
		if _, ok := seen[key]; ok {
			return fmt.Errorf("state objective %q has duplicate effect atom %s/%s", o.ObjectiveID, effect.Family, effect.Operation)
		}
		seen[key] = struct{}{}
	}
	if strings.TrimSpace(o.Lifetime) == "" {
		return fmt.Errorf("state objective %q requires a lifetime", o.ObjectiveID)
	}
	if strings.TrimSpace(o.ResourceRelation) == "" {
		return fmt.Errorf("state objective %q requires a resource relation", o.ObjectiveID)
	}
	if strings.TrimSpace(o.Persistence) == "" {
		return fmt.Errorf("state objective %q requires a persistence requirement", o.ObjectiveID)
	}
	return nil
}

func (o StateObjective) CanonicalEffects() []EffectAtom {
	effects := append([]EffectAtom{}, o.Effects...)
	sort.Slice(effects, func(i, j int) bool {
		if effects[i].Family == effects[j].Family {
			return effects[i].Operation < effects[j].Operation
		}
		return effects[i].Family < effects[j].Family
	})
	return effects
}

// ProfileRunKind records whether a profile run came from the future task
// synthesis pipeline or is only a calibration/documentation fixture. The
// distinction is mandatory: a real execution alone is not enough to become a
// StateSeed, because V2 coverage must only measure synthesized candidates.
type ProfileRunKind string

const (
	ProfileRunKindSynthesisCandidate ProfileRunKind = "synthesis-candidate"
	ProfileRunKindCalibrationFixture ProfileRunKind = "calibration-fixture"
)

func (k ProfileRunKind) Valid() bool {
	return k == ProfileRunKindSynthesisCandidate || k == ProfileRunKindCalibrationFixture
}

// ProfileRun is the validated evidence input to StateSeed promotion. Its
// recorded-plan ID is opaque at this stage; V2.3 supplies the executor.
type ProfileRun struct {
	SchemaVersion         string                        `json:"schema_version"`
	ProfileRunID          string                        `json:"profile_run_id"`
	Kind                  ProfileRunKind                `json:"kind"`
	SynthesisCandidateID  string                        `json:"synthesis_candidate_id,omitempty"`
	NativeCheckpointRunID string                        `json:"native_checkpoint_run_id,omitempty"`
	ObjectiveID           string                        `json:"objective_id"`
	TargetID              string                        `json:"target_id"`
	AdapterID             string                        `json:"adapter_id"`
	RecordedPlanID        string                        `json:"recorded_plan_id"`
	RecordedPlanArtifact  string                        `json:"recorded_plan_artifact"`
	CheckpointMap         profiling.CheckpointEffectMap `json:"checkpoint_effect_map"`
}

func (r ProfileRun) ValidateFor(objective StateObjective) error {
	if err := objective.Validate(); err != nil {
		return err
	}
	if r.SchemaVersion != "" && r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported profile run schema %q", r.SchemaVersion)
	}
	if strings.TrimSpace(r.ProfileRunID) == "" {
		return fmt.Errorf("profile run ID is required")
	}
	if !r.Kind.Valid() {
		return fmt.Errorf("profile run %q has invalid or missing kind %q", r.ProfileRunID, r.Kind)
	}
	if r.Kind == ProfileRunKindSynthesisCandidate && strings.TrimSpace(r.SynthesisCandidateID) == "" {
		return fmt.Errorf("synthesis profile run %q requires a scheduler-issued synthesis candidate ID", r.ProfileRunID)
	}
	if r.Kind == ProfileRunKindCalibrationFixture && strings.TrimSpace(r.SynthesisCandidateID) != "" {
		return fmt.Errorf("calibration profile run %q must not carry a synthesis candidate ID", r.ProfileRunID)
	}
	if r.ObjectiveID != objective.ObjectiveID {
		return fmt.Errorf("profile run objective %q does not match %q", r.ObjectiveID, objective.ObjectiveID)
	}
	if strings.TrimSpace(r.TargetID) == "" || strings.TrimSpace(r.AdapterID) == "" {
		return fmt.Errorf("profile run %q requires target and adapter IDs", r.ProfileRunID)
	}
	if strings.TrimSpace(r.RecordedPlanID) == "" || strings.TrimSpace(r.RecordedPlanArtifact) == "" {
		return fmt.Errorf("profile run %q requires a recorded plan ID and artifact", r.ProfileRunID)
	}
	if err := r.CheckpointMap.Validate(); err != nil {
		return fmt.Errorf("profile run %q checkpoint map: %w", r.ProfileRunID, err)
	}
	if r.CheckpointMap.RunID == "" {
		return fmt.Errorf("profile run %q checkpoint map has no run ID", r.ProfileRunID)
	}
	return nil
}
