package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetFeedbackSelectionOptions struct {
	FeedbackFrom        string
	Limit               int
	ExcludeCandidateIDs []string
}

func selectTargetMatrixCandidates(matrix *TargetScheduleMatrix, opts TargetFeedbackSelectionOptions) (*TargetScheduleMatrix, error) {
	if matrix == nil {
		return nil, fmt.Errorf("target schedule matrix is required")
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}

	candidates := append([]TargetScheduleCandidate{}, matrix.Candidates...)
	var summaryByCandidate map[string]TargetCandidateSummary
	if opts.FeedbackFrom != "" {
		feedback, err := readTargetMatrixFeedback(opts.FeedbackFrom)
		if err != nil {
			return nil, err
		}
		summaryByCandidate = make(map[string]TargetCandidateSummary, len(feedback.CandidateSummaries))
		for _, summary := range feedback.CandidateSummaries {
			summaryByCandidate[summary.CandidateID] = summary
		}
	}
	if len(opts.ExcludeCandidateIDs) > 0 {
		filtered := filterExcludedTargetCandidates(candidates, opts.ExcludeCandidateIDs)
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	if len(summaryByCandidate) > 0 {
		candidates = orderTargetFeedbackCandidates(candidates, summaryByCandidate)
	} else {
		candidates = orderTargetExplorationCandidates(candidates)
	}
	if opts.Limit > 0 && len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}

	selected := *matrix
	selected.Candidates = append([]TargetScheduleCandidate{}, candidates...)
	selected.TotalCandidates = len(selected.Candidates)
	return &selected, nil
}

func filterExcludedTargetCandidates(candidates []TargetScheduleCandidate, exclude []string) []TargetScheduleCandidate {
	excluded := make(map[string]struct{}, len(exclude))
	for _, id := range exclude {
		excluded[id] = struct{}{}
	}
	out := make([]TargetScheduleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := excluded[candidate.CandidateID]; ok {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func readTargetMatrixFeedback(path string) (*TargetMatrixResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read feedback target matrix result %s: %w", path, err)
	}
	var result TargetMatrixResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode feedback target matrix result %s: %w", path, err)
	}
	if result.SchemaVersion != "syncfuzz.target-matrix-result.v1" {
		return nil, fmt.Errorf("unsupported feedback target matrix schema %q", result.SchemaVersion)
	}
	if len(result.CandidateSummaries) == 0 {
		return nil, fmt.Errorf("feedback target matrix result %s has no candidate_summaries", path)
	}
	return &result, nil
}

func targetFeedbackSummaryLess(left TargetCandidateSummary, right TargetCandidateSummary) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.ReproducibilityRate != right.ReproducibilityRate {
		return left.ReproducibilityRate > right.ReproducibilityRate
	}
	if left.ContractViolations != right.ContractViolations {
		return left.ContractViolations > right.ContractViolations
	}
	if left.Confirmed != right.Confirmed {
		return left.Confirmed > right.Confirmed
	}
	if left.ComplianceViolated != right.ComplianceViolated {
		return left.ComplianceViolated < right.ComplianceViolated
	}
	if left.Errors != right.Errors {
		return left.Errors < right.Errors
	}
	if left.CostPenalty != right.CostPenalty {
		return left.CostPenalty < right.CostPenalty
	}
	if left.AvgDurationMillis != right.AvgDurationMillis {
		return left.AvgDurationMillis < right.AvgDurationMillis
	}
	if left.AvgArtifactBytes != right.AvgArtifactBytes {
		return left.AvgArtifactBytes < right.AvgArtifactBytes
	}
	return left.CandidateID < right.CandidateID
}

func orderTargetFeedbackCandidates(candidates []TargetScheduleCandidate, summaryByCandidate map[string]TargetCandidateSummary) []TargetScheduleCandidate {
	ranked := make([]TargetScheduleCandidate, 0, len(candidates))
	unranked := make([]TargetScheduleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := summaryByCandidate[candidate.CandidateID]; ok {
			ranked = append(ranked, candidate)
			continue
		}
		unranked = append(unranked, candidate)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return targetFeedbackSummaryLess(summaryByCandidate[ranked[i].CandidateID], summaryByCandidate[ranked[j].CandidateID])
	})
	unranked = orderTargetExplorationCandidates(unranked)
	return append(ranked, unranked...)
}

