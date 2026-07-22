package target

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	TargetRuntimePairSchemaVersion = "syncfuzz.target-runtime-pair.v1"
	TargetRuntimePairArtifact      = "target-runtime-pair.json"
)

// TargetRuntimePairOptions executes one explicit counterfactual control and
// target run before comparing their recorded artifacts. The two commands may
// differ, but they must exercise the same target and task so their lifecycle
// query remains comparable. Control semantics are declared rather than
// inferred from a command string.
type TargetRuntimePairOptions struct {
	PairID             string
	ControlKind        TargetPairControlKind
	ControlDescription string
	Control            TargetRunOptions
	Target             TargetRunOptions
	OutDir             string
}

// TargetRuntimePairResult records the two freshly executed runs and their
// deterministic pair comparison. Its counterfactual label is an observed
// oracle outcome, not a causal finding.
type TargetRuntimePairResult struct {
	SchemaVersion         string                        `json:"schema_version"`
	PairID                string                        `json:"pair_id"`
	StartedAt             string                        `json:"started_at"`
	FinishedAt            string                        `json:"finished_at"`
	ArtifactDir           string                        `json:"artifact_dir"`
	ControlKind           TargetPairControlKind         `json:"control_kind"`
	ControlDescription    string                        `json:"control_description,omitempty"`
	ControlRunID          string                        `json:"control_run_id"`
	ControlRunDir         string                        `json:"control_run_dir"`
	TargetRunID           string                        `json:"target_run_id"`
	TargetRunDir          string                        `json:"target_run_dir"`
	PairDifferential      string                        `json:"pair_differential_artifact"`
	QueryID               string                        `json:"query_id"`
	QueryStratum          TargetPairQueryStratum        `json:"query_stratum"`
	CounterfactualLabel   TargetPairCounterfactualLabel `json:"counterfactual_label"`
	CounterfactualReason  string                        `json:"counterfactual_reason,omitempty"`
	ContractCalibration   TargetPairContractCalibration `json:"contract_calibration"`
	RootCauseCandidateCnt int                           `json:"root_cause_candidate_count"`
}

