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
	Description              string                              `json:"description,omitempty"`
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
	PlantPrimitiveID         string                              `json:"plant_primitive_id,omitempty"`
	ActivationKindID         string                              `json:"activation_kind_id,omitempty"`
	OracleKindID             string                              `json:"oracle_kind_id,omitempty"`
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
	candidates := make([]TargetScheduleCandidate, 0, len(tasks)*len(promptProfiles))
	for _, taskID := range tasks {
		taskInfo, ok := targetTaskInfoByID(taskID)
		for _, promptProfile := range promptProfiles {
			candidate := TargetScheduleCandidate{
				CandidateID:              targetScheduleCandidateID(targetID, taskID, promptProfile.ProfileID),
				TargetID:                 targetID,
				TaskID:                   taskID,
				PromptProfileID:          promptProfile.ProfileID,
				PromptProfileDescription: promptProfile.Description,
				DefaultExpectedFiles:     target.DefaultTargetExpectedFiles(taskID),
				UsesLateObservation:      target.DefaultTargetLateObserveDelay(taskID) > 0,
				DefaultLateObserveDelay:  target.DefaultTargetLateObserveDelay(taskID).Milliseconds(),
				Signature:                target.TargetSignature(taskID),
			}
			if ok {
				candidate.ScenarioID = taskInfo.ScenarioID
				candidate.SeedID = taskInfo.SeedID
				candidate.Description = taskInfo.Description
				if len(taskInfo.DefaultExpectedFiles) > 0 {
					candidate.DefaultExpectedFiles = append([]string{}, taskInfo.DefaultExpectedFiles...)
				}
				candidate.UsesLateObservation = taskInfo.UsesLateObservation
				candidate.StateSurface = taskInfo.StateSurface
				candidate.LifecycleEdge = taskInfo.LifecycleEdge
				candidate.LifecycleOperationID = taskInfo.LifecycleOperationID
				candidate.PlantPrimitiveID = taskInfo.PlantPrimitiveID
				candidate.ActivationKindID = taskInfo.ActivationKindID
				candidate.OracleKindID = taskInfo.OracleKindID
				candidate.Mutations = append([]target.TargetScenarioMutation{}, taskInfo.Mutations...)
			} else {
				candidate.Description = "custom target task"
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
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		targetID = target.DefaultTargetAdapterID
	}
	promptProfileID = target.NormalizeTargetPromptProfileID(promptProfileID)
	if promptProfileID == target.TargetPromptProfileBaselineID {
		return strings.Join([]string{targetID, taskID}, "/")
	}
	return strings.Join([]string{targetID, taskID, promptProfileID}, "/")
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
	for _, task := range target.TargetTasks() {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return target.TargetTaskInfo{}, false
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
