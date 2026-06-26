package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SuiteOptions struct {
	OutDir            string
	Repeat            int
	Cases             []string
	Delay             time.Duration
	MockURL           string
	CorpusDir         string
	EnvKind           string
	ContainerImage    string
	Differential      bool
	TimingProfileID   string
	Matrix            bool
	TimingProfileIDs  []string
	FeedbackFrom      string
	CandidateLimit    int
	ExcludeCandidates []string
}

type SuiteCaseResult struct {
	CandidateID        string            `json:"candidate_id,omitempty"`
	CaseName           string            `json:"case_name"`
	Iteration          int               `json:"iteration"`
	RunID              string            `json:"run_id,omitempty"`
	PairID             string            `json:"pair_id,omitempty"`
	ControlRunID       string            `json:"control_run_id,omitempty"`
	FaultRunID         string            `json:"fault_run_id,omitempty"`
	FaultPlanID        string            `json:"fault_plan_id,omitempty"`
	PrimitiveID        string            `json:"primitive_id,omitempty"`
	TimingProfileID    string            `json:"timing_profile_id,omitempty"`
	Confirmed          bool              `json:"confirmed"`
	Signature          MismatchSignature `json:"signature,omitempty"`
	Differential       bool              `json:"differential,omitempty"`
	SecurityRelevant   bool              `json:"security_relevant,omitempty"`
	DifferentialReport string            `json:"differential_report,omitempty"`
	Interesting        bool              `json:"interesting"`
	Novelty            []string          `json:"novelty,omitempty"`
	Score              int               `json:"score"`
	ArtifactDir        string            `json:"artifact_dir,omitempty"`
	DurationMillis     int64             `json:"duration_ms,omitempty"`
	ArtifactBytes      int64             `json:"artifact_bytes,omitempty"`
	ArtifactFiles      int               `json:"artifact_files,omitempty"`
	MetricError        string            `json:"metric_error,omitempty"`
	Error              string            `json:"error,omitempty"`
}

type SuiteDiscovery struct {
	Kind               string            `json:"kind"`
	Key                string            `json:"key"`
	CandidateID        string            `json:"candidate_id,omitempty"`
	CaseName           string            `json:"case_name"`
	Iteration          int               `json:"iteration"`
	RunID              string            `json:"run_id"`
	PairID             string            `json:"pair_id,omitempty"`
	ControlRunID       string            `json:"control_run_id,omitempty"`
	FaultRunID         string            `json:"fault_run_id,omitempty"`
	FaultPlanID        string            `json:"fault_plan_id,omitempty"`
	PrimitiveID        string            `json:"primitive_id,omitempty"`
	TimingProfileID    string            `json:"timing_profile_id,omitempty"`
	Signature          MismatchSignature `json:"signature"`
	Differential       bool              `json:"differential,omitempty"`
	SecurityRelevant   bool              `json:"security_relevant,omitempty"`
	DifferentialReport string            `json:"differential_report,omitempty"`
	ArtifactDir        string            `json:"artifact_dir"`
}

type SuiteResult struct {
	SuiteID            string                   `json:"suite_id"`
	StartedAt          string                   `json:"started_at"`
	FinishedAt         string                   `json:"finished_at"`
	ArtifactDir        string                   `json:"artifact_dir"`
	Environment        string                   `json:"environment"`
	ContainerImage     string                   `json:"container_image,omitempty"`
	SchedulerMode      string                   `json:"scheduler_mode"`
	Repeat             int                      `json:"repeat"`
	Differential       bool                     `json:"differential"`
	TimingProfileID    string                   `json:"timing_profile_id,omitempty"`
	TimingProfileIDs   []string                 `json:"timing_profile_ids,omitempty"`
	TotalCandidates    int                      `json:"total_candidates,omitempty"`
	OriginalCandidates int                      `json:"original_candidates,omitempty"`
	CandidateLimit     int                      `json:"candidate_limit,omitempty"`
	FeedbackFrom       string                   `json:"feedback_from,omitempty"`
	ScheduleMatrix     string                   `json:"schedule_matrix,omitempty"`
	MatrixResult       string                   `json:"matrix_result,omitempty"`
	CandidateSummaries []MatrixCandidateSummary `json:"candidate_summaries,omitempty"`
	TotalRuns          int                      `json:"total_runs"`
	Confirmed          int                      `json:"confirmed"`
	Unconfirmed        int                      `json:"unconfirmed"`
	Errors             int                      `json:"errors"`
	UniqueSignatures   int                      `json:"unique_signatures"`
	UniqueStateClasses int                      `json:"unique_state_classes"`
	UniqueImpacts      int                      `json:"unique_impacts"`
	Discoveries        []SuiteDiscovery         `json:"discoveries"`
	CorpusEntries      []CorpusEntry            `json:"corpus_entries,omitempty"`
	Results            []SuiteCaseResult        `json:"results"`
}

