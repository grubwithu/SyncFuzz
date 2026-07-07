package cases

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func runPartialFilesystemRollback(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
	started := time.Now().UTC()
	artifacts := []string{
		"trace.jsonl",
		"snapshot-before.json",
		"process-before.json",
		"snapshot-mutated.json",
		"process-mutated.json",
		"snapshot-after.json",
		"process-after.json",
		"process-lineage.json",
		"filesystem-metadata.json",
		"result.json",
	}
	env, err := environment.NewEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, opts, started, true)
	if err != nil {
		return nil, err
	}
	defer run.Close()
	primitiveID := partialFilesystemRollbackPrimitiveID(opts)

	if err := run.Trace.Write(core.NewEvent(run, "P0", "run_started", map[string]any{
		"environment":    run.Environment,
		"workspace":      run.Workspace,
		"run_role":       run.RunRole,
		"timing_profile": run.Timing.ProfileID,
		"primitive_id":   primitiveID,
	})); err != nil {
		return nil, err
	}
	if err := core.RecordFaultPlan(run); err != nil {
		return nil, err
	}

	if _, err := env.ExecShell(ctx, run, "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt"); err != nil {
		return nil, fmt.Errorf("create baseline tracked file: %w", err)
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

	if err := run.Trace.Write(core.NewEvent(run, "P1", "tool_intent", map[string]any{
		"operation":    "mutate_filesystem",
		"primitive_id": primitiveID,
		"primitives":   partialFilesystemRollbackTracePrimitives(primitiveID),
	})); err != nil {
		return nil, err
	}

	if err := mutateFilesystemForRollback(ctx, env, run, primitiveID); err != nil {
		return nil, err
	}
	if err := run.Trace.Write(core.NewEvent(run, "P4", "filesystem_mutated", map[string]any{
		"primitive_id": primitiveID,
		"paths":        partialFilesystemRollbackMutationPaths(primitiveID),
	})); err != nil {
		return nil, err
	}

	mutated, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-mutated.json"), mutated); err != nil {
		return nil, err
	}
	processMutated, err := core.RecordProcessSnapshot(ctx, env, run, "P4", "process-mutated.json")
	if err != nil {
		return nil, err
	}

	if isControlRun(opts) {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "control_full_rollback", map[string]any{
			"primitive_id": primitiveID,
			"description":  partialFilesystemRollbackControlDescription(primitiveID),
		})); err != nil {
			return nil, err
		}
		if _, err := env.ExecShell(ctx, run, partialFilesystemRollbackControlScript(run, primitiveID)); err != nil {
			return nil, fmt.Errorf("perform full rollback: %w", err)
		}
	} else {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "fault_injected", map[string]any{
			"primitive_id": primitiveID,
			"fault":        "naive_tracked_content_rollback",
			"description":  partialFilesystemRollbackFaultDescription(primitiveID),
		})); err != nil {
			return nil, err
		}

		if _, err := env.ExecShell(ctx, run, partialFilesystemRollbackFaultScript(run, primitiveID)); err != nil {
			return nil, fmt.Errorf("perform naive rollback: %w", err)
		}
	}
	if err := core.WaitForTimingBoundary(ctx, run, "P6", "after_rollback_boundary"); err != nil {
		return nil, err
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
	if _, err := core.RecordProcessLineage(run, "P6", "process-lineage.json", processBefore, processMutated, processAfter, "process-before.json", "process-mutated.json", "process-after.json"); err != nil {
		return nil, err
	}
	if _, err := core.RecordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []core.FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P4", Artifact: "snapshot-mutated.json", Snapshot: mutated},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
		return nil, err
	}

	confirmed, evidence := partialFilesystemRollbackOracleForPrimitive(before, after, processAfter, run.Workspace, primitiveID)
	signature := partialFilesystemRollbackSignature(primitiveID)
	manifest := core.CaseManifest{
		Objective:         partialFilesystemRollbackObjective(primitiveID),
		StateClasses:      partialFilesystemRollbackStateClasses(primitiveID),
		FaultPhases:       partialFilesystemRollbackFaultPhases(primitiveID),
		Primitives:        partialFilesystemRollbackManifestPrimitives(primitiveID),
		ExpectedSignature: signature,
		Artifacts:         core.AppendPhase2Artifacts(artifacts),
	}
	observations := []core.StateObservation{
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P4", Artifact: "snapshot-mutated.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P4", Artifact: "process-mutated.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P6", Artifact: "snapshot-after.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-after.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P6", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P6", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
	}
	if primitiveID == "open-fd" {
		observations = append(observations,
			core.StateObservation{Layer: "os", StateClass: "fd", Phase: "P4", Artifact: "process-mutated.json", Kind: "process-snapshot", Description: "process snapshot with workspace-related open file descriptors after unlink"},
			core.StateObservation{Layer: "os", StateClass: "fd", Phase: "P6", Artifact: "process-after.json", Kind: "process-snapshot", Description: "post-rollback process snapshot showing deleted workspace file descriptors"},
			core.StateObservation{Layer: "os", StateClass: "fd", Phase: "P6", Artifact: "process-lineage.json", Kind: "process-lineage", Description: "process lineage for deleted workspace file descriptor residue"},
		)
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, confirmed, evidence, observations); err != nil {
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
		"confirmed": confirmed,
		"signature": signature.String(),
		"evidence":  evidence,
	})); err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "result.json"), result); err != nil {
		return nil, err
	}

	return result, nil
}

