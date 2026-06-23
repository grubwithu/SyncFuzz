package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type runContext struct {
	runID          string
	caseName       string
	runDir         string
	workspace      string
	environment    string
	containerName  string
	containerImage string
	turnID         string
	toolCallID     string
	trace          *eventWriter
	cleanup        func() error
}

func (r *runContext) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	if r.trace != nil {
		closeErr = r.trace.Close()
	}
	if r.cleanup != nil {
		if err := r.cleanup(); closeErr == nil && err != nil {
			closeErr = err
		}
	}
	return closeErr
}

// runOrphanProcess models a local OS residue bug. The agent-visible command
// returns quickly, but a detached child process materializes a filesystem
// change after the lifecycle boundary.
func runOrphanProcess(ctx context.Context, opts RunOptions) (*RunResult, error) {
	started := time.Now().UTC()
	env, err := newEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, opts, started, true)
	if err != nil {
		return nil, err
	}
	defer run.Close()

	if err := run.trace.Write(newEvent(run, "P0", "run_started", map[string]any{
		"environment": run.environment,
		"workspace":   run.workspace,
		"delay":       opts.Delay.String(),
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
	if _, err := recordProcessSnapshot(ctx, env, run, "P0", "process-before.json"); err != nil {
		return nil, err
	}

	// This is the state primitive: the parent shell exits before the child
	// writes late-effect. A real agent might checkpoint/cancel/replay here.
	command := "nohup sh -c 'sleep 1; touch late-effect' >/dev/null 2>&1 &"
	if err := run.trace.Write(newEvent(run, "P1", "tool_intent", map[string]any{
		"command": command,
	})); err != nil {
		return nil, err
	}

	commandResult, err := env.ExecShell(ctx, run, command)
	if err != nil {
		return nil, fmt.Errorf("execute testcase command: %w", err)
	}

	if err := run.trace.Write(newEvent(run, "P5", "command_returned", map[string]any{
		"stdout_stderr": commandResult.StdoutStderr,
	})); err != nil {
		return nil, err
	}
	if _, err := recordProcessSnapshot(ctx, env, run, "P5", "process-after-command.json"); err != nil {
		return nil, err
	}
	if err := run.trace.Write(newEvent(run, "P6", "simulated_agent_boundary", map[string]any{
		"description": "agent would treat the tool call as complete before delayed child effect materializes",
	})); err != nil {
		return nil, err
	}

	// Wait long enough for the delayed child effect to appear, then compare
	// before/after filesystem snapshots as a rollback-residue oracle.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(opts.Delay):
	}

	after, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}
	if _, err := recordProcessSnapshot(ctx, env, run, "P6", "process-after.json"); err != nil {
		return nil, err
	}

	confirmed, evidence := orphanProcessOracle(before, after)
	signature := MismatchSignature{
		LifecycleEvent: "cancel-recover",
		FaultPhase:     "after-command-return",
		StateClass:     "filesystem",
		Operation:      "delayed-write",
		Relation:       "agent-forgets-os-effect",
		Impact:         "rollback-residue",
	}
	if err := writeManifest(run, CaseManifest{
		Objective:         "Detect delayed filesystem residue after an agent-visible command has returned.",
		StateClasses:      []string{"filesystem", "process"},
		FaultPhases:       []string{"P5 command returned", "P6 simulated agent boundary"},
		Primitives:        []string{"detached child process", "delayed file write"},
		ExpectedSignature: signature,
		Artifacts:         []string{"trace.jsonl", "snapshot-before.json", "process-before.json", "process-after-command.json", "snapshot-after.json", "process-after.json", "result.json"},
	}); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &RunResult{
		RunID:          run.runID,
		CaseName:       opts.CaseName,
		Environment:    run.environment,
		ContainerImage: run.containerImage,
		Confirmed:      confirmed,
		Signature:      signature,
		Evidence:       evidence,
		ArtifactDir:    run.runDir,
		StartedAt:      started.Format(time.RFC3339Nano),
		FinishedAt:     finished.Format(time.RFC3339Nano),
	}

	if err := run.trace.Write(newEvent(run, "oracle", "result", map[string]any{
		"confirmed": confirmed,
		"signature": signature.String(),
		"evidence":  evidence,
	})); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "result.json"), result); err != nil {
		return nil, err
	}

	return result, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func recordProcessSnapshot(ctx context.Context, env Environment, run *runContext, phase string, artifact string) (ProcessSnapshot, error) {
	snapshot, err := env.SnapshotProcesses(ctx, run)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	if err := writeJSON(filepath.Join(run.runDir, artifact), snapshot); err != nil {
		return ProcessSnapshot{}, err
	}
	if err := run.trace.Write(newEvent(run, phase, "process_snapshot", map[string]any{
		"artifact":                artifact,
		"process_count":           len(snapshot.Processes),
		"workspace_process_count": countWorkspaceProcesses(snapshot.Processes),
		"environment":             snapshot.Environment,
		"container_name":          snapshot.ContainerName,
		"container_image":         snapshot.ContainerImage,
	})); err != nil {
		return ProcessSnapshot{}, err
	}
	return snapshot, nil
}

func countWorkspaceProcesses(processes []ProcessEntry) int {
	count := 0
	for _, process := range processes {
		if process.WorkspaceRelated {
			count++
		}
	}
	return count
}
