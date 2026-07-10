package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetCampaignOptions struct {
	AdapterID            string
	TargetID             string
	Tasks                []string
	TaskGroups           []string
	SeedIDs              []string
	Objective            string
	PromptProfileID      string
	PromptProfileIDs     []string
	Prompt               string
	PromptFile           string
	Command              string
	CommandFile          string
	OutDir               string
	CorpusDir            string
	Rounds               int
	Repeat               int
	CandidateLimit       int
	FeedbackFrom         string
	MinCoverageGainScore int
	MaxStagnantRounds    int
	AutoPivot            bool
	Timeout              time.Duration
	ObserveDelay         time.Duration
	LateObserveDelay     time.Duration
	EnvKind              string
	ContainerImage       string
	ExpectedFiles        []string
}

type TargetCampaignRoundResult struct {
	Round               int                                  `json:"round"`
	SuiteID             string                               `json:"suite_id"`
	SchedulerMode       string                               `json:"scheduler_mode"`
	ArtifactDir         string                               `json:"artifact_dir"`
	MatrixResult        string                               `json:"matrix_result"`
	FeedbackFrom        string                               `json:"feedback_from,omitempty"`
	OriginalCandidates  int                                  `json:"original_candidates,omitempty"`
	TotalCandidates     int                                  `json:"total_candidates"`
	TotalRuns           int                                  `json:"total_runs"`
	Confirmed           int                                  `json:"confirmed"`
	Unconfirmed         int                                  `json:"unconfirmed"`
	Errors              int                                  `json:"errors"`
	OutcomeSummaries    []TargetSuiteOutcomeStats            `json:"outcome_summaries,omitempty"`
	ActivationSummaries []TargetSuiteActivationStats         `json:"activation_summaries,omitempty"`
	DimensionCoverage   []TargetDimensionCoverageSummary     `json:"dimension_coverage,omitempty"`
	CoverageGain        []TargetDimensionCoverageGainSummary `json:"coverage_gain,omitempty"`
	CoverageGainStats   TargetDimensionCoverageGainStats     `json:"coverage_gain_stats,omitempty"`
	FrontierCandidates  []TargetFrontierCandidate            `json:"frontier_candidates,omitempty"`
	CorpusEntries       int                                  `json:"corpus_entries"`
	TopCandidate        *TargetCandidateSummary              `json:"top_candidate,omitempty"`
}

