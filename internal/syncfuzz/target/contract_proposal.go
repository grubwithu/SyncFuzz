package target

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	TargetContractProposalRequestSchemaVersion = "syncfuzz.target-contract-proposal-request.v1"
	TargetContractProposalRunSchemaVersion     = "syncfuzz.target-contract-proposal-run.v1"
	TargetContractProposalRequestArtifact      = "target-contract-proposal-request.json"
	TargetContractProposalCandidateArtifact    = "target-contract-candidates.json"
	TargetContractProposalResultArtifact       = "target-contract-proposal-run.json"

	targetContractProposalMaxSourceBytes = 256 * 1024
)

// TargetContractProposalOptions runs a user-selected proposal generator over
// an explicit, bounded source bundle. The command is caller-supplied; the
// resulting candidate set is proposal-only and is never loaded as a
// TargetContractProfile or oracle input by this pipeline.
type TargetContractProposalOptions struct {
	RunID            string
	TargetID         string
	TaskIDs          []string
	SourceRoot       string
	SourcePaths      []string
	GeneratorCommand string
	OutDir           string
	Timeout          time.Duration
}

// TargetContractProposalRequest is the fixed input boundary for an LLM or
// other structured proposal generator. Every included source is selected by
// the caller, path-confined to SourceRoot, text-only, and content-addressed.
type TargetContractProposalRequest struct {
	SchemaVersion            string                              `json:"schema_version"`
	GeneratedAt              string                              `json:"generated_at"`
	TargetID                 string                              `json:"target_id"`
	Tasks                    []TargetContractProposalTaskContext `json:"tasks"`
	SourceRoot               string                              `json:"source_root"`
	Sources                  []TargetContractProposalSource      `json:"sources"`
	OutputSchema             string                              `json:"output_schema"`
	AutomaticProfileAdoption string                              `json:"automatic_profile_adoption"`
	Instructions             []string                            `json:"instructions"`
}

// TargetContractProposalTaskContext exposes only the Scenario IR contract
// dimensions relevant to a proposal, not a mutable profile or oracle verdict.
type TargetContractProposalTaskContext struct {
	TaskID             string                   `json:"task_id"`
	ScenarioID         string                   `json:"scenario_id,omitempty"`
	QueryID            string                   `json:"query_id,omitempty"`
	RootQueryID        string                   `json:"root_query_id,omitempty"`
	StateSurface       string                   `json:"state_surface,omitempty"`
	LifecycleEdge      string                   `json:"lifecycle_edge,omitempty"`
	ViolationSignature TargetViolationSignature `json:"violation_signature"`
}

type TargetContractProposalSource struct {
	SourcePath string `json:"source_path"`
	SHA256     string `json:"sha256"`
	Content    string `json:"content"`
}

// TargetContractProposalRunResult records a completed generator invocation
// and deterministic source-grounding validation. The command text itself is
// deliberately not persisted because it can contain provider credentials.
type TargetContractProposalRunResult struct {
	SchemaVersion            string `json:"schema_version"`
	RunID                    string `json:"run_id"`
	StartedAt                string `json:"started_at"`
	FinishedAt               string `json:"finished_at"`
	ArtifactDir              string `json:"artifact_dir"`
	TargetID                 string `json:"target_id"`
	SourceRoot               string `json:"source_root"`
	GeneratorCommandSHA256   string `json:"generator_command_sha256"`
	RequestArtifact          string `json:"request_artifact"`
	CandidateSetArtifact     string `json:"candidate_set_artifact"`
	ValidationReportArtifact string `json:"validation_report_artifact"`
	Accepted                 int    `json:"accepted"`
	Unsupported              int    `json:"unsupported"`
	AutomaticProfileAdoption string `json:"automatic_profile_adoption"`
}

