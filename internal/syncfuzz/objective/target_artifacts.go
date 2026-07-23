package objective

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const (
	targetResultArtifact  = "target-result.json"
	targetTaskArtifact    = "target-task.json"
	checkpointMapArtifact = "checkpoint-effect-map.json"
)

// ImportTargetProfileRun converts a completed real target profiling artifact
// into the V2 ProfileRun IR. kind is supplied by the future synthesis
// scheduler: importing an arbitrary target run must not silently grant it
// StateSeed eligibility. synthesisCandidateID is required only for a run that
// the scheduler issued; it binds provenance without consuming legacy target
// scenario or mutation metadata. The import consumes only adapter identity, completion, the
// recorded task artifact, and checkpoint evidence; it never reads legacy
// scenario, mutation, prompt-variant, or genealogy fields.
func ImportTargetProfileRun(runDir string, objectiveID string, kind ProfileRunKind, synthesisCandidateID string) (ProfileRun, error) {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" || strings.TrimSpace(objectiveID) == "" {
		return ProfileRun{}, fmt.Errorf("target profile import requires run directory and objective ID")
	}
	if !kind.Valid() {
		return ProfileRun{}, fmt.Errorf("target profile import requires a valid profile run kind")
	}
	if kind == ProfileRunKindSynthesisCandidate && strings.TrimSpace(synthesisCandidateID) == "" {
		return ProfileRun{}, fmt.Errorf("synthesis target profile import requires a scheduler-issued synthesis candidate ID")
	}
	if kind == ProfileRunKindCalibrationFixture && strings.TrimSpace(synthesisCandidateID) != "" {
		return ProfileRun{}, fmt.Errorf("calibration target profile import must not receive a synthesis candidate ID")
	}
	resultPath := filepath.Join(runDir, targetResultArtifact)
	var result struct {
		RunID             string `json:"run_id"`
		AdapterID         string `json:"adapter_id"`
		TargetID          string `json:"target_id"`
		Completed         bool   `json:"completed"`
		ProfilingAnalysis *struct {
			HotFrontiers int `json:"hot_frontiers"`
		} `json:"profiling_analysis"`
	}
	if err := readJSON(resultPath, &result); err != nil {
		return ProfileRun{}, err
	}
	if !result.Completed {
		return ProfileRun{}, fmt.Errorf("target run %q did not complete", result.RunID)
	}
	if result.ProfilingAnalysis == nil {
		return ProfileRun{}, fmt.Errorf("target run %q has no profiling analysis; rerun with profiling enabled", result.RunID)
	}
	if strings.TrimSpace(result.RunID) == "" || strings.TrimSpace(result.AdapterID) == "" || strings.TrimSpace(result.TargetID) == "" {
		return ProfileRun{}, fmt.Errorf("target result %s lacks run, adapter, or target identity", resultPath)
	}
	checkpointMap, err := profiling.ReadCheckpointEffectMap(filepath.Join(runDir, checkpointMapArtifact))
	if err != nil {
		return ProfileRun{}, fmt.Errorf("read target checkpoint map: %w", err)
	}
	if checkpointMap.RunID != result.RunID {
		return ProfileRun{}, fmt.Errorf("target checkpoint map run %q does not match target result %q", checkpointMap.RunID, result.RunID)
	}
	planArtifact := filepath.Join(runDir, targetTaskArtifact)
	if _, err := os.Stat(planArtifact); err != nil {
		return ProfileRun{}, fmt.Errorf("recorded target plan artifact: %w", err)
	}
	var task struct {
		AdapterID            string `json:"adapter_id"`
		TargetID             string `json:"target_id"`
		SynthesisCandidateID string `json:"synthesis_candidate_id"`
	}
	if err := readJSON(planArtifact, &task); err != nil {
		return ProfileRun{}, fmt.Errorf("read recorded target plan: %w", err)
	}
	if task.AdapterID != result.AdapterID || task.TargetID != result.TargetID {
		return ProfileRun{}, fmt.Errorf("recorded target plan identity does not match target result %q", result.RunID)
	}
	if kind == ProfileRunKindSynthesisCandidate && task.SynthesisCandidateID != synthesisCandidateID {
		return ProfileRun{}, fmt.Errorf("target run %q was not executed for synthesis candidate %q", result.RunID, synthesisCandidateID)
	}
	if kind == ProfileRunKindCalibrationFixture && strings.TrimSpace(task.SynthesisCandidateID) != "" {
		return ProfileRun{}, fmt.Errorf("calibration target profile import rejects synthesis-tagged target run %q", result.RunID)
	}
	return ProfileRun{
		SchemaVersion:        SchemaVersion,
		ProfileRunID:         "target-profile:" + result.RunID,
		Kind:                 kind,
		SynthesisCandidateID: synthesisCandidateID,
		ObjectiveID:          objectiveID,
		TargetID:             result.TargetID,
		AdapterID:            result.AdapterID,
		RecordedPlanID:       "recorded-plan:target-run:" + result.RunID,
		RecordedPlanArtifact: planArtifact,
		CheckpointMap:        checkpointMap,
	}, nil
}
