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
}

type TargetSuiteRunResult struct {
	CandidateID            string                               `json:"candidate_id,omitempty"`
	TaskID                 string                               `json:"task_id"`
	TargetID               string                               `json:"target_id,omitempty"`
	PromptProfileID        string                               `json:"prompt_profile_id,omitempty"`
	Iteration              int                                  `json:"iteration"`
	RunID                  string                               `json:"run_id,omitempty"`
	Confirmed              bool                                 `json:"confirmed"`
	Completed              bool                                 `json:"completed"`
	LateObserveDelayMs     int64                                `json:"late_observe_delay_ms,omitempty"`
	TargetOracle           target.TargetOracleResult            `json:"target_oracle"`
	TaskCompliance         target.TargetTaskComplianceResult    `json:"task_compliance"`
	ContractInterpretation *target.TargetContractInterpretation `json:"contract_interpretation,omitempty"`
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
	AttributionSummaries []TargetSuiteAttributionStats `json:"attribution_summaries,omitempty"`
	ComplianceSummaries  []TargetSuiteComplianceStats  `json:"compliance_summaries,omitempty"`
	ContractSummaries    []TargetSuiteContractStats    `json:"contract_summaries,omitempty"`
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
	SchemaVersion        string                        `json:"schema_version"`
	SuiteID              string                        `json:"suite_id"`
	StartedAt            string                        `json:"started_at"`
	FinishedAt           string                        `json:"finished_at"`
	ArtifactDir          string                        `json:"artifact_dir"`
	AdapterID            string                        `json:"adapter_id"`
	TargetID             string                        `json:"target_id"`
	Environment          string                        `json:"environment"`
	ContainerImage       string                        `json:"container_image,omitempty"`
	Repeat               int                           `json:"repeat"`
	Tasks                []string                      `json:"tasks"`
	TaskGroups           []string                      `json:"task_groups,omitempty"`
	PromptProfiles       []string                      `json:"prompt_profiles,omitempty"`
	TimeoutMillis        int64                         `json:"timeout_ms"`
	ObserveDelayMs       int64                         `json:"observe_delay_ms"`
	LateObserveDelayMs   int64                         `json:"late_observe_delay_ms,omitempty"`
	TotalRuns            int                           `json:"total_runs"`
	Confirmed            int                           `json:"confirmed"`
	Unconfirmed          int                           `json:"unconfirmed"`
	Errors               int                           `json:"errors"`
	AttributionSummaries []TargetSuiteAttributionStats `json:"attribution_summaries,omitempty"`
	ComplianceSummaries  []TargetSuiteComplianceStats  `json:"compliance_summaries,omitempty"`
	ContractSummaries    []TargetSuiteContractStats    `json:"contract_summaries,omitempty"`
	TaskSummaries        []TargetSuiteTaskSummary      `json:"task_summaries"`
	SchedulerMode        string                        `json:"scheduler_mode,omitempty"`
	TotalCandidates      int                           `json:"total_candidates,omitempty"`
	OriginalCandidates   int                           `json:"original_candidates,omitempty"`
	CandidateLimit       int                           `json:"candidate_limit,omitempty"`
	FeedbackFrom         string                        `json:"feedback_from,omitempty"`
	ScheduleMatrix       string                        `json:"schedule_matrix,omitempty"`
	MatrixResult         string                        `json:"matrix_result,omitempty"`
	CandidateSummaries   []TargetCandidateSummary      `json:"candidate_summaries,omitempty"`
	Results              []TargetSuiteRunResult        `json:"results"`
	CorpusEntries        []corpus.CorpusEntry          `json:"corpus_entries,omitempty"`
}

const targetSuiteResultArtifact = "target-suite-result.json"
const (
	targetScheduleMatrixArtifact = "target-schedule-matrix.json"
	targetMatrixResultArtifact   = "target-matrix-result.json"
)

