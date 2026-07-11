package scheduler

import (
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetDimensionCoverageSummary struct {
	Dimension               string   `json:"dimension"`
	TotalValues             int      `json:"total_values"`
	ExecutedValues          int      `json:"executed_values"`
	ConfirmedValues         int      `json:"confirmed_values,omitempty"`
	ActivationReachedValues int      `json:"activation_reached_values,omitempty"`
	MissingValues           []string `json:"missing_values,omitempty"`
}

type TargetDimensionCoverageGainSummary struct {
	Dimension                  string   `json:"dimension"`
	NewExecutedValues          []string `json:"new_executed_values,omitempty"`
	NewConfirmedValues         []string `json:"new_confirmed_values,omitempty"`
	NewActivationReachedValues []string `json:"new_activation_reached_values,omitempty"`
}

type TargetDimensionCoverageGainStats struct {
	NewExecutedCount          int `json:"new_executed_count,omitempty"`
	NewConfirmedCount         int `json:"new_confirmed_count,omitempty"`
	NewActivationReachedCount int `json:"new_activation_reached_count,omitempty"`
	WeightedScore             int `json:"weighted_score,omitempty"`
}

type targetDimensionDescriptor struct {
	name   string
	values func(TargetScheduleCandidate) []string
}

type targetDimensionCoverageValue struct {
	executed          bool
	confirmed         bool
	activationReached bool
}

type targetDimensionExecutionState struct {
	executed          bool
	confirmed         bool
	activationReached bool
}

func summarizeTargetDimensionCoverage(universe []TargetScheduleCandidate, results []TargetSuiteRunResult) []TargetDimensionCoverageSummary {
	if len(universe) == 0 {
		return nil
	}

	descriptors := targetDimensionCoverageDescriptors()
	coverage := collectTargetDimensionCoverage(universe, results)

	out := make([]TargetDimensionCoverageSummary, 0, len(descriptors))
	for _, descriptor := range descriptors {
		values := coverage[descriptor.name]
		if len(values) == 0 {
			continue
		}
		summary := TargetDimensionCoverageSummary{
			Dimension: descriptor.name,
		}
		for value, current := range values {
			summary.TotalValues++
			if current.executed {
				summary.ExecutedValues++
			} else {
				summary.MissingValues = append(summary.MissingValues, value)
			}
			if current.confirmed {
				summary.ConfirmedValues++
			}
			if current.activationReached {
				summary.ActivationReachedValues++
			}
		}
		sort.Strings(summary.MissingValues)
		out = append(out, summary)
	}
	return out
}

func summarizeTargetDimensionCoverageGain(
	universe []TargetScheduleCandidate,
	previousResults []TargetSuiteRunResult,
	currentResults []TargetSuiteRunResult,
) []TargetDimensionCoverageGainSummary {
	if len(universe) == 0 || len(currentResults) == 0 {
		return nil
	}

	descriptors := targetDimensionCoverageDescriptors()
	before := collectTargetDimensionCoverage(universe, previousResults)
	combined := append(append([]TargetSuiteRunResult{}, previousResults...), currentResults...)
	after := collectTargetDimensionCoverage(universe, combined)

	out := make([]TargetDimensionCoverageGainSummary, 0, len(descriptors))
	for _, descriptor := range descriptors {
		gain := TargetDimensionCoverageGainSummary{Dimension: descriptor.name}
		valuesAfter := after[descriptor.name]
		valuesBefore := before[descriptor.name]
		for value, stateAfter := range valuesAfter {
			stateBefore := targetDimensionCoverageValue{}
			if valuesBefore != nil {
				if current, ok := valuesBefore[value]; ok {
					stateBefore = *current
				}
			}
			if !stateBefore.executed && stateAfter.executed {
				gain.NewExecutedValues = append(gain.NewExecutedValues, value)
			}
			if !stateBefore.confirmed && stateAfter.confirmed {
				gain.NewConfirmedValues = append(gain.NewConfirmedValues, value)
			}
			if !stateBefore.activationReached && stateAfter.activationReached {
				gain.NewActivationReachedValues = append(gain.NewActivationReachedValues, value)
			}
		}
		sort.Strings(gain.NewExecutedValues)
		sort.Strings(gain.NewConfirmedValues)
		sort.Strings(gain.NewActivationReachedValues)
		if len(gain.NewExecutedValues) == 0 && len(gain.NewConfirmedValues) == 0 && len(gain.NewActivationReachedValues) == 0 {
			continue
		}
		out = append(out, gain)
	}
	return out
}

func summarizeTargetDimensionCoverageGainStats(gains []TargetDimensionCoverageGainSummary) TargetDimensionCoverageGainStats {
	stats := TargetDimensionCoverageGainStats{}
	for _, gain := range gains {
		weight := targetDimensionGapWeight(gain.Dimension)
		stats.NewExecutedCount += len(gain.NewExecutedValues)
		stats.NewConfirmedCount += len(gain.NewConfirmedValues)
		stats.NewActivationReachedCount += len(gain.NewActivationReachedValues)
		stats.WeightedScore += weight * len(gain.NewExecutedValues)
		stats.WeightedScore += weight * 2 * len(gain.NewConfirmedValues)
		stats.WeightedScore += weight * 3 * len(gain.NewActivationReachedValues)
	}
	return stats
}

func collectTargetDimensionCoverage(universe []TargetScheduleCandidate, results []TargetSuiteRunResult) map[string]map[string]*targetDimensionCoverageValue {
	descriptors := targetDimensionCoverageDescriptors()
	byCandidate := make(map[string]TargetScheduleCandidate, len(universe))
	coverage := make(map[string]map[string]*targetDimensionCoverageValue, len(descriptors))
	for _, descriptor := range descriptors {
		coverage[descriptor.name] = make(map[string]*targetDimensionCoverageValue)
	}

	for _, candidate := range universe {
		if candidate.CandidateID != "" {
			byCandidate[candidate.CandidateID] = candidate
		}
		for _, descriptor := range descriptors {
			for _, value := range descriptor.values(candidate) {
				if value == "" {
					continue
				}
				if _, ok := coverage[descriptor.name][value]; !ok {
					coverage[descriptor.name][value] = &targetDimensionCoverageValue{}
				}
			}
		}
	}

	candidateState := make(map[string]targetDimensionExecutionState)
	for _, result := range results {
		if result.CandidateID == "" {
			continue
		}
		state := candidateState[result.CandidateID]
		state.executed = true
		if result.Confirmed {
			state.confirmed = true
		}
		if result.ActivationStage == TargetActivationStageActivationReached {
			state.activationReached = true
		}
		candidateState[result.CandidateID] = state
	}

	for candidateID, state := range candidateState {
		candidate, ok := byCandidate[candidateID]
		if !ok {
			continue
		}
		for _, descriptor := range descriptors {
			for _, value := range descriptor.values(candidate) {
				if value == "" {
					continue
				}
				current := coverage[descriptor.name][value]
				if current == nil {
					current = &targetDimensionCoverageValue{}
					coverage[descriptor.name][value] = current
				}
				current.executed = current.executed || state.executed
				current.confirmed = current.confirmed || state.confirmed
				current.activationReached = current.activationReached || state.activationReached
			}
		}
	}

	return coverage
}

func targetDimensionCoverageDescriptors() []targetDimensionDescriptor {
	return []targetDimensionDescriptor{
		{name: "scenario_id", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.ScenarioID) }},
		{name: "seed_id", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.SeedID) }},
		{name: "task_id", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.TaskID) }},
		{name: "prompt_profile_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.PromptProfileID)
		}},
		{name: "prompt_variant_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(target.NormalizeTargetPromptVariantID(candidate.PromptVariantID))
		}},
		{name: "state_surface", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.StateSurface) }},
		{name: "lifecycle_edge", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.LifecycleEdge)
		}},
		{name: "contract_rule_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.ContractRuleID)
		}},
		{name: "lifecycle_operation_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.LifecycleOperationID)
		}},
		{name: "plant_primitive_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.PlantPrimitiveID)
		}},
		{name: "seed_to_plant", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(candidate.SeedID, candidate.PlantPrimitiveID)
		}},
		{name: "activation_kind_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.ActivationKindID)
		}},
		{name: "plant_to_activation", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(candidate.PlantPrimitiveID, candidate.ActivationKindID)
		}},
		{name: "lifecycle_to_activation", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(candidate.LifecycleOperationID, candidate.ActivationKindID)
		}},
		{name: "oracle_kind_id", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.OracleKindID) }},
		{name: "activation_to_oracle", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(candidate.ActivationKindID, candidate.OracleKindID)
		}},
		{name: "mutation_focus_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateMutationFocusID(candidate))
		}},
		{name: "mutation_focus_to_oracle", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(targetCandidateMutationFocusID(candidate), candidate.OracleKindID)
		}},
	}
}

func targetDimensionSingle(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func targetDimensionPair(left string, right string) []string {
	if pair := targetDimensionPairValue(left, right); pair != "" {
		return []string{pair}
	}
	return nil
}

func targetDimensionPairValue(left string, right string) string {
	if left == "" || right == "" {
		return ""
	}
	return left + "->" + right
}
