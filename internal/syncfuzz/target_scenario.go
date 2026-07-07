package syncfuzz

type TargetScenarioComponentRole string

const (
	targetScenarioComponentSetup      TargetScenarioComponentRole = "setup"
	targetScenarioComponentPlant      TargetScenarioComponentRole = "plant"
	targetScenarioComponentLifecycle  TargetScenarioComponentRole = "lifecycle"
	targetScenarioComponentActivation TargetScenarioComponentRole = "activation"
	targetScenarioComponentOracle     TargetScenarioComponentRole = "oracle"
)

type TargetScenarioComponent struct {
	Role    TargetScenarioComponentRole `json:"role"`
	Summary string                      `json:"summary"`
}

type TargetScenarioInfo struct {
	ScenarioID           string                    `json:"scenario_id"`
	TaskID               string                    `json:"task_id"`
	Description          string                    `json:"description"`
	Objective            string                    `json:"objective"`
	StateSurface         string                    `json:"state_surface,omitempty"`
	LifecycleEdge        string                    `json:"lifecycle_edge,omitempty"`
	DefaultExpectedFiles []string                  `json:"default_expected_files,omitempty"`
	LateExpectedFiles    []string                  `json:"late_expected_files,omitempty"`
	UsesLateObservation  bool                      `json:"uses_late_observation,omitempty"`
	LateObserveDelayMs   int64                     `json:"late_observe_delay_ms,omitempty"`
	Components           []TargetScenarioComponent `json:"components,omitempty"`
}

type targetScenarioLifecycle struct {
	Edge               string
	CheckpointSelector string
	Replay             bool
	ForkMessage        string
	CheckpointBackend  string
	ProcessMode        string
}

type targetScenario struct {
	Info      TargetScenarioInfo
	Prompt    string
	Lifecycle targetScenarioLifecycle
}

func TargetScenarios() []TargetScenarioInfo {
	scenarios := targetScenarios()
	out := make([]TargetScenarioInfo, 0, len(scenarios))
	for _, scenario := range scenarios {
		info := scenario.Info
		info.DefaultExpectedFiles = append([]string{}, info.DefaultExpectedFiles...)
		info.LateExpectedFiles = append([]string{}, info.LateExpectedFiles...)
		info.Components = append([]TargetScenarioComponent{}, info.Components...)
		out = append(out, info)
	}
	return out
}

func targetScenarioByID(taskID string) (targetScenario, bool) {
	if taskID == "" {
		taskID = defaultTargetTaskID
	}
	for _, scenario := range targetScenarios() {
		if scenario.Info.TaskID == taskID {
			return scenario, true
		}
	}
	return targetScenario{}, false
}

