package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetMinimizationRunOptions struct {
	SourcePath     string
	OutDir         string
	CandidateLimit int
	MaxTrials      int
}

type TargetMinimizationRunResult struct {
	SchemaVersion      string                              `json:"schema_version"`
	MinimizationID     string                              `json:"minimization_id"`
	SourcePath         string                              `json:"source_path"`
	SourceSchema       string                              `json:"source_schema_version"`
	StartedAt          string                              `json:"started_at"`
	FinishedAt         string                              `json:"finished_at"`
	ArtifactDir        string                              `json:"artifact_dir"`
	CandidateLimit     int                                 `json:"candidate_limit,omitempty"`
	MaxTrials          int                                 `json:"max_trials"`
	ApplicablePlans    int                                 `json:"applicable_plans"`
	ExecutedCandidates int                                 `json:"executed_candidates"`
	TotalTrials        int                                 `json:"total_trials"`
	AcceptedReductions int                                 `json:"accepted_reductions"`
	Candidates         []TargetMinimizationExecutionResult `json:"candidates"`
}

type TargetMinimizationExecutionResult struct {
	CandidateID                 string                              `json:"candidate_id,omitempty"`
	OriginalRunID               string                              `json:"original_run_id,omitempty"`
	TargetID                    string                              `json:"target_id,omitempty"`
	TaskID                      string                              `json:"task_id"`
	OriginalPromptLines         int                                 `json:"original_prompt_lines"`
	MinimizedPromptLines        int                                 `json:"minimized_prompt_lines"`
	OriginalExecutionPlan       *target.TargetScenarioExecutionPlan `json:"original_execution_plan,omitempty"`
	MinimizedExecutionPlan      *target.TargetScenarioExecutionPlan `json:"minimized_execution_plan,omitempty"`
	Trials                      int                                 `json:"trials"`
	AcceptedReductions          int                                 `json:"accepted_reductions"`
	AcceptedPromptReductions    int                                 `json:"accepted_prompt_reductions,omitempty"`
	AcceptedExecutionReductions int                                 `json:"accepted_execution_reductions,omitempty"`
	AcceptedSteps               []string                            `json:"accepted_steps,omitempty"`
	Preserved                   bool                                `json:"preserved"`
	MinimizedRunID              string                              `json:"minimized_run_id,omitempty"`
	MinimizedArtifactDir        string                              `json:"minimized_artifact_dir,omitempty"`
	Error                       string                              `json:"error,omitempty"`
}

type targetExecutionPlanReducer struct {
	stepID string
	apply  func(*target.TargetScenarioExecutionPlan) bool
}

const targetMinimizationResultArtifact = "target-minimization-result.json"

