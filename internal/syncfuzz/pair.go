package syncfuzz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const differentialReportArtifact = "differential-report.json"

type PairOptions struct {
	CaseName        string
	OutDir          string
	Delay           time.Duration
	MockURL         string
	EnvKind         string
	ContainerImage  string
	FaultPlanID     string
	TimingProfileID string
}

type PairRunSummary struct {
	RunID              string            `json:"run_id"`
	RunRole            string            `json:"run_role"`
	ArtifactDir        string            `json:"artifact_dir"`
	StateTraceArtifact string            `json:"state_trace_artifact"`
	TimingProfileID    string            `json:"timing_profile_id,omitempty"`
	Confirmed          bool              `json:"confirmed"`
	Signature          MismatchSignature `json:"signature"`
	Evidence           []string          `json:"evidence,omitempty"`
}

type DifferentialVerdict struct {
	Differential     bool   `json:"differential"`
	SecurityRelevant bool   `json:"security_relevant"`
	SignatureMatched bool   `json:"signature_matched"`
	Reason           string `json:"reason"`
}

type ObservationCoverage struct {
	RunID            string   `json:"run_id"`
	RunRole          string   `json:"run_role"`
	ObservationCount int      `json:"observation_count"`
	Layers           []string `json:"layers"`
	StateClasses     []string `json:"state_classes"`
	Phases           []string `json:"phases"`
}

type PairResult struct {
	SchemaVersion       string                `json:"schema_version"`
	PairID              string                `json:"pair_id"`
	CaseName            string                `json:"case_name"`
	FaultPlanID         string                `json:"fault_plan_id,omitempty"`
	TimingProfileID     string                `json:"timing_profile_id,omitempty"`
	Environment         string                `json:"environment"`
	ContainerImage      string                `json:"container_image,omitempty"`
	ArtifactDir         string                `json:"artifact_dir"`
	GeneratedAt         string                `json:"generated_at"`
	Control             PairRunSummary        `json:"control"`
	Fault               PairRunSummary        `json:"fault"`
	Verdict             DifferentialVerdict   `json:"verdict"`
	ObservationCoverage []ObservationCoverage `json:"observation_coverage"`
	Artifacts           []string              `json:"artifacts"`
}

func RunPair(ctx context.Context, opts PairOptions) (*PairResult, error) {
	if opts.CaseName == "" {
		opts.CaseName = "orphan-process"
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if opts.EnvKind == "" {
		opts.EnvKind = "local"
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}
	if err := validateCaseNames([]string{opts.CaseName}); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	pairID := fmt.Sprintf("pair-%d", started.UnixNano())
	pairDir := filepath.Join(opts.OutDir, pairID)
	if err := os.MkdirAll(pairDir, 0o755); err != nil {
		return nil, fmt.Errorf("create pair directory: %w", err)
	}

	base := RunOptions{
		CaseName:        opts.CaseName,
		OutDir:          pairDir,
		Delay:           opts.Delay,
		MockURL:         opts.MockURL,
		EnvKind:         opts.EnvKind,
		ContainerImage:  opts.ContainerImage,
		FaultPlanID:     opts.FaultPlanID,
		TimingProfileID: opts.TimingProfileID,
	}
	controlOpts := base
	controlOpts.RunRole = RunRoleControl
	control, err := Run(ctx, controlOpts)
	if err != nil {
		return nil, fmt.Errorf("control run failed: %w", err)
	}

	faultOpts := base
	faultOpts.RunRole = RunRoleFault
	fault, err := Run(ctx, faultOpts)
	if err != nil {
		return nil, fmt.Errorf("fault run failed: %w", err)
	}

	result, err := buildPairResult(pairID, pairDir, control, fault)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(pairDir, differentialReportArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func buildPairResult(pairID string, pairDir string, control *RunResult, fault *RunResult) (*PairResult, error) {
	controlCoverage, err := observationCoverage(control)
	if err != nil {
		return nil, err
	}
	faultCoverage, err := observationCoverage(fault)
	if err != nil {
		return nil, err
	}

	return &PairResult{
		SchemaVersion:   "syncfuzz.differential-report.v1",
		PairID:          pairID,
		CaseName:        fault.CaseName,
		FaultPlanID:     fault.FaultPlanID,
		TimingProfileID: fault.TimingProfileID,
		Environment:     fault.Environment,
		ContainerImage:  fault.ContainerImage,
		ArtifactDir:     pairDir,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Control:         pairRunSummary(control),
		Fault:           pairRunSummary(fault),
		Verdict:         differentialVerdict(control, fault),
		ObservationCoverage: []ObservationCoverage{
			controlCoverage,
			faultCoverage,
		},
		Artifacts: []string{differentialReportArtifact},
	}, nil
}

func pairRunSummary(result *RunResult) PairRunSummary {
	return PairRunSummary{
		RunID:              result.RunID,
		RunRole:            result.RunRole,
		ArtifactDir:        result.ArtifactDir,
		StateTraceArtifact: filepath.Join(result.ArtifactDir, stateTraceArtifact),
		TimingProfileID:    result.TimingProfileID,
		Confirmed:          result.Confirmed,
		Signature:          result.Signature,
		Evidence:           result.Evidence,
	}
}

func differentialVerdict(control *RunResult, fault *RunResult) DifferentialVerdict {
	signatureMatched := control.Signature.String() == fault.Signature.String()
	switch {
	case !control.Confirmed && fault.Confirmed:
		return DifferentialVerdict{
			Differential:     true,
			SecurityRelevant: true,
			SignatureMatched: signatureMatched,
			Reason:           "fault run confirmed the mismatch while control run stayed clean",
		}
	case control.Confirmed && fault.Confirmed:
		return DifferentialVerdict{
			Differential:     false,
			SecurityRelevant: false,
			SignatureMatched: signatureMatched,
			Reason:           "control run also confirmed the mismatch, so the fault is not isolated",
		}
	case !fault.Confirmed:
		return DifferentialVerdict{
			Differential:     false,
			SecurityRelevant: false,
			SignatureMatched: signatureMatched,
			Reason:           "fault run did not confirm the expected mismatch",
		}
	default:
		return DifferentialVerdict{
			Differential:     false,
			SecurityRelevant: false,
			SignatureMatched: signatureMatched,
			Reason:           "control and fault results were inconclusive",
		}
	}
}

func observationCoverage(result *RunResult) (ObservationCoverage, error) {
	trace, err := readCrossLayerTrace(filepath.Join(result.ArtifactDir, stateTraceArtifact))
	if err != nil {
		return ObservationCoverage{}, err
	}
	return ObservationCoverage{
		RunID:            result.RunID,
		RunRole:          result.RunRole,
		ObservationCount: len(trace.Observations),
		Layers:           presentLayerNames(trace.Layers),
		StateClasses:     uniqueObservationValues(trace.Observations, func(o StateObservation) string { return o.StateClass }),
		Phases:           uniqueObservationValues(trace.Observations, func(o StateObservation) string { return o.Phase }),
	}, nil
}

func readCrossLayerTrace(path string) (CrossLayerTrace, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CrossLayerTrace{}, fmt.Errorf("read state trace %s: %w", path, err)
	}
	var trace CrossLayerTrace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return CrossLayerTrace{}, fmt.Errorf("decode state trace %s: %w", path, err)
	}
	return trace, nil
}
