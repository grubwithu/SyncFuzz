package syncfuzz

import (
	"fmt"
	"strings"
)

type TargetTaskInfo struct {
	TaskID               string   `json:"task_id"`
	Description          string   `json:"description"`
	DefaultExpectedFiles []string `json:"default_expected_files,omitempty"`
	UsesLateObservation  bool     `json:"uses_late_observation,omitempty"`
	StateSurface         string   `json:"state_surface,omitempty"`
	LifecycleEdge        string   `json:"lifecycle_edge,omitempty"`
}

type TargetTaskGroupInfo struct {
	GroupID     string   `json:"group_id"`
	Description string   `json:"description"`
	Tasks       []string `json:"tasks"`
}

func TargetTasks() []TargetTaskInfo {
	scenarios := TargetScenarios()
	tasks := make([]TargetTaskInfo, 0, len(scenarios))
	for _, scenario := range scenarios {
		tasks = append(tasks, TargetTaskInfo{
			TaskID:               scenario.TaskID,
			Description:          scenario.Description,
			DefaultExpectedFiles: append([]string{}, scenario.DefaultExpectedFiles...),
			UsesLateObservation:  scenario.UsesLateObservation,
			StateSurface:         scenario.StateSurface,
			LifecycleEdge:        scenario.LifecycleEdge,
		})
	}
	return tasks
}

func TargetTaskGroups() []TargetTaskGroupInfo {
	return []TargetTaskGroupInfo{
		{
			GroupID:     "phase5a-baseline",
			Description: "current Phase 5A LangGraph baseline covering delayed process, persistent shell, replay/fork shell checks, and workspace residue forks across multiple OS state surfaces",
			Tasks: append([]string{
				longDelayTargetTaskID,
				persistentShellTargetTaskID,
				persistentShellReplayTargetTaskID,
				persistentShellForkTargetTaskID,
			}, workspaceResidueTaskIDs()...),
		},
		{
			GroupID:     "shell-lifecycle",
			Description: "persistent-shell and replay/fork lifecycle tasks for PATH residue experiments",
			Tasks: []string{
				persistentShellTargetTaskID,
				persistentShellReplayTargetTaskID,
				persistentShellForkTargetTaskID,
			},
		},
		{
			GroupID:     "workspace-residue",
			Description: "fork-based workspace residue tasks covering filesystem objects plus open-fd, inherited-fd, and Unix listener capability residue",
			Tasks:       workspaceResidueTaskIDs(),
		},
	}
}

func expandTargetTasks(taskIDs, groupIDs []string) ([]string, []string, error) {
	var expanded []string
	var normalizedGroups []string
	seenTasks := make(map[string]struct{})
	seenGroups := make(map[string]struct{})
	groupCatalog := make(map[string]TargetTaskGroupInfo, len(TargetTaskGroups()))
	for _, group := range TargetTaskGroups() {
		groupCatalog[group.GroupID] = group
	}

	appendTask := func(taskID string) {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			return
		}
		if _, ok := seenTasks[taskID]; ok {
			return
		}
		seenTasks[taskID] = struct{}{}
		expanded = append(expanded, taskID)
	}

	for _, groupID := range groupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		group, ok := groupCatalog[groupID]
		if !ok {
			return nil, nil, fmt.Errorf("unknown target task group %q", groupID)
		}
		if _, ok := seenGroups[groupID]; !ok {
			seenGroups[groupID] = struct{}{}
			normalizedGroups = append(normalizedGroups, groupID)
		}
		for _, taskID := range group.Tasks {
			appendTask(taskID)
		}
	}

	for _, taskID := range taskIDs {
		appendTask(taskID)
	}

	if len(expanded) == 0 {
		return []string{defaultTargetTaskID}, normalizedGroups, nil
	}
	return expanded, normalizedGroups, nil
}

func targetSignature(taskID string) MismatchSignature {
	return MismatchSignature{
		LifecycleEvent: "real-target-run",
		FaultPhase:     "target-command",
		StateClass:     "workspace",
		Operation:      taskID,
		Relation:       "observation-only",
		Impact:         "target-adapter",
	}
}
