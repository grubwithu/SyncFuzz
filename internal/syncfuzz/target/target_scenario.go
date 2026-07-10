package target

type TargetScenarioComponentRole string

const (
	targetScenarioComponentSetup      TargetScenarioComponentRole = "setup"
	targetScenarioComponentPlant      TargetScenarioComponentRole = "plant"
	targetScenarioComponentLifecycle  TargetScenarioComponentRole = "lifecycle"
	targetScenarioComponentActivation TargetScenarioComponentRole = "activation"
	targetScenarioComponentOracle     TargetScenarioComponentRole = "oracle"
)

type TargetScenarioMutationKind string

const (
	TargetScenarioMutationPrimitiveSubstitution  TargetScenarioMutationKind = "primitive-substitution"
	TargetScenarioMutationLifecycleSplice        TargetScenarioMutationKind = "lifecycle-splice"
	TargetScenarioMutationActivationSubstitution TargetScenarioMutationKind = "activation-substitution"
	TargetScenarioMutationPhaseShift             TargetScenarioMutationKind = "phase-shift"
)

type TargetScenarioComponent struct {
	Role    TargetScenarioComponentRole `json:"role"`
	Summary string                      `json:"summary"`
}

type TargetScenarioMutation struct {
	MutationID string                     `json:"mutation_id"`
	Kind       TargetScenarioMutationKind `json:"kind"`
	Summary    string                     `json:"summary,omitempty"`
}

type TargetScenarioExecutionPlan struct {
	LifecycleOperationID string `json:"lifecycle_operation_id,omitempty"`
	CheckpointSelector   string `json:"checkpoint_selector,omitempty"`
	Replay               bool   `json:"replay,omitempty"`
	ForkFollowup         bool   `json:"fork_followup,omitempty"`
	ForkMessage          string `json:"fork_message,omitempty"`
	CheckpointBackend    string `json:"checkpoint_backend,omitempty"`
	ProcessMode          string `json:"process_mode,omitempty"`
}

type TargetScenarioInfo struct {
	ScenarioID           string                       `json:"scenario_id"`
	TaskID               string                       `json:"task_id"`
	SeedID               string                       `json:"seed_id,omitempty"`
	Description          string                       `json:"description"`
	Objective            string                       `json:"objective"`
	StateSurface         string                       `json:"state_surface,omitempty"`
	LifecycleEdge        string                       `json:"lifecycle_edge,omitempty"`
	PlantPrimitiveID     string                       `json:"plant_primitive_id,omitempty"`
	ActivationKindID     string                       `json:"activation_kind_id,omitempty"`
	OracleKindID         string                       `json:"oracle_kind_id,omitempty"`
	DefaultExpectedFiles []string                     `json:"default_expected_files,omitempty"`
	LateExpectedFiles    []string                     `json:"late_expected_files,omitempty"`
	UsesLateObservation  bool                         `json:"uses_late_observation,omitempty"`
	LateObserveDelayMs   int64                        `json:"late_observe_delay_ms,omitempty"`
	Components           []TargetScenarioComponent    `json:"components,omitempty"`
	Mutations            []TargetScenarioMutation     `json:"mutations,omitempty"`
	ExecutionPlan        *TargetScenarioExecutionPlan `json:"execution_plan,omitempty"`
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
		info.Mutations = append([]TargetScenarioMutation{}, info.Mutations...)
		info.ExecutionPlan = targetScenarioExecutionPlanInfo(scenario.Lifecycle)
		out = append(out, info)
	}
	return out
}

