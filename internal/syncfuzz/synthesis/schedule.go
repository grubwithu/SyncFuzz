package synthesis

import (
	"fmt"
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/coverage"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

// ObjectiveSelection is a scheduler decision. Scores are derived from declared
// effect atoms, V2 coverage, and optionally normalized recovery relations. It
// does not score testcase names, mutation results, prompt variants, task text,
// contracts, or query lineage.
type ObjectiveSelection struct {
	ObjectiveID                 string                 `json:"objective_id"`
	Score                       int                    `json:"score"`
	EffectCoverageScore         int                    `json:"effect_coverage_score"`
	RelationNoveltyScore        int                    `json:"relation_novelty_score"`
	RelationCoverageKnown       bool                   `json:"relation_coverage_known"`
	ProvenRelationTuples        int                    `json:"proven_relation_tuples"`
	UnknownCausalRelationTuples int                    `json:"unknown_causal_relation_tuples"`
	UncoveredEffects            []objective.EffectAtom `json:"uncovered_effects"`
	CoveredEffects              []objective.EffectAtom `json:"covered_effects"`
}

type ObjectiveSchedule struct {
	SchemaVersion string               `json:"schema_version"`
	Selections    []ObjectiveSelection `json:"selections"`
}

// ScheduleObjectives preserves the V2 atom-only scheduling interface. Call
// ScheduleObjectivesWithRelationNovelty when a complete relation-novelty
// ledger is available.
func ScheduleObjectives(objectives []objective.StateObjective, ledger []coverage.CoverageRecord, limit int) (ObjectiveSchedule, error) {
	return ScheduleObjectivesWithRelationNovelty(objectives, ledger, nil, limit)
}

// ScheduleObjectivesWithRelationNovelty prioritizes objectives with atoms that
// have not appeared in the global coverage ledger. When a relation ledger is
// supplied, it adds a smaller exploration signal for objective effect scopes
// with no, or few, causally proven relation tuples. Unknown causal tuples are
// reported but deliberately do not reduce that exploration signal.
//
// A relation tuple is selected only by its canonical effect scope. The
// scheduler does not inspect candidate task text, tool-call identity, command
// hashes, session identity, or contract status.
func ScheduleObjectivesWithRelationNovelty(objectives []objective.StateObjective, ledger []coverage.CoverageRecord, relationLedger *coverage.RelationNoveltyLedger, limit int) (ObjectiveSchedule, error) {
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
	relationCoverage := make(map[string]relationScopeCoverage)
	if relationLedger != nil {
		if err := relationLedger.Validate(); err != nil {
			return ObjectiveSchedule{}, err
		}
		relationCoverage = relationCoverageByScope(*relationLedger)
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
				selection.EffectCoverageScore += 1000
			} else {
				selection.CoveredEffects = append(selection.CoveredEffects, atom)
				// Low-frequency atoms retain an exploration bonus without letting
				// repeatedly observed families dominate the schedule.
				selection.EffectCoverageScore += 100 / (count + 1)
			}
		}
		selection.Score = selection.EffectCoverageScore
		if relationLedger != nil {
			selection.RelationCoverageKnown = true
			counts := relationCoverage[effectScopeKey(stateObjective.CanonicalEffects())]
			selection.ProvenRelationTuples = counts.proven
			selection.UnknownCausalRelationTuples = counts.unknown
			if counts.proven == 0 {
				// Relation evidence should help explore fully atom-covered
				// objectives, but must not outweigh a missing effect atom.
				selection.RelationNoveltyScore = 250
			} else {
				selection.RelationNoveltyScore = 100 / (counts.proven + 1)
			}
			selection.Score += selection.RelationNoveltyScore
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

type relationScopeCoverage struct {
	proven  int
	unknown int
}

func relationCoverageByScope(ledger coverage.RelationNoveltyLedger) map[string]relationScopeCoverage {
	proven := make(map[string]map[string]struct{})
	unknown := make(map[string]map[string]struct{})
	for _, record := range ledger.Records {
		scope := effectScopeKey(record.EffectScope)
		var tuples map[string]map[string]struct{}
		switch record.CausalEffectEvidenceStatus {
		case recovery.CausalEffectEvidenceProven:
			tuples = proven
		case recovery.CausalEffectEvidenceUnknown:
			tuples = unknown
		default:
			continue
		}
		if tuples[scope] == nil {
			tuples[scope] = make(map[string]struct{})
		}
		tuples[scope][record.TupleKey()] = struct{}{}
	}
	result := make(map[string]relationScopeCoverage, len(proven)+len(unknown))
	for scope, tuples := range proven {
		counts := result[scope]
		counts.proven = len(tuples)
		result[scope] = counts
	}
	for scope, tuples := range unknown {
		counts := result[scope]
		counts.unknown = len(tuples)
		result[scope] = counts
	}
	return result
}

func effectScopeKey(effects []objective.EffectAtom) string {
	canonical := append([]objective.EffectAtom(nil), effects...)
	sort.Slice(canonical, func(left, right int) bool {
		if canonical[left].Family == canonical[right].Family {
			return canonical[left].Operation < canonical[right].Operation
		}
		return canonical[left].Family < canonical[right].Family
	})
	key := ""
	for _, effect := range canonical {
		key += string(effect.Family) + "\x00" + effect.Operation + "\x00"
	}
	return key
}
