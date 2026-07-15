package corpus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type ReplayOptions struct {
	CorpusDir       string
	EntryID         string
	OutDir          string
	Delay           time.Duration
	MockURL         string
	EnvKind         string
	ContainerImage  string
	FaultPlanID     string
	PrimitiveID     string
	TimingProfileID string
}

type ReplayResult struct {
	ExecutionKind        string                                    `json:"execution_kind"`
	ReplayID             string                                    `json:"replay_id"`
	EntryID              string                                    `json:"entry_id"`
	CaseName             string                                    `json:"case_name"`
	AdapterID            string                                    `json:"adapter_id,omitempty"`
	TargetID             string                                    `json:"target_id,omitempty"`
	TaskID               string                                    `json:"task_id,omitempty"`
	PromptProfileID      string                                    `json:"prompt_profile_id,omitempty"`
	PromptVariantID      string                                    `json:"prompt_variant_id,omitempty"`
	Environment          string                                    `json:"environment"`
	ContainerImage       string                                    `json:"container_image,omitempty"`
	FaultPlanID          string                                    `json:"fault_plan_id,omitempty"`
	PrimitiveID          string                                    `json:"primitive_id,omitempty"`
	TimingProfileID      string                                    `json:"timing_profile_id,omitempty"`
	SourceSuiteID        string                                    `json:"source_suite_id"`
	SourceRunID          string                                    `json:"source_run_id"`
	ExpectedSignature    core.MismatchSignature                    `json:"expected_signature"`
	RunID                string                                    `json:"run_id"`
	Confirmed            bool                                      `json:"confirmed"`
	ActualSignature      core.MismatchSignature                    `json:"actual_signature"`
	SignatureMatched     bool                                      `json:"signature_matched"`
	Reproduced           bool                                      `json:"reproduced"`
	OutcomeCategory      ReplayOutcomeCategory                     `json:"outcome_category,omitempty"`
	OutcomeReason        string                                    `json:"outcome_reason,omitempty"`
	TargetOracleStatus   target.TargetOracleStatus                 `json:"target_oracle_status,omitempty"`
	TargetAttribution    string                                    `json:"target_attribution,omitempty"`
	TaskComplianceStatus target.TargetTaskComplianceStatus         `json:"task_compliance_status,omitempty"`
	ContractStatus       target.TargetContractInterpretationStatus `json:"contract_status,omitempty"`
	ArtifactDir          string                                    `json:"artifact_dir"`
	RunArtifactDir       string                                    `json:"run_artifact_dir"`
	StartedAt            string                                    `json:"started_at"`
	FinishedAt           string                                    `json:"finished_at"`
}

// ReplayCorpusEntry turns a compact corpus handle back into an executable
// testcase. Reproduction currently means: the run confirms and emits the same
// mismatch signature as the corpus entry.
func ReplayCorpusEntry(ctx context.Context, opts ReplayOptions) (*ReplayResult, error) {
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if err := environment.ValidateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	entry, err := ShowCorpusEntry(opts.CorpusDir, opts.EntryID)
	if err != nil {
		return nil, err
	}

	return replayEntry(ctx, *entry, opts)
}

func replayEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions) (*ReplayResult, error) {
	started := time.Now().UTC()
	replayID := fmt.Sprintf("replay-%d", started.UnixNano())
	replayDir := filepath.Join(opts.OutDir, replayID)
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		return nil, fmt.Errorf("create replay directory: %w", err)
	}

	switch entry.EffectiveExecutionKind() {
	case CorpusExecutionTarget:
		return replayTargetEntry(ctx, entry, opts, replayID, replayDir, started)
	default:
		return replayCaseEntry(ctx, entry, opts, replayID, replayDir, started)
	}
}

func replayCaseEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions, replayID string, replayDir string, started time.Time) (*ReplayResult, error) {
	runResult, err := cases.Run(ctx, core.RunOptions{
		CaseName:        entry.CaseName,
		OutDir:          replayDir,
		Delay:           opts.Delay,
		MockURL:         opts.MockURL,
		EnvKind:         opts.EnvKind,
		ContainerImage:  opts.ContainerImage,
		FaultPlanID:     core.FirstNonEmpty(opts.FaultPlanID, entry.FaultPlanID),
		PrimitiveID:     core.FirstNonEmpty(opts.PrimitiveID, entry.PrimitiveID),
		TimingProfileID: core.FirstNonEmpty(opts.TimingProfileID, entry.TimingProfileID),
	})
	if err != nil {
		return nil, err
	}

	signatureMatched := runResult.Signature.String() == entry.Signature.String()
	outcome := ClassifyCaseReplayOutcome(runResult, signatureMatched)
	finished := time.Now().UTC()
	result := &ReplayResult{
		ExecutionKind:     entry.EffectiveExecutionKind(),
		ReplayID:          replayID,
		EntryID:           entry.EntryID,
		CaseName:          entry.CaseName,
		Environment:       runResult.Environment,
		ContainerImage:    runResult.ContainerImage,
		FaultPlanID:       runResult.FaultPlanID,
		PrimitiveID:       runResult.PrimitiveID,
		TimingProfileID:   runResult.TimingProfileID,
		SourceSuiteID:     entry.SuiteID,
		SourceRunID:       entry.RunID,
		ExpectedSignature: entry.Signature,
		RunID:             runResult.RunID,
		Confirmed:         runResult.Confirmed,
		ActualSignature:   runResult.Signature,
		SignatureMatched:  signatureMatched,
		Reproduced:        runResult.Confirmed && signatureMatched,
		OutcomeCategory:   outcome.Category,
		OutcomeReason:     outcome.Reason,
		ArtifactDir:       replayDir,
		RunArtifactDir:    runResult.ArtifactDir,
		StartedAt:         started.Format(time.RFC3339Nano),
		FinishedAt:        finished.Format(time.RFC3339Nano),
	}
	if err := core.WriteJSON(filepath.Join(replayDir, "replay-result.json"), result); err != nil {
		return nil, err
	}
	return result, nil
}

func replayTargetEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions, replayID string, replayDir string, started time.Time) (*ReplayResult, error) {
	task, err := loadTargetTask(filepath.Join(entry.ArtifactDir, target.TargetTaskArtifact))
	if err != nil {
		return nil, err
	}
	var executionPlan *target.TargetScenarioExecutionPlan
	if task.Scenario != nil && task.Scenario.ExecutionPlan != nil {
		plan := *task.Scenario.ExecutionPlan
		executionPlan = &plan
	}

	runResult, err := target.RunTarget(ctx, target.TargetRunOptions{
		AdapterID:        task.AdapterID,
		TargetID:         task.TargetID,
		TaskID:           task.TaskID,
		Objective:        task.Objective,
		Scenario:         target.CloneTargetScenarioInfo(task.Scenario),
		ExecutionPlan:    executionPlan,
		PromptProfileID:  task.PromptProfileID,
		PromptVariantID:  task.PromptVariantID,
		Prompt:           task.Prompt,
		Command:          task.Command,
		OutDir:           replayDir,
		Timeout:          time.Duration(task.TimeoutMillis) * time.Millisecond,
		ObserveDelay:     time.Duration(task.ObserveDelayMs) * time.Millisecond,
		LateObserveDelay: time.Duration(task.LateObserveDelayMs) * time.Millisecond,
		EnvKind:          core.FirstNonEmpty(opts.EnvKind, task.Environment),
		ContainerImage:   core.FirstNonEmpty(opts.ContainerImage, task.ContainerImage),
		ExpectedFiles:    append([]string{}, task.ExpectedFiles...),
	})
	if err != nil {
		return nil, err
	}

	signatureMatched := runResult.Signature.String() == entry.Signature.String()
	outcome := ClassifyTargetReplayOutcome(runResult, signatureMatched)
	finished := time.Now().UTC()
	result := &ReplayResult{
		ExecutionKind:        entry.EffectiveExecutionKind(),
		ReplayID:             replayID,
		EntryID:              entry.EntryID,
		CaseName:             entry.Subject(),
		AdapterID:            runResult.AdapterID,
		TargetID:             runResult.TargetID,
		TaskID:               runResult.TaskID,
		PromptProfileID:      runResult.PromptProfileID,
		PromptVariantID:      runResult.PromptVariantID,
		Environment:          runResult.Environment,
		ContainerImage:       runResult.ContainerImage,
		SourceSuiteID:        entry.SuiteID,
		SourceRunID:          entry.RunID,
		ExpectedSignature:    entry.Signature,
		RunID:                runResult.RunID,
		Confirmed:            runResult.ExpectationsMet,
		ActualSignature:      runResult.Signature,
		SignatureMatched:     signatureMatched,
		Reproduced:           runResult.ExpectationsMet && signatureMatched,
		OutcomeCategory:      outcome.Category,
		OutcomeReason:        outcome.Reason,
		TargetOracleStatus:   runResult.TargetOracle.Status,
		TargetAttribution:    runResult.TargetOracle.Attribution,
		TaskComplianceStatus: runResult.TaskCompliance.Status,
		ContractStatus:       target.TargetContractInterpretationStatusValue(runResult.ContractInterpretation),
		ArtifactDir:          replayDir,
		RunArtifactDir:       runResult.ArtifactDir,
		StartedAt:            started.Format(time.RFC3339Nano),
		FinishedAt:           finished.Format(time.RFC3339Nano),
	}
	if err := core.WriteJSON(filepath.Join(replayDir, "replay-result.json"), result); err != nil {
		return nil, err
	}
	return result, nil
}

func loadTargetTask(path string) (target.TargetTask, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return target.TargetTask{}, fmt.Errorf("read target task %q: %w", path, err)
	}
	var task target.TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return target.TargetTask{}, fmt.Errorf("decode target task %q: %w", path, err)
	}
	return task, nil
}
