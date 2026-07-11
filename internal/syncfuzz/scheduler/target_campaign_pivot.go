package scheduler

import (
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetCampaignPivotRecommendation struct {
	Dimension string   `json:"dimension"`
	Values    []string `json:"values,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

type targetCampaignPivotOption struct {
	NextOpts          TargetCampaignOptions
	NextUniverse      *TargetScheduleMatrix
	Event             *TargetCampaignPivotEvent
	TopFrontier       *TargetFrontierCandidate
	NewCandidateCount int
	ValueRank         int
}

func summarizeTargetCampaignPivotRecommendations(universe *TargetScheduleMatrix) ([]TargetCampaignPivotRecommendation, bool) {
	if universe == nil {
		return nil, false
	}

	current := map[string]map[string]struct{}{
		"seed_id":            {},
		"prompt_profile_id":  {},
		"prompt_variant_id":  {},
		"state_surface":      {},
		"plant_primitive_id": {},
		"activation_kind_id": {},
		"oracle_kind_id":     {},
	}
	for _, candidate := range universe.Candidates {
		targetAddPivotValue(current["seed_id"], candidate.SeedID)
		targetAddPivotValue(current["prompt_profile_id"], candidate.PromptProfileID)
		targetAddPivotValue(current["prompt_variant_id"], target.NormalizeTargetPromptVariantID(candidate.PromptVariantID))
		targetAddPivotValue(current["state_surface"], candidate.StateSurface)
		targetAddPivotValue(current["plant_primitive_id"], candidate.PlantPrimitiveID)
		targetAddPivotValue(current["activation_kind_id"], candidate.ActivationKindID)
		targetAddPivotValue(current["oracle_kind_id"], candidate.OracleKindID)
	}

	global := map[string][]string{
		"seed_id":            targetPivotSeedValues(),
		"prompt_profile_id":  targetPivotPromptProfileValues(),
		"prompt_variant_id":  targetPivotPromptVariantValues(),
		"state_surface":      targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.StateSurface }),
		"plant_primitive_id": targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.PlantPrimitiveID }),
		"activation_kind_id": targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.ActivationKindID }),
		"oracle_kind_id":     targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.OracleKindID }),
	}

	order := []string{
		"seed_id",
		"prompt_profile_id",
		"prompt_variant_id",
		"state_surface",
		"plant_primitive_id",
		"activation_kind_id",
		"oracle_kind_id",
	}
	recommendations := make([]TargetCampaignPivotRecommendation, 0, len(order))
	for _, dimension := range order {
		values := targetMissingCatalogValues(global[dimension], current[dimension])
		if len(values) == 0 {
			continue
		}
		recommendations = append(recommendations, TargetCampaignPivotRecommendation{
			Dimension: dimension,
			Values:    values,
			Reason:    targetPivotReason(dimension),
		})
	}
	return recommendations, len(recommendations) == 0
}

func targetPivotSeedValues() []string {
	seeds := target.TargetScenarioSeeds()
	values := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		targetAppendPivotValue(&values, seed.SeedID)
	}
	sort.Strings(values)
	return values
}

func targetPivotPromptProfileValues() []string {
	profiles := target.TargetPromptProfiles()
	values := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		targetAppendPivotValue(&values, profile.ProfileID)
	}
	sort.Strings(values)
	return values
}

func targetPivotTaskDimensionValues(selector func(target.TargetTaskInfo) string) []string {
	tasks := target.TargetTasks()
	values := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		value := selector(task)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func targetPivotPromptVariantValues() []string {
	variants := target.TargetPromptVariants()
	values := make([]string, 0, len(variants))
	for _, variant := range variants {
		targetAppendPivotValue(&values, variant.VariantID)
	}
	sort.Strings(values)
	return values
}

func targetMissingCatalogValues(global []string, current map[string]struct{}) []string {
	out := make([]string, 0, len(global))
	for _, value := range global {
		if _, ok := current[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func targetPivotReason(dimension string) string {
	switch dimension {
	case "seed_id":
		return "expand into additional built-in scenario seed families"
	case "prompt_profile_id":
		return "rotate into additional deterministic prompt profiles"
	case "prompt_variant_id":
		return "rotate into additional deterministic prompt variants derived from scenario structure"
	case "state_surface":
		return "shift to a different OS or workspace state surface"
	case "plant_primitive_id":
		return "try different residue-planting primitives"
	case "activation_kind_id":
		return "exercise different trusted activation styles"
	case "oracle_kind_id":
		return "probe with different witness and oracle shapes"
	default:
		return "expand the current campaign universe"
	}
}

func targetCampaignBestPivotOption(
	opts TargetCampaignOptions,
	currentUniverse *TargetScheduleMatrix,
	previousResults []TargetSuiteRunResult,
	recommendation TargetCampaignPivotRecommendation,
	afterRound int,
) (TargetCampaignOptions, *TargetScheduleMatrix, *TargetCampaignPivotEvent, bool, error) {
	options := make([]targetCampaignPivotOption, 0, len(recommendation.Values))
	for _, value := range recommendation.Values {
		nextOpts, ok := targetCampaignPivotExpandValue(opts, recommendation.Dimension, value)
		if !ok {
			continue
		}
		nextUniverse, err := buildTargetCampaignUniverse(nextOpts)
		if err != nil {
			return TargetCampaignOptions{}, nil, nil, false, err
		}
		if !targetScheduleUniverseExpanded(currentUniverse, nextUniverse) {
			continue
		}
		frontier := summarizeTargetCoverageFrontier(nextUniverse, previousResults, nil, 1)
		option := targetCampaignPivotOption{
			NextOpts:          nextOpts,
			NextUniverse:      nextUniverse,
			NewCandidateCount: targetCampaignNewCandidateCount(currentUniverse, nextUniverse),
			ValueRank:         targetCampaignPivotValueRank(recommendation.Dimension, value),
			Event: &TargetCampaignPivotEvent{
				AfterRound:     afterRound,
				Dimension:      recommendation.Dimension,
				Values:         []string{value},
				Tasks:          append([]string{}, nextUniverse.Tasks...),
				SeedIDs:        append([]string{}, nextUniverse.SeedIDs...),
				PromptProfiles: append([]string{}, nextUniverse.PromptProfiles...),
				Reason:         recommendation.Reason,
			},
		}
		if len(frontier) > 0 {
			frontierTop := frontier[0]
			option.TopFrontier = &frontierTop
			option.Event.FrontierCandidate = frontierTop.CandidateID
			option.Event.FrontierGapScore = frontierTop.GapScore
			option.Event.FrontierNovelty = frontierTop.NoveltyScore
			option.Event.FrontierSelection = frontierTop.SelectionMode
		}
		option.Event.NewCandidateCount = option.NewCandidateCount
		options = append(options, option)
	}
	if len(options) == 0 {
		return TargetCampaignOptions{}, nil, nil, false, nil
	}
	sort.Slice(options, func(i, j int) bool {
		return targetCampaignPivotOptionLess(options[i], options[j])
	})
	best := options[0]
	return best.NextOpts, best.NextUniverse, best.Event, true, nil
}

func targetCampaignPivotExpandValue(opts TargetCampaignOptions, dimension string, value string) (TargetCampaignOptions, bool) {
	if value == "" {
		return TargetCampaignOptions{}, false
	}
	nextOpts := opts
	switch dimension {
	case "seed_id":
		nextOpts.SeedIDs = mergeStringLists(opts.SeedIDs, []string{value})
	case "prompt_profile_id":
		nextOpts.PromptProfileID = ""
		nextOpts.PromptProfileIDs = mergeStringLists(target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs), []string{value})
	case "prompt_variant_id":
		return TargetCampaignOptions{}, false
	case "state_surface", "plant_primitive_id", "activation_kind_id", "oracle_kind_id":
		nextOpts.Tasks = mergeStringLists(opts.Tasks, targetTaskIDsForDimensionValues(dimension, []string{value}))
	default:
		return TargetCampaignOptions{}, false
	}
	return nextOpts, true
}

func targetCampaignPivotOptionLess(left targetCampaignPivotOption, right targetCampaignPivotOption) bool {
	leftGapScore, leftNovelty := targetCampaignPivotFrontierScore(left.TopFrontier)
	rightGapScore, rightNovelty := targetCampaignPivotFrontierScore(right.TopFrontier)
	if leftGapScore != rightGapScore {
		return leftGapScore > rightGapScore
	}
	if leftNovelty != rightNovelty {
		return leftNovelty > rightNovelty
	}
	if left.NewCandidateCount != right.NewCandidateCount {
		return left.NewCandidateCount < right.NewCandidateCount
	}
	if left.ValueRank != right.ValueRank {
		return left.ValueRank < right.ValueRank
	}
	if left.Event.Dimension != right.Event.Dimension {
		return left.Event.Dimension < right.Event.Dimension
	}
	if len(left.Event.Values) == 0 || len(right.Event.Values) == 0 {
		return len(left.Event.Values) < len(right.Event.Values)
	}
	return left.Event.Values[0] < right.Event.Values[0]
}

func targetCampaignPivotFrontierScore(frontier *TargetFrontierCandidate) (int, int) {
	if frontier == nil {
		return 0, 0
	}
	return frontier.GapScore, frontier.NoveltyScore
}

func targetCampaignNewCandidateCount(current *TargetScheduleMatrix, next *TargetScheduleMatrix) int {
	if next == nil {
		return 0
	}
	if current == nil {
		return len(next.Candidates)
	}
	seen := make(map[string]struct{}, len(current.Candidates))
	for _, candidate := range current.Candidates {
		seen[candidate.CandidateID] = struct{}{}
	}
	count := 0
	for _, candidate := range next.Candidates {
		if _, ok := seen[candidate.CandidateID]; ok {
			continue
		}
		count++
	}
	return count
}

func targetCampaignPivotValueRank(dimension string, value string) int {
	switch dimension {
	case "prompt_profile_id":
		return targetPromptProfileRank(value)
	case "prompt_variant_id":
		return targetPromptVariantRank(value)
	default:
		return 0
	}
}

func targetPromptVariantRank(variantID string) int {
	switch target.NormalizeTargetPromptVariantID(variantID) {
	case target.TargetPromptVariantBaseID:
		return 0
	case target.TargetPromptVariantLifecycleBoundaryID:
		return 1
	case target.TargetPromptVariantMutationFocusID:
		return 2
	default:
		return 3
	}
}

func targetAddPivotValue(dst map[string]struct{}, value string) {
	if value == "" {
		return
	}
	dst[value] = struct{}{}
}

func targetAppendPivotValue(dst *[]string, value string) {
	if value == "" {
		return
	}
	*dst = append(*dst, value)
}
