package scheduler

import (
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

const targetFrontierDefaultLimit = 5

const (
	targetFrontierSelectionCoverageGap      = "coverage-gap"
	targetFrontierSelectionExploration      = "exploration-novelty"
	targetFrontierSelectionPromptRepair     = "prompt-repair"
	targetFrontierSelectionLifecycleRepair  = "lifecycle-repair"
	targetFrontierSelectionStateRepair      = "state-plant-repair"
	targetFrontierSelectionActivationRepair = "activation-repair"
	targetFrontierSelectionVariantExpand    = "variant-expansion"
	targetFrontierSelectionSeedExpand       = "seed-expansion"
)

type TargetFrontierCandidate struct {
	Rank                 int                             `json:"rank"`
	CandidateID          string                          `json:"candidate_id"`
	TargetID             string                          `json:"target_id"`
	ScenarioID           string                          `json:"scenario_id,omitempty"`
	SeedID               string                          `json:"seed_id,omitempty"`
	TaskID               string                          `json:"task_id"`
	PromptProfileID      string                          `json:"prompt_profile_id,omitempty"`
	PromptVariantID      string                          `json:"prompt_variant_id,omitempty"`
	LifecycleOperationID string                          `json:"lifecycle_operation_id,omitempty"`
	PlantPrimitiveID     string                          `json:"plant_primitive_id,omitempty"`
	ActivationKindID     string                          `json:"activation_kind_id,omitempty"`
	OracleKindID         string                          `json:"oracle_kind_id,omitempty"`
	Mutations            []target.TargetScenarioMutation `json:"mutations,omitempty"`
	ViolationSignature   target.TargetViolationSignature `json:"violation_signature"`
	GapScore             int                             `json:"gap_score,omitempty"`
	NoveltyScore         int                             `json:"novelty_score,omitempty"`
	SelectionMode        string                          `json:"selection_mode,omitempty"`
	CoveredGaps          []string                        `json:"covered_gaps,omitempty"`
}

func summarizeTargetCoverageFrontier(
	matrix *TargetScheduleMatrix,
	results []TargetSuiteRunResult,
	excludeCandidateIDs []string,
	limit int,
) []TargetFrontierCandidate {
	if matrix == nil || len(matrix.Candidates) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = targetFrontierDefaultLimit
	}

	candidateByID := make(map[string]TargetScheduleCandidate, len(matrix.Candidates))
	executedIDs := make(map[string]struct{}, len(results))
	executedCandidates := make([]TargetScheduleCandidate, 0, len(results))
	for _, candidate := range matrix.Candidates {
		candidateByID[candidate.CandidateID] = candidate
	}
	for _, candidateID := range excludeCandidateIDs {
		if candidateID == "" {
			continue
		}
		if _, ok := executedIDs[candidateID]; ok {
			continue
		}
		candidate, ok := candidateByID[candidateID]
		if !ok {
			continue
		}
		executedIDs[candidateID] = struct{}{}
		executedCandidates = append(executedCandidates, candidate)
	}
	for _, result := range results {
		if result.CandidateID == "" {
			continue
		}
		if _, ok := executedIDs[result.CandidateID]; ok {
			continue
		}
		candidate, ok := candidateByID[result.CandidateID]
		if !ok {
			continue
		}
		executedIDs[result.CandidateID] = struct{}{}
		executedCandidates = append(executedCandidates, candidate)
	}

	remaining := make([]TargetScheduleCandidate, 0, len(matrix.Candidates))
	for _, candidate := range matrix.Candidates {
		if _, ok := executedIDs[candidate.CandidateID]; ok {
			continue
		}
		remaining = append(remaining, candidate)
	}
	if len(remaining) == 0 {
		return nil
	}

	gaps := targetMissingDimensionValues(summarizeTargetDimensionCoverage(matrix.Candidates, results))
	state := newTargetExplorationStateFromCandidates(executedCandidates)
	repair := newTargetPromptRepairFeedbackFromResults(candidateByID, results)
	variantExpansion := newTargetVariantExpansionFeedbackFromResults(candidateByID, results)
	expansion := newTargetSeedExpansionFeedbackFromResults(candidateByID, results)
	sort.SliceStable(remaining, func(i, j int) bool {
		return targetExplorationBaseLess(remaining[i], remaining[j])
	})

	out := make([]TargetFrontierCandidate, 0, minInt(limit, len(remaining)))
	for len(remaining) > 0 && len(out) < limit {
		bestIdx := 0
		bestRepair := targetPromptRepairScore(remaining[0], repair)
		bestVariantExpansion := targetVariantExpansionScore(remaining[0], variantExpansion)
		bestExpansion := targetSeedExpansionScore(remaining[0], expansion)
		bestGapScore := targetGapCoverageScore(remaining[0], gaps)
		bestNovelty := state.noveltyScore(remaining[0])
		for i := 1; i < len(remaining); i++ {
			repairScore := targetPromptRepairScore(remaining[i], repair)
			variantExpansionScore := targetVariantExpansionScore(remaining[i], variantExpansion)
			expansionScore := targetSeedExpansionScore(remaining[i], expansion)
			gapScore := targetGapCoverageScore(remaining[i], gaps)
			novelty := state.noveltyScore(remaining[i])
			better := false
			switch {
			case repairScore > bestRepair:
				better = true
			case repairScore < bestRepair:
				better = false
			case variantExpansionScore > bestVariantExpansion:
				better = true
			case variantExpansionScore < bestVariantExpansion:
				better = false
			case expansionScore > bestExpansion:
				better = true
			case expansionScore < bestExpansion:
				better = false
			case gapScore > bestGapScore:
				better = true
			case gapScore < bestGapScore:
				better = false
			case novelty > bestNovelty:
				better = true
			case novelty < bestNovelty:
				better = false
			default:
				better = targetExplorationBaseLess(remaining[i], remaining[bestIdx])
			}
			if better {
				bestIdx = i
				bestRepair = repairScore
				bestVariantExpansion = variantExpansionScore
				bestExpansion = expansionScore
				bestGapScore = gapScore
				bestNovelty = novelty
			}
		}

		pick := remaining[bestIdx]
		coveredGaps := targetCandidateCoveredGaps(pick, gaps)
		selectionMode := targetFrontierSelectionExploration
		if bestRepair > 0 {
			selectionMode = targetFrontierPromptRepairSelectionMode(pick, repair)
		} else if bestVariantExpansion > 0 {
			selectionMode = targetFrontierSelectionVariantExpand
		} else if bestExpansion > 0 {
			selectionMode = targetFrontierSelectionSeedExpand
		} else if bestGapScore > 0 {
			selectionMode = targetFrontierSelectionCoverageGap
		}
		out = append(out, TargetFrontierCandidate{
			Rank:                 len(out) + 1,
			CandidateID:          pick.CandidateID,
			TargetID:             pick.TargetID,
			ScenarioID:           pick.ScenarioID,
			SeedID:               pick.SeedID,
			TaskID:               pick.TaskID,
			PromptProfileID:      pick.PromptProfileID,
			PromptVariantID:      target.NormalizeTargetPromptVariantID(pick.PromptVariantID),
			LifecycleOperationID: pick.LifecycleOperationID,
			PlantPrimitiveID:     pick.PlantPrimitiveID,
			ActivationKindID:     pick.ActivationKindID,
			OracleKindID:         pick.OracleKindID,
			Mutations:            append([]target.TargetScenarioMutation{}, pick.Mutations...),
			ViolationSignature:   pick.ViolationSignature,
			GapScore:             bestGapScore,
			NoveltyScore:         bestNovelty,
			SelectionMode:        selectionMode,
			CoveredGaps:          coveredGaps,
		})
		targetConsumePromptRepair(repair, pick)
		targetConsumeVariantExpansion(variantExpansion, pick)
		targetConsumeSeedExpansion(expansion, pick)
		targetConsumeGapCoverage(gaps, pick)
		state.record(pick)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return out
}

func targetFrontierPromptRepairSelectionMode(candidate TargetScheduleCandidate, feedback *targetPromptRepairFeedback) string {
	switch targetPromptRepairPreferredVariantForCandidate(candidate, feedback) {
	case target.TargetPromptVariantLifecycleBoundaryID:
		return targetFrontierSelectionLifecycleRepair
	case target.TargetPromptVariantMutationFocusID:
		return targetFrontierSelectionStateRepair
	case target.TargetPromptVariantActivationFocusID:
		return targetFrontierSelectionActivationRepair
	default:
		return targetFrontierSelectionPromptRepair
	}
}

func targetCandidateCoveredGaps(candidate TargetScheduleCandidate, gaps targetDimensionGapSet) []string {
	if len(gaps) == 0 {
		return nil
	}
	out := make([]string, 0)
	for _, descriptor := range targetDimensionCoverageDescriptors() {
		values, ok := gaps[descriptor.name]
		if !ok || len(values) == 0 {
			continue
		}
		for _, value := range descriptor.values(candidate) {
			if _, ok := values[value]; ok {
				out = append(out, descriptor.name+"="+value)
			}
		}
	}
	sort.Strings(out)
	return out
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
