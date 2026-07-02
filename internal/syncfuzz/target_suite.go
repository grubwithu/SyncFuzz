package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TargetSuiteOptions struct {
	AdapterID        string
	TargetID         string
	Tasks            []string
	Objective        string
	Prompt           string
	PromptFile       string
	Command          string
	CommandFile      string
	OutDir           string
	CorpusDir        string
	Repeat           int
	Timeout          time.Duration
	ObserveDelay     time.Duration
	LateObserveDelay time.Duration
	EnvKind          string
	ContainerImage   string
	ExpectedFiles    []string
}

type TargetSuiteRunResult struct {
	TaskID             string             `json:"task_id"`
	Iteration          int                `json:"iteration"`
	RunID              string             `json:"run_id,omitempty"`
	Confirmed          bool               `json:"confirmed"`
	Completed          bool               `json:"completed"`
	LateObserveDelayMs int64              `json:"late_observe_delay_ms,omitempty"`
	TargetOracle       TargetOracleResult `json:"target_oracle"`
	Signature          MismatchSignature  `json:"signature"`
	ArtifactDir        string             `json:"artifact_dir,omitempty"`
	DurationMillis     int64              `json:"duration_ms,omitempty"`
	ArtifactBytes      int64              `json:"artifact_bytes,omitempty"`
	ArtifactFiles      int                `json:"artifact_files,omitempty"`
	MetricError        string             `json:"metric_error,omitempty"`
	Error              string             `json:"error,omitempty"`
}

type TargetSuiteTaskSummary struct {
	TaskID      string `json:"task_id"`
	TotalRuns   int    `json:"total_runs"`
	Confirmed   int    `json:"confirmed"`
	Unconfirmed int    `json:"unconfirmed"`
	Errors      int    `json:"errors"`
}

type TargetSuiteResult struct {
	SchemaVersion      string                   `json:"schema_version"`
	SuiteID            string                   `json:"suite_id"`
	StartedAt          string                   `json:"started_at"`
	FinishedAt         string                   `json:"finished_at"`
	ArtifactDir        string                   `json:"artifact_dir"`
	AdapterID          string                   `json:"adapter_id"`
	TargetID           string                   `json:"target_id"`
	Environment        string                   `json:"environment"`
	ContainerImage     string                   `json:"container_image,omitempty"`
	Repeat             int                      `json:"repeat"`
	Tasks              []string                 `json:"tasks"`
	TimeoutMillis      int64                    `json:"timeout_ms"`
	ObserveDelayMs     int64                    `json:"observe_delay_ms"`
	LateObserveDelayMs int64                    `json:"late_observe_delay_ms,omitempty"`
	TotalRuns          int                      `json:"total_runs"`
	Confirmed          int                      `json:"confirmed"`
	Unconfirmed        int                      `json:"unconfirmed"`
	Errors             int                      `json:"errors"`
	TaskSummaries      []TargetSuiteTaskSummary `json:"task_summaries"`
	Results            []TargetSuiteRunResult   `json:"results"`
	CorpusEntries      []CorpusEntry            `json:"corpus_entries,omitempty"`
}

const targetSuiteResultArtifact = "target-suite-result.json"

func RunTargetSuite(ctx context.Context, opts TargetSuiteOptions) (*TargetSuiteResult, error) {
	if opts.AdapterID == "" {
		opts.AdapterID = defaultTargetAdapterID
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
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	tasks := normalizeTargetTasks(opts.Tasks)
	started := time.Now().UTC()
	suiteID := fmt.Sprintf("target-suite-%d", started.UnixNano())
	suiteDir := filepath.Join(opts.OutDir, suiteID)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target suite directory: %w", err)
	}

	result := &TargetSuiteResult{
		SchemaVersion:      "syncfuzz.target-suite-result.v1",
		SuiteID:            suiteID,
		StartedAt:          started.Format(time.RFC3339Nano),
		ArtifactDir:        suiteDir,
		AdapterID:          opts.AdapterID,
		TargetID:           opts.TargetID,
		Environment:        normalizedEnvKind(opts.EnvKind),
		ContainerImage:     containerImageForResult(opts.EnvKind, opts.ContainerImage),
		Repeat:             opts.Repeat,
		Tasks:              append([]string{}, tasks...),
		TimeoutMillis:      opts.Timeout.Milliseconds(),
		ObserveDelayMs:     opts.ObserveDelay.Milliseconds(),
		LateObserveDelayMs: opts.LateObserveDelay.Milliseconds(),
		Results:            []TargetSuiteRunResult{},
	}

	summaries := make(map[string]*TargetSuiteTaskSummary, len(tasks))
	for _, taskID := range tasks {
		summaries[taskID] = &TargetSuiteTaskSummary{TaskID: taskID}
	}

	for iteration := 1; iteration <= opts.Repeat; iteration++ {
		for _, taskID := range tasks {
			runLateObserveDelay := opts.LateObserveDelay
			if runLateObserveDelay == 0 {
				runLateObserveDelay = defaultTargetLateObserveDelay(taskID)
			}
			item := TargetSuiteRunResult{
				TaskID:             taskID,
				Iteration:          iteration,
				LateObserveDelayMs: runLateObserveDelay.Milliseconds(),
				Signature:          targetSignature(taskID),
			}
			startedRun := time.Now()
			runResult, err := RunTarget(ctx, TargetRunOptions{
				AdapterID:        opts.AdapterID,
				TargetID:         opts.TargetID,
				TaskID:           taskID,
				Objective:        opts.Objective,
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
				continue
			}

			item.RunID = runResult.RunID
			item.Confirmed = runResult.ExpectationsMet
			item.Completed = runResult.Completed
			item.LateObserveDelayMs = runResult.LateObserveDelayMs
			item.TargetOracle = runResult.TargetOracle
			item.Signature = runResult.Signature
			item.ArtifactDir = runResult.ArtifactDir
			finalizeTargetSuiteItemMetrics(&item, startedRun)

			result.TotalRuns++
			summaries[taskID].TotalRuns++
			if item.Confirmed {
				result.Confirmed++
				summaries[taskID].Confirmed++
			} else {
				result.Unconfirmed++
				summaries[taskID].Unconfirmed++
			}
			result.Results = append(result.Results, item)
		}
	}

	result.TotalRuns = len(result.Results)
	result.TaskSummaries = make([]TargetSuiteTaskSummary, 0, len(tasks))
	for _, taskID := range tasks {
		result.TaskSummaries = append(result.TaskSummaries, *summaries[taskID])
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	corpusEntries, err := WriteTargetCorpus(opts.CorpusDir, result)
	if err != nil {
		return nil, err
	}
	result.CorpusEntries = corpusEntries
	if err := writeJSON(filepath.Join(suiteDir, targetSuiteResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func finalizeTargetSuiteItemMetrics(item *TargetSuiteRunResult, started time.Time) {
	item.DurationMillis = time.Since(started).Milliseconds()
	if item.ArtifactDir == "" {
		return
	}
	metrics, err := measureArtifactMetrics(item.ArtifactDir)
	if err != nil {
		item.MetricError = err.Error()
		return
	}
	item.ArtifactBytes = metrics.Bytes
	item.ArtifactFiles = metrics.Files
}

func normalizeTargetTasks(tasks []string) []string {
	var normalized []string
	seen := make(map[string]struct{})
	for _, taskID := range tasks {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		normalized = append(normalized, taskID)
	}
	if len(normalized) == 0 {
		return []string{defaultTargetTaskID}
	}
	return normalized
}
