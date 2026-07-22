package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetSuiteOptions struct {
	AdapterID         string
	TargetID          string
	Tasks             []string
	TaskGroups        []string
	SeedIDs           []string
	CandidateScope    TargetCandidateScope
	Objective         string
	PromptProfileID   string
	PromptProfileIDs  []string
	Prompt            string
	PromptFile        string
	Command           string
	CommandFile       string
	OutDir            string
	CorpusDir         string
	Repeat            int
	Timeout           time.Duration
	ObserveDelay      time.Duration
	LateObserveDelay  time.Duration
	EnvKind           string
	ContainerImage    string
	ExpectedFiles     []string
	Matrix            bool
	FeedbackFrom      string
	CandidateLimit    int
	ExcludeCandidates []string
	SelectionPolicy   TargetSelectionPolicy
	RandomSeed        int64
}

type TargetSuiteRunResult struct {
	CandidateID            string                               `json:"candidate_id,omitempty"`
	ScenarioID             string                               `json:"scenario_id,omitempty"`
	QueryID                string                               `json:"query_id,omitempty"`
	ParentQueryID          string                               `json:"parent_query_id,omitempty"`
	RootQueryID            string                               `json:"root_query_id,omitempty"`
	SeedID                 string                               `json:"seed_id,omitempty"`
	TaskID                 string                               `json:"task_id"`
	TargetID               string                               `json:"target_id,omitempty"`
	PromptProfileID        string                               `json:"prompt_profile_id,omitempty"`
	PromptVariantID        string                               `json:"prompt_variant_id,omitempty"`
	LifecycleOperationID   string                               `json:"lifecycle_operation_id,omitempty"`
	PlantPrimitiveID       string                               `json:"plant_primitive_id,omitempty"`
	ActivationKindID       string                               `json:"activation_kind_id,omitempty"`
	OracleKindID           string                               `json:"oracle_kind_id,omitempty"`
	MutationFocusID        string                               `json:"mutation_focus_id,omitempty"`
	MutationFocusKind      target.TargetScenarioMutationKind    `json:"mutation_focus_kind,omitempty"`
	Mutations              []target.TargetScenarioMutation      `json:"mutations,omitempty"`
	ViolationSignature     target.TargetViolationSignature      `json:"violation_signature"`
	Iteration              int                                  `json:"iteration"`
	RunID                  string                               `json:"run_id,omitempty"`
	Confirmed              bool                                 `json:"confirmed"`
	Completed              bool                                 `json:"completed"`
	LateObserveDelayMs     int64                                `json:"late_observe_delay_ms,omitempty"`
	TargetOracle           target.TargetOracleResult            `json:"target_oracle"`
	TaskCompliance         target.TargetTaskComplianceResult    `json:"task_compliance"`
	ContractInterpretation *target.TargetContractInterpretation `json:"contract_interpretation,omitempty"`
	OutcomeCategory        corpus.TargetObservationCategory     `json:"outcome_category,omitempty"`
	OutcomeReason          string                               `json:"outcome_reason,omitempty"`
	ActivationStage        TargetActivationStage                `json:"activation_stage,omitempty"`
	MinimizationPlan       *TargetMinimizationPlan              `json:"minimization_plan,omitempty"`
	Signature              core.MismatchSignature               `json:"signature"`
	ArtifactDir            string                               `json:"artifact_dir,omitempty"`
	DurationMillis         int64                                `json:"duration_ms,omitempty"`
	ArtifactBytes          int64                                `json:"artifact_bytes,omitempty"`
	ArtifactFiles          int                                  `json:"artifact_files,omitempty"`
	MetricError            string                               `json:"metric_error,omitempty"`
	Error                  string                               `json:"error,omitempty"`
}

type TargetSuiteTaskSummary struct {
	TaskID               string                        `json:"task_id"`
	TotalRuns            int                           `json:"total_runs"`
	Confirmed            int                           `json:"confirmed"`
	Unconfirmed          int                           `json:"unconfirmed"`
	Errors               int                           `json:"errors"`
	OutcomeSummaries     []TargetSuiteOutcomeStats     `json:"outcome_summaries,omitempty"`
	ActivationSummaries  []TargetSuiteActivationStats  `json:"activation_summaries,omitempty"`
	AttributionSummaries []TargetSuiteAttributionStats `json:"attribution_summaries,omitempty"`
	ComplianceSummaries  []TargetSuiteComplianceStats  `json:"compliance_summaries,omitempty"`
	ContractSummaries    []TargetSuiteContractStats    `json:"contract_summaries,omitempty"`
}

type TargetActivationStage string

const (
	TargetActivationStageActivationReached TargetActivationStage = "activation-reached"
	TargetActivationStageActivationPending TargetActivationStage = "activation-pending"
	TargetActivationStageStateNotPlanted   TargetActivationStage = "state-not-planted"
	TargetActivationStageLifecyclePending  TargetActivationStage = "lifecycle-pending"
	TargetActivationStageTaskNoncompliant  TargetActivationStage = "task-noncompliant"
	TargetActivationStageExecutionPending  TargetActivationStage = "execution-pending"
	TargetActivationStagePreActivation     TargetActivationStage = "pre-activation"
)

