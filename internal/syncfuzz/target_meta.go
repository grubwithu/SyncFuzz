package syncfuzz

type TargetTaskInfo struct {
	TaskID               string   `json:"task_id"`
	Description          string   `json:"description"`
	DefaultExpectedFiles []string `json:"default_expected_files,omitempty"`
	UsesLateObservation  bool     `json:"uses_late_observation,omitempty"`
}

func TargetTasks() []TargetTaskInfo {
	return []TargetTaskInfo{
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
			Description:          "poison PATH inside a persistent shell session and capture the resolved git path",
			DefaultExpectedFiles: []string{"shell-poison-check.txt"},
		},
		{
			TaskID:               persistentShellReplayTargetTaskID,
			Description:          "replay from a pre-export checkpoint and detect duplicated attacker-bin PATH residue in the persistent shell",
			DefaultExpectedFiles: []string{"shell-poison-replay-check.txt", "langgraph-replay-summary.json"},
		},
		{
			TaskID:               persistentShellForkTargetTaskID,
			Description:          "fork from a pre-export checkpoint and detect inherited attacker-bin PATH residue in the persistent shell",
			DefaultExpectedFiles: []string{"shell-poison-fork-check.txt", "langgraph-fork-summary.json"},
		},
		{
			TaskID:               fileResidueForkTargetTaskID,
			Description:          "fork from a pre-write checkpoint and observe whether branch-note.txt still exists in the workspace",
			DefaultExpectedFiles: []string{"file-residue-fork-check.txt", "langgraph-fork-summary.json"},
		},
		{
			TaskID:               deleteResidueForkTargetTaskID,
			Description:          "fork from a pre-delete checkpoint and observe whether branch-delete-note.txt wrongly stays absent in the workspace",
			DefaultExpectedFiles: []string{"delete-residue-fork-check.txt", "langgraph-fork-summary.json"},
		},
		{
			TaskID:               symlinkResidueForkTargetTaskID,
			Description:          "fork from a pre-symlink checkpoint and observe whether branch-link.txt still exists in the workspace",
			DefaultExpectedFiles: []string{"symlink-residue-fork-check.txt", "langgraph-fork-summary.json"},
		},
	}
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
