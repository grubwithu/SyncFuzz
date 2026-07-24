package recovery

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const LangGraphProbeFidelityReportSchema = "syncfuzz.langgraph-probe-fidelity-report.v1"

const (
	LangGraphProbeFidelityAttemptSchema     = "syncfuzz.langgraph-probe-fidelity-attempt.v1"
	LangGraphProbeFidelityBatchReportSchema = "syncfuzz.langgraph-probe-fidelity-batch-report.v1"
)

type LangGraphProbeFidelityAttemptStatus string

const (
	LangGraphProbeFidelityAttemptAccepted               LangGraphProbeFidelityAttemptStatus = "accepted"
	LangGraphProbeFidelityAttemptRejectedSourceBaseline LangGraphProbeFidelityAttemptStatus = "rejected-source-baseline"
	LangGraphProbeFidelityAttemptExecutionFailed        LangGraphProbeFidelityAttemptStatus = "execution-failed"
)

// LangGraphProbeFidelityAttempt records one provider-facing attempt. Rejected
// baselines are retained as experiment data, but are never aggregated as a
// full/pruned pair.
type LangGraphProbeFidelityAttempt struct {
	SchemaVersion string                              `json:"schema_version"`
	AttemptIndex  int                                 `json:"attempt_index"`
	ArtifactRoot  string                              `json:"artifact_root"`
	Status        LangGraphProbeFidelityAttemptStatus `json:"status"`
	Reason        string                              `json:"reason,omitempty"`
	FailureStage  string                              `json:"failure_stage,omitempty"`
	LogArtifact   string                              `json:"log_artifact,omitempty"`
}

func (a LangGraphProbeFidelityAttempt) Validate() error {
	if a.SchemaVersion != LangGraphProbeFidelityAttemptSchema || a.AttemptIndex <= 0 || strings.TrimSpace(a.ArtifactRoot) == "" {
		return fmt.Errorf("LangGraph probe fidelity attempt is incomplete")
	}
	switch a.Status {
	case LangGraphProbeFidelityAttemptAccepted:
		if a.Reason != "" || a.FailureStage != "" || a.LogArtifact != "" {
			return fmt.Errorf("accepted LangGraph probe fidelity attempt must not record a failure")
		}
	case LangGraphProbeFidelityAttemptRejectedSourceBaseline:
		if strings.TrimSpace(a.Reason) == "" || strings.TrimSpace(a.FailureStage) == "" || strings.TrimSpace(a.LogArtifact) == "" {
			return fmt.Errorf("rejected LangGraph probe fidelity attempt requires reason, failure stage, and log artifact")
		}
	case LangGraphProbeFidelityAttemptExecutionFailed:
		if strings.TrimSpace(a.Reason) == "" || strings.TrimSpace(a.FailureStage) == "" || strings.TrimSpace(a.LogArtifact) == "" {
			return fmt.Errorf("failed LangGraph probe fidelity attempt requires reason, failure stage, and log artifact")
		}
	default:
		return fmt.Errorf("unsupported LangGraph probe fidelity attempt status %q", a.Status)
	}
	return nil
}

// LangGraphProbeFidelityBatchAttemptInput joins a recorded attempt with its
// full/pruned artifacts when and only when it was accepted.
type LangGraphProbeFidelityBatchAttemptInput struct {
	Attempt LangGraphProbeFidelityAttempt
	Trial   *LangGraphProbeFidelityTrialInput
}

// LangGraphProbeFidelityBatchReport makes the collection denominator explicit.
// Fidelity contains only accepted pairs; Attempts preserves all provider-facing
// runs, including invalid source baselines and execution failures.
type LangGraphProbeFidelityBatchReport struct {
	SchemaVersion               string                          `json:"schema_version"`
	TargetAcceptedTrials        int                             `json:"target_accepted_trials"`
	MaxAttempts                 int                             `json:"max_attempts"`
	AttemptCount                int                             `json:"attempt_count"`
	AcceptedTrialCount          int                             `json:"accepted_trial_count"`
	RejectedSourceBaselineCount int                             `json:"rejected_source_baseline_count"`
	ExecutionFailureCount       int                             `json:"execution_failure_count"`
	Complete                    bool                            `json:"complete"`
	Attempts                    []LangGraphProbeFidelityAttempt `json:"attempts"`
	Fidelity                    *LangGraphProbeFidelityReport   `json:"fidelity,omitempty"`
}

