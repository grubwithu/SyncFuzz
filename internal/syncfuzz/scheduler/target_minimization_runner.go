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
	Fidelity       TargetMinimizationFidelity
}

type TargetMinimizationFidelity string

const (
	TargetMinimizationFidelityExact    TargetMinimizationFidelity = "exact"
	TargetMinimizationFidelitySemantic TargetMinimizationFidelity = "semantic"
	TargetMinimizationFidelityImpact   TargetMinimizationFidelity = "impact"
)

type TargetMinimizationRunResult struct {
	SchemaVersion      string                              `json:"schema_version"`
	MinimizationID     string                              `json:"minimization_id"`
	SourcePath         string                              `json:"source_path"`
	SourceSchema       string                              `json:"source_schema_version"`
	Fidelity           TargetMinimizationFidelity          `json:"fidelity"`
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
	CandidateID                  string                              `json:"candidate_id,omitempty"`
	OriginalRunID                string                              `json:"original_run_id,omitempty"`
	TargetID                     string                              `json:"target_id,omitempty"`
	TaskID                       string                              `json:"task_id"`
	OriginalPromptLines          int                                 `json:"original_prompt_lines"`
	MinimizedPromptLines         int                                 `json:"minimized_prompt_lines"`
	OriginalCommandLines         int                                 `json:"original_command_lines,omitempty"`
	MinimizedCommandLines        int                                 `json:"minimized_command_lines,omitempty"`
	OriginalComponents           int                                 `json:"original_components,omitempty"`
	MinimizedComponents          int                                 `json:"minimized_components,omitempty"`
	OriginalMutations            int                                 `json:"original_mutations,omitempty"`
	MinimizedMutations           int                                 `json:"minimized_mutations,omitempty"`
	OriginalExecutionPlan        *target.TargetScenarioExecutionPlan `json:"original_execution_plan,omitempty"`
	MinimizedExecutionPlan       *target.TargetScenarioExecutionPlan `json:"minimized_execution_plan,omitempty"`
	Trials                       int                                 `json:"trials"`
	AcceptedReductions           int                                 `json:"accepted_reductions"`
	AcceptedPromptReductions     int                                 `json:"accepted_prompt_reductions,omitempty"`
	AcceptedCommandReductions    int                                 `json:"accepted_command_reductions,omitempty"`
	AcceptedComponentReductions  int                                 `json:"accepted_component_reductions,omitempty"`
	AcceptedMutationReductions   int                                 `json:"accepted_mutation_reductions,omitempty"`
	AcceptedActivationReductions int                                 `json:"accepted_activation_reductions,omitempty"`
	AcceptedExecutionReductions  int                                 `json:"accepted_execution_reductions,omitempty"`
	AcceptedSteps                []string                            `json:"accepted_steps,omitempty"`
	Preserved                    bool                                `json:"preserved"`
	MinimizedRunID               string                              `json:"minimized_run_id,omitempty"`
	MinimizedArtifactDir         string                              `json:"minimized_artifact_dir,omitempty"`
	Error                        string                              `json:"error,omitempty"`
}

type targetExecutionPlanReducer struct {
	stepID string
	apply  func(*target.TargetScenarioExecutionPlan) bool
}

type targetScenarioComponentReducer struct {
	stepID string
	apply  func(*target.TargetScenarioInfo, *target.TargetScenarioExecutionPlan) bool
}

type targetScenarioMutationReducer struct {
	stepID string
	apply  func(*target.TargetScenarioInfo) bool
}

