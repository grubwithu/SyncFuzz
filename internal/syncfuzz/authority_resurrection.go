package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func runAuthorityResurrection(ctx context.Context, opts RunOptions) (*RunResult, error) {
	started := time.Now().UTC()
	env, err := newEnvironment(opts.EnvKind, opts.ContainerImage)
	if err != nil {
		return nil, err
	}
	run, err := env.PrepareRun(ctx, opts, started, false)
	if err != nil {
		return nil, err
	}
	defer run.Close()

	// Authority state is modeled as an external, non-rollbackable service.
	// A real target may store this in an approval service, token broker, or UI.
	mockURL := trimURL(opts.MockURL)
	serverKind := "memory"
	var backend effectBackend = newMemoryEffectBackend()
	if mockURL != "" {
		backend = newHTTPEffectBackend(mockURL)
		serverKind = "http"
	}
	defer backend.Close()

	if err := run.trace.Write(newEvent(run, "P0", "run_started", map[string]any{
		"environment":    run.environment,
		"mock_url":       mockURL,
		"server_kind":    serverKind,
		"run_role":       run.runRole,
		"timing_profile": run.timing.ProfileID,
	})); err != nil {
		return nil, err
	}
	if err := recordFaultPlan(run); err != nil {
		return nil, err
	}

	if err := backend.Reset(ctx); err != nil {
		return nil, fmt.Errorf("reset external state: %w", err)
	}

	before, err := backend.State(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch external state before run: %w", err)
	}
	if err := writeJSON(filepath.Join(run.runDir, "external-before.json"), before); err != nil {
		return nil, err
	}

	// The token is single-use. The first consume is legitimate and changes the
	// authority server state even if the agent later restores older graph state.
	issued, err := backend.IssueToken(ctx, map[string]any{
		"scope":   "single-use:deploy",
		"subject": "agent-" + run.runID,
	})
	if err != nil {
		return nil, fmt.Errorf("issue authority token: %w", err)
	}
	if err := run.trace.Write(newEvent(run, "P1", "authority_issued", map[string]any{
		"token": issued.Token.Token,
		"scope": issued.Token.Scope,
	})); err != nil {
		return nil, err
	}

	first, err := backend.ConsumeToken(ctx, map[string]any{
		"token":     issued.Token.Token,
		"operation": "deploy-branch-a",
	})
	if err != nil {
		return nil, fmt.Errorf("consume authority token: %w", err)
	}
	if !first.Accepted {
		return nil, fmt.Errorf("first authority consume was rejected: %s", first.Error)
	}
	if err := run.trace.Write(newEvent(run, "P4", "authority_consumed", map[string]any{
		"token":      first.Token.Token,
		"operation":  first.Token.ConsumedBy,
		"consumedAt": first.Token.ConsumedAt,
	})); err != nil {
		return nil, err
	}

	var replay *consumeTokenResponse
	if isControlRun(opts) {
		if err := run.trace.Write(newEvent(run, "P6", "control_checkpoint_persisted", map[string]any{
			"description": "agent checkpoint records that the token has been consumed",
		})); err != nil {
			return nil, err
		}
	} else {
		if err := run.trace.Write(newEvent(run, "P6", "fault_injected", map[string]any{
			"fault":       "restore_checkpoint_before_authority_consume",
			"description": "agent state is restored to a checkpoint that still treats the token as unused",
		})); err != nil {
			return nil, err
		}
		if err := waitForTimingBoundary(ctx, run, "P8", "before_authority_reuse"); err != nil {
			return nil, err
		}

		// The second consume should fail. The vulnerability signal is the stale
		// reuse attempt itself: C_agent says unused, C_authority says consumed.
		replay, err = backend.ConsumeToken(ctx, map[string]any{
			"token":     issued.Token.Token,
			"operation": "deploy-branch-b",
		})
		if err != nil {
			return nil, fmt.Errorf("replay authority token consume: %w", err)
		}
		if err := run.trace.Write(newEvent(run, "P8", "authority_reuse_attempt", map[string]any{
			"token":     issued.Token.Token,
			"operation": "deploy-branch-b",
			"accepted":  replay.Accepted,
			"error":     replay.Error,
		})); err != nil {
			return nil, err
		}
	}

	after, err := backend.State(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch external state after run: %w", err)
	}
	if err := writeJSON(filepath.Join(run.runDir, "external-after.json"), after); err != nil {
		return nil, err
	}

	confirmed, evidence := authorityResurrectionOracle(after, issued.Token.Token, replay)
	signature := MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-authority-consume",
		StateClass:     "authority-state",
		Operation:      "stale-token-reuse",
		Relation:       "agent-resurrects-consumed-capability",
		Impact:         "authority-resurrection",
	}
	manifest := CaseManifest{
		Objective:         "Detect replay attempts that reuse a capability already consumed by the authority service.",
		StateClasses:      []string{"authority-state"},
		FaultPhases:       []string{"P4 authority consumed", "P6 restore checkpoint", "P8 replay"},
		Primitives:        []string{"single-use token", "authority consume", "stale token reuse"},
		ExpectedSignature: signature,
		Artifacts:         appendPhase2Artifacts([]string{"trace.jsonl", "external-before.json", "external-after.json", "result.json"}),
	}
	afterPhase := "P8"
	if isControlRun(opts) {
		afterPhase = "P6"
	}
	if err := writeCrossLayerArtifacts(run, manifest, confirmed, evidence, []StateObservation{
		{Layer: "authority", StateClass: "authority-state", Phase: "P0", Artifact: "external-before.json", Kind: "authority-state-snapshot"},
		{Layer: "authority", StateClass: "authority-state", Phase: afterPhase, Artifact: "external-after.json", Kind: "authority-state-snapshot"},
	}); err != nil {
		return nil, err
	}
	if err := writeManifest(run, manifest); err != nil {
		return nil, err
	}

	finished := time.Now().UTC()
	result := &RunResult{
		RunID:           run.runID,
		CaseName:        opts.CaseName,
		RunRole:         run.runRole,
		Environment:     run.environment,
		ContainerImage:  run.containerImage,
		FaultPlanID:     run.faultPlan.ID,
		PrimitiveID:     run.primitiveID,
		TimingProfileID: run.timing.ProfileID,
		Confirmed:       confirmed,
		Signature:       signature,
		Evidence:        evidence,
		ArtifactDir:     run.runDir,
		StartedAt:       started.Format(time.RFC3339Nano),
		FinishedAt:      finished.Format(time.RFC3339Nano),
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
