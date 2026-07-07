package core

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// RunContext is the mutable per-run state shared across phases. It owns the
// JSONL event trace and the container teardown hook.
type RunContext struct {
	RunID          string
	CaseName       string
	RunRole        string
	RunDir         string
	Workspace      string
	Environment    string
	ContainerName  string
	ContainerImage string
	PrimitiveID    string
	FaultPlan      FaultPlan
	Timing         FaultTiming
	TurnID         string
	ToolCallID     string
	Trace          *EventWriter
	Cleanup        func() error
}

func (r *RunContext) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	if r.Trace != nil {
		closeErr = r.Trace.Close()
	}
	if r.Cleanup != nil {
		if err := r.Cleanup(); closeErr == nil && err != nil {
			closeErr = err
		}
	}
	return closeErr
}

func RecordFaultPlan(run *RunContext) error {
	if run.FaultPlan.ID == "" {
		return nil
	}
	if err := WriteJSON(filepath.Join(run.RunDir, FaultPlanArtifact), run.FaultPlan); err != nil {
		return err
	}
	return run.Trace.Write(NewEvent(run, "P0", "fault_plan_selected", map[string]any{
		"artifact":        FaultPlanArtifact,
		"fault_plan_id":   run.FaultPlan.ID,
		"primitive_id":    run.PrimitiveID,
		"run_role":        run.RunRole,
		"timing_profile":  run.Timing.ProfileID,
		"inject_phase":    run.FaultPlan.InjectPhase,
		"fault":           run.FaultPlan.Fault,
		"lifecycle":       run.FaultPlan.Lifecycle,
		"expected_impact": run.FaultPlan.ExpectedImpact,
	}))
}

func WaitForTimingBoundary(ctx context.Context, run *RunContext, phase string, boundary string) error {
	delay, err := run.Timing.replayDelayDuration()
	if err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	if err := run.Trace.Write(NewEvent(run, phase, "timing_delay", map[string]any{
		"boundary":       boundary,
		"timing_profile": run.Timing.ProfileID,
		"delay":          delay.String(),
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

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func RecordProcessSnapshot(ctx context.Context, env Environment, run *RunContext, phase string, artifact string) (ProcessSnapshot, error) {
	snapshot, err := env.SnapshotProcesses(ctx, run)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	if err := WriteJSON(filepath.Join(run.RunDir, artifact), snapshot); err != nil {
		return ProcessSnapshot{}, err
	}
	if err := run.Trace.Write(NewEvent(run, phase, "process_snapshot", map[string]any{
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

func RecordProcessLineage(run *RunContext, phase string, artifact string, before ProcessSnapshot, boundary ProcessSnapshot, after ProcessSnapshot, beforeArtifact string, boundaryArtifact string, afterArtifact string) (ProcessLineageReport, error) {
	report := AnalyzeProcessLineage(before, boundary, after, beforeArtifact, boundaryArtifact, afterArtifact)
	if err := WriteJSON(filepath.Join(run.RunDir, artifact), report); err != nil {
		return ProcessLineageReport{}, err
	}
	if err := run.Trace.Write(NewEvent(run, phase, "process_lineage", map[string]any{
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

func RecordFilesystemMetadata(run *RunContext, phase string, artifact string, snapshots []FilesystemSnapshotArtifact) (FilesystemMetadataReport, error) {
	report := AnalyzeFilesystemMetadata(snapshots)
	if err := WriteJSON(filepath.Join(run.RunDir, artifact), report); err != nil {
		return FilesystemMetadataReport{}, err
	}
	added, removed, contentChanged, metadataChanged := filesystemDeltaCounts(report.Deltas)
	if err := run.Trace.Write(NewEvent(run, phase, "filesystem_metadata", map[string]any{
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
