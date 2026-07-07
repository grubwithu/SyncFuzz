package cases

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func runBranchLeakage(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
	started := time.Now().UTC()
	artifacts := []string{
		"trace.jsonl",
		"snapshot-before.json",
		"process-before.json",
		"snapshot-branch-a.json",
		"process-branch-a.json",
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

	if err := run.Trace.Write(core.NewEvent(run, "P0", "run_started", map[string]any{
		"environment":    run.Environment,
		"workspace":      run.Workspace,
		"run_role":       run.RunRole,
		"timing_profile": run.Timing.ProfileID,
	})); err != nil {
		return nil, err
	}
	if err := core.RecordFaultPlan(run); err != nil {
		return nil, err
	}

	if _, err := env.ExecShell(ctx, run, "printf 'checkpoint-base\\n' > base.txt"); err != nil {
		return nil, fmt.Errorf("create checkpoint base: %w", err)
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

	if err := run.Trace.Write(core.NewEvent(run, "P1", "fork_created", map[string]any{
		"checkpoint": "checkpoint-0001",
		"branches":   []string{"discarded-branch-a", "committed-branch-b"},
	})); err != nil {
		return nil, err
	}

	// Branch A is speculative and will be discarded at the agent layer. The
	// filesystem write below models an OS effect that the agent branch metadata
	// forgets to isolate or undo.
	if _, err := env.ExecShell(ctx, run, "printf 'leaked from discarded branch\\n' > discarded-branch-a.txt"); err != nil {
		return nil, fmt.Errorf("write discarded branch artifact: %w", err)
	}
	if err := run.Trace.Write(core.NewEvent(run, "P4", "discarded_branch_effect", map[string]any{
		"branch": "discarded-branch-a",
		"path":   "discarded-branch-a.txt",
	})); err != nil {
		return nil, err
	}

	branchA, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-branch-a.json"), branchA); err != nil {
		return nil, err
	}
	processBranchA, err := core.RecordProcessSnapshot(ctx, env, run, "P4", "process-branch-a.json")
	if err != nil {
		return nil, err
	}

	if isControlRun(opts) {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "branch_discarded_with_restore", map[string]any{
			"branch":      "discarded-branch-a",
			"description": "control run discards branch A and restores the underlying workspace",
		})); err != nil {
			return nil, err
		}
		if _, err := env.ExecShell(ctx, run, "rm -f discarded-branch-a.txt"); err != nil {
			return nil, fmt.Errorf("restore workspace after discarded branch: %w", err)
		}
	} else {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "branch_discarded", map[string]any{
			"branch":      "discarded-branch-a",
			"description": "agent discards branch A but does not restore the underlying workspace",
		})); err != nil {
			return nil, err
		}
	}
	if err := core.WaitForTimingBoundary(ctx, run, "P8", "before_alternate_branch_commit"); err != nil {
		return nil, err
	}

	if _, err := env.ExecShell(ctx, run, "printf 'committed branch output\\n' > committed-branch-b.txt"); err != nil {
		return nil, fmt.Errorf("write committed branch artifact: %w", err)
	}
	if err := run.Trace.Write(core.NewEvent(run, "P8", "branch_committed", map[string]any{
		"branch": "committed-branch-b",
		"path":   "committed-branch-b.txt",
	})); err != nil {
		return nil, err
	}

	after, err := core.SnapshotFilesystem(run.Workspace)
	if err != nil {
		return nil, err
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}
	processAfter, err := core.RecordProcessSnapshot(ctx, env, run, "P8", "process-after.json")
	if err != nil {
		return nil, err
	}
	if _, err := core.RecordProcessLineage(run, "P8", "process-lineage.json", processBefore, processBranchA, processAfter, "process-before.json", "process-branch-a.json", "process-after.json"); err != nil {
		return nil, err
	}
	if _, err := core.RecordFilesystemMetadata(run, "P8", "filesystem-metadata.json", []core.FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P4", Artifact: "snapshot-branch-a.json", Snapshot: branchA},
		{Phase: "P8", Artifact: "snapshot-after.json", Snapshot: after},
	}); err != nil {
		return nil, err
	}

	confirmed, evidence := branchLeakageOracle(before, after)
	signature := core.MismatchSignature{
		LifecycleEvent: "fork-discard",
		FaultPhase:     "after-discarded-branch-effect",
		StateClass:     "filesystem",
		Operation:      "discarded-branch-write",
		Relation:       "discarded-branch-affects-committed-branch",
		Impact:         "branch-leakage",
	}
	manifest := core.CaseManifest{
		Objective:         "Detect effects from a discarded speculative branch leaking into the committed branch state.",
		StateClasses:      []string{"filesystem", "process"},
		FaultPhases:       []string{"P1 fork", "P4 discarded branch effect", "P6 discard", "P8 commit alternate branch"},
		Primitives:        []string{"fork from checkpoint", "discarded branch write", "committed branch write"},
		ExpectedSignature: signature,
		Artifacts:         core.AppendPhase2Artifacts(artifacts),
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, confirmed, evidence, []core.StateObservation{
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P4", Artifact: "snapshot-branch-a.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P4", Artifact: "process-branch-a.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "filesystem", Phase: "P8", Artifact: "snapshot-after.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P8", Artifact: "process-after.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P8", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P8", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
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
