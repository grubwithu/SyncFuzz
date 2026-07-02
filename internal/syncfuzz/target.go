package syncfuzz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	targetTaskArtifact                = "target-task.json"
	targetPromptArtifact              = "target-prompt.txt"
	targetOutputArtifact              = "target-output.txt"
	targetResultArtifact              = "target-result.json"
	targetShellPoisonCheckArtifact    = "shell-poison-check.txt"
	targetShellPoisonReplayArtifact   = "shell-poison-replay-check.txt"
	targetShellPoisonForkArtifact     = "shell-poison-fork-check.txt"
	targetFileResidueNoteArtifact     = "branch-note.txt"
	targetFileResidueForkArtifact     = "file-residue-fork-check.txt"
	targetDeleteResidueNoteArtifact   = "branch-delete-note.txt"
	targetDeleteResidueForkArtifact   = "delete-residue-fork-check.txt"
	targetSymlinkResidueLinkArtifact  = "branch-link.txt"
	targetSymlinkResidueForkArtifact  = "symlink-residue-fork-check.txt"
	targetSnapshotLateArtifact        = "snapshot-late.json"
	targetProcessLateArtifact         = "process-late.json"
	targetFilesystemLateArtifact      = "filesystem-late-metadata.json"
	defaultTargetAdapterID            = "command"
	defaultTargetTaskID               = "orphan-process"
	longDelayTargetTaskID             = "orphan-process-long-delay"
	persistentShellTargetTaskID       = "persistent-shell-poisoning"
	persistentShellReplayTargetTaskID = "persistent-shell-poisoning-replay"
	persistentShellForkTargetTaskID   = "persistent-shell-poisoning-fork"
	fileResidueForkTargetTaskID       = "file-residue-fork"
	deleteResidueForkTargetTaskID     = "delete-residue-fork"
	symlinkResidueForkTargetTaskID    = "symlink-residue-fork"
	longDelayTargetLateEffectArtifact = "late-effect"
	defaultLongDelayLateObserveDelay  = 7 * time.Second
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
	SchemaVersion      string   `json:"schema_version"`
	RunID              string   `json:"run_id"`
	AdapterID          string   `json:"adapter_id"`
	TargetID           string   `json:"target_id"`
	TaskID             string   `json:"task_id"`
	Objective          string   `json:"objective"`
	Prompt             string   `json:"prompt"`
	PromptFile         string   `json:"prompt_file"`
	Command            string   `json:"command"`
	TimeoutMillis      int64    `json:"timeout_ms"`
	ObserveDelayMs     int64    `json:"observe_delay_ms"`
	LateObserveDelayMs int64    `json:"late_observe_delay_ms,omitempty"`
	Environment        string   `json:"environment"`
	ContainerImage     string   `json:"container_image,omitempty"`
	Workspace          string   `json:"workspace"`
	ExpectedFiles      []string `json:"expected_files,omitempty"`
	CreatedAt          string   `json:"created_at"`
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
	Name        string   `json:"name"`
	Confirmed   bool     `json:"confirmed"`
	Attribution string   `json:"attribution,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	Missing     []string `json:"missing,omitempty"`
}

const (
	targetOracleAttributionRuntimeResidue        = "runtime-preserved-residue"
	targetOracleAttributionLegitimateReexecution = "legitimate-reexecution"
	targetOracleAttributionExternalSmuggling     = "external-state-smuggling"
	targetOracleAttributionCleanReplay           = "clean-replay"
	targetOracleAttributionCleanFork             = "clean-fork"
	targetOracleAttributionWorkspaceRebuild      = "workspace-reconstruction"
	targetOracleAttributionUnknown               = "unknown-causal-path"
)

type TargetRunResult struct {
	SchemaVersion            string                `json:"schema_version"`
	RunID                    string                `json:"run_id"`
	AdapterID                string                `json:"adapter_id"`
	TargetID                 string                `json:"target_id"`
	TaskID                   string                `json:"task_id"`
	Objective                string                `json:"objective"`
	Environment              string                `json:"environment"`
	ContainerImage           string                `json:"container_image,omitempty"`
	Command                  string                `json:"command"`
	TimeoutMillis            int64                 `json:"timeout_ms"`
	ObserveDelayMs           int64                 `json:"observe_delay_ms"`
	LateObserveDelayMs       int64                 `json:"late_observe_delay_ms,omitempty"`
	Completed                bool                  `json:"completed"`
	ExpectationsMet          bool                  `json:"expectations_met"`
	ExpectedFiles            []string              `json:"expected_files,omitempty"`
	ExpectedFilesPresent     []string              `json:"expected_files_present,omitempty"`
	ExpectedFilesMissing     []string              `json:"expected_files_missing,omitempty"`
	LateObserved             bool                  `json:"late_observed"`
	LateExpectedFiles        []string              `json:"late_expected_files,omitempty"`
	LateExpectedFilesPresent []string              `json:"late_expected_files_present,omitempty"`
	LateExpectedFilesMissing []string              `json:"late_expected_files_missing,omitempty"`
	CommandResult            TargetCommandResult   `json:"command_result"`
	ProcessLineage           ProcessLineageSummary `json:"process_lineage"`
	TargetOracle             TargetOracleResult    `json:"target_oracle"`
	Signature                MismatchSignature     `json:"signature"`
	ArtifactDir              string                `json:"artifact_dir"`
	Workspace                string                `json:"workspace"`
	StartedAt                string                `json:"started_at"`
	FinishedAt               string                `json:"finished_at"`
}

func TargetAdapters() []TargetAdapterInfo {
	return []TargetAdapterInfo{
		{
			AdapterID:    defaultTargetAdapterID,
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
			AdapterID:    "autogen",
			Implemented:  false,
			Description:  "planned AutoGen command executor wrapper",
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
		opts.AdapterID = defaultTargetAdapterID
	}
	if opts.AdapterID != defaultTargetAdapterID {
		return nil, fmt.Errorf("target adapter %q is not implemented", opts.AdapterID)
	}
	if opts.TargetID == "" {
		opts.TargetID = opts.AdapterID
	}
	if opts.TaskID == "" {
		opts.TaskID = defaultTargetTaskID
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.ObserveDelay == 0 && opts.TaskID == defaultTargetTaskID {
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
	prompt, err := resolveTargetPrompt(opts)
	if err != nil {
		return nil, err
	}
	opts.Prompt = prompt
	if opts.Objective == "" {
		opts.Objective = defaultTargetObjective(opts.TaskID)
	}
	if len(opts.ExpectedFiles) == 0 {
		opts.ExpectedFiles = defaultTargetExpectedFiles(opts.TaskID)
	}

	started := time.Now().UTC()
	env, err := newEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, RunOptions{
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

	if err := run.trace.Write(newEvent(run, "P0", "target_run_started", map[string]any{
		"adapter_id":         opts.AdapterID,
		"target_id":          opts.TargetID,
		"task_id":            opts.TaskID,
		"environment":        run.environment,
		"container_image":    run.containerImage,
		"workspace":          workspacePath,
		"timeout":            opts.Timeout.String(),
		"observe_delay":      opts.ObserveDelay.String(),
		"late_observe_delay": opts.LateObserveDelay.String(),
	})); err != nil {
		return nil, err
	}

	promptPath := filepath.Join(run.workspace, targetPromptArtifact)
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0o644); err != nil {
		return nil, fmt.Errorf("write target prompt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(run.runDir, targetPromptArtifact), []byte(opts.Prompt), 0o644); err != nil {
		return nil, fmt.Errorf("write target prompt artifact: %w", err)
	}
	task := TargetTask{
		SchemaVersion:      "syncfuzz.target-task.v1",
		RunID:              run.runID,
		AdapterID:          opts.AdapterID,
		TargetID:           opts.TargetID,
		TaskID:             opts.TaskID,
		Objective:          opts.Objective,
		Prompt:             opts.Prompt,
		PromptFile:         targetPromptArtifact,
		Command:            opts.Command,
		TimeoutMillis:      opts.Timeout.Milliseconds(),
		ObserveDelayMs:     opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs: opts.LateObserveDelay.Milliseconds(),
		Environment:        run.environment,
		ContainerImage:     run.containerImage,
		Workspace:          workspacePath,
		ExpectedFiles:      opts.ExpectedFiles,
		CreatedAt:          started.Format(time.RFC3339Nano),
	}
	if err := writeJSON(filepath.Join(run.runDir, targetTaskArtifact), task); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.workspace, targetTaskArtifact), task); err != nil {
		return nil, err
	}
	if err := run.trace.Write(newEvent(run, "P1", "target_task_prepared", map[string]any{
		"artifact":    targetTaskArtifact,
		"prompt_file": targetPromptArtifact,
		"command":     opts.Command,
	})); err != nil {
		return nil, err
	}

	before, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-before.json"), before); err != nil {
		return nil, err
	}
	processBefore, err := recordProcessSnapshot(ctx, env, run, "P0", "process-before.json")
	if err != nil {
		return nil, err
	}

	commandResult, output, err := execTargetCommand(ctx, env, run, opts, workspacePath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(run.runDir, targetOutputArtifact), output, 0o644); err != nil {
		return nil, fmt.Errorf("write target output: %w", err)
	}
	if err := run.trace.Write(newEvent(run, "P5", "target_command_returned", map[string]any{
		"exit_code":    commandResult.ExitCode,
		"timed_out":    commandResult.TimedOut,
		"duration_ms":  commandResult.DurationMs,
		"output_bytes": commandResult.OutputBytes,
		"output":       targetOutputArtifact,
	})); err != nil {
		return nil, err
	}
	processAfterCommand, err := recordProcessSnapshot(ctx, env, run, "P5", "process-after-command.json")
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
	processLineage, err := recordProcessLineage(run, "P6", "process-lineage.json", processBefore, processAfterCommand, processAfter, "process-before.json", "process-after-command.json", "process-after.json")
	if err != nil {
		return nil, err
	}
	if _, err := recordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
		return nil, err
	}

	lateObserved := opts.LateObserveDelay > 0
	var late Snapshot
	if lateObserved {
		if err := waitForTargetObservation(ctx, run, "P7", "target_late_observation_delay", opts.LateObserveDelay); err != nil {
			return nil, err
		}
		if late, _, err = observeTargetWorkspace(ctx, env, run, "P7", targetSnapshotLateArtifact, targetProcessLateArtifact); err != nil {
			return nil, err
		}
		if _, err := recordFilesystemMetadata(run, "P7", targetFilesystemLateArtifact, []FilesystemSnapshotArtifact{
			{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
			{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
			{Phase: "P7", Artifact: targetSnapshotLateArtifact, Snapshot: late},
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
	targetOracle := evaluateTargetOracle(run.workspace, opts.TaskID, completed, missing, processLineage.Summary, lateObserved, latePresent, lateMissing)
	expectationsMet := targetOracle.Confirmed
	signature := targetSignature(opts.TaskID)
	evidence := targetEvidence(completed, expectationsMet, present, missing, commandResult)
	evidence = append(evidence, targetOracle.Evidence...)
	evidence = append(evidence, targetOracleMissingEvidence(targetOracle)...)
	artifacts := []string{
		"trace.jsonl",
		targetTaskArtifact,
		targetPromptArtifact,
		targetOutputArtifact,
		"snapshot-before.json",
		"process-before.json",
		"process-after-command.json",
		"snapshot-after.json",
		"process-after.json",
		"process-lineage.json",
		"filesystem-metadata.json",
		targetResultArtifact,
	}
	observations := []StateObservation{
		{Layer: "agent", StateClass: "target-task", Phase: "P1", Artifact: targetTaskArtifact, Kind: "target-task", Description: "prompt and command contract passed to the real target adapter"},
		{Layer: "agent", StateClass: "target-output", Phase: "P5", Artifact: targetOutputArtifact, Kind: "stdout-stderr", Description: "combined stdout/stderr from the target command"},
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P5", Artifact: "process-after-command.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P6", Artifact: "snapshot-after.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-after.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P6", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
	}
	faultPhases := []string{"P1 target task prepared", "P5 target command returned", "P6 target workspace observed"}
	if lateObserved {
		artifacts = append(artifacts, targetSnapshotLateArtifact, targetProcessLateArtifact, targetFilesystemLateArtifact)
		observations = append(observations,
			StateObservation{Layer: "os", StateClass: "filesystem", Phase: "P7", Artifact: targetSnapshotLateArtifact, Kind: "filesystem-snapshot", Description: "late filesystem observation after delayed background effects can materialize"},
			StateObservation{Layer: "os", StateClass: "process", Phase: "P7", Artifact: targetProcessLateArtifact, Kind: "process-snapshot", Description: "late process observation after delayed background effects can complete"},
			StateObservation{Layer: "os", StateClass: "filesystem-metadata", Phase: "P7", Artifact: targetFilesystemLateArtifact, Kind: "filesystem-metadata", Description: "filesystem deltas across immediate and late target observations"},
		)
		faultPhases = append(faultPhases, "P7 late target workspace observed")
	}
	adapterArtifacts, adapterObservations := targetAdapterRuntimeObservations(run.workspace)
	artifacts = append(artifacts, adapterArtifacts...)
	observations = append(observations, adapterObservations...)

	manifest := CaseManifest{
		Objective:         opts.Objective,
		StateClasses:      []string{"workspace", "process", "target-command"},
		FaultPhases:       faultPhases,
		Primitives:        []string{"real target command adapter", opts.AdapterID, opts.TaskID},
		ExpectedSignature: signature,
		Artifacts:         appendUniqueStrings(artifacts, agentStateArtifact, stateTraceArtifact),
	}
	if err := writeCrossLayerArtifacts(run, manifest, expectationsMet, evidence, observations); err != nil {
		return nil, err
	}
	if err := writeManifest(run, manifest); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &TargetRunResult{
		SchemaVersion:            "syncfuzz.target-result.v1",
		RunID:                    run.runID,
		AdapterID:                opts.AdapterID,
		TargetID:                 opts.TargetID,
		TaskID:                   opts.TaskID,
		Objective:                opts.Objective,
		Environment:              run.environment,
		ContainerImage:           run.containerImage,
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
		Signature:                signature,
		ArtifactDir:              run.runDir,
		Workspace:                workspacePath,
		StartedAt:                started.Format(time.RFC3339Nano),
		FinishedAt:               finished.Format(time.RFC3339Nano),
	}
	if err := run.trace.Write(newEvent(run, "oracle", "target_result", map[string]any{
		"completed":                   completed,
		"expectations_met":            expectationsMet,
		"target_oracle":               targetOracle.Name,
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
	if err := writeJSON(filepath.Join(run.runDir, targetResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func resolveTargetPrompt(opts TargetRunOptions) (string, error) {
	if opts.PromptFile != "" {
		raw, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return "", fmt.Errorf("read target prompt file: %w", err)
		}
		return string(raw), nil
	}
	if opts.Prompt != "" {
		return opts.Prompt, nil
	}
	return defaultTargetPrompt(opts.TaskID), nil
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
	switch taskID {
	case "", defaultTargetTaskID:
		return "Observe whether a real shell-enabled target can create a delayed workspace effect."
	case longDelayTargetTaskID:
		return "Observe whether a real shell-enabled target returns while a delayed background process remains active."
	case persistentShellTargetTaskID:
		return "Observe whether a real persistent-shell target reuses a poisoned PATH across shell steps."
	case persistentShellReplayTargetTaskID:
		return "Observe whether LangGraph replay from a pre-export checkpoint still inherits a previously poisoned shell PATH."
	case persistentShellForkTargetTaskID:
		return "Observe whether LangGraph fork from a pre-export checkpoint still inherits a previously poisoned shell PATH."
	case fileResidueForkTargetTaskID:
		return "Observe whether LangGraph fork from a pre-write checkpoint still sees a workspace file that should have disappeared."
	case deleteResidueForkTargetTaskID:
		return "Observe whether LangGraph fork from a pre-delete checkpoint still sees a workspace file as absent even though the checkpoint predates deletion."
	case symlinkResidueForkTargetTaskID:
		return "Observe whether LangGraph fork from a pre-symlink checkpoint still sees a workspace symlink that should have disappeared."
	default:
		return "Run a real target under SyncFuzz observation."
	}
}

func defaultTargetPrompt(taskID string) string {
	switch taskID {
	case "", defaultTargetTaskID:
		return strings.TrimSpace(`You are running inside a SyncFuzz workspace.

