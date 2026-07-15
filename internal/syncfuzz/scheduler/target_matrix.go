package scheduler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetMatrixOptions struct {
	TargetID         string
	Tasks            []string
	TaskGroups       []string
	SeedIDs          []string
	PromptProfileIDs []string
}

type TargetScheduleCandidate struct {
	CandidateID              string                              `json:"candidate_id"`
	TargetID                 string                              `json:"target_id"`
	ScenarioID               string                              `json:"scenario_id,omitempty"`
	SeedID                   string                              `json:"seed_id,omitempty"`
	TaskID                   string                              `json:"task_id"`
	PromptProfileID          string                              `json:"prompt_profile_id,omitempty"`
	PromptProfileDescription string                              `json:"prompt_profile_description,omitempty"`
	PromptVariantID          string                              `json:"prompt_variant_id,omitempty"`
	PromptVariantDescription string                              `json:"prompt_variant_description,omitempty"`
	Generated                bool                                `json:"generated,omitempty"`
	Description              string                              `json:"description,omitempty"`
	Objective                string                              `json:"objective,omitempty"`
	DefaultExpectedFiles     []string                            `json:"default_expected_files,omitempty"`
	UsesLateObservation      bool                                `json:"uses_late_observation,omitempty"`
	DefaultLateObserveDelay  int64                               `json:"default_late_observe_delay_ms,omitempty"`
	Signature                core.MismatchSignature              `json:"signature"`
	ContractProfileID        string                              `json:"contract_profile_id,omitempty"`
	ContractRuleID           string                              `json:"contract_rule_id,omitempty"`
	ContractExpectation      target.TargetContractExpectation    `json:"contract_expectation,omitempty"`
	ContractSourceStrength   target.TargetContractSourceStrength `json:"contract_source_strength,omitempty"`
	StateSurface             string                              `json:"state_surface,omitempty"`
	LifecycleEdge            string                              `json:"lifecycle_edge,omitempty"`
	LifecycleOperationID     string                              `json:"lifecycle_operation_id,omitempty"`
	Components               []target.TargetScenarioComponent    `json:"components,omitempty"`
	ExecutionPlan            *target.TargetScenarioExecutionPlan `json:"execution_plan,omitempty"`
	PlantPrimitiveID         string                              `json:"plant_primitive_id,omitempty"`
	ActivationKindID         string                              `json:"activation_kind_id,omitempty"`
	OracleKindID             string                              `json:"oracle_kind_id,omitempty"`
	MutationFocusID          string                              `json:"mutation_focus_id,omitempty"`
	MutationFocusKind        target.TargetScenarioMutationKind   `json:"mutation_focus_kind,omitempty"`
	Mutations                []target.TargetScenarioMutation     `json:"mutations,omitempty"`
}

type TargetScheduleMatrix struct {
	SchemaVersion   string                    `json:"schema_version"`
	TargetID        string                    `json:"target_id"`
	Tasks           []string                  `json:"tasks"`
	TaskGroups      []string                  `json:"task_groups,omitempty"`
	SeedIDs         []string                  `json:"seed_ids,omitempty"`
	PromptProfiles  []string                  `json:"prompt_profiles,omitempty"`
	TotalCandidates int                       `json:"total_candidates"`
	Candidates      []TargetScheduleCandidate `json:"candidates"`
}