// LangGraphProbeFidelityTrialInput names the two recovery artifacts produced
// from one retained source runtime. It deliberately contains no live runtime
// handles, so a report remains reproducible after its source lease is released.
type LangGraphProbeFidelityTrialInput struct {
	ArtifactRoot    string
	FullPlan        LangGraphForkPlan
	PrunedPlan      LangGraphForkPlan
	FullExecution   ForkRecoverySetExecution
	PrunedExecution ForkRecoverySetExecution
}

// ProbeFidelityMetricSummary describes post-recovery passive samples. Each
// recovery set has one such sample per before/after/head query.
type ProbeFidelityMetricSummary struct {
	SampleCount           int    `json:"sample_count"`
	TotalDurationNS       uint64 `json:"total_duration_ns"`
	MeanDurationNS        uint64 `json:"mean_duration_ns"`
	MinDurationNS         uint64 `json:"min_duration_ns"`
	MaxDurationNS         uint64 `json:"max_duration_ns"`
	TotalScannedProcesses uint64 `json:"total_scanned_processes"`
	MeanScannedProcesses  uint64 `json:"mean_scanned_processes"`
	TotalScannedFDs       uint64 `json:"total_scanned_fds"`
	MeanScannedFDs        uint64 `json:"mean_scanned_fds"`
}

// ProbeFidelityModeSummary keeps both recovery-set verdicts and the individual
// query outcomes. The latter makes a pruned inconclusive result auditable
// instead of presenting it as a failed full-probe verdict.
type ProbeFidelityModeSummary struct {
	RecoverySetOutcomes map[string]int             `json:"recovery_set_outcomes"`
	QueryOutcomes       map[string]int             `json:"query_outcomes"`
	Metrics             ProbeFidelityMetricSummary `json:"metrics"`
}

type ProbeFidelityComparisonSummary struct {
	PairedTrials                    int `json:"paired_trials"`
	ExactLayerStateOriginMatches    int `json:"exact_layer_state_origin_matches"`
	FinalOutcomeMatches             int `json:"final_outcome_matches"`
	FullMultiplicityProofs          int `json:"full_multiplicity_proofs"`
	PrunedMultiplicityUnknownTrials int `json:"pruned_multiplicity_unknown_trials"`
}

// LangGraphProbeFidelityTrial records the immutable source identity used to
// establish that a full/pruned comparison is a valid pair. The per-mode
// aggregate is reported above; per-trial data remains available for auditing.
type LangGraphProbeFidelityTrial struct {
	ArtifactRoot               string `json:"artifact_root"`
	RecordedPlanID             string `json:"recorded_plan_id"`
	SourceRuntimeID            string `json:"source_runtime_id"`
	WorkspaceSnapshotSHA256    string `json:"workspace_snapshot_sha256"`
	CheckpointStoreSHA256      string `json:"checkpoint_store_sha256"`
	SourceThreadID             string `json:"source_thread_id"`
	BeforeNativeCheckpointID   string `json:"before_native_checkpoint_id"`
	AfterNativeCheckpointID    string `json:"after_native_checkpoint_id"`
	HeadNativeCheckpointID     string `json:"head_native_checkpoint_id"`
	FullOutcome                string `json:"full_outcome"`
	PrunedOutcome              string `json:"pruned_outcome"`
	ExactLayerStateOriginMatch bool   `json:"exact_layer_state_origin_match"`
	FullMultiplicityProven     bool   `json:"full_multiplicity_proven"`
	PrunedMultiplicityUnknown  bool   `json:"pruned_multiplicity_unknown"`
}

// LangGraphProbeFidelityReport is evidence for probe-cost comparisons, not a
// vulnerability report. A pruned observer is expected to retain layer/origin
// evidence while losing the full observer's multiplicity proof.
type LangGraphProbeFidelityReport struct {
	SchemaVersion string                         `json:"schema_version"`
	Trials        []LangGraphProbeFidelityTrial  `json:"trials"`
	Full          ProbeFidelityModeSummary       `json:"full"`
	Pruned        ProbeFidelityModeSummary       `json:"pruned"`
	Comparison    ProbeFidelityComparisonSummary `json:"comparison"`
}