func mutateFilesystemForRollback(ctx context.Context, env core.Environment, run *core.RunContext, primitiveID string) error {
	script := partialFilesystemRollbackMutationScript(run, primitiveID)
	if _, err := env.ExecShell(ctx, run, script); err != nil {
		return fmt.Errorf("mutate filesystem for rollback: %w", err)
	}
	return nil
}

func partialFilesystemRollbackPrimitiveID(opts core.RunOptions) string {
	return opts.PrimitiveID
}

func partialFilesystemRollbackTracePrimitives(primitiveID string) []string {
	switch primitiveID {
	case "untracked-file":
		return []string{"tracked content modification", "untracked file"}
	case "symlink":
		return []string{"tracked content modification", "symlink"}
	case "chmod-xattr":
		return []string{"tracked content modification", "permission drift"}
	case "open-fd":
		return []string{"tracked content replacement", "open file descriptor", "unlink tracked path"}
	default:
		return []string{
			"tracked content modification",
			"untracked file",
			"symlink",
			"permission drift",
		}
	}
}

func partialFilesystemRollbackMutationPaths(primitiveID string) []string {
	switch primitiveID {
	case "untracked-file":
		return []string{"tracked.txt", "untracked.txt"}
	case "symlink":
		return []string{"tracked.txt", "link-to-tracked"}
	case "chmod-xattr":
		return []string{"tracked.txt"}
	case "open-fd":
		return []string{"tracked.txt"}
	default:
		return []string{"tracked.txt", "untracked.txt", "link-to-tracked"}
	}
}

func partialFilesystemRollbackControlDescription(primitiveID string) string {
	switch primitiveID {
	case "untracked-file":
		return "control rollback restores tracked content and removes the untracked residue"
	case "symlink":
		return "control rollback restores tracked content and removes the symlink residue"
	case "chmod-xattr":
		return "control rollback restores tracked content and resets tracked file mode"
	case "open-fd":
		return "control rollback recreates tracked.txt after terminating the process that held the deleted inode open"
	default:
		return "control rollback restores tracked content, mode, symlink, and untracked files"
	}
}

func partialFilesystemRollbackFaultDescription(primitiveID string) string {
	switch primitiveID {
	case "untracked-file":
		return "rollback restores tracked content but leaves the untracked file residue behind"
	case "symlink":
		return "rollback restores tracked content but leaves the symlink residue behind"
	case "chmod-xattr":
		return "rollback restores tracked content but misses tracked file mode drift"
	case "open-fd":
		return "rollback recreates tracked.txt but does not revoke the deleted workspace inode still held through an open file descriptor"
	default:
		return "rollback restores tracked content but misses untracked artifacts and metadata drift"
	}
}

func partialFilesystemRollbackObjective(primitiveID string) string {
	switch primitiveID {
	case "open-fd":
		return "Detect deleted workspace capabilities that survive a rollback which recreates the tracked path."
	default:
		return "Detect filesystem state classes that survive a rollback which only restores tracked file contents."
	}
}

func partialFilesystemRollbackStateClasses(primitiveID string) []string {
	switch primitiveID {
	case "open-fd":
		return []string{"filesystem", "process", "fd"}
	default:
		return []string{"filesystem", "process"}
	}
}

func partialFilesystemRollbackFaultPhases(primitiveID string) []string {
	switch primitiveID {
	case "open-fd":
		return []string{"P4 tracked file opened and unlinked", "P6 tracked path recreated without revoking the deleted fd", "oracle after process snapshot"}
	default:
		return []string{"P4 filesystem mutated", "P6 naive rollback", "oracle after snapshot"}
	}
}

