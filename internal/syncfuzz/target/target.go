package target

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

const (
	TargetTaskArtifact                         = "target-task.json"
	TargetPromptArtifact                       = "target-prompt.txt"
	TargetOutputArtifact                       = "target-output.txt"
	TargetResultArtifact                       = "target-result.json"
	TargetShellPoisonCheckArtifact             = "shell-poison-check.txt"
	TargetShellPoisonReplayArtifact            = "shell-poison-replay-check.txt"
	TargetShellPoisonForkArtifact              = "shell-poison-fork-check.txt"
	TargetShellShimDirArtifact                 = "workspace-bin"
	TargetShellShimExecArtifact                = TargetShellShimDirArtifact + "/git"
	targetShellShimMarker                      = "WORKSPACE_GIT"
	TargetShellLegacyShimDirArtifact           = "attacker-bin"
	targetShellLegacyShimMarker                = "ATTACKER_GIT"
	TargetFileResidueNoteArtifact              = "branch-note.txt"
	TargetFileResidueCheckArtifact             = "file-residue-check.txt"
	TargetFileResidueForkArtifact              = "file-residue-fork-check.txt"
	TargetDirectoryResidueDirArtifact          = "branch-dir"
	TargetDirectoryResidueCheckArtifact        = "directory-residue-check.txt"
	TargetDirectoryResidueForkArtifact         = "directory-residue-fork-check.txt"
	TargetDeleteResidueNoteArtifact            = "branch-delete-note.txt"
	TargetDeleteResidueCheckArtifact           = "delete-residue-check.txt"
	TargetDeleteResidueForkArtifact            = "delete-residue-fork-check.txt"
	TargetSymlinkResidueLinkArtifact           = "branch-link.txt"
	TargetSymlinkResidueCheckArtifact          = "symlink-residue-check.txt"
	TargetSymlinkResidueForkArtifact           = "symlink-residue-fork-check.txt"
	TargetRenameResidueSourceArtifact          = "branch-rename-src.txt"
	TargetRenameResidueDestArtifact            = "branch-rename-dst.txt"
	TargetRenameResidueCheckArtifact           = "rename-residue-check.txt"
	TargetRenameResidueForkArtifact            = "rename-residue-fork-check.txt"
	TargetModeResidueNoteArtifact              = "branch-mode-note.txt"
	TargetModeResidueCheckArtifact             = "mode-residue-check.txt"
	TargetModeResidueForkArtifact              = "mode-residue-fork-check.txt"
	TargetAppendResidueNoteArtifact            = "branch-append-note.txt"
	TargetAppendResidueCheckArtifact           = "append-residue-check.txt"
	TargetAppendResidueForkArtifact            = "append-residue-fork-check.txt"
	TargetHardlinkResidueLinkArtifact          = "branch-hardlink.txt"
	TargetHardlinkResidueCheckArtifact         = "hardlink-residue-check.txt"
	TargetHardlinkResidueForkArtifact          = "hardlink-residue-fork-check.txt"
	TargetFIFOResiduePipeArtifact              = "branch-fifo"
	TargetFIFOResidueCheckArtifact             = "fifo-residue-check.txt"
	TargetFIFOResidueForkArtifact              = "fifo-residue-fork-check.txt"
	TargetOpenFDResidueNoteArtifact            = "branch-fd-note.txt"
	TargetOpenFDResiduePIDArtifact             = "branch-fd-pid.txt"
	TargetOpenFDResidueForkArtifact            = "open-fd-residue-fork-check.txt"
	TargetDeletedOpenFDNoteArtifact            = "branch-deleted-fd-note.txt"
	TargetDeletedOpenFDPIDArtifact             = "branch-deleted-fd-pid.txt"
	TargetDeletedOpenFDForkArtifact            = "deleted-open-fd-residue-fork-check.txt"
	TargetInheritedFDLeakSecretArtifact        = "branch-inherited-fd-secret.txt"
	TargetInheritedFDLeakPIDArtifact           = "branch-inherited-fd-pid.txt"
	TargetInheritedFDLeakForkArtifact          = "inherited-fd-branch-leakage-check.txt"
	TargetUnixListenerResidueCheckArtifact     = "unix-listener-residue-check.txt"
	TargetUnixListenerSocketArtifact           = "branch-listener.sock"
	TargetUnixListenerPIDArtifact              = "branch-listener-pid.txt"
	TargetUnixListenerForkArtifact             = "unix-listener-residue-fork-check.txt"
	TargetTrustedClientResponseArtifact        = "trusted-client-response.txt"
	TargetDiscardedServerTrustedClientArtifact = "discarded-server-trusted-client-check.txt"
	TargetTrustedClientCacheArtifact           = "trusted-client-cache.txt"
	TargetSocketResponsePoisoningArtifact      = "socket-response-poisoning-check.txt"
	TargetSnapshotLateArtifact                 = "snapshot-late.json"
	TargetProcessLateArtifact                  = "process-late.json"
	TargetFilesystemLateArtifact               = "filesystem-late-metadata.json"
	TargetContractProfileArtifact              = "target-contract-profile.json"
	TargetEnvResidueCheckArtifact              = "env-residue-check.txt"
	targetEnvResidueVarName                    = "SYNCFUZZ_ENV_RESIDUE_FLAG"
	targetEnvResidueMarker                     = "SYNCFUZZ_ENV_RESIDUE_MARKER"
	TargetFunctionResidueCheckArtifact         = "function-residue-check.txt"
	targetFunctionResidueName                  = "syncfuzz_residue_probe"
	targetFunctionResidueMarker                = "SYNCFUZZ_FUNCTION_RESIDUE_MARKER"
	TargetCWDResidueCheckArtifact              = "cwd-residue-check.txt"
	TargetCWDResidueDirArtifact                = "branch-cwd-dir"
	TargetCWDResidueWitnessArtifact            = "cwd-relative-witness.txt"
	TargetCWDResidueForkArtifact               = "cwd-residue-fork-check.txt"
	TargetUmaskResidueCheckArtifact            = "umask-residue-check.txt"
	TargetUmaskResidueBaselineArtifact         = "baseline-umask.txt"
	TargetUmaskResidueWitnessArtifact          = "umask-witness.txt"
	TargetUmaskResidueForkArtifact             = "umask-residue-fork-check.txt"
	targetFileResidueMarker                    = "SYNCFUZZ_FILE_RESIDUE_MARKER"
	targetAppendResidueBaseMarker              = "SYNCFUZZ_APPEND_BASE"
	targetAppendResidueMarker                  = "SYNCFUZZ_APPEND_MARKER"
	targetModeResidueTightenedMode             = "400"

	DefaultTargetAdapterID                   = "command"
	DefaultTargetTaskID                      = "orphan-process"
	LongDelayTargetTaskID                    = "orphan-process-long-delay"
	PersistentShellTargetTaskID              = "persistent-shell-poisoning"
	PersistentShellReplayTargetTaskID        = "persistent-shell-poisoning-replay"
	PersistentShellForkTargetTaskID          = "persistent-shell-poisoning-fork"
	FileResidueTargetTaskID                  = "file-residue"
	DirectoryResidueTargetTaskID             = "directory-residue"
	DeleteResidueTargetTaskID                = "delete-residue"
	SymlinkResidueTargetTaskID               = "symlink-residue"
	RenameResidueTargetTaskID                = "rename-residue"
	ModeResidueTargetTaskID                  = "mode-residue"
	AppendResidueTargetTaskID                = "append-residue"
	HardlinkResidueTargetTaskID              = "hardlink-residue"
	FifoResidueTargetTaskID                  = "fifo-residue"
	FileResidueForkTargetTaskID              = "file-residue-fork"
	DirectoryResidueForkTargetTaskID         = "directory-residue-fork"
	DeleteResidueForkTargetTaskID            = "delete-residue-fork"
	SymlinkResidueForkTargetTaskID           = "symlink-residue-fork"
	RenameResidueForkTargetTaskID            = "rename-residue-fork"
	ModeResidueForkTargetTaskID              = "mode-residue-fork"
	AppendResidueForkTargetTaskID            = "append-residue-fork"
	HardlinkResidueForkTargetTaskID          = "hardlink-residue-fork"
	FifoResidueForkTargetTaskID              = "fifo-residue-fork"
	OpenFDResidueForkTargetTaskID            = "open-fd-residue-fork"
	DeletedOpenFDForkTargetTaskID            = "deleted-open-fd-residue-fork"
	InheritedFDLeakTargetTaskID              = "inherited-fd-branch-leakage"
	UnixListenerResidueTargetTaskID          = "unix-listener-residue"
	UnixListenerResidueForkTargetTaskID      = "unix-listener-residue-fork"
	DiscardedServerTrustedClientTargetTaskID = "discarded-server-trusted-client"
	SocketResponsePoisoningTargetTaskID      = "socket-response-poisoning"
	EnvResidueTargetTaskID                   = "env-residue"
	FunctionResidueTargetTaskID              = "function-residue"
	CWDResidueTargetTaskID                   = "cwd-residue"
	UmaskResidueTargetTaskID                 = "umask-residue"
	CWDResidueForkTargetTaskID               = "cwd-residue-fork"
	UmaskResidueForkTargetTaskID             = "umask-residue-fork"

	longDelayTargetLateEffectArtifact = "late-effect"
	DefaultLongDelayLateObserveDelay  = 7 * time.Second
)

