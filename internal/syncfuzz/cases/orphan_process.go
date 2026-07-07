package cases

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

// runOrphanProcess models a local OS residue bug. The agent-visible command
// returns quickly, but a detached child process materializes a filesystem
// change after the lifecycle boundary.
func runOrphanProcess(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
	started := time.Now().UTC()
	env, err := environment.NewEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, opts, started, true)
	if err != nil {
		return nil, err
	}
	defer run.Close()

	if err := run.Trace.Write(core.NewEvent(run, "P0", "run_started", map[string]any{
		"environment":    run.Environment,
		"workspace":      run.Workspace,
		"delay":          run.Timing.RecoveryDelay,
		"run_role":       run.RunRole,
		"timing_profile": run.Timing.ProfileID,
		"primitive_id":   run.PrimitiveID,
	})); err != nil {
		return nil, err
	}
	if err := core.RecordFaultPlan(run); err != nil {
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

	childDelay, err := run.Timing.OrphanChildDelayDuration()
	if err != nil {
		return nil, err
	}
	childSleep := core.ShellSleepDuration(childDelay)
	primitiveID := orphanProcessPrimitiveID(opts)
	command, primitiveDescription := orphanProcessCommand(primitiveID, childSleep, isControlRun(opts))
	if command == "" {
		return nil, fmt.Errorf("unsupported orphan-process primitive %q", primitiveID)
	}
	if isControlRun(opts) {
		primitiveDescription += " (control cleanup path)"
	}
	if err := run.Trace.Write(core.NewEvent(run, "P1", "tool_intent", map[string]any{
		"command":      command,
		"primitive_id": primitiveID,
		"primitive":    primitiveDescription,
	})); err != nil {
		return nil, err
	}

	commandResult, err := env.ExecShell(ctx, run, command)
	if err != nil {
		return nil, fmt.Errorf("execute testcase command: %w", err)
	}

	if err := run.Trace.Write(core.NewEvent(run, "P5", "command_returned", map[string]any{
		"stdout_stderr": commandResult.StdoutStderr,
	})); err != nil {
		return nil, err
	}
	processAfterCommand, err := core.RecordProcessSnapshot(ctx, env, run, "P5", "process-after-command.json")
	if err != nil {
		return nil, err
	}
	boundaryDescription := "agent would treat the tool call as complete before delayed child effect materializes"
	if isControlRun(opts) {
		boundaryDescription = "control run waits for the child effect and cleanup before the lifecycle boundary"
	}
	if err := run.Trace.Write(core.NewEvent(run, "P6", "simulated_agent_boundary", map[string]any{
		"description": boundaryDescription,
	})); err != nil {
		return nil, err
	}

	// Wait long enough for the delayed child effect to appear, then compare
	// before/after filesystem snapshots as a rollback-residue oracle.
	recoveryDelay, err := run.Timing.RecoveryDelayDuration()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(recoveryDelay):
	}

	after, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}
	processAfter, err := core.RecordProcessSnapshot(ctx, env, run, "P6", "process-after.json")
	if err != nil {
		return nil, err
	}
	if _, err := core.RecordProcessLineage(run, "P6", "process-lineage.json", processBefore, processAfterCommand, processAfter, "process-before.json", "process-after-command.json", "process-after.json"); err != nil {
		return nil, err
	}
	if _, err := core.RecordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []core.FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
		return nil, err
	}

	confirmed, evidence := orphanProcessOracle(before, after)
	signature := core.MismatchSignature{
		LifecycleEvent: "cancel-recover",
		FaultPhase:     "after-command-return",
		StateClass:     "filesystem",
		Operation:      "delayed-write",
		Relation:       "agent-forgets-os-effect",
		Impact:         "rollback-residue",
	}
	manifest := core.CaseManifest{
		Objective:         "Detect delayed filesystem residue after an agent-visible command has returned.",
		StateClasses:      []string{"filesystem", "process"},
		FaultPhases:       []string{"P5 command returned", "P6 simulated agent boundary"},
		Primitives:        []string{"detached child process", "delayed file write"},
		ExpectedSignature: signature,
		Artifacts:         core.AppendPhase2Artifacts([]string{"trace.jsonl", "snapshot-before.json", "process-before.json", "process-after-command.json", "snapshot-after.json", "process-after.json", "process-lineage.json", "filesystem-metadata.json", "result.json"}),
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, confirmed, evidence, []core.StateObservation{
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P5", Artifact: "process-after-command.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P6", Artifact: "snapshot-after.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-after.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P6", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
	}); err != nil {
		return nil, err
	}
	if err := core.WriteManifest(run, manifest); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &core.RunResult{
		RunID:           run.RunID,
		CaseName:        opts.CaseName,
		RunRole:         run.RunRole,
		Environment:     run.Environment,
		ContainerImage:  run.ContainerImage,
		FaultPlanID:     run.FaultPlan.ID,
		PrimitiveID:     run.PrimitiveID,
		TimingProfileID: run.Timing.ProfileID,
		Confirmed:       confirmed,
		Signature:       signature,
		Evidence:        evidence,
		ArtifactDir:     run.RunDir,
		StartedAt:       started.Format(time.RFC3339Nano),
		FinishedAt:      finished.Format(time.RFC3339Nano),
	}

	if err := run.Trace.Write(core.NewEvent(run, "oracle", "result", map[string]any{
		"confirmed":    confirmed,
		"signature":    signature.String(),
		"evidence":     evidence,
		"primitive_id": run.PrimitiveID,
	})); err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "result.json"), result); err != nil {
		return nil, err
	}

	return result, nil
}

func orphanProcessPrimitiveID(opts core.RunOptions) string {
	switch opts.PrimitiveID {
	case "", "delayed-write":
		return "delayed-write"
	default:
		return opts.PrimitiveID
	}
}

func orphanProcessCommand(primitiveID string, childSleep string, control bool) (string, string) {
	if control {
		return "sh -c 'sleep " + childSleep + "; touch late-effect; rm late-effect'", "wait for delayed write then remove it before the lifecycle boundary"
	}
	switch primitiveID {
	case "background-process", "delayed-write":
		return "nohup sh -c 'sleep " + childSleep + "; touch late-effect' >/dev/null 2>&1 &", "background child writes late-effect after command return"
	case "double-fork-daemon":
		return "setsid sh -c 'sleep " + childSleep + "; touch late-effect; sleep 5' >/dev/null 2>&1 &", "daemonized child writes late-effect and remains alive after recovery"
	default:
		return "", ""
	}
}
