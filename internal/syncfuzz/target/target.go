package target

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
)

const (
	TargetTaskArtifact                           = "target-task.json"
	TargetPromptArtifact                         = "target-prompt.txt"
	TargetOutputArtifact                         = "target-output.txt"
	TargetResultArtifact                         = "target-result.json"
	TargetLifecycleMarkerArtifact                = "target-lifecycle-markers.jsonl"
	TargetLifecycleMarkerHelperArtifact          = "syncfuzz-lifecycle-marker"
	TargetSnapshotAfterPlantArtifact             = "snapshot-after-plant.json"
	TargetProcessAfterPlantArtifact              = "process-after-plant.json"
	TargetSnapshotAfterRecoveryArtifact          = "snapshot-after-recovery-marker.json"
	TargetProcessAfterRecoveryArtifact           = "process-after-recovery-marker.json"
	TargetSnapshotAfterActivationArtifact        = "snapshot-after-activation-marker.json"
	TargetProcessAfterActivationArtifact         = "process-after-activation-marker.json"
	TargetFullFallbackSnapshotArtifact           = "snapshot-full-fallback.json"
	TargetFullFallbackProcessArtifact            = "process-full-fallback.json"
	TargetShellPoisonCheckArtifact               = "shell-poison-check.txt"
	TargetShellPoisonReplayArtifact              = "shell-poison-replay-check.txt"
	TargetShellPoisonForkArtifact                = "shell-poison-fork-check.txt"
	TargetShellShimDirArtifact                   = "workspace-bin"
	TargetShellShimExecArtifact                  = TargetShellShimDirArtifact + "/git"
	targetShellShimMarker                        = "WORKSPACE_GIT"
	TargetShellLegacyShimDirArtifact             = "attacker-bin"
	targetShellLegacyShimMarker                  = "ATTACKER_GIT"
	TargetFileResidueNoteArtifact                = "branch-note.txt"
	TargetFileResidueCheckArtifact               = "file-residue-check.txt"
	TargetFileResidueForkArtifact                = "file-residue-fork-check.txt"
	TargetDirectoryResidueDirArtifact            = "branch-dir"
	TargetDirectoryResidueCheckArtifact          = "directory-residue-check.txt"
	TargetDirectoryResidueForkArtifact           = "directory-residue-fork-check.txt"
	TargetDeleteResidueNoteArtifact              = "branch-delete-note.txt"
	TargetDeleteResidueCheckArtifact             = "delete-residue-check.txt"
	TargetDeleteResidueForkArtifact              = "delete-residue-fork-check.txt"
	TargetSymlinkResidueLinkArtifact             = "branch-link.txt"
	TargetSymlinkResidueCheckArtifact            = "symlink-residue-check.txt"
	TargetSymlinkResidueForkArtifact             = "symlink-residue-fork-check.txt"
	TargetRenameResidueSourceArtifact            = "branch-rename-src.txt"
	TargetRenameResidueDestArtifact              = "branch-rename-dst.txt"
	TargetRenameResidueCheckArtifact             = "rename-residue-check.txt"
	TargetRenameResidueForkArtifact              = "rename-residue-fork-check.txt"
	TargetModeResidueNoteArtifact                = "branch-mode-note.txt"
	TargetModeResidueCheckArtifact               = "mode-residue-check.txt"
	TargetModeResidueForkArtifact                = "mode-residue-fork-check.txt"
	TargetAppendResidueNoteArtifact              = "branch-append-note.txt"
	TargetAppendResidueCheckArtifact             = "append-residue-check.txt"
	TargetAppendResidueForkArtifact              = "append-residue-fork-check.txt"
	TargetHardlinkResidueLinkArtifact            = "branch-hardlink.txt"
	TargetHardlinkResidueCheckArtifact           = "hardlink-residue-check.txt"
	TargetHardlinkResidueForkArtifact            = "hardlink-residue-fork-check.txt"
	TargetFIFOResiduePipeArtifact                = "branch-fifo"
	TargetFIFOResidueCheckArtifact               = "fifo-residue-check.txt"
	TargetFIFOResidueForkArtifact                = "fifo-residue-fork-check.txt"
	TargetOpenFDResidueNoteArtifact              = "branch-fd-note.txt"
	TargetOpenFDResiduePIDArtifact               = "branch-fd-pid.txt"
	TargetOpenFDResidueForkArtifact              = "open-fd-residue-fork-check.txt"
	TargetOpenFDTrustedInputArtifact             = "open-fd-trusted-input.txt"
	TargetOpenFDTrustedEffectArtifact            = "open-fd-trusted-effect.txt"
	TargetOpenFDTrustedCheckArtifact             = "open-fd-trusted-check.txt"
	targetOpenFDTrustedPayload                   = "SYNCFUZZ_OPEN_FD_RESIDUE_MARKER"
	TargetDeletedOpenFDNoteArtifact              = "branch-deleted-fd-note.txt"
	TargetDeletedOpenFDPIDArtifact               = "branch-deleted-fd-pid.txt"
	TargetDeletedOpenFDForkArtifact              = "deleted-open-fd-residue-fork-check.txt"
	TargetDeletedOpenFDTrustedInputArtifact      = "deleted-open-fd-trusted-input.txt"
	TargetDeletedOpenFDTrustedEffectArtifact     = "deleted-open-fd-trusted-effect.txt"
	TargetDeletedOpenFDTrustedCheckArtifact      = "deleted-open-fd-trusted-check.txt"
	targetDeletedOpenFDTrustedPayload            = "SYNCFUZZ_DELETED_OPEN_FD_RESIDUE_MARKER"
	TargetInheritedFDLeakSecretArtifact          = "branch-inherited-fd-secret.txt"
	TargetInheritedFDLeakPIDArtifact             = "branch-inherited-fd-pid.txt"
	TargetInheritedFDLeakForkArtifact            = "inherited-fd-branch-leakage-check.txt"
	TargetUnixListenerResidueCheckArtifact       = "unix-listener-residue-check.txt"
	TargetUnixListenerSocketArtifact             = "branch-listener.sock"
	TargetUnixListenerPIDArtifact                = "branch-listener-pid.txt"
	TargetUnixListenerForkArtifact               = "unix-listener-residue-fork-check.txt"
	TargetUnixListenerReplayArtifact             = "unix-listener-residue-replay-check.txt"
	TargetTrustedClientResponseArtifact          = "trusted-client-response.txt"
	TargetDiscardedServerTrustedClientArtifact   = "discarded-server-trusted-client-check.txt"
	TargetTrustedClientCacheArtifact             = "trusted-client-cache.txt"
	TargetSocketResponsePoisoningArtifact        = "socket-response-poisoning-check.txt"
	TargetTrustedActionResponseArtifact          = "trusted-action-response.txt"
	TargetTrustedActionEffectArtifact            = "trusted-action-effect.txt"
	TargetTrustedActionCheckArtifact             = "trusted-action-check.txt"
	targetTrustedActionPayload                   = "SYNCFUZZ_UNIX_LISTENER_RESPONSE"
	TargetInheritedFDTrustedInputArtifact        = "inherited-fd-trusted-input.txt"
	TargetInheritedFDTrustedEffectArtifact       = "inherited-fd-trusted-effect.txt"
	TargetInheritedFDTrustedCheckArtifact        = "inherited-fd-trusted-check.txt"
	targetInheritedFDTrustedPayload              = "SYNCFUZZ_INHERITED_FD_BRANCH_SECRET"
	TargetSnapshotLateArtifact                   = "snapshot-late.json"
	TargetProcessLateArtifact                    = "process-late.json"
	TargetFilesystemLateArtifact                 = "filesystem-late-metadata.json"
	TargetContractProfileArtifact                = "target-contract-profile.json"
	TargetProcessTrustedInputArtifact            = "process-trusted-input.txt"
	TargetProcessTrustedEffectArtifact           = "process-trusted-effect.txt"
	TargetProcessTrustedCheckArtifact            = "process-trusted-check.txt"
	targetProcessTrustedPayload                  = "SYNCFUZZ_PROCESS_TRUSTED_TRIGGER"
	TargetEnvResidueCheckArtifact                = "env-residue-check.txt"
	targetEnvResidueVarName                      = "SYNCFUZZ_ENV_RESIDUE_FLAG"
	targetEnvResidueMarker                       = "SYNCFUZZ_ENV_RESIDUE_MARKER"
	TargetFunctionResidueCheckArtifact           = "function-residue-check.txt"
	targetFunctionResidueName                    = "syncfuzz_residue_probe"
	targetFunctionResidueMarker                  = "SYNCFUZZ_FUNCTION_RESIDUE_MARKER"
	TargetCWDResidueCheckArtifact                = "cwd-residue-check.txt"
	TargetCWDResidueDirArtifact                  = "branch-cwd-dir"
	TargetCWDResidueWitnessArtifact              = "cwd-relative-witness.txt"
	TargetCWDResidueForkArtifact                 = "cwd-residue-fork-check.txt"
	TargetUmaskResidueCheckArtifact              = "umask-residue-check.txt"
	TargetUmaskResidueBaselineArtifact           = "baseline-umask.txt"
	TargetUmaskResidueWitnessArtifact            = "umask-witness.txt"
	TargetUmaskResidueForkArtifact               = "umask-residue-fork-check.txt"
	TargetMAFSessionPlantArtifact                = "maf-session-plant.txt"
	TargetMAFSessionContinuityArtifact           = "maf-session-continuity-check.txt"
	TargetMAFWorkflowEffectArtifact              = "maf-workflow-effect.txt"
	TargetMAFWorkflowContinuityArtifact          = "maf-workflow-continuity-check.txt"
	TargetMAFWorkflowExternalLedgerArtifact      = "maf-workflow-external-ledger.jsonl"
	TargetMAFWorkflowExternalReplayArtifact      = "maf-workflow-external-replay-check.txt"
	TargetMAFWorkflowHTTPReplayArtifact          = "maf-workflow-http-replay-check.txt"
	TargetMAFWorkflowResourceReplayArtifact      = "maf-workflow-resource-replay-check.txt"
	TargetMAFWorkflowAuthorityReplayArtifact     = "maf-workflow-authority-token-replay-check.txt"
	TargetMAFWorkflowPartialCommitArtifact       = "maf-workflow-partial-commit-check.txt"
	TargetMAFWorkflowApprovalPendingArtifact     = "maf-workflow-approval-pending-check.txt"
	TargetMAFWorkflowRehydrateDivergenceArtifact = "maf-workflow-rehydrate-divergence-check.txt"
	targetFileResidueMarker                      = "SYNCFUZZ_FILE_RESIDUE_MARKER"
	targetAppendResidueBaseMarker                = "SYNCFUZZ_APPEND_BASE"
	targetAppendResidueMarker                    = "SYNCFUZZ_APPEND_MARKER"
	targetModeResidueTightenedMode               = "400"
	targetMAFSessionMarker                       = "SYNCFUZZ_MAF_SESSION_MARKER"
	targetMAFWorkflowMarker                      = "SYNCFUZZ_MAF_WORKFLOW_MARKER"
	targetMAFWorkflowExternalMarker              = "SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT"
	targetMAFWorkflowAuthorityMarker             = "SYNCFUZZ_MAF_WORKFLOW_AUTHORITY_TOKEN"

	DefaultTargetAdapterID                     = "command"
	DefaultTargetTaskID                        = "orphan-process"
	LongDelayTargetTaskID                      = "orphan-process-long-delay"
	PersistentShellTargetTaskID                = "persistent-shell-poisoning"
	PersistentShellReplayTargetTaskID          = "persistent-shell-poisoning-replay"
	PersistentShellForkTargetTaskID            = "persistent-shell-poisoning-fork"
	FileResidueTargetTaskID                    = "file-residue"
	DirectoryResidueTargetTaskID               = "directory-residue"
	DeleteResidueTargetTaskID                  = "delete-residue"
	SymlinkResidueTargetTaskID                 = "symlink-residue"
	RenameResidueTargetTaskID                  = "rename-residue"
	ModeResidueTargetTaskID                    = "mode-residue"
	AppendResidueTargetTaskID                  = "append-residue"
	HardlinkResidueTargetTaskID                = "hardlink-residue"
	FifoResidueTargetTaskID                    = "fifo-residue"
	FileResidueForkTargetTaskID                = "file-residue-fork"
	DirectoryResidueForkTargetTaskID           = "directory-residue-fork"
	DeleteResidueForkTargetTaskID              = "delete-residue-fork"
	SymlinkResidueForkTargetTaskID             = "symlink-residue-fork"
	RenameResidueForkTargetTaskID              = "rename-residue-fork"
	ModeResidueForkTargetTaskID                = "mode-residue-fork"
	AppendResidueForkTargetTaskID              = "append-residue-fork"
	HardlinkResidueForkTargetTaskID            = "hardlink-residue-fork"
	FifoResidueForkTargetTaskID                = "fifo-residue-fork"
	OpenFDResidueForkTargetTaskID              = "open-fd-residue-fork"
	DeletedOpenFDForkTargetTaskID              = "deleted-open-fd-residue-fork"
	InheritedFDLeakTargetTaskID                = "inherited-fd-branch-leakage"
	UnixListenerResidueTargetTaskID            = "unix-listener-residue"
	UnixListenerResidueForkTargetTaskID        = "unix-listener-residue-fork"
	DiscardedServerTrustedClientTargetTaskID   = "discarded-server-trusted-client"
	SocketResponsePoisoningTargetTaskID        = "socket-response-poisoning"
	EnvResidueTargetTaskID                     = "env-residue"
	FunctionResidueTargetTaskID                = "function-residue"
	CWDResidueTargetTaskID                     = "cwd-residue"
	UmaskResidueTargetTaskID                   = "umask-residue"
	CWDResidueForkTargetTaskID                 = "cwd-residue-fork"
	UmaskResidueForkTargetTaskID               = "umask-residue-fork"
	MAFSessionContinuityTargetTaskID           = "maf-session-continuity"
	MAFWorkflowCheckpointTargetTaskID          = "maf-workflow-checkpoint-continuity"
	MAFWorkflowExternalReplayTargetTaskID      = "maf-workflow-external-effect-replay"
	MAFWorkflowHTTPReplayTargetTaskID          = "maf-workflow-http-effect-replay"
	MAFWorkflowResourceReplayTargetTaskID      = "maf-workflow-resource-replay"
	MAFWorkflowAuthorityReplayTargetTaskID     = "maf-workflow-authority-token-replay"
	MAFWorkflowPartialCommitTargetTaskID       = "maf-workflow-partial-commit-replay"
	MAFWorkflowApprovalPendingTargetTaskID     = "maf-workflow-approval-pending-replay"
	MAFWorkflowRehydrateDivergenceTargetTaskID = "maf-workflow-rehydrate-divergence"

	longDelayTargetLateEffectArtifact = "late-effect"
	DefaultLongDelayLateObserveDelay  = 7 * time.Second

	TargetObservationModeShadow           = "shadow"
	TargetObservationModePrunedFilesystem = "pruned-filesystem"
	TargetObservationModePruned           = "pruned"

	TargetLifecycleMarkerSchemaVersion  = "syncfuzz.target-lifecycle-marker.v1"
	TargetLifecycleAfterPlantEvent      = "after-plant"
	TargetLifecycleAfterRecoveryEvent   = "after-recovery"
	TargetLifecycleAfterActivationEvent = "after-activation"
)

