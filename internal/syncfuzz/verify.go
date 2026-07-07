package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type VerifyOptions struct {
	CorpusDir       string
	OutDir          string
	Limit           int
	Delay           time.Duration
	MockURL         string
	EnvKind         string
	ContainerImage  string
	TimingProfileID string
}

type VerificationEntryResult struct {
	ExecutionKind     string                `json:"execution_kind"`
	EntryID           string                `json:"entry_id"`
	Kind              string                `json:"kind"`
	CaseName          string                `json:"case_name"`
	AdapterID         string                `json:"adapter_id,omitempty"`
	TargetID          string                `json:"target_id,omitempty"`
	TaskID            string                `json:"task_id,omitempty"`
	PromptProfileID   string                `json:"prompt_profile_id,omitempty"`
	FaultPlanID       string                `json:"fault_plan_id,omitempty"`
	PrimitiveID       string                `json:"primitive_id,omitempty"`
	TimingProfileID   string                `json:"timing_profile_id,omitempty"`
	Environment       string                `json:"environment,omitempty"`
	ContainerImage    string                `json:"container_image,omitempty"`
	ExpectedSignature MismatchSignature     `json:"expected_signature"`
	ReplayID          string                `json:"replay_id,omitempty"`
	RunID             string                `json:"run_id,omitempty"`
	Confirmed         bool                  `json:"confirmed"`
	ActualSignature   MismatchSignature     `json:"actual_signature,omitempty"`
	SignatureMatched  bool                  `json:"signature_matched"`
	Reproduced        bool                  `json:"reproduced"`
	OutcomeCategory   ReplayOutcomeCategory `json:"outcome_category,omitempty"`
	OutcomeReason     string                `json:"outcome_reason,omitempty"`
	ReplayArtifactDir string                `json:"replay_artifact_dir,omitempty"`
	RunArtifactDir    string                `json:"run_artifact_dir,omitempty"`
	Error             string                `json:"error,omitempty"`
}

type VerificationOutcomeStats struct {
	Category     ReplayOutcomeCategory `json:"category"`
	TotalEntries int                   `json:"total_entries"`
}

type VerificationSubjectStats struct {
	ExecutionKind    string                     `json:"execution_kind"`
	CaseName         string                     `json:"case_name,omitempty"`
	AdapterID        string                     `json:"adapter_id,omitempty"`
	TargetID         string                     `json:"target_id,omitempty"`
	TaskID           string                     `json:"task_id,omitempty"`
	TotalEntries     int                        `json:"total_entries"`
	Reproduced       int                        `json:"reproduced"`
	SignatureDrift   int                        `json:"signature_drift"`
	Unconfirmed      int                        `json:"unconfirmed"`
	Errors           int                        `json:"errors"`
	OutcomeSummaries []VerificationOutcomeStats `json:"outcome_summaries,omitempty"`
}

type VerificationResult struct {
	VerificationID      string                     `json:"verification_id"`
	StartedAt           string                     `json:"started_at"`
	FinishedAt          string                     `json:"finished_at"`
	ArtifactDir         string                     `json:"artifact_dir"`
	CorpusDir           string                     `json:"corpus_dir"`
	Environment         string                     `json:"environment"`
	ContainerImage      string                     `json:"container_image,omitempty"`
	Limit               int                        `json:"limit,omitempty"`
	TotalEntries        int                        `json:"total_entries"`
	Verified            int                        `json:"verified"`
	Reproduced          int                        `json:"reproduced"`
	Failed              int                        `json:"failed"`
	SignatureDrift      int                        `json:"signature_drift"`
	Unconfirmed         int                        `json:"unconfirmed"`
	Errors              int                        `json:"errors"`
	ReproducibilityRate float64                    `json:"reproducibility_rate"`
	OutcomeSummaries    []VerificationOutcomeStats `json:"outcome_summaries,omitempty"`
	SubjectSummaries    []VerificationSubjectStats `json:"subject_summaries,omitempty"`
	Entries             []VerificationEntryResult  `json:"entries"`
}

