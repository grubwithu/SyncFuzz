package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func runPersistentShellPoisoning(ctx context.Context, opts RunOptions) (*RunResult, error) {
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

	shell, err := env.StartPersistentShell(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("start persistent shell: %w", err)
	}
	defer shell.Close()

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

	confirmed, evidence := persistentShellPoisoningOracle(before, after)
	signature := MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-shell-mutation",
		StateClass:     "shell-state",
		Operation:      "persistent-env-cwd",
		Relation:       "agent-restores-graph-not-shell",
		Impact:         "shell-state-residue",
	}
	if err := writeManifest(run, CaseManifest{
		Objective:         "Detect shell process residue when graph state is restored but the persistent shell is reused.",
		StateClasses:      []string{"shell-state"},
		FaultPhases:       []string{"P4 shell state mutated", "P6 graph restore", "P8 probe reused shell"},
		Primitives:        []string{"PATH prepend", "cwd change", "alias injection"},
		ExpectedSignature: signature,
		Artifacts:         []string{"trace.jsonl", "shell-before.json", "shell-after.json", "result.json"},
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
