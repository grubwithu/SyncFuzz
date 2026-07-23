package profiling

import (
	"fmt"
	"strings"
)

// CheckpointRecorder records controller observation boundaries in the same
// CLOCK_MONOTONIC domain as bpf_ktime_get_ns. These are deliberately not
// claims about an agent's own durable checkpoint API; adapters with such an
// API will later emit additional semantic checkpoints into the same catalog.
type CheckpointRecorder struct {
	runID       string
	checkpoints []Checkpoint
}

func NewCheckpointRecorder(runID string) (*CheckpointRecorder, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("create checkpoint recorder: run_id is required")
	}
	return &CheckpointRecorder{runID: runID}, nil
}

func (r *CheckpointRecorder) Mark(checkpointID string, logicalPhase string) (Checkpoint, error) {
	if r == nil {
		return Checkpoint{}, fmt.Errorf("mark checkpoint: recorder is nil")
	}
	if strings.TrimSpace(checkpointID) == "" {
		return Checkpoint{}, fmt.Errorf("mark checkpoint: checkpoint_id is required")
	}
	for _, checkpoint := range r.checkpoints {
		if checkpoint.CheckpointID == checkpointID {
			return Checkpoint{}, fmt.Errorf("mark checkpoint: duplicate checkpoint_id %q", checkpointID)
		}
	}
	monotonicNS, err := monotonicNowNS()
	if err != nil {
		return Checkpoint{}, fmt.Errorf("mark checkpoint %q: %w", checkpointID, err)
	}
	if len(r.checkpoints) > 0 && monotonicNS <= r.checkpoints[len(r.checkpoints)-1].MonotonicNS {
		monotonicNS = r.checkpoints[len(r.checkpoints)-1].MonotonicNS + 1
	}
	checkpoint := Checkpoint{
		CheckpointID: checkpointID,
		MonotonicNS:  monotonicNS,
		LogicalPhase: strings.TrimSpace(logicalPhase),
	}
	r.checkpoints = append(r.checkpoints, checkpoint)
	return checkpoint, nil
}

func (r *CheckpointRecorder) Catalog() CheckpointCatalog {
	if r == nil {
		return CheckpointCatalog{SchemaVersion: SchemaVersion}
	}
	return CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         r.runID,
		Checkpoints:   append([]Checkpoint{}, r.checkpoints...),
	}
}
