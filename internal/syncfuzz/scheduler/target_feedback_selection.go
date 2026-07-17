package scheduler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetSelectionPolicy string

const (
	TargetSelectionPolicyExplore  TargetSelectionPolicy = "explore"
	TargetSelectionPolicyFeedback TargetSelectionPolicy = "feedback"
	TargetSelectionPolicyFixed    TargetSelectionPolicy = "fixed"
	TargetSelectionPolicyRandom   TargetSelectionPolicy = "random"

	DefaultTargetRandomSeed int64 = 1
)

type TargetFeedbackSelectionOptions struct {
	FeedbackFrom        string
	Limit               int
	ExcludeCandidateIDs []string
	SelectionPolicy     TargetSelectionPolicy
	RandomSeed          int64
}

type targetDimensionGapSet map[string]map[string]struct{}

type targetExplorationState struct {
	seenSeeds          map[string]struct{}
	seenPrimitives     map[string]struct{}
	seenTasks          map[string]struct{}
	seenRules          map[string]struct{}
	seenPromptVariants map[string]struct{}
	seenSurfaces       map[string]struct{}
	seenEdges          map[string]struct{}
	seenLifecycleOps   map[string]struct{}
	seenSelectors      map[string]struct{}
	seenBackends       map[string]struct{}
	seenProcessModes   map[string]struct{}
	seenLifecycleModes map[string]struct{}
	seenActivations    map[string]struct{}
	seenOracles        map[string]struct{}
	seenMutations      map[string]struct{}
	seenSeedPlant      map[string]struct{}
	seenPlantAct       map[string]struct{}
	seenLifecycleAct   map[string]struct{}
	seenSelectorAct    map[string]struct{}
	seenActOracle      map[string]struct{}
	seenActivationPath map[string]struct{}
	seenObservation    map[string]struct{}
	seenTransitions    map[string]struct{}
	seenMutationOracle map[string]struct{}
}

type targetPromptRepairFeedback struct {
	taskScores        map[string]int
	preferredVariants map[string]string
	seenRealizations  map[string]map[string]struct{}
	selected          map[string]struct{}
}

type targetVariantExpansionContext struct {
	taskID               string
	promptProfileID      string
	lifecycleOperationID string
	confirmed            bool
	seenVariants         map[string]struct{}
}

type targetVariantExpansionFeedback struct {
	contexts []targetVariantExpansionContext
}

type targetSeedExpansionContext struct {
	seedID               string
	plantPrimitiveID     string
	lifecycleOperationID string
	confirmed            bool
	taskIDs              map[string]struct{}
	activationKinds      map[string]struct{}
	oracleKinds          map[string]struct{}
	mutationIDs          map[string]struct{}
}

type targetSeedExpansionFeedback struct {
	contexts []targetSeedExpansionContext
	selected map[string]struct{}
}

func selectTargetMatrixCandidates(matrix *TargetScheduleMatrix, opts TargetFeedbackSelectionOptions) (*TargetScheduleMatrix, error) {
	if matrix == nil {
		return nil, fmt.Errorf("target schedule matrix is required")
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}
	policy, err := normalizeTargetSelectionPolicy(opts.SelectionPolicy, opts.FeedbackFrom)
	if err != nil {
		return nil, err
	}
	randomSeed := opts.RandomSeed
	if policy == TargetSelectionPolicyRandom && randomSeed == 0 {
		randomSeed = DefaultTargetRandomSeed
	}

	candidates := append([]TargetScheduleCandidate{}, matrix.Candidates...)
	var summaryByCandidate map[string]TargetCandidateSummary
	var dimensionCoverage []TargetDimensionCoverageSummary
	if policy == TargetSelectionPolicyFeedback && opts.FeedbackFrom != "" {
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
	switch {
	case policy == TargetSelectionPolicyFixed:
	case policy == TargetSelectionPolicyRandom:
		orderTargetRandomCandidates(candidates, randomSeed)
	case len(summaryByCandidate) > 0:
		candidates = orderTargetFeedbackCandidates(candidates, summaryByCandidate, dimensionCoverage)
	default:
		candidates = orderTargetExplorationCandidates(candidates, nil, nil, nil)
	}
	if opts.Limit > 0 && len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}

	selected := *matrix
	selected.Candidates = append([]TargetScheduleCandidate{}, candidates...)
	selected.TotalCandidates = len(selected.Candidates)
	return &selected, nil
}

func normalizeTargetSelectionPolicy(policy TargetSelectionPolicy, feedbackFrom string) (TargetSelectionPolicy, error) {
	normalized := TargetSelectionPolicy(strings.TrimSpace(string(policy)))
	if normalized == "" {
		if strings.TrimSpace(feedbackFrom) != "" {
			return TargetSelectionPolicyFeedback, nil
		}
		return TargetSelectionPolicyExplore, nil
	}
	switch normalized {
	case TargetSelectionPolicyExplore, TargetSelectionPolicyFeedback, TargetSelectionPolicyFixed, TargetSelectionPolicyRandom:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown target selection policy %q", policy)
	}
}

