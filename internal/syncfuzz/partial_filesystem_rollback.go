package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func runPartialFilesystemRollback(ctx context.Context, opts RunOptions) (*RunResult, error) {
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
	})); err != nil {
		return nil, err
	}

	trackedPath := filepath.Join(run.workspace, "tracked.txt")
	if err := os.WriteFile(trackedPath, []byte("original\n"), 0o644); err != nil {
		return nil, fmt.Errorf("create baseline tracked file: %w", err)
	}

	before, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-before.json"), before); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P1", "tool_intent", map[string]any{
		"operation": "mutate_filesystem",
		"primitives": []string{
			"tracked content modification",
			"untracked file",
			"symlink",
			"permission drift",
		},
	})); err != nil {
		return nil, err
	}

	if err := mutateFilesystemForRollback(run.workspace); err != nil {
		return nil, err
	}
	if err := run.trace.Write(newEvent(run, "P4", "filesystem_mutated", map[string]any{
		"paths": []string{"tracked.txt", "untracked.txt", "link-to-tracked"},
	})); err != nil {
		return nil, err
	}

	mutated, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-mutated.json"), mutated); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P6", "fault_injected", map[string]any{
		"fault":       "naive_tracked_content_rollback",
		"description": "rollback restores tracked content but misses untracked artifacts and metadata drift",
	})); err != nil {
		return nil, err
	}

	// This intentionally mimics a weak rollback mechanism. It restores the
	// tracked file bytes but does not remove untracked artifacts or reset mode.
	if err := os.WriteFile(trackedPath, []byte("original\n"), 0o644); err != nil {
		return nil, fmt.Errorf("perform naive rollback: %w", err)
	}

	after, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}

	confirmed, evidence := partialFilesystemRollbackOracle(before, after)
	signature := MismatchSignature{
		LifecycleEvent: "rollback",
		FaultPhase:     "after-naive-filesystem-restore",
		StateClass:     "filesystem",
		Operation:      "partial-restore",
		Relation:       "unsupported-state-residue",
		Impact:         "partial-filesystem-rollback",
	}
	if err := writeManifest(run, CaseManifest{
		Objective:         "Detect filesystem state classes that survive a rollback which only restores tracked file contents.",
		StateClasses:      []string{"filesystem"},
		FaultPhases:       []string{"P4 filesystem mutated", "P6 naive rollback", "oracle after snapshot"},
		Primitives:        []string{"tracked content restore", "untracked file", "symlink", "permission drift"},
		ExpectedSignature: signature,
		Artifacts:         []string{"trace.jsonl", "snapshot-before.json", "snapshot-mutated.json", "snapshot-after.json", "result.json"},
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

func mutateFilesystemForRollback(workspace string) error {
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		return fmt.Errorf("mutate tracked file: %w", err)
	}
	if err := os.Chmod(filepath.Join(workspace, "tracked.txt"), 0o755); err != nil {
		return fmt.Errorf("mutate tracked file mode: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "untracked.txt"), []byte("residue\n"), 0o644); err != nil {
		return fmt.Errorf("create untracked file: %w", err)
	}
	if err := os.Symlink("tracked.txt", filepath.Join(workspace, "link-to-tracked")); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}
	return nil
}