// RunTargetContractProposalGenerator creates the fixed request artifact,
// invokes the explicitly supplied generator command, and validates its JSON
// output with the existing source-grounding gate. It never calls a provider by
// itself or adopts a generated candidate into a profile.
func RunTargetContractProposalGenerator(ctx context.Context, opts TargetContractProposalOptions) (*TargetContractProposalRunResult, error) {
	if strings.TrimSpace(opts.TargetID) == "" {
		return nil, fmt.Errorf("contract proposal target id is required")
	}
	if strings.TrimSpace(opts.GeneratorCommand) == "" {
		return nil, fmt.Errorf("contract proposal generator command is required")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("contract proposal output directory is required")
	}
	sourceRoot, err := targetContractCandidateSourceRoot(opts.SourceRoot)
	if err != nil {
		return nil, err
	}
	tasks, err := targetContractProposalTaskContexts(opts.TaskIDs)
	if err != nil {
		return nil, err
	}
	sources, err := targetContractProposalSources(sourceRoot, opts.SourcePaths)
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		runID = fmt.Sprintf("target-contract-proposal-%d", started.UnixNano())
	}
	if !isTargetContractProposalRunID(runID) {
		return nil, fmt.Errorf("invalid contract proposal run id %q", runID)
	}
	artifactDir, err := filepath.Abs(filepath.Join(opts.OutDir, runID))
	if err != nil {
		return nil, fmt.Errorf("resolve contract proposal artifact directory: %w", err)
	}
	if _, err := os.Stat(artifactDir); err == nil {
		return nil, fmt.Errorf("contract proposal artifact directory %s already exists", artifactDir)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat contract proposal artifact directory %s: %w", artifactDir, err)
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create contract proposal artifact directory: %w", err)
	}
	request := TargetContractProposalRequest{
		SchemaVersion:            TargetContractProposalRequestSchemaVersion,
		GeneratedAt:              started.Format(time.RFC3339Nano),
		TargetID:                 strings.TrimSpace(opts.TargetID),
		Tasks:                    tasks,
		SourceRoot:               sourceRoot,
		Sources:                  sources,
		OutputSchema:             TargetContractCandidateSetSchemaVersion,
		AutomaticProfileAdoption: TargetContractCandidateAutomaticAdoptionDisabled,
		Instructions: []string{
			"Return exactly one JSON object using the declared output_schema.",
			"Every candidate must cite one supplied source_path and an exact inclusive line span with a quote matching that span after CRLF normalization.",
			"Candidates are proposals only: do not claim an oracle verdict, mutate a contract profile, or request automatic profile adoption.",
			"Use only the supplied task contexts and source bundle; omit unsupported claims instead of inventing a source span.",
		},
	}
	requestPath := filepath.Join(artifactDir, TargetContractProposalRequestArtifact)
	if err := core.WriteJSON(requestPath, request); err != nil {
		return nil, fmt.Errorf("write contract proposal request: %w", err)
	}
	candidatePath := filepath.Join(artifactDir, TargetContractProposalCandidateArtifact)
	if err := runTargetContractProposalCommand(ctx, opts.GeneratorCommand, sourceRoot, requestPath, candidatePath, opts.Timeout); err != nil {
		return nil, err
	}
	validationPath := filepath.Join(artifactDir, "target-contract-candidate-validation.json")
	validation, err := ValidateTargetContractCandidates(TargetContractCandidateValidationOptions{
		InputPath:          candidatePath,
		SourceRoot:         sourceRoot,
		ExpectedTargetID:   request.TargetID,
		AllowedTaskIDs:     targetContractProposalTaskIDs(tasks),
		AllowedSourcePaths: targetContractProposalSourcePaths(sources),
		OutputPath:         validationPath,
	})
	if err != nil {
		return nil, fmt.Errorf("validate generated contract candidates: %w", err)
	}
	commandHash := sha256.Sum256([]byte(opts.GeneratorCommand))
	result := &TargetContractProposalRunResult{
		SchemaVersion:            TargetContractProposalRunSchemaVersion,
		RunID:                    runID,
		StartedAt:                started.Format(time.RFC3339Nano),
		FinishedAt:               time.Now().UTC().Format(time.RFC3339Nano),
		ArtifactDir:              artifactDir,
		TargetID:                 request.TargetID,
		SourceRoot:               sourceRoot,
		GeneratorCommandSHA256:   fmt.Sprintf("%x", commandHash[:]),
		RequestArtifact:          TargetContractProposalRequestArtifact,
		CandidateSetArtifact:     TargetContractProposalCandidateArtifact,
		ValidationReportArtifact: filepath.Base(validationPath),
		Accepted:                 validation.Accepted,
		Unsupported:              validation.Unsupported,
		AutomaticProfileAdoption: TargetContractCandidateAutomaticAdoptionDisabled,
	}
	if err := core.WriteJSON(filepath.Join(artifactDir, TargetContractProposalResultArtifact), result); err != nil {
		return nil, fmt.Errorf("write contract proposal run result: %w", err)
	}
	return result, nil
}

func targetContractProposalSourcePaths(sources []TargetContractProposalSource) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.SourcePath)
	}
	return paths
}

func targetContractProposalTaskIDs(tasks []TargetContractProposalTaskContext) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.TaskID)
	}
	return ids
}

