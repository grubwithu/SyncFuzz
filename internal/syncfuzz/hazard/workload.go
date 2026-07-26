// Package hazard contains the V3 recovery-time dependency and hazard layer.
// It deliberately sits above recovery's static A/O relation package: a
// residual relation never becomes a realized hazard without typed use evidence.
package hazard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const WorkloadSchemaVersion = "syncfuzz.workload.v1"

// Workload is the frozen low-frequency normal task pair used across multiple
// EnvironmentProgram and RecoveryPlan choices. It carries no resource path,
// expected role, checkpoint, or finding predicate.
type Workload struct {
	SchemaVersion      string `json:"schema_version"`
	WorkloadID         string `json:"workload_id"`
	BaseProjectID      string `json:"base_project_id"`
	InitialPrompt      string `json:"initial_prompt"`
	ContinuationPrompt string `json:"continuation_prompt"`
	RunnerConstraints  string `json:"runner_constraints"`
}

type WorkloadOptions struct {
	BaseProjectID      string
	InitialPrompt      string
	ContinuationPrompt string
	RunnerConstraints  string
}

func NewWorkload(options WorkloadOptions) (Workload, error) {
	workload := Workload{
		SchemaVersion:      WorkloadSchemaVersion,
		BaseProjectID:      strings.TrimSpace(options.BaseProjectID),
		InitialPrompt:      strings.TrimSpace(options.InitialPrompt),
		ContinuationPrompt: strings.TrimSpace(options.ContinuationPrompt),
		RunnerConstraints:  strings.TrimSpace(options.RunnerConstraints),
	}
	workload.WorkloadID = workloadID(workload)
	if err := workload.Validate(); err != nil {
		return Workload{}, err
	}
	return workload, nil
}

func (w Workload) Validate() error {
	if w.SchemaVersion != WorkloadSchemaVersion || strings.TrimSpace(w.BaseProjectID) == "" || strings.TrimSpace(w.InitialPrompt) == "" || strings.TrimSpace(w.ContinuationPrompt) == "" || strings.TrimSpace(w.RunnerConstraints) == "" {
		return fmt.Errorf("workload is incomplete")
	}
	if w.WorkloadID != workloadID(w) {
		return fmt.Errorf("workload ID does not match frozen workload bytes")
	}
	return nil
}

func workloadID(workload Workload) string {
	identity := struct {
		BaseProjectID      string `json:"base_project_id"`
		InitialPrompt      string `json:"initial_prompt"`
		ContinuationPrompt string `json:"continuation_prompt"`
		RunnerConstraints  string `json:"runner_constraints"`
	}{
		BaseProjectID:      workload.BaseProjectID,
		InitialPrompt:      workload.InitialPrompt,
		ContinuationPrompt: workload.ContinuationPrompt,
		RunnerConstraints:  workload.RunnerConstraints,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return "workload:" + hex.EncodeToString(digest[:])
}