type TargetMatrixResult struct {
	SchemaVersion      string                   `json:"schema_version"`
	SuiteID            string                   `json:"suite_id"`
	GeneratedAt        string                   `json:"generated_at"`
	ScheduleMatrix     string                   `json:"schedule_matrix"`
	PromptProfiles     []string                 `json:"prompt_profiles,omitempty"`
	TotalCandidates    int                      `json:"total_candidates"`
	OriginalCandidates int                      `json:"original_candidates,omitempty"`
	CandidateLimit     int                      `json:"candidate_limit,omitempty"`
	FeedbackFrom       string                   `json:"feedback_from,omitempty"`
	Repeat             int                      `json:"repeat"`
	TotalRuns          int                      `json:"total_runs"`
	Confirmed          int                      `json:"confirmed"`
	Unconfirmed        int                      `json:"unconfirmed"`
	Errors             int                      `json:"errors"`
	CandidateSummaries []TargetCandidateSummary `json:"candidate_summaries"`
	Results            []TargetSuiteRunResult   `json:"results"`
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
	schedulerMode := suiteSchedulerCaseList
	var (
		tasks                  []string
		taskGroups             []string
		matrix                 *TargetScheduleMatrix
		originalCandidateCount int
		err                    error
	)
	if opts.Matrix {
		schedulerMode = suiteSchedulerMatrix
		matrix, err = BuildTargetScheduleMatrix(TargetMatrixOptions{
			TargetID:         opts.TargetID,
			Tasks:            opts.Tasks,
			TaskGroups:       opts.TaskGroups,
			PromptProfileIDs: target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs),
		})
		if err != nil {
			return nil, err
		}
		originalCandidateCount = matrix.TotalCandidates
		tasks = append([]string{}, matrix.Tasks...)
		taskGroups = append([]string{}, matrix.TaskGroups...)
		if opts.FeedbackFrom != "" || opts.CandidateLimit > 0 || len(opts.ExcludeCandidates) > 0 {
			schedulerMode = suiteSchedulerFeedback
			matrix, err = selectTargetMatrixCandidates(matrix, TargetFeedbackSelectionOptions{
				FeedbackFrom:        opts.FeedbackFrom,
				Limit:               opts.CandidateLimit,
				ExcludeCandidateIDs: opts.ExcludeCandidates,
			})
			if err != nil {
				return nil, err
			}
			tasks = targetCandidateTaskIDs(matrix.Candidates)
		}
	} else {
		tasks, taskGroups, err = target.ExpandTargetTasks(opts.Tasks, opts.TaskGroups)
		if err != nil {
			return nil, err
		}
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
		SchedulerMode:      schedulerMode,
		TimeoutMillis:      opts.Timeout.Milliseconds(),
		ObserveDelayMs:     opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs: opts.LateObserveDelay.Milliseconds(),
		Results:            []TargetSuiteRunResult{},
	}
	if matrix != nil {
		result.PromptProfiles = append([]string{}, matrix.PromptProfiles...)
		result.TotalCandidates = matrix.TotalCandidates
		result.OriginalCandidates = originalCandidateCount
		result.CandidateLimit = opts.CandidateLimit
		result.FeedbackFrom = opts.FeedbackFrom
		result.ScheduleMatrix = filepath.Join(suiteDir, targetScheduleMatrixArtifact)
		result.MatrixResult = filepath.Join(suiteDir, targetMatrixResultArtifact)
	} else if selection := target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs); len(selection) > 0 {
		result.PromptProfiles = append([]string{}, selection...)
	}

	summaries := make(map[string]*TargetSuiteTaskSummary, len(tasks))
	attributionSummary := make(map[string]*TargetSuiteAttributionStats)
	taskAttributions := make(map[string]map[string]*TargetSuiteAttributionStats, len(tasks))
	complianceSummary := make(map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
	taskCompliances := make(map[string]map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats, len(tasks))
	contractSummary := make(map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats)
	taskContracts := make(map[string]map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats, len(tasks))
	for _, taskID := range tasks {
		summaries[taskID] = &TargetSuiteTaskSummary{TaskID: taskID}
		taskAttributions[taskID] = make(map[string]*TargetSuiteAttributionStats)
		taskCompliances[taskID] = make(map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats)
		taskContracts[taskID] = make(map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats)
	}

	for iteration := 1; iteration <= opts.Repeat; iteration++ {
		if matrix != nil {
			for _, candidate := range matrix.Candidates {
				runTargetSuiteTask(ctx, opts, suiteDir, iteration, candidate.TaskID, candidate.PromptProfileID, candidate.CandidateID, summaries, attributionSummary, taskAttributions, complianceSummary, taskCompliances, contractSummary, taskContracts, result)
			}
			continue
		}
		for _, taskID := range tasks {
			runTargetSuiteTask(ctx, opts, suiteDir, iteration, taskID, opts.PromptProfileID, "", summaries, attributionSummary, taskAttributions, complianceSummary, taskCompliances, contractSummary, taskContracts, result)
		}
	}

	result.TotalRuns = len(result.Results)
	result.TaskSummaries = make([]TargetSuiteTaskSummary, 0, len(tasks))
	for _, taskID := range tasks {
		summaries[taskID].AttributionSummaries = targetSuiteAttributionStats(taskAttributions[taskID])
		summaries[taskID].ComplianceSummaries = targetSuiteComplianceStats(taskCompliances[taskID])
		summaries[taskID].ContractSummaries = targetSuiteContractStats(taskContracts[taskID])
		result.TaskSummaries = append(result.TaskSummaries, *summaries[taskID])
	}
	result.AttributionSummaries = targetSuiteAttributionStats(attributionSummary)
	result.ComplianceSummaries = targetSuiteComplianceStats(complianceSummary)
	result.ContractSummaries = targetSuiteContractStats(contractSummary)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if matrix != nil {
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
			TotalCandidates:    result.TotalCandidates,
			OriginalCandidates: result.OriginalCandidates,
			CandidateLimit:     result.CandidateLimit,
			FeedbackFrom:       result.FeedbackFrom,
			Repeat:             result.Repeat,
			TotalRuns:          result.TotalRuns,
			Confirmed:          result.Confirmed,
			Unconfirmed:        result.Unconfirmed,
			Errors:             result.Errors,
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
	taskID string,
	promptProfileID string,
	candidateID string,
	summaries map[string]*TargetSuiteTaskSummary,
	attributionSummary map[string]*TargetSuiteAttributionStats,
	taskAttributions map[string]map[string]*TargetSuiteAttributionStats,
	complianceSummary map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats,
	taskCompliances map[string]map[target.TargetTaskComplianceStatus]*TargetSuiteComplianceStats,
	contractSummary map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats,
	taskContracts map[string]map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats,
	result *TargetSuiteResult,
) {
	runLateObserveDelay := opts.LateObserveDelay
	if runLateObserveDelay == 0 {
		runLateObserveDelay = target.DefaultTargetLateObserveDelay(taskID)
	}
	item := TargetSuiteRunResult{
		CandidateID:        candidateID,
		TaskID:             taskID,
		TargetID:           opts.TargetID,
		PromptProfileID:    target.NormalizeTargetPromptProfileID(promptProfileID),
		Iteration:          iteration,
		LateObserveDelayMs: runLateObserveDelay.Milliseconds(),
		Signature:          target.TargetSignature(taskID),
	}
	startedRun := time.Now()
	runResult, err := target.RunTarget(ctx, target.TargetRunOptions{
		AdapterID:        opts.AdapterID,
		TargetID:         opts.TargetID,
		TaskID:           taskID,
		Objective:        opts.Objective,
		PromptProfileID:  promptProfileID,
		Prompt:           opts.Prompt,
		PromptFile:       opts.PromptFile,
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
	item.Confirmed = runResult.ExpectationsMet
	item.Completed = runResult.Completed
	item.LateObserveDelayMs = runResult.LateObserveDelayMs
	item.TargetOracle = runResult.TargetOracle
	item.TaskCompliance = runResult.TaskCompliance
	item.ContractInterpretation = runResult.ContractInterpretation
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
	recordTargetSuiteAttribution(attributionSummary, item.TargetOracle.Attribution, item.Confirmed)
	recordTargetSuiteAttribution(taskAttributions[taskID], item.TargetOracle.Attribution, item.Confirmed)
	recordTargetSuiteCompliance(complianceSummary, item.TaskCompliance.Status, item.Confirmed)
	recordTargetSuiteCompliance(taskCompliances[taskID], item.TaskCompliance.Status, item.Confirmed)
	recordTargetSuiteContract(contractSummary, target.TargetContractInterpretationStatusValue(item.ContractInterpretation), item.Confirmed)
	recordTargetSuiteContract(taskContracts[taskID], target.TargetContractInterpretationStatusValue(item.ContractInterpretation), item.Confirmed)
	result.Results = append(result.Results, item)
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