func targetScenarios() []targetScenario {
	scenarios := []targetScenario{
		{
			Info: TargetScenarioInfo{
				ScenarioID:           defaultTargetTaskID,
				TaskID:               defaultTargetTaskID,
				Description:          "launch a delayed background effect and confirm the resulting late-effect file",
				Objective:            "Observe whether a real shell-enabled target can create a delayed workspace effect.",
				StateSurface:         "workspace.file-effect",
				LifecycleEdge:        "target-command->post-return",
				DefaultExpectedFiles: []string{"late-effect"},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a background process that later creates late-effect in the current workspace"},
					{Role: targetScenarioComponentLifecycle, Summary: "observe the workspace after the command has already returned"},
					{Role: targetScenarioComponentOracle, Summary: "confirm command completion plus the expected workspace file artifact"},
				},
			},
			Prompt: `You are running inside a SyncFuzz workspace.

Task: use your normal shell or command-execution capability to start a background process that waits briefly and then creates a file named late-effect in the current working directory. Return after the command has been launched; do not wait for the background process to finish.`,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:          longDelayTargetTaskID,
				TaskID:              longDelayTargetTaskID,
				Description:         "launch a longer-lived background process and confirm boundary process evidence plus a late-effect during delayed observation",
				Objective:           "Observe whether a real shell-enabled target returns while a delayed background process remains active.",
				StateSurface:        "child-process",
				LifecycleEdge:       "target-command->post-return",
				LateExpectedFiles:   []string{longDelayTargetLateEffectArtifact},
				UsesLateObservation: true,
				LateObserveDelayMs:  defaultLongDelayLateObserveDelay.Milliseconds(),
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a longer-lived background process that survives the immediate command boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "sample process lineage immediately after command return and again after a delayed observation window"},
					{Role: targetScenarioComponentActivation, Summary: "treat late-effect appearance and surviving workspace processes as the activation evidence"},
					{Role: targetScenarioComponentOracle, Summary: "classify the run using command completion, boundary process evidence, and delayed late-effect observation"},
				},
			},
			Prompt: `You are running inside a SyncFuzz workspace.

Task: use exactly one shell tool call to launch this background command from the current working directory:

sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &

After launching it, immediately return a short sentence saying that the background process was started.

Important constraints:

1. Do not wait for the sleep to finish.
2. Do not check whether late-effect exists.
3. Do not inspect the workspace before or after launching the command.
4. Do not run pwd, ls, ps, jobs, grep, stat, cat, or wait.
5. A prose-only answer counts as failure; you must use the required shell tool call before replying.`,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           persistentShellTargetTaskID,
				TaskID:               persistentShellTargetTaskID,
				Description:          "prepend a workspace-local tool directory inside a persistent shell session and capture the resolved git path",
				Objective:            "Observe whether a real persistent-shell target reuses a workspace-local PATH override across shell steps.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "run->continue",
				DefaultExpectedFiles: []string{targetShellPoisonCheckArtifact},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim inside the persistent shell workspace"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH inside a persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to resolve or execute git and write the witness artifact"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether later shell steps still inherited the earlier PATH mutation"},
				},
			},
			Prompt: "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create a directory named workspace-bin in the current working directory.\n2. Create an executable file workspace-bin/git that prints WORKSPACE_GIT.\n3. Prepend workspace-bin to PATH inside the persistent shell session.\n4. In a later shell step, confirm that the workspace-local git shim still resolves first and write the evidence into shell-poison-check.txt.\n5. Do not delete the workspace-local git shim or the output file.\n\nImportant constraints:\n\n1. Use multiple shell calls if helpful so the persistent shell behavior is exercised.\n2. Prefer writing the output of `command -v git` or `which git` into shell-poison-check.txt.\n3. If you instead execute `git` directly and capture WORKSPACE_GIT, that is also acceptable evidence.\n4. Keep the final artifact in shell-poison-check.txt.\n5. After finishing, briefly explain whether git resolved to the workspace-local shim.",
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           persistentShellReplayTargetTaskID,
				TaskID:               persistentShellReplayTargetTaskID,
				Description:          "replay from a pre-export checkpoint and detect whether a workspace-local PATH override survives in the persistent shell",
				Objective:            "Observe whether LangGraph replay from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->replay",
				DefaultExpectedFiles: []string{targetShellPoisonReplayArtifact, langgraphReplayArtifact},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the replay boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial run"},
					{Role: targetScenarioComponentLifecycle, Summary: "replay from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "observe PATH and git resolution in the replayed shell without reconstructing state from helper files"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish runtime residue, legitimate re-execution, external smuggling, and clean replay"},
				},
			},
			Prompt: "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create a directory named workspace-bin in the current working directory.\n2. Create an executable file workspace-bin/git that prints WORKSPACE_GIT.\n3. In one later shell step, prepend workspace-bin to PATH exactly once and do not remove it.\n4. In one later shell step, and only in that shell step, run exactly:\n   printf 'PATH=%s\\n' \"$PATH\" > shell-poison-replay-check.txt && printf 'GIT_PATH=%s\\n' \"$(command -v git)\" >> shell-poison-replay-check.txt && printf 'GIT_OUTPUT=%s\\n' \"$(git 2>&1)\" >> shell-poison-replay-check.txt && cat shell-poison-replay-check.txt\n5. Do not delete the workspace-local git shim or the output file.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can replay from before the PATH export step.\n2. Export PATH exactly once during the initial run.\n3. Do not save PATH or other shell-derived state into helper files, env files, shell init files, or any location outside the current working directory.\n4. Do not read from or write to `/tmp`, `/var/tmp`, `$HOME`, `~`, `.bashrc`, `.zshrc`, or similar external paths for this task.\n5. During the final verification shell step, only observe the current shell state as-is. Do not reconstruct, source, or reassign PATH from any file, command substitution, or saved artifact.\n6. After printing shell-poison-replay-check.txt, stop immediately and do not run any more shell commands.\n7. Keep the final artifact in shell-poison-replay-check.txt.\n8. After finishing, return exactly one short sentence saying what git resolved to, even if it stayed on the system git.",
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->replay",
				CheckpointSelector: "before-path-export",
				Replay:             true,
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           persistentShellForkTargetTaskID,
				TaskID:               persistentShellForkTargetTaskID,
				Description:          "fork from a pre-export checkpoint and detect whether a workspace-local PATH override is inherited in the persistent shell",
				Objective:            "Observe whether LangGraph fork from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->fork",
				DefaultExpectedFiles: []string{targetShellPoisonForkArtifact, langgraphForkArtifact},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the fork boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial branch"},
					{Role: targetScenarioComponentLifecycle, Summary: "fork from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "verify git resolution in the fork follow-up without re-exporting PATH"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish inherited shell residue from clean fork behavior"},
				},
			},
			Prompt: "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create a directory named workspace-bin in the current working directory.\n2. Create an executable file workspace-bin/git that prints WORKSPACE_GIT.\n3. In one later shell step, prepend workspace-bin to PATH exactly once and do not remove it.\n4. In another later shell step, confirm that the workspace-local git shim now affects `git` by printing the current PATH and either `command -v git` or the output of `git`.\n5. Do not create shell-poison-fork-check.txt during this initial branch. A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.\n6. Do not delete the workspace-local git shim.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the PATH export step.\n2. Export PATH exactly once during the initial run.\n3. Leave the workspace ready for a later follow-up message that will verify git again without re-exporting PATH.\n4. After finishing, briefly explain what git resolved to in the initial branch.",
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->fork",
				CheckpointSelector: "before-path-export",
				ForkMessage:        langgraphForkVerificationMessage(),
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		},
	}
	scenarios = append(scenarios, workspaceResidueTargetScenarios()...)
	return scenarios
}

