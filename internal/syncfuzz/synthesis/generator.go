package synthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxGeneratorResponseBytes = 64 << 10

// Generate invokes a user-selected task generator. The command receives the
// request JSON path in SYNCFUZZ_SYNTHESIS_REQUEST and must emit exactly one
// GeneratorResponse JSON object on stdout. Its command line is intentionally
// not persisted in the candidate artifact, because it may contain credentials
// or deployment-local model configuration.
func Generate(ctx context.Context, command string, request GeneratorRequest, generatorID string) (SynthesisCandidate, error) {
	if err := request.Validate(); err != nil {
		return SynthesisCandidate{}, err
	}
	if strings.TrimSpace(command) == "" {
		return SynthesisCandidate{}, fmt.Errorf("synthesis generator command is required")
	}
	if info, err := os.Stat(request.ScaffoldArtifact); err != nil {
		return SynthesisCandidate{}, fmt.Errorf("read synthesis scaffold %s: %w", request.ScaffoldArtifact, err)
	} else if info.IsDir() {
		return SynthesisCandidate{}, fmt.Errorf("synthesis scaffold %s must be an artifact file", request.ScaffoldArtifact)
	}
	dir, err := os.MkdirTemp("", "syncfuzz-synthesis-request-")
	if err != nil {
		return SynthesisCandidate{}, fmt.Errorf("create synthesis request directory: %w", err)
	}
	defer os.RemoveAll(dir)
	requestPath := filepath.Join(dir, "request.json")
	if err := writeJSON(requestPath, request); err != nil {
		return SynthesisCandidate{}, err
	}
	process := exec.CommandContext(ctx, "sh", "-c", command)
	process.Env = append(os.Environ(), "SYNCFUZZ_SYNTHESIS_REQUEST="+requestPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		return SynthesisCandidate{}, fmt.Errorf("run synthesis generator: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 || stdout.Len() > maxGeneratorResponseBytes {
		return SynthesisCandidate{}, fmt.Errorf("synthesis generator response must contain 1..%d bytes, got %d", maxGeneratorResponseBytes, stdout.Len())
	}
	var response GeneratorResponse
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&response); err != nil {
		return SynthesisCandidate{}, fmt.Errorf("decode synthesis generator response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return SynthesisCandidate{}, fmt.Errorf("synthesis generator emitted multiple JSON values")
		}
		return SynthesisCandidate{}, fmt.Errorf("synthesis generator emitted trailing non-JSON output: %w", err)
	}
	return NewCandidate(request, generatorID, response)
}