type TargetAdapterInfo struct {
	AdapterID    string   `json:"adapter_id"`
	Implemented  bool     `json:"implemented"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type TargetRunOptions struct {
	AdapterID        string
	TargetID         string
	TaskID           string
	Objective        string
	PromptProfileID  string
	PromptVariantID  string
	Prompt           string
	PromptFile       string
	Command          string
	CommandFile      string
	OutDir           string
	Workspace        string
	Timeout          time.Duration
	ObserveDelay     time.Duration
	LateObserveDelay time.Duration
	EnvKind          string
	ContainerImage   string
	ExpectedFiles    []string
}

type TargetTask struct {
	SchemaVersion      string              `json:"schema_version"`
	RunID              string              `json:"run_id"`
	AdapterID          string              `json:"adapter_id"`
	TargetID           string              `json:"target_id"`
	TaskID             string              `json:"task_id"`
	Objective          string              `json:"objective"`
	PromptProfileID    string              `json:"prompt_profile_id,omitempty"`
	PromptVariantID    string              `json:"prompt_variant_id,omitempty"`
	Scenario           *TargetScenarioInfo `json:"scenario,omitempty"`
	Prompt             string              `json:"prompt"`
	PromptFile         string              `json:"prompt_file"`
	Command            string              `json:"command"`
	TimeoutMillis      int64               `json:"timeout_ms"`
	ObserveDelayMs     int64               `json:"observe_delay_ms"`
	LateObserveDelayMs int64               `json:"late_observe_delay_ms,omitempty"`
	Environment        string              `json:"environment"`
	ContainerImage     string              `json:"container_image,omitempty"`
	Workspace          string              `json:"workspace"`
	ExpectedFiles      []string            `json:"expected_files,omitempty"`
	CreatedAt          string              `json:"created_at"`
}

type TargetCommandResult struct {
	ExitCode     int    `json:"exit_code"`
	TimedOut     bool   `json:"timed_out"`
	DurationMs   int64  `json:"duration_ms"`
	OutputBytes  int    `json:"output_bytes"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
	Error        string `json:"error,omitempty"`
}

type TargetOracleResult struct {
	Name        string             `json:"name"`
	Status      TargetOracleStatus `json:"status,omitempty"`
	Confirmed   bool               `json:"confirmed"`
	Attribution string             `json:"attribution,omitempty"`
	Evidence    []string           `json:"evidence,omitempty"`
	Missing     []string           `json:"missing,omitempty"`
}

const (
	TargetOracleStatusConfirmed    TargetOracleStatus = "confirmed"
	TargetOracleStatusNegative     TargetOracleStatus = "negative"
	TargetOracleStatusInconclusive TargetOracleStatus = "inconclusive"

	TargetOracleAttributionRuntimeResidue        = "runtime-preserved-residue"
	TargetOracleAttributionLegitimateReexecution = "legitimate-reexecution"
	TargetOracleAttributionExternalSmuggling     = "external-state-smuggling"
	TargetOracleAttributionCleanReplay           = "clean-replay"
	TargetOracleAttributionCleanFork             = "clean-fork"
	TargetOracleAttributionWorkspaceRebuild      = "workspace-reconstruction"
	TargetOracleAttributionUnknown               = "unknown-causal-path"
)

type TargetOracleStatus string

type TargetRunResult struct {
	SchemaVersion            string                        `json:"schema_version"`
	RunID                    string                        `json:"run_id"`
	AdapterID                string                        `json:"adapter_id"`
	TargetID                 string                        `json:"target_id"`
	TaskID                   string                        `json:"task_id"`
	Objective                string                        `json:"objective"`
	PromptProfileID          string                        `json:"prompt_profile_id,omitempty"`
	PromptVariantID          string                        `json:"prompt_variant_id,omitempty"`
	Environment              string                        `json:"environment"`
	ContainerImage           string                        `json:"container_image,omitempty"`
	Command                  string                        `json:"command"`
	TimeoutMillis            int64                         `json:"timeout_ms"`
	ObserveDelayMs           int64                         `json:"observe_delay_ms"`
	LateObserveDelayMs       int64                         `json:"late_observe_delay_ms,omitempty"`
	Completed                bool                          `json:"completed"`
	ExpectationsMet          bool                          `json:"expectations_met"`
	ExpectedFiles            []string                      `json:"expected_files,omitempty"`
	ExpectedFilesPresent     []string                      `json:"expected_files_present,omitempty"`
	ExpectedFilesMissing     []string                      `json:"expected_files_missing,omitempty"`
	LateObserved             bool                          `json:"late_observed"`
	LateExpectedFiles        []string                      `json:"late_expected_files,omitempty"`
	LateExpectedFilesPresent []string                      `json:"late_expected_files_present,omitempty"`
	LateExpectedFilesMissing []string                      `json:"late_expected_files_missing,omitempty"`
	CommandResult            TargetCommandResult           `json:"command_result"`
	ProcessLineage           core.ProcessLineageSummary    `json:"process_lineage"`
	TargetOracle             TargetOracleResult            `json:"target_oracle"`
	TaskCompliance           TargetTaskComplianceResult    `json:"task_compliance"`
	ContractInterpretation   *TargetContractInterpretation `json:"contract_interpretation,omitempty"`
	Signature                core.MismatchSignature        `json:"signature"`
	ArtifactDir              string                        `json:"artifact_dir"`
	Workspace                string                        `json:"workspace"`
	StartedAt                string                        `json:"started_at"`
	FinishedAt               string                        `json:"finished_at"`
}

func TargetAdapters() []TargetAdapterInfo {
	return []TargetAdapterInfo{
		{
			AdapterID:    DefaultTargetAdapterID,
			Implemented:  true,
			Description:  "run any local or container-visible agent command inside a SyncFuzz workspace",
			Capabilities: []string{"run", "reset-by-workspace", "workspace-binding", "stdout-stderr-capture", "filesystem-snapshot", "process-snapshot"},
		},
		{
			AdapterID:    "langgraph",
			Implemented:  false,
			Description:  "planned LangGraph wrapper with checkpoint/replay lifecycle hooks",
			Capabilities: []string{"run", "checkpoint", "replay", "cancel-resume"},
		},
		{
			AdapterID:    "maf",
			Implemented:  false,
			Description:  "planned Microsoft Agent Framework workflow wrapper",
			Capabilities: []string{"run", "workflow-checkpoint", "resume", "rehydrate"},
		},
		{
			AdapterID:    "autogen",
			Implemented:  false,
			Description:  "planned AutoGen command executor wrapper for historical comparison",
			Capabilities: []string{"run", "command-executor", "workspace-binding"},
		},
		{
			AdapterID:    "openhands",
			Implemented:  false,
			Description:  "planned OpenHands runtime/sandbox wrapper",
			Capabilities: []string{"run", "sandbox", "workspace-binding"},
		},
	}
}