func targetContractProposalTaskContexts(taskIDs []string) ([]TargetContractProposalTaskContext, error) {
	unique := make(map[string]struct{}, len(taskIDs))
	normalized := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		if _, exists := unique[taskID]; exists {
			continue
		}
		unique[taskID] = struct{}{}
		normalized = append(normalized, taskID)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one known target task is required for contract proposal generation")
	}
	sort.Strings(normalized)
	contexts := make([]TargetContractProposalTaskContext, 0, len(normalized))
	for _, taskID := range normalized {
		task, ok := TargetTaskByID(taskID)
		if !ok {
			return nil, fmt.Errorf("unknown target task %q for contract proposal generation", taskID)
		}
		contexts = append(contexts, TargetContractProposalTaskContext{
			TaskID:             task.TaskID,
			ScenarioID:         task.ScenarioID,
			QueryID:            task.QueryID,
			RootQueryID:        task.RootQueryID,
			StateSurface:       task.StateSurface,
			LifecycleEdge:      task.LifecycleEdge,
			ViolationSignature: task.ViolationSignature,
		})
	}
	return contexts, nil
}

func targetContractProposalSources(sourceRoot string, sourcePaths []string) ([]TargetContractProposalSource, error) {
	unique := make(map[string]struct{}, len(sourcePaths))
	normalized := make([]string, 0, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		if filepath.IsAbs(sourcePath) {
			return nil, fmt.Errorf("contract proposal source path %q must be relative to source root", sourcePath)
		}
		clean := filepath.Clean(sourcePath)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("contract proposal source path %q escapes source root", sourcePath)
		}
		clean = filepath.ToSlash(clean)
		if _, exists := unique[clean]; exists {
			continue
		}
		unique[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one source path is required for contract proposal generation")
	}
	sort.Strings(normalized)
	sources := make([]TargetContractProposalSource, 0, len(normalized))
	for _, sourcePath := range normalized {
		resolved, err := filepath.EvalSymlinks(filepath.Join(sourceRoot, sourcePath))
		if err != nil {
			return nil, fmt.Errorf("resolve contract proposal source %s: %w", sourcePath, err)
		}
		resolved = filepath.Clean(resolved)
		if !targetContractCandidatePathWithin(sourceRoot, resolved) {
			return nil, fmt.Errorf("contract proposal source %s resolves outside source root", sourcePath)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("stat contract proposal source %s: %w", sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("contract proposal source %s is not a regular file", sourcePath)
		}
		if info.Size() > targetContractProposalMaxSourceBytes {
			return nil, fmt.Errorf("contract proposal source %s exceeds %d byte limit", sourcePath, targetContractProposalMaxSourceBytes)
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("read contract proposal source %s: %w", sourcePath, err)
		}
		if !utf8.Valid(content) {
			return nil, fmt.Errorf("contract proposal source %s is not valid UTF-8 text", sourcePath)
		}
		digest := sha256.Sum256(content)
		sources = append(sources, TargetContractProposalSource{
			SourcePath: sourcePath,
			SHA256:     fmt.Sprintf("%x", digest[:]),
			Content:    normalizeTargetContractCandidateLineEndings(string(content)),
		})
	}
	return sources, nil
}

func runTargetContractProposalCommand(ctx context.Context, command string, sourceRoot string, requestPath string, candidatePath string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandContext, "bash", "-lc", command)
	cmd.Dir = sourceRoot
	cmd.Env = append(os.Environ(),
		"SYNCFUZZ_CONTRACT_PROPOSAL_REQUEST="+requestPath,
		"SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT="+candidatePath,
		"SYNCFUZZ_CONTRACT_PROPOSAL_OUTPUT_SCHEMA="+TargetContractCandidateSetSchemaVersion,
		"SYNCFUZZ_CONTRACT_PROPOSAL_AUTHORITY=proposal-only",
	)
	if _, err := cmd.CombinedOutput(); err != nil {
		if commandContext.Err() == context.DeadlineExceeded {
			return fmt.Errorf("contract proposal generator timed out after %s", timeout)
		}
		return fmt.Errorf("contract proposal generator command failed: %w", err)
	}
	info, err := os.Stat(candidatePath)
	if err != nil {
		return fmt.Errorf("contract proposal generator did not write %s: %w", TargetContractProposalCandidateArtifact, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("contract proposal generator output %s is not a regular file", candidatePath)
	}
	return nil
}

func isTargetContractProposalRunID(value string) bool {
	if value == "" || filepath.IsAbs(value) || filepath.Base(value) != value {
		return false
	}
	for _, runeValue := range value {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= 'A' && runeValue <= 'Z') || (runeValue >= '0' && runeValue <= '9') || runeValue == '-' || runeValue == '_' {
			continue
		}
		return false
	}
	return true
}