func partialFilesystemRollbackManifestPrimitives(primitiveID string) []string {
	switch primitiveID {
	case "untracked-file":
		return []string{"tracked content restore", "untracked file"}
	case "symlink":
		return []string{"tracked content restore", "symlink"}
	case "chmod-xattr":
		return []string{"tracked content restore", "permission drift"}
	case "open-fd":
		return []string{"tracked content replacement", "open file descriptor", "unlink tracked path"}
	default:
		return []string{"tracked content restore", "untracked file", "symlink", "permission drift"}
	}
}

func partialFilesystemRollbackSignature(primitiveID string) core.MismatchSignature {
	stateClass := "filesystem"
	operation := "partial-restore"
	if primitiveID == "open-fd" {
		stateClass = "fd"
		operation = "deleted-open-fd"
	}
	return core.MismatchSignature{
		LifecycleEvent: "rollback",
		FaultPhase:     "after-naive-filesystem-restore",
		StateClass:     stateClass,
		Operation:      operation,
		Relation:       "unsupported-state-residue",
		Impact:         "partial-filesystem-rollback",
	}
}

func partialFilesystemRollbackOracleForPrimitive(before core.Snapshot, after core.Snapshot, processAfter core.ProcessSnapshot, workspace string, primitiveID string) (bool, []string) {
	if primitiveID == "open-fd" {
		return partialFilesystemRollbackFDOracle(after, processAfter, workspace)
	}
	return partialFilesystemRollbackOracle(before, after)
}

func partialFilesystemRollbackMutationScript(run *core.RunContext, primitiveID string) string {
	switch primitiveID {
	case "untracked-file":
		return `printf 'mutated\n' > tracked.txt
printf 'residue\n' > untracked.txt`
	case "symlink":
		return `printf 'mutated\n' > tracked.txt
ln -s tracked.txt link-to-tracked`
	case "chmod-xattr":
		return `printf 'mutated\n' > tracked.txt
chmod 755 tracked.txt
`
	case "open-fd":
		holder := `nohup sh -c 'exec 9<tracked.txt; rm -f tracked.txt; sleep 2' >/dev/null 2>&1 &`
		waitForDelete := `for _ in 1 2 3 4 5 6 7 8 9 10; do
[ ! -e tracked.txt ] && break
sleep 0.05
done`
		if isControlRun(core.RunOptions{RunRole: run.RunRole}) {
			return fmt.Sprintf("printf 'mutated\\n' > tracked.txt\n%s\nprintf '%%s\\n' \"$!\" > %s\n%s",
				holder,
				core.ShellQuote(partialFilesystemRollbackPIDFile(run)),
				waitForDelete,
			)
		}
		return "printf 'mutated\\n' > tracked.txt\n" + holder + "\n" + waitForDelete
	default:
		return `printf 'mutated\n' > tracked.txt
chmod 755 tracked.txt
printf 'residue\n' > untracked.txt
ln -s tracked.txt link-to-tracked`
	}
}

func partialFilesystemRollbackControlScript(run *core.RunContext, primitiveID string) string {
	switch primitiveID {
	case "untracked-file":
		return "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt\nrm -f untracked.txt"
	case "symlink":
		return "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt\nrm -f link-to-tracked"
	case "chmod-xattr":
		return "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt"
	case "open-fd":
		pidFile := core.ShellQuote(partialFilesystemRollbackPIDFile(run))
		return fmt.Sprintf(`if [ -f %s ]; then
  pid=$(cat %s 2>/dev/null || true)
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    for _ in 1 2 3 4 5; do
      [ ! -d "/proc/$pid" ] && break
      sleep 0.05
    done
  fi
  rm -f %s
fi
printf 'original\n' > tracked.txt
chmod 644 tracked.txt`, pidFile, pidFile, pidFile)
	default:
		return "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt\nrm -f untracked.txt link-to-tracked"
	}
}

func partialFilesystemRollbackFaultScript(_ *core.RunContext, primitiveID string) string {
	switch primitiveID {
	case "open-fd":
		return "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt"
	default:
		return "printf 'original\\n' > tracked.txt"
	}
}

func partialFilesystemRollbackPIDFile(run *core.RunContext) string {
	return filepath.Join("/tmp", "syncfuzz-open-fd-"+run.RunID+".pid")
}
