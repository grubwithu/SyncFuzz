package target

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestRunTargetContractProposalGeneratorWritesBoundedRequestAndValidation(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	sourcePath := filepath.Join(sourceRoot, "contracts", "replay.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	const quote = "A replay from this checkpoint must not retain later PATH mutations."
	if err := os.WriteFile(sourcePath, []byte("# Replay\n"+quote+"\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	writeTargetContractProposalGeneratorFixture(t, sourceRoot, TargetContractCandidateSet{
		SchemaVersion: TargetContractCandidateSetSchemaVersion,
		Generator:     "test-generator",
		Candidates: []TargetContractCandidate{{
			CandidateID:   "shell-path-replay-reset",
			TargetID:      "langgraph-shell-react",
			TaskID:        PersistentShellReplayTargetTaskID,
			StateSurface:  "shell-session.path",
			LifecycleEdge: "checkpoint->replay",
			Expectation:   TargetContractExpectationReset,
			SourceType:    TargetContractCandidateSourceDocumentedContract,
			Source: TargetContractCandidateSource{
				SourcePath: "contracts/replay.md",
				StartLine:  2,
				EndLine:    2,
				Quote:      quote,
			},
		}},
	})

	result, err := RunTargetContractProposalGenerator(context.Background(), TargetContractProposalOptions{
		RunID:            "proposal-run",
		TargetID:         "langgraph-shell-react",
		TaskIDs:          []string{PersistentShellReplayTargetTaskID},
		SourceRoot:       sourceRoot,
		SourcePaths:      []string{"contracts/replay.md"},
		GeneratorCommand: "./generator.sh",
		OutDir:           filepath.Join(tmp, "runs"),
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunTargetContractProposalGenerator failed: %v", err)
	}
	if result.SchemaVersion != TargetContractProposalRunSchemaVersion || result.Accepted != 1 || result.Unsupported != 0 || result.AutomaticProfileAdoption != TargetContractCandidateAutomaticAdoptionDisabled {
		t.Fatalf("unexpected proposal result: %#v", result)
	}
	if len(result.GeneratorCommandSHA256) != 64 {
		t.Fatalf("expected generator command digest, got %#v", result)
	}
	requestPath := filepath.Join(result.ArtifactDir, result.RequestArtifact)
	rawRequest, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("read proposal request: %v", err)
	}
	var request TargetContractProposalRequest
	if err := json.Unmarshal(rawRequest, &request); err != nil {
		t.Fatalf("decode proposal request: %v", err)
	}
	if request.SchemaVersion != TargetContractProposalRequestSchemaVersion || request.OutputSchema != TargetContractCandidateSetSchemaVersion || len(request.Tasks) != 1 || request.Tasks[0].TaskID != PersistentShellReplayTargetTaskID || len(request.Sources) != 1 || request.Sources[0].SourcePath != "contracts/replay.md" || request.Sources[0].Content != "# Replay\n"+quote+"\n" {
		t.Fatalf("unexpected bounded proposal request: %#v", request)
	}
	validationRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, result.ValidationReportArtifact))
	if err != nil {
		t.Fatalf("read proposal validation report: %v", err)
	}
	var validation TargetContractCandidateValidationReport
	if err := json.Unmarshal(validationRaw, &validation); err != nil {
		t.Fatalf("decode proposal validation report: %v", err)
	}
	if len(validation.AllowedSourcePaths) != 1 || validation.AllowedSourcePaths[0] != "contracts/replay.md" || validation.Accepted != 1 {
		t.Fatalf("proposal validation did not retain source bundle boundary: %#v", validation)
	}
}