// VerifyCorpus turns the current corpus into a regression set. Each compact
// entry is replayed independently so one unstable case cannot hide the rest of
// the corpus' health.
func VerifyCorpus(ctx context.Context, opts VerifyOptions) (*VerificationResult, error) {
	if opts.CorpusDir == "" {
		opts.CorpusDir = "corpus"
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	entries, err := ListCorpus(opts.CorpusDir, opts.Limit)
	if err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	verifyID := fmt.Sprintf("verify-%d", started.UnixNano())
	verifyDir := filepath.Join(opts.OutDir, verifyID)
	if err := os.MkdirAll(verifyDir, 0o755); err != nil {
		return nil, fmt.Errorf("create verification directory: %w", err)
	}

	result := &VerificationResult{
		VerificationID: verifyID,
		StartedAt:      started.Format(time.RFC3339Nano),
		ArtifactDir:    verifyDir,
		CorpusDir:      opts.CorpusDir,
		Environment:    normalizedEnvKind(opts.EnvKind),
		ContainerImage: containerImageForResult(opts.EnvKind, opts.ContainerImage),
		Limit:          opts.Limit,
		TotalEntries:   len(entries),
		Entries:        []VerificationEntryResult{},
	}
	outcomeSummary := make(map[ReplayOutcomeCategory]*VerificationOutcomeStats)
	subjectSummary := make(map[string]*VerificationSubjectStats)

	for _, entry := range entries {
		item := VerificationEntryResult{
			ExecutionKind:     entry.EffectiveExecutionKind(),
			EntryID:           entry.EntryID,
			Kind:              entry.Kind,
			CaseName:          entry.Subject(),
			AdapterID:         entry.AdapterID,
			TargetID:          entry.TargetID,
			TaskID:            entry.TaskID,
			PromptProfileID:   entry.PromptProfileID,
			FaultPlanID:       entry.FaultPlanID,
			PrimitiveID:       entry.PrimitiveID,
			TimingProfileID:   entry.TimingProfileID,
			ExpectedSignature: entry.Signature,
		}

		replay, err := replayEntry(ctx, entry, ReplayOptions{
			OutDir:          verifyDir,
			Delay:           opts.Delay,
			MockURL:         opts.MockURL,
			EnvKind:         opts.EnvKind,
			ContainerImage:  opts.ContainerImage,
			TimingProfileID: opts.TimingProfileID,
		})
		if err != nil {
			item.Error = err.Error()
			item.OutcomeCategory = replayOutcomeError
			item.OutcomeReason = "replay execution returned an error before a replay result was recorded"
			result.Errors++
			recordVerificationOutcome(outcomeSummary, item.OutcomeCategory)
			recordVerificationSubject(subjectSummary, item)
			result.Entries = append(result.Entries, item)
			continue
		}

		item.ReplayID = replay.ReplayID
		item.RunID = replay.RunID
		item.AdapterID = firstNonEmpty(replay.AdapterID, item.AdapterID)
		item.TargetID = firstNonEmpty(replay.TargetID, item.TargetID)
		item.TaskID = firstNonEmpty(replay.TaskID, item.TaskID)
		item.PromptProfileID = firstNonEmpty(replay.PromptProfileID, item.PromptProfileID)
		item.Environment = replay.Environment
		item.ContainerImage = replay.ContainerImage
		item.FaultPlanID = replay.FaultPlanID
		item.PrimitiveID = replay.PrimitiveID
		item.TimingProfileID = replay.TimingProfileID
		item.Confirmed = replay.Confirmed
		item.ActualSignature = replay.ActualSignature
		item.SignatureMatched = replay.SignatureMatched
		item.Reproduced = replay.Reproduced
		item.OutcomeCategory = replay.OutcomeCategory
		item.OutcomeReason = replay.OutcomeReason
		item.ReplayArtifactDir = replay.ArtifactDir
		item.RunArtifactDir = replay.RunArtifactDir

		if replay.Reproduced {
			result.Reproduced++
		} else if replay.Confirmed && !replay.SignatureMatched {
			result.SignatureDrift++
		} else if !replay.Confirmed {
			result.Unconfirmed++
		}
		recordVerificationOutcome(outcomeSummary, item.OutcomeCategory)
		recordVerificationSubject(subjectSummary, item)
		result.Entries = append(result.Entries, item)
	}

	result.Verified = len(result.Entries)
	result.Failed = result.TotalEntries - result.Reproduced
	if result.TotalEntries > 0 {
		result.ReproducibilityRate = float64(result.Reproduced) / float64(result.TotalEntries)
	}
	result.OutcomeSummaries = verificationOutcomeStats(outcomeSummary)
	result.SubjectSummaries = verificationSubjectStats(subjectSummary)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeJSON(filepath.Join(verifyDir, "verification-result.json"), result); err != nil {
		return nil, err
	}
	return result, nil
}

func recordVerificationOutcome(stats map[ReplayOutcomeCategory]*VerificationOutcomeStats, category ReplayOutcomeCategory) {
	if category == "" {
		return
	}
	item, ok := stats[category]
	if !ok {
		item = &VerificationOutcomeStats{Category: category}
		stats[category] = item
	}
	item.TotalEntries++
}

func verificationOutcomeStats(stats map[ReplayOutcomeCategory]*VerificationOutcomeStats) []VerificationOutcomeStats {
	if len(stats) == 0 {
		return nil
	}
	order := make([]ReplayOutcomeCategory, 0, len(stats))
	for category := range stats {
		order = append(order, category)
	}
	sort.Slice(order, func(i, j int) bool {
		li := replayOutcomeCategoryOrder(order[i])
		lj := replayOutcomeCategoryOrder(order[j])
		if li != lj {
			return li < lj
		}
		return order[i] < order[j]
	})
	summary := make([]VerificationOutcomeStats, 0, len(order))
	for _, category := range order {
		summary = append(summary, *stats[category])
	}
	return summary
}

func replayOutcomeCategoryOrder(category ReplayOutcomeCategory) int {
	switch category {
	case replayOutcomeReproduced:
		return 0
	case replayOutcomeSignatureDrift:
		return 1
	case replayOutcomeExecutionNotReached:
		return 2
	case replayOutcomeTaskNoncompliant:
		return 3
	case replayOutcomeLifecycleNotTriggered:
		return 4
	case replayOutcomeStateNotPlanted:
		return 5
	case replayOutcomeResidueNotObserved:
		return 6
	case replayOutcomeActivationNotTriggered:
		return 7
	case replayOutcomeOracleInconclusive:
		return 8
	case replayOutcomeCleanNegative:
		return 9
	case replayOutcomeError:
		return 10
	default:
		return 11
	}
}

func verificationSubjectKey(item VerificationEntryResult) string {
	if item.ExecutionKind == corpusExecutionTarget {
		return strings.Join([]string{item.ExecutionKind, item.AdapterID, item.TargetID, item.TaskID}, "\x00")
	}
	return strings.Join([]string{item.ExecutionKind, item.CaseName}, "\x00")
}

func recordVerificationSubject(stats map[string]*VerificationSubjectStats, item VerificationEntryResult) {
	key := verificationSubjectKey(item)
	entry, ok := stats[key]
	if !ok {
		entry = &VerificationSubjectStats{
			ExecutionKind: item.ExecutionKind,
			CaseName:      item.CaseName,
			AdapterID:     item.AdapterID,
			TargetID:      item.TargetID,
			TaskID:        item.TaskID,
		}
		stats[key] = entry
	}
	entry.TotalEntries++
	if item.Reproduced {
		entry.Reproduced++
	} else if item.Confirmed && !item.SignatureMatched {
		entry.SignatureDrift++
	} else if item.Error != "" {
		entry.Errors++
	} else {
		entry.Unconfirmed++
	}

	outcomeStats := make(map[ReplayOutcomeCategory]*VerificationOutcomeStats, len(entry.OutcomeSummaries))
	for i := range entry.OutcomeSummaries {
		current := entry.OutcomeSummaries[i]
		copyItem := current
		outcomeStats[current.Category] = &copyItem
	}
	recordVerificationOutcome(outcomeStats, item.OutcomeCategory)
	entry.OutcomeSummaries = verificationOutcomeStats(outcomeStats)
}

func verificationSubjectStats(stats map[string]*VerificationSubjectStats) []VerificationSubjectStats {
	if len(stats) == 0 {
		return nil
	}
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := stats[keys[i]]
		right := stats[keys[j]]
		if left.ExecutionKind != right.ExecutionKind {
			return left.ExecutionKind < right.ExecutionKind
		}
		if left.TargetID != right.TargetID {
			return left.TargetID < right.TargetID
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		return left.CaseName < right.CaseName
	})
	result := make([]VerificationSubjectStats, 0, len(keys))
	for _, key := range keys {
		result = append(result, *stats[key])
	}
	return result
}