func RunTarget(ctx context.Context, opts TargetRunOptions) (*TargetRunResult, error) {
	if opts.AdapterID == "" {
		opts.AdapterID = DefaultTargetAdapterID
	}
	if opts.AdapterID != DefaultTargetAdapterID {
		return nil, fmt.Errorf("target adapter %q is not implemented", opts.AdapterID)
	}
	if opts.TargetID == "" {
		opts.TargetID = opts.AdapterID
	}
	if opts.TaskID == "" {
		opts.TaskID = DefaultTargetTaskID
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.ObserveDelay == 0 && opts.TaskID == DefaultTargetTaskID {
		opts.ObserveDelay = 500 * time.Millisecond
	}
	if opts.EnvKind == "" {
		opts.EnvKind = "local"
	}
	command, err := resolveTargetCommand(opts)
	if err != nil {
		return nil, err
	}
	opts.Command = command
	prompt, promptProfileID, promptVariantID, err := resolveTargetPrompt(opts)
	if err != nil {
		return nil, err
	}
	opts.Prompt = prompt
	opts.PromptProfileID = promptProfileID
	opts.PromptVariantID = promptVariantID
	if opts.Objective == "" {
		opts.Objective = defaultTargetObjective(opts.TaskID)
	}
	if len(opts.ExpectedFiles) == 0 {
		opts.ExpectedFiles = DefaultTargetExpectedFiles(opts.TaskID)
	}

	started := time.Now().UTC()
	env, err := environment.NewEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, core.RunOptions{
		CaseName:       "target-" + sanitizeTargetID(opts.TargetID) + "-" + sanitizeTargetID(opts.TaskID),
		OutDir:         opts.OutDir,
		Workspace:      opts.Workspace,
		EnvKind:        opts.EnvKind,
		ContainerImage: opts.ContainerImage,
		RunRole:        "target",
	}, started, true)
	if err != nil {
		return nil, err
	}
	defer run.Close()
	workspacePath := targetWorkspaceForEnvironment(run)

	if err := run.Trace.Write(core.NewEvent(run, "P0", "target_run_started", map[string]any{
		"adapter_id":         opts.AdapterID,
		"target_id":          opts.TargetID,
		"task_id":            opts.TaskID,
		"environment":        run.Environment,
		"container_image":    run.ContainerImage,
		"workspace":          workspacePath,
		"timeout":            opts.Timeout.String(),
		"observe_delay":      opts.ObserveDelay.String(),
		"late_observe_delay": opts.LateObserveDelay.String(),
	})); err != nil {
		return nil, err
	}

	promptPath := filepath.Join(run.Workspace, TargetPromptArtifact)
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0o644); err != nil {
		return nil, fmt.Errorf("write target prompt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(run.RunDir, TargetPromptArtifact), []byte(opts.Prompt), 0o644); err != nil {
		return nil, fmt.Errorf("write target prompt artifact: %w", err)
	}
	task := TargetTask{
		SchemaVersion:      "syncfuzz.target-task.v1",
		RunID:              run.RunID,
		AdapterID:          opts.AdapterID,
		TargetID:           opts.TargetID,
		TaskID:             opts.TaskID,
		Objective:          opts.Objective,
		PromptProfileID:    opts.PromptProfileID,
		PromptVariantID:    opts.PromptVariantID,
		Prompt:             opts.Prompt,
		PromptFile:         TargetPromptArtifact,
		Command:            opts.Command,
		TimeoutMillis:      opts.Timeout.Milliseconds(),
		ObserveDelayMs:     opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs: opts.LateObserveDelay.Milliseconds(),
		Environment:        run.Environment,
		ContainerImage:     run.ContainerImage,
		Workspace:          workspacePath,
		ExpectedFiles:      opts.ExpectedFiles,
		CreatedAt:          started.Format(time.RFC3339Nano),
	}
	if scenario, ok := targetScenarioByID(opts.TaskID); ok {
		info := scenario.Info
		info.DefaultExpectedFiles = append([]string{}, info.DefaultExpectedFiles...)
		info.LateExpectedFiles = append([]string{}, info.LateExpectedFiles...)
		info.Components = append([]TargetScenarioComponent{}, info.Components...)
		info.Mutations = append([]TargetScenarioMutation{}, info.Mutations...)
		info.ExecutionPlan = targetScenarioExecutionPlanInfo(scenario.Lifecycle)
		task.Scenario = &info
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, TargetTaskArtifact), task); err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.Workspace, TargetTaskArtifact), task); err != nil {
		return nil, err
	}
	if err := run.Trace.Write(core.NewEvent(run, "P1", "target_task_prepared", map[string]any{
		"artifact":       TargetTaskArtifact,
		"prompt_file":    TargetPromptArtifact,
		"prompt_profile": opts.PromptProfileID,
		"prompt_variant": opts.PromptVariantID,
		"command":        opts.Command,
	})); err != nil {
		return nil, err
	}

	before, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-before.json"), before); err != nil {
		return nil, err
	}
	processBefore, err := core.RecordProcessSnapshot(ctx, env, run, "P0", "process-before.json")
	if err != nil {
		return nil, err
	}

	commandResult, output, err := execTargetCommand(ctx, env, run, opts, workspacePath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(run.RunDir, TargetOutputArtifact), output, 0o644); err != nil {
		return nil, fmt.Errorf("write target output: %w", err)
	}
	if err := run.Trace.Write(core.NewEvent(run, "P5", "target_command_returned", map[string]any{
		"exit_code":    commandResult.ExitCode,
		"timed_out":    commandResult.TimedOut,
		"duration_ms":  commandResult.DurationMs,
		"output_bytes": commandResult.OutputBytes,
		"output":       TargetOutputArtifact,
	})); err != nil {
		return nil, err
	}
	processAfterCommand, err := core.RecordProcessSnapshot(ctx, env, run, "P5", "process-after-command.json")
	if err != nil {
		return nil, err
	}

	if err := waitForTargetObservation(ctx, run, "P6", "target_observation_delay", opts.ObserveDelay); err != nil {
		return nil, err
	}
	after, processAfter, err := observeTargetWorkspace(ctx, env, run, "P6", "snapshot-after.json", "process-after.json")
	if err != nil {
		return nil, err
	}
	processLineage, err := core.RecordProcessLineage(run, "P6", "process-lineage.json", processBefore, processAfterCommand, processAfter, "process-before.json", "process-after-command.json", "process-after.json")
	if err != nil {
		return nil, err
	}
	if _, err := core.RecordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []core.FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
		return nil, err
	}

	lateObserved := opts.LateObserveDelay > 0
	var late core.Snapshot
	if lateObserved {
		if err := waitForTargetObservation(ctx, run, "P7", "target_late_observation_delay", opts.LateObserveDelay); err != nil {
			return nil, err
		}
		if late, _, err = observeTargetWorkspace(ctx, env, run, "P7", TargetSnapshotLateArtifact, TargetProcessLateArtifact); err != nil {
			return nil, err
		}
		if _, err := core.RecordFilesystemMetadata(run, "P7", TargetFilesystemLateArtifact, []core.FilesystemSnapshotArtifact{
			{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
			{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
			{Phase: "P7", Artifact: TargetSnapshotLateArtifact, Snapshot: late},
		}); err != nil {
			return nil, err
		}
	}

	present, missing := expectedFileStatus(after, opts.ExpectedFiles)
	lateExpected := defaultTargetLateExpectedFiles(opts.TaskID)
	var latePresent []string
	var lateMissing []string
	if lateObserved {
		latePresent, lateMissing = expectedFileStatus(late, lateExpected)
	}
	completed := commandResult.ExitCode == 0 && !commandResult.TimedOut
	targetOracle := evaluateTargetOracle(run.Workspace, opts.TargetID, opts.TaskID, completed, missing, processLineage.Summary, lateObserved, latePresent, lateMissing)
	taskCompliance := evaluateTargetTaskComplianceForTarget(run.Workspace, opts.TargetID, opts.TaskID)
	contractProfile := TargetContractProfileFor(opts.TargetID)
	if contractProfile != nil {
		if err := core.WriteJSON(filepath.Join(run.RunDir, TargetContractProfileArtifact), contractProfile); err != nil {
			return nil, err
		}
	}
	contractInterpretation := EvaluateTargetContractInterpretation(contractProfile, opts.TaskID, targetOracle, taskCompliance)
	expectationsMet := targetOracle.Confirmed
	signature := TargetSignature(opts.TaskID)
	evidence := targetEvidence(completed, expectationsMet, present, missing, commandResult)
	evidence = append(evidence, targetOracle.Evidence...)
	evidence = append(evidence, targetOracleMissingEvidence(targetOracle)...)
	if contractInterpretation != nil {
		if contractInterpretation.Summary != "" {
			evidence = append(evidence, "contract interpretation: "+contractInterpretation.Summary)
		}
		evidence = append(evidence, contractInterpretation.Evidence...)
	}
	artifacts := []string{
		"trace.jsonl",
		TargetTaskArtifact,
		TargetPromptArtifact,
		TargetOutputArtifact,
		"snapshot-before.json",
		"process-before.json",
		"process-after-command.json",
		"snapshot-after.json",
		"process-after.json",
		"process-lineage.json",
		"filesystem-metadata.json",
		TargetResultArtifact,
	}
	observations := []core.StateObservation{
		{Layer: "agent", StateClass: "target-task", Phase: "P1", Artifact: TargetTaskArtifact, Kind: "target-task", Description: "prompt and command contract passed to the real target adapter"},
		{Layer: "agent", StateClass: "target-output", Phase: "P5", Artifact: TargetOutputArtifact, Kind: "stdout-stderr", Description: "combined stdout/stderr from the target command"},
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P5", Artifact: "process-after-command.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P6", Artifact: "snapshot-after.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-after.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P6", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
	}
	if contractProfile != nil {
		artifacts = append(artifacts, TargetContractProfileArtifact)
		observations = append(observations, core.StateObservation{
			Layer:       "agent",
			StateClass:  "target-contract-profile",
			Phase:       "P6",
			Artifact:    TargetContractProfileArtifact,
			Kind:        "json-summary",
			Description: "contract profile used to interpret real-target residue against the selected lifecycle boundary",
		})
	}
	faultPhases := []string{"P1 target task prepared", "P5 target command returned", "P6 target workspace observed"}
	if lateObserved {
		artifacts = append(artifacts, TargetSnapshotLateArtifact, TargetProcessLateArtifact, TargetFilesystemLateArtifact)
		observations = append(observations,
			core.StateObservation{Layer: "os", StateClass: "filesystem", Phase: "P7", Artifact: TargetSnapshotLateArtifact, Kind: "filesystem-snapshot", Description: "late filesystem observation after delayed background effects can materialize"},
			core.StateObservation{Layer: "os", StateClass: "process", Phase: "P7", Artifact: TargetProcessLateArtifact, Kind: "process-snapshot", Description: "late process observation after delayed background effects can complete"},
			core.StateObservation{Layer: "os", StateClass: "filesystem-metadata", Phase: "P7", Artifact: TargetFilesystemLateArtifact, Kind: "filesystem-metadata", Description: "filesystem deltas across immediate and late target observations"},
		)
		faultPhases = append(faultPhases, "P7 late target workspace observed")
	}
	adapterArtifacts, adapterObservations := targetAdapterRuntimeObservations(run.Workspace)
	artifacts = append(artifacts, adapterArtifacts...)
	observations = append(observations, adapterObservations...)

	manifest := core.CaseManifest{
		Objective:         opts.Objective,
		StateClasses:      []string{"workspace", "process", "target-command"},
		FaultPhases:       faultPhases,
		Primitives:        []string{"real target command adapter", opts.AdapterID, opts.TaskID},
		ExpectedSignature: signature,
		Artifacts:         core.AppendUniqueStrings(artifacts, core.AgentStateArtifact, core.StateTraceArtifact),
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, expectationsMet, evidence, observations); err != nil {
		return nil, err
	}
	if err := core.WriteManifest(run, manifest); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &TargetRunResult{
		SchemaVersion:            "syncfuzz.target-result.v1",
		RunID:                    run.RunID,
		AdapterID:                opts.AdapterID,
		TargetID:                 opts.TargetID,
		TaskID:                   opts.TaskID,
		Objective:                opts.Objective,
		PromptProfileID:          opts.PromptProfileID,
		PromptVariantID:          opts.PromptVariantID,
		Environment:              run.Environment,
		ContainerImage:           run.ContainerImage,
		Command:                  opts.Command,
		TimeoutMillis:            opts.Timeout.Milliseconds(),
		ObserveDelayMs:           opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs:       opts.LateObserveDelay.Milliseconds(),
		Completed:                completed,
		ExpectationsMet:          expectationsMet,
		ExpectedFiles:            opts.ExpectedFiles,
		ExpectedFilesPresent:     present,
		ExpectedFilesMissing:     missing,
		LateObserved:             lateObserved,
		LateExpectedFiles:        lateExpected,
		LateExpectedFilesPresent: latePresent,
		LateExpectedFilesMissing: lateMissing,
		CommandResult:            commandResult,
		ProcessLineage:           processLineage.Summary,
		TargetOracle:             targetOracle,
		TaskCompliance:           taskCompliance,
		ContractInterpretation:   contractInterpretation,
		Signature:                signature,
		ArtifactDir:              run.RunDir,
		Workspace:                workspacePath,
		StartedAt:                started.Format(time.RFC3339Nano),
		FinishedAt:               finished.Format(time.RFC3339Nano),
	}
	if err := run.Trace.Write(core.NewEvent(run, "oracle", "target_result", map[string]any{
		"completed":                   completed,
		"expectations_met":            expectationsMet,
		"target_oracle":               targetOracle.Name,
		"task_compliance":             taskCompliance.Name,
		"task_compliance_status":      taskCompliance.Status,
		"contract_status":             TargetContractInterpretationStatusValue(contractInterpretation),
		"contract_rule_id":            TargetContractInterpretationRuleIDValue(contractInterpretation),
		"prompt_profile":              opts.PromptProfileID,
		"prompt_variant":              opts.PromptVariantID,
		"oracle_confirmed":            targetOracle.Confirmed,
		"workspace_remaining_after":   processLineage.Summary.WorkspaceRemainingAfter,
		"late_observed":               lateObserved,
		"late_expected_files_present": latePresent,
		"late_expected_files_missing": lateMissing,
		"expected_files_present":      present,
		"expected_files_missing":      missing,
	})); err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, TargetResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func resolveTargetPrompt(opts TargetRunOptions) (string, string, string, error) {
	profileID := strings.TrimSpace(opts.PromptProfileID)
	if profileID != "" {
		profile, err := resolveTargetPromptProfile(profileID)
		if err != nil {
			return "", "", "", err
		}
		profileID = profile.ProfileID
	}
	variant, err := resolveTargetPromptVariant(opts.PromptVariantID)
	if err != nil {
		return "", "", "", err
	}
	if opts.PromptFile != "" {
		raw, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return "", "", "", fmt.Errorf("read target prompt file: %w", err)
		}
		return string(raw), profileID, variant.VariantID, nil
	}
	if opts.Prompt != "" {
		return opts.Prompt, profileID, variant.VariantID, nil
	}
	profileID = NormalizeTargetPromptProfileID(profileID)
	return defaultTargetPromptVariantForTargetWithProfile(opts.TargetID, opts.TaskID, profileID, variant.VariantID), profileID, variant.VariantID, nil
}

func resolveTargetCommand(opts TargetRunOptions) (string, error) {
	if strings.TrimSpace(opts.Command) != "" && strings.TrimSpace(opts.CommandFile) != "" {
		return "", errors.New("target command and command file are mutually exclusive")
	}
	if opts.CommandFile != "" {
		raw, err := os.ReadFile(opts.CommandFile)
		if err != nil {
			return "", fmt.Errorf("read target command file: %w", err)
		}
		command := strings.TrimSpace(string(raw))
		if command == "" {
			return "", errors.New("target command file is empty")
		}
		return command, nil
	}
	if strings.TrimSpace(opts.Command) == "" {
		return "", errors.New("target command is required")
	}
	return opts.Command, nil
}

func defaultTargetObjective(taskID string) string {
	if scenario, ok := targetScenarioByID(taskID); ok {
		return scenario.Info.Objective
	}
	return "Run a real target under SyncFuzz observation."
}

func DefaultTargetPrompt(taskID string) string {
	return DefaultTargetPromptWithProfile(taskID, TargetPromptProfileBaselineID)
}

func DefaultTargetPromptWithProfile(taskID string, profileID string) string {
	return defaultTargetPromptForTargetWithProfile("", taskID, profileID)
}

func defaultTargetPromptForTargetWithProfile(targetID string, taskID string, profileID string) string {
	profileID = NormalizeTargetPromptProfileID(profileID)
	if prompt, ok := targetSpecificPrompt(targetID, taskID); ok {
		return applyTargetPromptProfile(strings.TrimSpace(prompt), profileID)
	}
	if scenario, ok := targetScenarioByID(taskID); ok {
		return applyTargetPromptProfile(strings.TrimSpace(scenario.Prompt), profileID)
	}
	return applyTargetPromptProfile("You are running inside a SyncFuzz workspace. Complete the requested task in the current working directory and leave observable artifacts.", profileID)
}

func targetSpecificPrompt(targetID string, taskID string) (string, bool) {
	switch strings.TrimSpace(targetID) {
	case "maf-github-copilot-shell":
		switch taskID {
		case DefaultTargetTaskID:
			return MAFOrphanProcessPrompt, true
		case PersistentShellTargetTaskID:
			return MAFPersistentShellPrompt, true
		}
	}
	return "", false
}

func DefaultTargetExpectedFiles(taskID string) []string {
	if scenario, ok := targetScenarioByID(taskID); ok {
		return append([]string{}, scenario.Info.DefaultExpectedFiles...)
	}
	return nil
}

func defaultTargetLateExpectedFiles(taskID string) []string {
	if scenario, ok := targetScenarioByID(taskID); ok {
		return append([]string{}, scenario.Info.LateExpectedFiles...)
	}
	return nil
}

func DefaultTargetLateObserveDelay(taskID string) time.Duration {
	if scenario, ok := targetScenarioByID(taskID); ok {
		return time.Duration(scenario.Info.LateObserveDelayMs) * time.Millisecond
	}
	return 0
}

func evaluateTargetOracle(workspace string, targetID string, taskID string, completed bool, immediateMissing []string, lineage core.ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	switch taskID {
	case LongDelayTargetTaskID:
		if strings.TrimSpace(targetID) == "maf-github-copilot-shell" {
			return evaluateMAFLongDelayTargetOracle(workspace, completed, lateObserved, latePresent, lateMissing)
		}
		return evaluateLongDelayTargetOracle(completed, lineage, lateObserved, latePresent, lateMissing)
	case PersistentShellTargetTaskID:
		if strings.TrimSpace(targetID) == "maf-github-copilot-shell" {
			return evaluateMAFPersistentShellTargetOracle(workspace, completed, immediateMissing)
		}
		return evaluatePersistentShellTargetOracle(workspace, completed, immediateMissing)
	case FileResidueTargetTaskID, DirectoryResidueTargetTaskID, DeleteResidueTargetTaskID,
		SymlinkResidueTargetTaskID, RenameResidueTargetTaskID, ModeResidueTargetTaskID,
		AppendResidueTargetTaskID, HardlinkResidueTargetTaskID, FifoResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetOracle(workspace, targetID, taskID, completed, immediateMissing)
	case UnixListenerResidueTargetTaskID:
		return evaluateUnixListenerResidueTargetOracle(workspace, targetID, completed, immediateMissing)
	case EnvResidueTargetTaskID:
		return evaluateEnvResidueTargetOracle(workspace, targetID, completed, immediateMissing)
	case FunctionResidueTargetTaskID:
		return evaluateFunctionResidueTargetOracle(workspace, targetID, completed, immediateMissing)
	case PersistentShellReplayTargetTaskID:
		return evaluatePersistentShellReplayTargetOracle(workspace, completed, immediateMissing)
	case PersistentShellForkTargetTaskID:
		return evaluatePersistentShellForkTargetOracle(workspace, completed, immediateMissing)
	case FileResidueForkTargetTaskID:
		return evaluateFileResidueForkTargetOracle(workspace, completed, immediateMissing)
	case DirectoryResidueForkTargetTaskID:
		return evaluateDirectoryResidueForkTargetOracle(workspace, completed, immediateMissing)
	case DeleteResidueForkTargetTaskID:
		return evaluateDeleteResidueForkTargetOracle(workspace, completed, immediateMissing)
	case SymlinkResidueForkTargetTaskID:
		return evaluateSymlinkResidueForkTargetOracle(workspace, completed, immediateMissing)
	case RenameResidueForkTargetTaskID:
		return evaluateRenameResidueForkTargetOracle(workspace, completed, immediateMissing)
	case ModeResidueForkTargetTaskID:
		return evaluateModeResidueForkTargetOracle(workspace, completed, immediateMissing)
	case AppendResidueForkTargetTaskID:
		return evaluateAppendResidueForkTargetOracle(workspace, completed, immediateMissing)
	case HardlinkResidueForkTargetTaskID:
		return evaluateHardlinkResidueForkTargetOracle(workspace, completed, immediateMissing)
	case FifoResidueForkTargetTaskID:
		return evaluateFIFOResidueForkTargetOracle(workspace, completed, immediateMissing)
	case OpenFDResidueForkTargetTaskID:
		return evaluateOpenFDResidueForkTargetOracle(workspace, completed, immediateMissing)
	case DeletedOpenFDForkTargetTaskID:
		return evaluateDeletedOpenFDResidueForkTargetOracle(workspace, completed, immediateMissing)
	case InheritedFDLeakTargetTaskID:
		return evaluateInheritedFDLeakTargetOracle(workspace, completed, immediateMissing)
	case UnixListenerResidueForkTargetTaskID:
		return evaluateUnixListenerResidueForkTargetOracle(workspace, completed, immediateMissing)
	case DiscardedServerTrustedClientTargetTaskID:
		return evaluateDiscardedServerTrustedClientTargetOracle(workspace, completed, immediateMissing)
	case SocketResponsePoisoningTargetTaskID:
		return evaluateSocketResponsePoisoningTargetOracle(workspace, completed, immediateMissing)
	case CWDResidueTargetTaskID:
		return evaluateCWDResidueTargetOracle(workspace, targetID, completed, immediateMissing)
	case UmaskResidueTargetTaskID:
		return evaluateUmaskResidueTargetOracle(workspace, targetID, completed, immediateMissing)
	case CWDResidueForkTargetTaskID:
		return evaluateCWDResidueForkTargetOracle(workspace, completed, immediateMissing)
	case UmaskResidueForkTargetTaskID:
		return evaluateUmaskResidueForkTargetOracle(workspace, completed, immediateMissing)
	default:
		oracle := newTargetOracleResult("command-and-expected-files")
		if completed {
			oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
		} else {
			markTargetOracleNegative(&oracle, "target command completed successfully")
		}
		if len(immediateMissing) == 0 {
			oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
		} else {
			markTargetOracleNegative(&oracle, immediateMissing...)
		}
		return finalizeTargetOracle(oracle)
	}
}

func evaluatePersistentShellTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("persistent-shell-poisoning")
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleInconclusive(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}

	witness, err := readTargetOracleFile(workspace, TargetShellPoisonCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetShellPoisonCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed git witness: "+witness)
	transcript, err := inspectLangGraphPersistentShellEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Confirmed {
			oracle.Attribution = transcript.Attribution
			oracle.Evidence = append(oracle.Evidence, "langgraph transcript confirmed persistent shell reuse across later shell calls")
			return finalizeTargetOracle(oracle)
		}
	}

	witnessShowsShimPath := outputShowsAttackerPath(witness)
	witnessShowsShimMarker := outputShowsAttackerMarker(witness)
	if witnessShowsShimPath {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
		oracle.Evidence = append(oracle.Evidence, "witness resolved or executed the workspace-local git shim")
		if transcript.Available {
			markTargetOracleInconclusive(&oracle, "langgraph transcript proved that the later shim resolution came from persistent shell reuse")
		}
		return finalizeTargetOracle(oracle)
	}

	if transcript.Available {
		markTargetOracleNegative(&oracle, "langgraph transcript confirmed persistent shell reuse across later shell calls")
		return finalizeTargetOracle(oracle)
	}
	if outputShowsSystemGitResolution(witness) {
		markTargetOracleNegative(&oracle, "witness resolved or executed the workspace-local git shim")
		return finalizeTargetOracle(oracle)
	}
	if witnessShowsShimMarker {
		markTargetOracleInconclusive(&oracle, "transcript-backed proof that the shim marker came from a later shell call without another PATH export")
		return finalizeTargetOracle(oracle)
	}
	markTargetOracleInconclusive(&oracle, "witness resolved or executed the workspace-local git shim")
	return finalizeTargetOracle(oracle)
}

func evaluatePersistentShellReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("persistent-shell-poisoning-replay")
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleInconclusive(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}

	witness, err := readTargetOracleFile(workspace, TargetShellPoisonReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetShellPoisonReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	sawInitialExport, err := langgraphHistoryShowsPathExport(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialExport {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured an earlier PATH export to the workspace-local tool directory")
	} else {
		markTargetOracleInconclusive(&oracle, "langgraph history captured an earlier PATH export to the workspace-local tool directory")
	}

	transcript, err := inspectLangGraphReplayPoisonEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if !transcript.Confirmed {
			markTargetOracleStatusFromAttribution(&oracle, transcript.Attribution)
		}
	} else {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary artifact was present and decodable")
	}

	smuggling, err := inspectLangGraphReplayStateSmuggling(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if smuggling.Available && smuggling.Smuggled {
		oracle.Attribution = TargetOracleAttributionExternalSmuggling
		markTargetOracleNegative(&oracle)
		oracle.Evidence = append(oracle.Evidence, smuggling.Details...)
	}

	duplicatedPath := maxAttackerBinEntriesInPath(witness) >= 2
	attackerPathInWitness := outputShowsAttackerPathEnv(witness)
	attackerGitInWitness := outputShowsAttackerPath(witness) || outputShowsAttackerMarker(witness)
	if duplicatedPath {
		oracle.Evidence = append(oracle.Evidence, "replay witness showed the workspace-local tool directory duplicated inside PATH")
	} else if attackerPathInWitness {
		oracle.Evidence = append(oracle.Evidence, "replay witness still showed the workspace-local tool directory inside PATH after replay")
	} else {
		markTargetOracleStatusFromAttribution(&oracle, oracle.Attribution)
		appendTargetOracleMissing(&oracle, "replay witness preserved the workspace-local PATH override across replay")
	}

	if attackerGitInWitness {
		oracle.Evidence = append(oracle.Evidence, "replay witness resolved or executed the workspace-local git shim")
	} else {
		markTargetOracleStatusFromAttribution(&oracle, oracle.Attribution)
		appendTargetOracleMissing(&oracle, "replay witness resolved or executed the workspace-local git shim")
	}
	if !oracle.Confirmed {
		appendTargetOracleMissing(&oracle, replayAttributionMissingReason(oracle.Attribution))
	}
	return finalizeTargetOracle(oracle)
}

func evaluatePersistentShellForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("persistent-shell-poisoning-fork")
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleInconclusive(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}

	witness, err := readTargetOracleFile(workspace, TargetShellPoisonForkArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetShellPoisonForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	sawInitialExport, err := langgraphHistoryShowsPathExport(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialExport {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured an earlier PATH export to the workspace-local tool directory")
	} else {
		markTargetOracleInconclusive(&oracle, "langgraph history captured an earlier PATH export to the workspace-local tool directory")
	}

	transcript, err := inspectLangGraphForkPoisonEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if !transcript.Confirmed {
			markTargetOracleStatusFromAttribution(&oracle, transcript.Attribution)
			appendTargetOracleMissing(&oracle, forkAttributionMissingReason(oracle.Attribution))
		}
	} else {
		markTargetOracleInconclusive(&oracle, "langgraph fork summary artifact was present and decodable")
	}

	if outputShowsAttackerPathEnv(witness) {
		oracle.Evidence = append(oracle.Evidence, "fork witness still showed the workspace-local tool directory inside PATH")
	} else {
		markTargetOracleStatusFromAttribution(&oracle, oracle.Attribution)
		appendTargetOracleMissing(&oracle, "fork witness still showed the workspace-local tool directory inside PATH")
	}

	if outputShowsAttackerPath(witness) || outputShowsAttackerMarker(witness) {
		oracle.Evidence = append(oracle.Evidence, "fork witness resolved or executed the workspace-local git shim")
	} else {
		markTargetOracleStatusFromAttribution(&oracle, oracle.Attribution)
		appendTargetOracleMissing(&oracle, "fork witness resolved or executed the workspace-local git shim")
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown && replayOutputLooksObserved(witness) {
		oracle.Attribution = TargetOracleAttributionCleanFork
		markTargetOracleNegative(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateFileResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "file-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetFileResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetFileResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	expectedMarker, err := targetRunMarker(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	}
	switch {
	case expectedMarker != "" && strings.Contains(witness, expectedMarker):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved the expected branch-note marker")
	case outputShowsFileResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved a branch-note marker")
	case strings.Contains(witness, "MISSING_BRANCH_NOTE"):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-note.txt was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-note marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetFileResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-note.txt creation")
	}

	transcript, err := inspectLangGraphForkFileResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fork residue occurred without recreating branch-note.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-note.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-note.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateDirectoryResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "directory-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetDirectoryResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetDirectoryResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsDirectoryResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved branch-dir as a directory")
	case outputShowsMissingBranchDir(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-dir was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-dir marker")
	}

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceDirectoryCreate(workspace, TargetDirectoryResidueDirArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCreate {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-dir directory creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-dir directory creation")
	}

	transcript, err := inspectLangGraphForkDirectoryResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fork residue occurred without recreating branch-dir during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-dir across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-dir")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateDeleteResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "delete-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetDeleteResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetDeleteResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	expectedMarker, err := targetDeleteRunMarker(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	}
	switch {
	case outputShowsMissingBranchDeleteNote(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-delete-note.txt was absent")
	case expectedMarker != "" && strings.Contains(witness, expectedMarker):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved the expected branch-delete-note marker")
	case outputShowsDeleteResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved a branch-delete-note marker")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained either the branch-delete-note marker or MISSING_BRANCH_DELETE_NOTE")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetDeleteResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-delete-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-delete-note.txt creation")
	}

	sawInitialDelete, err := langgraphHistoryShowsWorkspaceFileDelete(workspace, TargetDeleteResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialDelete {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-delete-note.txt deletion")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-delete-note.txt deletion")
	}

	transcript, err := inspectLangGraphForkDeleteResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "delete residue occurred without modifying branch-delete-note.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-delete-note.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing branch-delete-note.txt in the fork workspace")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateSymlinkResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "symlink-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetSymlinkResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetSymlinkResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsSymlinkResidueTarget(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved branch-link.txt -> target-prompt.txt")
	case outputShowsMissingBranchLink(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-link.txt was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-link.txt target")
	}

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceSymlinkCreate(workspace, TargetSymlinkResidueLinkArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCreate {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-link.txt symlink creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-link.txt symlink creation")
	}

	transcript, err := inspectLangGraphForkSymlinkResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fork residue occurred without recreating branch-link.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-link.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-link.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateRenameResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "rename-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetRenameResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetRenameResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsRenameResidueDest(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved the renamed destination file branch-rename-dst.txt")
	case outputShowsRenameResidueSource(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the original branch-rename-src.txt still existed")
	case outputShowsMissingRenameArtifacts(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that neither branch-rename-src.txt nor branch-rename-dst.txt existed")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable rename-side marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetRenameResidueSourceArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-rename-src.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-rename-src.txt creation")
	}

	sawInitialRename, err := langgraphHistoryShowsWorkspaceRename(workspace, TargetRenameResidueSourceArtifact, TargetRenameResidueDestArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialRename {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the rename from branch-rename-src.txt to branch-rename-dst.txt")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the rename from branch-rename-src.txt to branch-rename-dst.txt")
	}

	transcript, err := inspectLangGraphForkRenameResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "rename residue occurred without recreating, deleting, or re-renaming the branch-rename files during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-rename-dst.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing the existing rename-side files")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateModeResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "mode-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetModeResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetModeResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsModeResidue(witness, targetModeResidueTightenedMode):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved the tightened "+targetModeResidueTightenedMode+" mode on branch-mode-note.txt")
	case outputShowsModeResidue(witness, "644"):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-mode-note.txt had mode 644")
	case outputShowsMissingBranchModeNote(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-mode-note.txt was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-mode-note.txt mode marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetModeResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-mode-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-mode-note.txt creation")
	}

	sawInitialChmod, err := langgraphHistoryShowsWorkspaceModeChange(workspace, TargetModeResidueNoteArtifact, targetModeResidueTightenedMode)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialChmod {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the chmod that tightened branch-mode-note.txt to "+targetModeResidueTightenedMode)
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the chmod that tightened branch-mode-note.txt to "+targetModeResidueTightenedMode)
	}

	transcript, err := inspectLangGraphForkModeResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "mode residue occurred without rewriting, deleting, or chmod-ing branch-mode-note.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the tightened "+targetModeResidueTightenedMode+" mode across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing the existing mode of branch-mode-note.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateAppendResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "append-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetAppendResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetAppendResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsAppendResidueBaseMarker(witness) && outputShowsAppendResidueExtraMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved the appended extra marker in branch-append-note.txt")
	case outputShowsAppendResidueBaseMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that only the base marker was present in branch-append-note.txt")
	case outputShowsMissingBranchAppendNote(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-append-note.txt was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-append-note.txt content marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetAppendResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-append-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-append-note.txt creation")
	}

	sawInitialAppend, err := langgraphHistoryShowsWorkspaceAppend(workspace, TargetAppendResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialAppend {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the append that added the extra marker to branch-append-note.txt")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the append that added the extra marker to branch-append-note.txt")
	}

	transcript, err := inspectLangGraphForkAppendResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "append residue occurred without rewriting, deleting, or appending branch-append-note.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the appended extra marker across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing the existing contents of branch-append-note.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateHardlinkResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "hardlink-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetHardlinkResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetHardlinkResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsHardlinkResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved branch-hardlink.txt as a hardlink-like entry")
	case outputShowsMissingBranchHardlink(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-hardlink.txt was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-hardlink.txt marker")
	}

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceHardlinkCreate(workspace, TargetHardlinkResidueLinkArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCreate {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-hardlink.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-hardlink.txt creation")
	}

	transcript, err := inspectLangGraphForkHardlinkResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "hardlink residue occurred without recreating branch-hardlink.txt during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-hardlink.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-hardlink.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateFIFOResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "fifo-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetFIFOResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetFIFOResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsFIFOResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved branch-fifo as a named pipe")
	case outputShowsMissingBranchFIFO(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that branch-fifo was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-fifo marker")
	}

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceFIFOCreate(workspace, TargetFIFOResiduePipeArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCreate {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-fifo creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-fifo creation")
	}

	transcript, err := inspectLangGraphForkFIFOResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fifo residue occurred without recreating branch-fifo during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-fifo across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-fifo")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateOpenFDResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "open-fd-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetOpenFDResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetOpenFDResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsOpenFDResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved a process that still held branch-fd-note.txt open")
	case outputShowsMissingOpenFDResidue(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the branch-fd-note.txt holder was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable branch-fd-note.txt fd marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetOpenFDResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-fd-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-fd-note.txt creation")
	}

	sawInitialOpenFD, err := langgraphHistoryShowsWorkspaceOpenFD(workspace, TargetOpenFDResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialOpenFD {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-fd-note.txt fd holder launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-fd-note.txt fd holder launch")
	}

	transcript, err := inspectLangGraphForkOpenFDResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "open-fd residue occurred without relaunching the branch-fd-note.txt holder during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the branch-fd-note.txt holder across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness did not come from observing an already-running branch-fd-note.txt holder")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateDeletedOpenFDResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "deleted-open-fd-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetDeletedOpenFDForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetDeletedOpenFDForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsDeletedOpenFDResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness preserved a process that still held the deleted branch-deleted-fd-note.txt inode open")
	case outputShowsMissingDeletedOpenFDResidue(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the deleted branch-deleted-fd-note.txt holder was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable deleted-open-fd marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetDeletedOpenFDNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-deleted-fd-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-deleted-fd-note.txt creation")
	}

	sawInitialDeletedOpenFD, err := langgraphHistoryShowsWorkspaceDeletedOpenFD(workspace, TargetDeletedOpenFDNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialDeletedOpenFD {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the deleted branch-deleted-fd-note.txt holder launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the deleted branch-deleted-fd-note.txt holder launch")
	}

	transcript, err := inspectLangGraphForkDeletedOpenFDResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "deleted-open-fd residue occurred without relaunching the deleted branch-deleted-fd-note.txt holder during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the deleted branch-deleted-fd-note.txt holder across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness did not come from observing an already-running deleted branch-deleted-fd-note.txt holder")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateInheritedFDLeakTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "inherited-fd-branch-leakage",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetInheritedFDLeakForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetInheritedFDLeakForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsInheritedFDLeakageMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness recovered the discarded branch secret through an inherited fd")
	case outputShowsMissingInheritedFDLeakage(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the inherited fd secret was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable inherited fd branch-leakage marker")
	}

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetInheritedFDLeakSecretArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch secret creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch secret creation")
	}

	sawInitialHolder, err := langgraphHistoryShowsWorkspaceDeletedOpenFD(workspace, TargetInheritedFDLeakSecretArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialHolder {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the discarded branch fd holder launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the discarded branch fd holder launch")
	}

	transcript, err := inspectLangGraphForkInheritedFDLeakEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "inherited fd leakage occurred without relaunching or recreating the discarded branch secret during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the discarded branch fd capability across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from reading an already-running discarded branch fd holder")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateUnixListenerResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "unix-listener-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetUnixListenerForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetUnixListenerForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsUnixListenerResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness received a response from the branch Unix listener")
	case outputShowsMissingUnixListenerResidue(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the branch Unix listener was absent")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable Unix listener marker")
	}

	sawInitialLaunch, err := langgraphHistoryShowsUnixListenerLaunch(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialLaunch {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch Unix listener launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch Unix listener launch")
	}

	transcript, err := inspectLangGraphForkUnixListenerResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "Unix listener residue occurred without relaunching the branch listener during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved the branch Unix listener across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from connecting to an already-running branch Unix listener")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateDiscardedServerTrustedClientTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        DiscardedServerTrustedClientTargetTaskID,
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetDiscardedServerTrustedClientArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetDiscardedServerTrustedClientArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsDiscardedServerTrustedClientMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor-branch trusted client consumed the discarded branch response")
	case outputShowsMissingDiscardedServerTrustedClient(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor-branch trusted client could not consume the discarded branch response")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable trusted-client residue marker")
	}

	sawInitialLaunch, err := langgraphHistoryShowsUnixListenerLaunch(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialLaunch {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch Unix listener launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch Unix listener launch")
	}

	transcript, err := inspectLangGraphForkDiscardedServerTrustedClientEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "trusted-client consumption occurred without relaunching the branch listener during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork let the successor branch consume the discarded branch service response across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from consuming an already-running discarded branch service response")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateSocketResponsePoisoningTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        SocketResponsePoisoningTargetTaskID,
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetSocketResponsePoisoningArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetSocketResponsePoisoningArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	switch {
	case outputShowsSocketResponsePoisoningMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor branch cached the discarded branch response")
	case outputShowsMissingSocketResponsePoisoning(witness):
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor branch did not cache the discarded branch response")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable response-poisoning marker")
	}

	sawInitialLaunch, err := langgraphHistoryShowsUnixListenerLaunch(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialLaunch {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch Unix listener launch")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch Unix listener launch")
	}

	transcript, err := inspectLangGraphForkSocketResponsePoisoningEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "response caching occurred without relaunching the branch listener during the fork follow-up")
			case TargetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork let the successor branch cache the discarded branch response across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from caching an already-running discarded branch response")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateCWDResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "cwd-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}

	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetCWDResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetCWDResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	witnessKind := ""
	switch {
	case outputShowsCWDResidueMarker(witness):
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor branch still started inside branch-cwd-dir")
	case outputShowsMissingBranchCWDResidue(witness):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "fork witness reported that the successor branch started outside branch-cwd-dir")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness contained a recognizable CWD residue marker")
	}

	if pwd := targetOracleLineValue(witness, "PWD="); pwd != "" {
		oracle.Evidence = append(oracle.Evidence, "fork witness recorded pwd: "+pwd)
	}
	if relativeWitness := targetOracleLineValue(witness, "RELATIVE_WITNESS="); relativeWitness != "" {
		oracle.Evidence = append(oracle.Evidence, "fork witness recorded relative witness path: "+relativeWitness)
	}

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceDirectoryCreate(workspace, TargetCWDResidueDirArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCreate {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-cwd-dir creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-cwd-dir creation")
	}

	sawInitialCD, err := langgraphHistoryShowsWorkingDirectoryChange(workspace, TargetCWDResidueDirArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialCD {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial cd into branch-cwd-dir")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial cd into branch-cwd-dir")
	}

	transcript, err := inspectLangGraphForkCWDResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "cwd residue occurred without changing directories during the fork follow-up")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing the inherited cwd without another cd")
			}
		} else {
			switch witnessKind {
			case "residue":
				oracle.Attribution = TargetOracleAttributionRuntimeResidue
			case "clean":
				oracle.Confirmed = false
				oracle.Attribution = TargetOracleAttributionCleanFork
				appendTargetOracleMissing(&oracle, "fork preserved branch-cwd-dir as the active cwd across the checkpoint boundary")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func evaluateUmaskResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "umask-residue-fork",
		Confirmed:   true,
		Attribution: TargetOracleAttributionUnknown,
	}

	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, immediateMissing...)
		return oracle
	}

	witness, err := readTargetOracleFile(workspace, TargetUmaskResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+TargetUmaskResidueForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	baselineUmask := targetOracleLineValue(witness, "BASELINE_UMASK=")
	witnessMode := targetOracleLineValue(witness, "WITNESS_MODE=")
	if baselineUmask != "" {
		oracle.Evidence = append(oracle.Evidence, "fork witness recorded baseline umask: "+baselineUmask)
	}
	if witnessMode != "" {
		oracle.Evidence = append(oracle.Evidence, "fork witness recorded umask-witness.txt mode: "+witnessMode)
	}

	witnessKind := ""
	baselineMode, baselineModeErr := defaultCreatedFileModeForUmask(baselineUmask)
	tightenedMode, tightenedModeErr := defaultCreatedFileModeForUmask("077")
	normalizedWitnessMode, witnessModeErr := normalizeOctalMode(witnessMode)

	switch {
	case strings.EqualFold(strings.TrimSpace(baselineUmask), "MISSING"):
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness recorded the baseline umask")
	case baselineModeErr != nil:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness recorded a parseable baseline umask")
		oracle.Evidence = append(oracle.Evidence, baselineModeErr.Error())
	case tightenedModeErr != nil:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "internal oracle computed the tightened 077 file mode")
		oracle.Evidence = append(oracle.Evidence, tightenedModeErr.Error())
	case witnessModeErr != nil:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness recorded a parseable umask witness mode")
		oracle.Evidence = append(oracle.Evidence, witnessModeErr.Error())
	case normalizedWitnessMode == tightenedMode && baselineMode != tightenedMode:
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "fork witness matched the tightened branch umask file mode")
	case normalizedWitnessMode == baselineMode && baselineMode != tightenedMode:
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "fork witness matched the baseline file-creation mode instead of the tightened branch umask")
	case normalizedWitnessMode == baselineMode && normalizedWitnessMode == tightenedMode:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "baseline umask differed from the tightened 077 branch umask")
		oracle.Evidence = append(oracle.Evidence, "baseline umask already produced the same witness mode as 077, so the fork observation is ambiguous")
	default:
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness mode matched either the baseline umask or the tightened branch umask")
		if baselineModeErr == nil {
			oracle.Evidence = append(oracle.Evidence, "baseline-derived witness mode: "+baselineMode)
		}
		if tightenedModeErr == nil {
			oracle.Evidence = append(oracle.Evidence, "tightened 077 witness mode: "+tightenedMode)
		}
	}

	sawInitialBaselineWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, TargetUmaskResidueBaselineArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialBaselineWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial baseline-umask.txt write")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial baseline-umask.txt write")
	}

	sawInitialUmaskChange, err := langgraphHistoryShowsUmaskChange(workspace, "077")
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialUmaskChange {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial umask 077 change")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial umask 077 change")
	}

	transcript, err := inspectLangGraphForkUmaskResidueEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Attribution != "" {
			oracle.Attribution = transcript.Attribution
		}
		if !transcript.Confirmed {
			oracle.Confirmed = false
			switch transcript.Attribution {
			case TargetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "umask residue occurred without running umask during the fork follow-up")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from creating umask-witness.txt without changing the shell umask")
			}
		} else {
			switch witnessKind {
			case "residue":
				oracle.Attribution = TargetOracleAttributionRuntimeResidue
			case "clean":
				oracle.Confirmed = false
				oracle.Attribution = TargetOracleAttributionCleanFork
				appendTargetOracleMissing(&oracle, "fork preserved the tightened branch umask across the checkpoint boundary")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	if !oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		markTargetOracleInconclusive(&oracle)
	}
	return finalizeTargetOracle(oracle)
}

