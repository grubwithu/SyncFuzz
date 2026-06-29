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
	targetSnapshotLateArtifact        = "snapshot-late.json"
	targetProcessLateArtifact         = "process-late.json"
	targetFilesystemLateArtifact      = "filesystem-late-metadata.json"
	defaultTargetAdapterID            = "command"
	defaultTargetTaskID               = "orphan-process"
	longDelayTargetTaskID             = "orphan-process-long-delay"
	longDelayTargetLateEffectArtifact = "late-effect"
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
	Name      string   `json:"name"`
	Confirmed bool     `json:"confirmed"`
	Evidence  []string `json:"evidence,omitempty"`
	Missing   []string `json:"missing,omitempty"`
}

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

	if err := run.trace.Write(newEvent(run, "P0", "target_run_started", map[string]any{
		"adapter_id":         opts.AdapterID,
		"target_id":          opts.TargetID,
		"task_id":            opts.TaskID,
		"environment":        run.environment,
		"container_image":    run.containerImage,
		"workspace":          targetWorkspaceForEnvironment(run),
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
		Workspace:          targetWorkspaceForEnvironment(run),
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

	commandResult, output, err := execTargetCommand(ctx, env, run, opts)
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

	if opts.ObserveDelay > 0 {
		if err := run.trace.Write(newEvent(run, "P6", "target_observation_delay", map[string]any{
			"delay": opts.ObserveDelay.String(),
		})); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(opts.ObserveDelay):
		}
	}

	after, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}
	processAfter, err := recordProcessSnapshot(ctx, env, run, "P6", "process-after.json")
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
		if err := run.trace.Write(newEvent(run, "P7", "target_late_observation_delay", map[string]any{
			"delay": opts.LateObserveDelay.String(),
		})); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(opts.LateObserveDelay):
		}

		late, err = SnapshotFilesystem(run.workspace)
		if err != nil {
			return nil, err
		}
		if err := writeJSON(filepath.Join(run.runDir, targetSnapshotLateArtifact), late); err != nil {
			return nil, err
		}
		if _, err := recordProcessSnapshot(ctx, env, run, "P7", targetProcessLateArtifact); err != nil {
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
	targetOracle := evaluateTargetOracle(opts.TaskID, completed, missing, processLineage.Summary, lateObserved, latePresent, lateMissing)
	expectationsMet := targetOracle.Confirmed
	signature := MismatchSignature{
		LifecycleEvent: "real-target-run",
		FaultPhase:     "target-command",
		StateClass:     "workspace",
		Operation:      opts.TaskID,
		Relation:       "observation-only",
		Impact:         "target-adapter",
	}
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
		ArtifactDir:              run.runDir,
		Workspace:                targetWorkspaceForEnvironment(run),
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
	default:
		return "You are running inside a SyncFuzz workspace. Complete the requested task in the current working directory and leave observable artifacts."
	}
}

func defaultTargetExpectedFiles(taskID string) []string {
	switch taskID {
	case "", defaultTargetTaskID:
		return []string{"late-effect"}
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

func evaluateTargetOracle(taskID string, completed bool, immediateMissing []string, lineage ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	switch taskID {
	case longDelayTargetTaskID:
		return evaluateLongDelayTargetOracle(completed, lineage, lateObserved, latePresent, lateMissing)
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

func execTargetCommand(ctx context.Context, env Environment, run *runContext, opts TargetRunOptions) (TargetCommandResult, []byte, error) {
	commandEnv := targetCommandEnv(run, opts)
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

func targetCommandEnv(run *runContext, opts TargetRunOptions) map[string]string {
	promptFile := targetWorkspaceArtifactPath(run, targetPromptArtifact)
	taskFile := targetWorkspaceArtifactPath(run, targetTaskArtifact)
	return map[string]string{
		"SYNCFUZZ_ADAPTER_ID":  opts.AdapterID,
		"SYNCFUZZ_TARGET_ID":   opts.TargetID,
		"SYNCFUZZ_TASK_ID":     opts.TaskID,
		"SYNCFUZZ_RUN_ID":      run.runID,
		"SYNCFUZZ_REPO_ROOT":   targetRepoRoot(),
		"SYNCFUZZ_WORKSPACE":   targetWorkspaceForEnvironment(run),
		"SYNCFUZZ_PROMPT":      opts.Prompt,
		"SYNCFUZZ_PROMPT_FILE": promptFile,
		"SYNCFUZZ_TASK_FILE":   taskFile,
	}
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

func targetWorkspaceArtifactPath(run *runContext, artifact string) string {
	return filepath.Join(targetWorkspaceForEnvironment(run), artifact)
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
