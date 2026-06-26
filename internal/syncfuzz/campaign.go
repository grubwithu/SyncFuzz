package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CampaignOptions struct {
	OutDir           string
	CorpusDir        string
	Rounds           int
	Repeat           int
	CandidateLimit   int
	Cases            []string
	TimingProfileIDs []string
	Delay            time.Duration
	MockURL          string
	EnvKind          string
	ContainerImage   string
	Differential     bool
	FeedbackFrom     string
}

type CampaignRoundResult struct {
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
	Discoveries        int                     `json:"discoveries"`
	CorpusEntries      int                     `json:"corpus_entries"`
	TopCandidate       *MatrixCandidateSummary `json:"top_candidate,omitempty"`
}

type CampaignResult struct {
	SchemaVersion      string                `json:"schema_version"`
	CampaignID         string                `json:"campaign_id"`
	StartedAt          string                `json:"started_at"`
	FinishedAt         string                `json:"finished_at"`
	ArtifactDir        string                `json:"artifact_dir"`
	Environment        string                `json:"environment"`
	ContainerImage     string                `json:"container_image,omitempty"`
	Rounds             int                   `json:"rounds"`
	Repeat             int                   `json:"repeat"`
	CandidateLimit     int                   `json:"candidate_limit,omitempty"`
	SeedFeedbackFrom   string                `json:"seed_feedback_from,omitempty"`
	Cases              []string              `json:"cases,omitempty"`
	TimingProfileIDs   []string              `json:"timing_profile_ids,omitempty"`
	Differential       bool                  `json:"differential"`
	TotalSuites        int                   `json:"total_suites"`
	TotalRuns          int                   `json:"total_runs"`
	Confirmed          int                   `json:"confirmed"`
	Unconfirmed        int                   `json:"unconfirmed"`
	Errors             int                   `json:"errors"`
	Discoveries        int                   `json:"discoveries"`
	CorpusEntries      int                   `json:"corpus_entries"`
	UniqueCandidates   int                   `json:"unique_candidates"`
	RepeatedCandidates int                   `json:"repeated_candidates"`
	RoundResults       []CampaignRoundResult `json:"round_results"`
}

const campaignResultArtifact = "campaign-result.json"

func RunCampaign(ctx context.Context, opts CampaignOptions) (*CampaignResult, error) {
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
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if opts.CandidateLimit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}
	if len(opts.Cases) > 0 {
		if err := validateCaseNames(opts.Cases); err != nil {
			return nil, err
		}
	}
	if _, err := resolveMatrixTimingProfiles(opts.TimingProfileIDs); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	campaignID := fmt.Sprintf("campaign-%d", started.UnixNano())
	campaignDir := filepath.Join(opts.OutDir, campaignID)
	if err := os.MkdirAll(campaignDir, 0o755); err != nil {
		return nil, fmt.Errorf("create campaign directory: %w", err)
	}

	result := &CampaignResult{
		SchemaVersion:    "syncfuzz.campaign-result.v1",
		CampaignID:       campaignID,
		StartedAt:        started.Format(time.RFC3339Nano),
		ArtifactDir:      campaignDir,
		Environment:      normalizedEnvKind(opts.EnvKind),
		ContainerImage:   containerImageForResult(opts.EnvKind, opts.ContainerImage),
		Rounds:           opts.Rounds,
		Repeat:           opts.Repeat,
		CandidateLimit:   opts.CandidateLimit,
		SeedFeedbackFrom: opts.FeedbackFrom,
		Cases:            append([]string{}, opts.Cases...),
		TimingProfileIDs: append([]string{}, opts.TimingProfileIDs...),
		Differential:     opts.Differential,
		RoundResults:     []CampaignRoundResult{},
	}

	feedbackFrom := opts.FeedbackFrom
	seenCandidates := make(map[string]struct{})
	for round := 1; round <= opts.Rounds; round++ {
		suite, err := RunSuite(ctx, SuiteOptions{
			OutDir:            campaignDir,
			Repeat:            opts.Repeat,
			Cases:             opts.Cases,
			Delay:             opts.Delay,
			MockURL:           opts.MockURL,
			CorpusDir:         opts.CorpusDir,
			EnvKind:           opts.EnvKind,
			ContainerImage:    opts.ContainerImage,
			Differential:      opts.Differential,
			Matrix:            true,
			TimingProfileIDs:  opts.TimingProfileIDs,
			FeedbackFrom:      feedbackFrom,
			CandidateLimit:    opts.CandidateLimit,
			ExcludeCandidates: sortedSet(seenCandidates),
		})
		if err != nil {
			return nil, fmt.Errorf("campaign round %d failed: %w", round, err)
		}

		roundResult := campaignRoundResult(round, suite)
		result.RoundResults = append(result.RoundResults, roundResult)
		result.TotalRuns += suite.TotalRuns
		result.Confirmed += suite.Confirmed
		result.Unconfirmed += suite.Unconfirmed
		result.Errors += suite.Errors
		result.Discoveries += len(suite.Discoveries)
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
	if err := writeJSON(filepath.Join(campaignDir, campaignResultArtifact), result); err != nil {
		return nil, err
	}
	return result, nil
}

func campaignRoundResult(round int, suite *SuiteResult) CampaignRoundResult {
	item := CampaignRoundResult{
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
		Discoveries:        len(suite.Discoveries),
		CorpusEntries:      len(suite.CorpusEntries),
	}
	if len(suite.CandidateSummaries) > 0 {
		top := suite.CandidateSummaries[0]
		item.TopCandidate = &top
	}
	return item
}
