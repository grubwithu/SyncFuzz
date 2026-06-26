package syncfuzz

import "sort"

type MatrixCandidateSummary struct {
	Rank                int      `json:"rank"`
	CandidateID         string   `json:"candidate_id"`
	CaseName            string   `json:"case_name"`
	PrimitiveID         string   `json:"primitive_id"`
	TimingProfileID     string   `json:"timing_profile_id"`
	Runs                int      `json:"runs"`
	Confirmed           int      `json:"confirmed"`
	Unconfirmed         int      `json:"unconfirmed"`
	Errors              int      `json:"errors"`
	InterestingRuns     int      `json:"interesting_runs"`
	NoveltyScore        int      `json:"novelty_score"`
	Score               int      `json:"score"`
	CostPenalty         int      `json:"cost_penalty"`
	ReproducibilityRate float64  `json:"reproducibility_rate"`
	TotalDurationMillis int64    `json:"total_duration_ms"`
	AvgDurationMillis   int64    `json:"avg_duration_ms"`
	TotalArtifactBytes  int64    `json:"total_artifact_bytes"`
	AvgArtifactBytes    int64    `json:"avg_artifact_bytes"`
	MaxArtifactBytes    int64    `json:"max_artifact_bytes"`
	TotalArtifactFiles  int      `json:"total_artifact_files"`
	AvgArtifactFiles    int      `json:"avg_artifact_files"`
	Status              string   `json:"status"`
	Signatures          []string `json:"signatures,omitempty"`
	StateClasses        []string `json:"state_classes,omitempty"`
	Impacts             []string `json:"impacts,omitempty"`
}

type matrixCandidateAccumulator struct {
	summary      MatrixCandidateSummary
	signatures   map[string]struct{}
	stateClasses map[string]struct{}
	impacts      map[string]struct{}
}

func summarizeMatrixCandidates(results []SuiteCaseResult) []MatrixCandidateSummary {
	accumulators := make(map[string]*matrixCandidateAccumulator)
	for _, result := range results {
		if result.CandidateID == "" {
			continue
		}
		accumulator := accumulators[result.CandidateID]
		if accumulator == nil {
			accumulator = &matrixCandidateAccumulator{
				summary: MatrixCandidateSummary{
					CandidateID:     result.CandidateID,
					CaseName:        result.CaseName,
					PrimitiveID:     result.PrimitiveID,
					TimingProfileID: result.TimingProfileID,
				},
				signatures:   make(map[string]struct{}),
				stateClasses: make(map[string]struct{}),
				impacts:      make(map[string]struct{}),
			}
			accumulators[result.CandidateID] = accumulator
		}
		accumulator.observe(result)
	}

	summaries := make([]MatrixCandidateSummary, 0, len(accumulators))
	for _, accumulator := range accumulators {
		summary := accumulator.summary
		if summary.Runs > 0 {
			summary.ReproducibilityRate = float64(summary.Confirmed) / float64(summary.Runs)
			summary.AvgDurationMillis = summary.TotalDurationMillis / int64(summary.Runs)
			summary.AvgArtifactBytes = summary.TotalArtifactBytes / int64(summary.Runs)
			summary.AvgArtifactFiles = summary.TotalArtifactFiles / summary.Runs
		}
		summary.Score = matrixCandidateScore(summary)
		summary.CostPenalty = matrixCandidateCostPenalty(summary)
		summary.Status = matrixCandidateStatus(summary)
		summary.Signatures = sortedSet(accumulator.signatures)
		summary.StateClasses = sortedSet(accumulator.stateClasses)
		summary.Impacts = sortedSet(accumulator.impacts)
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Score != summaries[j].Score {
			return summaries[i].Score > summaries[j].Score
		}
		if summaries[i].ReproducibilityRate != summaries[j].ReproducibilityRate {
			return summaries[i].ReproducibilityRate > summaries[j].ReproducibilityRate
		}
		if summaries[i].Confirmed != summaries[j].Confirmed {
			return summaries[i].Confirmed > summaries[j].Confirmed
		}
		if summaries[i].CostPenalty != summaries[j].CostPenalty {
			return summaries[i].CostPenalty < summaries[j].CostPenalty
		}
		if summaries[i].AvgDurationMillis != summaries[j].AvgDurationMillis {
			return summaries[i].AvgDurationMillis < summaries[j].AvgDurationMillis
		}
		if summaries[i].AvgArtifactBytes != summaries[j].AvgArtifactBytes {
			return summaries[i].AvgArtifactBytes < summaries[j].AvgArtifactBytes
		}
		return summaries[i].CandidateID < summaries[j].CandidateID
	})
	for i := range summaries {
		summaries[i].Rank = i + 1
	}
	return summaries
}

func (a *matrixCandidateAccumulator) observe(result SuiteCaseResult) {
	a.summary.Runs++
	switch {
	case result.Error != "":
		a.summary.Errors++
	case result.Confirmed:
		a.summary.Confirmed++
	default:
		a.summary.Unconfirmed++
	}
	if result.Interesting {
		a.summary.InterestingRuns++
	}
	a.summary.NoveltyScore += result.Score
	a.summary.TotalDurationMillis += result.DurationMillis
	a.summary.TotalArtifactBytes += result.ArtifactBytes
	a.summary.TotalArtifactFiles += result.ArtifactFiles
	if result.ArtifactBytes > a.summary.MaxArtifactBytes {
		a.summary.MaxArtifactBytes = result.ArtifactBytes
	}
	if signaturePresent(result.Signature) {
		a.signatures[result.Signature.String()] = struct{}{}
	}
	if result.Signature.StateClass != "" {
		a.stateClasses[result.Signature.StateClass] = struct{}{}
	}
	if result.Signature.Impact != "" {
		a.impacts[result.Signature.Impact] = struct{}{}
	}
}

func matrixCandidateScore(summary MatrixCandidateSummary) int {
	return summary.NoveltyScore + summary.Confirmed*2 + summary.InterestingRuns*3 - summary.Errors*5
}

func matrixCandidateCostPenalty(summary MatrixCandidateSummary) int {
	durationPenalty := int(summary.AvgDurationMillis / 1000)
	artifactPenalty := int(summary.AvgArtifactBytes / (1024 * 1024))
	filePenalty := summary.AvgArtifactFiles / 100
	return durationPenalty + artifactPenalty + filePenalty
}

func matrixCandidateStatus(summary MatrixCandidateSummary) string {
	switch {
	case summary.Runs == 0:
		return "not-run"
	case summary.Errors == summary.Runs:
		return "error"
	case summary.Confirmed > 0:
		return "confirmed"
	case summary.Unconfirmed > 0:
		return "unconfirmed"
	default:
		return "unknown"
	}
}

func signaturePresent(signature MismatchSignature) bool {
	return signature.LifecycleEvent != "" ||
		signature.FaultPhase != "" ||
		signature.StateClass != "" ||
		signature.Operation != "" ||
		signature.Relation != "" ||
		signature.Impact != ""
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
