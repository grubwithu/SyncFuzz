package synthesis

import (
	"fmt"
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

// CandidateEvaluation is derived entirely from a completed profile run. A
// generator's claimed effects do not enter this result.
type CandidateEvaluation struct {
	SchemaVersion        string                 `json:"schema_version"`
	CandidateID          string                 `json:"candidate_id"`
	ProfileRunID         string                 `json:"profile_run_id"`
	ValidatedFrontiers   []string               `json:"validated_frontiers,omitempty"`
	ObservedEffects      []objective.EffectAtom `json:"observed_effects"`
	MissingEffects       []objective.EffectAtom `json:"missing_effects"`
	Feedback             []AtomFeedback         `json:"feedback"`
	EligibleForRetention bool                   `json:"eligible_for_retention"`
}

// EvaluateProfile checks whether each objective atom is linked to a
// probe-confirmed persistent delta at one or more frontiers. It requires the
// scheduler candidate identity carried by the ProfileRun and never reads
// legacy target metadata.
func EvaluateProfile(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun) (CandidateEvaluation, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return CandidateEvaluation{}, err
	}
	if err := run.ValidateFor(stateObjective); err != nil {
		return CandidateEvaluation{}, err
	}
	if run.SynthesisCandidateID != candidate.CandidateID {
		return CandidateEvaluation{}, fmt.Errorf("profile run candidate %q does not match scheduler candidate %q", run.SynthesisCandidateID, candidate.CandidateID)
	}
	if run.TargetID != candidate.TargetID || run.AdapterID != candidate.AdapterID {
		return CandidateEvaluation{}, fmt.Errorf("profile run target identity does not match synthesis candidate %q", candidate.CandidateID)
	}
	frontiers := make([]string, 0)
	observed := make(map[string]objective.EffectAtom)
	for _, interval := range run.CheckpointMap.Intervals {
		if !interval.IsFrontier || !interval.PersistentDelta.Changed() || len(interval.EvidenceLinks) == 0 {
			continue
		}
		linked := make(map[string]struct{}, len(interval.EvidenceLinks))
		for _, link := range interval.EvidenceLinks {
			linked[link.EffectID] = struct{}{}
		}
		foundInFrontier := false
		for _, atom := range stateObjective.CanonicalEffects() {
			if frontierHasLinkedAtom(interval, linked, atom) {
				observed[effectKey(atom)] = atom
				foundInFrontier = true
			}
		}
		if foundInFrontier {
			frontiers = append(frontiers, interval.FrontierID)
		}
	}
	observedEffects := sortedAtoms(observed)
	missingEffects := make([]objective.EffectAtom, 0)
	feedback := make([]AtomFeedback, 0, len(stateObjective.Effects))
	for _, atom := range stateObjective.CanonicalEffects() {
		_, found := observed[effectKey(atom)]
		feedback = append(feedback, AtomFeedback{Family: atom.Family, Operation: atom.Operation, Observed: found, Reason: feedbackReason(found)})
		if !found {
			missingEffects = append(missingEffects, atom)
		}
	}
	sort.Strings(frontiers)
	return CandidateEvaluation{
		SchemaVersion:        SchemaVersion,
		CandidateID:          candidate.CandidateID,
		ProfileRunID:         run.ProfileRunID,
		ValidatedFrontiers:   frontiers,
		ObservedEffects:      observedEffects,
		MissingEffects:       missingEffects,
		Feedback:             feedback,
		EligibleForRetention: len(missingEffects) == 0 && len(frontiers) > 0,
	}, nil
}

func frontierHasLinkedAtom(interval profiling.CheckpointInterval, linked map[string]struct{}, atom objective.EffectAtom) bool {
	for _, effect := range interval.Effects {
		if _, ok := linked[effect.EffectID]; ok && effect.Family == atom.Family && effect.Operation == atom.Operation {
			return true
		}
	}
	return false
}

func sortedAtoms(atoms map[string]objective.EffectAtom) []objective.EffectAtom {
	result := make([]objective.EffectAtom, 0, len(atoms))
	for _, atom := range atoms {
		result = append(result, atom)
	}
	sort.Slice(result, func(i, j int) bool { return effectKey(result[i]) < effectKey(result[j]) })
	return result
}

func feedbackReason(observed bool) string {
	if observed {
		return "linked persistent frontier observed"
	}
	return "no linked persistent frontier observed"
}