// RunTargetRuntimePair gives a controlled runtime campaign a single-pair
// execution primitive. Each side receives an independent workspace under the
// pair artifact directory, then CompareTargetRuns enforces matching query
// identity over the freshly recorded artifacts.
func RunTargetRuntimePair(ctx context.Context, opts TargetRuntimePairOptions) (*TargetRuntimePairResult, error) {
	if !isTargetPairControlKind(opts.ControlKind) {
		return nil, fmt.Errorf("unsupported runtime pair control kind %q", opts.ControlKind)
	}
	controlDescription := strings.TrimSpace(opts.ControlDescription)
	if opts.ControlKind == TargetPairControlCustom && controlDescription == "" {
		return nil, fmt.Errorf("custom runtime pair control requires a description")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("runtime pair output directory is required")
	}
	if strings.TrimSpace(opts.Control.Workspace) != "" || strings.TrimSpace(opts.Target.Workspace) != "" {
		return nil, fmt.Errorf("runtime pair controls its own isolated workspaces")
	}

	control, targetRun, err := normalizeTargetRuntimePairRuns(opts.Control, opts.Target)
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	pairID := strings.TrimSpace(opts.PairID)
	if pairID == "" {
		pairID = fmt.Sprintf("target-runtime-pair-%d", started.UnixNano())
	}
	if !isTargetPairCampaignPairID(pairID) {
		return nil, fmt.Errorf("invalid runtime pair id %q", pairID)
	}
	pairDir, err := filepath.Abs(filepath.Join(opts.OutDir, pairID))
	if err != nil {
		return nil, fmt.Errorf("resolve runtime pair artifact directory: %w", err)
	}
	if _, err := os.Stat(pairDir); err == nil {
		return nil, fmt.Errorf("runtime pair artifact directory %s already exists", pairDir)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat runtime pair artifact directory %s: %w", pairDir, err)
	}
	if err := os.MkdirAll(pairDir, 0o755); err != nil {
		return nil, fmt.Errorf("create runtime pair artifact directory: %w", err)
	}
	control.OutDir = filepath.Join(pairDir, "control")
	targetRun.OutDir = filepath.Join(pairDir, "target")

	controlResult, err := RunTarget(ctx, control)
	if err != nil {
		return nil, fmt.Errorf("run runtime pair control: %w", err)
	}
	targetResult, err := RunTarget(ctx, targetRun)
	if err != nil {
		return nil, fmt.Errorf("run runtime pair target: %w", err)
	}
	differentialPath := filepath.Join(pairDir, TargetPairDifferentialArtifact)
	differential, err := CompareTargetRuns(TargetPairDifferentialOptions{
		ControlRunDir: controlResult.ArtifactDir,
		TargetRunDir:  targetResult.ArtifactDir,
		OutputPath:    differentialPath,
	})
	if err != nil {
		return nil, fmt.Errorf("compare freshly executed runtime pair: %w", err)
	}
	result := &TargetRuntimePairResult{
		SchemaVersion:         TargetRuntimePairSchemaVersion,
		PairID:                pairID,
		StartedAt:             started.Format(time.RFC3339Nano),
		FinishedAt:            time.Now().UTC().Format(time.RFC3339Nano),
		ArtifactDir:           pairDir,
		ControlKind:           opts.ControlKind,
		ControlDescription:    controlDescription,
		ControlRunID:          controlResult.RunID,
		ControlRunDir:         controlResult.ArtifactDir,
		TargetRunID:           targetResult.RunID,
		TargetRunDir:          targetResult.ArtifactDir,
		PairDifferential:      TargetPairDifferentialArtifact,
		QueryID:               differential.QueryID,
		QueryStratum:          differential.QueryStratum,
		CounterfactualLabel:   differential.CounterfactualLabel,
		CounterfactualReason:  differential.CounterfactualReason,
		ContractCalibration:   differential.ContractCalibration,
		RootCauseCandidateCnt: len(differential.RootCauseCandidates),
	}
	if err := core.WriteJSON(filepath.Join(pairDir, TargetRuntimePairArtifact), result); err != nil {
		return nil, fmt.Errorf("write runtime pair result: %w", err)
	}
	return result, nil
}

func normalizeTargetRuntimePairRuns(control TargetRunOptions, targetRun TargetRunOptions) (TargetRunOptions, TargetRunOptions, error) {
	if control.AdapterID == "" {
		control.AdapterID = DefaultTargetAdapterID
	}
	if targetRun.AdapterID == "" {
		targetRun.AdapterID = DefaultTargetAdapterID
	}
	if control.TargetID == "" {
		control.TargetID = control.AdapterID
	}
	if targetRun.TargetID == "" {
		targetRun.TargetID = targetRun.AdapterID
	}
	if control.TaskID == "" {
		control.TaskID = DefaultTargetTaskID
	}
	if targetRun.TaskID == "" {
		targetRun.TaskID = DefaultTargetTaskID
	}
	if control.AdapterID != targetRun.AdapterID {
		return TargetRunOptions{}, TargetRunOptions{}, fmt.Errorf("runtime pair adapter %q does not match %q", control.AdapterID, targetRun.AdapterID)
	}
	if control.TargetID != targetRun.TargetID {
		return TargetRunOptions{}, TargetRunOptions{}, fmt.Errorf("runtime pair target %q does not match %q", control.TargetID, targetRun.TargetID)
	}
	if control.TaskID != targetRun.TaskID {
		return TargetRunOptions{}, TargetRunOptions{}, fmt.Errorf("runtime pair task %q does not match %q", control.TaskID, targetRun.TaskID)
	}
	return control, targetRun, nil
}