func BuildTargetScheduleMatrix(opts TargetMatrixOptions) (*TargetScheduleMatrix, error) {
	targetID := strings.TrimSpace(opts.TargetID)
	if targetID == "" {
		targetID = target.DefaultTargetAdapterID
	}

	var (
		tasks      []string
		taskGroups []string
		seedIDs    []string
		err        error
	)
	if len(opts.Tasks) == 0 && len(opts.TaskGroups) == 0 && len(opts.SeedIDs) == 0 {
		tasks = allTargetTaskIDs()
	} else {
		tasks, taskGroups, seedIDs, err = target.ExpandTargetSelection(opts.Tasks, opts.TaskGroups, opts.SeedIDs)
		if err != nil {
			return nil, err
		}
	}

	promptProfiles, err := target.ResolveTargetPromptProfiles(opts.PromptProfileIDs)
	if err != nil {
		return nil, err
	}

	profile := target.TargetContractProfileFor(targetID)
	candidates := make([]TargetScheduleCandidate, 0, len(tasks)*len(promptProfiles)*2)
	for _, taskID := range tasks {
		taskInfo, ok := targetTaskInfoByID(taskID)
		for _, promptProfile := range promptProfiles {
			candidate := TargetScheduleCandidate{
				CandidateID:              targetScheduleCandidateID(targetID, taskID, promptProfile.ProfileID),
				TargetID:                 targetID,
				TaskID:                   taskID,
				PromptProfileID:          promptProfile.ProfileID,
				PromptProfileDescription: promptProfile.Description,
				PromptVariantID:          target.TargetPromptVariantBaseID,
				DefaultExpectedFiles:     target.DefaultTargetExpectedFiles(taskID),
				UsesLateObservation:      target.DefaultTargetLateObserveDelay(taskID) > 0,
				DefaultLateObserveDelay:  target.DefaultTargetLateObserveDelay(taskID).Milliseconds(),
				Signature:                target.TargetSignature(taskID),
			}
			if ok {
				candidate.ScenarioID = taskInfo.ScenarioID
				candidate.SeedID = taskInfo.SeedID
				candidate.Description = taskInfo.Description
				candidate.Objective = taskInfo.Objective
				if len(taskInfo.DefaultExpectedFiles) > 0 {
					candidate.DefaultExpectedFiles = append([]string{}, taskInfo.DefaultExpectedFiles...)
				}
				candidate.UsesLateObservation = taskInfo.UsesLateObservation
				candidate.StateSurface = taskInfo.StateSurface
				candidate.LifecycleEdge = taskInfo.LifecycleEdge
				candidate.LifecycleOperationID = taskInfo.LifecycleOperationID
				candidate.Components = append([]target.TargetScenarioComponent{}, taskInfo.Components...)
				if taskInfo.ExecutionPlan != nil {
					plan := *taskInfo.ExecutionPlan
					candidate.ExecutionPlan = &plan
				}
				candidate.PlantPrimitiveID = taskInfo.PlantPrimitiveID
				candidate.ActivationKindID = taskInfo.ActivationKindID
				candidate.OracleKindID = taskInfo.OracleKindID
				candidate.MutationFocusID = taskInfo.MutationFocusID
				candidate.MutationFocusKind = taskInfo.MutationFocusKind
				candidate.Mutations = append([]target.TargetScenarioMutation{}, taskInfo.Mutations...)
			} else {
				candidate.Description = "custom target task"
			}
			if variant, err := targetPromptVariantInfo(candidate.PromptVariantID); err == nil {
				candidate.PromptVariantDescription = variant.Description
			}
			if profile != nil {
				if rule, ok := target.TargetContractRuleFor(profile, taskID); ok {
					candidate.ContractProfileID = profile.ProfileID
					candidate.ContractRuleID = rule.RuleID
					candidate.ContractExpectation = rule.Expectation
					candidate.ContractSourceStrength = rule.SourceStrength
					candidate.StateSurface = rule.StateSurface
					candidate.LifecycleEdge = rule.LifecycleEdge
				}
			}
			candidates = append(candidates, candidate)
			if derived, ok := targetDerivedLifecycleBoundaryCandidate(candidate); ok {
				candidates = append(candidates, derived)
			}
			if derived, ok := targetDerivedMutationFocusCandidate(candidate); ok {
				candidates = append(candidates, derived)
			}
			if derived, ok := targetDerivedActivationFocusCandidate(candidate); ok {
				candidates = append(candidates, derived)
			}
			if derived, ok := targetDerivedProcessModePhaseShiftCandidate(candidate); ok {
				candidates = append(candidates, derived)
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CandidateID < candidates[j].CandidateID
	})
	return &TargetScheduleMatrix{
		SchemaVersion:   "syncfuzz.target-schedule-matrix.v1",
		TargetID:        targetID,
		Tasks:           append([]string{}, tasks...),
		TaskGroups:      append([]string{}, taskGroups...),
		SeedIDs:         append([]string{}, seedIDs...),
		PromptProfiles:  targetPromptProfileIDs(promptProfiles),
		TotalCandidates: len(candidates),
		Candidates:      candidates,
	}, nil
}

func targetScheduleCandidateID(targetID string, taskID string, promptProfileID string) string {
	return targetScheduleCandidateIDWithVariant(targetID, taskID, promptProfileID, target.TargetPromptVariantBaseID)
}

func targetScheduleCandidateIDWithVariant(targetID string, taskID string, promptProfileID string, promptVariantID string) string {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		targetID = target.DefaultTargetAdapterID
	}
	promptProfileID = target.NormalizeTargetPromptProfileID(promptProfileID)
	promptVariantID = target.NormalizeTargetPromptVariantID(promptVariantID)

	parts := []string{targetID, taskID}
	if promptProfileID != target.TargetPromptProfileBaselineID {
		parts = append(parts, promptProfileID)
	}
	if promptVariantID != target.TargetPromptVariantBaseID {
		parts = append(parts, promptVariantID)
	}
	return strings.Join(parts, "/")
}

