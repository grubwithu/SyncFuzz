package target

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	TargetContractCandidateSetSchemaVersion        = "syncfuzz.target-contract-candidates.v1"
	TargetContractCandidateValidationSchemaVersion = "syncfuzz.target-contract-candidate-validation.v1"
)

// TargetContractCandidateSourceType describes the claim being proposed. All
// source types require a checked local source span; this type affects review,
// not automatic contract adoption or oracle behavior.
type TargetContractCandidateSourceType string

const (
	TargetContractCandidateSourceDocumentedContract     TargetContractCandidateSourceType = "documented-contract"
	TargetContractCandidateSourceDerivedSafetyInvariant TargetContractCandidateSourceType = "derived-safety-invariant"
	TargetContractCandidateSourceScenarioAssumption     TargetContractCandidateSourceType = "scenario-assumption"
)

type TargetContractCandidateValidationStatus string

const (
	TargetContractCandidateValidationAccepted    TargetContractCandidateValidationStatus = "accepted"
	TargetContractCandidateValidationUnsupported TargetContractCandidateValidationStatus = "unsupported"
)

const (
	TargetContractCandidateClassificationSourceGroundedProposal = "source-grounded-proposal"
	TargetContractCandidateClassificationUnsupported            = "unsupported"
	TargetContractCandidateAutomaticAdoptionDisabled            = "disabled"
)

// TargetContractCandidateSet is an input boundary for a human or LLM to
// propose a contract rule. It intentionally is not a TargetContractProfile:
// valid candidates remain proposals until a reviewer separately turns them
// into a maintained profile and tests that profile.
type TargetContractCandidateSet struct {
	SchemaVersion string                    `json:"schema_version"`
	Generator     string                    `json:"generator,omitempty"`
	Description   string                    `json:"description,omitempty"`
	Candidates    []TargetContractCandidate `json:"candidates"`
}

type TargetContractCandidate struct {
	CandidateID    string                            `json:"candidate_id"`
	TargetID       string                            `json:"target_id"`
	TaskID         string                            `json:"task_id"`
	ScenarioID     string                            `json:"scenario_id,omitempty"`
	ProposedRuleID string                            `json:"proposed_rule_id,omitempty"`
	StateSurface   string                            `json:"state_surface"`
	LifecycleEdge  string                            `json:"lifecycle_edge"`
	Expectation    TargetContractExpectation         `json:"expectation"`
	SourceType     TargetContractCandidateSourceType `json:"source_type"`
	Source         TargetContractCandidateSource     `json:"source"`
	Rationale      string                            `json:"rationale,omitempty"`
}

