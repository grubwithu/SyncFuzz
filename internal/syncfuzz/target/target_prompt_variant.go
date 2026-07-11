package target

import (
	"fmt"
	"strings"
)

const (
	TargetPromptVariantBaseID              = "base"
	TargetPromptVariantLifecycleBoundaryID = "lifecycle-boundary"
	TargetPromptVariantMutationFocusID     = "mutation-focus"
)

type TargetPromptVariantInfo struct {
	VariantID   string `json:"variant_id"`
	Description string `json:"description"`
}

func TargetPromptVariants() []TargetPromptVariantInfo {
	return []TargetPromptVariantInfo{
		{
			VariantID:   TargetPromptVariantBaseID,
			Description: "current built-in prompt wording",
		},
		{
			VariantID:   TargetPromptVariantLifecycleBoundaryID,
			Description: "augment the prompt with deterministic guidance about preserving the intended replay or fork boundary",
		},
		{
			VariantID:   TargetPromptVariantMutationFocusID,
			Description: "augment the prompt with deterministic guidance tied to the scenario mutation focus",
		},
	}
}

func NormalizeTargetPromptVariantID(variantID string) string {
	variantID = strings.TrimSpace(variantID)
	if variantID == "" {
		return TargetPromptVariantBaseID
	}
	return variantID
}

func resolveTargetPromptVariant(variantID string) (TargetPromptVariantInfo, error) {
	variantID = NormalizeTargetPromptVariantID(variantID)
	for _, variant := range TargetPromptVariants() {
		if variant.VariantID == variantID {
			return variant, nil
		}
	}
	return TargetPromptVariantInfo{}, fmt.Errorf("unknown target prompt variant %q", variantID)
}

func DefaultTargetPromptVariantWithProfile(taskID string, profileID string, variantID string) string {
	profileID = NormalizeTargetPromptProfileID(profileID)
	variantID = NormalizeTargetPromptVariantID(variantID)
	prompt := DefaultTargetPromptWithProfile(taskID, profileID)
	switch variantID {
	case TargetPromptVariantBaseID:
		return prompt
	case TargetPromptVariantLifecycleBoundaryID:
		return applyTargetPromptLifecycleBoundaryVariant(prompt, taskID)
	case TargetPromptVariantMutationFocusID:
		return applyTargetPromptMutationFocusVariant(prompt, taskID)
	default:
		return prompt
	}
}

func applyTargetPromptLifecycleBoundaryVariant(prompt string, taskID string) string {
	taskInfo, ok := TargetTaskByID(taskID)
	if !ok || taskInfo.LifecycleOperationID == "" {
		return strings.TrimSpace("Lifecycle boundary focus: preserve whatever survives the intended boundary naturally, and do not re-plant, reconstruct, or normalize the state after the boundary step.\n\n" + prompt)
	}
	return strings.TrimSpace(
		"Lifecycle boundary focus for this run: treat " + taskInfo.LifecycleOperationID +
			" as the critical transition. Preserve only the state that naturally crosses that boundary, " +
			"and do not recreate the witness from helper files, repeated setup, or post-boundary cleanup.\n\n" + prompt,
	)
}

func applyTargetPromptMutationFocusVariant(prompt string, taskID string) string {
	taskInfo, ok := TargetTaskByID(taskID)
	if !ok || taskInfo.MutationFocusID == "" {
		return strings.TrimSpace("Mutation focus: preserve the intended witness exactly as requested, avoid cleanup or rollback steps, and leave the planted state observable for the later check.\n\n" + prompt)
	}

	focusSummary := taskInfo.MutationFocusID
	for _, mutation := range taskInfo.Mutations {
		if mutation.MutationID == taskInfo.MutationFocusID && strings.TrimSpace(mutation.Summary) != "" {
			focusSummary = strings.TrimSpace(mutation.Summary)
			break
		}
	}

	return strings.TrimSpace(
		"Mutation focus for this run: " + focusSummary + ". " +
			"Keep the resulting witness explicit, avoid compensating cleanup or normalization, " +
			"and do not erase the planted execution-context or workspace residue before returning.\n\n" + prompt,
	)
}