type TargetSuiteOutcomeStats struct {
	Category    corpus.TargetObservationCategory `json:"category"`
	TotalRuns   int                              `json:"total_runs"`
	Confirmed   int                              `json:"confirmed"`
	Unconfirmed int                              `json:"unconfirmed"`
}

type TargetSuiteActivationStats struct {
	Stage       TargetActivationStage `json:"stage"`
	TotalRuns   int                   `json:"total_runs"`
	Confirmed   int                   `json:"confirmed"`
	Unconfirmed int                   `json:"unconfirmed"`
}

type TargetSuiteAttributionStats struct {
	Attribution string `json:"attribution"`
	TotalRuns   int    `json:"total_runs"`
	Confirmed   int    `json:"confirmed"`
	Unconfirmed int    `json:"unconfirmed"`
}

type TargetSuiteComplianceStats struct {
	Status      target.TargetTaskComplianceStatus `json:"status"`
	TotalRuns   int                               `json:"total_runs"`
	Confirmed   int                               `json:"confirmed"`
	Unconfirmed int                               `json:"unconfirmed"`
}

type TargetSuiteContractStats struct {
	Status      target.TargetContractInterpretationStatus `json:"status"`
	TotalRuns   int                                       `json:"total_runs"`
	Confirmed   int                                       `json:"confirmed"`
	Unconfirmed int                                       `json:"unconfirmed"`
}

type TargetSuiteResult struct {
	SchemaVersion        string                           `json:"schema_version"`
	SuiteID              string                           `json:"suite_id"`
	StartedAt            string                           `json:"started_at"`
	FinishedAt           string                           `json:"finished_at"`
	ArtifactDir          string                           `json:"artifact_dir"`
	AdapterID            string                           `json:"adapter_id"`
	TargetID             string                           `json:"target_id"`
	Environment          string                           `json:"environment"`
	ContainerImage       string                           `json:"container_image,omitempty"`
	Repeat               int                              `json:"repeat"`
	Tasks                []string                         `json:"tasks"`
	TaskGroups           []string                         `json:"task_groups,omitempty"`
	SeedIDs              []string                         `json:"seed_ids,omitempty"`
	CandidateScope       TargetCandidateScope             `json:"candidate_scope,omitempty"`
	PromptProfiles       []string                         `json:"prompt_profiles,omitempty"`
	TimeoutMillis        int64                            `json:"timeout_ms"`
	ObserveDelayMs       int64                            `json:"observe_delay_ms"`
	LateObserveDelayMs   int64                            `json:"late_observe_delay_ms,omitempty"`
	TotalRuns            int                              `json:"total_runs"`
	Confirmed            int                              `json:"confirmed"`
	Unconfirmed          int                              `json:"unconfirmed"`
	Errors               int                              `json:"errors"`
	OutcomeSummaries     []TargetSuiteOutcomeStats        `json:"outcome_summaries,omitempty"`
	ActivationSummaries  []TargetSuiteActivationStats     `json:"activation_summaries,omitempty"`
	DimensionCoverage    []TargetDimensionCoverageSummary `json:"dimension_coverage,omitempty"`
	AttributionSummaries []TargetSuiteAttributionStats    `json:"attribution_summaries,omitempty"`
	ComplianceSummaries  []TargetSuiteComplianceStats     `json:"compliance_summaries,omitempty"`
	ContractSummaries    []TargetSuiteContractStats       `json:"contract_summaries,omitempty"`
	FrontierCandidates   []TargetFrontierCandidate        `json:"frontier_candidates,omitempty"`
	TaskSummaries        []TargetSuiteTaskSummary         `json:"task_summaries"`
	SchedulerMode        string                           `json:"scheduler_mode,omitempty"`
	TotalCandidates      int                              `json:"total_candidates,omitempty"`
	OriginalCandidates   int                              `json:"original_candidates,omitempty"`
	CandidateLimit       int                              `json:"candidate_limit,omitempty"`
	FeedbackFrom         string                           `json:"feedback_from,omitempty"`
	SelectionPolicy      TargetSelectionPolicy            `json:"selection_policy,omitempty"`
	RandomSeed           int64                            `json:"random_seed,omitempty"`
	ScheduleMatrix       string                           `json:"schedule_matrix,omitempty"`
	MatrixResult         string                           `json:"matrix_result,omitempty"`
	CandidateSummaries   []TargetCandidateSummary         `json:"candidate_summaries,omitempty"`
	Results              []TargetSuiteRunResult           `json:"results"`
	CorpusEntries        []corpus.CorpusEntry             `json:"corpus_entries,omitempty"`
}

const targetSuiteResultArtifact = "target-suite-result.json"
const (
	targetScheduleMatrixArtifact = "target-schedule-matrix.json"
	targetMatrixResultArtifact   = "target-matrix-result.json"
)