func RunTargetMinimization(ctx context.Context, opts TargetMinimizationRunOptions) (*TargetMinimizationRunResult, error) {
	if strings.TrimSpace(opts.SourcePath) == "" {
		return nil, fmt.Errorf("source path is required")
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.CandidateLimit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}
	if opts.MaxTrials <= 0 {
		opts.MaxTrials = 32
	}

	sourceSchema, sourceResults, err := loadTargetMinimizationSource(opts.SourcePath)
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	minimizationID := fmt.Sprintf("target-minimize-run-%d", started.UnixNano())
	artifactDir := filepath.Join(opts.OutDir, minimizationID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target minimization run directory: %w", err)
	}

	result := &TargetMinimizationRunResult{
		SchemaVersion:  "syncfuzz.target-minimization-result.v1",
		MinimizationID: minimizationID,
		SourcePath:     opts.SourcePath,
		SourceSchema:   sourceSchema,
		StartedAt:      started.Format(time.RFC3339Nano),
		ArtifactDir:    artifactDir,
		CandidateLimit: opts.CandidateLimit,
		MaxTrials:      opts.MaxTrials,
		Candidates:     []TargetMinimizationExecutionResult{},
	}
	for _, source := range sourceResults {
		plan := targetMinimizationPlanForResult(source)
		if !plan.Applicable {
			continue
		}
		result.ApplicablePlans++
		if opts.CandidateLimit > 0 && result.ExecutedCandidates >= opts.CandidateLimit {
			continue
		}
		item := runTargetPromptMinimization(ctx, artifactDir, source, opts.MaxTrials)
		result.ExecutedCandidates++
		result.TotalTrials += item.Trials
		result.AcceptedReductions += item.AcceptedReductions
		result.Candidates = append(result.Candidates, item)
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := core.WriteJSON(filepath.Join(artifactDir, targetMinimizationResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func runTargetPromptMinimization(ctx context.Context, outDir string, source TargetSuiteRunResult, maxTrials int) TargetMinimizationExecutionResult {
	item := TargetMinimizationExecutionResult{
		CandidateID:   source.CandidateID,
		OriginalRunID: source.RunID,
		TargetID:      source.TargetID,
		TaskID:        source.TaskID,
	}
	task, err := readTargetTaskForMinimization(source.ArtifactDir)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	lines := targetPromptReductionLines(task.Prompt)
	item.OriginalPromptLines = len(lines)
	if len(lines) == 0 {
		item.Error = "source target prompt is empty"
		return item
	}

	current := append([]string{}, lines...)
	currentPlan := targetExecutionPlanForMinimization(task)
	item.OriginalExecutionPlan = cloneTargetExecutionPlan(currentPlan)
	var lastAccepted *target.TargetRunResult
	completedTrials := 0
	lastTrialError := ""
	for index := 0; index < len(current) && item.Trials < maxTrials; {
		if len(current) == 1 {
			break
		}
		trialLines := append([]string{}, current[:index]...)
		trialLines = append(trialLines, current[index+1:]...)
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(trialLines, "\n"), currentPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial) {
			current = trialLines
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedPromptReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, fmt.Sprintf("prompt-line-delete:%d", index+1))
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
		index++
	}
	for _, reducer := range targetExecutionPlanReducers() {
		if item.Trials >= maxTrials || currentPlan == nil {
			break
		}
		trialPlan := cloneTargetExecutionPlan(currentPlan)
		if !reducer.apply(trialPlan) {
			continue
		}
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), trialPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial) {
			currentPlan = trialPlan
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedExecutionReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, reducer.stepID)
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
	}
	item.MinimizedPromptLines = len(current)
	item.MinimizedExecutionPlan = cloneTargetExecutionPlan(currentPlan)
	if lastAccepted != nil {
		item.Preserved = true
		item.MinimizedRunID = lastAccepted.RunID
		item.MinimizedArtifactDir = lastAccepted.ArtifactDir
		return item
	}
	if item.Trials >= maxTrials {
		if completedTrials == 0 {
			item.Error = lastTrialError
			if item.Error == "" {
				item.Error = "no minimization trial completed"
			}
			return item
		}
		item.Preserved = true
		item.MinimizedRunID = source.RunID
		item.MinimizedArtifactDir = source.ArtifactDir
		return item
	}

	trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), currentPlan)
	item.Trials++
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.Preserved = targetMinimizationPreserved(source, trial)
	item.MinimizedRunID = trial.RunID
	item.MinimizedArtifactDir = trial.ArtifactDir
	if !item.Preserved {
		item.Error = "final prompt did not preserve the source oracle constraints"
	}
	return item
}

func recordTargetMinimizationTrialOutcome(completed int, lastError string, trial *target.TargetRunResult, err error) (int, string) {
	if err != nil {
		return completed, err.Error()
	}
	if trial == nil {
		return completed, "minimization trial produced no result"
	}
	if !trial.Completed {
		if trial.CommandResult.Error != "" {
			return completed, trial.CommandResult.Error
		}
		return completed, "minimization trial did not complete"
	}
	return completed + 1, lastError
}

func readTargetTaskForMinimization(artifactDir string) (target.TargetTask, error) {
	path := filepath.Join(strings.TrimSpace(artifactDir), target.TargetTaskArtifact)
	data, err := os.ReadFile(path)
	if err != nil {
		return target.TargetTask{}, fmt.Errorf("read source target task %s: %w", path, err)
	}
	var task target.TargetTask
	if err := json.Unmarshal(data, &task); err != nil {
		return target.TargetTask{}, fmt.Errorf("decode source target task %s: %w", path, err)
	}
	if strings.TrimSpace(task.Command) == "" {
		return target.TargetTask{}, fmt.Errorf("source target task %s has no command", path)
	}
	return task, nil
}

