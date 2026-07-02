package syncfuzz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ReplayOptions struct {
	CorpusDir       string
	EntryID         string
	OutDir          string
	Delay           time.Duration
	MockURL         string
	EnvKind         string
	ContainerImage  string
	FaultPlanID     string
	PrimitiveID     string
	TimingProfileID string
}

type ReplayResult struct {
	ExecutionKind     string            `json:"execution_kind"`
	ReplayID          string            `json:"replay_id"`
	EntryID           string            `json:"entry_id"`
	CaseName          string            `json:"case_name"`
	AdapterID         string            `json:"adapter_id,omitempty"`
	TargetID          string            `json:"target_id,omitempty"`
	TaskID            string            `json:"task_id,omitempty"`
	Environment       string            `json:"environment"`
	ContainerImage    string            `json:"container_image,omitempty"`
	FaultPlanID       string            `json:"fault_plan_id,omitempty"`
	PrimitiveID       string            `json:"primitive_id,omitempty"`
	TimingProfileID   string            `json:"timing_profile_id,omitempty"`
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

	switch entry.EffectiveExecutionKind() {
	case corpusExecutionTarget:
		return replayTargetEntry(ctx, entry, opts, replayID, replayDir, started)
	default:
		return replayCaseEntry(ctx, entry, opts, replayID, replayDir, started)
	}
}

func replayCaseEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions, replayID string, replayDir string, started time.Time) (*ReplayResult, error) {
	runResult, err := Run(ctx, RunOptions{
		CaseName:        entry.CaseName,
		OutDir:          replayDir,
		Delay:           opts.Delay,
		MockURL:         opts.MockURL,
		EnvKind:         opts.EnvKind,
		ContainerImage:  opts.ContainerImage,
		FaultPlanID:     firstNonEmpty(opts.FaultPlanID, entry.FaultPlanID),
		PrimitiveID:     firstNonEmpty(opts.PrimitiveID, entry.PrimitiveID),
		TimingProfileID: firstNonEmpty(opts.TimingProfileID, entry.TimingProfileID),
	})
	if err != nil {
		return nil, err
	}

	signatureMatched := runResult.Signature.String() == entry.Signature.String()
	finished := time.Now().UTC()
	result := &ReplayResult{
		ExecutionKind:     entry.EffectiveExecutionKind(),
		ReplayID:          replayID,
		EntryID:           entry.EntryID,
		CaseName:          entry.CaseName,
		Environment:       runResult.Environment,
		ContainerImage:    runResult.ContainerImage,
		FaultPlanID:       runResult.FaultPlanID,
		PrimitiveID:       runResult.PrimitiveID,
		TimingProfileID:   runResult.TimingProfileID,
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

func replayTargetEntry(ctx context.Context, entry CorpusEntry, opts ReplayOptions, replayID string, replayDir string, started time.Time) (*ReplayResult, error) {
	task, err := loadTargetTask(filepath.Join(entry.ArtifactDir, targetTaskArtifact))
	if err != nil {
		return nil, err
	}

	runResult, err := RunTarget(ctx, TargetRunOptions{
		AdapterID:        task.AdapterID,
		TargetID:         task.TargetID,
		TaskID:           task.TaskID,
		Objective:        task.Objective,
		Prompt:           task.Prompt,
		Command:          task.Command,
		OutDir:           replayDir,
		Timeout:          time.Duration(task.TimeoutMillis) * time.Millisecond,
		ObserveDelay:     time.Duration(task.ObserveDelayMs) * time.Millisecond,
		LateObserveDelay: time.Duration(task.LateObserveDelayMs) * time.Millisecond,
		EnvKind:          firstNonEmpty(opts.EnvKind, task.Environment),
		ContainerImage:   firstNonEmpty(opts.ContainerImage, task.ContainerImage),
		ExpectedFiles:    append([]string{}, task.ExpectedFiles...),
	})
	if err != nil {
		return nil, err
	}

	signatureMatched := runResult.Signature.String() == entry.Signature.String()
	finished := time.Now().UTC()
	result := &ReplayResult{
		ExecutionKind:     entry.EffectiveExecutionKind(),
		ReplayID:          replayID,
		EntryID:           entry.EntryID,
		CaseName:          entry.Subject(),
		AdapterID:         runResult.AdapterID,
		TargetID:          runResult.TargetID,
		TaskID:            runResult.TaskID,
		Environment:       runResult.Environment,
		ContainerImage:    runResult.ContainerImage,
		SourceSuiteID:     entry.SuiteID,
		SourceRunID:       entry.RunID,
		ExpectedSignature: entry.Signature,
		RunID:             runResult.RunID,
		Confirmed:         runResult.ExpectationsMet,
		ActualSignature:   runResult.Signature,
		SignatureMatched:  signatureMatched,
		Reproduced:        runResult.ExpectationsMet && signatureMatched,
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

func loadTargetTask(path string) (TargetTask, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TargetTask{}, fmt.Errorf("read target task %q: %w", path, err)
	}
	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return TargetTask{}, fmt.Errorf("decode target task %q: %w", path, err)
	}
	return task, nil
}