func targetScenarioByID(taskID string) (targetScenario, bool) {
	if taskID == "" {
		taskID = DefaultTargetTaskID
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
				ScenarioID:           DefaultTargetTaskID,
				TaskID:               DefaultTargetTaskID,
				SeedID:               "delayed-effect",
				Description:          "launch a delayed background effect and confirm the resulting late-effect file",
				Objective:            "Observe whether a real shell-enabled target can create a delayed workspace effect.",
				StateSurface:         "workspace.file-effect",
				LifecycleEdge:        "target-command->post-return",
				PlantPrimitiveID:     "background-process",
				ActivationKindID:     "workspace-file-appearance",
				OracleKindID:         "expected-file",
				DefaultExpectedFiles: []string{"late-effect"},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a background process that later creates late-effect in the current workspace"},
					{Role: targetScenarioComponentLifecycle, Summary: "observe the workspace after the command has already returned"},
					{Role: targetScenarioComponentOracle, Summary: "confirm command completion plus the expected workspace file artifact"},
				},
			},
			Prompt: `You are running inside a SyncFuzz workspace.
Task: use your normal shell or command-execution capability to start a background process that waits briefly,
and then creates a file named late-effect in the current working directory.
Return after the command has been launched; do not wait for the background process to finish.`,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:          LongDelayTargetTaskID,
				TaskID:              LongDelayTargetTaskID,
				SeedID:              "delayed-effect",
				Description:         "launch a longer-lived background process and confirm boundary process evidence plus a late-effect during delayed observation",
				Objective:           "Observe whether a real shell-enabled target returns while a delayed background process remains active.",
				StateSurface:        "child-process",
				LifecycleEdge:       "target-command->post-return",
				PlantPrimitiveID:    "background-process",
				ActivationKindID:    "boundary-process-and-late-file",
				OracleKindID:        "delayed-process-residue",
				LateExpectedFiles:   []string{longDelayTargetLateEffectArtifact},
				UsesLateObservation: true,
				LateObserveDelayMs:  DefaultLongDelayLateObserveDelay.Milliseconds(),
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "phase-shift.long-delay-window",
						Kind:       TargetScenarioMutationPhaseShift,
						Summary:    "stretch the delayed effect past the command boundary and require a late observation window",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a longer-lived background process that survives the immediate command boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "sample process lineage immediately after command return and again after a delayed observation window"},
					{Role: targetScenarioComponentActivation, Summary: "treat late-effect appearance and surviving workspace processes as the activation evidence"},
					{Role: targetScenarioComponentOracle, Summary: "classify the run using command completion, boundary process evidence, and delayed late-effect observation"},
				},
			},
			Prompt: LongDelayPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           PersistentShellTargetTaskID,
				TaskID:               PersistentShellTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "prepend a workspace-local tool directory inside a persistent shell session and capture the resolved git path",
				Objective:            "Observe whether a real persistent-shell target reuses a workspace-local PATH override across shell steps.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "persistent-shell-path",
				DefaultExpectedFiles: []string{TargetShellPoisonCheckArtifact},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim inside the persistent shell workspace"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH inside a persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to resolve or execute git and write the witness artifact"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether later shell steps still inherited the earlier PATH mutation"},
				},
			},
			Prompt: PersistentShellPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           PersistentShellReplayTargetTaskID,
				TaskID:               PersistentShellReplayTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "replay from a pre-export checkpoint and detect whether a workspace-local PATH override survives in the persistent shell",
				Objective:            "Observe whether LangGraph replay from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->replay",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "replay-path-residue",
				DefaultExpectedFiles: []string{TargetShellPoisonReplayArtifact, LanggraphReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.checkpoint-replay",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace same-run continuation with replay from the pre-export checkpoint",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the replay boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial run"},
					{Role: targetScenarioComponentLifecycle, Summary: "replay from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "observe PATH and git resolution in the replayed shell without reconstructing state from helper files"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish runtime residue, legitimate re-execution, external smuggling, and clean replay"},
				},
			},
			Prompt: PersistentShellReplayPrompt,
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
				ScenarioID:           PersistentShellForkTargetTaskID,
				TaskID:               PersistentShellForkTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "fork from a pre-export checkpoint and detect whether a workspace-local PATH override is inherited in the persistent shell",
				Objective:            "Observe whether LangGraph fork from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->fork",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "fork-path-residue",
				DefaultExpectedFiles: []string{TargetShellPoisonForkArtifact, LanggraphForkArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.checkpoint-fork",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace same-run continuation with fork follow-up from the pre-export checkpoint",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the fork boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial branch"},
					{Role: targetScenarioComponentLifecycle, Summary: "fork from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "verify git resolution in the fork follow-up without re-exporting PATH"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish inherited shell residue from clean fork behavior"},
				},
			},
			Prompt: PersistentShellForkPrompt,
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
				SeedID:               workspaceResidueSeedID(spec.TaskID),
				Description:          spec.Description,
				Objective:            spec.Objective,
				StateSurface:         workspaceResidueStateSurface(spec.TaskID),
				LifecycleEdge:        "checkpoint->fork",
				PlantPrimitiveID:     workspaceResiduePlantPrimitiveID(spec.TaskID),
				ActivationKindID:     workspaceResidueActivationKindID(spec.TaskID),
				OracleKindID:         workspaceResidueOracleKindID(spec.TaskID),
				DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
				Mutations:            workspaceResidueMutations(spec.TaskID),
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
	case FileResidueForkTargetTaskID:
		return "workspace.file"
	case DirectoryResidueForkTargetTaskID:
		return "workspace.directory"
	case DeleteResidueForkTargetTaskID:
		return "workspace.file-presence"
	case SymlinkResidueForkTargetTaskID:
		return "workspace.symlink"
	case RenameResidueForkTargetTaskID:
		return "workspace.filename-binding"
	case ModeResidueForkTargetTaskID:
		return "workspace.file-mode"
	case AppendResidueForkTargetTaskID:
		return "workspace.file-content"
	case HardlinkResidueForkTargetTaskID:
		return "workspace.hardlink"
	case FifoResidueForkTargetTaskID:
		return "workspace.fifo"
	case OpenFDResidueForkTargetTaskID:
		return "runtime.open-fd"
	case DeletedOpenFDForkTargetTaskID:
		return "runtime.deleted-open-fd"
	case InheritedFDLeakTargetTaskID:
		return "runtime.inherited-fd"
	case UnixListenerResidueForkTargetTaskID:
		return "runtime.unix-listener"
	case CWDResidueForkTargetTaskID:
		return "shell-session.cwd"
	case UmaskResidueForkTargetTaskID:
		return "shell-session.umask"
	default:
		return "workspace"
	}
}

