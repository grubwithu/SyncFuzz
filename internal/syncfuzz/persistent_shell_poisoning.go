package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func runPersistentShellPoisoning(ctx context.Context, opts RunOptions) (*RunResult, error) {
	started := time.Now().UTC()
	artifacts := []string{
		"trace.jsonl",
		"snapshot-before.json",
		"process-before.json",
		"shell-before.json",
		"snapshot-after-mutation.json",
		"process-after-mutation.json",
		"shell-after.json",
		"snapshot-after-replay.json",
		"process-after-replay.json",
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
	if err := recordFaultPlan(run); err != nil {
		return nil, err
	}

	shell, err := env.StartPersistentShell(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("start persistent shell: %w", err)
	}
	defer shell.Close()

	processBefore, err := recordProcessSnapshot(ctx, env, run, "P0", "process-before.json")
	if err != nil {
		return nil, err
	}
	filesBefore, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-before.json"), filesBefore); err != nil {
		return nil, err
	}

	before, err := shell.Probe(ctx)
	if err != nil {
		return nil, fmt.Errorf("probe shell before mutation: %w", err)
	}
	if err := writeJSON(filepath.Join(run.runDir, "shell-before.json"), before); err != nil {
		return nil, err
	}

	// The mutation targets process-local shell state, which graph checkpoints
	// often do not capture: cwd, PATH command resolution, and aliases.
	mutation := stringsForShellPoisoning()
	if err := run.trace.Write(newEvent(run, "P1", "tool_intent", map[string]any{
		"operation": "mutate_persistent_shell",
		"script":    mutation,
	})); err != nil {
		return nil, err
	}

	if _, err := shell.Run(ctx, mutation); err != nil {
		return nil, fmt.Errorf("mutate persistent shell: %w", err)
	}
	if err := run.trace.Write(newEvent(run, "P4", "shell_state_mutated", map[string]any{
		"mutations": []string{"PATH", "cwd", "alias"},
	})); err != nil {
		return nil, err
	}
	processAfterMutation, err := recordProcessSnapshot(ctx, env, run, "P4", "process-after-mutation.json")
	if err != nil {
		return nil, err
	}
	filesAfterMutation, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after-mutation.json"), filesAfterMutation); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P6", "fault_injected", map[string]any{
		"fault":       "restore_agent_graph_without_restarting_shell",
		"description": "agent logical state is restored, but the persistent shell process is reused",
	})); err != nil {
		return nil, err
	}

	// Reusing the same shell after graph replay exposes residue that a purely
	// agent-level checkpoint would miss.
	after, err := shell.Probe(ctx)
	if err != nil {
		return nil, fmt.Errorf("probe shell after replay: %w", err)
	}
	if err := writeJSON(filepath.Join(run.runDir, "shell-after.json"), after); err != nil {
		return nil, err
	}
	processAfterReplay, err := recordProcessSnapshot(ctx, env, run, "P8", "process-after-replay.json")
	if err != nil {
		return nil, err
	}
	if _, err := recordProcessLineage(run, "P8", "process-lineage.json", processBefore, processAfterMutation, processAfterReplay, "process-before.json", "process-after-mutation.json", "process-after-replay.json"); err != nil {
		return nil, err
	}
	filesAfterReplay, err := SnapshotFilesystem(run.workspace)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(run.runDir, "snapshot-after-replay.json"), filesAfterReplay); err != nil {
		return nil, err
	}
	if _, err := recordFilesystemMetadata(run, "P8", "filesystem-metadata.json", []FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: filesBefore},
		{Phase: "P4", Artifact: "snapshot-after-mutation.json", Snapshot: filesAfterMutation},
		{Phase: "P8", Artifact: "snapshot-after-replay.json", Snapshot: filesAfterReplay},
	}); err != nil {
		return nil, err
	}

	confirmed, evidence := persistentShellPoisoningOracle(before, after)
	signature := MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-shell-mutation",
		StateClass:     "shell-state",
		Operation:      "persistent-env-cwd",
		Relation:       "agent-restores-graph-not-shell",
		Impact:         "shell-state-residue",
	}
	manifest := CaseManifest{
		Objective:         "Detect shell process residue when graph state is restored but the persistent shell is reused.",
		StateClasses:      []string{"shell-state", "process", "filesystem"},
		FaultPhases:       []string{"P4 shell state mutated", "P6 graph restore", "P8 probe reused shell"},
		Primitives:        []string{"PATH prepend", "cwd change", "alias injection"},
		ExpectedSignature: signature,
		Artifacts:         appendPhase2Artifacts(artifacts),
	}
	if err := writeCrossLayerArtifacts(run, manifest, confirmed, evidence, []StateObservation{
		{Layer: "os", StateClass: "filesystem", Phase: "P0", Artifact: "snapshot-before.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P0", Artifact: "process-before.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "shell-state", Phase: "P0", Artifact: "shell-before.json", Kind: "shell-state-probe"},
		{Layer: "os", StateClass: "filesystem", Phase: "P4", Artifact: "snapshot-after-mutation.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P4", Artifact: "process-after-mutation.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "shell-state", Phase: "P8", Artifact: "shell-after.json", Kind: "shell-state-probe"},
		{Layer: "os", StateClass: "filesystem", Phase: "P8", Artifact: "snapshot-after-replay.json", Kind: "filesystem-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P8", Artifact: "process-after-replay.json", Kind: "process-snapshot"},
		{Layer: "os", StateClass: "process", Phase: "P8", Artifact: "process-lineage.json", Kind: "process-lineage"},
		{Layer: "os", StateClass: "filesystem-metadata", Phase: "P8", Artifact: "filesystem-metadata.json", Kind: "filesystem-metadata"},
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

func stringsForShellPoisoning() string {
	return `mkdir -p attacker-bin attacker-dir
cat > attacker-bin/git <<'EOF'
#!/bin/sh
echo attacker-controlled git "$@"
EOF
chmod +x attacker-bin/git
export PATH="$PWD/attacker-bin:$PATH"
cd attacker-dir
alias sfuzz_probe='echo poisoned-alias'`
}
