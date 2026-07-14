package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
)

type TargetMinimizationBatchOptions struct {
	SourcePath string
	OutDir     string
}

type TargetMinimizationBatchResult struct {
	SchemaVersion       string                        `json:"schema_version"`
	BatchID             string                        `json:"batch_id"`
	SourcePath          string                        `json:"source_path"`
	SourceSchemaVersion string                        `json:"source_schema_version"`
	GeneratedAt         string                        `json:"generated_at"`
	ArtifactDir         string                        `json:"artifact_dir"`
	TotalResults        int                           `json:"total_results"`
	ApplicablePlans     int                           `json:"applicable_plans"`
	SkippedPlans        int                           `json:"skipped_plans"`
	Plans               []TargetMinimizationCandidate `json:"plans"`
}

type TargetMinimizationCandidate struct {
	CandidateID      string                           `json:"candidate_id,omitempty"`
	RunID            string                           `json:"run_id,omitempty"`
	TargetID         string                           `json:"target_id,omitempty"`
	TaskID           string                           `json:"task_id"`
	PromptProfileID  string                           `json:"prompt_profile_id,omitempty"`
	PromptVariantID  string                           `json:"prompt_variant_id,omitempty"`
	OutcomeCategory  corpus.TargetObservationCategory `json:"outcome_category,omitempty"`
	Attribution      string                           `json:"attribution,omitempty"`
	ArtifactDir      string                           `json:"artifact_dir,omitempty"`
	MinimizationPlan TargetMinimizationPlan           `json:"minimization_plan"`
}

const targetMinimizationBatchArtifact = "target-minimization-plan.json"

func BuildTargetMinimizationBatch(opts TargetMinimizationBatchOptions) (*TargetMinimizationBatchResult, error) {
	if opts.SourcePath == "" {
		return nil, fmt.Errorf("source path is required")
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	sourceSchema, results, err := loadTargetMinimizationSource(opts.SourcePath)
	if err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	batchID := fmt.Sprintf("target-minimize-%d", started.UnixNano())
	artifactDir := filepath.Join(opts.OutDir, batchID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target minimization batch directory: %w", err)
	}

	batch := &TargetMinimizationBatchResult{
		SchemaVersion:       "syncfuzz.target-minimization-batch.v1",
		BatchID:             batchID,
		SourcePath:          opts.SourcePath,
		SourceSchemaVersion: sourceSchema,
		GeneratedAt:         started.Format(time.RFC3339Nano),
		ArtifactDir:         artifactDir,
		TotalResults:        len(results),
		Plans:               make([]TargetMinimizationCandidate, 0, len(results)),
	}
	for _, item := range results {
		plan := targetMinimizationPlanForResult(item)
		if plan.Applicable {
			batch.ApplicablePlans++
		} else {
			batch.SkippedPlans++
		}
		batch.Plans = append(batch.Plans, TargetMinimizationCandidate{
			CandidateID:      item.CandidateID,
			RunID:            item.RunID,
			TargetID:         item.TargetID,
			TaskID:           item.TaskID,
			PromptProfileID:  item.PromptProfileID,
			PromptVariantID:  item.PromptVariantID,
			OutcomeCategory:  item.OutcomeCategory,
			Attribution:      item.TargetOracle.Attribution,
			ArtifactDir:      item.ArtifactDir,
			MinimizationPlan: plan,
		})
	}
	if err := core.WriteJSON(filepath.Join(artifactDir, targetMinimizationBatchArtifact), batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func loadTargetMinimizationSource(path string) (string, []TargetSuiteRunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read target minimization source %s: %w", path, err)
	}
	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return "", nil, fmt.Errorf("decode target minimization source header %s: %w", path, err)
	}
	switch header.SchemaVersion {
	case "syncfuzz.target-suite-result.v1":
		var result TargetSuiteResult
		if err := json.Unmarshal(data, &result); err != nil {
			return "", nil, fmt.Errorf("decode target suite result %s: %w", path, err)
		}
		return header.SchemaVersion, result.Results, nil
	case "syncfuzz.target-matrix-result.v1":
		var result TargetMatrixResult
		if err := json.Unmarshal(data, &result); err != nil {
			return "", nil, fmt.Errorf("decode target matrix result %s: %w", path, err)
		}
		return header.SchemaVersion, result.Results, nil
	default:
		return "", nil, fmt.Errorf("unsupported target minimization source schema %q", header.SchemaVersion)
	}
}

func targetMinimizationPlanForResult(item TargetSuiteRunResult) TargetMinimizationPlan {
	if item.MinimizationPlan != nil {
		return *item.MinimizationPlan
	}
	observation := corpus.TargetObservationDetails{
		Category:          item.OutcomeCategory,
		Reason:            item.OutcomeReason,
		ActivationReached: item.ActivationStage == TargetActivationStageActivationReached,
	}
	return *buildTargetMinimizationPlan(TargetScheduleCandidate{
		CandidateID:          item.CandidateID,
		TargetID:             item.TargetID,
		TaskID:               item.TaskID,
		PromptProfileID:      item.PromptProfileID,
		PromptVariantID:      item.PromptVariantID,
		ScenarioID:           item.ScenarioID,
		SeedID:               item.SeedID,
		LifecycleOperationID: item.LifecycleOperationID,
		PlantPrimitiveID:     item.PlantPrimitiveID,
		ActivationKindID:     item.ActivationKindID,
		OracleKindID:         item.OracleKindID,
		MutationFocusID:      item.MutationFocusID,
		MutationFocusKind:    item.MutationFocusKind,
		Mutations:            item.Mutations,
	}, item, observation)
}
