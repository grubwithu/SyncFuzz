package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ReplayOptions struct {
	CorpusDir      string
	EntryID        string
	OutDir         string
	Delay          time.Duration
	MockURL        string
	EnvKind        string
	ContainerImage string
	FaultPlanID    string
}

type ReplayResult struct {
	ReplayID          string            `json:"replay_id"`
	EntryID           string            `json:"entry_id"`
	CaseName          string            `json:"case_name"`
	Environment       string            `json:"environment"`
	ContainerImage    string            `json:"container_image,omitempty"`
	FaultPlanID       string            `json:"fault_plan_id,omitempty"`
	SourceSuiteID     string            `json:"source_suite_id"`
	SourceRunID       string            `json:"source_run_id"`
	ExpectedSignature MismatchSignature `json:"expected_signature"`
	RunID             string            `json:"run_id"`
	Confirmed         bool              `json:"confirmed"`
	ActualSignature   MismatchSignature `json:"actual_signature"`
	SignatureMatched  bool              `json:"signature_matched"`
	Reproduced        bool              `json:"reproduced"`
	ArtifactDir       string            `json:"artifact_dir"`
	RunArtifactDir    string            `json:"run_artifact_dir"`
	StartedAt         string            `json:"started_at"`
	FinishedAt        string            `json:"finished_at"`
}

// ReplayCorpusEntry turns a compact corpus handle back into an executable
// testcase. Reproduction currently means: the run confirms and emits the same
// mismatch signature as the corpus entry.
func ReplayCorpusEntry(ctx context.Context, opts ReplayOptions) (*ReplayResult, error) {
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	entry, err := ShowCorpusEntry(opts.CorpusDir, opts.EntryID)
	if err != nil {
		return nil, err
	}

	return replayEntry(ctx, *entry, opts)
}

func replayEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions) (*ReplayResult, error) {
	started := time.Now().UTC()
	replayID := fmt.Sprintf("replay-%d", started.UnixNano())
	replayDir := filepath.Join(opts.OutDir, replayID)
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		return nil, fmt.Errorf("create replay directory: %w", err)
	}

	runResult, err := Run(ctx, RunOptions{
		CaseName:       entry.CaseName,
		OutDir:         replayDir,
		Delay:          opts.Delay,
		MockURL:        opts.MockURL,
		EnvKind:        opts.EnvKind,
		ContainerImage: opts.ContainerImage,
		FaultPlanID:    firstNonEmpty(opts.FaultPlanID, entry.FaultPlanID),
	})
	if err != nil {
		return nil, err
	}

	signatureMatched := runResult.Signature.String() == entry.Signature.String()
	finished := time.Now().UTC()
	result := &ReplayResult{
		ReplayID:          replayID,
		EntryID:           entry.EntryID,
		CaseName:          entry.CaseName,
		Environment:       runResult.Environment,
		ContainerImage:    runResult.ContainerImage,
		FaultPlanID:       runResult.FaultPlanID,
		SourceSuiteID:     entry.SuiteID,
		SourceRunID:       entry.RunID,
		ExpectedSignature: entry.Signature,
		RunID:             runResult.RunID,
		Confirmed:         runResult.Confirmed,
		ActualSignature:   runResult.Signature,
		SignatureMatched:  signatureMatched,
		Reproduced:        runResult.Confirmed && signatureMatched,
		ArtifactDir:       replayDir,
		RunArtifactDir:    runResult.ArtifactDir,
		StartedAt:         started.Format(time.RFC3339Nano),
		FinishedAt:        finished.Format(time.RFC3339Nano),
	}
	if err := writeJSON(filepath.Join(replayDir, "replay-result.json"), result); err != nil {
		return nil, err
	}
	return result, nil
}