type SuiteMatrixResult struct {
	SchemaVersion      string                   `json:"schema_version"`
	SuiteID            string                   `json:"suite_id"`
	GeneratedAt        string                   `json:"generated_at"`
	ScheduleMatrix     string                   `json:"schedule_matrix"`
	TotalCandidates    int                      `json:"total_candidates"`
	OriginalCandidates int                      `json:"original_candidates,omitempty"`
	CandidateLimit     int                      `json:"candidate_limit,omitempty"`
	FeedbackFrom       string                   `json:"feedback_from,omitempty"`
	Repeat             int                      `json:"repeat"`
	TotalRuns          int                      `json:"total_runs"`
	Confirmed          int                      `json:"confirmed"`
	Unconfirmed        int                      `json:"unconfirmed"`
	Errors             int                      `json:"errors"`
	CandidateSummaries []MatrixCandidateSummary `json:"candidate_summaries"`
	Results            []SuiteCaseResult        `json:"results"`
}

const (
	suiteSchedulerCaseList = "case-list"
	suiteSchedulerMatrix   = "matrix"
	suiteSchedulerFeedback = "matrix-feedback"
	scheduleMatrixArtifact = "schedule-matrix.json"
	matrixResultArtifact   = "matrix-result.json"
)

// RunSuite is the first scheduler-shaped API. It still runs deterministically,
// but it gives us a stable place to add repetition, selection, baselines, and
// feedback-guided scheduling later.
func RunSuite(ctx context.Context, opts SuiteOptions) (*SuiteResult, error) {
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Repeat <= 0 {
		opts.Repeat = 1
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	selected := opts.Cases
	if len(selected) == 0 {
		selected = caseNames()
	}
	if err := validateCaseNames(selected); err != nil {
		return nil, err
	}

	schedulerMode := suiteSchedulerCaseList
	var matrix *ScheduleMatrix
	originalCandidateCount := 0
	if opts.Matrix {
		schedulerMode = suiteSchedulerMatrix
		var err error
		matrix, err = BuildScheduleMatrix(MatrixOptions{
			Cases:            selected,
			TimingProfileIDs: opts.TimingProfileIDs,
		})
		if err != nil {
			return nil, err
		}
		originalCandidateCount = matrix.TotalCandidates
		if opts.FeedbackFrom != "" || opts.CandidateLimit > 0 {
			schedulerMode = suiteSchedulerFeedback
			matrix, err = selectScheduleMatrixCandidates(matrix, FeedbackSelectionOptions{
				FeedbackFrom:        opts.FeedbackFrom,
				Limit:               opts.CandidateLimit,
				ExcludeCandidateIDs: opts.ExcludeCandidates,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	started := time.Now().UTC()
	suiteID := fmt.Sprintf("suite-%d", started.UnixNano())
	suiteDir := filepath.Join(opts.OutDir, suiteID)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return nil, fmt.Errorf("create suite directory: %w", err)
	}
	if matrix != nil {
		if err := writeJSON(filepath.Join(suiteDir, scheduleMatrixArtifact), matrix); err != nil {
			return nil, err
		}
	}

	result := &SuiteResult{
		SuiteID:         suiteID,
		StartedAt:       started.Format(time.RFC3339Nano),
		ArtifactDir:     suiteDir,
		Environment:     normalizedEnvKind(opts.EnvKind),
		ContainerImage:  containerImageForResult(opts.EnvKind, opts.ContainerImage),
		SchedulerMode:   schedulerMode,
		Repeat:          opts.Repeat,
		Differential:    opts.Differential,
		TimingProfileID: opts.TimingProfileID,
		Discoveries:     []SuiteDiscovery{},
		Results:         []SuiteCaseResult{},
	}
	if matrix != nil {
		result.TimingProfileIDs = append([]string{}, matrix.TimingProfiles...)
		result.TotalCandidates = matrix.TotalCandidates
		result.OriginalCandidates = originalCandidateCount
		result.CandidateLimit = opts.CandidateLimit
		result.FeedbackFrom = opts.FeedbackFrom
		result.ScheduleMatrix = filepath.Join(suiteDir, scheduleMatrixArtifact)
		result.MatrixResult = filepath.Join(suiteDir, matrixResultArtifact)
	}
	feedback := newSuiteFeedback()

	for iteration := 1; iteration <= opts.Repeat; iteration++ {
		if matrix != nil {
			for _, candidate := range matrix.Candidates {
				item := SuiteCaseResult{
					CandidateID:     candidate.CandidateID,
					CaseName:        candidate.CaseName,
					Iteration:       iteration,
					FaultPlanID:     candidate.FaultPlanID,
					PrimitiveID:     candidate.PrimitiveID,
					TimingProfileID: candidate.TimingProfileID,
				}
				runSuiteItem(ctx, opts, suiteDir, &item, feedback, result)
			}
			continue
		}
		for _, caseName := range selected {
			item := SuiteCaseResult{
				CaseName:  caseName,
				Iteration: iteration,
			}
			runSuiteItem(ctx, opts, suiteDir, &item, feedback, result)
		}
	}

	result.TotalRuns = len(result.Results)
	result.UniqueSignatures = len(feedback.signatures)
	result.UniqueStateClasses = len(feedback.stateClasses)
	result.UniqueImpacts = len(feedback.impacts)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if matrix != nil {
		result.CandidateSummaries = summarizeMatrixCandidates(result.Results)
	}
	corpusEntries, err := WriteCorpus(opts.CorpusDir, result)
	if err != nil {
		return nil, err
	}
	result.CorpusEntries = corpusEntries
	if err := writeJSON(filepath.Join(suiteDir, "suite-result.json"), result); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(suiteDir, "interesting.json"), result.Discoveries); err != nil {
		return nil, err
	}
	if matrix != nil {
		matrixResult := SuiteMatrixResult{
			SchemaVersion:      "syncfuzz.matrix-result.v1",
			SuiteID:            result.SuiteID,
			GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
			ScheduleMatrix:     result.ScheduleMatrix,
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
		if err := writeJSON(filepath.Join(suiteDir, matrixResultArtifact), matrixResult); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func runSuiteItem(ctx context.Context, opts SuiteOptions, suiteDir string, item *SuiteCaseResult, feedback *suiteFeedback, result *SuiteResult) {
	started := time.Now()
	if opts.Differential {
		pairResult, err := RunPair(ctx, PairOptions{
			CaseName:        item.CaseName,
			OutDir:          suiteDir,
			Delay:           opts.Delay,
			MockURL:         opts.MockURL,
			EnvKind:         opts.EnvKind,
			ContainerImage:  opts.ContainerImage,
			FaultPlanID:     item.FaultPlanID,
			PrimitiveID:     item.PrimitiveID,
			TimingProfileID: firstNonEmpty(item.TimingProfileID, opts.TimingProfileID),
		})
		if err != nil {
			item.Error = err.Error()
			finalizeSuiteItemMetrics(item, started)
			result.Errors++
			result.Results = append(result.Results, *item)
			return
		}
		applyPairResult(item, pairResult)
	} else {
		runResult, err := Run(ctx, RunOptions{
			CaseName:        item.CaseName,
			OutDir:          suiteDir,
			Delay:           opts.Delay,
			MockURL:         opts.MockURL,
			EnvKind:         opts.EnvKind,
			ContainerImage:  opts.ContainerImage,
			FaultPlanID:     item.FaultPlanID,
			PrimitiveID:     item.PrimitiveID,
			TimingProfileID: firstNonEmpty(item.TimingProfileID, opts.TimingProfileID),
		})
		if err != nil {
			item.Error = err.Error()
			finalizeSuiteItemMetrics(item, started)
			result.Errors++
			result.Results = append(result.Results, *item)
			return
		}
		applyRunResult(item, runResult)
	}
	finalizeSuiteItemMetrics(item, started)
	if item.Confirmed {
		result.Confirmed++
		feedback.Apply(item, result)
	} else {
		result.Unconfirmed++
	}
	result.Results = append(result.Results, *item)
}

func finalizeSuiteItemMetrics(item *SuiteCaseResult, started time.Time) {
	item.DurationMillis = time.Since(started).Milliseconds()
	metrics, err := measureArtifactMetrics(item.ArtifactDir)
	if err != nil {
		item.MetricError = err.Error()
		return
	}
	item.ArtifactBytes = metrics.Bytes
	item.ArtifactFiles = metrics.Files
}

func applyRunResult(item *SuiteCaseResult, runResult *RunResult) {
	item.RunID = runResult.RunID
	item.FaultRunID = runResult.RunID
	item.FaultPlanID = runResult.FaultPlanID
	item.PrimitiveID = firstNonEmpty(item.PrimitiveID, runResult.PrimitiveID)
	item.TimingProfileID = runResult.TimingProfileID
	item.Confirmed = runResult.Confirmed
	item.Signature = runResult.Signature
	item.ArtifactDir = runResult.ArtifactDir
}

func applyPairResult(item *SuiteCaseResult, pairResult *PairResult) {
	item.RunID = pairResult.Fault.RunID
	item.PairID = pairResult.PairID
	item.ControlRunID = pairResult.Control.RunID
	item.FaultRunID = pairResult.Fault.RunID
	item.FaultPlanID = pairResult.FaultPlanID
	item.PrimitiveID = firstNonEmpty(item.PrimitiveID, pairResult.PrimitiveID)
	item.TimingProfileID = pairResult.TimingProfileID
	item.Confirmed = pairResult.Verdict.SecurityRelevant
	item.Signature = pairResult.Fault.Signature
	item.Differential = pairResult.Verdict.Differential
	item.SecurityRelevant = pairResult.Verdict.SecurityRelevant
	item.DifferentialReport = filepath.Join(pairResult.ArtifactDir, differentialReportArtifact)
	item.ArtifactDir = pairResult.ArtifactDir
}

type suiteFeedback struct {
	signatures   map[string]struct{}
	stateClasses map[string]struct{}
	impacts      map[string]struct{}
}

func newSuiteFeedback() *suiteFeedback {
	return &suiteFeedback{
		signatures:   make(map[string]struct{}),
		stateClasses: make(map[string]struct{}),
		impacts:      make(map[string]struct{}),
	}
}

func (f *suiteFeedback) Apply(item *SuiteCaseResult, result *SuiteResult) {
	f.observe("new-signature", item.Signature.String(), 10, item, result, f.signatures)
	f.observe("new-state-class", item.Signature.StateClass, 3, item, result, f.stateClasses)
	f.observe("new-impact", item.Signature.Impact, 5, item, result, f.impacts)
	if item.Score > 0 {
		item.Interesting = true
	}
}

func (f *suiteFeedback) observe(kind string, key string, score int, item *SuiteCaseResult, result *SuiteResult, seen map[string]struct{}) {
	if key == "" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	item.Novelty = append(item.Novelty, kind)
	item.Score += score
	result.Discoveries = append(result.Discoveries, SuiteDiscovery{
		Kind:               kind,
		Key:                key,
		CandidateID:        item.CandidateID,
		CaseName:           item.CaseName,
		Iteration:          item.Iteration,
		RunID:              item.RunID,
		PairID:             item.PairID,
		ControlRunID:       item.ControlRunID,
		FaultRunID:         item.FaultRunID,
		FaultPlanID:        item.FaultPlanID,
		PrimitiveID:        item.PrimitiveID,
		TimingProfileID:    item.TimingProfileID,
		Signature:          item.Signature,
		Differential:       item.Differential,
		SecurityRelevant:   item.SecurityRelevant,
		DifferentialReport: item.DifferentialReport,
		ArtifactDir:        item.ArtifactDir,
	})
}

func caseNames() []string {
	cases := Cases()
	names := make([]string, 0, len(cases))
	for _, c := range cases {
		names = append(names, c.Name)
	}
	return names
}

func validateCaseNames(names []string) error {
	known := make(map[string]struct{})
	for _, name := range caseNames() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown case %q", name)
		}
	}
	return nil
}