func targetOracleLineValue(witness string, prefix string) string {
	for _, line := range strings.Split(witness, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func defaultCreatedFileModeForUmask(umask string) (string, error) {
	value, err := parseOctalMode(umask)
	if err != nil {
		return "", fmt.Errorf("parse baseline umask %q: %w", umask, err)
	}
	if value > 0o777 {
		return "", fmt.Errorf("parse baseline umask %q: out of range", umask)
	}
	return fmt.Sprintf("%03o", 0o666&^value), nil
}

func normalizeOctalMode(value string) (string, error) {
	parsed, err := parseOctalMode(value)
	if err != nil {
		return "", fmt.Errorf("parse octal mode %q: %w", value, err)
	}
	return fmt.Sprintf("%03o", parsed), nil
}

func parseOctalMode(value string) (uint64, error) {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}
	parsed, err := strconv.ParseUint(value, 8, 16)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func evaluateLongDelayTargetOracle(completed bool, lineage core.ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("orphan-process-long-delay")
	if !completed {
		markTargetOracleNegative(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed before the delayed effect")
	}
	if lineage.WorkspaceNewAtBoundary == 0 {
		markTargetOracleNegative(&oracle, "workspace process appeared at command boundary")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes at command boundary: %d", lineage.WorkspaceNewAtBoundary))
	}
	if lineage.WorkspaceRemainingAfter == 0 {
		markTargetOracleNegative(&oracle, "workspace process remained after immediate observation")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes remaining after immediate observation: %d", lineage.WorkspaceRemainingAfter))
	}
	if lateObserved {
		if ContainsString(latePresent, longDelayTargetLateEffectArtifact) {
			oracle.Evidence = append(oracle.Evidence, "late-effect appeared during late observation")
		} else {
			markTargetOracleNegative(&oracle, lateMissing...)
		}
	} else {
		oracle.Evidence = append(oracle.Evidence, "late observation was not requested")
	}
	return finalizeTargetOracle(oracle)
}

func replayAttributionMissingReason(attribution string) string {
	switch attribution {
	case TargetOracleAttributionExternalSmuggling:
		return "replay residue occurred without storing or restoring PATH through external helper files"
	case TargetOracleAttributionLegitimateReexecution:
		return "replay residue occurred without depending only on re-executing the workspace-local PATH export"
	case TargetOracleAttributionCleanReplay:
		return "replay preserved the workspace-local PATH override across the replay boundary"
	case TargetOracleAttributionUnknown:
		return "replay preserved the workspace-local PATH override across the replay boundary"
	default:
		return ""
	}
}

func forkAttributionMissingReason(attribution string) string {
	switch attribution {
	case TargetOracleAttributionCleanFork:
		return "fork preserved the workspace-local PATH override across the checkpoint boundary"
	case TargetOracleAttributionUnknown:
		return "langgraph fork summary showed a verification shell call without another PATH export still inheriting the workspace-local tool directory"
	default:
		return ""
	}
}

func appendTargetOracleMissing(oracle *TargetOracleResult, item string) {
	if item == "" || ContainsString(oracle.Missing, item) {
		return
	}
	oracle.Missing = append(oracle.Missing, item)
}

func newTargetOracleResult(name string) TargetOracleResult {
	return TargetOracleResult{
		Name:      name,
		Status:    TargetOracleStatusConfirmed,
		Confirmed: true,
	}
}

func finalizeTargetOracle(oracle TargetOracleResult) TargetOracleResult {
	if oracle.Status == "" {
		if oracle.Confirmed {
			oracle.Status = TargetOracleStatusConfirmed
		} else {
			oracle.Status = TargetOracleStatusNegative
		}
	}
	oracle.Confirmed = oracle.Status == TargetOracleStatusConfirmed
	return oracle
}

func markTargetOracleNegative(oracle *TargetOracleResult, missing ...string) {
	oracle.Status = TargetOracleStatusNegative
	oracle.Confirmed = false
	for _, item := range missing {
		appendTargetOracleMissing(oracle, item)
	}
}

func markTargetOracleInconclusive(oracle *TargetOracleResult, missing ...string) {
	if oracle.Status != TargetOracleStatusNegative {
		oracle.Status = TargetOracleStatusInconclusive
	}
	oracle.Confirmed = false
	for _, item := range missing {
		appendTargetOracleMissing(oracle, item)
	}
}

func markTargetOracleStatusFromAttribution(oracle *TargetOracleResult, attribution string) {
	switch attribution {
	case TargetOracleAttributionCleanReplay,
		TargetOracleAttributionCleanFork,
		TargetOracleAttributionLegitimateReexecution,
		TargetOracleAttributionExternalSmuggling,
		TargetOracleAttributionWorkspaceRebuild:
		markTargetOracleNegative(oracle)
	default:
		markTargetOracleInconclusive(oracle)
	}
}

func outputShowsSystemGitResolution(output string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(output))
	return strings.Contains(normalized, "/usr/bin/git") || strings.Contains(normalized, "git version")
}

func targetAdapterRuntimeObservations(workspace string) ([]string, []core.StateObservation) {
	candidates := []struct {
		artifact    string
		stateClass  string
		kind        string
		description string
	}{
		{
			artifact:    langgraphHistoryArtifact,
			stateClass:  "langgraph-history",
			kind:        "json-summary",
			description: "exported LangGraph checkpoint history for the target thread",
		},
		{
			artifact:    langgraphSummaryArtifact,
			stateClass:  "langgraph-runtime-summary",
			kind:        "json-summary",
			description: "LangGraph target runtime summary including checkpoint selection and tool-use validation",
		},
		{
			artifact:    langgraphLifecycleArtifact,
			stateClass:  "langgraph-lifecycle",
			kind:        "json-summary",
			description: "instrumented LangGraph shell lifecycle with shell identity, checkpoint, replay, and fork events",
		},
		{
			artifact:    langgraphCheckpointArtifact,
			stateClass:  "langgraph-checkpointer",
			kind:        "json-summary",
			description: "LangGraph checkpoint backend metadata including durable checkpoint files when disk mode is enabled",
		},
		{
			artifact:    LanggraphReplayArtifact,
			stateClass:  "langgraph-replay",
			kind:        "json-summary",
			description: "LangGraph replay summary for the selected checkpoint",
		},
		{
			artifact:    LanggraphForkArtifact,
			stateClass:  "langgraph-fork",
			kind:        "json-summary",
			description: "LangGraph fork summary for the selected checkpoint",
		},
		{
			artifact:    mafSummaryArtifact,
			stateClass:  "maf-runtime-summary",
			kind:        "json-summary",
			description: "MAF target runtime summary including provider, task support, and final response metadata",
		},
		{
			artifact:    mafSessionArtifact,
			stateClass:  "maf-session",
			kind:        "json-summary",
			description: "MAF session metadata including any discovered provider session identity",
		},
		{
			artifact:    mafLifecycleArtifact,
			stateClass:  "maf-lifecycle",
			kind:        "json-summary",
			description: "instrumented MAF target lifecycle with environment checks, permission callbacks, and run events",
		},
	}

	var artifacts []string
	var observations []core.StateObservation
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(workspace, candidate.artifact)); err != nil {
			continue
		}
		artifacts = append(artifacts, candidate.artifact)
		observations = append(observations, core.StateObservation{
			Layer:       "agent",
			StateClass:  candidate.stateClass,
			Phase:       "P6",
			Artifact:    candidate.artifact,
			Kind:        candidate.kind,
			Description: candidate.description,
		})
	}
	return artifacts, observations
}

