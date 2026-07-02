package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	ExecutionKind     string            `json:"execution_kind"`
	EntryID           string            `json:"entry_id"`
	Kind              string            `json:"kind"`
	CaseName          string            `json:"case_name"`
	AdapterID         string            `json:"adapter_id,omitempty"`
	TargetID          string            `json:"target_id,omitempty"`
	TaskID            string            `json:"task_id,omitempty"`
	FaultPlanID       string            `json:"fault_plan_id,omitempty"`
	PrimitiveID       string            `json:"primitive_id,omitempty"`
	TimingProfileID   string            `json:"timing_profile_id,omitempty"`
	Environment       string            `json:"environment,omitempty"`
	ContainerImage    string            `json:"container_image,omitempty"`
	ExpectedSignature MismatchSignature `json:"expected_signature"`
	ReplayID          string            `json:"replay_id,omitempty"`
	RunID             string            `json:"run_id,omitempty"`
	Confirmed         bool              `json:"confirmed"`
	ActualSignature   MismatchSignature `json:"actual_signature,omitempty"`
	SignatureMatched  bool              `json:"signature_matched"`
	Reproduced        bool              `json:"reproduced"`
	ReplayArtifactDir string            `json:"replay_artifact_dir,omitempty"`
	RunArtifactDir    string            `json:"run_artifact_dir,omitempty"`
	Error             string            `json:"error,omitempty"`
}

type VerificationResult struct {
	VerificationID      string                    `json:"verification_id"`
	StartedAt           string                    `json:"started_at"`
	FinishedAt          string                    `json:"finished_at"`
	ArtifactDir         string                    `json:"artifact_dir"`
	CorpusDir           string                    `json:"corpus_dir"`
	Environment         string                    `json:"environment"`
	ContainerImage      string                    `json:"container_image,omitempty"`
	Limit               int                       `json:"limit,omitempty"`
	TotalEntries        int                       `json:"total_entries"`
	Verified            int                       `json:"verified"`
	Reproduced          int                       `json:"reproduced"`
	Failed              int                       `json:"failed"`
	SignatureDrift      int                       `json:"signature_drift"`
	Unconfirmed         int                       `json:"unconfirmed"`
	Errors              int                       `json:"errors"`
	ReproducibilityRate float64                   `json:"reproducibility_rate"`
	Entries             []VerificationEntryResult `json:"entries"`
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

	for _, entry := range entries {
		item := VerificationEntryResult{
			ExecutionKind:     entry.EffectiveExecutionKind(),
			EntryID:           entry.EntryID,
			Kind:              entry.Kind,
			CaseName:          entry.Subject(),
			AdapterID:         entry.AdapterID,
			TargetID:          entry.TargetID,
			TaskID:            entry.TaskID,
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
			result.Errors++
			result.Entries = append(result.Entries, item)
			continue
		}

		item.ReplayID = replay.ReplayID
		item.RunID = replay.RunID
		item.AdapterID = firstNonEmpty(replay.AdapterID, item.AdapterID)
		item.TargetID = firstNonEmpty(replay.TargetID, item.TargetID)
		item.TaskID = firstNonEmpty(replay.TaskID, item.TaskID)
		item.Environment = replay.Environment
		item.ContainerImage = replay.ContainerImage
		item.FaultPlanID = replay.FaultPlanID
		item.PrimitiveID = replay.PrimitiveID
		item.TimingProfileID = replay.TimingProfileID
		item.Confirmed = replay.Confirmed
		item.ActualSignature = replay.ActualSignature
		item.SignatureMatched = replay.SignatureMatched
		item.Reproduced = replay.Reproduced
		item.ReplayArtifactDir = replay.ArtifactDir
		item.RunArtifactDir = replay.RunArtifactDir

		if replay.Reproduced {
			result.Reproduced++
		} else if replay.Confirmed && !replay.SignatureMatched {
			result.SignatureDrift++
		} else if !replay.Confirmed {
			result.Unconfirmed++
		}
		result.Entries = append(result.Entries, item)
	}

	result.Verified = len(result.Entries)
	result.Failed = result.TotalEntries - result.Reproduced
	if result.TotalEntries > 0 {
		result.ReproducibilityRate = float64(result.Reproduced) / float64(result.TotalEntries)
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeJSON(filepath.Join(verifyDir, "verification-result.json"), result); err != nil {
		return nil, err
	}
	return result, nil
}