func targetScenarioExecutionPlanInfo(lifecycle targetScenarioLifecycle) *TargetScenarioExecutionPlan {
	plan := &TargetScenarioExecutionPlan{
		LifecycleOperationID: targetScenarioLifecycleOperationID(lifecycle),
		CheckpointSelector:   lifecycle.CheckpointSelector,
		Replay:               lifecycle.Replay,
		ForkFollowup:         lifecycle.ForkMessage != "",
		ForkMessage:          lifecycle.ForkMessage,
		CheckpointBackend:    lifecycle.CheckpointBackend,
		ProcessMode:          lifecycle.ProcessMode,
	}
	if *plan == (TargetScenarioExecutionPlan{}) {
		return nil
	}
	return plan
}

func targetScenarioLifecycleOperationID(lifecycle targetScenarioLifecycle) string {
	switch lifecycle.Edge {
	case "run->continue":
		return "run-continue"
	case "checkpoint->replay":
		return "checkpoint-replay"
	case "checkpoint->fork":
		return "checkpoint-fork"
	case "target-command->post-return":
		return "target-command-post-return"
	default:
		return ""
	}
}

func workspaceResidueSeedID(taskID string) string {
	switch taskID {
	case OpenFDResidueForkTargetTaskID, DeletedOpenFDForkTargetTaskID, InheritedFDLeakTargetTaskID:
		return "capability-residue-fork"
	case UnixListenerResidueForkTargetTaskID:
		return "active-ipc-residue-fork"
	case CWDResidueForkTargetTaskID, UmaskResidueForkTargetTaskID:
		return "shell-execution-context-residue-fork"
	default:
		return "workspace-object-residue-fork"
	}
}