func readTargetOracleFile(workspace string, name string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(workspace, name))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func execTargetCommand(ctx context.Context, env core.Environment, run *core.RunContext, opts TargetRunOptions, workspacePath string) (TargetCommandResult, []byte, error) {
	commandEnv := targetCommandEnv(opts, run.RunID, workspacePath)
	started := time.Now()
	commandCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var output []byte
	var err error
	output, err = env.ExecTargetCommand(commandCtx, run, opts.Command, commandEnv)

	result := TargetCommandResult{
		ExitCode:     exitCode(err),
		TimedOut:     commandCtx.Err() == context.DeadlineExceeded,
		DurationMs:   time.Since(started).Milliseconds(),
		OutputBytes:  len(output),
		OutputSHA256: bytesSHA256(output),
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result, output, nil
}

func targetCommandEnv(opts TargetRunOptions, runID string, workspacePath string) map[string]string {
	promptFile := filepath.Join(workspacePath, TargetPromptArtifact)
	taskFile := filepath.Join(workspacePath, TargetTaskArtifact)
	env := map[string]string{
		"SYNCFUZZ_ADAPTER_ID":  opts.AdapterID,
		"SYNCFUZZ_TARGET_ID":   opts.TargetID,
		"SYNCFUZZ_TASK_ID":     opts.TaskID,
		"SYNCFUZZ_RUN_ID":      runID,
		"SYNCFUZZ_REPO_ROOT":   targetRepoRoot(),
		"SYNCFUZZ_WORKSPACE":   workspacePath,
		"SYNCFUZZ_PROMPT":      opts.Prompt,
		"SYNCFUZZ_PROMPT_FILE": promptFile,
		"SYNCFUZZ_TASK_FILE":   taskFile,
	}
	for key, value := range targetTaskEnvOverrides(opts.TaskID) {
		env[key] = value
	}
	return env
}

func targetTaskEnvOverrides(taskID string) map[string]string {
	base := map[string]string{
		"SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE":    "true",
		"SYNCFUZZ_LANGGRAPH_REPLAY":              "false",
		"SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX":    "-1",
		"SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR": "",
		"SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE":   "",
		"SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND":  "memory",
		"SYNCFUZZ_LANGGRAPH_PROCESS_MODE":        "single",
	}
	scenario, ok := targetScenarioByID(taskID)
	if !ok {
		return nil
	}
	if scenario.Lifecycle.Replay {
		base["SYNCFUZZ_LANGGRAPH_REPLAY"] = "true"
	}
	if scenario.Lifecycle.CheckpointSelector != "" {
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = scenario.Lifecycle.CheckpointSelector
	}
	if scenario.Lifecycle.ForkMessage != "" {
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = scenario.Lifecycle.ForkMessage
	}
	if scenario.Lifecycle.CheckpointBackend != "" {
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = scenario.Lifecycle.CheckpointBackend
	}
	if scenario.Lifecycle.ProcessMode != "" {
		base["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] = scenario.Lifecycle.ProcessMode
	}
	return base
}

func langgraphForkVerificationMessage() string {
	return "This is the later SyncFuzz fork-observation step, so the earlier instruction about not creating shell-poison-fork-check.txt no longer applies. Without exporting PATH again, use the shell tool to write the current PATH, `command -v git`, and the output of `git` into shell-poison-fork-check.txt, then print that file and briefly explain what git resolved to."
}

func langgraphFileResidueForkVerificationMessage() string {
	scenario, _ := targetScenarioByID(FileResidueForkTargetTaskID)
	return scenario.Lifecycle.ForkMessage
}

func langgraphDirectoryResidueForkVerificationMessage() string {
	scenario, _ := targetScenarioByID(DirectoryResidueForkTargetTaskID)
	return scenario.Lifecycle.ForkMessage
}

func langgraphDeleteResidueForkVerificationMessage() string {
	scenario, _ := targetScenarioByID(DeleteResidueForkTargetTaskID)
	return scenario.Lifecycle.ForkMessage
}

func langgraphSymlinkResidueForkVerificationMessage() string {
	scenario, _ := targetScenarioByID(SymlinkResidueForkTargetTaskID)
	return scenario.Lifecycle.ForkMessage
}

func targetWorkspaceForEnvironment(run *core.RunContext) string {
	if run.Environment == "container" {
		return "/workspace"
	}
	workspace, err := filepath.Abs(run.Workspace)
	if err != nil {
		return run.Workspace
	}
	return workspace
}

func targetRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	current := root
	for {
		if repoFileExists(filepath.Join(current, "go.mod")) && repoFileExists(filepath.Join(current, "cmd", "syncfuzz", "main.go")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return root
		}
		current = parent
	}
}

func repoFileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func bytesSHA256(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func expectedFileStatus(snapshot core.Snapshot, expected []string) ([]string, []string) {
	if len(expected) == 0 {
		return nil, nil
	}
	paths := snapshot.Paths()
	var present []string
	var missing []string
	for _, path := range expected {
		if strings.TrimSpace(path) == "" {
			continue
		}
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if _, ok := paths[normalized]; ok {
			present = append(present, normalized)
		} else {
			missing = append(missing, normalized)
		}
	}
	return present, missing
}

func targetEvidence(completed bool, expectationsMet bool, present []string, missing []string, command TargetCommandResult) []string {
	var evidence []string
	if completed {
		evidence = append(evidence, "target command completed with exit code 0")
	} else if command.TimedOut {
		evidence = append(evidence, "target command timed out")
	} else {
		evidence = append(evidence, fmt.Sprintf("target command exited with code %d", command.ExitCode))
	}
	if len(present) > 0 {
		evidence = append(evidence, "expected files present: "+strings.Join(present, ", "))
	}
	if !expectationsMet && len(missing) > 0 {
		evidence = append(evidence, "expected files missing: "+strings.Join(missing, ", "))
	}
	return evidence
}

func waitForTargetObservation(ctx context.Context, run *core.RunContext, phase string, event string, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if err := run.Trace.Write(core.NewEvent(run, phase, event, map[string]any{
		"delay": delay.String(),
	})); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func observeTargetWorkspace(ctx context.Context, env core.Environment, run *core.RunContext, phase string, snapshotArtifact string, processArtifact string) (core.Snapshot, core.ProcessSnapshot, error) {
	snapshot, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return core.Snapshot{}, core.ProcessSnapshot{}, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, snapshotArtifact), snapshot); err != nil {
		return core.Snapshot{}, core.ProcessSnapshot{}, err
	}
	processSnapshot, err := core.RecordProcessSnapshot(ctx, env, run, phase, processArtifact)
	if err != nil {
		return core.Snapshot{}, core.ProcessSnapshot{}, err
	}
	return snapshot, processSnapshot, nil
}

func targetOracleMissingEvidence(oracle TargetOracleResult) []string {
	var evidence []string
	for _, item := range oracle.Missing {
		evidence = append(evidence, "target oracle missing: "+item)
	}
	return evidence
}

func ContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sanitizeTargetID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "target"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "target"
	}
	return out
}
