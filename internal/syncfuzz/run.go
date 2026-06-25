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
	faultPlan      FaultPlan
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
	if err := recordFaultPlan(run); err != nil {
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
	processAfterCommand, err := recordProcessSnapshot(ctx, env, run, "P5", "process-after-command.json")
	if err != nil {
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
	processAfter, err := recordProcessSnapshot(ctx, env, run, "P6", "process-after.json")
	if err != nil {
		return nil, err
	}
	if _, err := recordProcessLineage(run, "P6", "process-lineage.json", processBefore, processAfterCommand, processAfter, "process-before.json", "process-after-command.json", "process-after.json"); err != nil {
		return nil, err
	}
	if _, err := recordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
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
	manifest := CaseManifest{
		Objective:         "Detect delayed filesystem residue after an agent-visible command has returned.",
		StateClasses:      []string{"filesystem", "process"},
		FaultPhases:       []string{"P5 command returned", "P6 simulated agent boundary"},
		Primitives:        []string{"detached child process", "delayed file write"},
		ExpectedSignature: signature,
		Artifacts:         appendPhase2Artifacts([]string{"trace.jsonl", "snapshot-before.json", "process-before.json", "process-after-command.json", "snapshot-after.json", "process-after.json", "process-lineage.json", "filesystem-metadata.json", "result.json"}),
	}
	if err := writeCrossLayerArtifacts(run, manifest, confirmed, evidence, []StateObservation{
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
	if err := writeManifest(run, manifest); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &RunResult{
		RunID:          run.runID,
		CaseName:       opts.CaseName,
		Environment:    run.environment,
		ContainerImage: run.containerImage,
		FaultPlanID:    run.faultPlan.ID,
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

func recordFaultPlan(run *runContext) error {
	if run.faultPlan.ID == "" {
		return nil
	}
	if err := writeJSON(filepath.Join(run.runDir, faultPlanArtifact), run.faultPlan); err != nil {
		return err
	}
	return run.trace.Write(newEvent(run, "P0", "fault_plan_selected", map[string]any{
		"artifact":        faultPlanArtifact,
		"fault_plan_id":   run.faultPlan.ID,
		"inject_phase":    run.faultPlan.InjectPhase,
		"fault":           run.faultPlan.Fault,
		"lifecycle":       run.faultPlan.Lifecycle,
		"expected_impact": run.faultPlan.ExpectedImpact,
	}))
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

func recordProcessLineage(run *runContext, phase string, artifact string, before ProcessSnapshot, boundary ProcessSnapshot, after ProcessSnapshot, beforeArtifact string, boundaryArtifact string, afterArtifact string) (ProcessLineageReport, error) {
	report := AnalyzeProcessLineage(before, boundary, after, beforeArtifact, boundaryArtifact, afterArtifact)
	if err := writeJSON(filepath.Join(run.runDir, artifact), report); err != nil {
		return ProcessLineageReport{}, err
	}
	if err := run.trace.Write(newEvent(run, phase, "process_lineage", map[string]any{
		"artifact":                           artifact,
		"before_artifact":                    beforeArtifact,
		"boundary_artifact":                  boundaryArtifact,
		"after_artifact":                     afterArtifact,
		"new_at_boundary":                    report.Summary.NewAtBoundary,
		"remaining_after":                    report.Summary.RemainingAfter,
		"exited_after":                       report.Summary.ExitedAfter,
		"carried_over_after":                 report.Summary.CarriedOverAfter,
		"workspace_new_at_boundary":          report.Summary.WorkspaceNewAtBoundary,
		"workspace_remaining_after":          report.Summary.WorkspaceRemainingAfter,
		"workspace_carried_over_after":       report.Summary.WorkspaceCarriedOverAfter,
		"workspace_carried_over_at_boundary": report.Summary.WorkspaceCarriedOverAtBoundary,
		"environment":                        report.Environment,
		"container_name":                     report.ContainerName,
		"container_image":                    report.ContainerImage,
	})); err != nil {
		return ProcessLineageReport{}, err
	}
	return report, nil
}

func recordFilesystemMetadata(run *runContext, phase string, artifact string, snapshots []FilesystemSnapshotArtifact) (FilesystemMetadataReport, error) {
	report := AnalyzeFilesystemMetadata(snapshots)
	if err := writeJSON(filepath.Join(run.runDir, artifact), report); err != nil {
		return FilesystemMetadataReport{}, err
	}
	added, removed, contentChanged, metadataChanged := filesystemDeltaCounts(report.Deltas)
	if err := run.trace.Write(newEvent(run, phase, "filesystem_metadata", map[string]any{
		"artifact":                artifact,
		"snapshot_count":          len(report.Snapshots),
		"delta_count":             len(report.Deltas),
		"added_paths":             added,
		"removed_paths":           removed,
		"content_changed_paths":   contentChanged,
		"metadata_changed_fields": metadataChanged,
	})); err != nil {
		return FilesystemMetadataReport{}, err
	}
	return report, nil
}

func filesystemDeltaCounts(deltas []FilesystemMetadataDelta) (int, int, int, int) {
	var added int
	var removed int
	var contentChanged int
	var metadataChanged int
	for _, delta := range deltas {
		added += len(delta.Added)
		removed += len(delta.Removed)
		contentChanged += len(delta.ContentChanged)
		metadataChanged += len(delta.MetadataChanged)
	}
	return added, removed, contentChanged, metadataChanged
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
