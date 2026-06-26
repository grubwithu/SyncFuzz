package syncfuzz

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type FeedbackSelectionOptions struct {
	FeedbackFrom        string
	Limit               int
	ExcludeCandidateIDs []string
}

func selectScheduleMatrixCandidates(matrix *ScheduleMatrix, opts FeedbackSelectionOptions) (*ScheduleMatrix, error) {
	if matrix == nil {
		return nil, fmt.Errorf("schedule matrix is required")
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("candidate limit cannot be negative")
	}

	candidates := append([]ScheduleCandidate{}, matrix.Candidates...)
	if opts.FeedbackFrom != "" {
		feedback, err := readMatrixFeedback(opts.FeedbackFrom)
		if err != nil {
			return nil, err
		}
		summaryByCandidate := make(map[string]MatrixCandidateSummary, len(feedback.CandidateSummaries))
		for _, summary := range feedback.CandidateSummaries {
			summaryByCandidate[summary.CandidateID] = summary
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left, leftOK := summaryByCandidate[candidates[i].CandidateID]
			right, rightOK := summaryByCandidate[candidates[j].CandidateID]
			switch {
			case leftOK && rightOK:
				return matrixFeedbackSummaryLess(left, right)
			case leftOK:
				return true
			case rightOK:
				return false
			default:
				return candidates[i].CandidateID < candidates[j].CandidateID
			}
		})
	}
	if len(opts.ExcludeCandidateIDs) > 0 {
		filtered := filterExcludedCandidates(candidates, opts.ExcludeCandidateIDs)
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	if opts.Limit > 0 && len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}

	selected := *matrix
	selected.Candidates = append([]ScheduleCandidate{}, candidates...)
	selected.TotalCandidates = len(selected.Candidates)
	return &selected, nil
}

func filterExcludedCandidates(candidates []ScheduleCandidate, exclude []string) []ScheduleCandidate {
	excluded := make(map[string]struct{}, len(exclude))
	for _, id := range exclude {
		excluded[id] = struct{}{}
	}
	out := make([]ScheduleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := excluded[candidate.CandidateID]; ok {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func readMatrixFeedback(path string) (*SuiteMatrixResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read feedback matrix result %s: %w", path, err)
	}
	var result SuiteMatrixResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode feedback matrix result %s: %w", path, err)
	}
	if result.SchemaVersion != "syncfuzz.matrix-result.v1" {
		return nil, fmt.Errorf("unsupported feedback matrix schema %q", result.SchemaVersion)
	}
	if len(result.CandidateSummaries) == 0 {
		return nil, fmt.Errorf("feedback matrix result %s has no candidate_summaries", path)
	}
	return &result, nil
}

func matrixFeedbackSummaryLess(left MatrixCandidateSummary, right MatrixCandidateSummary) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.ReproducibilityRate != right.ReproducibilityRate {
		return left.ReproducibilityRate > right.ReproducibilityRate
	}
	if left.Confirmed != right.Confirmed {
		return left.Confirmed > right.Confirmed
	}
	if left.Errors != right.Errors {
		return left.Errors < right.Errors
	}
	if left.CostPenalty != right.CostPenalty {
		return left.CostPenalty < right.CostPenalty
	}
	if left.AvgDurationMillis != right.AvgDurationMillis {
		return left.AvgDurationMillis < right.AvgDurationMillis
	}
	if left.AvgArtifactBytes != right.AvgArtifactBytes {
		return left.AvgArtifactBytes < right.AvgArtifactBytes
	}
	return left.CandidateID < right.CandidateID
}