func orderTargetExplorationCandidates(candidates []TargetScheduleCandidate) []TargetScheduleCandidate {
	if len(candidates) <= 1 {
		return append([]TargetScheduleCandidate{}, candidates...)
	}

	remaining := append([]TargetScheduleCandidate{}, candidates...)
	sort.SliceStable(remaining, func(i, j int) bool {
		return targetExplorationBaseLess(remaining[i], remaining[j])
	})

	selected := make([]TargetScheduleCandidate, 0, len(remaining))
	seenSeeds := make(map[string]struct{}, len(remaining))
	seenPrimitives := make(map[string]struct{}, len(remaining))
	seenTasks := make(map[string]struct{}, len(remaining))
	seenRules := make(map[string]struct{}, len(remaining))
	seenSurfaces := make(map[string]struct{}, len(remaining))
	seenEdges := make(map[string]struct{}, len(remaining))
	seenLifecycleOps := make(map[string]struct{}, len(remaining))
	seenActivations := make(map[string]struct{}, len(remaining))
	seenOracles := make(map[string]struct{}, len(remaining))
	seenMutations := make(map[string]struct{}, len(remaining))

	for len(remaining) > 0 {
		bestIdx := 0
		bestScore := targetExplorationNoveltyScore(remaining[0], seenSeeds, seenPrimitives, seenTasks, seenRules, seenSurfaces, seenEdges, seenLifecycleOps, seenActivations, seenOracles, seenMutations)
		for i := 1; i < len(remaining); i++ {
			score := targetExplorationNoveltyScore(remaining[i], seenSeeds, seenPrimitives, seenTasks, seenRules, seenSurfaces, seenEdges, seenLifecycleOps, seenActivations, seenOracles, seenMutations)
			if score > bestScore || (score == bestScore && targetExplorationBaseLess(remaining[i], remaining[bestIdx])) {
				bestIdx = i
				bestScore = score
			}
		}

		pick := remaining[bestIdx]
		selected = append(selected, pick)
		targetRecordExplorationCandidate(pick, seenSeeds, seenPrimitives, seenTasks, seenRules, seenSurfaces, seenEdges, seenLifecycleOps, seenActivations, seenOracles, seenMutations)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

func targetExplorationNoveltyScore(
	candidate TargetScheduleCandidate,
	seenSeeds map[string]struct{},
	seenPrimitives map[string]struct{},
	seenTasks map[string]struct{},
	seenRules map[string]struct{},
	seenSurfaces map[string]struct{},
	seenEdges map[string]struct{},
	seenLifecycleOps map[string]struct{},
	seenActivations map[string]struct{},
	seenOracles map[string]struct{},
	seenMutations map[string]struct{},
) int {
	score := 0
	if candidate.SeedID != "" {
		if _, ok := seenSeeds[candidate.SeedID]; !ok {
			score += 32
		}
	}
	if candidate.PlantPrimitiveID != "" {
		if _, ok := seenPrimitives[candidate.PlantPrimitiveID]; !ok {
			score += 16
		}
	}
	if _, ok := seenTasks[candidate.TaskID]; !ok {
		score += 8
	}
	if candidate.ContractRuleID != "" {
		if _, ok := seenRules[candidate.ContractRuleID]; !ok {
			score += 4
		}
	}
	if candidate.StateSurface != "" {
		if _, ok := seenSurfaces[candidate.StateSurface]; !ok {
			score += 2
		}
	}
	if candidate.LifecycleEdge != "" {
		if _, ok := seenEdges[candidate.LifecycleEdge]; !ok {
			score += 1
		}
	}
	if candidate.LifecycleOperationID != "" {
		if _, ok := seenLifecycleOps[candidate.LifecycleOperationID]; !ok {
			score += 4
		}
	}
	if candidate.ActivationKindID != "" {
		if _, ok := seenActivations[candidate.ActivationKindID]; !ok {
			score += 2
		}
	}
	if candidate.OracleKindID != "" {
		if _, ok := seenOracles[candidate.OracleKindID]; !ok {
			score += 1
		}
	}
	for _, mutation := range candidate.Mutations {
		if mutation.MutationID == "" {
			continue
		}
		if _, ok := seenMutations[mutation.MutationID]; !ok {
			score += 2
		}
	}
	return score
}

func targetRecordExplorationCandidate(
	candidate TargetScheduleCandidate,
	seenSeeds map[string]struct{},
	seenPrimitives map[string]struct{},
	seenTasks map[string]struct{},
	seenRules map[string]struct{},
	seenSurfaces map[string]struct{},
	seenEdges map[string]struct{},
	seenLifecycleOps map[string]struct{},
	seenActivations map[string]struct{},
	seenOracles map[string]struct{},
	seenMutations map[string]struct{},
) {
	if candidate.SeedID != "" {
		seenSeeds[candidate.SeedID] = struct{}{}
	}
	if candidate.PlantPrimitiveID != "" {
		seenPrimitives[candidate.PlantPrimitiveID] = struct{}{}
	}
	seenTasks[candidate.TaskID] = struct{}{}
	if candidate.ContractRuleID != "" {
		seenRules[candidate.ContractRuleID] = struct{}{}
	}
	if candidate.StateSurface != "" {
		seenSurfaces[candidate.StateSurface] = struct{}{}
	}
	if candidate.LifecycleEdge != "" {
		seenEdges[candidate.LifecycleEdge] = struct{}{}
	}
	if candidate.LifecycleOperationID != "" {
		seenLifecycleOps[candidate.LifecycleOperationID] = struct{}{}
	}
	if candidate.ActivationKindID != "" {
		seenActivations[candidate.ActivationKindID] = struct{}{}
	}
	if candidate.OracleKindID != "" {
		seenOracles[candidate.OracleKindID] = struct{}{}
	}
	for _, mutation := range candidate.Mutations {
		if mutation.MutationID == "" {
			continue
		}
		seenMutations[mutation.MutationID] = struct{}{}
	}
}

func targetExplorationBaseLess(left TargetScheduleCandidate, right TargetScheduleCandidate) bool {
	leftHasContract := left.ContractRuleID != ""
	rightHasContract := right.ContractRuleID != ""
	if leftHasContract != rightHasContract {
		return leftHasContract
	}
	leftProfileRank := targetPromptProfileRank(left.PromptProfileID)
	rightProfileRank := targetPromptProfileRank(right.PromptProfileID)
	if leftProfileRank != rightProfileRank {
		return leftProfileRank < rightProfileRank
	}
	if left.LifecycleEdge != right.LifecycleEdge {
		return left.LifecycleEdge < right.LifecycleEdge
	}
	if left.SeedID != right.SeedID {
		return left.SeedID < right.SeedID
	}
	if left.PlantPrimitiveID != right.PlantPrimitiveID {
		return left.PlantPrimitiveID < right.PlantPrimitiveID
	}
	if left.LifecycleOperationID != right.LifecycleOperationID {
		return left.LifecycleOperationID < right.LifecycleOperationID
	}
	if left.StateSurface != right.StateSurface {
		return left.StateSurface < right.StateSurface
	}
	if left.TaskID != right.TaskID {
		return left.TaskID < right.TaskID
	}
	return left.CandidateID < right.CandidateID
}

func targetPromptProfileRank(profileID string) int {
	switch target.NormalizeTargetPromptProfileID(profileID) {
	case target.TargetPromptProfileBaselineID:
		return 0
	case target.TargetPromptProfileWorkflowID:
		return 1
	case target.TargetPromptProfileAuditID:
		return 2
	default:
		return 3
	}
}