type TargetMatrixResult struct {
	SchemaVersion      string                           `json:"schema_version"`
	SuiteID            string                           `json:"suite_id"`
	GeneratedAt        string                           `json:"generated_at"`
	ScheduleMatrix     string                           `json:"schedule_matrix"`
	PromptProfiles     []string                         `json:"prompt_profiles,omitempty"`
	CandidateScope     TargetCandidateScope             `json:"candidate_scope,omitempty"`
	TotalCandidates    int                              `json:"total_candidates"`
	OriginalCandidates int                              `json:"original_candidates,omitempty"`
	CandidateLimit     int                              `json:"candidate_limit,omitempty"`
	FeedbackFrom       string                           `json:"feedback_from,omitempty"`
	SelectionPolicy    TargetSelectionPolicy            `json:"selection_policy,omitempty"`
	RandomSeed         int64                            `json:"random_seed,omitempty"`
	Repeat             int                              `json:"repeat"`
	TotalRuns          int                              `json:"total_runs"`
	Confirmed          int                              `json:"confirmed"`
	Unconfirmed        int                              `json:"unconfirmed"`
	Errors             int                              `json:"errors"`
	DimensionCoverage  []TargetDimensionCoverageSummary `json:"dimension_coverage,omitempty"`
	FrontierCandidates []TargetFrontierCandidate        `json:"frontier_candidates,omitempty"`
	CandidateSummaries []TargetCandidateSummary         `json:"candidate_summaries"`
	Results            []TargetSuiteRunResult           `json:"results"`
}

