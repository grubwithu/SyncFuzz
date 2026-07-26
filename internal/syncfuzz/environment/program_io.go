package environment

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadEnvironmentProgram imports an immutable, validated topology program.
// It deliberately rejects unknown JSON fields so an execution cannot acquire
// unreviewed target-side behavior through an older controller.
func ReadEnvironmentProgram(path string) (EnvironmentProgram, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return EnvironmentProgram{}, fmt.Errorf("environment program path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return EnvironmentProgram{}, fmt.Errorf("read environment program %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var program EnvironmentProgram
	if err := decoder.Decode(&program); err != nil {
		return EnvironmentProgram{}, fmt.Errorf("decode environment program %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return EnvironmentProgram{}, fmt.Errorf("decode environment program %s: trailing JSON value", path)
		}
		return EnvironmentProgram{}, fmt.Errorf("decode environment program %s: trailing data: %w", path, err)
	}
	if err := program.Validate(); err != nil {
		return EnvironmentProgram{}, fmt.Errorf("validate environment program %s: %w", path, err)
	}
	return program, nil
}

// WriteEnvironmentProgram persists an immutable topology program for a target
// adapter. The write is intentionally ordinary artifact output; callers that
// need non-overwrite semantics must create the destination first.
func WriteEnvironmentProgram(path string, program EnvironmentProgram) error {
	if err := program.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("environment program output path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create environment program directory: %w", err)
	}
	data, err := json.MarshalIndent(program, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal environment program: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write environment program %s: %w", path, err)
	}
	return nil
}