// ReadLangGraphProbeFidelityTrial loads the standard full/pruned artifacts
// below one `synthesis-langgraph-v3-fidelity` trial directory.
func ReadLangGraphProbeFidelityTrial(root string) (LangGraphProbeFidelityTrialInput, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return LangGraphProbeFidelityTrialInput{}, fmt.Errorf("LangGraph probe fidelity trial root is required")
	}
	fullPlan, err := ReadLangGraphForkPlan(filepath.Join(root, "full", "langgraph-fork-plan.json"))
	if err != nil {
		return LangGraphProbeFidelityTrialInput{}, err
	}
	prunedPlan, err := ReadLangGraphForkPlan(filepath.Join(root, "pruned", "langgraph-fork-plan.json"))
	if err != nil {
		return LangGraphProbeFidelityTrialInput{}, err
	}
	fullExecution, err := ReadForkRecoverySetExecution(filepath.Join(root, "full", "recovery-set-execution.json"))
	if err != nil {
		return LangGraphProbeFidelityTrialInput{}, err
	}
	prunedExecution, err := ReadForkRecoverySetExecution(filepath.Join(root, "pruned", "recovery-set-execution.json"))
	if err != nil {
		return LangGraphProbeFidelityTrialInput{}, err
	}
	return LangGraphProbeFidelityTrialInput{
		ArtifactRoot:    root,
		FullPlan:        fullPlan,
		PrunedPlan:      prunedPlan,
		FullExecution:   fullExecution,
		PrunedExecution: prunedExecution,
	}, nil
}

// BuildLangGraphProbeFidelityReport verifies paired source identity before it
// aggregates data. A report rejects a pair that differs in any recovery input
// other than the probe mode and its isolated runtime root.
func BuildLangGraphProbeFidelityReport(inputs []LangGraphProbeFidelityTrialInput) (LangGraphProbeFidelityReport, error) {
	if len(inputs) == 0 {
		return LangGraphProbeFidelityReport{}, fmt.Errorf("at least one LangGraph probe fidelity trial is required")
	}
	report := LangGraphProbeFidelityReport{
		SchemaVersion: LangGraphProbeFidelityReportSchema,
		Trials:        make([]LangGraphProbeFidelityTrial, 0, len(inputs)),
		Full:          newProbeFidelityModeSummary(),
		Pruned:        newProbeFidelityModeSummary(),
	}
	for _, input := range inputs {
		trial, fullMetrics, prunedMetrics, err := buildLangGraphProbeFidelityTrial(input)
		if err != nil {
			return LangGraphProbeFidelityReport{}, err
		}
		report.Trials = append(report.Trials, trial)
		addModeExecution(&report.Full, input.FullExecution, fullMetrics)
		addModeExecution(&report.Pruned, input.PrunedExecution, prunedMetrics)
		report.Comparison.PairedTrials++
		if trial.ExactLayerStateOriginMatch {
			report.Comparison.ExactLayerStateOriginMatches++
		}
		if trial.FullOutcome == trial.PrunedOutcome {
			report.Comparison.FinalOutcomeMatches++
		}
		if trial.FullMultiplicityProven {
			report.Comparison.FullMultiplicityProofs++
		}
		if trial.PrunedMultiplicityUnknown {
			report.Comparison.PrunedMultiplicityUnknownTrials++
		}
	}
	finalizeMetricSummary(&report.Full.Metrics)
	finalizeMetricSummary(&report.Pruned.Metrics)
	sort.Slice(report.Trials, func(left, right int) bool {
		return report.Trials[left].ArtifactRoot < report.Trials[right].ArtifactRoot
	})
	return report, nil
}

