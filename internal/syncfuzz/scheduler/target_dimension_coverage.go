package scheduler

import (
	"sort"
	"strings"

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
	Dimension                   string   `json:"dimension"`
	NewExecutedValues           []string `json:"new_executed_values,omitempty"`
	NewConfirmedValues          []string `json:"new_confirmed_values,omitempty"`
	NewActivationProgressValues []string `json:"new_activation_progress_values,omitempty"`
	NewActivationReachedValues  []string `json:"new_activation_reached_values,omitempty"`
}

type TargetDimensionCoverageGainStats struct {
	NewExecutedCount           int `json:"new_executed_count,omitempty"`
	NewConfirmedCount          int `json:"new_confirmed_count,omitempty"`
	NewActivationProgressCount int `json:"new_activation_progress_count,omitempty"`
	NewActivationReachedCount  int `json:"new_activation_reached_count,omitempty"`
	WeightedScore              int `json:"weighted_score,omitempty"`
}

type targetDimensionDescriptor struct {
	name   string
	values func(TargetScheduleCandidate) []string
}

type targetDimensionCoverageValue struct {
	executed          bool
	confirmed         bool
	activationReached bool
	activationStage   TargetActivationStage
}

type targetDimensionExecutionState struct {
	executed          bool
	confirmed         bool
	activationReached bool
	activationStage   TargetActivationStage
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
			if !stateAfter.activationReached && targetActivationStageProgressScore(stateAfter.activationStage) > targetActivationStageProgressScore(stateBefore.activationStage) {
				gain.NewActivationProgressValues = append(gain.NewActivationProgressValues, value)
			}
			if !stateBefore.activationReached && stateAfter.activationReached {
				gain.NewActivationReachedValues = append(gain.NewActivationReachedValues, value)
			}
		}
		sort.Strings(gain.NewExecutedValues)
		sort.Strings(gain.NewConfirmedValues)
		sort.Strings(gain.NewActivationProgressValues)
		sort.Strings(gain.NewActivationReachedValues)
		if len(gain.NewExecutedValues) == 0 && len(gain.NewConfirmedValues) == 0 && len(gain.NewActivationProgressValues) == 0 && len(gain.NewActivationReachedValues) == 0 {
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
		stats.NewActivationProgressCount += len(gain.NewActivationProgressValues)
		stats.NewActivationReachedCount += len(gain.NewActivationReachedValues)
		stats.WeightedScore += weight * len(gain.NewExecutedValues)
		stats.WeightedScore += weight * 2 * len(gain.NewConfirmedValues)
		stats.WeightedScore += weight * 2 * len(gain.NewActivationProgressValues)
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
		if targetActivationStageProgressScore(result.ActivationStage) > targetActivationStageProgressScore(state.activationStage) {
			state.activationStage = result.ActivationStage
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
				if targetActivationStageProgressScore(state.activationStage) > targetActivationStageProgressScore(current.activationStage) {
					current.activationStage = state.activationStage
				}
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
		{name: "violation_relation", values: func(candidate TargetScheduleCandidate) []string {
			return targetViolationRelations(candidate.ViolationSignature)
		}},
		{name: "violation_resource_class", values: func(candidate TargetScheduleCandidate) []string {
			return targetViolationResourceClasses(candidate.ViolationSignature)
		}},
		{name: "violation_lifecycle_boundary", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(string(candidate.ViolationSignature.LifecycleBoundary))
		}},
		{name: "violation_persistence_mechanism", values: func(candidate TargetScheduleCandidate) []string {
			return targetViolationPersistenceMechanisms(candidate.ViolationSignature)
		}},
		{name: "violation_consequence", values: func(candidate TargetScheduleCandidate) []string {
			return targetViolationConsequences(candidate.ViolationSignature)
		}},
		{name: "violation_signature_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.ViolationSignature.SignatureID)
		}},
		{name: "contract_rule_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.ContractRuleID)
		}},
		{name: "lifecycle_operation_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(candidate.LifecycleOperationID)
		}},
		{name: "checkpoint_selector", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateCheckpointSelector(candidate))
		}},
		{name: "checkpoint_backend", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateCheckpointBackend(candidate))
		}},
		{name: "process_mode", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateProcessMode(candidate))
		}},
		{name: "lifecycle_mode_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateLifecycleModeID(candidate))
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
		{name: "selector_to_activation", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(targetCandidateCheckpointSelector(candidate), candidate.ActivationKindID)
		}},
		{name: "oracle_kind_id", values: func(candidate TargetScheduleCandidate) []string { return targetDimensionSingle(candidate.OracleKindID) }},
		{name: "activation_to_oracle", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(candidate.ActivationKindID, candidate.OracleKindID)
		}},
		{name: "activation_path_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateActivationPathID(candidate))
		}},
		{name: "observation_path_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateObservationPathID(candidate))
		}},
		{name: "transition_signature", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateTransitionSignature(candidate))
		}},
		{name: "mutation_focus_id", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionSingle(targetCandidateMutationFocusID(candidate))
		}},
		{name: "mutation_focus_to_oracle", values: func(candidate TargetScheduleCandidate) []string {
			return targetDimensionPair(targetCandidateMutationFocusID(candidate), candidate.OracleKindID)
		}},
	}
}

