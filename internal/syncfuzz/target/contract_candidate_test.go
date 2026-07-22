package target_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestValidateTargetContractCandidatesAcceptsExactSourceGroundedProposal(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	sourcePath := filepath.Join(sourceRoot, "contracts", "replay.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	const quote = "A replay from this checkpoint must not retain later PATH mutations."
	if err := os.WriteFile(sourcePath, []byte("# Replay\r\n"+quote+"\r\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	inputPath := filepath.Join(tmp, "candidates.json")
	writeTargetContractCandidateSet(t, inputPath, target.TargetContractCandidateSet{
		SchemaVersion: target.TargetContractCandidateSetSchemaVersion,
		Candidates: []target.TargetContractCandidate{{
			CandidateID:   "shell-path-replay-reset",
			TargetID:      "langgraph-shell-react",
			TaskID:        target.PersistentShellReplayTargetTaskID,
			StateSurface:  "shell-session.path",
			LifecycleEdge: "checkpoint->replay",
			Expectation:   target.TargetContractExpectationReset,
			SourceType:    target.TargetContractCandidateSourceDocumentedContract,
			Source: target.TargetContractCandidateSource{
				SourcePath: filepath.Join("contracts", "replay.md"),
				StartLine:  2,
				EndLine:    2,
				Quote:      quote,
			},
		}},
	})

	reportPath := filepath.Join(tmp, "reports", "target-contract-candidate-validation.json")
	report, err := target.ValidateTargetContractCandidates(target.TargetContractCandidateValidationOptions{
		InputPath:  inputPath,
		SourceRoot: sourceRoot,
		OutputPath: reportPath,
	})
	if err != nil {
		t.Fatalf("ValidateTargetContractCandidates failed: %v", err)
	}
	if report.SchemaVersion != target.TargetContractCandidateValidationSchemaVersion || report.Accepted != 1 || report.Unsupported != 0 || report.AutomaticProfileAdoption != target.TargetContractCandidateAutomaticAdoptionDisabled {
		t.Fatalf("unexpected validation report: %#v", report)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Status != target.TargetContractCandidateValidationAccepted || report.Candidates[0].Classification != target.TargetContractCandidateClassificationSourceGroundedProposal || report.Candidates[0].ResolvedSourcePath != sourcePath {
		t.Fatalf("unexpected candidate validation result: %#v", report.Candidates)
	}
	encoded, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read written validation report: %v", err)
	}
	var persisted target.TargetContractCandidateValidationReport
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("decode written validation report: %v", err)
	}
	if persisted.Accepted != 1 || persisted.Candidates[0].Classification != target.TargetContractCandidateClassificationSourceGroundedProposal {
		t.Fatalf("unexpected persisted validation report: %#v", persisted)
	}
}

func TestValidateTargetContractCandidatesMarksInvalidSpansUnsupported(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	sourcePath := filepath.Join(sourceRoot, "contracts", "replay.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	const quote = "A replay from this checkpoint must not retain later PATH mutations."
	if err := os.WriteFile(sourcePath, []byte(quote+"\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	inputPath := filepath.Join(tmp, "candidates.json")
	base := target.TargetContractCandidate{
		TargetID:      "langgraph-shell-react",
		TaskID:        target.PersistentShellReplayTargetTaskID,
		StateSurface:  "shell-session.path",
		LifecycleEdge: "checkpoint->replay",
		Expectation:   target.TargetContractExpectationReset,
		SourceType:    target.TargetContractCandidateSourceScenarioAssumption,
		Source: target.TargetContractCandidateSource{
			SourcePath: filepath.Join("contracts", "replay.md"),
			StartLine:  1,
			EndLine:    1,
			Quote:      quote,
		},
	}
	quoteMismatch := base
	quoteMismatch.CandidateID = "quote-mismatch"
	quoteMismatch.Source.Quote = "different quote"
	pathEscape := base
	pathEscape.CandidateID = "path-escape"
	pathEscape.Source.SourcePath = filepath.Join("..", "outside.md")
	badRange := base
	badRange.CandidateID = "bad-range"
	badRange.Source.StartLine = 0
	outsidePath := filepath.Join(tmp, "outside.md")
	if err := os.WriteFile(outsidePath, []byte(quote+"\n"), 0o644); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	symlinkPath := filepath.Join(sourceRoot, "contracts", "outside-link.md")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Fatalf("create source escape symlink: %v", err)
	}
	symlinkEscape := base
	symlinkEscape.CandidateID = "symlink-escape"
	symlinkEscape.Source.SourcePath = filepath.Join("contracts", "outside-link.md")
	writeTargetContractCandidateSet(t, inputPath, target.TargetContractCandidateSet{
		SchemaVersion: target.TargetContractCandidateSetSchemaVersion,
		Candidates:    []target.TargetContractCandidate{quoteMismatch, pathEscape, badRange, symlinkEscape},
	})

	report, err := target.ValidateTargetContractCandidates(target.TargetContractCandidateValidationOptions{
		InputPath:  inputPath,
		SourceRoot: sourceRoot,
		OutputPath: filepath.Join(tmp, "validation.json"),
	})
	if err != nil {
		t.Fatalf("ValidateTargetContractCandidates failed: %v", err)
	}
	if report.Accepted != 0 || report.Unsupported != 4 || len(report.Candidates) != 4 {
		t.Fatalf("unexpected invalid-candidate report: %#v", report)
	}
	for _, result := range report.Candidates {
		if result.Status != target.TargetContractCandidateValidationUnsupported || result.Classification != target.TargetContractCandidateClassificationUnsupported || len(result.Reasons) == 0 {
			t.Fatalf("expected unsupported candidate result: %#v", result)
		}
	}
}

func writeTargetContractCandidateSet(t *testing.T, path string, set target.TargetContractCandidateSet) {
	t.Helper()
	if err := core.WriteJSON(path, set); err != nil {
		t.Fatalf("write contract candidate set: %v", err)
	}
}
