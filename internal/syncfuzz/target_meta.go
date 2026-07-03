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
}

type TargetTaskGroupInfo struct {
	GroupID     string   `json:"group_id"`
	Description string   `json:"description"`
	Tasks       []string `json:"tasks"`
}

func TargetTasks() []TargetTaskInfo {
	tasks := []TargetTaskInfo{
		{
			TaskID:               defaultTargetTaskID,
			Description:          "launch a delayed background effect and confirm the resulting late-effect file",
			DefaultExpectedFiles: []string{"late-effect"},
		},
		{
			TaskID:              longDelayTargetTaskID,
			Description:         "launch a longer-lived background process and confirm boundary process evidence plus a late-effect during delayed observation",
			UsesLateObservation: true,
		},
		{
			TaskID:               persistentShellTargetTaskID,
			Description:          "prepend a workspace-local tool directory inside a persistent shell session and capture the resolved git path",
			DefaultExpectedFiles: []string{"shell-poison-check.txt"},
		},
		{
			TaskID:               persistentShellReplayTargetTaskID,
			Description:          "replay from a pre-export checkpoint and detect whether a workspace-local PATH override survives in the persistent shell",
			DefaultExpectedFiles: []string{"shell-poison-replay-check.txt", "langgraph-replay-summary.json"},
		},
		{
			TaskID:               persistentShellForkTargetTaskID,
			Description:          "fork from a pre-export checkpoint and detect whether a workspace-local PATH override is inherited in the persistent shell",
			DefaultExpectedFiles: []string{"shell-poison-fork-check.txt", "langgraph-fork-summary.json"},
		},
	}
	for _, spec := range workspaceResidueTaskSpecs() {
		tasks = append(tasks, TargetTaskInfo{
			TaskID:               spec.TaskID,
			Description:          spec.Description,
			DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
		})
	}
	return tasks
}

func TargetTaskGroups() []TargetTaskGroupInfo {
	return []TargetTaskGroupInfo{
		{
			GroupID:     "phase5a-baseline",
			Description: "current Phase 5A LangGraph baseline covering delayed process, persistent shell, replay/fork shell checks, and workspace residue forks",
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
			Description: "fork-based filesystem residue tasks covering regular files, directories, deletion state, and symlinks",
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
