package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetCampaignOptions struct {
	AdapterID        string
	TargetID         string
	Tasks            []string
	TaskGroups       []string
	Objective        string
	PromptProfileID  string
	PromptProfileIDs []string
	Prompt           string
	PromptFile       string
	Command          string
	CommandFile      string
	OutDir           string
	CorpusDir        string
	Rounds           int
	Repeat           int
	CandidateLimit   int
	FeedbackFrom     string
	Timeout          time.Duration
	ObserveDelay     time.Duration
	LateObserveDelay time.Duration
	EnvKind          string
	ContainerImage   string
	ExpectedFiles    []string
}

type TargetCampaignRoundResult struct {
	Round              int                     `json:"round"`
	SuiteID            string                  `json:"suite_id"`
	SchedulerMode      string                  `json:"scheduler_mode"`
	ArtifactDir        string                  `json:"artifact_dir"`
	MatrixResult       string                  `json:"matrix_result"`
	FeedbackFrom       string                  `json:"feedback_from,omitempty"`
	OriginalCandidates int                     `json:"original_candidates,omitempty"`
	TotalCandidates    int                     `json:"total_candidates"`
	TotalRuns          int                     `json:"total_runs"`
	Confirmed          int                     `json:"confirmed"`
	Unconfirmed        int                     `json:"unconfirmed"`
	Errors             int                     `json:"errors"`
	CorpusEntries      int                     `json:"corpus_entries"`
	TopCandidate       *TargetCandidateSummary `json:"top_candidate,omitempty"`
}

type TargetCampaignResult struct {
	SchemaVersion      string                      `json:"schema_version"`
	CampaignID         string                      `json:"campaign_id"`
	StartedAt          string                      `json:"started_at"`
	FinishedAt         string                      `json:"finished_at"`
	ArtifactDir        string                      `json:"artifact_dir"`
	AdapterID          string                      `json:"adapter_id"`
	TargetID           string                      `json:"target_id"`
	Environment        string                      `json:"environment"`
	ContainerImage     string                      `json:"container_image,omitempty"`
	Rounds             int                         `json:"rounds"`
	Repeat             int                         `json:"repeat"`
	CandidateLimit     int                         `json:"candidate_limit,omitempty"`
	SeedFeedbackFrom   string                      `json:"seed_feedback_from,omitempty"`
	Tasks              []string                    `json:"tasks,omitempty"`
	TaskGroups         []string                    `json:"task_groups,omitempty"`
	PromptProfiles     []string                    `json:"prompt_profiles,omitempty"`
	TotalSuites        int                         `json:"total_suites"`
	TotalRuns          int                         `json:"total_runs"`
	Confirmed          int                         `json:"confirmed"`
	Unconfirmed        int                         `json:"unconfirmed"`
	Errors             int                         `json:"errors"`
	CorpusEntries      int                         `json:"corpus_entries"`
	UniqueCandidates   int                         `json:"unique_candidates"`
	RepeatedCandidates int                         `json:"repeated_candidates"`
	RoundResults       []TargetCampaignRoundResult `json:"round_results"`
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
		SchemaVersion:    "syncfuzz.target-campaign-result.v1",
		CampaignID:       campaignID,
		StartedAt:        started.Format(time.RFC3339Nano),
		ArtifactDir:      campaignDir,
		AdapterID:        opts.AdapterID,
		TargetID:         opts.TargetID,
		Environment:      environment.NormalizedEnvKind(opts.EnvKind),
		ContainerImage:   environment.ContainerImageForResult(opts.EnvKind, opts.ContainerImage),
		Rounds:           opts.Rounds,
		Repeat:           opts.Repeat,
		CandidateLimit:   opts.CandidateLimit,
		SeedFeedbackFrom: opts.FeedbackFrom,
		Tasks:            append([]string{}, opts.Tasks...),
		TaskGroups:       append([]string{}, opts.TaskGroups...),
		PromptProfiles:   append([]string{}, target.TargetPromptProfileSelection(opts.PromptProfileID, opts.PromptProfileIDs)...),
		RoundResults:     []TargetCampaignRoundResult{},
	}

	feedbackFrom := opts.FeedbackFrom
	seenCandidates := make(map[string]struct{})
	for round := 1; round <= opts.Rounds; round++ {
		suite, err := RunTargetSuite(ctx, TargetSuiteOptions{
			AdapterID:         opts.AdapterID,
			TargetID:          opts.TargetID,
			Tasks:             opts.Tasks,
			TaskGroups:        opts.TaskGroups,
			Objective:         opts.Objective,
			PromptProfileID:   opts.PromptProfileID,
			PromptProfileIDs:  opts.PromptProfileIDs,
			Prompt:            opts.Prompt,
			PromptFile:        opts.PromptFile,
			Command:           opts.Command,
			CommandFile:       opts.CommandFile,
			OutDir:            campaignDir,
			CorpusDir:         opts.CorpusDir,
			Repeat:            opts.Repeat,
			Timeout:           opts.Timeout,
			ObserveDelay:      opts.ObserveDelay,
			LateObserveDelay:  opts.LateObserveDelay,
			EnvKind:           opts.EnvKind,
			ContainerImage:    opts.ContainerImage,
			ExpectedFiles:     opts.ExpectedFiles,
			Matrix:            true,
			FeedbackFrom:      feedbackFrom,
			CandidateLimit:    opts.CandidateLimit,
			ExcludeCandidates: sortedSet(seenCandidates),
		})
		if err != nil {
			return nil, fmt.Errorf("target campaign round %d failed: %w", round, err)
		}
		if len(result.PromptProfiles) == 0 && len(suite.PromptProfiles) > 0 {
			result.PromptProfiles = append([]string{}, suite.PromptProfiles...)
		}

		roundResult := targetCampaignRoundResult(round, suite)
		result.RoundResults = append(result.RoundResults, roundResult)
		result.TotalRuns += suite.TotalRuns
		result.Confirmed += suite.Confirmed
		result.Unconfirmed += suite.Unconfirmed
		result.Errors += suite.Errors
		result.CorpusEntries += len(suite.CorpusEntries)
		for _, item := range suite.Results {
			if item.CandidateID == "" {
				continue
			}
			if _, ok := seenCandidates[item.CandidateID]; ok {
				result.RepeatedCandidates++
			}
			seenCandidates[item.CandidateID] = struct{}{}
		}
		feedbackFrom = suite.MatrixResult
	}

	result.TotalSuites = len(result.RoundResults)
	result.UniqueCandidates = len(seenCandidates)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := core.WriteJSON(filepath.Join(campaignDir, targetCampaignResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func targetCampaignRoundResult(round int, suite *TargetSuiteResult) TargetCampaignRoundResult {
	item := TargetCampaignRoundResult{
		Round:              round,
		SuiteID:            suite.SuiteID,
		SchedulerMode:      suite.SchedulerMode,
		ArtifactDir:        suite.ArtifactDir,
		MatrixResult:       suite.MatrixResult,
		FeedbackFrom:       suite.FeedbackFrom,
		OriginalCandidates: suite.OriginalCandidates,
		TotalCandidates:    suite.TotalCandidates,
		TotalRuns:          suite.TotalRuns,
		Confirmed:          suite.Confirmed,
		Unconfirmed:        suite.Unconfirmed,
		Errors:             suite.Errors,
		CorpusEntries:      len(suite.CorpusEntries),
	}
	if len(suite.CandidateSummaries) > 0 {
		top := suite.CandidateSummaries[0]
		item.TopCandidate = &top
	}
	return item
}
