package core

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

func WriteManifest(run *RunContext, manifest CaseManifest) error {
	manifest.RunID = run.RunID
	manifest.CaseName = run.CaseName
	manifest.Environment = run.Environment
	manifest.ContainerImage = run.ContainerImage
	if err := WriteJSON(filepath.Join(run.RunDir, "manifest.json"), manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}
