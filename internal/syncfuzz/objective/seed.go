package objective

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

// StateSeed is automatically promoted from a ProfileRun only after every
// objective atom is backed by a linked effect and a probe-confirmed persistent
// state delta across one frontier.
type StateSeed struct {
	SchemaVersion         string         `json:"schema_version"`
	SeedID                string         `json:"seed_id"`
	ObjectiveID           string         `json:"objective_id"`
	ProfileRunID          string         `json:"profile_run_id"`
	ProfileRunKind        ProfileRunKind `json:"profile_run_kind"`
	SynthesisCandidateID  string         `json:"synthesis_candidate_id"`
	NativeCheckpointRunID string         `json:"native_checkpoint_run_id,omitempty"`
	TargetID              string         `json:"target_id"`
	AdapterID             string         `json:"adapter_id"`
	RecordedPlanID        string         `json:"recorded_plan_id"`
	RecordedPlanArtifact  string         `json:"recorded_plan_artifact"`
	FrontierID            string         `json:"frontier_id"`
	BeforeCheckpointID    string         `json:"before_checkpoint_id"`
	AfterCheckpointID     string         `json:"after_checkpoint_id"`
	ValidatedEffects      []EffectAtom   `json:"validated_effects"`
	ResourceIDs           []string       `json:"resource_ids"`
}

// PromoteStateSeed is deterministic. It refuses manual fixtures, unlinked
// syscall traces, non-frontier intervals, and profile runs that omit a
// recorded execution plan.
func PromoteStateSeed(objective StateObjective, run ProfileRun, frontierID string) (*StateSeed, error) {
	if err := run.ValidateFor(objective); err != nil {
		return nil, err
	}
	if run.Kind != ProfileRunKindSynthesisCandidate {
		return nil, fmt.Errorf("profile run %q is %q and cannot become a StateSeed", run.ProfileRunID, run.Kind)
	}
	frontier, ok := findFrontier(run.CheckpointMap, frontierID)
	if !ok {
		return nil, fmt.Errorf("profile run %q has no frontier %q", run.ProfileRunID, frontierID)
	}
	if !frontier.IsFrontier || len(frontier.EvidenceLinks) == 0 || !frontier.PersistentDelta.Changed() {
		return nil, fmt.Errorf("frontier %q is not backed by linked persistent state evidence", frontierID)
	}

	linkedEffects := make(map[string]struct{}, len(frontier.EvidenceLinks))
	resourceIDs := make(map[string]struct{}, len(frontier.EvidenceLinks))
	for _, link := range frontier.EvidenceLinks {
		linkedEffects[link.EffectID] = struct{}{}
		resourceIDs[link.ResourceID] = struct{}{}
	}
	validated := make([]EffectAtom, 0, len(objective.Effects))
	for _, atom := range objective.CanonicalEffects() {
		if !frontierContainsLinkedAtom(frontier, linkedEffects, atom) {
			return nil, fmt.Errorf("frontier %q does not validate objective atom %s/%s", frontierID, atom.Family, atom.Operation)
		}
		validated = append(validated, atom)
	}
	if len(resourceIDs) == 0 {
		return nil, fmt.Errorf("frontier %q has no linked persistent resource", frontierID)
	}
	resources := make([]string, 0, len(resourceIDs))
	for resourceID := range resourceIDs {
		resources = append(resources, resourceID)
	}
	sort.Strings(resources)
	return &StateSeed{
		SchemaVersion:         SchemaVersion,
		SeedID:                "state-seed:" + run.ProfileRunID + ":" + frontier.FrontierID,
		ObjectiveID:           objective.ObjectiveID,
		ProfileRunID:          run.ProfileRunID,
		ProfileRunKind:        run.Kind,
		SynthesisCandidateID:  run.SynthesisCandidateID,
		NativeCheckpointRunID: run.NativeCheckpointRunID,
		TargetID:              run.TargetID,
		AdapterID:             run.AdapterID,
		RecordedPlanID:        run.RecordedPlanID,
		RecordedPlanArtifact:  run.RecordedPlanArtifact,
		FrontierID:            frontier.FrontierID,
		BeforeCheckpointID:    frontier.BeforeCheckpointID,
		AfterCheckpointID:     frontier.AfterCheckpointID,
		ValidatedEffects:      validated,
		ResourceIDs:           resources,
	}, nil
}