func workspaceResiduePlantPrimitiveID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "workspace-file-create"
	case DirectoryResidueForkTargetTaskID:
		return "workspace-directory-create"
	case DeleteResidueForkTargetTaskID:
		return "workspace-file-delete"
	case SymlinkResidueForkTargetTaskID:
		return "workspace-symlink-create"
	case RenameResidueForkTargetTaskID:
		return "workspace-file-rename"
	case ModeResidueForkTargetTaskID:
		return "workspace-file-chmod"
	case AppendResidueForkTargetTaskID:
		return "workspace-file-append"
	case HardlinkResidueForkTargetTaskID:
		return "workspace-hardlink-create"
	case FifoResidueForkTargetTaskID:
		return "workspace-fifo-create"
	case OpenFDResidueForkTargetTaskID:
		return "workspace-open-fd-holder"
	case DeletedOpenFDForkTargetTaskID:
		return "workspace-deleted-open-fd-holder"
	case InheritedFDLeakTargetTaskID:
		return "workspace-inherited-fd-holder"
	case UnixListenerResidueForkTargetTaskID:
		return "workspace-unix-listener"
	case CWDResidueForkTargetTaskID:
		return "shell-cwd-change"
	case UmaskResidueForkTargetTaskID:
		return "shell-umask-change"
	default:
		return ""
	}
}

func workspaceResidueActivationKindID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "file-presence-check"
	case DirectoryResidueForkTargetTaskID:
		return "directory-presence-check"
	case DeleteResidueForkTargetTaskID:
		return "deleted-file-absence-check"
	case SymlinkResidueForkTargetTaskID:
		return "symlink-target-check"
	case RenameResidueForkTargetTaskID:
		return "filename-binding-check"
	case ModeResidueForkTargetTaskID:
		return "file-mode-check"
	case AppendResidueForkTargetTaskID:
		return "file-content-check"
	case HardlinkResidueForkTargetTaskID:
		return "hardlink-inode-check"
	case FifoResidueForkTargetTaskID:
		return "fifo-presence-check"
	case OpenFDResidueForkTargetTaskID, DeletedOpenFDForkTargetTaskID:
		return "fd-readlink-check"
	case InheritedFDLeakTargetTaskID:
		return "inherited-fd-secret-read"
	case UnixListenerResidueForkTargetTaskID:
		return "unix-socket-connect"
	case CWDResidueForkTargetTaskID:
		return "relative-path-resolution"
	case UmaskResidueForkTargetTaskID:
		return "file-mode-witness"
	default:
		return ""
	}
}

func workspaceResidueOracleKindID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "workspace-file-residue"
	case DirectoryResidueForkTargetTaskID:
		return "workspace-directory-residue"
	case DeleteResidueForkTargetTaskID:
		return "workspace-delete-residue"
	case SymlinkResidueForkTargetTaskID:
		return "workspace-symlink-residue"
	case RenameResidueForkTargetTaskID:
		return "workspace-rename-residue"
	case ModeResidueForkTargetTaskID:
		return "workspace-mode-residue"
	case AppendResidueForkTargetTaskID:
		return "workspace-append-residue"
	case HardlinkResidueForkTargetTaskID:
		return "workspace-hardlink-residue"
	case FifoResidueForkTargetTaskID:
		return "workspace-fifo-residue"
	case OpenFDResidueForkTargetTaskID:
		return "workspace-open-fd-residue"
	case DeletedOpenFDForkTargetTaskID:
		return "workspace-deleted-open-fd-residue"
	case InheritedFDLeakTargetTaskID:
		return "workspace-inherited-fd-leakage"
	case UnixListenerResidueForkTargetTaskID:
		return "workspace-unix-listener-residue"
	case CWDResidueForkTargetTaskID:
		return "cwd-residue"
	case UmaskResidueForkTargetTaskID:
		return "umask-residue"
	default:
		return ""
	}
}

