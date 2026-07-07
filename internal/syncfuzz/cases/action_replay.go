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

func runActionReplay(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
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

	// By default the external service is an in-memory model. Passing --mock-url
	// switches to the TypeScript HTTP mock server without changing the testcase.
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

	// One logical action intentionally uses two request IDs across replay. This
	// simulates an agent that lost the receipt and cannot deduplicate safely.
	payload := map[string]any{
		"logical_operation": "create-ci-resource",
		"run_id":            run.RunID,
	}
	firstRequestID := "req-" + run.RunID + "-attempt-1"
	if err := run.Trace.Write(core.NewEvent(run, "P1", "tool_intent", map[string]any{
		"operation":  "create_resource",
		"request_id": firstRequestID,
		"payload":    payload,
	})); err != nil {
		return nil, err
	}

	first, err := backend.CreateResource(ctx, map[string]any{
		"requestId": firstRequestID,
		"kind":      "ci-resource",
		"payload":   payload,
	})
	if err != nil {
		return nil, fmt.Errorf("create first external resource: %w", err)
	}
	if err := run.Trace.Write(core.NewEvent(run, "P4", "external_effect_released", map[string]any{
		"resource_id": first.Resource.ID,
		"request_id":  first.Resource.RequestID,
	})); err != nil {
		return nil, err
	}

	if isControlRun(opts) {
		if err := run.Trace.Write(core.NewEvent(run, "P6", "control_checkpoint_persisted", map[string]any{
			"description": "receipt is preserved, so replay is not needed",
		})); err != nil {
			return nil, err
		}
	} else {
		if err := run.Trace.Write(core.NewEvent(run, "P5", "fault_injected", map[string]any{
			"fault":       "drop_tool_result",
			"description": "external effect committed, but the agent loses the receipt before checkpoint persistence",
		})); err != nil {
			return nil, err
		}
		if err := core.WaitForTimingBoundary(ctx, run, "P8", "before_replay"); err != nil {
			return nil, err
		}

		// Replay repeats the logical action against an external state that did
		// not roll back, producing the forgotten-external-effect signature.
		secondRequestID := "req-" + run.RunID + "-attempt-2"
		if err := run.Trace.Write(core.NewEvent(run, "P8", "replay_started", map[string]any{
			"operation":  "create_resource",
			"request_id": secondRequestID,
			"reason":     "agent replays the logical action with a new request id",
		})); err != nil {
			return nil, err
		}

		second, err := backend.CreateResource(ctx, map[string]any{
			"requestId": secondRequestID,
			"kind":      "ci-resource",
			"payload":   payload,
		})
		if err != nil {
			return nil, fmt.Errorf("create replayed external resource: %w", err)
		}
		if err := run.Trace.Write(core.NewEvent(run, "P8", "external_effect_replayed", map[string]any{
			"resource_id": second.Resource.ID,
			"request_id":  second.Resource.RequestID,
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

	confirmed, evidence := actionReplayOracle(before, after, run.RunID)
	signature := core.MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-external-commit",
		StateClass:     "external-effect",
		Operation:      "duplicate-create",
		Relation:       "missing-receipt",
		Impact:         "forgotten-external-effect",
	}
	manifest := core.CaseManifest{
		Objective:         "Detect duplicated external resources after a committed effect loses its receipt and is replayed.",
		StateClasses:      []string{"external-effect"},
		FaultPhases:       []string{"P4 external effect released", "P5 dropped tool result", "P8 replay"},
		Primitives:        []string{"external create", "dropped receipt", "replay with new request id"},
		ExpectedSignature: signature,
		Artifacts:         core.AppendPhase2Artifacts([]string{"trace.jsonl", "external-before.json", "external-after.json", "result.json"}),
	}
	afterPhase := "P8"
	if isControlRun(opts) {
		afterPhase = "P6"
	}
	if err := core.WriteCrossLayerArtifacts(run, manifest, confirmed, evidence, []core.StateObservation{
		{Layer: "external", StateClass: "external-effect", Phase: "P0", Artifact: "external-before.json", Kind: "external-state-snapshot"},
		{Layer: "external", StateClass: "external-effect", Phase: afterPhase, Artifact: "external-after.json", Kind: "external-state-snapshot"},
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