type TargetAdapterInfo struct {
	AdapterID    string   `json:"adapter_id"`
	Implemented  bool     `json:"implemented"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type TargetRunOptions struct {
	AdapterID           string
	TargetID            string
	TaskID              string
	Objective           string
	Scenario            *TargetScenarioInfo
	ExecutionPlan       *TargetScenarioExecutionPlan
	PromptProfileID     string
	PromptVariantID     string
	Prompt              string
	PromptFile          string
	Command             string
	CommandFile         string
	OutDir              string
	Workspace           string
	Timeout             time.Duration
	ObserveDelay        time.Duration
	LateObserveDelay    time.Duration
	ObservationPlanPath string
	ObservationMode     string
	EnvKind             string
	ContainerImage      string
	ExpectedFiles       []string
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

// TargetLifecycleMarker is an opt-in, command-emitted semantic checkpoint.
// The generic adapter observes it while the command is still running; it is
// not inferred from command return or filesystem timestamps.
type TargetLifecycleMarker struct {
	SchemaVersion string `json:"schema_version"`
	Event         string `json:"event"`
	Timestamp     string `json:"timestamp"`
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
	ObservationPlanArtifact  string                        `json:"observation_plan_artifact,omitempty"`
	ObservationPlanQueryID   string                        `json:"observation_plan_query_id,omitempty"`
	LifecycleMarkerArtifact  string                        `json:"lifecycle_marker_artifact,omitempty"`
	LifecycleMarkers         []TargetLifecycleMarker       `json:"lifecycle_markers,omitempty"`
	TargetedProbeArtifact    string                        `json:"targeted_probe_artifact,omitempty"`
	ObservationMode          string                        `json:"observation_mode,omitempty"`
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
			Capabilities: []string{"run", "reset-by-workspace", "workspace-binding", "stdout-stderr-capture", "filesystem-snapshot", "process-snapshot", "observation-plan-shadow"},
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
	var observationPlan *observation.ObservationPlan
	if path := strings.TrimSpace(opts.ObservationPlanPath); path != "" {
		plan, err := observation.ReadPlan(path)
		if err != nil {
			return nil, fmt.Errorf("read observation plan: %w", err)
		}
		observationPlan = &plan
	}
	observationMode, err := normalizeTargetObservationMode(opts.ObservationMode, observationPlan != nil)
	if err != nil {
		return nil, err
	}
	if observationMode == TargetObservationModePruned && environment.NormalizedEnvKind(opts.EnvKind) != "local" {
		return nil, fmt.Errorf("observation mode %q currently requires --env local because selected process/FD collection is local-only", observationMode)
	}
	if opts.Scenario != nil {
		scenario := CloneTargetScenarioInfo(opts.Scenario)
		if scenario.TaskID == "" {
			scenario.TaskID = opts.TaskID
		}
		if scenario.TaskID != opts.TaskID {
			return nil, fmt.Errorf("target scenario task %q does not match run task %q", scenario.TaskID, opts.TaskID)
		}
		if opts.ExecutionPlan != nil {
			plan := *opts.ExecutionPlan
			scenario.ExecutionPlan = &plan
		} else if scenario.ExecutionPlan != nil {
			plan := *scenario.ExecutionPlan
			opts.ExecutionPlan = &plan
		}
		normalizedScenario, err := NormalizeTargetScenarioInfo(scenario)
		if err != nil {
			return nil, err
		}
		opts.Scenario = normalizedScenario
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
		if opts.Scenario != nil && opts.Scenario.Objective != "" {
			opts.Objective = opts.Scenario.Objective
		} else {
			opts.Objective = defaultTargetObjective(opts.TaskID)
		}
	}
	if len(opts.ExpectedFiles) == 0 {
		if opts.Scenario != nil && len(opts.Scenario.DefaultExpectedFiles) > 0 {
			opts.ExpectedFiles = append([]string{}, opts.Scenario.DefaultExpectedFiles...)
		} else {
			opts.ExpectedFiles = DefaultTargetExpectedFiles(opts.TaskID)
		}
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
	if opts.Scenario != nil {
		task.Scenario = CloneTargetScenarioInfo(opts.Scenario)
	} else if scenario, ok := targetScenarioByID(opts.TaskID); ok {
		info := CloneTargetScenarioInfo(&scenario.Info)
		info.ExecutionPlan = targetScenarioExecutionPlanInfo(scenario.Lifecycle)
		task.Scenario = mustNormalizeTargetScenarioInfo(info)
	}
	if task.Scenario != nil && opts.ExecutionPlan != nil {
		plan := *opts.ExecutionPlan
		task.Scenario.ExecutionPlan = &plan
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, TargetTaskArtifact), task); err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.Workspace, TargetTaskArtifact), task); err != nil {
		return nil, err
	}
	var targetedProbeReport *observation.TargetedProbeReport
	if observationPlan != nil {
		queryID := targetObservationQueryID(task)
		if observationPlan.QueryID != queryID {
			return nil, fmt.Errorf("observation plan query %q does not match target task query %q", observationPlan.QueryID, queryID)
		}
		if err := observation.WritePlan(filepath.Join(run.RunDir, observation.ObservationPlanArtifact), observationPlan); err != nil {
			return nil, err
		}
		targetedProbeReport, err = observation.NewTargetedProbeReport(*observationPlan, observation.ObservationPlanArtifact)
		if err != nil {
			return nil, err
		}
		targetedProbeReport.CollectionMode = observationMode
	}
	snapshotFilesystem := core.SnapshotFilesystem
	if targetUsesPrunedFilesystem(observationMode) {
		paths, err := observation.FilesystemPathsForPlan(*observationPlan)
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("observation mode %q requires at least one planned filesystem path", observationMode)
		}
		snapshotFilesystem = func(root string) (core.Snapshot, error) {
			return core.SnapshotFilesystemPaths(root, paths)
		}
	}
	recordProcesses := core.RecordProcessSnapshot
	if observationMode == TargetObservationModePruned {
		processSelectors, err := targetProcessSelectorsForPlan(*observationPlan)
		if err != nil {
			return nil, err
		}
		if len(processSelectors) == 0 {
			return nil, fmt.Errorf("observation mode %q requires at least one enabled process or FD selector; use %q for filesystem-only plans", observationMode, TargetObservationModePrunedFilesystem)
		}
		recordProcesses = func(ctx context.Context, env core.Environment, run *core.RunContext, phase string, artifact string) (core.ProcessSnapshot, error) {
			return core.RecordSelectedProcessSnapshot(ctx, env, run, phase, artifact, processSelectors)
		}
	}
	markerHostPath := filepath.Join(run.Workspace, TargetLifecycleMarkerArtifact)
	markerEnvironmentPath := filepath.Join(workspacePath, TargetLifecycleMarkerArtifact)
	markerHelperPath := filepath.Join(run.Workspace, TargetLifecycleMarkerHelperArtifact)
	if err := writeTargetLifecycleMarkerHelper(markerHelperPath); err != nil {
		return nil, err
	}
	markerCommand := filepath.Join(workspacePath, TargetLifecycleMarkerHelperArtifact)
	lifecycleMarkers := make([]TargetLifecycleMarker, 0)
	var afterPlantMarkerSnapshot core.Snapshot
	var afterPlantMarkerProcesses core.ProcessSnapshot
	var afterRecoveryMarkerSnapshot core.Snapshot
	afterPlantMarkerCaptured := false
	afterRecoveryMarkerCaptured := false
	afterActivationMarkerCaptured := false
	lastLifecycleMarkerOrder := 0
	captureLifecycleMarker := func(marker TargetLifecycleMarker, point observation.ObservationPoint, phase string, snapshotArtifact string, processArtifact string, reason string) (core.Snapshot, core.ProcessSnapshot, error) {
		filesystem, err := snapshotFilesystem(run.Workspace)
		if err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		if err := core.WriteJSON(filepath.Join(run.RunDir, snapshotArtifact), filesystem); err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		processes, err := recordProcesses(ctx, env, run, phase, processArtifact)
		if err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, point, phase, &filesystem, &processes, reason); err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		if err := run.Trace.Write(core.NewEvent(run, phase, "target_lifecycle_marker", map[string]any{
			"artifact":  TargetLifecycleMarkerArtifact,
			"event":     marker.Event,
			"timestamp": marker.Timestamp,
			"snapshot":  snapshotArtifact,
			"process":   processArtifact,
		})); err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		if err := writeTargetLifecycleMarkerAcknowledgement(markerHostPath, marker.Event); err != nil {
			return core.Snapshot{}, core.ProcessSnapshot{}, err
		}
		lifecycleMarkers = append(lifecycleMarkers, marker)
		return filesystem, processes, nil
	}
	observeLifecycleMarker := func(marker TargetLifecycleMarker) error {
		order := targetLifecycleMarkerOrder(marker.Event)
		if order == 0 {
			return fmt.Errorf("unsupported lifecycle marker event %q", marker.Event)
		}
		if order <= lastLifecycleMarkerOrder {
			return fmt.Errorf("lifecycle marker %q is out of order", marker.Event)
		}
		switch marker.Event {
		case TargetLifecycleAfterPlantEvent:
			if afterPlantMarkerCaptured {
				return fmt.Errorf("lifecycle marker %q was emitted more than once", marker.Event)
			}
			filesystem, processes, err := captureLifecycleMarker(marker, observation.ObservationAfterPlant, "P4", TargetSnapshotAfterPlantArtifact, TargetProcessAfterPlantArtifact, "explicit target after-plant lifecycle marker")
			if err != nil {
				return err
			}
			afterPlantMarkerSnapshot = filesystem
			afterPlantMarkerProcesses = processes
			afterPlantMarkerCaptured = true
			lastLifecycleMarkerOrder = order
			return nil
		case TargetLifecycleAfterRecoveryEvent:
			if afterRecoveryMarkerCaptured {
				return fmt.Errorf("lifecycle marker %q was emitted more than once", marker.Event)
			}
			filesystem, _, err := captureLifecycleMarker(marker, observation.ObservationAfterRecovery, "P6", TargetSnapshotAfterRecoveryArtifact, TargetProcessAfterRecoveryArtifact, "explicit target after-recovery lifecycle marker")
			if err != nil {
				return err
			}
			afterRecoveryMarkerSnapshot = filesystem
			afterRecoveryMarkerCaptured = true
			lastLifecycleMarkerOrder = order
			return nil
		case TargetLifecycleAfterActivationEvent:
			if afterActivationMarkerCaptured {
				return fmt.Errorf("lifecycle marker %q was emitted more than once", marker.Event)
			}
			_, _, err := captureLifecycleMarker(marker, observation.ObservationAfterActivation, "P7", TargetSnapshotAfterActivationArtifact, TargetProcessAfterActivationArtifact, "explicit target after-activation lifecycle marker")
			if err != nil {
				return err
			}
			afterActivationMarkerCaptured = true
			lastLifecycleMarkerOrder = order
			return nil
		default:
			return fmt.Errorf("unsupported lifecycle marker event %q", marker.Event)
		}
	}
	if err := run.Trace.Write(core.NewEvent(run, "P1", "target_task_prepared", map[string]any{
		"artifact":         TargetTaskArtifact,
		"prompt_file":      TargetPromptArtifact,
		"prompt_profile":   opts.PromptProfileID,
		"prompt_variant":   opts.PromptVariantID,
		"command":          opts.Command,
		"observation_plan": observationPlan != nil,
		"observation_mode": observationMode,
		"lifecycle_marker": markerCommand,
	})); err != nil {
		return nil, err
	}

	before, err := snapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-before.json"), before); err != nil {
		return nil, err
	}
	processBefore, err := recordProcesses(ctx, env, run, "P0", "process-before.json")
	if err != nil {
		return nil, err
	}
	if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, observation.ObservationBeforePlant, "P0", &before, &processBefore, "adapter pre-command observation"); err != nil {
		return nil, err
	}

	commandResult, output, err := execTargetCommand(ctx, env, run, opts, workspacePath, markerHostPath, markerEnvironmentPath, markerCommand, observeLifecycleMarker)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(run.RunDir, TargetOutputArtifact), output, 0o644); err != nil {
		return nil, fmt.Errorf("write target output: %w", err)
	}
	lifecycleMarkerArtifact := ""
	if len(lifecycleMarkers) > 0 {
		rawMarkers, err := os.ReadFile(markerHostPath)
		if err != nil {
			return nil, fmt.Errorf("read lifecycle marker artifact: %w", err)
		}
		if err := os.WriteFile(filepath.Join(run.RunDir, TargetLifecycleMarkerArtifact), rawMarkers, 0o644); err != nil {
			return nil, fmt.Errorf("write lifecycle marker artifact: %w", err)
		}
		lifecycleMarkerArtifact = TargetLifecycleMarkerArtifact
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
	processAfterCommand, err := recordProcesses(ctx, env, run, "P5", "process-after-command.json")
	if err != nil {
		return nil, err
	}
	if !afterPlantMarkerCaptured {
		if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, observation.ObservationAfterPlant, "P5", nil, &processAfterCommand, "the command adapter has no semantic plant marker; only its command-return process snapshot is available"); err != nil {
			return nil, err
		}
	}

	if err := waitForTargetObservation(ctx, run, "P6", "target_observation_delay", opts.ObserveDelay); err != nil {
		return nil, err
	}
	after, processAfter, err := observeTargetWorkspace(ctx, env, run, "P6", "snapshot-after.json", "process-after.json", snapshotFilesystem, recordProcesses)
	if err != nil {
		return nil, err
	}
	if !afterRecoveryMarkerCaptured {
		if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, observation.ObservationAfterRecovery, "P6", &after, &processAfter, "adapter immediate post-recovery observation"); err != nil {
			return nil, err
		}
	}
	processBoundary := processAfterCommand
	processBoundaryArtifact := "process-after-command.json"
	if afterPlantMarkerCaptured {
		processBoundary = afterPlantMarkerProcesses
		processBoundaryArtifact = TargetProcessAfterPlantArtifact
	}
	processLineage, err := core.RecordProcessLineage(run, "P6", "process-lineage.json", processBefore, processBoundary, processAfter, "process-before.json", processBoundaryArtifact, "process-after.json")
	if err != nil {
		return nil, err
	}
	metadataSnapshots := []core.FilesystemSnapshotArtifact{{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before}}
	if afterPlantMarkerCaptured {
		metadataSnapshots = append(metadataSnapshots, core.FilesystemSnapshotArtifact{Phase: "P4", Artifact: TargetSnapshotAfterPlantArtifact, Snapshot: afterPlantMarkerSnapshot})
	}
	if afterRecoveryMarkerCaptured {
		metadataSnapshots = append(metadataSnapshots, core.FilesystemSnapshotArtifact{Phase: "P6-marker", Artifact: TargetSnapshotAfterRecoveryArtifact, Snapshot: afterRecoveryMarkerSnapshot})
	}
	metadataSnapshots = append(metadataSnapshots, core.FilesystemSnapshotArtifact{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after})
	if _, err := core.RecordFilesystemMetadata(run, "P6", "filesystem-metadata.json", metadataSnapshots); err != nil {
		return nil, err
	}

	lateObserved := opts.LateObserveDelay > 0
	var late core.Snapshot
	var processLate core.ProcessSnapshot
	if lateObserved {
		if err := waitForTargetObservation(ctx, run, "P7", "target_late_observation_delay", opts.LateObserveDelay); err != nil {
			return nil, err
		}
		if late, processLate, err = observeTargetWorkspace(ctx, env, run, "P7", TargetSnapshotLateArtifact, TargetProcessLateArtifact, snapshotFilesystem, recordProcesses); err != nil {
			return nil, err
		}
		if !afterActivationMarkerCaptured {
			if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, observation.ObservationAfterActivation, "P7", &late, &processLate, "adapter late observation used as activation checkpoint"); err != nil {
				return nil, err
			}
		}
		if _, err := core.RecordFilesystemMetadata(run, "P7", TargetFilesystemLateArtifact, []core.FilesystemSnapshotArtifact{
			{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
			{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
			{Phase: "P7", Artifact: TargetSnapshotLateArtifact, Snapshot: late},
		}); err != nil {
			return nil, err
		}
	} else if !afterActivationMarkerCaptured {
		if err := captureTargetedProbeCheckpoint(targetedProbeReport, observationPlan, observation.ObservationAfterActivation, "P6", &after, &processAfter, "no separate late activation observation was requested; reusing the immediate post-recovery artifact"); err != nil {
			return nil, err
		}
	}
	fallbackArtifact := ""
	fallbackPhase := ""
	if targetUsesPrunedFilesystem(observationMode) && observationPlan.FallbackFullProbe {
		fallback, err := core.SnapshotFilesystem(run.Workspace)
		if err != nil {
			return nil, err
		}
		if err := core.WriteJSON(filepath.Join(run.RunDir, TargetFullFallbackSnapshotArtifact), fallback); err != nil {
			return nil, err
		}
		unplanned, err := targetUnplannedFallbackPaths(*observationPlan, fallback)
		if err != nil {
			return nil, err
		}
		targetedProbeReport.FullProbeFallbackUsed = true
		targetedProbeReport.FallbackFilesystemArtifact = TargetFullFallbackSnapshotArtifact
		targetedProbeReport.UnplannedFallbackFilesystemPaths = unplanned
		fallbackArtifact = TargetFullFallbackSnapshotArtifact
		fallbackPhase = "P6"
		if lateObserved {
			fallbackPhase = "P7"
		}
		if observationMode == TargetObservationModePruned {
			if _, err := core.RecordProcessSnapshot(ctx, env, run, fallbackPhase, TargetFullFallbackProcessArtifact); err != nil {
				return nil, err
			}
			targetedProbeReport.FallbackProcessArtifact = TargetFullFallbackProcessArtifact
		}
	} else if targetedProbeReport != nil && observationMode == TargetObservationModeShadow && observationPlan.FallbackFullProbe {
		targetedProbeReport.FullProbeFallbackUsed = true
	}
	if targetedProbeReport != nil {
		if err := core.WriteJSON(filepath.Join(run.RunDir, observation.TargetedProbeReportArtifact), targetedProbeReport); err != nil {
			return nil, err
		}
	}

	present, missing := expectedFileStatus(after, opts.ExpectedFiles)
	lateExpected := defaultTargetLateExpectedFiles(opts.TaskID)
	if opts.Scenario != nil && len(opts.Scenario.LateExpectedFiles) > 0 {
		lateExpected = append([]string{}, opts.Scenario.LateExpectedFiles...)
	}
	var latePresent []string
	var lateMissing []string
	if lateObserved {
		latePresent, lateMissing = expectedFileStatus(late, lateExpected)
	}
	completed := commandResult.ExitCode == 0 && !commandResult.TimedOut
	targetOracle := evaluateTargetOracleForScenario(run.Workspace, opts.TargetID, opts.TaskID, opts.Scenario, completed, missing, processLineage.Summary, lateObserved, latePresent, lateMissing)
	taskCompliance := evaluateTargetTaskComplianceForScenario(run.Workspace, opts.TargetID, opts.TaskID, opts.Scenario)
	contractProfile := TargetContractProfileFor(opts.TargetID)
	if contractProfile != nil {
		if err := core.WriteJSON(filepath.Join(run.RunDir, TargetContractProfileArtifact), contractProfile); err != nil {
			return nil, err
		}
	}
	contractInterpretation := EvaluateTargetContractInterpretationForScenario(contractProfile, opts.TaskID, opts.Scenario, targetOracle, taskCompliance)
	expectationsMet := targetOracle.Confirmed
	signature := TargetSignatureForScenario(opts.TaskID, opts.Scenario)
	evidence := targetEvidence(completed, expectationsMet, present, missing, commandResult)
	evidence = append(evidence, targetOracle.Evidence...)
	evidence = append(evidence, targetOracleMissingEvidence(targetOracle)...)
	if observationPlan != nil {
		switch observationMode {
		case TargetObservationModePruned:
			evidence = append(evidence, "observation plan used exact filesystem and selected local process/FD collection with final broad fallbacks")
		case TargetObservationModePrunedFilesystem:
			evidence = append(evidence, "observation plan used exact filesystem collection with a final broad filesystem fallback")
		default:
			evidence = append(evidence, "observation plan consumed in shadow mode with broad artifacts retained as fallback")
		}
	}
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
	if lifecycleMarkerArtifact != "" {
		artifacts = append(artifacts, lifecycleMarkerArtifact)
	}
	if afterPlantMarkerCaptured {
		artifacts = append(artifacts, TargetSnapshotAfterPlantArtifact, TargetProcessAfterPlantArtifact)
	}
	if afterRecoveryMarkerCaptured {
		artifacts = append(artifacts, TargetSnapshotAfterRecoveryArtifact, TargetProcessAfterRecoveryArtifact)
	}
	if afterActivationMarkerCaptured {
		artifacts = append(artifacts, TargetSnapshotAfterActivationArtifact, TargetProcessAfterActivationArtifact)
	}
	if observationPlan != nil {
		artifacts = append(artifacts, observation.ObservationPlanArtifact, observation.TargetedProbeReportArtifact)
	}
	if fallbackArtifact != "" {
		artifacts = append(artifacts, fallbackArtifact)
	}
	if observationMode == TargetObservationModePruned && fallbackArtifact != "" {
		artifacts = append(artifacts, TargetFullFallbackProcessArtifact)
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
	if afterPlantMarkerCaptured {
		observations = append(observations,
			core.StateObservation{Layer: "agent", StateClass: "lifecycle-marker", Phase: "P4", Artifact: lifecycleMarkerArtifact, Kind: "jsonl", Description: "explicit target-emitted after-plant marker observed while the command was still running"},
			core.StateObservation{Layer: "os", StateClass: "filesystem", Phase: "P4", Artifact: TargetSnapshotAfterPlantArtifact, Kind: "filesystem-snapshot", Description: "filesystem observation captured at the explicit after-plant marker"},
			core.StateObservation{Layer: "os", StateClass: "process", Phase: "P4", Artifact: TargetProcessAfterPlantArtifact, Kind: "process-snapshot", Description: "process observation captured at the explicit after-plant marker"},
		)
	}
	if afterRecoveryMarkerCaptured {
		observations = append(observations,
			core.StateObservation{Layer: "agent", StateClass: "lifecycle-marker", Phase: "P6", Artifact: lifecycleMarkerArtifact, Kind: "jsonl", Description: "explicit target-emitted after-recovery marker observed while the command was still running"},
			core.StateObservation{Layer: "os", StateClass: "filesystem", Phase: "P6", Artifact: TargetSnapshotAfterRecoveryArtifact, Kind: "filesystem-snapshot", Description: "filesystem observation captured at the explicit after-recovery marker"},
			core.StateObservation{Layer: "os", StateClass: "process", Phase: "P6", Artifact: TargetProcessAfterRecoveryArtifact, Kind: "process-snapshot", Description: "process observation captured at the explicit after-recovery marker"},
		)
	}
	if afterActivationMarkerCaptured {
		observations = append(observations,
			core.StateObservation{Layer: "agent", StateClass: "lifecycle-marker", Phase: "P7", Artifact: lifecycleMarkerArtifact, Kind: "jsonl", Description: "explicit target-emitted after-activation marker observed while the command was still running"},
			core.StateObservation{Layer: "os", StateClass: "filesystem", Phase: "P7", Artifact: TargetSnapshotAfterActivationArtifact, Kind: "filesystem-snapshot", Description: "filesystem observation captured at the explicit after-activation marker"},
			core.StateObservation{Layer: "os", StateClass: "process", Phase: "P7", Artifact: TargetProcessAfterActivationArtifact, Kind: "process-snapshot", Description: "process observation captured at the explicit after-activation marker"},
		)
	}
	if observationPlan != nil {
		probeDescription := "plan-selected objects projected from retained broad artifacts"
		if observationMode == TargetObservationModePruned {
			probeDescription = "plan-selected filesystem and local process/FD snapshots with final broad filesystem/process fallbacks"
		} else if observationMode == TargetObservationModePrunedFilesystem {
			probeDescription = "plan-selected filesystem snapshots with a final broad filesystem fallback"
		}
		observations = append(observations,
			core.StateObservation{Layer: "agent", StateClass: "observation-plan", Phase: "P1", Artifact: observation.ObservationPlanArtifact, Kind: "query-specific-probe-plan", Description: "validated query-specific plan copied into the target run"},
			core.StateObservation{Layer: "os", StateClass: "targeted-probe", Phase: "P6", Artifact: observation.TargetedProbeReportArtifact, Kind: "targeted-probe-report", Description: probeDescription},
		)
	}
	if fallbackArtifact != "" {
		observations = append(observations, core.StateObservation{Layer: "os", StateClass: "filesystem", Phase: fallbackPhase, Artifact: fallbackArtifact, Kind: "full-fallback-filesystem-snapshot", Description: "full filesystem fallback retained after pruned primary collection"})
	}
	if observationMode == TargetObservationModePruned && fallbackArtifact != "" {
		observations = append(observations, core.StateObservation{Layer: "os", StateClass: "process", Phase: fallbackPhase, Artifact: TargetFullFallbackProcessArtifact, Kind: "full-fallback-process-snapshot", Description: "full workspace-related process fallback retained after selected process/FD collection"})
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
	if afterPlantMarkerCaptured {
		faultPhases = append(faultPhases, "P4 explicit target after-plant marker observed")
	}
	if afterRecoveryMarkerCaptured {
		faultPhases = append(faultPhases, "P6 explicit target after-recovery marker observed")
	}
	if afterActivationMarkerCaptured {
		faultPhases = append(faultPhases, "P7 explicit target after-activation marker observed")
	}
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
	stateClasses := []string{"workspace", "process", "target-command"}
	if afterPlantMarkerCaptured {
		stateClasses = append(stateClasses, "lifecycle-marker")
	}
	if observationPlan != nil {
		stateClasses = append(stateClasses, "query-specific-targeted-probe")
	}

	manifest := core.CaseManifest{
		Objective:         opts.Objective,
		StateClasses:      stateClasses,
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
		ObservationPlanArtifact:  targetObservationPlanArtifactValue(observationPlan),
		ObservationPlanQueryID:   targetObservationPlanQueryIDValue(observationPlan),
		LifecycleMarkerArtifact:  lifecycleMarkerArtifact,
		LifecycleMarkers:         lifecycleMarkers,
		TargetedProbeArtifact:    targetedProbeArtifactValue(targetedProbeReport),
		ObservationMode:          observationMode,
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

func targetObservationQueryID(task TargetTask) string {
	if task.Scenario != nil {
		if scenarioID := strings.TrimSpace(task.Scenario.ScenarioID); scenarioID != "" {
			return scenarioID
		}
		if scenarioTaskID := strings.TrimSpace(task.Scenario.TaskID); scenarioTaskID != "" {
			return scenarioTaskID
		}
	}
	return strings.TrimSpace(task.TaskID)
}

func normalizeTargetObservationMode(value string, hasObservationPlan bool) (string, error) {
	mode := strings.TrimSpace(value)
	if !hasObservationPlan {
		if mode == "" {
			return "", nil
		}
		return "", fmt.Errorf("observation mode %q requires --observation-plan", mode)
	}
	if mode == "" {
		return TargetObservationModeShadow, nil
	}
	switch mode {
	case TargetObservationModeShadow, TargetObservationModePrunedFilesystem, TargetObservationModePruned:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported observation mode %q; use %s, %s, or %s", mode, TargetObservationModeShadow, TargetObservationModePrunedFilesystem, TargetObservationModePruned)
	}
}

func targetUsesPrunedFilesystem(mode string) bool {
	return mode == TargetObservationModePrunedFilesystem || mode == TargetObservationModePruned
}

func targetProcessSelectorsForPlan(plan observation.ObservationPlan) ([]core.ProcessSelector, error) {
	selectors, err := observation.ProcessSelectorsForPlan(plan)
	if err != nil {
		return nil, err
	}
	coreSelectors := make([]core.ProcessSelector, 0, len(selectors))
	for _, selector := range selectors {
		coreSelectors = append(coreSelectors, core.ProcessSelector{
			Executable:  selector.Executable,
			CommandLine: selector.CommandLine,
		})
	}
	return core.NormalizeProcessSelectors(coreSelectors), nil
}

func targetUnplannedFallbackPaths(plan observation.ObservationPlan, snapshot core.Snapshot) ([]string, error) {
	paths, err := observation.UnplannedFilesystemPaths(plan, snapshot)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasPrefix(path, TargetLifecycleMarkerArtifact+".") {
			continue
		}
		switch path {
		case TargetPromptArtifact, TargetTaskArtifact, TargetLifecycleMarkerArtifact, TargetLifecycleMarkerHelperArtifact:
			continue
		default:
			filtered = append(filtered, path)
		}
	}
	return filtered, nil
}

func captureTargetedProbeCheckpoint(report *observation.TargetedProbeReport, plan *observation.ObservationPlan, point observation.ObservationPoint, runnerPhase string, filesystem *core.Snapshot, processes *core.ProcessSnapshot, reason string) error {
	if report == nil || plan == nil || !observationPlanIncludes(*plan, point) {
		return nil
	}
	checkpoint, err := observation.CaptureTargetedProbeCheckpoint(*plan, point, runnerPhase, filesystem, processes, reason)
	if err != nil {
		return err
	}
	return report.AddCheckpoint(checkpoint)
}

func observationPlanIncludes(plan observation.ObservationPlan, point observation.ObservationPoint) bool {
	for _, candidate := range plan.Checkpoints {
		if candidate == point {
			return true
		}
	}
	return false
}

func targetObservationPlanArtifactValue(plan *observation.ObservationPlan) string {
	if plan == nil {
		return ""
	}
	return observation.ObservationPlanArtifact
}

func targetObservationPlanQueryIDValue(plan *observation.ObservationPlan) string {
	if plan == nil {
		return ""
	}
	return plan.QueryID
}

func targetedProbeArtifactValue(report *observation.TargetedProbeReport) string {
	if report == nil {
		return ""
	}
	return observation.TargetedProbeReportArtifact
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

func evaluateTargetOracleForScenario(workspace string, targetID string, taskID string, scenario *TargetScenarioInfo, completed bool, immediateMissing []string, lineage core.ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	if scenario != nil {
		if scenario.ScenarioID == GeneratedEnvReplayPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedEnvReplayTargetOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedFunctionReplayPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedFunctionReplayTargetOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedEnvForkPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedEnvForkTargetOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedFunctionForkPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedFunctionForkTargetOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedTrustedActionContinuationScenarioID {
			return evaluateGeneratedTrustedActionContinuationOracle(workspace, targetID, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedProcessTrustedActionScenarioID {
			return evaluateGeneratedProcessTrustedActionOracle(workspace, completed, immediateMissing, lineage, lateObserved, latePresent, lateMissing)
		}
		if scenario.ScenarioID == GeneratedTrustedActionActivationScenarioID {
			return evaluateGeneratedTrustedActionTargetOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedOpenFDTrustedActionScenarioID {
			return evaluateGeneratedOpenFDTrustedActionOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedDeletedOpenFDTrustedActionScenarioID {
			return evaluateGeneratedDeletedOpenFDTrustedActionOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedInheritedFDTrustedActionScenarioID {
			return evaluateGeneratedInheritedFDTrustedActionOracle(workspace, completed, immediateMissing)
		}
		if scenario.ScenarioID == GeneratedUnixListenerReplayLifecycleSpliceScenarioID {
			return evaluateGeneratedUnixListenerReplayLifecycleSpliceOracle(workspace, completed, immediateMissing)
		}
		switch strings.TrimSpace(scenario.OracleKindID) {
		case "env-residue":
			return evaluateEnvResidueTargetOracle(workspace, targetID, completed, immediateMissing)
		case "function-residue":
			return evaluateFunctionResidueTargetOracle(workspace, targetID, completed, immediateMissing)
		case "cwd-residue":
			return evaluateCWDResidueTargetOracle(workspace, targetID, completed, immediateMissing)
		case "umask-residue":
			return evaluateUmaskResidueTargetOracle(workspace, targetID, completed, immediateMissing)
		}
	}
	return evaluateTargetOracle(workspace, targetID, taskID, completed, immediateMissing, lineage, lateObserved, latePresent, lateMissing)
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
	case MAFSessionContinuityTargetTaskID:
		return evaluateMAFSessionContinuityTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowCheckpointTargetTaskID:
		return evaluateMAFWorkflowCheckpointTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowExternalReplayTargetTaskID:
		return evaluateMAFWorkflowExternalReplayTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowHTTPReplayTargetTaskID:
		return evaluateMAFWorkflowHTTPReplayTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowResourceReplayTargetTaskID:
		return evaluateMAFWorkflowResourceReplayTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowAuthorityReplayTargetTaskID:
		return evaluateMAFWorkflowAuthorityReplayTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowPartialCommitTargetTaskID:
		return evaluateMAFWorkflowPartialCommitTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowApprovalPendingTargetTaskID:
		return evaluateMAFWorkflowApprovalPendingTargetOracle(workspace, completed, immediateMissing)
	case MAFWorkflowRehydrateDivergenceTargetTaskID:
		return evaluateMAFWorkflowRehydrateDivergenceTargetOracle(workspace, completed, immediateMissing)
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

func execTargetCommand(ctx context.Context, env core.Environment, run *core.RunContext, opts TargetRunOptions, workspacePath string, markerHostPath string, markerEnvironmentPath string, markerCommand string, observeMarker func(TargetLifecycleMarker) error) (TargetCommandResult, []byte, error) {
	commandEnv := targetCommandEnv(opts, run.RunID, workspacePath, markerEnvironmentPath, markerCommand)
	started := time.Now()
	commandCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	type targetCommandExecution struct {
		output []byte
		err    error
	}
	executionDone := make(chan targetCommandExecution, 1)
	go func() {
		output, err := env.ExecTargetCommand(commandCtx, run, opts.Command, commandEnv)
		executionDone <- targetCommandExecution{output: output, err: err}
	}()

	observedMarkers := 0
	consumeMarkers := func() error {
		markers, err := readTargetLifecycleMarkers(markerHostPath)
		if err != nil {
			return err
		}
		for observedMarkers < len(markers) {
			if observeMarker != nil {
				if err := observeMarker(markers[observedMarkers]); err != nil {
					return err
				}
			}
			observedMarkers++
		}
		return nil
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	var execution targetCommandExecution
	for {
		select {
		case execution = <-executionDone:
			if err := consumeMarkers(); err != nil {
				return TargetCommandResult{}, nil, err
			}
			goto commandFinished
		case <-ticker.C:
			if err := consumeMarkers(); err != nil {
				cancel()
				return TargetCommandResult{}, nil, err
			}
		case <-ctx.Done():
			cancel()
			return TargetCommandResult{}, nil, ctx.Err()
		}
	}

commandFinished:
	output := execution.output
	err := execution.err

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

func targetCommandEnv(opts TargetRunOptions, runID string, workspacePath string, markerPath string, markerCommand string) map[string]string {
	promptFile := filepath.Join(workspacePath, TargetPromptArtifact)
	taskFile := filepath.Join(workspacePath, TargetTaskArtifact)
	env := map[string]string{
		"SYNCFUZZ_ADAPTER_ID":            opts.AdapterID,
		"SYNCFUZZ_TARGET_ID":             opts.TargetID,
		"SYNCFUZZ_TASK_ID":               opts.TaskID,
		"SYNCFUZZ_RUN_ID":                runID,
		"SYNCFUZZ_REPO_ROOT":             targetRepoRoot(),
		"SYNCFUZZ_WORKSPACE":             workspacePath,
		"SYNCFUZZ_PROMPT":                opts.Prompt,
		"SYNCFUZZ_PROMPT_FILE":           promptFile,
		"SYNCFUZZ_TASK_FILE":             taskFile,
		"SYNCFUZZ_LIFECYCLE_MARKER_FILE": markerPath,
		"SYNCFUZZ_LIFECYCLE_MARKER":      markerCommand,
	}
	for key, value := range targetTaskEnvOverridesWithPlan(opts.TaskID, opts.ExecutionPlan) {
		env[key] = value
	}
	return env
}

func writeTargetLifecycleMarkerHelper(path string) error {
	const helper = `#!/usr/bin/env bash
set -euo pipefail
if [ "$#" -ne 1 ]; then
  printf 'usage: %s after-plant|after-recovery|after-activation\n' "$0" >&2
  exit 2
fi
case "$1" in
  after-plant|after-recovery|after-activation) ;;
  *) printf 'unsupported lifecycle event: %s\n' "$1" >&2; exit 2 ;;