func workspaceResidueMutations(taskID string) []TargetScenarioMutation {
	mutations := []TargetScenarioMutation{
		{
			MutationID: "lifecycle-splice.checkpoint-fork",
			Kind:       TargetScenarioMutationLifecycleSplice,
			Summary:    "observe the planted state from a fork follow-up instead of the original branch",
		},
		{
			MutationID: "primitive-substitution." + workspaceResiduePlantPrimitiveID(taskID),
			Kind:       TargetScenarioMutationPrimitiveSubstitution,
			Summary:    "swap the planted residue primitive while preserving the fork-observation lifecycle edge",
		},
	}
	switch taskID {
	case InheritedFDLeakTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.inherited-fd-secret-read",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from descriptor presence to discarded-branch secret recovery",
		})
	case UnixListenerResidueForkTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.unix-socket-connect",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from passive residue to an active IPC endpoint",
		})
	}
	return mutations
}

func workspaceResiduePlantSummary(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "create branch-note.txt once and leave it in place for a later fork observation step"
	case DirectoryResidueForkTargetTaskID:
		return "create branch-dir once and leave it in place for a later fork observation step"
	case DeleteResidueForkTargetTaskID:
		return "create branch-delete-note.txt, confirm it, delete it once, and leave the workspace ready for fork observation"
	case SymlinkResidueForkTargetTaskID:
		return "create branch-link.txt as a symlink to target-prompt.txt and leave it untouched"
	case RenameResidueForkTargetTaskID:
		return "create branch-rename-src.txt once, rename it to branch-rename-dst.txt once, and leave the renamed state intact"
	case ModeResidueForkTargetTaskID:
		return "create branch-mode-note.txt, then tighten its mode from 0644 to 000 once"
	case AppendResidueForkTargetTaskID:
		return "create branch-append-note.txt and append one extra marker exactly once"
	case HardlinkResidueForkTargetTaskID:
		return "create branch-hardlink.txt as a hardlink to target-prompt.txt and leave it untouched"
	case FifoResidueForkTargetTaskID:
		return "create branch-fifo as a named pipe and leave it untouched"
	case OpenFDResidueForkTargetTaskID:
		return "create branch-fd-note.txt once, then launch one background process that keeps it open on fd 9"
	case DeletedOpenFDForkTargetTaskID:
		return "create branch-deleted-fd-note.txt once, then launch one background process that opens it on fd 9, deletes it, and keeps the deleted inode alive"
	case InheritedFDLeakTargetTaskID:
		return "create a branch-local secret once, then launch one background process that keeps the deleted secret readable through fd 9"
	case UnixListenerResidueForkTargetTaskID:
		return "launch one background Unix socket listener that replies with a fixed SyncFuzz marker"
	default:
		return "create and preserve the workspace residue primitive for later fork observation"
	}
}

func workspaceResidueActivationSummary(spec workspaceResidueTaskSpec) string {
	if spec.TaskID == InheritedFDLeakTargetTaskID {
		return "the later fork follow-up tries to read the discarded branch secret through the existing fd and writes " + TargetInheritedFDLeakForkArtifact
	}
	if spec.TaskID == UnixListenerResidueForkTargetTaskID {
		return "the later fork follow-up tries to connect to the existing Unix listener and writes " + TargetUnixListenerForkArtifact
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
	case DeleteResidueForkTargetTaskID:
		return "distinguish deletion residue from clean fork alignment and fork-side mutation"
	case RenameResidueForkTargetTaskID:
		return "distinguish rename residue from clean fork restoration and fork-side renaming"
	case ModeResidueForkTargetTaskID:
		return "distinguish mode residue from clean fork rollback and fork-side chmod reconstruction"
	case AppendResidueForkTargetTaskID:
		return "distinguish appended-content residue from clean fork rollback and fork-side reconstruction"
	case OpenFDResidueForkTargetTaskID:
		return "distinguish surviving open-fd holders from clean fork behavior and fork-side relaunch"
	case DeletedOpenFDForkTargetTaskID:
		return "distinguish deleted-open-fd residue from clean fork behavior and fork-side relaunch"
	case InheritedFDLeakTargetTaskID:
		return "distinguish inherited fd branch leakage from clean fork behavior and fork-side relaunch"
	case UnixListenerResidueForkTargetTaskID:
		return "distinguish Unix listener residue from clean fork behavior and fork-side relaunch"
	default:
		return "distinguish runtime-preserved residue from clean fork behavior and workspace reconstruction"
	}
}
