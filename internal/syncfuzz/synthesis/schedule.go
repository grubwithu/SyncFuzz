package synthesis

import (
	"fmt"
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/coverage"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

// ObjectiveSelection is a scheduler decision. Score is intentionally derived
// only from declared effect atoms and V2 coverage records; it does not score
// testcase names, mutation results, prompt variants, or query lineage.
type ObjectiveSelection struct {
	ObjectiveID      string                 `json:"objective_id"`
	Score            int                    `json:"score"`
	UncoveredEffects []objective.EffectAtom `json:"uncovered_effects"`
	CoveredEffects   []objective.EffectAtom `json:"covered_effects"`
}

type ObjectiveSchedule struct {
	SchemaVersion string               `json:"schema_version"`
	Selections    []ObjectiveSelection `json:"selections"`
}

// ScheduleObjectives prioritizes objectives with atoms that have not appeared
// in the global coverage ledger. Existing coverage lowers priority but never
// suppresses an objective, so repair/reproducibility budgets can still select
// it with a sufficiently large limit.
func ScheduleObjectives(objectives []objective.StateObjective, ledger []coverage.CoverageRecord, limit int) (ObjectiveSchedule, error) {
	if limit < 0 {
		return ObjectiveSchedule{}, fmt.Errorf("synthesis schedule limit must not be negative")
	}
	coverageCounts := make(map[string]int)
	for _, record := range ledger {
		if err := record.Validate(); err != nil {
			return ObjectiveSchedule{}, err
		}
		coverageCounts[effectKey(objective.EffectAtom{Family: record.Family, Operation: record.Operation})]++
	}
	seenObjectives := make(map[string]struct{}, len(objectives))
	selections := make([]ObjectiveSelection, 0, len(objectives))
	for _, stateObjective := range objectives {
		if err := stateObjective.Validate(); err != nil {
			return ObjectiveSchedule{}, err
		}
		if _, exists := seenObjectives[stateObjective.ObjectiveID]; exists {
			return ObjectiveSchedule{}, fmt.Errorf("duplicate synthesis objective %q", stateObjective.ObjectiveID)
		}
		seenObjectives[stateObjective.ObjectiveID] = struct{}{}
		selection := ObjectiveSelection{
			ObjectiveID:      stateObjective.ObjectiveID,
			UncoveredEffects: make([]objective.EffectAtom, 0),
			CoveredEffects:   make([]objective.EffectAtom, 0),
		}
		for _, atom := range stateObjective.CanonicalEffects() {
			count := coverageCounts[effectKey(atom)]
			if count == 0 {
				selection.UncoveredEffects = append(selection.UncoveredEffects, atom)
				selection.Score += 1000
			} else {
				selection.CoveredEffects = append(selection.CoveredEffects, atom)
				// Low-frequency atoms retain an exploration bonus without letting
				// repeatedly observed families dominate the schedule.
				selection.Score += 100 / (count + 1)
			}
		}
		selections = append(selections, selection)
	}
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].Score == selections[j].Score {
			return selections[i].ObjectiveID < selections[j].ObjectiveID
		}
		return selections[i].Score > selections[j].Score
	})
	if limit > 0 && len(selections) > limit {
		selections = selections[:limit]
	}
	return ObjectiveSchedule{SchemaVersion: SchemaVersion, Selections: selections}, nil
}
