package syncfuzz

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func runActionReplay(ctx context.Context, opts RunOptions) (*RunResult, error) {
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

	// By default the external service is an in-memory model. Passing --mock-url
	// switches to the TypeScript HTTP mock server without changing the testcase.
	mockURL := trimURL(opts.MockURL)
	serverKind := "memory"
	var backend effectBackend = newMemoryEffectBackend()
	if mockURL != "" {
		backend = newHTTPEffectBackend(mockURL)
		serverKind = "http"
	}
	defer backend.Close()

	if err := run.trace.Write(newEvent(run, "P0", "run_started", map[string]any{
		"environment": run.environment,
		"mock_url":    mockURL,
		"server_kind": serverKind,
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

	// One logical action intentionally uses two request IDs across replay. This
	// simulates an agent that lost the receipt and cannot deduplicate safely.
	payload := map[string]any{
		"logical_operation": "create-ci-resource",
		"run_id":            run.runID,
	}
	firstRequestID := "req-" + run.runID + "-attempt-1"
	if err := run.trace.Write(newEvent(run, "P1", "tool_intent", map[string]any{
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
	if err := run.trace.Write(newEvent(run, "P4", "external_effect_released", map[string]any{
		"resource_id": first.Resource.ID,
		"request_id":  first.Resource.RequestID,
	})); err != nil {
		return nil, err
	}

	if err := run.trace.Write(newEvent(run, "P5", "fault_injected", map[string]any{
		"fault":       "drop_tool_result",
		"description": "external effect committed, but the agent loses the receipt before checkpoint persistence",
	})); err != nil {
		return nil, err
	}

	// Replay repeats the logical action against an external state that did not
	// roll back, producing the forgotten-external-effect signature.
	secondRequestID := "req-" + run.runID + "-attempt-2"
	if err := run.trace.Write(newEvent(run, "P8", "replay_started", map[string]any{
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
	if err := run.trace.Write(newEvent(run, "P8", "external_effect_replayed", map[string]any{
		"resource_id": second.Resource.ID,
		"request_id":  second.Resource.RequestID,
	})); err != nil {
		return nil, err
	}

	after, err := backend.State(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch external state after run: %w", err)
	}
	if err := writeJSON(filepath.Join(run.runDir, "external-after.json"), after); err != nil {
		return nil, err
	}

	confirmed, evidence := actionReplayOracle(before, after, run.runID)
	signature := MismatchSignature{
		LifecycleEvent: "replay",
		FaultPhase:     "after-external-commit",
		StateClass:     "external-effect",
		Operation:      "duplicate-create",
		Relation:       "missing-receipt",
		Impact:         "forgotten-external-effect",
	}
	manifest := CaseManifest{
		Objective:         "Detect duplicated external resources after a committed effect loses its receipt and is replayed.",
		StateClasses:      []string{"external-effect"},
		FaultPhases:       []string{"P4 external effect released", "P5 dropped tool result", "P8 replay"},
		Primitives:        []string{"external create", "dropped receipt", "replay with new request id"},
		ExpectedSignature: signature,
		Artifacts:         appendPhase2Artifacts([]string{"trace.jsonl", "external-before.json", "external-after.json", "result.json"}),
	}
	if err := writeCrossLayerArtifacts(run, manifest, confirmed, evidence, []StateObservation{
		{Layer: "external", StateClass: "external-effect", Phase: "P0", Artifact: "external-before.json", Kind: "external-state-snapshot"},
		{Layer: "external", StateClass: "external-effect", Phase: "P8", Artifact: "external-after.json", Kind: "external-state-snapshot"},
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
