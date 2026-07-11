package scheduler

import "github.com/grubwithu/syncfuzz/internal/syncfuzz/target"

func targetMutationFocus(mutations []target.TargetScenarioMutation) (target.TargetScenarioMutation, bool) {
	return target.TargetScenarioMutationFocus(mutations)
}

func targetCandidateMutationFocusID(candidate TargetScheduleCandidate) string {
	if candidate.MutationFocusID != "" {
		return candidate.MutationFocusID
	}
	focus, ok := targetMutationFocus(candidate.Mutations)
	if !ok {
		return ""
	}
	return focus.MutationID
}

func targetCandidateMutationFocusKind(candidate TargetScheduleCandidate) target.TargetScenarioMutationKind {
	if candidate.MutationFocusKind != "" {
		return candidate.MutationFocusKind
	}
	focus, ok := targetMutationFocus(candidate.Mutations)
	if !ok {
		return ""
	}
	return focus.Kind
}
