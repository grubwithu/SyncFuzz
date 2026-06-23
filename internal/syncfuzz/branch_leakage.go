package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func runBranchLeakage(ctx context.Context, opts RunOptions) (*RunResult, error) {
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

	if err := os.WriteFile(filepath.Join(run.workspace, "base.txt"), []byte("checkpoint-base\n"), 0o644); err != nil {
		return nil, fmt.Errorf("create checkpoint base: %w", err)
	}
	before, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-before.json"), before); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P1", "fork_created", map[string]any{
		"checkpoint": "checkpoint-0001",
		"branches":   []string{"discarded-branch-a", "committed-branch-b"},
	})); err != nil {
		return nil, err
	}

	// Branch A is speculative and will be discarded at the agent layer. The
	// filesystem write below models an OS effect that the agent branch metadata
	// forgets to isolate or undo.
	if err := os.WriteFile(filepath.Join(run.workspace, "discarded-branch-a.txt"), []byte("leaked from discarded branch\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write discarded branch artifact: %w", err)
	}
	if err := run.trace.Write(newEvent(run, "P4", "discarded_branch_effect", map[string]any{
		"branch": "discarded-branch-a",
		"path":   "discarded-branch-a.txt",
	})); err != nil {
		return nil, err
	}

	branchA, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-branch-a.json"), branchA); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P6", "branch_discarded", map[string]any{
		"branch":      "discarded-branch-a",
		"description": "agent discards branch A but does not restore the underlying workspace",
	})); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(run.workspace, "committed-branch-b.txt"), []byte("committed branch output\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write committed branch artifact: %w", err)
	}
	if err := run.trace.Write(newEvent(run, "P8", "branch_committed", map[string]any{
		"branch": "committed-branch-b",
		"path":   "committed-branch-b.txt",
	})); err != nil {
		return nil, err
	}

	after, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after.json"), after); err != nil {
		return nil, err
	}

	confirmed, evidence := branchLeakageOracle(before, after)
	signature := MismatchSignature{
		LifecycleEvent: "fork-discard",
		FaultPhase:     "after-discarded-branch-effect",
		StateClass:     "filesystem",
		Operation:      "discarded-branch-write",
		Relation:       "discarded-branch-affects-committed-branch",
		Impact:         "branch-leakage",
	}
	if err := writeManifest(run, CaseManifest{
		Objective:         "Detect effects from a discarded speculative branch leaking into the committed branch state.",
		StateClasses:      []string{"filesystem"},
		FaultPhases:       []string{"P1 fork", "P4 discarded branch effect", "P6 discard", "P8 commit alternate branch"},
		Primitives:        []string{"fork from checkpoint", "discarded branch write", "committed branch write"},
		ExpectedSignature: signature,
		Artifacts:         []string{"trace.jsonl", "snapshot-before.json", "snapshot-branch-a.json", "snapshot-after.json", "result.json"},
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