func targetViolationRelations(signature target.TargetViolationSignature) []string {
	values := make([]string, 0, len(signature.Relations))
	for _, value := range signature.Relations {
		values = append(values, string(value))
	}
	return values
}

func targetViolationResourceClasses(signature target.TargetViolationSignature) []string {
	values := make([]string, 0, len(signature.ResourceClasses))
	for _, value := range signature.ResourceClasses {
		values = append(values, string(value))
	}
	return values
}

func targetViolationPersistenceMechanisms(signature target.TargetViolationSignature) []string {
	values := make([]string, 0, len(signature.PersistenceMechanisms))
	for _, value := range signature.PersistenceMechanisms {
		values = append(values, string(value))
	}
	return values
}

func targetViolationConsequences(signature target.TargetViolationSignature) []string {
	values := make([]string, 0, len(signature.Consequences))
	for _, value := range signature.Consequences {
		values = append(values, string(value))
	}
	return values
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

func targetDimensionChainValue(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return ""
		}
		trimmed = append(trimmed, part)
	}
	return strings.Join(trimmed, "=>")
}

func targetCandidateCheckpointSelector(candidate TargetScheduleCandidate) string {
	if candidate.ExecutionPlan == nil {
		return ""
	}
	return strings.TrimSpace(candidate.ExecutionPlan.CheckpointSelector)
}

func targetCandidateCheckpointBackend(candidate TargetScheduleCandidate) string {
	if candidate.ExecutionPlan == nil {
		return ""
	}
	return strings.TrimSpace(candidate.ExecutionPlan.CheckpointBackend)
}

func targetCandidateProcessMode(candidate TargetScheduleCandidate) string {
	if candidate.ExecutionPlan == nil {
		return ""
	}
	return strings.TrimSpace(candidate.ExecutionPlan.ProcessMode)
}

func targetCandidateLifecycleModeID(candidate TargetScheduleCandidate) string {
	if candidate.ExecutionPlan == nil {
		return ""
	}
	mode := "continue"
	switch {
	case candidate.ExecutionPlan.Replay:
		mode = "replay"
	case candidate.ExecutionPlan.ForkFollowup:
		mode = "fork-followup"
	}
	parts := []string{mode}
	if selector := targetCandidateCheckpointSelector(candidate); selector != "" {
		parts = append(parts, selector)
	}
	if backend := targetCandidateCheckpointBackend(candidate); backend != "" {
		parts = append(parts, backend)
	}
	if processMode := targetCandidateProcessMode(candidate); processMode != "" {
		parts = append(parts, processMode)
	}
	return strings.Join(parts, "|")
}

func targetCandidateActivationPathID(candidate TargetScheduleCandidate) string {
	lifecycleKey := candidate.LifecycleOperationID
	if mode := targetCandidateLifecycleModeID(candidate); mode != "" {
		lifecycleKey = mode
	}
	return targetDimensionChainValue(
		lifecycleKey,
		candidate.ActivationKindID,
	)
}

func targetCandidateObservationPathID(candidate TargetScheduleCandidate) string {
	lifecycleKey := candidate.LifecycleOperationID
	if mode := targetCandidateLifecycleModeID(candidate); mode != "" {
		lifecycleKey = mode
	}
	return targetDimensionChainValue(
		lifecycleKey,
		candidate.ActivationKindID,
		candidate.OracleKindID,
	)
}

func targetCandidateTransitionSignature(candidate TargetScheduleCandidate) string {
	lifecycleKey := candidate.LifecycleOperationID
	if mode := targetCandidateLifecycleModeID(candidate); mode != "" {
		lifecycleKey = mode
	}
	return targetDimensionChainValue(
		candidate.StateSurface,
		lifecycleKey,
		candidate.ActivationKindID,
		candidate.OracleKindID,
	)
}