func (s StateSeed) ValidateFor(objective StateObjective) error {
	if err := objective.Validate(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if s.ObjectiveID != objective.ObjectiveID {
		return fmt.Errorf("state seed objective %q does not match %q", s.ObjectiveID, objective.ObjectiveID)
	}
	if len(s.ValidatedEffects) != len(objective.Effects) {
		return fmt.Errorf("state seed %q does not carry every objective atom", s.SeedID)
	}
	expected := make(map[string]struct{}, len(objective.Effects))
	for _, atom := range objective.Effects {
		expected[effectAtomKey(atom)] = struct{}{}
	}
	for _, atom := range s.ValidatedEffects {
		key := effectAtomKey(atom)
		if _, ok := expected[key]; !ok {
			return fmt.Errorf("state seed %q carries unexpected objective atom %s/%s", s.SeedID, atom.Family, atom.Operation)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		return fmt.Errorf("state seed %q omits an objective atom", s.SeedID)
	}
	return nil
}

// Validate checks the self-contained seed invariants for downstream recovery
// code, which intentionally need not load the human-maintained objective.
func (s StateSeed) Validate() error {
	if s.SchemaVersion != "" && s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported state seed schema %q", s.SchemaVersion)
	}
	if strings.TrimSpace(s.SeedID) == "" || strings.TrimSpace(s.ProfileRunID) == "" {
		return fmt.Errorf("state seed requires seed and profile run IDs")
	}
	if s.ProfileRunKind != ProfileRunKindSynthesisCandidate {
		return fmt.Errorf("state seed %q must derive from a synthesis-candidate profile run", s.SeedID)
	}
	if strings.TrimSpace(s.SynthesisCandidateID) == "" {
		return fmt.Errorf("state seed %q lacks a scheduler-issued synthesis candidate ID", s.SeedID)
	}
	if strings.TrimSpace(s.TargetID) == "" || strings.TrimSpace(s.AdapterID) == "" || strings.TrimSpace(s.RecordedPlanID) == "" || strings.TrimSpace(s.RecordedPlanArtifact) == "" {
		return fmt.Errorf("state seed %q lacks execution identity", s.SeedID)
	}
	if strings.TrimSpace(s.FrontierID) == "" || strings.TrimSpace(s.BeforeCheckpointID) == "" || strings.TrimSpace(s.AfterCheckpointID) == "" || s.BeforeCheckpointID == s.AfterCheckpointID {
		return fmt.Errorf("state seed %q lacks a valid checkpoint frontier", s.SeedID)
	}
	if len(s.ValidatedEffects) == 0 {
		return fmt.Errorf("state seed %q has no validated effect atoms", s.SeedID)
	}
	if len(s.ResourceIDs) == 0 {
		return fmt.Errorf("state seed %q has no validated resources", s.SeedID)
	}
	return nil
}

func effectAtomKey(atom EffectAtom) string {
	return string(atom.Family) + "\x00" + atom.Operation
}

func findFrontier(effectMap profiling.CheckpointEffectMap, frontierID string) (profiling.CheckpointInterval, bool) {
	for _, frontier := range effectMap.Intervals {
		if frontier.FrontierID == frontierID {
			return frontier, true
		}
	}
	return profiling.CheckpointInterval{}, false
}

func frontierContainsLinkedAtom(frontier profiling.CheckpointInterval, linkedEffects map[string]struct{}, atom EffectAtom) bool {
	for _, effect := range frontier.Effects {
		if _, ok := linkedEffects[effect.EffectID]; !ok {
			continue
		}
		if effect.Family == atom.Family && effect.Operation == atom.Operation {
			return true
		}
	}
	return false
}