esac
: "${SYNCFUZZ_LIFECYCLE_MARKER_FILE:?SYNCFUZZ_LIFECYCLE_MARKER_FILE is required}"
timestamp=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
printf '{"schema_version":"syncfuzz.target-lifecycle-marker.v1","event":"%s","timestamp":"%s"}\n' "$1" "$timestamp" >> "$SYNCFUZZ_LIFECYCLE_MARKER_FILE"
ack_path="${SYNCFUZZ_LIFECYCLE_MARKER_FILE}.${1}.ack"
while [ ! -f "$ack_path" ]; do sleep 0.005; done
`
	if err := os.WriteFile(path, []byte(helper), 0o755); err != nil {
		return fmt.Errorf("write lifecycle marker helper: %w", err)
	}
	return nil
}

func writeTargetLifecycleMarkerAcknowledgement(markerPath string, event string) error {
	if !isSupportedTargetLifecycleMarkerEvent(event) {
		return fmt.Errorf("unsupported lifecycle marker acknowledgement event %q", event)
	}
	path := markerPath + "." + event + ".ack"
	if err := os.WriteFile(path, []byte("captured\n"), 0o644); err != nil {
		return fmt.Errorf("write lifecycle marker acknowledgement: %w", err)
	}
	return nil
}

func readTargetLifecycleMarkers(path string) ([]TargetLifecycleMarker, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open lifecycle marker artifact: %w", err)
	}
	defer file.Close()

	markers := make([]TargetLifecycleMarker, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var marker TargetLifecycleMarker
		if err := json.Unmarshal([]byte(line), &marker); err != nil {
			return nil, fmt.Errorf("decode lifecycle marker: %w", err)
		}
		if marker.SchemaVersion != TargetLifecycleMarkerSchemaVersion {
			return nil, fmt.Errorf("unsupported lifecycle marker schema %q", marker.SchemaVersion)
		}
		if !isSupportedTargetLifecycleMarkerEvent(marker.Event) {
			return nil, fmt.Errorf("unsupported lifecycle marker event %q", marker.Event)
		}
		if strings.TrimSpace(marker.Timestamp) == "" {
			return nil, fmt.Errorf("lifecycle marker %q is missing timestamp", marker.Event)
		}
		markers = append(markers, marker)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read lifecycle marker artifact: %w", err)
	}
	return markers, nil
}

func isSupportedTargetLifecycleMarkerEvent(event string) bool {
	return targetLifecycleMarkerOrder(event) != 0
}

func targetLifecycleMarkerOrder(event string) int {
	switch event {
	case TargetLifecycleAfterPlantEvent:
		return 1
	case TargetLifecycleAfterRecoveryEvent:
		return 2
	case TargetLifecycleAfterActivationEvent:
		return 3
	default:
		return 0
	}
}

func targetTaskEnvOverrides(taskID string) map[string]string {
	return targetTaskEnvOverridesWithPlan(taskID, nil)
}

func targetTaskEnvOverridesWithPlan(taskID string, executionPlan *TargetScenarioExecutionPlan) map[string]string {
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
	plan := targetScenarioExecutionPlanInfo(scenario.Lifecycle)
	if executionPlan != nil {
		copyPlan := *executionPlan
		plan = &copyPlan
	}
	if plan == nil {
		return base
	}
	if plan.Replay {
		base["SYNCFUZZ_LANGGRAPH_REPLAY"] = "true"
	}
	if plan.CheckpointSelector != "" {
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = plan.CheckpointSelector
	}
	if plan.ForkFollowup && plan.ForkMessage != "" {
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = plan.ForkMessage
	}
	if plan.CheckpointBackend != "" {
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = plan.CheckpointBackend
	}
	if plan.ProcessMode != "" {
		base["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] = plan.ProcessMode
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

func observeTargetWorkspace(ctx context.Context, env core.Environment, run *core.RunContext, phase string, snapshotArtifact string, processArtifact string, snapshotFilesystem func(string) (core.Snapshot, error), recordProcesses func(context.Context, core.Environment, *core.RunContext, string, string) (core.ProcessSnapshot, error)) (core.Snapshot, core.ProcessSnapshot, error) {
	if snapshotFilesystem == nil {
		snapshotFilesystem = core.SnapshotFilesystem
	}
	if recordProcesses == nil {
		recordProcesses = core.RecordProcessSnapshot
	}
	snapshot, err := snapshotFilesystem(run.Workspace)
	if err != nil {
		return core.Snapshot{}, core.ProcessSnapshot{}, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, snapshotArtifact), snapshot); err != nil {
		return core.Snapshot{}, core.ProcessSnapshot{}, err
	}
	processSnapshot, err := recordProcesses(ctx, env, run, phase, processArtifact)
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