func orderTargetRandomCandidates(candidates []TargetScheduleCandidate, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
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
	if left.ActivationProgressScore != right.ActivationProgressScore {
		return left.ActivationProgressScore > right.ActivationProgressScore
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
	repair := newTargetPromptRepairFeedback(summaryByCandidate)
	variantExpansion := newTargetVariantExpansionFeedback(summaryByCandidate)
	expansion := newTargetSeedExpansionFeedback(summaryByCandidate)
	if len(gaps) > 0 {
		unranked = orderTargetGapCandidates(unranked, gaps, repair, variantExpansion, expansion)
	} else {
		unranked = orderTargetExplorationCandidates(unranked, repair, variantExpansion, expansion)
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

func orderTargetGapCandidates(candidates []TargetScheduleCandidate, gaps targetDimensionGapSet, repair *targetPromptRepairFeedback, variantExpansion *targetVariantExpansionFeedback, expansion *targetSeedExpansionFeedback) []TargetScheduleCandidate {
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
		selected = append(selected, pick)
		targetConsumePromptRepair(repair, pick)
		targetConsumeVariantExpansion(variantExpansion, pick)
		targetConsumeSeedExpansion(expansion, pick)
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
	case "seed_to_plant":
		return 14
	case "scenario_id":
		return 12
	case "task_id":
		return 8
	case "lifecycle_operation_id":
		return 8
	case "checkpoint_selector":
		return 8
	case "transition_signature":
		return 8
	case "activation_kind_id":
		return 6
	case "lifecycle_mode_id":
		return 6
	case "plant_to_activation":
		return 10
	case "lifecycle_to_activation":
		return 9
	case "selector_to_activation":
		return 8
	case "activation_path_id":
		return 9
	case "observation_path_id":
		return 8
	case "mutation_focus_id":
		return 6
	case "mutation_focus_to_oracle":
		return 8
	case "state_surface":
		return 4
	case "contract_rule_id":
		return 4
	case "checkpoint_backend":
		return 4
	case "process_mode":
		return 4
	case "prompt_profile_id":
		return 3
	case "prompt_variant_id":
		return 3
	case "oracle_kind_id":
		return 3
	case "activation_to_oracle":
		return 5
	case "lifecycle_edge":
		return 2
	default:
		return 1
	}
}

func orderTargetExplorationCandidates(candidates []TargetScheduleCandidate, repair *targetPromptRepairFeedback, variantExpansion *targetVariantExpansionFeedback, expansion *targetSeedExpansionFeedback) []TargetScheduleCandidate {
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
		bestRepair := targetPromptRepairScore(remaining[0], repair)
		bestVariantExpansion := targetVariantExpansionScore(remaining[0], variantExpansion)
		bestExpansion := targetSeedExpansionScore(remaining[0], expansion)
		bestScore := state.noveltyScore(remaining[0])
		for i := 1; i < len(remaining); i++ {
			repairScore := targetPromptRepairScore(remaining[i], repair)
			variantExpansionScore := targetVariantExpansionScore(remaining[i], variantExpansion)
			expansionScore := targetSeedExpansionScore(remaining[i], expansion)
			score := state.noveltyScore(remaining[i])
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
			case score > bestScore:
				better = true
			case score < bestScore:
				better = false
			default:
				better = targetExplorationBaseLess(remaining[i], remaining[bestIdx])
			}
			if better {
				bestIdx = i
				bestRepair = repairScore
				bestVariantExpansion = variantExpansionScore
				bestExpansion = expansionScore
				bestScore = score
			}
		}

		pick := remaining[bestIdx]
		selected = append(selected, pick)
		targetConsumePromptRepair(repair, pick)
		targetConsumeVariantExpansion(variantExpansion, pick)
		targetConsumeSeedExpansion(expansion, pick)
		state.record(pick)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

func newTargetExplorationState(capacity int) *targetExplorationState {
	return &targetExplorationState{
		seenSeeds:          make(map[string]struct{}, capacity),
		seenPrimitives:     make(map[string]struct{}, capacity),
		seenTasks:          make(map[string]struct{}, capacity),
		seenRules:          make(map[string]struct{}, capacity),
		seenPromptVariants: make(map[string]struct{}, capacity),
		seenSurfaces:       make(map[string]struct{}, capacity),
		seenEdges:          make(map[string]struct{}, capacity),
		seenLifecycleOps:   make(map[string]struct{}, capacity),
		seenSelectors:      make(map[string]struct{}, capacity),
		seenBackends:       make(map[string]struct{}, capacity),
		seenProcessModes:   make(map[string]struct{}, capacity),
		seenLifecycleModes: make(map[string]struct{}, capacity),
		seenActivations:    make(map[string]struct{}, capacity),
		seenOracles:        make(map[string]struct{}, capacity),
		seenMutations:      make(map[string]struct{}, capacity),
		seenSeedPlant:      make(map[string]struct{}, capacity),
		seenPlantAct:       make(map[string]struct{}, capacity),
		seenLifecycleAct:   make(map[string]struct{}, capacity),
		seenSelectorAct:    make(map[string]struct{}, capacity),
		seenActOracle:      make(map[string]struct{}, capacity),
		seenActivationPath: make(map[string]struct{}, capacity),
		seenObservation:    make(map[string]struct{}, capacity),
		seenTransitions:    make(map[string]struct{}, capacity),
		seenMutationOracle: make(map[string]struct{}, capacity),
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
		s.seenPromptVariants,
		s.seenSurfaces,
		s.seenEdges,
		s.seenLifecycleOps,
		s.seenSelectors,
		s.seenBackends,
		s.seenProcessModes,
		s.seenLifecycleModes,
		s.seenActivations,
		s.seenOracles,
		s.seenMutations,
		s.seenSeedPlant,
		s.seenPlantAct,
		s.seenLifecycleAct,
		s.seenSelectorAct,
		s.seenActOracle,
		s.seenActivationPath,
		s.seenObservation,
		s.seenTransitions,
		s.seenMutationOracle,
	)
}

func (s *targetExplorationState) record(candidate TargetScheduleCandidate) {
	targetRecordExplorationCandidate(
		candidate,
		s.seenSeeds,
		s.seenPrimitives,
		s.seenTasks,
		s.seenRules,
		s.seenPromptVariants,
		s.seenSurfaces,
		s.seenEdges,
		s.seenLifecycleOps,
		s.seenSelectors,
		s.seenBackends,
		s.seenProcessModes,
		s.seenLifecycleModes,
		s.seenActivations,
		s.seenOracles,
		s.seenMutations,
		s.seenSeedPlant,
		s.seenPlantAct,
		s.seenLifecycleAct,
		s.seenSelectorAct,
		s.seenActOracle,
		s.seenActivationPath,
		s.seenObservation,
		s.seenTransitions,
		s.seenMutationOracle,
	)
}

func targetExplorationNoveltyScore(
	candidate TargetScheduleCandidate,
	seenSeeds map[string]struct{},
	seenPrimitives map[string]struct{},
	seenTasks map[string]struct{},
	seenRules map[string]struct{},
	seenPromptVariants map[string]struct{},
	seenSurfaces map[string]struct{},
	seenEdges map[string]struct{},
	seenLifecycleOps map[string]struct{},
	seenSelectors map[string]struct{},
	seenBackends map[string]struct{},
	seenProcessModes map[string]struct{},
	seenLifecycleModes map[string]struct{},
	seenActivations map[string]struct{},
	seenOracles map[string]struct{},
	seenMutations map[string]struct{},
	seenSeedPlant map[string]struct{},
	seenPlantAct map[string]struct{},
	seenLifecycleAct map[string]struct{},
	seenSelectorAct map[string]struct{},
	seenActOracle map[string]struct{},
	seenActivationPath map[string]struct{},
	seenObservation map[string]struct{},
	seenTransitions map[string]struct{},
	seenMutationOracle map[string]struct{},
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
	if pair := targetDimensionPairValue(candidate.SeedID, candidate.PlantPrimitiveID); pair != "" {
		if _, ok := seenSeedPlant[pair]; !ok {
			score += 12
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
	if promptVariantID := target.NormalizeTargetPromptVariantID(candidate.PromptVariantID); promptVariantID != "" {
		if _, ok := seenPromptVariants[promptVariantID]; !ok {
			score += 3
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
	if selector := targetCandidateCheckpointSelector(candidate); selector != "" {
		if _, ok := seenSelectors[selector]; !ok {
			score += 5
		}
	}
	if backend := targetCandidateCheckpointBackend(candidate); backend != "" {
		if _, ok := seenBackends[backend]; !ok {
			score += 3
		}
	}
	if processMode := targetCandidateProcessMode(candidate); processMode != "" {
		if _, ok := seenProcessModes[processMode]; !ok {
			score += 3
		}
	}
	if lifecycleMode := targetCandidateLifecycleModeID(candidate); lifecycleMode != "" {
		if _, ok := seenLifecycleModes[lifecycleMode]; !ok {
			score += 6
		}
	}
	if candidate.ActivationKindID != "" {
		if _, ok := seenActivations[candidate.ActivationKindID]; !ok {
			score += 2
		}
	}
	if pair := targetDimensionPairValue(candidate.PlantPrimitiveID, candidate.ActivationKindID); pair != "" {
		if _, ok := seenPlantAct[pair]; !ok {
			score += 6
		}
	}
	if pair := targetDimensionPairValue(candidate.LifecycleOperationID, candidate.ActivationKindID); pair != "" {
		if _, ok := seenLifecycleAct[pair]; !ok {
			score += 5
		}
	}
	if pair := targetDimensionPairValue(targetCandidateCheckpointSelector(candidate), candidate.ActivationKindID); pair != "" {
		if _, ok := seenSelectorAct[pair]; !ok {
			score += 6
		}
	}
	if candidate.OracleKindID != "" {
		if _, ok := seenOracles[candidate.OracleKindID]; !ok {
			score += 1
		}
	}
	if pair := targetDimensionPairValue(candidate.ActivationKindID, candidate.OracleKindID); pair != "" {
		if _, ok := seenActOracle[pair]; !ok {
			score += 3
		}
	}
	if activationPath := targetCandidateActivationPathID(candidate); activationPath != "" {
		if _, ok := seenActivationPath[activationPath]; !ok {
			score += 7
		}
	}
	if observationPath := targetCandidateObservationPathID(candidate); observationPath != "" {
		if _, ok := seenObservation[observationPath]; !ok {
			score += 6
		}
	}
	if signature := targetCandidateTransitionSignature(candidate); signature != "" {
		if _, ok := seenTransitions[signature]; !ok {
			score += 8
		}
	}
	if mutationID := targetCandidateMutationFocusID(candidate); mutationID != "" {
		if _, ok := seenMutations[mutationID]; !ok {
			score += 2
		}
		if pair := targetDimensionPairValue(mutationID, candidate.OracleKindID); pair != "" {
			if _, ok := seenMutationOracle[pair]; !ok {
				score += 4
			}
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
	seenPromptVariants map[string]struct{},
	seenSurfaces map[string]struct{},
	seenEdges map[string]struct{},
	seenLifecycleOps map[string]struct{},
	seenSelectors map[string]struct{},
	seenBackends map[string]struct{},
	seenProcessModes map[string]struct{},
	seenLifecycleModes map[string]struct{},
	seenActivations map[string]struct{},
	seenOracles map[string]struct{},
	seenMutations map[string]struct{},
	seenSeedPlant map[string]struct{},
	seenPlantAct map[string]struct{},
	seenLifecycleAct map[string]struct{},
	seenSelectorAct map[string]struct{},
	seenActOracle map[string]struct{},
	seenActivationPath map[string]struct{},
	seenObservation map[string]struct{},
	seenTransitions map[string]struct{},
	seenMutationOracle map[string]struct{},
) {
	if candidate.SeedID != "" {
		seenSeeds[candidate.SeedID] = struct{}{}
	}
	if candidate.PlantPrimitiveID != "" {
		seenPrimitives[candidate.PlantPrimitiveID] = struct{}{}
	}
	if pair := targetDimensionPairValue(candidate.SeedID, candidate.PlantPrimitiveID); pair != "" {
		seenSeedPlant[pair] = struct{}{}
	}
	seenTasks[candidate.TaskID] = struct{}{}
	if candidate.ContractRuleID != "" {
		seenRules[candidate.ContractRuleID] = struct{}{}
	}
	if promptVariantID := target.NormalizeTargetPromptVariantID(candidate.PromptVariantID); promptVariantID != "" {
		seenPromptVariants[promptVariantID] = struct{}{}
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
	if selector := targetCandidateCheckpointSelector(candidate); selector != "" {
		seenSelectors[selector] = struct{}{}
	}
	if backend := targetCandidateCheckpointBackend(candidate); backend != "" {
		seenBackends[backend] = struct{}{}
	}
	if processMode := targetCandidateProcessMode(candidate); processMode != "" {
		seenProcessModes[processMode] = struct{}{}
	}
	if lifecycleMode := targetCandidateLifecycleModeID(candidate); lifecycleMode != "" {
		seenLifecycleModes[lifecycleMode] = struct{}{}
	}
	if candidate.ActivationKindID != "" {
		seenActivations[candidate.ActivationKindID] = struct{}{}
	}
	if pair := targetDimensionPairValue(candidate.PlantPrimitiveID, candidate.ActivationKindID); pair != "" {
		seenPlantAct[pair] = struct{}{}
	}
	if pair := targetDimensionPairValue(candidate.LifecycleOperationID, candidate.ActivationKindID); pair != "" {
		seenLifecycleAct[pair] = struct{}{}
	}
	if pair := targetDimensionPairValue(targetCandidateCheckpointSelector(candidate), candidate.ActivationKindID); pair != "" {
		seenSelectorAct[pair] = struct{}{}
	}
	if candidate.OracleKindID != "" {
		seenOracles[candidate.OracleKindID] = struct{}{}
	}
	if pair := targetDimensionPairValue(candidate.ActivationKindID, candidate.OracleKindID); pair != "" {
		seenActOracle[pair] = struct{}{}
	}
	if activationPath := targetCandidateActivationPathID(candidate); activationPath != "" {
		seenActivationPath[activationPath] = struct{}{}
	}
	if observationPath := targetCandidateObservationPathID(candidate); observationPath != "" {
		seenObservation[observationPath] = struct{}{}
	}
	if signature := targetCandidateTransitionSignature(candidate); signature != "" {
		seenTransitions[signature] = struct{}{}
	}
	if mutationID := targetCandidateMutationFocusID(candidate); mutationID != "" {
		seenMutations[mutationID] = struct{}{}
		if pair := targetDimensionPairValue(mutationID, candidate.OracleKindID); pair != "" {
			seenMutationOracle[pair] = struct{}{}
		}
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
	leftVariantID := target.NormalizeTargetPromptVariantID(left.PromptVariantID)
	rightVariantID := target.NormalizeTargetPromptVariantID(right.PromptVariantID)
	if leftVariantID != rightVariantID {
		return leftVariantID < rightVariantID
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

func newTargetPromptRepairFeedback(summaryByCandidate map[string]TargetCandidateSummary) *targetPromptRepairFeedback {
	if len(summaryByCandidate) == 0 {
		return nil
	}
	type taskState struct {
		seenRealizations      map[string]struct{}
		preferredVariantScore map[string]int
		activationReached     int
		repairScore           int
	}
	tasks := make(map[string]*taskState)
	for _, summary := range summaryByCandidate {
		if summary.TaskID == "" {
			continue
		}
		state := tasks[summary.TaskID]
		if state == nil {
			state = &taskState{
				seenRealizations:      make(map[string]struct{}),
				preferredVariantScore: make(map[string]int),
			}
			tasks[summary.TaskID] = state
		}
		state.seenRealizations[targetPromptRepairRealizationID(summary.PromptProfileID, summary.PromptVariantID)] = struct{}{}
		state.activationReached += summary.ActivationReached
		summaryRepairScore := 0
		for _, outcome := range summary.OutcomeSummaries {
			weight := outcome.TotalRuns * targetPromptRepairOutcomeWeight(outcome.Category)
			summaryRepairScore += weight
			state.repairScore += weight
			if variantID := targetPromptRepairVariantForOutcome(outcome.Category); variantID != "" {
				state.preferredVariantScore[variantID] += weight
			}
		}
		if summaryRepairScore == 0 && summary.ActivationReached == 0 {
			stage := summary.MaxActivationStage
			if stage == "" {
				stage = targetCandidateMaxActivationStage(summary.ActivationSummaries)
			}
			category := targetPromptRepairCategoryForStage(stage)
			runs := summary.Runs
			if runs < 1 {
				runs = 1
			}
			weight := runs * targetPromptRepairOutcomeWeight(category)
			state.repairScore += weight
			if variantID := targetPromptRepairVariantForOutcome(category); variantID != "" {
				state.preferredVariantScore[variantID] += weight
			}
		}
	}

	feedback := &targetPromptRepairFeedback{
		taskScores:        make(map[string]int),
		preferredVariants: make(map[string]string),
		seenRealizations:  make(map[string]map[string]struct{}),
		selected:          make(map[string]struct{}),
	}
	for taskID, state := range tasks {
		if state.activationReached > 0 || state.repairScore <= 0 {
			continue
		}
		feedback.taskScores[taskID] = state.repairScore
		feedback.preferredVariants[taskID] = targetPromptRepairPreferredVariant(state.preferredVariantScore)
		feedback.seenRealizations[taskID] = state.seenRealizations
	}
	if len(feedback.taskScores) == 0 {
		return nil
	}
	return feedback
}

func newTargetPromptRepairFeedbackFromResults(candidateByID map[string]TargetScheduleCandidate, results []TargetSuiteRunResult) *targetPromptRepairFeedback {
	if len(results) == 0 {
		return nil
	}
	type taskState struct {
		seenRealizations      map[string]struct{}
		preferredVariantScore map[string]int
		activationReached     int
		repairScore           int
	}
	tasks := make(map[string]*taskState)
	for _, result := range results {
		taskID := result.TaskID
		profileID := result.PromptProfileID
		variantID := result.PromptVariantID
		if candidate, ok := candidateByID[result.CandidateID]; ok {
			if taskID == "" {
				taskID = candidate.TaskID
			}
			if profileID == "" {
				profileID = candidate.PromptProfileID
			}
			if variantID == "" {
				variantID = candidate.PromptVariantID
			}
		}
		if taskID == "" {
			continue
		}
		state := tasks[taskID]
		if state == nil {
			state = &taskState{
				seenRealizations:      make(map[string]struct{}),
				preferredVariantScore: make(map[string]int),
			}
			tasks[taskID] = state
		}
		state.seenRealizations[targetPromptRepairRealizationID(profileID, variantID)] = struct{}{}
		if result.ActivationStage == TargetActivationStageActivationReached {
			state.activationReached++
		}
		category := result.OutcomeCategory
		weight := targetPromptRepairOutcomeWeight(category)
		if weight == 0 && result.ActivationStage != TargetActivationStageActivationReached {
			category = targetPromptRepairCategoryForStage(result.ActivationStage)
			weight = targetPromptRepairOutcomeWeight(category)
		}
		state.repairScore += weight
		if variantID := targetPromptRepairVariantForOutcome(category); variantID != "" {
			state.preferredVariantScore[variantID] += weight
		}
	}

	feedback := &targetPromptRepairFeedback{
		taskScores:        make(map[string]int),
		preferredVariants: make(map[string]string),
		seenRealizations:  make(map[string]map[string]struct{}),
		selected:          make(map[string]struct{}),
	}
	for taskID, state := range tasks {
		if state.activationReached > 0 || state.repairScore <= 0 {
			continue
		}
		feedback.taskScores[taskID] = state.repairScore
		feedback.preferredVariants[taskID] = targetPromptRepairPreferredVariant(state.preferredVariantScore)
		feedback.seenRealizations[taskID] = state.seenRealizations
	}
	if len(feedback.taskScores) == 0 {
		return nil
	}
	return feedback
}

func targetPromptRepairOutcomeWeight(category corpus.TargetObservationCategory) int {
	switch category {
	case corpus.TargetObservationExecutionNotReached:
		return 8
	case corpus.TargetObservationTaskNoncompliant:
		return 7
	case corpus.TargetObservationLifecycleNotTriggered:
		return 5
	case corpus.TargetObservationStateNotPlanted:
		return 4
	case corpus.TargetObservationActivationNotTriggered:
		return 4
	case corpus.TargetObservationOracleInconclusive:
		return 2
	default:
		return 0
	}
}

func targetPromptRepairVariantForOutcome(category corpus.TargetObservationCategory) string {
	switch category {
	case corpus.TargetObservationLifecycleNotTriggered:
		return target.TargetPromptVariantLifecycleBoundaryID
	case corpus.TargetObservationStateNotPlanted:
		return target.TargetPromptVariantMutationFocusID
	case corpus.TargetObservationActivationNotTriggered:
		return target.TargetPromptVariantActivationFocusID
	default:
		return ""
	}
}

func targetPromptRepairCategoryForStage(stage TargetActivationStage) corpus.TargetObservationCategory {
	switch stage {
	case TargetActivationStageExecutionPending:
		return corpus.TargetObservationExecutionNotReached
	case TargetActivationStageTaskNoncompliant:
		return corpus.TargetObservationTaskNoncompliant
	case TargetActivationStageLifecyclePending:
		return corpus.TargetObservationLifecycleNotTriggered
	case TargetActivationStageStateNotPlanted:
		return corpus.TargetObservationStateNotPlanted
	case TargetActivationStageActivationPending:
		return corpus.TargetObservationActivationNotTriggered
	default:
		return ""
	}
}

func targetPromptRepairPreferredVariant(scores map[string]int) string {
	bestVariant := ""
	bestScore := 0
	for variantID, score := range scores {
		if score > bestScore || (score == bestScore && targetPromptVariantRank(variantID) > targetPromptVariantRank(bestVariant)) {
			bestVariant = variantID
			bestScore = score
		}
	}
	return bestVariant
}

func targetPromptRepairScore(candidate TargetScheduleCandidate, feedback *targetPromptRepairFeedback) int {
	if feedback == nil || candidate.TaskID == "" {
		return 0
	}
	if _, ok := feedback.selected[candidate.TaskID]; ok {
		return 0
	}
	score, ok := feedback.taskScores[candidate.TaskID]
	if !ok || score <= 0 {
		return 0
	}
	seenRealizations := feedback.seenRealizations[candidate.TaskID]
	if _, ok := seenRealizations[targetPromptRepairRealizationID(candidate.PromptProfileID, candidate.PromptVariantID)]; ok {
		return 0
	}
	if target.NormalizeTargetPromptVariantID(candidate.PromptVariantID) == feedback.preferredVariants[candidate.TaskID] {
		score += 3
	}
	return score
}

func targetPromptRepairPreferredVariantForCandidate(candidate TargetScheduleCandidate, feedback *targetPromptRepairFeedback) string {
	if feedback == nil || candidate.TaskID == "" {
		return ""
	}
	variantID := feedback.preferredVariants[candidate.TaskID]
	if variantID == "" || target.NormalizeTargetPromptVariantID(candidate.PromptVariantID) != variantID {
		return ""
	}
	return variantID
}

func targetConsumePromptRepair(feedback *targetPromptRepairFeedback, candidate TargetScheduleCandidate) {
	if feedback == nil || candidate.TaskID == "" {
		return
	}
	if targetPromptRepairScore(candidate, feedback) <= 0 {
		return
	}
	feedback.selected[candidate.TaskID] = struct{}{}
}

func targetPromptRepairRealizationID(promptProfileID string, promptVariantID string) string {
	return target.NormalizeTargetPromptProfileID(promptProfileID) + "|" + target.NormalizeTargetPromptVariantID(promptVariantID)
}

func newTargetVariantExpansionFeedback(summaryByCandidate map[string]TargetCandidateSummary) *targetVariantExpansionFeedback {
	if len(summaryByCandidate) == 0 {
		return nil
	}
	contexts := make([]targetVariantExpansionContext, 0, len(summaryByCandidate))
	for _, summary := range summaryByCandidate {
		if !targetVariantExpansionEligibleTask(summary.TaskID, summary.LifecycleOperationID) {
			continue
		}
		if summary.Confirmed == 0 && summary.ActivationReached == 0 {
			continue
		}
		contexts = append(contexts, targetVariantExpansionContext{
			taskID:               summary.TaskID,
			promptProfileID:      target.NormalizeTargetPromptProfileID(summary.PromptProfileID),
			lifecycleOperationID: summary.LifecycleOperationID,
			confirmed:            summary.Confirmed > 0,
			seenVariants: map[string]struct{}{
				target.NormalizeTargetPromptVariantID(summary.PromptVariantID): {},
			},
		})
	}
	if len(contexts) == 0 {
		return nil
	}
	return &targetVariantExpansionFeedback{contexts: contexts}
}

func newTargetVariantExpansionFeedbackFromResults(candidateByID map[string]TargetScheduleCandidate, results []TargetSuiteRunResult) *targetVariantExpansionFeedback {
	if len(results) == 0 {
		return nil
	}
	contexts := make([]targetVariantExpansionContext, 0, len(results))
	for _, result := range results {
		candidate, ok := candidateByID[result.CandidateID]
		if !ok {
			continue
		}
		if !targetVariantExpansionEligibleCandidate(candidate) {
			continue
		}
		if !result.Confirmed && result.ActivationStage != TargetActivationStageActivationReached {
			continue
		}
		contexts = append(contexts, targetVariantExpansionContext{
			taskID:               candidate.TaskID,
			promptProfileID:      target.NormalizeTargetPromptProfileID(candidate.PromptProfileID),
			lifecycleOperationID: candidate.LifecycleOperationID,
			confirmed:            result.Confirmed,
			seenVariants: map[string]struct{}{
				target.NormalizeTargetPromptVariantID(candidate.PromptVariantID): {},
			},
		})
	}
	if len(contexts) == 0 {
		return nil
	}
	return &targetVariantExpansionFeedback{contexts: contexts}
}

func targetVariantExpansionScore(candidate TargetScheduleCandidate, feedback *targetVariantExpansionFeedback) int {
	if feedback == nil || !targetVariantExpansionEligibleCandidate(candidate) {
		return 0
	}
	best := 0
	candidateVariant := target.NormalizeTargetPromptVariantID(candidate.PromptVariantID)
	for _, ctx := range feedback.contexts {
		if candidate.TaskID != ctx.taskID {
			continue
		}
		if target.NormalizeTargetPromptProfileID(candidate.PromptProfileID) != ctx.promptProfileID {
			continue
		}
		if candidate.LifecycleOperationID != ctx.lifecycleOperationID {
			continue
		}
		if _, ok := ctx.seenVariants[candidateVariant]; ok {
			continue
		}
		score := 0
		if ctx.confirmed {
			score += 8
		} else {
			score += 4
		}
		if candidateVariant != target.TargetPromptVariantBaseID {
			score += 3
		}
		if score > best {
			best = score
		}
	}
	return best
}

func targetConsumeVariantExpansion(feedback *targetVariantExpansionFeedback, candidate TargetScheduleCandidate) {
	if feedback == nil || !targetVariantExpansionEligibleCandidate(candidate) {
		return
	}
	candidateVariant := target.NormalizeTargetPromptVariantID(candidate.PromptVariantID)
	for i := range feedback.contexts {
		ctx := &feedback.contexts[i]
		if candidate.TaskID != ctx.taskID {
			continue
		}
		if target.NormalizeTargetPromptProfileID(candidate.PromptProfileID) != ctx.promptProfileID {
			continue
		}
		if candidate.LifecycleOperationID != ctx.lifecycleOperationID {
			continue
		}
		ctx.seenVariants[candidateVariant] = struct{}{}
	}
}

func targetVariantExpansionEligibleCandidate(candidate TargetScheduleCandidate) bool {
	return targetVariantExpansionEligibleTask(candidate.TaskID, candidate.LifecycleOperationID)
}

func targetVariantExpansionEligibleTask(taskID string, lifecycleOperationID string) bool {
	if strings.TrimSpace(taskID) == "" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(lifecycleOperationID), "checkpoint-")
}

func newTargetSeedExpansionFeedback(summaryByCandidate map[string]TargetCandidateSummary) *targetSeedExpansionFeedback {
	if len(summaryByCandidate) == 0 {
		return nil
	}
	contexts := make([]targetSeedExpansionContext, 0, len(summaryByCandidate))
	for _, summary := range summaryByCandidate {
		if summary.SeedID == "" {
			continue
		}
		if summary.Confirmed == 0 && summary.ActivationReached == 0 {
			continue
		}
		ctx := targetSeedExpansionContext{
			seedID:               summary.SeedID,
			plantPrimitiveID:     summary.PlantPrimitiveID,
			lifecycleOperationID: summary.LifecycleOperationID,
			confirmed:            summary.Confirmed > 0,
			taskIDs:              make(map[string]struct{}, 1),
			activationKinds:      make(map[string]struct{}, 1),
			oracleKinds:          make(map[string]struct{}, 1),
			mutationIDs:          make(map[string]struct{}, 1),
		}
		if summary.TaskID != "" {
			ctx.taskIDs[summary.TaskID] = struct{}{}
		}
		if summary.ActivationKindID != "" {
			ctx.activationKinds[summary.ActivationKindID] = struct{}{}
		}
		if summary.OracleKindID != "" {
			ctx.oracleKinds[summary.OracleKindID] = struct{}{}
		}
		if summary.MutationFocusID != "" {
			ctx.mutationIDs[summary.MutationFocusID] = struct{}{}
		}
		contexts = append(contexts, ctx)
	}
	if len(contexts) == 0 {
		return nil
	}
	return &targetSeedExpansionFeedback{
		contexts: contexts,
		selected: make(map[string]struct{}),
	}
}

func newTargetSeedExpansionFeedbackFromResults(candidateByID map[string]TargetScheduleCandidate, results []TargetSuiteRunResult) *targetSeedExpansionFeedback {
	if len(results) == 0 {
		return nil
	}
	contexts := make([]targetSeedExpansionContext, 0, len(results))
	for _, result := range results {
		candidate, ok := candidateByID[result.CandidateID]
		if !ok || candidate.SeedID == "" {
			continue
		}
		if !result.Confirmed && result.ActivationStage != TargetActivationStageActivationReached {
			continue
		}
		ctx := targetSeedExpansionContext{
			seedID:               candidate.SeedID,
			plantPrimitiveID:     candidate.PlantPrimitiveID,
			lifecycleOperationID: candidate.LifecycleOperationID,
			confirmed:            result.Confirmed,
			taskIDs:              map[string]struct{}{candidate.TaskID: struct{}{}},
			activationKinds:      make(map[string]struct{}, 1),
			oracleKinds:          make(map[string]struct{}, 1),
			mutationIDs:          make(map[string]struct{}, 1),
		}
		if candidate.ActivationKindID != "" {
			ctx.activationKinds[candidate.ActivationKindID] = struct{}{}
		}
		if candidate.OracleKindID != "" {
			ctx.oracleKinds[candidate.OracleKindID] = struct{}{}
		}
		if mutationID := targetCandidateMutationFocusID(candidate); mutationID != "" {
			ctx.mutationIDs[mutationID] = struct{}{}
		}
		contexts = append(contexts, ctx)
	}
	if len(contexts) == 0 {
		return nil
	}
	return &targetSeedExpansionFeedback{
		contexts: contexts,
		selected: make(map[string]struct{}),
	}
}

func targetSeedExpansionScore(candidate TargetScheduleCandidate, feedback *targetSeedExpansionFeedback) int {
	if feedback == nil || candidate.TaskID == "" || candidate.SeedID == "" {
		return 0
	}
	if _, ok := feedback.selected[candidate.TaskID]; ok {
		return 0
	}
	best := 0
	for _, ctx := range feedback.contexts {
		if candidate.SeedID != ctx.seedID {
			continue
		}
		if _, sameTask := ctx.taskIDs[candidate.TaskID]; sameTask {
			continue
		}
		score := 0
		if ctx.confirmed {
			score += 4
		} else {
			score += 2
		}
		if candidate.PlantPrimitiveID != "" && candidate.PlantPrimitiveID == ctx.plantPrimitiveID {
			score += 4
		}
		if candidate.LifecycleOperationID != "" && candidate.LifecycleOperationID == ctx.lifecycleOperationID {
			score += 3
		}
		if candidate.ActivationKindID != "" {
			if _, ok := ctx.activationKinds[candidate.ActivationKindID]; !ok {
				score += 3
			}
		}
		if candidate.OracleKindID != "" {
			if _, ok := ctx.oracleKinds[candidate.OracleKindID]; !ok {
				score += 2
			}
		}
		if mutationID := targetCandidateMutationFocusID(candidate); mutationID != "" {
			if _, ok := ctx.mutationIDs[mutationID]; !ok {
				score += 2
			}
		}
		if score > best {
			best = score
		}
	}
	return best
}

func targetConsumeSeedExpansion(feedback *targetSeedExpansionFeedback, candidate TargetScheduleCandidate) {
	if feedback == nil || candidate.TaskID == "" {
		return
	}
	if targetSeedExpansionScore(candidate, feedback) <= 0 {
		return
	}
	feedback.selected[candidate.TaskID] = struct{}{}
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
