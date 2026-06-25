package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func runPartialFilesystemRollback(ctx context.Context, opts RunOptions) (*RunResult, error) {
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

	if _, err := env.ExecShell(ctx, run, "printf 'original\\n' > tracked.txt\nchmod 644 tracked.txt"); err != nil {
		return nil, fmt.Errorf("create baseline tracked file: %w", err)
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

	if err := mutateFilesystemForRollback(ctx, env, run); err != nil {
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
	processMutated, err := recordProcessSnapshot(ctx, env, run, "P4", "process-mutated.json")
	if err != nil {
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
	if _, err := env.ExecShell(ctx, run, "printf 'original\\n' > tracked.txt"); err != nil {
		return nil, fmt.Errorf("perform naive rollback: %w", err)
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
	if _, err := recordProcessLineage(run, "P6", "process-lineage.json", processBefore, processMutated, processAfter, "process-before.json", "process-mutated.json", "process-after.json"); err != nil {
		return nil, err
	}
	if _, err := recordFilesystemMetadata(run, "P6", "filesystem-metadata.json", []FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P4", Artifact: "snapshot-mutated.json", Snapshot: mutated},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
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
	manifest := CaseManifest{
		Objective:         "Detect filesystem state classes that survive a rollback which only restores tracked file contents.",
		StateClasses:      []string{"filesystem", "process"},
		FaultPhases:       []string{"P4 filesystem mutated", "P6 naive rollback", "oracle after snapshot"},
		Primitives:        []string{"tracked content restore", "untracked file", "symlink", "permission drift"},
		ExpectedSignature: signature,
		Artifacts:         appendPhase2Artifacts(artifacts),
	}
	if err := writeCrossLayerArtifacts(run, manifest, confirmed, evidence, []StateObservation{
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P4", Artifact: "snapshot-mutated.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P4", Artifact: "process-mutated.json", Kind: "process-snapshot"},
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

func mutateFilesystemForRollback(ctx context.Context, env Environment, run *runContext) error {
	script := `printf 'mutated\n' > tracked.txt
chmod 755 tracked.txt
printf 'residue\n' > untracked.txt
ln -s tracked.txt link-to-tracked`
	if _, err := env.ExecShell(ctx, run, script); err != nil {
		return fmt.Errorf("mutate filesystem for rollback: %w", err)
	}
	return nil
}