func runTargetMinimizationTrial(ctx context.Context, outDir string, task target.TargetTask, prompt string, executionPlan *target.TargetScenarioExecutionPlan) (*target.TargetRunResult, error) {
	return target.RunTarget(ctx, target.TargetRunOptions{
		AdapterID:        task.AdapterID,
		TargetID:         task.TargetID,
		TaskID:           task.TaskID,
		Objective:        task.Objective,
		Scenario:         target.CloneTargetScenarioInfo(task.Scenario),
		ExecutionPlan:    cloneTargetExecutionPlan(executionPlan),
		PromptProfileID:  task.PromptProfileID,
		PromptVariantID:  task.PromptVariantID,
		Prompt:           prompt,
		Command:          task.Command,
		OutDir:           outDir,
		Timeout:          time.Duration(task.TimeoutMillis) * time.Millisecond,
		ObserveDelay:     time.Duration(task.ObserveDelayMs) * time.Millisecond,
		LateObserveDelay: time.Duration(task.LateObserveDelayMs) * time.Millisecond,
		EnvKind:          task.Environment,
		ContainerImage:   task.ContainerImage,
		ExpectedFiles:    append([]string{}, task.ExpectedFiles...),
	})
}

func targetExecutionPlanForMinimization(task target.TargetTask) *target.TargetScenarioExecutionPlan {
	if task.Scenario == nil {
		return nil
	}
	return cloneTargetExecutionPlan(task.Scenario.ExecutionPlan)
}

func targetExecutionPlanReducers() []targetExecutionPlanReducer {
	return []targetExecutionPlanReducer{
		{
			stepID: "execution-plan-clear-process-mode",
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				if plan == nil || plan.ProcessMode == "" {
					return false
				}
				plan.ProcessMode = ""
				return true
			},
		},
		{
			stepID: "execution-plan-clear-checkpoint-backend",
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				if plan == nil || plan.CheckpointBackend == "" {
					return false
				}
				plan.CheckpointBackend = ""
				return true
			},
		},
		{
			stepID: "execution-plan-clear-checkpoint-selector",
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				if plan == nil || plan.CheckpointSelector == "" {
					return false
				}
				plan.CheckpointSelector = ""
				return true
			},
		},
		{
			stepID: "execution-plan-remove-fork-followup",
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				if plan == nil || (!plan.ForkFollowup && plan.ForkMessage == "") {
					return false
				}
				plan.ForkFollowup = false
				plan.ForkMessage = ""
				return true
			},
		},
		{
			stepID: "execution-plan-disable-replay",
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				if plan == nil || !plan.Replay {
					return false
				}
				plan.Replay = false
				return true
			},
		},
	}
}

func targetPromptReductionLines(prompt string) []string {
	rawLines := strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n")
	start := 0
	for start < len(rawLines) && strings.TrimSpace(rawLines[start]) == "" {
		start++
	}
	end := len(rawLines)
	for end > start && strings.TrimSpace(rawLines[end-1]) == "" {
		end--
	}
	return append([]string{}, rawLines[start:end]...)
}

func targetMinimizationPreserved(source TargetSuiteRunResult, trial *target.TargetRunResult) bool {
	if trial == nil || !trial.Completed {
		return false
	}
	wantStatus := source.TargetOracle.Status
	if wantStatus == "" && source.Confirmed {
		wantStatus = target.TargetOracleStatusConfirmed
	}
	gotStatus := trial.TargetOracle.Status
	if gotStatus == "" && trial.TargetOracle.Confirmed {
		gotStatus = target.TargetOracleStatusConfirmed
	}
	if wantStatus != "" && gotStatus != wantStatus {
		return false
	}
	if source.TargetOracle.Attribution != "" && trial.TargetOracle.Attribution != source.TargetOracle.Attribution {
		return false
	}
	if source.Signature != (core.MismatchSignature{}) && trial.Signature != source.Signature {
		return false
	}
	if source.TaskCompliance.Status != "" && trial.TaskCompliance.Status != source.TaskCompliance.Status {
		return false
	}
	wantContract := target.TargetContractInterpretationStatusValue(source.ContractInterpretation)
	if wantContract != "" && target.TargetContractInterpretationStatusValue(trial.ContractInterpretation) != wantContract {
		return false
	}
	return true
}
