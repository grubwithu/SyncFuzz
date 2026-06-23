package syncfuzz

import (
	"fmt"
	"path/filepath"
)

// CaseManifest describes the intent of a testcase run. result.json says what
// happened; manifest.json says what the testcase was designed to exercise.
type CaseManifest struct {
	RunID             string            `json:"run_id"`
	CaseName          string            `json:"case_name"`
	Environment       string            `json:"environment"`
	ContainerImage    string            `json:"container_image,omitempty"`
	Objective         string            `json:"objective"`
	StateClasses      []string          `json:"state_classes"`
	FaultPhases       []string          `json:"fault_phases"`
	Primitives        []string          `json:"primitives"`
	ExpectedSignature MismatchSignature `json:"expected_signature"`
	Artifacts         []string          `json:"artifacts"`
}

func writeManifest(run *runContext, manifest CaseManifest) error {
	manifest.RunID = run.runID
	manifest.CaseName = run.caseName
	manifest.Environment = run.environment
	manifest.ContainerImage = run.containerImage
	if err := writeJSON(filepath.Join(run.runDir, "manifest.json"), manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}