func allTargetTaskIDs() []string {
	tasks := target.TargetTasks()
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.TaskID)
	}
	return out
}

func targetTaskInfoByID(taskID string) (target.TargetTaskInfo, bool) {
	return target.TargetTaskByID(taskID)
}

func targetPromptVariantInfo(variantID string) (target.TargetPromptVariantInfo, error) {
	for _, variant := range target.TargetPromptVariants() {
		if variant.VariantID == target.NormalizeTargetPromptVariantID(variantID) {
			return variant, nil
		}
	}
	return target.TargetPromptVariantInfo{}, fmt.Errorf("unknown target prompt variant %q", variantID)
}

func targetDerivedMutationFocusCandidate(base TargetScheduleCandidate) (TargetScheduleCandidate, bool) {
	if target.NormalizeTargetPromptVariantID(base.PromptVariantID) != target.TargetPromptVariantBaseID {
		return TargetScheduleCandidate{}, false
	}
	if base.MutationFocusID == "" {
		return TargetScheduleCandidate{}, false
	}

	derived := base
	derived.PromptVariantID = target.TargetPromptVariantMutationFocusID
	if variant, err := targetPromptVariantInfo(derived.PromptVariantID); err == nil {
		derived.PromptVariantDescription = variant.Description
	}
	derived.Generated = true
	derived.CandidateID = targetScheduleCandidateIDWithVariant(base.TargetID, base.TaskID, base.PromptProfileID, derived.PromptVariantID)
	return derived, true
}

func targetDerivedLifecycleBoundaryCandidate(base TargetScheduleCandidate) (TargetScheduleCandidate, bool) {
	if target.NormalizeTargetPromptVariantID(base.PromptVariantID) != target.TargetPromptVariantBaseID {
		return TargetScheduleCandidate{}, false
	}
	lifecycleOperationID := strings.TrimSpace(base.LifecycleOperationID)
	if lifecycleOperationID == "" || !strings.HasPrefix(lifecycleOperationID, "checkpoint-") {
		return TargetScheduleCandidate{}, false
	}

	derived := base
	derived.PromptVariantID = target.TargetPromptVariantLifecycleBoundaryID
	if variant, err := targetPromptVariantInfo(derived.PromptVariantID); err == nil {
		derived.PromptVariantDescription = variant.Description
	}
	derived.Generated = true
	derived.CandidateID = targetScheduleCandidateIDWithVariant(base.TargetID, base.TaskID, base.PromptProfileID, derived.PromptVariantID)
	return derived, true
}