type targetActivationCommandReducer struct {
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
	fidelity, err := normalizeTargetMinimizationFidelity(opts.Fidelity)
	if err != nil {
		return nil, err
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
		Fidelity:       fidelity,
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
		item := runTargetPromptMinimization(ctx, artifactDir, source, opts.MaxTrials, fidelity)
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

func normalizeTargetMinimizationFidelity(value TargetMinimizationFidelity) (TargetMinimizationFidelity, error) {
	switch strings.TrimSpace(string(value)) {
	case "", string(TargetMinimizationFidelityExact):
		return TargetMinimizationFidelityExact, nil
	case string(TargetMinimizationFidelitySemantic):
		return TargetMinimizationFidelitySemantic, nil
	case string(TargetMinimizationFidelityImpact):
		return TargetMinimizationFidelityImpact, nil
	default:
		return "", fmt.Errorf("unsupported target minimization fidelity %q", value)
	}
}

func runTargetPromptMinimization(ctx context.Context, outDir string, source TargetSuiteRunResult, maxTrials int, fidelity TargetMinimizationFidelity) TargetMinimizationExecutionResult {
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
	commandLines := targetPromptReductionLines(task.Command)
	currentCommand := append([]string{}, commandLines...)
	item.OriginalCommandLines = len(commandLines)
	currentScenario := target.CloneTargetScenarioInfo(task.Scenario)
	item.OriginalComponents = targetScenarioComponentCount(currentScenario)
	item.OriginalMutations = targetScenarioMutationCount(currentScenario)
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
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(trialLines, "\n"), strings.Join(currentCommand, "\n"), currentScenario, currentPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
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
	for index := 0; index < len(currentCommand) && item.Trials < maxTrials; {
		if len(currentCommand) == 1 {
			break
		}
		trialCommand := append([]string{}, currentCommand[:index]...)
		trialCommand = append(trialCommand, currentCommand[index+1:]...)
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(trialCommand, "\n"), currentScenario, currentPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
			currentCommand = trialCommand
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedCommandReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, fmt.Sprintf("command-line-delete:%d", index+1))
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
		index++
	}
	plan := targetMinimizationPlanForResult(source)
	for _, reducer := range targetScenarioComponentReducers(plan, currentScenario, fidelity) {
		if item.Trials >= maxTrials || currentScenario == nil {
			break
		}
		trialScenario := target.CloneTargetScenarioInfo(currentScenario)
		trialPlan := cloneTargetExecutionPlan(currentPlan)
		if !reducer.apply(trialScenario, trialPlan) {
			continue
		}
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(currentCommand, "\n"), trialScenario, trialPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
			currentScenario = trialScenario
			currentPlan = trialPlan
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedComponentReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, reducer.stepID)
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
	}
	for _, reducer := range targetScenarioMutationReducers(plan, currentScenario) {
		if item.Trials >= maxTrials || currentScenario == nil {
			break
		}
		trialScenario := target.CloneTargetScenarioInfo(currentScenario)
		if !reducer.apply(trialScenario) {
			continue
		}
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(currentCommand, "\n"), trialScenario, currentPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
			currentScenario = trialScenario
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedMutationReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, reducer.stepID)
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
	}
	for _, reducer := range targetActivationCommandReducers(plan, currentPlan) {
		if item.Trials >= maxTrials || currentPlan == nil {
			break
		}
		trialPlan := cloneTargetExecutionPlan(currentPlan)
		if !reducer.apply(trialPlan) {
			continue
		}
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(currentCommand, "\n"), currentScenario, trialPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
			currentPlan = trialPlan
			lastAccepted = trial
			item.AcceptedReductions++
			item.AcceptedActivationReductions++
			item.AcceptedSteps = append(item.AcceptedSteps, reducer.stepID)
			continue
		}
		if ctx.Err() != nil {
			item.Error = ctx.Err().Error()
			return item
		}
	}
	for _, reducer := range targetExecutionPlanReducers() {
		if item.Trials >= maxTrials || currentPlan == nil {
			break
		}
		trialPlan := cloneTargetExecutionPlan(currentPlan)
		if !reducer.apply(trialPlan) {
			continue
		}
		trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(currentCommand, "\n"), currentScenario, trialPlan)
		item.Trials++
		completedTrials, lastTrialError = recordTargetMinimizationTrialOutcome(completedTrials, lastTrialError, trial, err)
		if err == nil && targetMinimizationPreserved(source, trial, fidelity) {
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
	item.MinimizedCommandLines = len(currentCommand)
	item.MinimizedComponents = targetScenarioComponentCount(currentScenario)
	item.MinimizedMutations = targetScenarioMutationCount(currentScenario)
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

	trial, err := runTargetMinimizationTrial(ctx, outDir, task, strings.Join(current, "\n"), strings.Join(currentCommand, "\n"), currentScenario, currentPlan)
	item.Trials++
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.Preserved = targetMinimizationPreserved(source, trial, fidelity)
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

func runTargetMinimizationTrial(ctx context.Context, outDir string, task target.TargetTask, prompt string, command string, scenario *target.TargetScenarioInfo, executionPlan *target.TargetScenarioExecutionPlan) (*target.TargetRunResult, error) {
	return target.RunTarget(ctx, target.TargetRunOptions{
		AdapterID:        task.AdapterID,
		TargetID:         task.TargetID,
		TaskID:           task.TaskID,
		Objective:        task.Objective,
		Scenario:         target.CloneTargetScenarioInfo(scenario),
		ExecutionPlan:    cloneTargetExecutionPlan(executionPlan),
		PromptProfileID:  task.PromptProfileID,
		PromptVariantID:  task.PromptVariantID,
		Prompt:           prompt,
		Command:          command,
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

func targetActivationCommandReducers(plan TargetMinimizationPlan, executionPlan *target.TargetScenarioExecutionPlan) []targetActivationCommandReducer {
	if executionPlan == nil || strings.TrimSpace(executionPlan.ForkMessage) == "" {
		return nil
	}
	hasActivationStep := false
	for _, step := range plan.Steps {
		if step.Kind == "activation-minimization" {
			hasActivationStep = true
			break
		}
	}
	if !hasActivationStep {
		return nil
	}
	lines := targetPromptReductionLines(executionPlan.ForkMessage)
	if len(lines) <= 1 {
		return nil
	}
	reducers := make([]targetActivationCommandReducer, 0, len(lines))
	for index := range lines {
		lineIndex := index
		reducers = append(reducers, targetActivationCommandReducer{
			stepID: fmt.Sprintf("activation-command-line-delete:%d", lineIndex+1),
			apply: func(plan *target.TargetScenarioExecutionPlan) bool {
				return targetExecutionPlanDeleteForkMessageLine(plan, lineIndex)
			},
		})
	}
	return reducers
}

func targetExecutionPlanDeleteForkMessageLine(plan *target.TargetScenarioExecutionPlan, lineIndex int) bool {
	if plan == nil || lineIndex < 0 {
		return false
	}
	lines := targetPromptReductionLines(plan.ForkMessage)
	if len(lines) <= 1 || lineIndex >= len(lines) {
		return false
	}
	next := append([]string{}, lines[:lineIndex]...)
	next = append(next, lines[lineIndex+1:]...)
	plan.ForkMessage = strings.Join(next, "\n")
	return true
}

func targetScenarioMutationReducers(plan TargetMinimizationPlan, scenario *target.TargetScenarioInfo) []targetScenarioMutationReducer {
	if scenario == nil || len(scenario.Mutations) == 0 {
		return nil
	}
	mutationsByID := make(map[string]target.TargetScenarioMutation, len(scenario.Mutations))
	for _, mutation := range scenario.Mutations {
		mutationsByID[mutation.MutationID] = mutation
	}
	reducers := []targetScenarioMutationReducer{}
	seen := make(map[string]struct{})
	for _, step := range plan.Steps {
		if step.Kind != "mutation-axis-check" {
			continue
		}
		mutation, ok := targetScenarioMutationForStep(step, scenario, mutationsByID, seen)
		if !ok {
			continue
		}
		mutationID := strings.TrimSpace(mutation.MutationID)
		seen[mutationID] = struct{}{}
		reducers = append(reducers, targetScenarioMutationReducer{
			stepID: "mutation-delete:" + mutationID,
			apply: func(scenario *target.TargetScenarioInfo) bool {
				return targetScenarioDeleteMutation(scenario, mutationID)
			},
		})
	}
	return reducers
}

func targetScenarioMutationForStep(step TargetMinimizationStep, scenario *target.TargetScenarioInfo, mutationsByID map[string]target.TargetScenarioMutation, seen map[string]struct{}) (target.TargetScenarioMutation, bool) {
	mutationID := strings.TrimSpace(step.MutationID)
	if mutationID != "" {
		mutation, ok := mutationsByID[mutationID]
		if !ok {
			return target.TargetScenarioMutation{}, false
		}
		if _, exists := seen[mutationID]; exists {
			return target.TargetScenarioMutation{}, false
		}
		return mutation, true
	}
	for _, mutation := range scenario.Mutations {
		id := strings.TrimSpace(mutation.MutationID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		if step.MutationKind != "" && mutation.Kind != step.MutationKind {
			continue
		}
		return mutation, true
	}
	return target.TargetScenarioMutation{}, false
}

func targetScenarioComponentReducers(plan TargetMinimizationPlan, scenario *target.TargetScenarioInfo, fidelity TargetMinimizationFidelity) []targetScenarioComponentReducer {
	if scenario == nil || len(scenario.Components) == 0 {
		return nil
	}
	normalizedFidelity, err := normalizeTargetMinimizationFidelity(fidelity)
	if err != nil {
		return nil
	}
	componentsByID := make(map[string]target.TargetScenarioComponent, len(scenario.Components))
	for _, component := range scenario.Components {
		componentsByID[component.ComponentID] = component
	}
	reducers := []targetScenarioComponentReducer{}
	for _, step := range plan.Steps {
		componentID := strings.TrimSpace(step.ComponentID)
		if componentID == "" {
			continue
		}
		component, ok := componentsByID[componentID]
		if !ok {
			continue
		}
		reducers = append(reducers, targetScenarioComponentReducersForStep(step, component, scenario, normalizedFidelity)...)
	}
	return reducers
}

func targetScenarioComponentReducersForStep(step TargetMinimizationStep, component target.TargetScenarioComponent, scenario *target.TargetScenarioInfo, fidelity TargetMinimizationFidelity) []targetScenarioComponentReducer {
	componentID := strings.TrimSpace(component.ComponentID)
	reducers := []targetScenarioComponentReducer{}
	switch {
	case step.Kind == "component-deletion" && targetScenarioOptionalComponentDeletionAllowed(component):
		reducers = append(reducers, targetScenarioComponentReducer{
			stepID: "component-delete:" + componentID,
			apply: func(scenario *target.TargetScenarioInfo, _ *target.TargetScenarioExecutionPlan) bool {
				return targetScenarioDeleteComponent(scenario, componentID)
			},
		})
	case step.Kind == "primitive-minimization" && targetScenarioPlantMetadataReductionAllowed(component, scenario, fidelity):
		kindID := strings.TrimSpace(component.KindID)
		reducers = append(reducers, targetScenarioComponentReducer{
			stepID: "component-clear-plant-metadata:" + componentID,
			apply: func(scenario *target.TargetScenarioInfo, _ *target.TargetScenarioExecutionPlan) bool {
				return targetScenarioClearPlantMetadata(scenario, componentID, kindID)
			},
		})
	case step.Kind == "lifecycle-tightening" && targetScenarioLifecycleMetadataReductionAllowed(component, scenario, fidelity):
		kindID := strings.TrimSpace(component.KindID)
		reducers = append(reducers, targetScenarioComponentReducer{
			stepID: "component-clear-lifecycle-metadata:" + componentID,
			apply: func(scenario *target.TargetScenarioInfo, plan *target.TargetScenarioExecutionPlan) bool {
				return targetScenarioClearLifecycleMetadata(scenario, plan, componentID, kindID)
			},
		})
	case step.Kind == "oracle-preservation" && targetScenarioOracleMetadataReductionAllowed(component, scenario, fidelity):
		kindID := strings.TrimSpace(component.KindID)
		reducers = append(reducers, targetScenarioComponentReducer{
			stepID: "component-clear-oracle-metadata:" + componentID,
			apply: func(scenario *target.TargetScenarioInfo, _ *target.TargetScenarioExecutionPlan) bool {
				return targetScenarioClearOracleMetadata(scenario, componentID, kindID)
			},
		})
	}
	if targetScenarioComponentSummaryReductionAllowed(component) {
		reducers = append(reducers, targetScenarioComponentReducer{
			stepID: "component-clear-summary:" + componentID,
			apply: func(scenario *target.TargetScenarioInfo, _ *target.TargetScenarioExecutionPlan) bool {
				return targetScenarioClearComponentSummary(scenario, componentID)
			},
		})
	}
	return reducers
}

func targetScenarioOptionalComponentDeletionAllowed(component target.TargetScenarioComponent) bool {
	return component.Role == target.TargetScenarioComponentSetup || component.Role == target.TargetScenarioComponentFault
}

func targetScenarioPlantMetadataReductionAllowed(component target.TargetScenarioComponent, scenario *target.TargetScenarioInfo, fidelity TargetMinimizationFidelity) bool {
	if scenario == nil || component.Role != target.TargetScenarioComponentPlant {
		return false
	}
	if fidelity != TargetMinimizationFidelitySemantic && fidelity != TargetMinimizationFidelityImpact {
		return false
	}
	plantID := strings.TrimSpace(scenario.PlantPrimitiveID)
	return plantID != "" && plantID == strings.TrimSpace(component.KindID)
}

func targetScenarioLifecycleMetadataReductionAllowed(component target.TargetScenarioComponent, scenario *target.TargetScenarioInfo, fidelity TargetMinimizationFidelity) bool {
	if scenario == nil || component.Role != target.TargetScenarioComponentLifecycle {
		return false
	}
	if fidelity != TargetMinimizationFidelityImpact {
		return false
	}
	lifecycleID := targetScenarioLifecycleMetadataID(scenario, nil)
	return lifecycleID != "" && lifecycleID == strings.TrimSpace(component.KindID)
}

func targetScenarioOracleMetadataReductionAllowed(component target.TargetScenarioComponent, scenario *target.TargetScenarioInfo, fidelity TargetMinimizationFidelity) bool {
	if scenario == nil || component.Role != target.TargetScenarioComponentOracle {
		return false
	}
	if fidelity != TargetMinimizationFidelityImpact {
		return false
	}
	oracleID := strings.TrimSpace(scenario.OracleKindID)
	return oracleID != "" && oracleID == strings.TrimSpace(component.KindID)
}

func targetScenarioComponentSummaryReductionAllowed(component target.TargetScenarioComponent) bool {
	return strings.TrimSpace(component.Summary) != ""
}

func targetScenarioDeleteComponent(scenario *target.TargetScenarioInfo, componentID string) bool {
	if scenario == nil || componentID == "" || len(scenario.Components) == 0 {
		return false
	}
	components := scenario.Components[:0]
	deleted := false
	for _, component := range scenario.Components {
		if component.ComponentID == componentID {
			deleted = true
			continue
		}
		components = append(components, component)
	}
	scenario.Components = components
	return deleted
}

func targetScenarioClearPlantMetadata(scenario *target.TargetScenarioInfo, componentID string, kindID string) bool {
	if scenario == nil || strings.TrimSpace(scenario.PlantPrimitiveID) == "" {
		return false
	}
	if strings.TrimSpace(kindID) != "" && strings.TrimSpace(scenario.PlantPrimitiveID) != strings.TrimSpace(kindID) {
		return false
	}
	if !targetScenarioDeleteComponent(scenario, componentID) {
		return false
	}
	scenario.PlantPrimitiveID = ""
	return true
}

func targetScenarioClearLifecycleMetadata(scenario *target.TargetScenarioInfo, plan *target.TargetScenarioExecutionPlan, componentID string, kindID string) bool {
	if scenario == nil {
		return false
	}
	lifecycleID := targetScenarioLifecycleMetadataID(scenario, plan)
	if lifecycleID == "" {
		return false
	}
	if strings.TrimSpace(kindID) != "" && lifecycleID != strings.TrimSpace(kindID) {
		return false
	}
	if !targetScenarioDeleteComponent(scenario, componentID) {
		return false
	}
	scenario.LifecycleEdge = ""
	if scenario.ExecutionPlan != nil {
		scenario.ExecutionPlan.LifecycleOperationID = ""
	}
	if plan != nil {
		plan.LifecycleOperationID = ""
	}
	return true
}

func targetScenarioClearOracleMetadata(scenario *target.TargetScenarioInfo, componentID string, kindID string) bool {
	if scenario == nil || strings.TrimSpace(scenario.OracleKindID) == "" {
		return false
	}
	if strings.TrimSpace(kindID) != "" && strings.TrimSpace(scenario.OracleKindID) != strings.TrimSpace(kindID) {
		return false
	}
	if !targetScenarioDeleteComponent(scenario, componentID) {
		return false
	}
	scenario.OracleKindID = ""
	return true
}

func targetScenarioClearComponentSummary(scenario *target.TargetScenarioInfo, componentID string) bool {
	if scenario == nil || strings.TrimSpace(componentID) == "" {
		return false
	}
	for index := range scenario.Components {
		if scenario.Components[index].ComponentID != componentID {
			continue
		}
		if strings.TrimSpace(scenario.Components[index].Summary) == "" {
			return false
		}
		scenario.Components[index].Summary = ""
		return true
	}
	return false
}

func targetScenarioDeleteMutation(scenario *target.TargetScenarioInfo, mutationID string) bool {
	if scenario == nil || strings.TrimSpace(mutationID) == "" || len(scenario.Mutations) == 0 {
		return false
	}
	mutations := scenario.Mutations[:0]
	deleted := false
	for _, mutation := range scenario.Mutations {
		if mutation.MutationID == mutationID {
			deleted = true
			continue
		}
		mutations = append(mutations, mutation)
	}
	scenario.Mutations = mutations
	return deleted
}

func targetScenarioLifecycleMetadataID(scenario *target.TargetScenarioInfo, plan *target.TargetScenarioExecutionPlan) string {
	if plan != nil && strings.TrimSpace(plan.LifecycleOperationID) != "" {
		return strings.TrimSpace(plan.LifecycleOperationID)
	}
	if scenario != nil && scenario.ExecutionPlan != nil && strings.TrimSpace(scenario.ExecutionPlan.LifecycleOperationID) != "" {
		return strings.TrimSpace(scenario.ExecutionPlan.LifecycleOperationID)
	}
	if scenario != nil {
		return strings.TrimSpace(scenario.LifecycleEdge)
	}
	return ""
}

func targetScenarioComponentCount(scenario *target.TargetScenarioInfo) int {
	if scenario == nil {
		return 0
	}
	return len(scenario.Components)
}

func targetScenarioMutationCount(scenario *target.TargetScenarioInfo) int {
	if scenario == nil {
		return 0
	}
	return len(scenario.Mutations)
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

func targetMinimizationPreserved(source TargetSuiteRunResult, trial *target.TargetRunResult, fidelity TargetMinimizationFidelity) bool {
	if trial == nil || !trial.Completed {
		return false
	}
	normalized, err := normalizeTargetMinimizationFidelity(fidelity)
	if err != nil {
		return false
	}
	switch normalized {
	case TargetMinimizationFidelityExact:
		return targetMinimizationPreservedExact(source, trial)
	case TargetMinimizationFidelitySemantic:
		return targetMinimizationPreservedSemantic(source, trial)
	case TargetMinimizationFidelityImpact:
		return targetMinimizationPreservedImpact(source, trial)
	default:
		return false
	}
}

func targetMinimizationPreservedExact(source TargetSuiteRunResult, trial *target.TargetRunResult) bool {
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

func targetMinimizationPreservedSemantic(source TargetSuiteRunResult, trial *target.TargetRunResult) bool {
	if !targetMinimizationOracleStatusMatches(source, trial) {
		return false
	}
	if source.Signature != (core.MismatchSignature{}) && !targetMinimizationSemanticSignatureMatches(source.Signature, trial.Signature) {
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

func targetMinimizationPreservedImpact(source TargetSuiteRunResult, trial *target.TargetRunResult) bool {
	if !targetMinimizationOracleStatusMatches(source, trial) {
		return false
	}
	if source.Signature.Impact != "" {
		return trial.Signature.Impact == source.Signature.Impact
	}
	return true
}

func targetMinimizationOracleStatusMatches(source TargetSuiteRunResult, trial *target.TargetRunResult) bool {
	wantStatus := source.TargetOracle.Status
	if wantStatus == "" && source.Confirmed {
		wantStatus = target.TargetOracleStatusConfirmed
	}
	gotStatus := trial.TargetOracle.Status
	if gotStatus == "" && trial.TargetOracle.Confirmed {
		gotStatus = target.TargetOracleStatusConfirmed
	}
	return wantStatus == "" || gotStatus == wantStatus
}

func targetMinimizationSemanticSignatureMatches(want core.MismatchSignature, got core.MismatchSignature) bool {
	return targetMinimizationSignatureFieldMatches(want.LifecycleEvent, got.LifecycleEvent) &&
		targetMinimizationSignatureFieldMatches(want.FaultPhase, got.FaultPhase) &&
		targetMinimizationSignatureFieldMatches(want.StateClass, got.StateClass) &&
		targetMinimizationSignatureFieldMatches(want.Relation, got.Relation) &&
		targetMinimizationSignatureFieldMatches(want.Impact, got.Impact)
}

func targetMinimizationSignatureFieldMatches(want string, got string) bool {
	return strings.TrimSpace(want) == "" || strings.TrimSpace(got) == strings.TrimSpace(want)
}
