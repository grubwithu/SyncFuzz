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

type targetDimensionGapSet map[string]map[string]struct{}

type targetExplorationState struct {
	seenSeeds        map[string]struct{}
	seenPrimitives   map[string]struct{}
	seenTasks        map[string]struct{}
	seenRules        map[string]struct{}
	seenSurfaces     map[string]struct{}
	seenEdges        map[string]struct{}
	seenLifecycleOps map[string]struct{}
	seenActivations  map[string]struct{}
	seenOracles      map[string]struct{}
	seenMutations    map[string]struct{}
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
	var dimensionCoverage []TargetDimensionCoverageSummary
	if opts.FeedbackFrom != "" {
		feedback, err := readTargetMatrixFeedback(opts.FeedbackFrom)
		if err != nil {
			return nil, err
		}
		summaryByCandidate = make(map[string]TargetCandidateSummary, len(feedback.CandidateSummaries))
		for _, summary := range feedback.CandidateSummaries {
			summaryByCandidate[summary.CandidateID] = summary
		}
		dimensionCoverage = append([]TargetDimensionCoverageSummary{}, feedback.DimensionCoverage...)
	}
	if len(opts.ExcludeCandidateIDs) > 0 {
		candidates = filterExcludedTargetCandidates(candidates, opts.ExcludeCandidateIDs)
	}
	if len(summaryByCandidate) > 0 {
		candidates = orderTargetFeedbackCandidates(candidates, summaryByCandidate, dimensionCoverage)
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
	if left.ActivationReached != right.ActivationReached {
		return left.ActivationReached > right.ActivationReached
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

func orderTargetFeedbackCandidates(candidates []TargetScheduleCandidate, summaryByCandidate map[string]TargetCandidateSummary, dimensionCoverage []TargetDimensionCoverageSummary) []TargetScheduleCandidate {
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
	gaps := targetMissingDimensionValues(dimensionCoverage)
	if len(gaps) > 0 {
		unranked = orderTargetGapCandidates(unranked, gaps)
	} else {
		unranked = orderTargetExplorationCandidates(unranked)
	}
	return append(ranked, unranked...)
}

func targetMissingDimensionValues(summaries []TargetDimensionCoverageSummary) targetDimensionGapSet {
	gaps := make(targetDimensionGapSet, len(summaries))
	for _, summary := range summaries {
		if len(summary.MissingValues) == 0 {
			continue
		}
		values := make(map[string]struct{}, len(summary.MissingValues))
		for _, value := range summary.MissingValues {
			if value == "" {
				continue
			}
			values[value] = struct{}{}
		}
		if len(values) > 0 {
			gaps[summary.Dimension] = values
		}
	}
	return gaps
}

func orderTargetGapCandidates(candidates []TargetScheduleCandidate, gaps targetDimensionGapSet) []TargetScheduleCandidate {
	if len(candidates) <= 1 {
		return append([]TargetScheduleCandidate{}, candidates...)
	}

	remaining := append([]TargetScheduleCandidate{}, candidates...)
	sort.SliceStable(remaining, func(i, j int) bool {
		return targetExplorationBaseLess(remaining[i], remaining[j])
	})

	selected := make([]TargetScheduleCandidate, 0, len(remaining))
	state := newTargetExplorationState(len(remaining))

	for len(remaining) > 0 {
		bestIdx := 0
		bestGapScore := targetGapCoverageScore(remaining[0], gaps)
		bestNovelty := state.noveltyScore(remaining[0])
		for i := 1; i < len(remaining); i++ {
			gapScore := targetGapCoverageScore(remaining[i], gaps)
			novelty := state.noveltyScore(remaining[i])
			if gapScore > bestGapScore ||
				(gapScore == bestGapScore && (novelty > bestNovelty ||
					(novelty == bestNovelty && targetExplorationBaseLess(remaining[i], remaining[bestIdx])))) {
				bestIdx = i
				bestGapScore = gapScore
				bestNovelty = novelty
			}
		}

		pick := remaining[bestIdx]
		selected = append(selected, pick)
		targetConsumeGapCoverage(gaps, pick)
		state.record(pick)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

func targetGapCoverageScore(candidate TargetScheduleCandidate, gaps targetDimensionGapSet) int {
	if len(gaps) == 0 {
		return 0
	}
	score := 0
	for _, descriptor := range targetDimensionCoverageDescriptors() {
		values, ok := gaps[descriptor.name]
		if !ok || len(values) == 0 {
			continue
		}
		for _, value := range descriptor.values(candidate) {
			if _, ok := values[value]; ok {
				score += targetDimensionGapWeight(descriptor.name)
			}
		}
	}
	return score
}

func targetConsumeGapCoverage(gaps targetDimensionGapSet, candidate TargetScheduleCandidate) {
	for _, descriptor := range targetDimensionCoverageDescriptors() {
		values, ok := gaps[descriptor.name]
		if !ok || len(values) == 0 {
			continue
		}
		for _, value := range descriptor.values(candidate) {
			delete(values, value)
		}
		if len(values) == 0 {
			delete(gaps, descriptor.name)
		}
	}
}

func targetDimensionGapWeight(dimension string) int {
	switch dimension {
	case "seed_id":
		return 32
	case "plant_primitive_id":
		return 16
	case "scenario_id":
		return 12
	case "task_id":
		return 8
	case "lifecycle_operation_id":
		return 8
	case "activation_kind_id":
		return 6
	case "mutation_id":
		return 6
	case "state_surface":
		return 4
	case "contract_rule_id":
		return 4
	case "prompt_profile_id":
		return 3
	case "oracle_kind_id":
		return 3
	case "lifecycle_edge":
		return 2
	default:
		return 1
	}
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
	state := newTargetExplorationState(len(remaining))

	for len(remaining) > 0 {
		bestIdx := 0
		bestScore := state.noveltyScore(remaining[0])
		for i := 1; i < len(remaining); i++ {
			score := state.noveltyScore(remaining[i])
			if score > bestScore || (score == bestScore && targetExplorationBaseLess(remaining[i], remaining[bestIdx])) {
				bestIdx = i
				bestScore = score
			}
		}

		pick := remaining[bestIdx]
		selected = append(selected, pick)
		state.record(pick)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

func newTargetExplorationState(capacity int) *targetExplorationState {
	return &targetExplorationState{
		seenSeeds:        make(map[string]struct{}, capacity),
		seenPrimitives:   make(map[string]struct{}, capacity),
		seenTasks:        make(map[string]struct{}, capacity),
		seenRules:        make(map[string]struct{}, capacity),
		seenSurfaces:     make(map[string]struct{}, capacity),
		seenEdges:        make(map[string]struct{}, capacity),
		seenLifecycleOps: make(map[string]struct{}, capacity),
		seenActivations:  make(map[string]struct{}, capacity),
		seenOracles:      make(map[string]struct{}, capacity),
		seenMutations:    make(map[string]struct{}, capacity),
	}
}

func newTargetExplorationStateFromCandidates(candidates []TargetScheduleCandidate) *targetExplorationState {
	state := newTargetExplorationState(len(candidates))
	for _, candidate := range candidates {
		state.record(candidate)
	}
	return state
}

func (s *targetExplorationState) noveltyScore(candidate TargetScheduleCandidate) int {
	return targetExplorationNoveltyScore(
		candidate,
		s.seenSeeds,
		s.seenPrimitives,
		s.seenTasks,
		s.seenRules,
		s.seenSurfaces,
		s.seenEdges,
		s.seenLifecycleOps,
		s.seenActivations,
		s.seenOracles,
		s.seenMutations,
	)
}

func (s *targetExplorationState) record(candidate TargetScheduleCandidate) {
	targetRecordExplorationCandidate(
		candidate,
		s.seenSeeds,
		s.seenPrimitives,
		s.seenTasks,
		s.seenRules,
		s.seenSurfaces,
		s.seenEdges,
		s.seenLifecycleOps,
		s.seenActivations,
		s.seenOracles,
		s.seenMutations,
	)
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
