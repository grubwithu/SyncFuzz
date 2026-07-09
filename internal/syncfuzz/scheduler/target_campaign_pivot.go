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

func summarizeTargetCampaignPivotRecommendations(universe *TargetScheduleMatrix) ([]TargetCampaignPivotRecommendation, bool) {
	if universe == nil {
		return nil, false
	}

	current := map[string]map[string]struct{}{
		"seed_id":            {},
		"prompt_profile_id":  {},
		"state_surface":      {},
		"plant_primitive_id": {},
		"activation_kind_id": {},
		"oracle_kind_id":     {},
	}
	for _, candidate := range universe.Candidates {
		targetAddPivotValue(current["seed_id"], candidate.SeedID)
		targetAddPivotValue(current["prompt_profile_id"], candidate.PromptProfileID)
		targetAddPivotValue(current["state_surface"], candidate.StateSurface)
		targetAddPivotValue(current["plant_primitive_id"], candidate.PlantPrimitiveID)
		targetAddPivotValue(current["activation_kind_id"], candidate.ActivationKindID)
		targetAddPivotValue(current["oracle_kind_id"], candidate.OracleKindID)
	}

	global := map[string][]string{
		"seed_id":            targetPivotSeedValues(),
		"prompt_profile_id":  targetPivotPromptProfileValues(),
		"state_surface":      targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.StateSurface }),
		"plant_primitive_id": targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.PlantPrimitiveID }),
		"activation_kind_id": targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.ActivationKindID }),
		"oracle_kind_id":     targetPivotTaskDimensionValues(func(task target.TargetTaskInfo) string { return task.OracleKindID }),
	}

	order := []string{
		"seed_id",
		"prompt_profile_id",
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
