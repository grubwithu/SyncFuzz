package cases

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/effect"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func runAuthorityResurrection(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
	started := time.Now().UTC()
	env, err := environment.NewEnvironment(opts.EnvKind, opts.ContainerImage)
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
	mockURL := effect.TrimURL(opts.MockURL)
	serverKind := "memory"
	var backend effect.EffectBackend = effect.NewMemoryEffectBackend()
	if mockURL != "" {
		backend = effect.NewHTTPEffectBackend(mockURL)
		serverKind = "http"
	}
	defer backend.Close()

	if err := run.Trace.Write(core.NewEvent(run, "P0", "run_started", map[string]any{
		"environment":    run.Environment,
		"mock_url":       mockURL,
		"server_kind":    serverKind,
		"run_role":       run.RunRole,
		"timing_profile": run.Timing.ProfileID,
	})); err != nil {
		return nil, err
	}
	if err := core.RecordFaultPlan(run); err != nil {
		return nil, err
	}

	if err := backend.Reset(ctx); err != nil {
		return nil, fmt.Errorf("reset external state: %w", err)
	}

	before, err := backend.State(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch external state before run: %w", err)
	}
	if err := core.WriteJSON(filepath.Join(run.RunDir, "external-before.json"), before); err != nil {
		return nil, err
	}

	// The token is single-use. The first consume is legitimate and changes the
	// authority server state even if the agent later restores older graph state.
	issued, err := backend.IssueToken(ctx, map[string]any{
		"scope":   "single-use:deploy",
		"subject": "agent-" + run.RunID,
	})
	if err != nil {
		return nil, fmt.Errorf("issue authority token: %w", err)
	}
	if err := run.Trace.Write(core.NewEvent(run, "P1", "authority_issued", map[string]any{
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
	if err := run.Trace.Write(core.NewEvent(run, "P4", "authority_consumed", map[string]any{
		"token":      first.Token.Token,
		"operation":  first.Token.ConsumedBy,
		"consumedAt": first.Token.ConsumedAt,
	})); err != nil {
		return nil, err
	}

	var replay *effect.ConsumeTokenResponse
	if isControlRun(opts) {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "control_checkpoint_persisted", map[string]any{
			"description": "agent checkpoint records that the token has been consumed",
		})); err != nil {
			return nil, err
		}
	} else {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "fault_injected", map[string]any{
			"fault":       "restore_checkpoint_before_authority_consume",
			"description": "agent state is restored to a checkpoint that still treats the token as unused",
		})); err != nil {
			return nil, err
		}
		if err := core.WaitForTimingBoundary(ctx, run, "P8", "before_authority_reuse"); err != nil {
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
		if err := run.Trace.Write(core.NewEvent(run, "P8", "authority_reuse_attempt", map[string]any{
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
	if err := core.WriteJSON(filepath.Join(run.RunDir, "external-after.json"), after); err != nil {
		return nil, err
	}

	confirmed, evidence := authorityResurrectionOracle(after, issued.Token.Token, replay)
	signature := core.MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-authority-consume",
		StateClass:     "authority-state",
		Operation:      "stale-token-reuse",
		Relation:       "agent-resurrects-consumed-capability",
		Impact:         "authority-resurrection",
	}
	manifest := core.CaseManifest{
		Objective:         "Detect replay attempts that reuse a capability already consumed by the authority service.",
		StateClasses:      []string{"authority-state"},
		FaultPhases:       []string{"P4 authority consumed", "P6 restore checkpoint", "P8 replay"},
		Primitives:        []string{"single-use token", "authority consume", "stale token reuse"},
		ExpectedSignature: signature,
		Artifacts:         core.AppendPhase2Artifacts([]string{"trace.jsonl", "external-before.json", "external-after.json", "result.json"}),
	}
	afterPhase := "P8"
	if isControlRun(opts) {
		afterPhase = "P6"
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, confirmed, evidence, []core.StateObservation{
		{Layer: "authority", StateClass: "authority-state", Phase: "P0", Artifact: "external-before.json", Kind: "authority-state-snapshot"},
		{Layer: "authority", StateClass: "authority-state", Phase: afterPhase, Artifact: "external-after.json", Kind: "authority-state-snapshot"},
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