// TargetContractCandidateSource pins a proposal to exact lines in a local
// source tree. The quote is compared byte-for-byte after CRLF normalization.
type TargetContractCandidateSource struct {
	SourcePath string `json:"source_path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Quote      string `json:"quote"`
}

type TargetContractCandidateValidationOptions struct {
	InputPath  string
	SourceRoot string
	OutputPath string
}

// TargetContractCandidateValidationReport is a deterministic source-grounding
// check. Its accepted results are source-grounded proposals only; this report
// never loads, changes, or selects a TargetContractProfile.
type TargetContractCandidateValidationReport struct {
	SchemaVersion            string                                    `json:"schema_version"`
	GeneratedAt              string                                    `json:"generated_at"`
	CandidateSetPath         string                                    `json:"candidate_set_path"`
	SourceRoot               string                                    `json:"source_root"`
	AutomaticProfileAdoption string                                    `json:"automatic_profile_adoption"`
	Accepted                 int                                       `json:"accepted"`
	Unsupported              int                                       `json:"unsupported"`
	Candidates               []TargetContractCandidateValidationResult `json:"candidates"`
}

type TargetContractCandidateValidationResult struct {
	Candidate          TargetContractCandidate                 `json:"candidate"`
	Status             TargetContractCandidateValidationStatus `json:"status"`
	Classification     string                                  `json:"classification"`
	ResolvedSourcePath string                                  `json:"resolved_source_path,omitempty"`
	Reasons            []string                                `json:"reasons,omitempty"`
}

// ValidateTargetContractCandidates accepts only proposals whose declared
// source span resolves inside SourceRoot and exactly matches the source quote.
// Per-candidate defects are recorded as unsupported results so a generated
// candidate set remains auditable; malformed set-level JSON/schema is an error.
func ValidateTargetContractCandidates(opts TargetContractCandidateValidationOptions) (*TargetContractCandidateValidationReport, error) {
	inputPath := strings.TrimSpace(opts.InputPath)
	if inputPath == "" {
		return nil, fmt.Errorf("contract candidate input path is required")
	}
	outputPath := strings.TrimSpace(opts.OutputPath)
	if outputPath == "" {
		return nil, fmt.Errorf("contract candidate validation output path is required")
	}
	sourceRoot, err := targetContractCandidateSourceRoot(opts.SourceRoot)
	if err != nil {
		return nil, err
	}
	inputPath, err = filepath.Abs(inputPath)
	if err != nil {
		return nil, fmt.Errorf("resolve contract candidate input path: %w", err)
	}
	set, err := readTargetPairJSON[TargetContractCandidateSet](inputPath)
	if err != nil {
		return nil, fmt.Errorf("read contract candidate set %s: %w", inputPath, err)
	}
	if set.SchemaVersion != TargetContractCandidateSetSchemaVersion {
		return nil, fmt.Errorf("contract candidate set %s has unsupported schema %q", inputPath, set.SchemaVersion)
	}
	if len(set.Candidates) == 0 {
		return nil, fmt.Errorf("contract candidate set %s has no candidates", inputPath)
	}

	counts := make(map[string]int, len(set.Candidates))
	for _, candidate := range set.Candidates {
		candidateID := strings.TrimSpace(candidate.CandidateID)
		if candidateID != "" {
			counts[candidateID]++
		}
	}
	report := &TargetContractCandidateValidationReport{
		SchemaVersion:            TargetContractCandidateValidationSchemaVersion,
		GeneratedAt:              time.Now().UTC().Format(time.RFC3339Nano),
		CandidateSetPath:         inputPath,
		SourceRoot:               sourceRoot,
		AutomaticProfileAdoption: TargetContractCandidateAutomaticAdoptionDisabled,
		Candidates:               make([]TargetContractCandidateValidationResult, 0, len(set.Candidates)),
	}
	for _, original := range set.Candidates {
		candidate := normalizeTargetContractCandidate(original)
		result := TargetContractCandidateValidationResult{Candidate: candidate}
		reasons := validateTargetContractCandidateShape(candidate, counts)
		resolvedSource, sourceReasons := validateTargetContractCandidateSource(sourceRoot, candidate.Source)
		if resolvedSource != "" {
			result.ResolvedSourcePath = resolvedSource
		}
		reasons = append(reasons, sourceReasons...)
		if len(reasons) == 0 {
			result.Status = TargetContractCandidateValidationAccepted
			result.Classification = TargetContractCandidateClassificationSourceGroundedProposal
			report.Accepted++
		} else {
			result.Status = TargetContractCandidateValidationUnsupported
			result.Classification = TargetContractCandidateClassificationUnsupported
			result.Reasons = reasons
			report.Unsupported++
		}
		report.Candidates = append(report.Candidates, result)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("create contract candidate validation output directory: %w", err)
	}
	if err := core.WriteJSON(outputPath, report); err != nil {
		return nil, fmt.Errorf("write contract candidate validation report %s: %w", outputPath, err)
	}
	return report, nil
}

func normalizeTargetContractCandidate(candidate TargetContractCandidate) TargetContractCandidate {
	candidate.CandidateID = strings.TrimSpace(candidate.CandidateID)
	candidate.TargetID = strings.TrimSpace(candidate.TargetID)
	candidate.TaskID = strings.TrimSpace(candidate.TaskID)
	candidate.ScenarioID = strings.TrimSpace(candidate.ScenarioID)
	candidate.ProposedRuleID = strings.TrimSpace(candidate.ProposedRuleID)
	candidate.StateSurface = strings.TrimSpace(candidate.StateSurface)
	candidate.LifecycleEdge = strings.TrimSpace(candidate.LifecycleEdge)
	candidate.Expectation = TargetContractExpectation(strings.TrimSpace(string(candidate.Expectation)))
	candidate.SourceType = TargetContractCandidateSourceType(strings.TrimSpace(string(candidate.SourceType)))
	candidate.Source.SourcePath = strings.TrimSpace(candidate.Source.SourcePath)
	candidate.Source.Quote = normalizeTargetContractCandidateLineEndings(candidate.Source.Quote)
	return candidate
}

func validateTargetContractCandidateShape(candidate TargetContractCandidate, counts map[string]int) []string {
	reasons := make([]string, 0, 8)
	if candidate.CandidateID == "" {
		reasons = append(reasons, "candidate_id is required")
	} else if counts[candidate.CandidateID] > 1 {
		reasons = append(reasons, "candidate_id is duplicated")
	}
	if candidate.TargetID == "" {
		reasons = append(reasons, "target_id is required")
	}
	if candidate.TaskID == "" {
		reasons = append(reasons, "task_id is required")
	}
	if candidate.StateSurface == "" {
		reasons = append(reasons, "state_surface is required")
	}
	if candidate.LifecycleEdge == "" {
		reasons = append(reasons, "lifecycle_edge is required")
	}
	if !isTargetContractCandidateExpectation(candidate.Expectation) {
		reasons = append(reasons, fmt.Sprintf("unsupported expectation %q", candidate.Expectation))
	}
	if !isTargetContractCandidateSourceType(candidate.SourceType) {
		reasons = append(reasons, fmt.Sprintf("unsupported source_type %q", candidate.SourceType))
	}
	return reasons
}

func isTargetContractCandidateExpectation(expectation TargetContractExpectation) bool {
	switch expectation {
	case TargetContractExpectationPreserve, TargetContractExpectationReset, TargetContractExpectationUnspecified:
		return true
	default:
		return false
	}
}

func isTargetContractCandidateSourceType(sourceType TargetContractCandidateSourceType) bool {
	switch sourceType {
	case TargetContractCandidateSourceDocumentedContract, TargetContractCandidateSourceDerivedSafetyInvariant, TargetContractCandidateSourceScenarioAssumption:
		return true
	default:
		return false
	}
}

func targetContractCandidateSourceRoot(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("contract candidate source root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve contract candidate source root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve contract candidate source root %s: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat contract candidate source root %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("contract candidate source root %s is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func validateTargetContractCandidateSource(sourceRoot string, source TargetContractCandidateSource) (string, []string) {
	reasons := make([]string, 0, 5)
	sourcePath := strings.TrimSpace(source.SourcePath)
	if sourcePath == "" {
		return "", append(reasons, "source.source_path is required")
	}
	if filepath.IsAbs(sourcePath) {
		return "", append(reasons, "source.source_path must be relative to source_root")
	}
	cleanSourcePath := filepath.Clean(sourcePath)
	if cleanSourcePath == "." || cleanSourcePath == ".." || strings.HasPrefix(cleanSourcePath, ".."+string(filepath.Separator)) {
		return "", append(reasons, "source.source_path escapes source_root")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(sourceRoot, cleanSourcePath))
	if err != nil {
		return "", append(reasons, fmt.Sprintf("resolve source.source_path: %v", err))
	}
	resolved = filepath.Clean(resolved)
	if !targetContractCandidatePathWithin(sourceRoot, resolved) {
		return "", append(reasons, "source.source_path resolves outside source_root")
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return resolved, append(reasons, fmt.Sprintf("read source.source_path: %v", err))
	}
	if source.StartLine < 1 || source.EndLine < source.StartLine {
		return resolved, append(reasons, "source line range must be positive and ordered")
	}
	lines := strings.Split(normalizeTargetContractCandidateLineEndings(string(content)), "\n")
	if source.EndLine > len(lines) {
		return resolved, append(reasons, fmt.Sprintf("source line range %d-%d exceeds file length %d", source.StartLine, source.EndLine, len(lines)))
	}
	quote := strings.Join(lines[source.StartLine-1:source.EndLine], "\n")
	if strings.TrimSpace(quote) == "" {
		return resolved, append(reasons, "source line range contains no non-whitespace content")
	}
	if normalizeTargetContractCandidateLineEndings(source.Quote) != quote {
		return resolved, append(reasons, "source quote does not exactly match the declared source line range")
	}
	return resolved, reasons
}

func targetContractCandidatePathWithin(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func normalizeTargetContractCandidateLineEndings(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