// BuildLangGraphProbeFidelityBatchReport keeps rejected and failed attempts in
// the report while aggregating only source-valid full/pruned pairs.
func BuildLangGraphProbeFidelityBatchReport(targetAcceptedTrials, maxAttempts int, inputs []LangGraphProbeFidelityBatchAttemptInput) (LangGraphProbeFidelityBatchReport, error) {
	if targetAcceptedTrials <= 0 || maxAttempts < targetAcceptedTrials {
		return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("LangGraph probe fidelity batch requires positive target accepted trials and max attempts at least that target")
	}
	report := LangGraphProbeFidelityBatchReport{
		SchemaVersion:        LangGraphProbeFidelityBatchReportSchema,
		TargetAcceptedTrials: targetAcceptedTrials,
		MaxAttempts:          maxAttempts,
		Attempts:             make([]LangGraphProbeFidelityAttempt, 0, len(inputs)),
	}
	seenAttempts := make(map[int]struct{}, len(inputs))
	acceptedInputs := make([]LangGraphProbeFidelityTrialInput, 0, len(inputs))
	for _, input := range inputs {
		if err := input.Attempt.Validate(); err != nil {
			return LangGraphProbeFidelityBatchReport{}, err
		}
		if input.Attempt.AttemptIndex > maxAttempts {
			return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("LangGraph probe fidelity attempt %d exceeds max attempts %d", input.Attempt.AttemptIndex, maxAttempts)
		}
		if _, exists := seenAttempts[input.Attempt.AttemptIndex]; exists {
			return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("LangGraph probe fidelity batch has duplicate attempt index %d", input.Attempt.AttemptIndex)
		}
		seenAttempts[input.Attempt.AttemptIndex] = struct{}{}
		report.Attempts = append(report.Attempts, input.Attempt)
		switch input.Attempt.Status {
		case LangGraphProbeFidelityAttemptAccepted:
			if input.Trial == nil {
				return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("accepted LangGraph probe fidelity attempt %d has no paired artifacts", input.Attempt.AttemptIndex)
			}
			report.AcceptedTrialCount++
			acceptedInputs = append(acceptedInputs, *input.Trial)
		case LangGraphProbeFidelityAttemptRejectedSourceBaseline:
			if input.Trial != nil {
				return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("rejected LangGraph probe fidelity attempt %d must not contribute paired artifacts", input.Attempt.AttemptIndex)
			}
			report.RejectedSourceBaselineCount++
		case LangGraphProbeFidelityAttemptExecutionFailed:
			if input.Trial != nil {
				return LangGraphProbeFidelityBatchReport{}, fmt.Errorf("failed LangGraph probe fidelity attempt %d must not contribute paired artifacts", input.Attempt.AttemptIndex)
			}
			report.ExecutionFailureCount++
		}
	}
	sort.Slice(report.Attempts, func(left, right int) bool {
		return report.Attempts[left].AttemptIndex < report.Attempts[right].AttemptIndex
	})
	report.AttemptCount = len(report.Attempts)
	report.Complete = report.AcceptedTrialCount >= report.TargetAcceptedTrials
	if len(acceptedInputs) == 0 {
		return report, nil
	}
	fidelity, err := BuildLangGraphProbeFidelityReport(acceptedInputs)
	if err != nil {
		return LangGraphProbeFidelityBatchReport{}, err
	}
	report.Fidelity = &fidelity
	return report, nil
}