func RunTargetSuite(ctx context.Context, opts TargetSuiteOptions) (*TargetSuiteResult, error) {
	if opts.AdapterID == "" {
		opts.AdapterID = target.DefaultTargetAdapterID
	}
	if opts.TargetID == "" {
		opts.TargetID = opts.AdapterID
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Repeat <= 0 {
		opts.Repeat = 1
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if err := environment.ValidateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}
	selectionPolicy, err := normalizeTargetSelectionPolicy(opts.SelectionPolicy, opts.FeedbackFrom)
	if err != nil {
		return nil, err
	}
	randomSeed := opts.RandomSeed
	if selectionPolicy == TargetSelectionPolicyRandom && randomSeed == 0 {
		randomSeed = DefaultTargetRandomSeed
	}
	effectiveFeedbackFrom := opts.FeedbackFrom
	if selectionPolicy != TargetSelectionPolicyFeedback {
		effectiveFeedbackFrom = ""
	}
	schedulerMode := suiteSchedulerCaseList
	var (
		tasks                  []string
		taskGroups             []string
		seedIDs                []string
		matrix                 *TargetScheduleMatrix
		coverageUniverse       *TargetScheduleMatrix
		originalCandidateCount int
	)
	if opts.Matrix {
		schedulerMode = suiteSchedulerMatrix
		matrix, err = BuildTargetScheduleMatrix(TargetMatrixOptions{
			TargetID:         opts.TargetID,
			Tasks:            opts.Tasks,
			TaskGroups:       opts.TaskGroups,
			SeedIDs:          opts.SeedIDs,
			PromptProfileIDs: target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs),
			CandidateScope:   opts.CandidateScope,
		})
		if err != nil {
			return nil, err
		}
		coverageUniverse = matrix
		originalCandidateCount = matrix.TotalCandidates
		tasks = append([]string{}, matrix.Tasks...)
		taskGroups = append([]string{}, matrix.TaskGroups...)
		selectionRequested := opts.FeedbackFrom != "" || opts.CandidateLimit > 0 || len(opts.ExcludeCandidates) > 0 || opts.SelectionPolicy != ""
		if selectionRequested {
			schedulerMode = suiteSchedulerFeedback
			matrix, err = selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
				FeedbackFrom:        effectiveFeedbackFrom,
				Limit:               opts.CandidateLimit,
				ExcludeCandidateIDs: opts.ExcludeCandidates,
				SelectionPolicy:     selectionPolicy,
				RandomSeed:          randomSeed,
			})
			if err != nil {
				return nil, err
			}
			tasks = targetCandidateTaskIDs(matrix.Candidates)
		}
	} else {
		tasks, taskGroups, seedIDs, err = target.ExpandTargetSelection(opts.Tasks, opts.TaskGroups, opts.SeedIDs)
		if err != nil {
			return nil, err
		}
		opts.SeedIDs = seedIDs
	}
	started := time.Now().UTC()
	suiteID := fmt.Sprintf("target-suite-%d", started.UnixNano())
	suiteDir := filepath.Join(opts.OutDir, suiteID)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target suite directory: %w", err)
	}
	if matrix != nil {
		if err := core.WriteJSON(filepath.Join(suiteDir, targetScheduleMatrixArtifact), matrix); err != nil {
			return nil, err
		}
	}

	result := &TargetSuiteResult{
		SchemaVersion:      "syncfuzz.target-suite-result.v1",
		SuiteID:            suiteID,
		StartedAt:          started.Format(time.RFC3339Nano),
		ArtifactDir:        suiteDir,
		AdapterID:          opts.AdapterID,
		TargetID:           opts.TargetID,
		Environment:        environment.NormalizedEnvKind(opts.EnvKind),
		ContainerImage:     environment.ContainerImageForResult(opts.EnvKind, opts.ContainerImage),
		Repeat:             opts.Repeat,
		Tasks:              append([]string{}, tasks...),
		TaskGroups:         append([]string{}, taskGroups...),
		SeedIDs:            append([]string{}, opts.SeedIDs...),
		SchedulerMode:      schedulerMode,
		TimeoutMillis:      opts.Timeout.Milliseconds(),
		ObserveDelayMs:     opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs: opts.LateObserveDelay.Milliseconds(),
		Results:            []TargetSuiteRunResult{},
	}
	if matrix != nil {
		result.PromptProfiles = append([]string{}, matrix.PromptProfiles...)
		result.CandidateScope = matrix.CandidateScope
		result.SeedIDs = append([]string{}, matrix.SeedIDs...)
		result.TotalCandidates = matrix.TotalCandidates
		result.OriginalCandidates = originalCandidateCount
		result.CandidateLimit = opts.CandidateLimit
		result.FeedbackFrom = effectiveFeedbackFrom
		if schedulerMode == suiteSchedulerMatrix && opts.SelectionPolicy == "" && opts.FeedbackFrom == "" {
			result.SelectionPolicy = TargetSelectionPolicyFixed
		} else {
			result.SelectionPolicy = selectionPolicy
		}
		if result.SelectionPolicy == TargetSelectionPolicyRandom {
			result.RandomSeed = randomSeed
		}
		result.ScheduleMatrix = filepath.Join(suiteDir, targetScheduleMatrixArtifact)
		result.MatrixResult = filepath.Join(suiteDir, targetMatrixResultArtifact)
	} else if selection := target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs); len(selection) > 0 {
		result.PromptProfiles = append([]string{}, selection...)
	}

	summaries := make(map[string]*TargetSuiteTaskSummary, len(tasks))
	outcomeSummary := make(map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats)
	taskOutcomes := make(map[string]map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats, len(tasks))
	activationSummary := make(map[TargetActivationStage]*TargetSuiteActivationStats)
	taskActivations := make(map[string]map[TargetActivationStage]*TargetSuiteActivationStats, len(tasks))
	attributionSummary := make(map[string]*TargetSuiteAttributionStats)
	taskAttributions := make(map[string]map[string]*TargetSuiteAttributionStats, len(tasks))
	complianceSummary := make(map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
	taskCompliances := make(map[string]map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats, len(tasks))
	contractSummary := make(map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats)
	taskContracts := make(map[string]map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats, len(tasks))
	for _, taskID := range tasks {
		summaries[taskID] = &TargetSuiteTaskSummary{TaskID: taskID}
		taskOutcomes[taskID] = make(map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats)
		taskActivations[taskID] = make(map[TargetActivationStage]*TargetSuiteActivationStats)
		taskAttributions[taskID] = make(map[string]*TargetSuiteAttributionStats)
		taskCompliances[taskID] = make(map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
		taskContracts[taskID] = make(map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats)
	}

	for iteration := 1; iteration <= opts.Repeat; iteration++ {
		if matrix != nil {
			for _, candidate := range matrix.Candidates {
				runTargetSuiteTask(ctx, opts, suiteDir, iteration, candidate, summaries, outcomeSummary, taskOutcomes, activationSummary, taskActivations, attributionSummary, taskAttributions, complianceSummary, taskCompliances, contractSummary, taskContracts, result)
			}
			continue
		}
		for _, taskID := range tasks {
			runTargetSuiteTask(ctx, opts, suiteDir, iteration, targetScheduledTaskCandidate(opts.TargetID, taskID, opts.PromptProfileID, ""), summaries, outcomeSummary, taskOutcomes, activationSummary, taskActivations, attributionSummary, taskAttributions, complianceSummary, taskCompliances, contractSummary, taskContracts, result)
		}
	}

	result.TotalRuns = len(result.Results)
	result.TaskSummaries = make([]TargetSuiteTaskSummary, 0, len(tasks))
	for _, taskID := range tasks {
		summaries[taskID].OutcomeSummaries = targetSuiteOutcomeStats(taskOutcomes[taskID])
		summaries[taskID].ActivationSummaries = targetSuiteActivationStats(taskActivations[taskID])
		summaries[taskID].AttributionSummaries = targetSuiteAttributionStats(taskAttributions[taskID])
		summaries[taskID].ComplianceSummaries = targetSuiteComplianceStats(taskCompliances[taskID])
		summaries[taskID].ContractSummaries = targetSuiteContractStats(taskContracts[taskID])
		result.TaskSummaries = append(result.TaskSummaries, *summaries[taskID])
	}
	result.OutcomeSummaries = targetSuiteOutcomeStats(outcomeSummary)
	result.ActivationSummaries = targetSuiteActivationStats(activationSummary)
	result.AttributionSummaries = targetSuiteAttributionStats(attributionSummary)
	result.ComplianceSummaries = targetSuiteComplianceStats(complianceSummary)
	result.ContractSummaries = targetSuiteContractStats(contractSummary)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if matrix != nil {
		if coverageUniverse == nil {
			coverageUniverse = matrix
		}
		result.DimensionCoverage = summarizeTargetDimensionCoverage(coverageUniverse.Candidates, result.Results)
		result.FrontierCandidates = summarizeTargetCoverageFrontier(coverageUniverse, result.Results, opts.ExcludeCandidates, targetFrontierDefaultLimit)
		result.CandidateSummaries = summarizeTargetCandidates(result.Results)
	}
	corpusEntries, err := WriteTargetCorpus(opts.CorpusDir, result)
	if err != nil {
		return nil, err
	}
	result.CorpusEntries = corpusEntries
	if err := core.WriteJSON(filepath.Join(suiteDir, targetSuiteResultArtifact), result); err != nil {
		return nil, err
	}
	if matrix != nil {
		matrixResult := TargetMatrixResult{
			SchemaVersion:      "syncfuzz.target-matrix-result.v1",
			SuiteID:            result.SuiteID,
			GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
			ScheduleMatrix:     result.ScheduleMatrix,
			PromptProfiles:     append([]string{}, result.PromptProfiles...),
			CandidateScope:     result.CandidateScope,
			TotalCandidates:    result.TotalCandidates,
			OriginalCandidates: result.OriginalCandidates,
			CandidateLimit:     result.CandidateLimit,
			FeedbackFrom:       result.FeedbackFrom,
			SelectionPolicy:    result.SelectionPolicy,
			RandomSeed:         result.RandomSeed,
			Repeat:             result.Repeat,
			TotalRuns:          result.TotalRuns,
			Confirmed:          result.Confirmed,
			Unconfirmed:        result.Unconfirmed,
			Errors:             result.Errors,
			DimensionCoverage:  append([]TargetDimensionCoverageSummary{}, result.DimensionCoverage...),
			FrontierCandidates: append([]TargetFrontierCandidate{}, result.FrontierCandidates...),
			CandidateSummaries: result.CandidateSummaries,
			Results:            result.Results,
		}
		if err := core.WriteJSON(filepath.Join(suiteDir, targetMatrixResultArtifact), matrixResult); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func runTargetSuiteTask(
	ctx context.Context,
	opts TargetSuiteOptions,
	suiteDir string,
	iteration int,
	candidate TargetScheduleCandidate,
	summaries map[string]*TargetSuiteTaskSummary,
	outcomeSummary map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats,
	taskOutcomes map[string]map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats,
	activationSummary map[TargetActivationStage]*TargetSuiteActivationStats,
	taskActivations map[string]map[TargetActivationStage]*TargetSuiteActivationStats,
	attributionSummary map[string]*TargetSuiteAttributionStats,
	taskAttributions map[string]map[string]*TargetSuiteAttributionStats,
	complianceSummary map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats,
	taskCompliances map[string]map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats,
	contractSummary map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats,
	taskContracts map[string]map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats,
	result *TargetSuiteResult,
) {
	taskID := candidate.TaskID
	runLateObserveDelay := opts.LateObserveDelay
	if runLateObserveDelay == 0 {
		if candidate.DefaultLateObserveDelay > 0 {
			runLateObserveDelay = time.Duration(candidate.DefaultLateObserveDelay) * time.Millisecond
		} else {
			runLateObserveDelay = target.DefaultTargetLateObserveDelay(taskID)
		}
	}
	item := TargetSuiteRunResult{
		CandidateID:          candidate.CandidateID,
		ScenarioID:           candidate.ScenarioID,
		QueryID:              candidate.QueryID,
		ParentQueryID:        candidate.ParentQueryID,
		RootQueryID:          candidate.RootQueryID,
		SeedID:               candidate.SeedID,
		TaskID:               taskID,
		TargetID:             opts.TargetID,
		PromptProfileID:      target.NormalizeTargetPromptProfileID(candidate.PromptProfileID),
		PromptVariantID:      target.NormalizeTargetPromptVariantID(candidate.PromptVariantID),
		LifecycleOperationID: candidate.LifecycleOperationID,
		PlantPrimitiveID:     candidate.PlantPrimitiveID,
		ActivationKindID:     candidate.ActivationKindID,
		OracleKindID:         candidate.OracleKindID,
		MutationFocusID:      candidate.MutationFocusID,
		MutationFocusKind:    candidate.MutationFocusKind,
		Mutations:            append([]target.TargetScenarioMutation{}, candidate.Mutations...),
		ViolationSignature:   candidate.ViolationSignature,
		Iteration:            iteration,
		LateObserveDelayMs:   runLateObserveDelay.Milliseconds(),
		Signature:            target.TargetSignature(taskID),
	}
	startedRun := time.Now()
	runPrompt := opts.Prompt
	runPromptFile := opts.PromptFile
	if runPrompt == "" && runPromptFile == "" {
		if candidate.Prompt != "" {
			runPrompt = candidate.Prompt
		} else if target.NormalizeTargetPromptVariantID(candidate.PromptVariantID) != target.TargetPromptVariantBaseID {
			runPrompt = target.DefaultTargetPromptVariantWithProfile(taskID, candidate.PromptProfileID, candidate.PromptVariantID)
		}
	}
	runResult, err := target.RunTarget(ctx, target.TargetRunOptions{
		AdapterID:        opts.AdapterID,
		TargetID:         opts.TargetID,
		TaskID:           taskID,
		Objective:        opts.Objective,
		Scenario:         targetScenarioForCandidate(candidate),
		ExecutionPlan:    cloneTargetExecutionPlan(candidate.ExecutionPlan),
		PromptProfileID:  candidate.PromptProfileID,
		PromptVariantID:  candidate.PromptVariantID,
		Prompt:           runPrompt,
		PromptFile:       runPromptFile,
		Command:          opts.Command,
		CommandFile:      opts.CommandFile,
		OutDir:           suiteDir,
		Timeout:          opts.Timeout,
		ObserveDelay:     opts.ObserveDelay,
		LateObserveDelay: runLateObserveDelay,
		EnvKind:          opts.EnvKind,
		ContainerImage:   opts.ContainerImage,
		ExpectedFiles:    opts.ExpectedFiles,
	})
	if err != nil {
		item.Error = err.Error()
		item.DurationMillis = time.Since(startedRun).Milliseconds()
		result.Errors++
		summaries[taskID].TotalRuns++
		summaries[taskID].Errors++
		result.Results = append(result.Results, item)
		return
	}

	item.RunID = runResult.RunID
	item.TargetID = runResult.TargetID
	item.PromptProfileID = runResult.PromptProfileID
	item.PromptVariantID = runResult.PromptVariantID
	item.Confirmed = runResult.ExpectationsMet
	item.Completed = runResult.Completed
	item.LateObserveDelayMs = runResult.LateObserveDelayMs
	item.TargetOracle = runResult.TargetOracle
	item.TaskCompliance = runResult.TaskCompliance
	item.ContractInterpretation = runResult.ContractInterpretation
	if runResult.ViolationSignature != nil {
		item.ViolationSignature = *runResult.ViolationSignature
	}
	observation := corpus.ClassifyTargetObservation(runResult)
	item.OutcomeCategory = observation.Category
	item.OutcomeReason = observation.Reason
	item.ActivationStage = targetActivationStageForObservation(observation)
	item.MinimizationPlan = buildTargetMinimizationPlan(candidate, item, observation)
	item.Signature = runResult.Signature
	item.ArtifactDir = runResult.ArtifactDir
	finalizeTargetSuiteItemMetrics(&item, startedRun)

	summaries[taskID].TotalRuns++
	if item.Confirmed {
		result.Confirmed++
		summaries[taskID].Confirmed++
	} else {
		result.Unconfirmed++
		summaries[taskID].Unconfirmed++
	}
	recordTargetSuiteOutcome(outcomeSummary, item.OutcomeCategory, item.Confirmed)
	recordTargetSuiteOutcome(taskOutcomes[taskID], item.OutcomeCategory, item.Confirmed)
	recordTargetSuiteActivation(activationSummary, item.ActivationStage, item.Confirmed)
	recordTargetSuiteActivation(taskActivations[taskID], item.ActivationStage, item.Confirmed)
	recordTargetSuiteAttribution(attributionSummary, item.TargetOracle.Attribution, item.Confirmed)
	recordTargetSuiteAttribution(taskAttributions[taskID], item.TargetOracle.Attribution, item.Confirmed)
	recordTargetSuiteCompliance(complianceSummary, item.TaskCompliance.Status, item.Confirmed)
	recordTargetSuiteCompliance(taskCompliances[taskID], item.TaskCompliance.Status, item.Confirmed)
	recordTargetSuiteContract(contractSummary, target.TargetContractInterpretationStatusValue(item.ContractInterpretation), item.Confirmed)
	recordTargetSuiteContract(taskContracts[taskID], target.TargetContractInterpretationStatusValue(item.ContractInterpretation), item.Confirmed)
	result.Results = append(result.Results, item)
}

func cloneTargetExecutionPlan(plan *target.TargetScenarioExecutionPlan) *target.TargetScenarioExecutionPlan {
	if plan == nil {
		return nil
	}
	copyPlan := *plan
	return &copyPlan
}

func targetScenarioForCandidate(candidate TargetScheduleCandidate) *target.TargetScenarioInfo {
	scenario, ok := target.TargetScenarioByTaskID(candidate.TaskID)
	if !ok {
		return nil
	}
	if candidate.ScenarioID != "" {
		scenario.ScenarioID = candidate.ScenarioID
	}
	if candidate.QueryID != "" {
		scenario.QueryID = candidate.QueryID
	}
	if candidate.ParentQueryID != "" {
		scenario.ParentQueryID = candidate.ParentQueryID
	}
	if candidate.RootQueryID != "" {
		scenario.RootQueryID = candidate.RootQueryID
	}
	if candidate.ScenarioSchemaVersion != "" {
		scenario.SchemaVersion = candidate.ScenarioSchemaVersion
	}
	if candidate.SeedID != "" {
		scenario.SeedID = candidate.SeedID
	}
	if candidate.Description != "" {
		scenario.Description = candidate.Description
	}
	if candidate.Objective != "" {
		scenario.Objective = candidate.Objective
	}
	if candidate.StateSurface != "" {
		scenario.StateSurface = candidate.StateSurface
	}
	if candidate.LifecycleEdge != "" {
		scenario.LifecycleEdge = candidate.LifecycleEdge
	}
	if candidate.ViolationSignature.SchemaVersion != "" {
		scenario.ViolationSignature = candidate.ViolationSignature
	}
	if candidate.PlantPrimitiveID != "" {
		scenario.PlantPrimitiveID = candidate.PlantPrimitiveID
	}
	if candidate.ActivationKindID != "" {
		scenario.ActivationKindID = candidate.ActivationKindID
	}
	if candidate.OracleKindID != "" {
		scenario.OracleKindID = candidate.OracleKindID
	}
	if candidate.DefaultExpectedFiles != nil {
		scenario.DefaultExpectedFiles = append([]string{}, candidate.DefaultExpectedFiles...)
	}
	if candidate.LateExpectedFiles != nil {
		scenario.LateExpectedFiles = append([]string{}, candidate.LateExpectedFiles...)
	}
	scenario.UsesLateObservation = candidate.UsesLateObservation
	if candidate.DefaultLateObserveDelay > 0 {
		scenario.LateObserveDelayMs = candidate.DefaultLateObserveDelay
	}
	if candidate.Components != nil {
		scenario.Components = append([]target.TargetScenarioComponent{}, candidate.Components...)
	}
	if candidate.Mutations != nil {
		scenario.Mutations = append([]target.TargetScenarioMutation{}, candidate.Mutations...)
	}
	if candidate.ExecutionPlan != nil {
		scenario.ExecutionPlan = cloneTargetExecutionPlan(candidate.ExecutionPlan)
	}
	return scenario
}

func targetScheduledTaskCandidate(targetID string, taskID string, promptProfileID string, candidateID string) TargetScheduleCandidate {
	item := TargetScheduleCandidate{
		CandidateID:             candidateID,
		TargetID:                targetID,
		TaskID:                  taskID,
		PromptProfileID:         target.NormalizeTargetPromptProfileID(promptProfileID),
		PromptVariantID:         target.TargetPromptVariantBaseID,
		DefaultExpectedFiles:    target.DefaultTargetExpectedFiles(taskID),
		UsesLateObservation:     target.DefaultTargetLateObserveDelay(taskID) > 0,
		DefaultLateObserveDelay: target.DefaultTargetLateObserveDelay(taskID).Milliseconds(),
		Signature:               target.TargetSignature(taskID),
	}
	if taskInfo, ok := targetTaskInfoByID(taskID); ok {
		item.ScenarioSchemaVersion = taskInfo.ScenarioSchemaVersion
		item.ScenarioID = taskInfo.ScenarioID
		item.QueryID = taskInfo.QueryID
		item.ParentQueryID = taskInfo.ParentQueryID
		item.RootQueryID = taskInfo.RootQueryID
		item.SeedID = taskInfo.SeedID
		item.Description = taskInfo.Description
		item.StateSurface = taskInfo.StateSurface
		item.LifecycleEdge = taskInfo.LifecycleEdge
		item.ViolationSignature = taskInfo.ViolationSignature
		item.LifecycleOperationID = taskInfo.LifecycleOperationID
		item.PlantPrimitiveID = taskInfo.PlantPrimitiveID
		item.ActivationKindID = taskInfo.ActivationKindID
		item.OracleKindID = taskInfo.OracleKindID
		item.MutationFocusID = taskInfo.MutationFocusID
		item.MutationFocusKind = taskInfo.MutationFocusKind
		item.Mutations = append([]target.TargetScenarioMutation{}, taskInfo.Mutations...)
	}
	return item
}

func finalizeTargetSuiteItemMetrics(item *TargetSuiteRunResult, started time.Time) {
	item.DurationMillis = time.Since(started).Milliseconds()
	if item.ArtifactDir == "" {
		return
	}
	metrics, err := core.MeasureArtifactMetrics(item.ArtifactDir)
	if err != nil {
		item.MetricError = err.Error()
		return
	}
	item.ArtifactBytes = metrics.Bytes
	item.ArtifactFiles = metrics.Files
}

func targetActivationStageForObservation(details corpus.TargetObservationDetails) TargetActivationStage {
	if details.ActivationReached {
		return TargetActivationStageActivationReached
	}
	switch details.Category {
	case corpus.TargetObservationExecutionNotReached, corpus.TargetObservationError:
		return TargetActivationStageExecutionPending
	case corpus.TargetObservationTaskNoncompliant:
		return TargetActivationStageTaskNoncompliant
	case corpus.TargetObservationLifecycleNotTriggered:
		return TargetActivationStageLifecyclePending
	case corpus.TargetObservationStateNotPlanted:
		return TargetActivationStageStateNotPlanted
	case corpus.TargetObservationActivationNotTriggered:
		return TargetActivationStageActivationPending
	}
	return TargetActivationStagePreActivation
}

func recordTargetSuiteOutcome(stats map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats, category corpus.TargetObservationCategory, confirmed bool) {
	if category == "" {
		return
	}
	item, ok := stats[category]
	if !ok {
		item = &TargetSuiteOutcomeStats{Category: category}
		stats[category] = item
	}
	item.TotalRuns++
	if confirmed {
		item.Confirmed++
		return
	}
	item.Unconfirmed++
}

func recordTargetSuiteActivation(stats map[TargetActivationStage]*TargetSuiteActivationStats, stage TargetActivationStage, confirmed bool) {
	if stage == "" {
		return
	}
	item, ok := stats[stage]
	if !ok {
		item = &TargetSuiteActivationStats{Stage: stage}
		stats[stage] = item
	}
	item.TotalRuns++
	if confirmed {
		item.Confirmed++
		return
	}
	item.Unconfirmed++
}

func recordTargetSuiteAttribution(stats map[string]*TargetSuiteAttributionStats, attribution string, confirmed bool) {
	if attribution == "" {
		return
	}
	item, ok := stats[attribution]
	if !ok {
		item = &TargetSuiteAttributionStats{Attribution: attribution}
		stats[attribution] = item
	}
	item.TotalRuns++
	if confirmed {
		item.Confirmed++
		return
	}
	item.Unconfirmed++
}

func recordTargetSuiteCompliance(stats map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats, status target.TargetTaskComplianceStatus, confirmed bool) {
	if status == "" {
		return
	}
	item, ok := stats[status]
	if !ok {
		item = &TargetSuiteComplianceStats{Status: status}
		stats[status] = item
	}
	item.TotalRuns++
	if confirmed {
		item.Confirmed++
		return
	}
	item.Unconfirmed++
}

func recordTargetSuiteContract(stats map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats, status target.TargetContractInterpretationStatus, confirmed bool) {
	if status == "" {
		return
	}
	item, ok := stats[status]
	if !ok {
		item = &TargetSuiteContractStats{Status: status}
		stats[status] = item
	}
	item.TotalRuns++
	if confirmed {
		item.Confirmed++
		return
	}
	item.Unconfirmed++
}

func targetSuiteAttributionStats(stats map[string]*TargetSuiteAttributionStats) []TargetSuiteAttributionStats {
	if len(stats) == 0 {
		return nil
	}
	attributions := make([]string, 0, len(stats))
	for attribution := range stats {
		attributions = append(attributions, attribution)
	}
	sort.Strings(attributions)
	summary := make([]TargetSuiteAttributionStats, 0, len(attributions))
	for _, attribution := range attributions {
		summary = append(summary, *stats[attribution])
	}
	return summary
}

func targetSuiteOutcomeStats(stats map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats) []TargetSuiteOutcomeStats {
	if len(stats) == 0 {
		return nil
	}
	categories := make([]corpus.TargetObservationCategory, 0, len(stats))
	for category := range stats {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i, j int) bool {
		li := corpus.TargetObservationCategoryOrder(categories[i])
		lj := corpus.TargetObservationCategoryOrder(categories[j])
		if li != lj {
			return li < lj
		}
		return categories[i] < categories[j]
	})
	summary := make([]TargetSuiteOutcomeStats, 0, len(categories))
	for _, category := range categories {
		summary = append(summary, *stats[category])
	}
	return summary
}

func targetSuiteActivationStats(stats map[TargetActivationStage]*TargetSuiteActivationStats) []TargetSuiteActivationStats {
	if len(stats) == 0 {
		return nil
	}
	stages := make([]TargetActivationStage, 0, len(stats))
	for stage := range stats {
		stages = append(stages, stage)
	}
	sort.Slice(stages, func(i, j int) bool {
		li := targetActivationStageOrder(stages[i])
		lj := targetActivationStageOrder(stages[j])
		if li != lj {
			return li < lj
		}
		return stages[i] < stages[j]
	})
	summary := make([]TargetSuiteActivationStats, 0, len(stages))
	for _, stage := range stages {
		summary = append(summary, *stats[stage])
	}
	return summary
}

func targetSuiteComplianceStats(stats map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats) []TargetSuiteComplianceStats {
	if len(stats) == 0 {
		return nil
	}
	statuses := make([]target.TargetTaskComplianceStatus, 0, len(stats))
	for status := range stats {
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		li := targetTaskComplianceStatusOrder(statuses[i])
		lj := targetTaskComplianceStatusOrder(statuses[j])
		if li != lj {
			return li < lj
		}
		return statuses[i] < statuses[j]
	})
	summary := make([]TargetSuiteComplianceStats, 0, len(statuses))
	for _, status := range statuses {
		summary = append(summary, *stats[status])
	}
	return summary
}

func targetSuiteContractStats(stats map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats) []TargetSuiteContractStats {
	if len(stats) == 0 {
		return nil
	}
	statuses := make([]target.TargetContractInterpretationStatus, 0, len(stats))
	for status := range stats {
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		li := targetContractInterpretationStatusOrder(statuses[i])
		lj := targetContractInterpretationStatusOrder(statuses[j])
		if li != lj {
			return li < lj
		}
		return statuses[i] < statuses[j]
	})
	summary := make([]TargetSuiteContractStats, 0, len(statuses))
	for _, status := range statuses {
		summary = append(summary, *stats[status])
	}
	return summary
}

func targetTaskComplianceStatusOrder(status target.TargetTaskComplianceStatus) int {
	switch status {
	case target.TargetTaskComplianceStatusCompliant:
		return 0
	case target.TargetTaskComplianceStatusViolated:
		return 1
	case target.TargetTaskComplianceStatusUnknown:
		return 2
	case target.TargetTaskComplianceStatusNotApplicable:
		return 3
	default:
		return 4
	}
}

func targetActivationStageOrder(stage TargetActivationStage) int {
	switch stage {
	case TargetActivationStageActivationReached:
		return 0
	case TargetActivationStageActivationPending:
		return 1
	case TargetActivationStageStateNotPlanted:
		return 2
	case TargetActivationStageLifecyclePending:
		return 3
	case TargetActivationStageTaskNoncompliant:
		return 4
	case TargetActivationStageExecutionPending:
		return 5
	case TargetActivationStagePreActivation:
		return 6
	default:
		return 7
	}
}

func targetContractInterpretationStatusOrder(status target.TargetContractInterpretationStatus) int {
	switch status {
	case target.TargetContractStatusViolation:
		return 0
	case target.TargetContractStatusConsistent:
		return 1
	case target.TargetContractStatusUnknown:
		return 2
	default:
		return 3
	}
}