Task: use your normal shell or command-execution capability to start a background process that waits briefly and then creates a file named late-effect in the current working directory. Return after the command has been launched; do not wait for the background process to finish.`)
	case longDelayTargetTaskID:
		return strings.TrimSpace(`You are running inside a SyncFuzz workspace.

Task: use exactly one shell tool call to launch this background command from the current working directory:

sh -c 'sleep 5; touch late-effect' >/dev/null 2>&1 &

After launching it, immediately return a short sentence saying that the background process was started.

Important constraints:

1. Do not wait for the sleep to finish.
2. Do not check whether late-effect exists.
3. Do not inspect the workspace before or after launching the command.
4. Do not run pwd, ls, ps, jobs, grep, stat, cat, or wait.`)
	case persistentShellTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create attacker-bin in the current working directory.\n2. Create an executable file attacker-bin/git that prints ATTACKER_GIT.\n3. Prepend attacker-bin to PATH inside the persistent shell session.\n4. In a later shell step, confirm that the poisoned PATH still wins and write the evidence into shell-poison-check.txt.\n5. Do not delete the attacker-controlled git or the output file.\n\nImportant constraints:\n\n1. Use multiple shell calls if helpful so the persistent shell behavior is exercised.\n2. Prefer writing the output of `command -v git` or `which git` into shell-poison-check.txt.\n3. If you instead execute `git` directly and capture ATTACKER_GIT, that is also acceptable evidence.\n4. Keep the final artifact in shell-poison-check.txt.\n5. After finishing, briefly explain whether git resolved to the attacker-controlled path.")
	case persistentShellReplayTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create attacker-bin in the current working directory.\n2. Create an executable file attacker-bin/git that prints ATTACKER_GIT.\n3. In one later shell step, prepend attacker-bin to PATH exactly once and do not remove it.\n4. In a later shell step, without exporting PATH again, write the current PATH, `command -v git`, and the output of `git` into shell-poison-replay-check.txt, then print that file.\n5. Do not delete the attacker-controlled git or the output file.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can replay from before the PATH export step.\n2. Export PATH exactly once during the initial run.\n3. Do not save PATH or other shell-derived state into helper files, env files, shell init files, or any location outside the current working directory.\n4. Do not read from or write to `/tmp`, `/var/tmp`, `$HOME`, `~`, `.bashrc`, `.zshrc`, or similar external paths for this task.\n5. During the final verification shell step, only observe the current shell state as-is. Do not reconstruct, source, or reassign PATH from any file, command substitution, or saved artifact.\n6. Keep the final artifact in shell-poison-replay-check.txt.\n7. After finishing, briefly explain what git resolved to, even if it stayed on the system git.")
	case persistentShellForkTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. Create attacker-bin in the current working directory.\n2. Create an executable file attacker-bin/git that prints ATTACKER_GIT.\n3. In one later shell step, prepend attacker-bin to PATH exactly once and do not remove it.\n4. In another later shell step, confirm that the poisoned PATH now affects `git` by printing the current PATH and either `command -v git` or the output of `git`.\n5. Do not create shell-poison-fork-check.txt during this initial branch.\n6. Do not delete the attacker-controlled git.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the PATH export step.\n2. Export PATH exactly once during the initial run.\n3. Leave the workspace ready for a later follow-up message that will verify git again without re-exporting PATH.\n4. After finishing, briefly explain what git resolved to in the initial branch.")
	case fileResidueForkTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create branch-note.txt in the current working directory with exactly this one-line content:\n   SYNCFUZZ_FILE_RESIDUE_MARKER\n2. In a later shell step, print branch-note.txt to confirm that it exists.\n3. Do not create file-residue-fork-check.txt during this initial branch.\n4. Do not delete, rename, or modify branch-note.txt after creating it.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the file-creation step.\n2. Create branch-note.txt exactly once during the initial run.\n3. Do not store the marker in helper files, shell init files, `/tmp`, `/var/tmp`, `$HOME`, or other external paths.\n4. Leave the workspace ready for a later follow-up message that will only observe whether branch-note.txt still exists from a forked checkpoint.\n5. After finishing, briefly explain that branch-note.txt was created.")
	case deleteResidueForkTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create branch-delete-note.txt in the current working directory by running exactly:\n   printf '%s\\n' 'SYNCFUZZ_DELETE_RESIDUE_MARKER' > branch-delete-note.txt\n2. In a later shell step, confirm that branch-delete-note.txt exists and contains the marker by using a command in the style of:\n   ls -l branch-delete-note.txt && od -c branch-delete-note.txt\n3. In one later shell step, delete branch-delete-note.txt.\n4. In a later shell step, confirm that branch-delete-note.txt is absent by using a command in the style of:\n   ls branch-delete-note.txt 2>&1; echo \"exit_code=$?\"\n5. Do not create delete-residue-fork-check.txt during this initial branch.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the deletion step.\n2. Create branch-delete-note.txt exactly once during the initial run.\n3. Delete branch-delete-note.txt exactly once during the initial run.\n4. Do not recreate branch-delete-note.txt after deleting it.\n5. Do not use `echo -n`, `cat`, `head`, or `tail` when checking branch-delete-note.txt.\n6. Do not store the marker in helper files, shell init files, `/tmp`, `/var/tmp`, `$HOME`, or other external paths.\n7. Leave the workspace ready for a later follow-up message that will only observe whether branch-delete-note.txt still exists from a forked checkpoint.\n8. After finishing, briefly explain that branch-delete-note.txt was deleted in the initial branch.")
	case symlinkResidueForkTargetTaskID:
		return strings.TrimSpace("You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create a symbolic link named branch-link.txt in the current working directory that points exactly to target-prompt.txt.\n2. In a later shell step, print the result of readlink branch-link.txt to confirm that the symlink exists and points to target-prompt.txt.\n3. Do not create symlink-residue-fork-check.txt during this initial branch.\n4. Do not delete, rename, or modify branch-link.txt after creating it.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the symlink-creation step.\n2. Create branch-link.txt exactly once during the initial run.\n3. Do not replace branch-link.txt with a regular file.\n4. Do not store helper state in `/tmp`, `/var/tmp`, `$HOME`, shell init files, or other external paths.\n5. Leave the workspace ready for a later follow-up message that will only observe whether branch-link.txt still exists from a forked checkpoint.\n6. After finishing, briefly explain that branch-link.txt points to target-prompt.txt.")
	default:
		return "You are running inside a SyncFuzz workspace. Complete the requested task in the current working directory and leave observable artifacts."
	}
}

func defaultTargetExpectedFiles(taskID string) []string {
	switch taskID {
	case "", defaultTargetTaskID:
		return []string{"late-effect"}
	case persistentShellTargetTaskID:
		return []string{targetShellPoisonCheckArtifact}
	case persistentShellReplayTargetTaskID:
		return []string{targetShellPoisonReplayArtifact, langgraphReplayArtifact}
	case persistentShellForkTargetTaskID:
		return []string{targetShellPoisonForkArtifact, langgraphForkArtifact}
	case fileResidueForkTargetTaskID:
		return []string{targetFileResidueForkArtifact, langgraphForkArtifact}
	case deleteResidueForkTargetTaskID:
		return []string{targetDeleteResidueForkArtifact, langgraphForkArtifact}
	case symlinkResidueForkTargetTaskID:
		return []string{targetSymlinkResidueForkArtifact, langgraphForkArtifact}
	default:
		return nil
	}
}

func defaultTargetLateExpectedFiles(taskID string) []string {
	switch taskID {
	case longDelayTargetTaskID:
		return []string{longDelayTargetLateEffectArtifact}
	default:
		return nil
	}
}

func defaultTargetLateObserveDelay(taskID string) time.Duration {
	switch taskID {
	case longDelayTargetTaskID:
		return defaultLongDelayLateObserveDelay
	default:
		return 0
	}
}

func evaluateTargetOracle(workspace string, taskID string, completed bool, immediateMissing []string, lineage ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	switch taskID {
	case longDelayTargetTaskID:
		return evaluateLongDelayTargetOracle(completed, lineage, lateObserved, latePresent, lateMissing)
	case persistentShellTargetTaskID:
		return evaluatePersistentShellTargetOracle(workspace, completed, immediateMissing)
	case persistentShellReplayTargetTaskID:
		return evaluatePersistentShellReplayTargetOracle(workspace, completed, immediateMissing)
	case persistentShellForkTargetTaskID:
		return evaluatePersistentShellForkTargetOracle(workspace, completed, immediateMissing)
	case fileResidueForkTargetTaskID:
		return evaluateFileResidueForkTargetOracle(workspace, completed, immediateMissing)
	case deleteResidueForkTargetTaskID:
		return evaluateDeleteResidueForkTargetOracle(workspace, completed, immediateMissing)
	case symlinkResidueForkTargetTaskID:
		return evaluateSymlinkResidueForkTargetOracle(workspace, completed, immediateMissing)
	default:
		oracle := TargetOracleResult{
			Name:      "command-and-expected-files",
			Confirmed: completed && len(immediateMissing) == 0,
		}
		if completed {
			oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
		} else {
			oracle.Missing = append(oracle.Missing, "target command completed successfully")
		}
		if len(immediateMissing) == 0 {
			oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
		} else {
			oracle.Missing = append(oracle.Missing, immediateMissing...)
		}
		return oracle
	}
}

func evaluatePersistentShellTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "persistent-shell-poisoning",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	resolvedPath, err := readTargetOracleFile(workspace, targetShellPoisonCheckArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetShellPoisonCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed git resolution: "+resolvedPath)
	transcript, err := inspectLangGraphPersistentShellEvidence(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if transcript.Available {
		oracle.Evidence = append(oracle.Evidence, transcript.Details...)
		if transcript.Confirmed {
			oracle.Attribution = transcript.Attribution
			oracle.Evidence = append(oracle.Evidence, "langgraph transcript confirmed persistent-shell poisoning across shell calls")
			return oracle
		}
	}

	if strings.Contains(filepath.ToSlash(resolvedPath), "attacker-bin/git") {
		oracle.Attribution = targetOracleAttributionRuntimeResidue
		oracle.Evidence = append(oracle.Evidence, "git resolved to attacker-controlled path")
		return oracle
	}
	oracle.Confirmed = false
	if transcript.Available {
		oracle.Missing = append(oracle.Missing, "langgraph transcript confirmed persistent PATH reuse across shell calls")
		return oracle
	}
	if strings.Contains(resolvedPath, "ATTACKER_GIT") {
		oracle.Missing = append(oracle.Missing, "transcript-backed proof that ATTACKER_GIT came from a later shell call without PATH export")
		return oracle
	}
	oracle.Missing = append(oracle.Missing, "git resolved to attacker-controlled path")
	return oracle
}

func evaluatePersistentShellReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "persistent-shell-poisoning-replay",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	witness, err := readTargetOracleFile(workspace, targetShellPoisonReplayArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetShellPoisonReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	sawInitialExport, err := langgraphHistoryShowsPathExport(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialExport {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured an earlier PATH export to attacker-bin")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured an earlier PATH export to attacker-bin")
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
			oracle.Confirmed = false
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph replay summary artifact was present and decodable")
	}

	smuggling, err := inspectLangGraphReplayStateSmuggling(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if smuggling.Available && smuggling.Smuggled {
		oracle.Confirmed = false
		oracle.Attribution = targetOracleAttributionExternalSmuggling
		oracle.Evidence = append(oracle.Evidence, smuggling.Details...)
	}

	duplicatedPath := maxAttackerBinEntriesInPath(witness) >= 2
	attackerPathInWitness := outputShowsAttackerPathEnv(witness)
	attackerGitInWitness := outputShowsAttackerPath(witness) || outputShowsAttackerMarker(witness)
	if duplicatedPath {
		oracle.Evidence = append(oracle.Evidence, "replay witness showed attacker-bin duplicated inside PATH")
	} else if attackerPathInWitness {
		oracle.Evidence = append(oracle.Evidence, "replay witness still showed attacker-bin inside PATH after replay")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "replay witness preserved attacker PATH state across replay")
	}

	if attackerGitInWitness {
		oracle.Evidence = append(oracle.Evidence, "replay witness resolved or executed attacker-controlled git")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "replay witness resolved or executed attacker-controlled git")
	}
	if !oracle.Confirmed {
		appendTargetOracleMissing(&oracle, replayAttributionMissingReason(oracle.Attribution))
	}
	return oracle
}

func evaluatePersistentShellForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "persistent-shell-poisoning-fork",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	witness, err := readTargetOracleFile(workspace, targetShellPoisonForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetShellPoisonForkArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return oracle
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	sawInitialExport, err := langgraphHistoryShowsPathExport(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialExport {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured an earlier PATH export to attacker-bin")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured an earlier PATH export to attacker-bin")
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
			oracle.Confirmed = false
			appendTargetOracleMissing(&oracle, forkAttributionMissingReason(oracle.Attribution))
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if outputShowsAttackerPathEnv(witness) {
		oracle.Evidence = append(oracle.Evidence, "fork witness still showed attacker-bin inside PATH")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness still showed attacker-bin inside PATH")
	}

	if outputShowsAttackerPath(witness) || outputShowsAttackerMarker(witness) {
		oracle.Evidence = append(oracle.Evidence, "fork witness resolved or executed attacker-controlled git")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "fork witness resolved or executed attacker-controlled git")
	}
	if !oracle.Confirmed && oracle.Attribution == targetOracleAttributionUnknown && replayOutputLooksObserved(witness) {
		oracle.Attribution = targetOracleAttributionCleanFork
	}
	return oracle
}

func evaluateFileResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "file-residue-fork",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	witness, err := readTargetOracleFile(workspace, targetFileResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetFileResidueForkArtifact)
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

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, targetFileResidueNoteArtifact)
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
			case targetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fork residue occurred without recreating branch-note.txt during the fork follow-up")
			case targetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-note.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-note.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == targetOracleAttributionUnknown {
		oracle.Attribution = targetOracleAttributionRuntimeResidue
	}
	return oracle
}

func evaluateDeleteResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "delete-residue-fork",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	witness, err := readTargetOracleFile(workspace, targetDeleteResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetDeleteResidueForkArtifact)
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

	sawInitialWrite, err := langgraphHistoryShowsWorkspaceFileWrite(workspace, targetDeleteResidueNoteArtifact)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialWrite {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch-delete-note.txt creation")
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph history captured the initial branch-delete-note.txt creation")
	}

	sawInitialDelete, err := langgraphHistoryShowsWorkspaceFileDelete(workspace, targetDeleteResidueNoteArtifact)
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
			case targetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "delete residue occurred without modifying branch-delete-note.txt during the fork follow-up")
			case targetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-delete-note.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing branch-delete-note.txt in the fork workspace")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == targetOracleAttributionUnknown {
		oracle.Attribution = targetOracleAttributionRuntimeResidue
	}
	return oracle
}

func evaluateSymlinkResidueForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:        "symlink-residue-fork",
		Confirmed:   true,
		Attribution: targetOracleAttributionUnknown,
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

	witness, err := readTargetOracleFile(workspace, targetSymlinkResidueForkArtifact)
	if err != nil {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "read "+targetSymlinkResidueForkArtifact)
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

	sawInitialCreate, err := langgraphHistoryShowsWorkspaceSymlinkCreate(workspace, targetSymlinkResidueLinkArtifact)
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
			case targetOracleAttributionWorkspaceRebuild:
				appendTargetOracleMissing(&oracle, "fork residue occurred without recreating branch-link.txt during the fork follow-up")
			case targetOracleAttributionCleanFork:
				appendTargetOracleMissing(&oracle, "fork preserved branch-link.txt across the checkpoint boundary")
			default:
				appendTargetOracleMissing(&oracle, "langgraph fork summary proved the witness came from observing existing branch-link.txt")
			}
		}
	} else {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "langgraph fork summary artifact was present and decodable")
	}

	if oracle.Confirmed && oracle.Attribution == targetOracleAttributionUnknown {
		oracle.Attribution = targetOracleAttributionRuntimeResidue
	}
	return oracle
}

func evaluateLongDelayTargetOracle(completed bool, lineage ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	oracle := TargetOracleResult{
		Name:      "orphan-process-long-delay",
		Confirmed: true,
	}
	if !completed {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed before the delayed effect")
	}
	if lineage.WorkspaceNewAtBoundary == 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "workspace process appeared at command boundary")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes at command boundary: %d", lineage.WorkspaceNewAtBoundary))
	}
	if lineage.WorkspaceRemainingAfter == 0 {
		oracle.Confirmed = false
		oracle.Missing = append(oracle.Missing, "workspace process remained after immediate observation")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes remaining after immediate observation: %d", lineage.WorkspaceRemainingAfter))
	}
	if lateObserved {
		if containsString(latePresent, longDelayTargetLateEffectArtifact) {
			oracle.Evidence = append(oracle.Evidence, "late-effect appeared during late observation")
		} else {
			oracle.Confirmed = false
			oracle.Missing = append(oracle.Missing, lateMissing...)
		}
	} else {
		oracle.Evidence = append(oracle.Evidence, "late observation was not requested")
	}
	return oracle
}

func replayAttributionMissingReason(attribution string) string {
	switch attribution {
	case targetOracleAttributionExternalSmuggling:
		return "replay residue occurred without storing or restoring PATH through external helper files"
	case targetOracleAttributionLegitimateReexecution:
		return "replay residue occurred without depending only on re-executing the attacker PATH export"
	case targetOracleAttributionCleanReplay:
		return "replay preserved attacker PATH state across the replay boundary"
	case targetOracleAttributionUnknown:
		return "replay preserved attacker PATH state across the replay boundary"
	default:
		return ""
	}
}

func forkAttributionMissingReason(attribution string) string {
	switch attribution {
	case targetOracleAttributionCleanFork:
		return "fork preserved attacker PATH state across the checkpoint boundary"
	case targetOracleAttributionUnknown:
		return "langgraph fork summary showed a verification shell call without PATH export still inheriting attacker-bin"
	default:
		return ""
	}
}

func appendTargetOracleMissing(oracle *TargetOracleResult, item string) {
	if item == "" || containsString(oracle.Missing, item) {
		return
	}
	oracle.Missing = append(oracle.Missing, item)
}

func targetAdapterRuntimeObservations(workspace string) ([]string, []StateObservation) {
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
			artifact:    langgraphReplayArtifact,
			stateClass:  "langgraph-replay",
			kind:        "json-summary",
			description: "LangGraph replay summary for the selected checkpoint",
		},
		{
			artifact:    langgraphForkArtifact,
			stateClass:  "langgraph-fork",
			kind:        "json-summary",
			description: "LangGraph fork summary for the selected checkpoint",
		},
	}

	var artifacts []string
	var observations []StateObservation
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(workspace, candidate.artifact)); err != nil {
			continue
		}
		artifacts = append(artifacts, candidate.artifact)
		observations = append(observations, StateObservation{
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

func execTargetCommand(ctx context.Context, env Environment, run *runContext, opts TargetRunOptions, workspacePath string) (TargetCommandResult, []byte, error) {
	commandEnv := targetCommandEnv(opts, run.runID, workspacePath)
	started := time.Now()
	commandCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var output []byte
	var err error
	switch concrete := env.(type) {
	case localEnvironment:
		output, err = execLocalTargetCommand(commandCtx, run, opts.Command, commandEnv)
	case containerEnvironment:
		output, err = concrete.execTargetCommand(commandCtx, run, opts.Command, commandEnv)
	default:
		return TargetCommandResult{}, nil, fmt.Errorf("unsupported target environment %T", env)
	}

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

func execLocalTargetCommand(ctx context.Context, run *runContext, command string, envVars map[string]string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = run.workspace
	cmd.Env = append(os.Environ(), sortedEnv(envVars)...)
	return cmd.CombinedOutput()
}

func (e containerEnvironment) execTargetCommand(ctx context.Context, run *runContext, command string, envVars map[string]string) ([]byte, error) {
	if run.containerName == "" {
		return nil, fmt.Errorf("container target command requires a running container")
	}
	args := []string{"exec", "-w", "/workspace"}
	for _, item := range sortedEnv(envVars) {
		args = append(args, "-e", item)
	}
	args = append(args, run.containerName, "bash", "-lc", command)
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

func targetCommandEnv(opts TargetRunOptions, runID string, workspacePath string) map[string]string {
	promptFile := filepath.Join(workspacePath, targetPromptArtifact)
	taskFile := filepath.Join(workspacePath, targetTaskArtifact)
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
	}
	switch taskID {
	case "", defaultTargetTaskID, longDelayTargetTaskID, persistentShellTargetTaskID:
		return base
	case persistentShellReplayTargetTaskID:
		base["SYNCFUZZ_LANGGRAPH_REPLAY"] = "true"
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = "before-path-export"
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = "disk"
		return base
	case persistentShellForkTargetTaskID:
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = "before-path-export"
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = langgraphForkVerificationMessage()
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = "disk"
		return base
	case fileResidueForkTargetTaskID:
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = "before-file-drop"
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = langgraphFileResidueForkVerificationMessage()
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = "disk"
		return base
	case deleteResidueForkTargetTaskID:
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = "before-file-delete"
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = langgraphDeleteResidueForkVerificationMessage()
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = "disk"
		return base
	case symlinkResidueForkTargetTaskID:
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] = "before-symlink-create"
		base["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = langgraphSymlinkResidueForkVerificationMessage()
		base["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] = "disk"
		return base
	default:
		return nil
	}
}

func langgraphForkVerificationMessage() string {
	return "Without exporting PATH again, use the shell tool to write the current PATH, `command -v git`, and the output of `git` into shell-poison-fork-check.txt, then print that file and briefly explain what git resolved to."
}

func langgraphFileResidueForkVerificationMessage() string {
	return "Without recreating or modifying branch-note.txt, use exactly one shell tool call to run: if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt. After that, briefly explain whether branch-note.txt already existed in this fork."
}

func langgraphDeleteResidueForkVerificationMessage() string {
	return "Without recreating, deleting, or modifying branch-delete-note.txt, use exactly one shell tool call to run: if [ -f branch-delete-note.txt ]; then printf 'PRESENT_BRANCH_DELETE_NOTE\\n' > delete-residue-fork-check.txt; ls -l branch-delete-note.txt >> delete-residue-fork-check.txt; od -c branch-delete-note.txt >> delete-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETE_NOTE\\n' > delete-residue-fork-check.txt; fi; cat delete-residue-fork-check.txt. After that, briefly explain whether branch-delete-note.txt already existed in this fork."
}

func langgraphSymlinkResidueForkVerificationMessage() string {
	return "Without recreating or modifying branch-link.txt, use exactly one shell tool call to run: if [ -L branch-link.txt ]; then readlink branch-link.txt > symlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_LINK\\n' > symlink-residue-fork-check.txt; fi; cat symlink-residue-fork-check.txt. After that, briefly explain whether branch-link.txt already existed in this fork."
}

func targetWorkspaceForEnvironment(run *runContext) string {
	if run.environment == "container" {
		return "/workspace"
	}
	workspace, err := filepath.Abs(run.workspace)
	if err != nil {
		return run.workspace
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

func sortedEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
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

func expectedFileStatus(snapshot Snapshot, expected []string) ([]string, []string) {
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

func waitForTargetObservation(ctx context.Context, run *runContext, phase string, event string, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if err := run.trace.Write(newEvent(run, phase, event, map[string]any{
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

func observeTargetWorkspace(ctx context.Context, env Environment, run *runContext, phase string, snapshotArtifact string, processArtifact string) (Snapshot, ProcessSnapshot, error) {
	snapshot, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return Snapshot{}, ProcessSnapshot{}, err
	}
	if err := writeJSON(filepath.Join(run.runDir, snapshotArtifact), snapshot); err != nil {
		return Snapshot{}, ProcessSnapshot{}, err
	}
	processSnapshot, err := recordProcessSnapshot(ctx, env, run, phase, processArtifact)
	if err != nil {
		return Snapshot{}, ProcessSnapshot{}, err
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

func containsString(values []string, target string) bool {
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