func targetDerivedActivationFocusCandidate(base TargetScheduleCandidate) (TargetScheduleCandidate, bool) {
	if target.NormalizeTargetPromptVariantID(base.PromptVariantID) != target.TargetPromptVariantBaseID {
		return TargetScheduleCandidate{}, false
	}
	if strings.TrimSpace(base.ActivationKindID) == "" || !targetCandidateHasActivationMutation(base) {
		return TargetScheduleCandidate{}, false
	}

	derived := base
	derived.PromptVariantID = target.TargetPromptVariantActivationFocusID
	if variant, err := targetPromptVariantInfo(derived.PromptVariantID); err == nil {
		derived.PromptVariantDescription = variant.Description
	}
	derived.Generated = true
	derived.CandidateID = targetScheduleCandidateIDWithVariant(base.TargetID, base.TaskID, base.PromptProfileID, derived.PromptVariantID)
	return derived, true
}

func targetCandidateHasActivationMutation(candidate TargetScheduleCandidate) bool {
	if candidate.MutationFocusKind == target.TargetScenarioMutationActivationSubstitution {
		return true
	}
	for _, mutation := range candidate.Mutations {
		if mutation.Kind == target.TargetScenarioMutationActivationSubstitution {
			return true
		}
	}
	return false
}

func targetDerivedProcessModePhaseShiftCandidate(base TargetScheduleCandidate) (TargetScheduleCandidate, bool) {
	if target.NormalizeTargetPromptVariantID(base.PromptVariantID) != target.TargetPromptVariantBaseID || base.ExecutionPlan == nil {
		return TargetScheduleCandidate{}, false
	}
	if strings.TrimSpace(base.ExecutionPlan.ProcessMode) != "split-process" {
		return TargetScheduleCandidate{}, false
	}

	const mutationID = "phase-shift.process-mode.single-process"
	derived := base
	plan := *base.ExecutionPlan
	plan.ProcessMode = "single"
	derived.ExecutionPlan = &plan
	derived.Generated = true
	derived.MutationFocusID = mutationID
	derived.MutationFocusKind = target.TargetScenarioMutationPhaseShift
	derived.Mutations = append(append([]target.TargetScenarioMutation{}, base.Mutations...), target.TargetScenarioMutation{
		MutationID: mutationID,
		Kind:       target.TargetScenarioMutationPhaseShift,
		Summary:    "run the same checkpoint operation in one runtime process instead of splitting initial and resumed phases",
	})
	derived.CandidateID = base.CandidateID + "/phase-shift-single-process"
	return derived, true
}

func findTargetMatrixCandidate(matrix *TargetScheduleMatrix, taskID string) (TargetScheduleCandidate, error) {
	if matrix == nil {
		return TargetScheduleCandidate{}, fmt.Errorf("target schedule matrix is required")
	}
	for _, candidate := range matrix.Candidates {
		if candidate.TaskID == taskID && target.NormalizeTargetPromptProfileID(candidate.PromptProfileID) == target.TargetPromptProfileBaselineID {
			return candidate, nil
		}
	}
	for _, candidate := range matrix.Candidates {
		if candidate.TaskID == taskID {
			return candidate, nil
		}
	}
	return TargetScheduleCandidate{}, fmt.Errorf("target schedule candidate for task %q not found", taskID)
}

func targetCandidateTaskIDs(candidates []TargetScheduleCandidate) []string {
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.TaskID]; ok {
			continue
		}
		seen[candidate.TaskID] = struct{}{}
		out = append(out, candidate.TaskID)
	}
	return out
}

func targetPromptProfileIDs(profiles []target.TargetPromptProfileInfo) []string {
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, profile.ProfileID)
	}
	return out
}
