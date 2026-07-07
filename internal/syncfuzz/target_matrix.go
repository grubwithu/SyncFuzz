package syncfuzz

import (
	"fmt"
	"sort"
	"strings"
)

type TargetMatrixOptions struct {
	TargetID         string
	Tasks            []string
	TaskGroups       []string
	PromptProfileIDs []string
}

type TargetScheduleCandidate struct {
	CandidateID              string                       `json:"candidate_id"`
	TargetID                 string                       `json:"target_id"`
	TaskID                   string                       `json:"task_id"`
	PromptProfileID          string                       `json:"prompt_profile_id,omitempty"`
	PromptProfileDescription string                       `json:"prompt_profile_description,omitempty"`
	Description              string                       `json:"description,omitempty"`
	DefaultExpectedFiles     []string                     `json:"default_expected_files,omitempty"`
	UsesLateObservation      bool                         `json:"uses_late_observation,omitempty"`
	DefaultLateObserveDelay  int64                        `json:"default_late_observe_delay_ms,omitempty"`
	Signature                MismatchSignature            `json:"signature"`
	ContractProfileID        string                       `json:"contract_profile_id,omitempty"`
	ContractRuleID           string                       `json:"contract_rule_id,omitempty"`
	ContractExpectation      TargetContractExpectation    `json:"contract_expectation,omitempty"`
	ContractSourceStrength   TargetContractSourceStrength `json:"contract_source_strength,omitempty"`
	StateSurface             string                       `json:"state_surface,omitempty"`
	LifecycleEdge            string                       `json:"lifecycle_edge,omitempty"`
}

type TargetScheduleMatrix struct {
	SchemaVersion   string                    `json:"schema_version"`
	TargetID        string                    `json:"target_id"`
	Tasks           []string                  `json:"tasks"`
	TaskGroups      []string                  `json:"task_groups,omitempty"`
	PromptProfiles  []string                  `json:"prompt_profiles,omitempty"`
	TotalCandidates int                       `json:"total_candidates"`
	Candidates      []TargetScheduleCandidate `json:"candidates"`
}

func BuildTargetScheduleMatrix(opts TargetMatrixOptions) (*TargetScheduleMatrix, error) {
	targetID := strings.TrimSpace(opts.TargetID)
	if targetID == "" {
		targetID = defaultTargetAdapterID
	}

	var (
		tasks      []string
		taskGroups []string
		err        error
	)
	if len(opts.Tasks) == 0 && len(opts.TaskGroups) == 0 {
		tasks = allTargetTaskIDs()
	} else {
		tasks, taskGroups, err = expandTargetTasks(opts.Tasks, opts.TaskGroups)
		if err != nil {
			return nil, err
		}
	}

	promptProfiles, err := resolveTargetPromptProfiles(opts.PromptProfileIDs)
	if err != nil {
		return nil, err
	}

	profile := targetContractProfile(targetID)
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
				DefaultExpectedFiles:     defaultTargetExpectedFiles(taskID),
				UsesLateObservation:      defaultTargetLateObserveDelay(taskID) > 0,
				DefaultLateObserveDelay:  defaultTargetLateObserveDelay(taskID).Milliseconds(),
				Signature:                targetSignature(taskID),
			}
			if ok {
				candidate.Description = taskInfo.Description
				if len(taskInfo.DefaultExpectedFiles) > 0 {
					candidate.DefaultExpectedFiles = append([]string{}, taskInfo.DefaultExpectedFiles...)
				}
				candidate.UsesLateObservation = taskInfo.UsesLateObservation
				candidate.StateSurface = taskInfo.StateSurface
				candidate.LifecycleEdge = taskInfo.LifecycleEdge
			} else {
				candidate.Description = "custom target task"
			}
			if profile != nil {
				if rule, ok := targetContractRule(profile, taskID); ok {
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
		PromptProfiles:  targetPromptProfileIDs(promptProfiles),
		TotalCandidates: len(candidates),
		Candidates:      candidates,
	}, nil
}

func targetScheduleCandidateID(targetID string, taskID string, promptProfileID string) string {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		targetID = defaultTargetAdapterID
	}
	promptProfileID = normalizeTargetPromptProfileID(promptProfileID)
	if promptProfileID == targetPromptProfileBaselineID {
		return strings.Join([]string{targetID, taskID}, "/")
	}
	return strings.Join([]string{targetID, taskID, promptProfileID}, "/")
}

func allTargetTaskIDs() []string {
	tasks := TargetTasks()
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.TaskID)
	}
	return out
}

func targetTaskInfoByID(taskID string) (TargetTaskInfo, bool) {
	for _, task := range TargetTasks() {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return TargetTaskInfo{}, false
}

func findTargetMatrixCandidate(matrix *TargetScheduleMatrix, taskID string) (TargetScheduleCandidate, error) {
	if matrix == nil {
		return TargetScheduleCandidate{}, fmt.Errorf("target schedule matrix is required")
	}
	for _, candidate := range matrix.Candidates {
		if candidate.TaskID == taskID && normalizeTargetPromptProfileID(candidate.PromptProfileID) == targetPromptProfileBaselineID {
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

func targetPromptProfileIDs(profiles []TargetPromptProfileInfo) []string {
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, profile.ProfileID)
	}
	return out
}