func buildLangGraphProbeFidelityTrial(input LangGraphProbeFidelityTrialInput) (LangGraphProbeFidelityTrial, []PassiveProbeMetrics, []PassiveProbeMetrics, error) {
	if strings.TrimSpace(input.ArtifactRoot) == "" {
		return LangGraphProbeFidelityTrial{}, nil, nil, fmt.Errorf("LangGraph probe fidelity trial has no artifact root")
	}
	if err := validateLangGraphProbeFidelityPlans(input.FullPlan, input.PrunedPlan); err != nil {
		return LangGraphProbeFidelityTrial{}, nil, nil, fmt.Errorf("LangGraph probe fidelity trial %q: %w", input.ArtifactRoot, err)
	}
	fullMetrics, err := validateFidelityExecution(input.FullExecution, input.FullPlan, LangGraphPassiveProbeFull)
	if err != nil {
		return LangGraphProbeFidelityTrial{}, nil, nil, fmt.Errorf("LangGraph probe fidelity trial %q full: %w", input.ArtifactRoot, err)
	}
	prunedMetrics, err := validateFidelityExecution(input.PrunedExecution, input.PrunedPlan, LangGraphPassiveProbePruned)
	if err != nil {
		return LangGraphProbeFidelityTrial{}, nil, nil, fmt.Errorf("LangGraph probe fidelity trial %q pruned: %w", input.ArtifactRoot, err)
	}
	if !sameRecoverySetCoordinates(input.FullExecution, input.PrunedExecution) {
		return LangGraphProbeFidelityTrial{}, nil, nil, fmt.Errorf("LangGraph full/pruned recovery executions do not share exact checkpoint coordinates")
	}
	before, after, head := input.FullExecution.Before, input.FullExecution.After, input.FullExecution.Head
	return LangGraphProbeFidelityTrial{
		ArtifactRoot:               input.ArtifactRoot,
		RecordedPlanID:             input.FullPlan.RecordedPlanID,
		SourceRuntimeID:            input.FullPlan.SourceRuntime.ContainerID,
		WorkspaceSnapshotSHA256:    input.FullPlan.WorkspaceSnapshot.WorkspaceSHA256,
		CheckpointStoreSHA256:      input.FullPlan.WorkspaceSnapshot.CheckpointStoreSHA256,
		SourceThreadID:             input.FullPlan.SourceThreadID,
		BeforeNativeCheckpointID:   input.FullPlan.CheckpointCoordinates[before.CheckpointID].SourceCheckpointID,
		AfterNativeCheckpointID:    input.FullPlan.CheckpointCoordinates[after.CheckpointID].SourceCheckpointID,
		HeadNativeCheckpointID:     input.FullPlan.CheckpointCoordinates[head.CheckpointID].SourceCheckpointID,
		FullOutcome:                input.FullExecution.Classification.Outcome,
		PrunedOutcome:              input.PrunedExecution.Classification.Outcome,
		ExactLayerStateOriginMatch: sameLayerStateOrigin(input.FullExecution, input.PrunedExecution),
		FullMultiplicityProven:     allMultiplicity(input.FullExecution, EffectMultiplicitySingle),
		PrunedMultiplicityUnknown:  allMultiplicity(input.PrunedExecution, EffectMultiplicityUnknown),
	}, fullMetrics, prunedMetrics, nil
}

func validateLangGraphProbeFidelityPlans(full, pruned LangGraphForkPlan) error {
	if full.SchemaVersion != LangGraphForkPlanSchema || pruned.SchemaVersion != LangGraphForkPlanSchema {
		return fmt.Errorf("both plans must use schema %q", LangGraphForkPlanSchema)
	}
	if full.PassiveProbeMode.Effective() != LangGraphPassiveProbeFull || pruned.PassiveProbeMode.Effective() != LangGraphPassiveProbePruned {
		return fmt.Errorf("paired plans must be full and pruned passive probes")
	}
	if full.RecordedPlanID == "" || full.AdapterID != LangGraphForkAdapterID || full.TargetID == "" {
		return fmt.Errorf("full plan has no LangGraph recorded-plan identity")
	}
	if full.RecordedPlanID != pruned.RecordedPlanID || full.AdapterID != pruned.AdapterID || full.TargetID != pruned.TargetID || full.CandidateID != pruned.CandidateID || full.Task != pruned.Task || full.Model != pruned.Model || full.ContainerImage != pruned.ContainerImage || full.PassiveUnixSocketPath != pruned.PassiveUnixSocketPath || full.PassiveObservationID != pruned.PassiveObservationID || full.MaterializationHeadID != pruned.MaterializationHeadID || full.MaterializationHeadCheckpointID != pruned.MaterializationHeadCheckpointID || full.SourceThreadID != pruned.SourceThreadID {
		return fmt.Errorf("paired plans change recovery identity outside passive probe mode")
	}
	if !reflect.DeepEqual(full.SourceRuntime, pruned.SourceRuntime) || !reflect.DeepEqual(full.WorkspaceSnapshot, pruned.WorkspaceSnapshot) || !reflect.DeepEqual(full.UnixSocketProbe, pruned.UnixSocketProbe) || !reflect.DeepEqual(full.CheckpointCoordinates, pruned.CheckpointCoordinates) || !reflect.DeepEqual(full.AgentStateByCheckpoint, pruned.AgentStateByCheckpoint) {
		return fmt.Errorf("paired plans do not share source runtime, snapshot, listener identity, or checkpoint mapping")
	}
	return nil
}