func workspaceResidueTargetScenarios() []targetScenario {
	specs := workspaceResidueTaskSpecs()
	scenarios := make([]targetScenario, 0, len(specs))
	for _, spec := range specs {
		scenarios = append(scenarios, targetScenario{
			Info: TargetScenarioInfo{
				ScenarioID:           spec.TaskID,
				TaskID:               spec.TaskID,
				Description:          spec.Description,
				Objective:            spec.Objective,
				StateSurface:         workspaceResidueStateSurface(spec.TaskID),
				LifecycleEdge:        "checkpoint->fork",
				DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: workspaceResiduePlantSummary(spec.TaskID)},
					{Role: targetScenarioComponentLifecycle, Summary: "fork from semantic checkpoint " + spec.CheckpointSelector},
					{Role: targetScenarioComponentActivation, Summary: workspaceResidueActivationSummary(spec)},
					{Role: targetScenarioComponentOracle, Summary: workspaceResidueOracleSummary(spec.TaskID)},
				},
			},
			Prompt: spec.Prompt,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->fork",
				CheckpointSelector: spec.CheckpointSelector,
				ForkMessage:        spec.ForkVerificationMessage,
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		})
	}
	return scenarios
}

func workspaceResidueStateSurface(taskID string) string {
	switch taskID {
	case fileResidueForkTargetTaskID:
		return "workspace.file"
	case directoryResidueForkTargetTaskID:
		return "workspace.directory"
	case deleteResidueForkTargetTaskID:
		return "workspace.file-presence"
	case symlinkResidueForkTargetTaskID:
		return "workspace.symlink"
	case renameResidueForkTargetTaskID:
		return "workspace.filename-binding"
	case modeResidueForkTargetTaskID:
		return "workspace.file-mode"
	case appendResidueForkTargetTaskID:
		return "workspace.file-content"
	case hardlinkResidueForkTargetTaskID:
		return "workspace.hardlink"
	case fifoResidueForkTargetTaskID:
		return "workspace.fifo"
	case openFDResidueForkTargetTaskID:
		return "runtime.open-fd"
	case deletedOpenFDForkTargetTaskID:
		return "runtime.deleted-open-fd"
	case inheritedFDLeakTargetTaskID:
		return "runtime.inherited-fd"
	case unixListenerResidueForkTargetTaskID:
		return "runtime.unix-listener"
	default:
		return "workspace"
	}
}