type TargetCampaignPivotEvent struct {
	AfterRound        int      `json:"after_round"`
	Dimension         string   `json:"dimension"`
	Values            []string `json:"values,omitempty"`
	Tasks             []string `json:"tasks,omitempty"`
	SeedIDs           []string `json:"seed_ids,omitempty"`
	PromptProfiles    []string `json:"prompt_profiles,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	NewCandidateCount int      `json:"new_candidate_count,omitempty"`
	FrontierCandidate string   `json:"frontier_candidate,omitempty"`
	FrontierGapScore  int      `json:"frontier_gap_score,omitempty"`
	FrontierNovelty   int      `json:"frontier_novelty_score,omitempty"`
	FrontierSelection string   `json:"frontier_selection_mode,omitempty"`
}

type TargetCampaignResult struct {
	SchemaVersion        string                              `json:"schema_version"`
	CampaignID           string                              `json:"campaign_id"`
	StartedAt            string                              `json:"started_at"`
	FinishedAt           string                              `json:"finished_at"`
	ArtifactDir          string                              `json:"artifact_dir"`
	AdapterID            string                              `json:"adapter_id"`
	TargetID             string                              `json:"target_id"`
	Environment          string                              `json:"environment"`
	ContainerImage       string                              `json:"container_image,omitempty"`
	Rounds               int                                 `json:"rounds"`
	Repeat               int                                 `json:"repeat"`
	CandidateLimit       int                                 `json:"candidate_limit,omitempty"`
	SeedFeedbackFrom     string                              `json:"seed_feedback_from,omitempty"`
	Tasks                []string                            `json:"tasks,omitempty"`
	TaskGroups           []string                            `json:"task_groups,omitempty"`
	SeedIDs              []string                            `json:"seed_ids,omitempty"`
	PromptProfiles       []string                            `json:"prompt_profiles,omitempty"`
	TotalSuites          int                                 `json:"total_suites"`
	TotalRuns            int                                 `json:"total_runs"`
	Confirmed            int                                 `json:"confirmed"`
	Unconfirmed          int                                 `json:"unconfirmed"`
	Errors               int                                 `json:"errors"`
	OutcomeSummaries     []TargetSuiteOutcomeStats           `json:"outcome_summaries,omitempty"`
	ActivationSummaries  []TargetSuiteActivationStats        `json:"activation_summaries,omitempty"`
	DimensionCoverage    []TargetDimensionCoverageSummary    `json:"dimension_coverage,omitempty"`
	FrontierCandidates   []TargetFrontierCandidate           `json:"frontier_candidates,omitempty"`
	CorpusEntries        int                                 `json:"corpus_entries"`
	UniqueCandidates     int                                 `json:"unique_candidates"`
	RepeatedCandidates   int                                 `json:"repeated_candidates"`
	MinCoverageGainScore int                                 `json:"min_coverage_gain_score,omitempty"`
	MaxStagnantRounds    int                                 `json:"max_stagnant_rounds,omitempty"`
	AutoPivot            bool                                `json:"auto_pivot,omitempty"`
	StoppedEarly         bool                                `json:"stopped_early,omitempty"`
	StopReason           string                              `json:"stop_reason,omitempty"`
	PivotRecommendations []TargetCampaignPivotRecommendation `json:"pivot_recommendations,omitempty"`
	CatalogExhausted     bool                                `json:"catalog_exhausted,omitempty"`
	PivotHistory         []TargetCampaignPivotEvent          `json:"pivot_history,omitempty"`
	RoundResults         []TargetCampaignRoundResult         `json:"round_results"`
}

const targetCampaignResultArtifact = "target-campaign-result.json"

func RunTargetCampaign(ctx context.Context, opts TargetCampaignOptions) (*TargetCampaignResult, error) {
	if opts.AdapterID == "" {
		opts.AdapterID = target.DefaultTargetAdapterID
	}
	if opts.TargetID == "" {
		opts.TargetID = opts.AdapterID
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.CorpusDir == "" {
		opts.CorpusDir = "corpus"
	}
	if opts.Rounds <= 0 {
		opts.Rounds = 2
	}
	if opts.Repeat <= 0 {
		opts.Repeat = 1
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.CandidateLimit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}
	if opts.MaxStagnantRounds < 0 {
		return nil, fmt.Errorf("max stagnant rounds cannot be negative")
	}
	if err := environment.ValidateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	campaignID := fmt.Sprintf("target-campaign-%d", started.UnixNano())
	campaignDir := filepath.Join(opts.OutDir, campaignID)
	if err := os.MkdirAll(campaignDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target campaign directory: %w", err)
	}

	result := &TargetCampaignResult{
		SchemaVersion:        "syncfuzz.target-campaign-result.v1",
		CampaignID:           campaignID,
		StartedAt:            started.Format(time.RFC3339Nano),
		ArtifactDir:          campaignDir,
		AdapterID:            opts.AdapterID,
		TargetID:             opts.TargetID,
		Environment:          environment.NormalizedEnvKind(opts.EnvKind),
		ContainerImage:       environment.ContainerImageForResult(opts.EnvKind, opts.ContainerImage),
		Rounds:               opts.Rounds,
		Repeat:               opts.Repeat,
		CandidateLimit:       opts.CandidateLimit,
		SeedFeedbackFrom:     opts.FeedbackFrom,
		MinCoverageGainScore: opts.MinCoverageGainScore,
		MaxStagnantRounds:    opts.MaxStagnantRounds,
		AutoPivot:            opts.AutoPivot,
		Tasks:                append([]string{}, opts.Tasks...),
		TaskGroups:           append([]string{}, opts.TaskGroups...),
		SeedIDs:              append([]string{}, opts.SeedIDs...),
		PromptProfiles:       append([]string{}, target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs)...),
		PivotHistory:         []TargetCampaignPivotEvent{},
		RoundResults:         []TargetCampaignRoundResult{},
	}
	runningOpts := opts
	universe, err := buildTargetCampaignUniverse(runningOpts)
	if err != nil {
		return nil, err
	}
	outcomeSummary := make(map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats)
	activationSummary := make(map[TargetActivationStage]*TargetSuiteActivationStats)
	allResults := make([]TargetSuiteRunResult, 0)

	feedbackFrom := opts.FeedbackFrom
	seenCandidates := make(map[string]struct{})
	stagnantRounds := 0
	for round := 1; round <= opts.Rounds; round++ {
		suite, err := RunTargetSuite(ctx, TargetSuiteOptions{
			AdapterID:         runningOpts.AdapterID,
			TargetID:          runningOpts.TargetID,
			Tasks:             runningOpts.Tasks,
			TaskGroups:        runningOpts.TaskGroups,
			SeedIDs:           runningOpts.SeedIDs,
			Objective:         runningOpts.Objective,
			PromptProfileID:   runningOpts.PromptProfileID,
			PromptProfileIDs:  runningOpts.PromptProfileIDs,
			Prompt:            runningOpts.Prompt,
			PromptFile:        runningOpts.PromptFile,
			Command:           runningOpts.Command,
			CommandFile:       runningOpts.CommandFile,
			OutDir:            campaignDir,
			CorpusDir:         runningOpts.CorpusDir,
			Repeat:            runningOpts.Repeat,
			Timeout:           runningOpts.Timeout,
			ObserveDelay:      runningOpts.ObserveDelay,
			LateObserveDelay:  runningOpts.LateObserveDelay,
			EnvKind:           runningOpts.EnvKind,
			ContainerImage:    runningOpts.ContainerImage,
			ExpectedFiles:     runningOpts.ExpectedFiles,
			Matrix:            true,
			FeedbackFrom:      feedbackFrom,
			CandidateLimit:    runningOpts.CandidateLimit,
			ExcludeCandidates: sortedSet(seenCandidates),
		})
		if err != nil {
			return nil, fmt.Errorf("target campaign round %d failed: %w", round, err)
		}
		if len(result.PromptProfiles) == 0 && len(suite.PromptProfiles) > 0 {
			result.PromptProfiles = append([]string{}, suite.PromptProfiles...)
		}
		if len(result.SeedIDs) == 0 && len(suite.SeedIDs) > 0 {
			result.SeedIDs = append([]string{}, suite.SeedIDs...)
		}

		roundGain := summarizeTargetDimensionCoverageGain(universe.Candidates, allResults, suite.Results)
		roundResult := targetCampaignRoundResult(round, suite, roundGain)
		result.RoundResults = append(result.RoundResults, roundResult)
		result.TotalRuns += suite.TotalRuns
		result.Confirmed += suite.Confirmed
		result.Unconfirmed += suite.Unconfirmed
		result.Errors += suite.Errors
		result.CorpusEntries += len(suite.CorpusEntries)
		mergeTargetSuiteOutcomeStats(outcomeSummary, suite.OutcomeSummaries)
		mergeTargetSuiteActivationStats(activationSummary, suite.ActivationSummaries)
		allResults = append(allResults, suite.Results...)
		for _, item := range suite.Results {
			if item.CandidateID == "" {
				continue
			}
			if _, ok := seenCandidates[item.CandidateID]; ok {
				result.RepeatedCandidates++
			}
			seenCandidates[item.CandidateID] = struct{}{}
		}
		if suite.TotalCandidates == 0 {
			if opts.AutoPivot {
				nextOpts, nextUniverse, pivotEvent, ok, err := applyTargetCampaignPivot(runningOpts, universe, allResults, round)
				if err != nil {
					return nil, err
				}
				if ok {
					runningOpts = nextOpts
					universe = nextUniverse
					result.Tasks = append([]string{}, nextUniverse.Tasks...)
					result.TaskGroups = append([]string{}, nextUniverse.TaskGroups...)
					result.SeedIDs = append([]string{}, nextUniverse.SeedIDs...)
					result.PromptProfiles = append([]string{}, nextUniverse.PromptProfiles...)
					result.PivotHistory = append(result.PivotHistory, *pivotEvent)
					stagnantRounds = 0
					feedbackFrom = ""
					continue
				}
			}
			result.StoppedEarly = true
			result.StopReason = targetCampaignExhaustedStopReason()
			break
		}
		feedbackFrom = suite.MatrixResult
		if opts.MaxStagnantRounds > 0 && round < opts.Rounds {
			if roundResult.CoverageGainStats.WeightedScore <= opts.MinCoverageGainScore {
				stagnantRounds++
			} else {
				stagnantRounds = 0
			}
			if stagnantRounds >= opts.MaxStagnantRounds {
				if opts.AutoPivot {
					nextOpts, nextUniverse, pivotEvent, ok, err := applyTargetCampaignPivot(runningOpts, universe, allResults, round)
					if err != nil {
						return nil, err
					}
					if ok {
						runningOpts = nextOpts
						universe = nextUniverse
						result.Tasks = append([]string{}, nextUniverse.Tasks...)
						result.TaskGroups = append([]string{}, nextUniverse.TaskGroups...)
						result.SeedIDs = append([]string{}, nextUniverse.SeedIDs...)
						result.PromptProfiles = append([]string{}, nextUniverse.PromptProfiles...)
						result.PivotHistory = append(result.PivotHistory, *pivotEvent)
						stagnantRounds = 0
						feedbackFrom = ""
						continue
					}
				}
				result.StoppedEarly = true
				result.StopReason = targetCampaignStopReason(stagnantRounds, opts.MinCoverageGainScore, roundResult.CoverageGainStats.WeightedScore)
				break
			}
		}
	}

	result.TotalSuites = len(result.RoundResults)
	result.UniqueCandidates = len(seenCandidates)
	result.OutcomeSummaries = targetSuiteOutcomeStats(outcomeSummary)
	result.ActivationSummaries = targetSuiteActivationStats(activationSummary)
	result.DimensionCoverage = summarizeTargetDimensionCoverage(universe.Candidates, allResults)
	result.FrontierCandidates = summarizeTargetCoverageFrontier(universe, allResults, nil, targetFrontierDefaultLimit)
	result.PivotRecommendations, result.CatalogExhausted = summarizeTargetCampaignPivotRecommendations(universe)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := core.WriteJSON(filepath.Join(campaignDir, targetCampaignResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func targetCampaignStopReason(stagnantRounds int, threshold int, score int) string {
	return fmt.Sprintf("coverage gain score %d stayed at or below threshold %d for %d consecutive rounds", score, threshold, stagnantRounds)
}

func targetCampaignExhaustedStopReason() string {
	return "current campaign universe has no unexplored candidates remaining"
}

func buildTargetCampaignUniverse(opts TargetCampaignOptions) (*TargetScheduleMatrix, error) {
	return BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:         opts.TargetID,
		Tasks:            opts.Tasks,
		TaskGroups:       opts.TaskGroups,
		SeedIDs:          opts.SeedIDs,
		PromptProfileIDs: target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs),
	})
}

func applyTargetCampaignPivot(
	opts TargetCampaignOptions,
	currentUniverse *TargetScheduleMatrix,
	previousResults []TargetSuiteRunResult,
	afterRound int,
) (TargetCampaignOptions, *TargetScheduleMatrix, *TargetCampaignPivotEvent, bool, error) {
	recommendations, _ := summarizeTargetCampaignPivotRecommendations(currentUniverse)
	for _, dimension := range targetCampaignAutoPivotDimensions() {
		recommendation, ok := targetCampaignPivotRecommendationByDimension(recommendations, dimension)
		if !ok {
			continue
		}
		nextOpts, nextUniverse, event, ok, err := targetCampaignBestPivotOption(opts, currentUniverse, previousResults, recommendation, afterRound)
		if err != nil {
			return TargetCampaignOptions{}, nil, nil, false, fmt.Errorf("apply target campaign pivot %s: %w", recommendation.Dimension, err)
		}
		if ok {
			return nextOpts, nextUniverse, event, true, nil
		}
	}
	return TargetCampaignOptions{}, nil, nil, false, nil
}

func targetCampaignAutoPivotDimensions() []string {
	return []string{
		"prompt_profile_id",
		"seed_id",
		"state_surface",
		"plant_primitive_id",
		"activation_kind_id",
		"oracle_kind_id",
	}
}

func targetCampaignPivotRecommendationByDimension(recommendations []TargetCampaignPivotRecommendation, dimension string) (TargetCampaignPivotRecommendation, bool) {
	for _, recommendation := range recommendations {
		if recommendation.Dimension == dimension {
			return recommendation, true
		}
	}
	return TargetCampaignPivotRecommendation{}, false
}

func mergeStringLists(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range base {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range extra {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func targetTaskIDsForDimensionValues(dimension string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	want := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		want[value] = struct{}{}
	}
	tasks := target.TargetTasks()
	out := make([]string, 0, len(tasks))
	for _, taskInfo := range tasks {
		var current string
		switch dimension {
		case "state_surface":
			current = taskInfo.StateSurface
		case "plant_primitive_id":
			current = taskInfo.PlantPrimitiveID
		case "activation_kind_id":
			current = taskInfo.ActivationKindID
		case "oracle_kind_id":
			current = taskInfo.OracleKindID
		default:
			continue
		}
		if _, ok := want[current]; ok {
			out = append(out, taskInfo.TaskID)
		}
	}
	return out
}

func targetScheduleUniverseExpanded(current *TargetScheduleMatrix, next *TargetScheduleMatrix) bool {
	if next == nil {
		return false
	}
	if current == nil {
		return len(next.Candidates) > 0
	}
	seen := make(map[string]struct{}, len(current.Candidates))
	for _, candidate := range current.Candidates {
		seen[candidate.CandidateID] = struct{}{}
	}
	for _, candidate := range next.Candidates {
		if _, ok := seen[candidate.CandidateID]; !ok {
			return true
		}
	}
	return false
}

func targetCampaignRoundResult(round int, suite *TargetSuiteResult, coverageGain []TargetDimensionCoverageGainSummary) TargetCampaignRoundResult {
	item := TargetCampaignRoundResult{
		Round:               round,
		SuiteID:             suite.SuiteID,
		SchedulerMode:       suite.SchedulerMode,
		ArtifactDir:         suite.ArtifactDir,
		MatrixResult:        suite.MatrixResult,
		FeedbackFrom:        suite.FeedbackFrom,
		OriginalCandidates:  suite.OriginalCandidates,
		TotalCandidates:     suite.TotalCandidates,
		TotalRuns:           suite.TotalRuns,
		Confirmed:           suite.Confirmed,
		Unconfirmed:         suite.Unconfirmed,
		Errors:              suite.Errors,
		OutcomeSummaries:    append([]TargetSuiteOutcomeStats{}, suite.OutcomeSummaries...),
		ActivationSummaries: append([]TargetSuiteActivationStats{}, suite.ActivationSummaries...),
		DimensionCoverage:   append([]TargetDimensionCoverageSummary{}, suite.DimensionCoverage...),
		CoverageGain:        append([]TargetDimensionCoverageGainSummary{}, coverageGain...),
		CoverageGainStats:   summarizeTargetDimensionCoverageGainStats(coverageGain),
		FrontierCandidates:  append([]TargetFrontierCandidate{}, suite.FrontierCandidates...),
		CorpusEntries:       len(suite.CorpusEntries),
	}
	if len(suite.CandidateSummaries) > 0 {
		top := suite.CandidateSummaries[0]
		item.TopCandidate = &top
	}
	return item
}

func mergeTargetSuiteOutcomeStats(dst map[corpus.TargetObservationCategory]*TargetSuiteOutcomeStats, src []TargetSuiteOutcomeStats) {
	for _, item := range src {
		current, ok := dst[item.Category]
		if !ok {
			copyItem := item
			dst[item.Category] = &copyItem
			continue
		}
		current.TotalRuns += item.TotalRuns
		current.Confirmed += item.Confirmed
		current.Unconfirmed += item.Unconfirmed
	}
}

func mergeTargetSuiteActivationStats(dst map[TargetActivationStage]*TargetSuiteActivationStats, src []TargetSuiteActivationStats) {
	for _, item := range src {
		current, ok := dst[item.Stage]
		if !ok {
			copyItem := item
			dst[item.Stage] = &copyItem
			continue
		}
		current.TotalRuns += item.TotalRuns
		current.Confirmed += item.Confirmed
		current.Unconfirmed += item.Unconfirmed
	}
}