func validateFidelityExecution(execution ForkRecoverySetExecution, plan LangGraphForkPlan, mode LangGraphPassiveProbeMode) ([]PassiveProbeMetrics, error) {
	if execution.SchemaVersion != ExecutionSchemaVersion || execution.RecordedPlanID != plan.RecordedPlanID || execution.RetentionPolicy != RetentionPolicyRetainRelevantOSState || execution.MaterializationHead.CheckpointID != plan.MaterializationHeadCheckpointID {
		return nil, fmt.Errorf("recovery execution does not match its recorded V3 plan")
	}
	metrics := make([]PassiveProbeMetrics, 0, 3)
	for _, observation := range []RecoveryObservation{execution.Before, execution.After, execution.Head} {
		if observation.RecordedPlanID != plan.RecordedPlanID || observation.CheckpointID == "" || observation.PassiveProbe == nil || observation.PassiveProbe.Mode != mode || !observation.PassiveProbe.Valid() {
			return nil, fmt.Errorf("recovery observation %q lacks %s passive probe metrics", observation.QueryID, mode)
		}
		if _, ok := plan.CheckpointCoordinates[observation.CheckpointID]; !ok {
			return nil, fmt.Errorf("recovery observation %q uses an unplanned checkpoint", observation.QueryID)
		}
		metrics = append(metrics, *observation.PassiveProbe)
	}
	return metrics, nil
}

func sameRecoverySetCoordinates(full, pruned ForkRecoverySetExecution) bool {
	return full.RecoverySetID == pruned.RecoverySetID && full.SeedID == pruned.SeedID && full.FrontierID == pruned.FrontierID && full.RecordedPlanID == pruned.RecordedPlanID && reflect.DeepEqual(full.MaterializationHead, pruned.MaterializationHead) && full.RetentionPolicy == pruned.RetentionPolicy && full.Before.CheckpointID == pruned.Before.CheckpointID && full.After.CheckpointID == pruned.After.CheckpointID && full.Head.CheckpointID == pruned.Head.CheckpointID
}

func sameLayerStateOrigin(full, pruned ForkRecoverySetExecution) bool {
	for index, left := range []RecoveryObservation{full.Before, full.After, full.Head} {
		right := []RecoveryObservation{pruned.Before, pruned.After, pruned.Head}[index]
		if left.AgentState != right.AgentState || left.OSState != right.OSState || left.OSStateOrigin != right.OSStateOrigin {
			return false
		}
	}
	return true
}

func allMultiplicity(execution ForkRecoverySetExecution, expected EffectMultiplicity) bool {
	for _, observation := range []RecoveryObservation{execution.Before, execution.After, execution.Head} {
		if observation.EffectMultiplicity != expected {
			return false
		}
	}
	return true
}

func newProbeFidelityModeSummary() ProbeFidelityModeSummary {
	return ProbeFidelityModeSummary{
		RecoverySetOutcomes: make(map[string]int),
		QueryOutcomes:       make(map[string]int),
	}
}

func addModeExecution(summary *ProbeFidelityModeSummary, execution ForkRecoverySetExecution, metrics []PassiveProbeMetrics) {
	summary.RecoverySetOutcomes[execution.Classification.Outcome]++
	for _, outcome := range []string{execution.Classification.BeforeOutcome, execution.Classification.AfterOutcome, execution.Classification.HeadOutcome} {
		summary.QueryOutcomes[outcome]++
	}
	for _, metric := range metrics {
		addPassiveProbeMetric(&summary.Metrics, metric)
	}
}

func addPassiveProbeMetric(summary *ProbeFidelityMetricSummary, metric PassiveProbeMetrics) {
	summary.SampleCount++
	if summary.SampleCount == 1 || metric.DurationNS < summary.MinDurationNS {
		summary.MinDurationNS = metric.DurationNS
	}
	if metric.DurationNS > summary.MaxDurationNS {
		summary.MaxDurationNS = metric.DurationNS
	}
	summary.TotalDurationNS += metric.DurationNS
	summary.TotalScannedProcesses += uint64(metric.ScannedProcesses)
	summary.TotalScannedFDs += uint64(metric.ScannedFDs)
}

func finalizeMetricSummary(summary *ProbeFidelityMetricSummary) {
	if summary.SampleCount == 0 {
		return
	}
	samples := uint64(summary.SampleCount)
	summary.MeanDurationNS = summary.TotalDurationNS / samples
	summary.MeanScannedProcesses = summary.TotalScannedProcesses / samples
	summary.MeanScannedFDs = summary.TotalScannedFDs / samples
}