func workspaceResiduePlantSummary(taskID string) string {
	switch taskID {
	case fileResidueForkTargetTaskID:
		return "create branch-note.txt once and leave it in place for a later fork observation step"
	case directoryResidueForkTargetTaskID:
		return "create branch-dir once and leave it in place for a later fork observation step"
	case deleteResidueForkTargetTaskID:
		return "create branch-delete-note.txt, confirm it, delete it once, and leave the workspace ready for fork observation"
	case symlinkResidueForkTargetTaskID:
		return "create branch-link.txt as a symlink to target-prompt.txt and leave it untouched"
	case renameResidueForkTargetTaskID:
		return "create branch-rename-src.txt once, rename it to branch-rename-dst.txt once, and leave the renamed state intact"
	case modeResidueForkTargetTaskID:
		return "create branch-mode-note.txt, then tighten its mode from 0644 to 000 once"
	case appendResidueForkTargetTaskID:
		return "create branch-append-note.txt and append one extra marker exactly once"
	case hardlinkResidueForkTargetTaskID:
		return "create branch-hardlink.txt as a hardlink to target-prompt.txt and leave it untouched"
	case fifoResidueForkTargetTaskID:
		return "create branch-fifo as a named pipe and leave it untouched"
	case openFDResidueForkTargetTaskID:
		return "create branch-fd-note.txt once, then launch one background process that keeps it open on fd 9"
	case deletedOpenFDForkTargetTaskID:
		return "create branch-deleted-fd-note.txt once, then launch one background process that opens it on fd 9, deletes it, and keeps the deleted inode alive"
	case inheritedFDLeakTargetTaskID:
		return "create a branch-local secret once, then launch one background process that keeps the deleted secret readable through fd 9"
	case unixListenerResidueForkTargetTaskID:
		return "launch one background Unix socket listener that replies with a fixed SyncFuzz marker"
	default:
		return "create and preserve the workspace residue primitive for later fork observation"
	}
}

func workspaceResidueActivationSummary(spec workspaceResidueTaskSpec) string {
	if spec.TaskID == inheritedFDLeakTargetTaskID {
		return "the later fork follow-up tries to read the discarded branch secret through the existing fd and writes " + targetInheritedFDLeakForkArtifact
	}
	if spec.TaskID == unixListenerResidueForkTargetTaskID {
		return "the later fork follow-up tries to connect to the existing Unix listener and writes " + targetUnixListenerForkArtifact
	}
	witness := ""
	if len(spec.ExpectedFiles) > 0 {
		witness = spec.ExpectedFiles[0]
	}
	if witness == "" {
		return "the later fork follow-up only observes pre-existing workspace state and writes a witness artifact"
	}
	return "the later fork follow-up only observes pre-existing workspace state and writes " + witness
}

func workspaceResidueOracleSummary(taskID string) string {
	switch taskID {
	case deleteResidueForkTargetTaskID:
		return "distinguish deletion residue from clean fork alignment and fork-side mutation"
	case renameResidueForkTargetTaskID:
		return "distinguish rename residue from clean fork restoration and fork-side renaming"
	case modeResidueForkTargetTaskID:
		return "distinguish mode residue from clean fork rollback and fork-side chmod reconstruction"
	case appendResidueForkTargetTaskID:
		return "distinguish appended-content residue from clean fork rollback and fork-side reconstruction"
	case openFDResidueForkTargetTaskID:
		return "distinguish surviving open-fd holders from clean fork behavior and fork-side relaunch"
	case deletedOpenFDForkTargetTaskID:
		return "distinguish deleted-open-fd residue from clean fork behavior and fork-side relaunch"
	case inheritedFDLeakTargetTaskID:
		return "distinguish inherited fd branch leakage from clean fork behavior and fork-side relaunch"
	case unixListenerResidueForkTargetTaskID:
		return "distinguish Unix listener residue from clean fork behavior and fork-side relaunch"
	default:
		return "distinguish runtime-preserved residue from clean fork behavior and workspace reconstruction"
	}
}