func TestRunTargetContractProposalGeneratorRejectsUnrequestedSourceClaims(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	for path, content := range map[string]string{
		"contracts/replay.md": "# Replay\nIncluded source claim.\n",
		"contracts/hidden.md": "# Hidden\nUnrequested source claim.\n",
	} {
		fullPath := filepath.Join(sourceRoot, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create source directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write source %s: %v", path, err)
		}
	}
	writeTargetContractProposalGeneratorFixture(t, sourceRoot, TargetContractCandidateSet{
		SchemaVersion: TargetContractCandidateSetSchemaVersion,
		Candidates: []TargetContractCandidate{{
			CandidateID:   "hidden-source-claim",
			TargetID:      "different-target",
			TaskID:        DefaultTargetTaskID,
			StateSurface:  "shell-session.path",
			LifecycleEdge: "checkpoint->replay",
			Expectation:   TargetContractExpectationReset,
			SourceType:    TargetContractCandidateSourceDocumentedContract,
			Source: TargetContractCandidateSource{
				SourcePath: "contracts/hidden.md",
				StartLine:  2,
				EndLine:    2,
				Quote:      "Unrequested source claim.",
			},
		}},
	})
	result, err := RunTargetContractProposalGenerator(context.Background(), TargetContractProposalOptions{
		TargetID:         "langgraph-shell-react",
		TaskIDs:          []string{PersistentShellReplayTargetTaskID},
		SourceRoot:       sourceRoot,
		SourcePaths:      []string{"contracts/replay.md"},
		GeneratorCommand: "./generator.sh",
		OutDir:           filepath.Join(tmp, "runs"),
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunTargetContractProposalGenerator failed: %v", err)
	}
	if result.Accepted != 0 || result.Unsupported != 1 {
		t.Fatalf("expected unrequested source claim to be unsupported: %#v", result)
	}
	validationRaw, err := os.ReadFile(filepath.Join(result.ArtifactDir, result.ValidationReportArtifact))
	if err != nil {
		t.Fatalf("read proposal validation report: %v", err)
	}
	var validation TargetContractCandidateValidationReport
	if err := json.Unmarshal(validationRaw, &validation); err != nil {
		t.Fatalf("decode proposal validation report: %v", err)
	}
	if len(validation.Candidates) != 1 || !strings.Contains(strings.Join(validation.Candidates[0].Reasons, "\n"), "expected_target_id") || !strings.Contains(strings.Join(validation.Candidates[0].Reasons, "\n"), "allowed_task_ids") || !strings.Contains(strings.Join(validation.Candidates[0].Reasons, "\n"), "allowed_source_paths") {
		t.Fatalf("expected proposal scope rejections, got %#v", validation)
	}
}

func TestRunTargetContractProposalGeneratorReportsSafeFailureCategory(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("create source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "replay.md"), []byte("# Replay\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	const script = "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' '{\"schema_version\":\"syncfuzz.target-contract-proposal-failure.v1\",\"category\":\"provider-transport\"}' > \"$SYNCFUZZ_CONTRACT_PROPOSAL_FAILURE\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(sourceRoot, "generator.sh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write failing generator: %v", err)
	}

	_, err := RunTargetContractProposalGenerator(context.Background(), TargetContractProposalOptions{
		TargetID:         "langgraph-shell-react",
		TaskIDs:          []string{PersistentShellReplayTargetTaskID},
		SourceRoot:       sourceRoot,
		SourcePaths:      []string{"replay.md"},
		GeneratorCommand: "./generator.sh",
		OutDir:           filepath.Join(tmp, "runs"),
		Timeout:          5 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "provider-transport") {
		t.Fatalf("expected safe provider-transport failure category, got %v", err)
	}
}

func writeTargetContractProposalGeneratorFixture(t *testing.T, sourceRoot string, candidates TargetContractCandidateSet) {
	t.Helper()
	fixturePath := filepath.Join(sourceRoot, "generator-candidates.json")
	if err := core.WriteJSON(fixturePath, candidates); err != nil {
		t.Fatalf("write proposal generator fixture: %v", err)
	}
	scriptPath := filepath.Join(sourceRoot, "generator.sh")
	const script = "#!/usr/bin/env bash\nset -euo pipefail\ntest -f \"$SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST\"\ntest \"$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA\" = \"syncfuzz.target-contract-candidates.v1\"\ntest \"$SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY\" = \"proposal-only\"\ncp generator-candidates.json \"$SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write proposal generator script: %v", err)
	}
}
