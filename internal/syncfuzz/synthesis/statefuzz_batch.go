package synthesis

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const StateFuzzBatchReportSchema = "syncfuzz.statefuzz-batch-report.v1"

type StateFuzzBatchEntryStatus string

const (
	StateFuzzBatchAccepted               StateFuzzBatchEntryStatus = "accepted"
	StateFuzzBatchRejectedEvaluation     StateFuzzBatchEntryStatus = "rejected-evaluation"
	StateFuzzBatchRejectedSourceBaseline StateFuzzBatchEntryStatus = "rejected-source-baseline"
	StateFuzzBatchExecutionFailed        StateFuzzBatchEntryStatus = "execution-failed"
	StateFuzzBatchInvalidArtifactRoot    StateFuzzBatchEntryStatus = "invalid-artifact-root"
)

// StateFuzzBatchEntry is one artifact-root audit result. Invalid roots remain
// in the denominator so a previous provider run cannot be hidden by mixing or
// replacing files in an attempt directory.
type StateFuzzBatchEntry struct {
	ArtifactRoot    string                    `json:"artifact_root"`
	Attempt         int                       `json:"attempt"`
	Legacy          bool                      `json:"legacy"`
	Status          StateFuzzBatchEntryStatus `json:"status"`
	Reason          string                    `json:"reason,omitempty"`
	CandidateID     string                    `json:"candidate_id,omitempty"`
	ProfileRunID    string                    `json:"profile_run_id,omitempty"`
	SeedID          string                    `json:"seed_id,omitempty"`
	RecoveryOutcome string                    `json:"recovery_outcome,omitempty"`
}

// StateFuzzBatchReport aggregates generated-candidate attempts without
// turning rejected or structurally invalid roots into positive recovery data.
type StateFuzzBatchReport struct {
	SchemaVersion               string                `json:"schema_version"`
	ObjectiveID                 string                `json:"objective_id"`
	BatchRoot                   string                `json:"batch_root"`
	AttemptCount                int                   `json:"attempt_count"`
	AcceptedCount               int                   `json:"accepted_count"`
	RejectedEvaluationCount     int                   `json:"rejected_evaluation_count"`
	RejectedSourceBaselineCount int                   `json:"rejected_source_baseline_count"`
	ExecutionFailureCount       int                   `json:"execution_failure_count"`
	InvalidArtifactRootCount    int                   `json:"invalid_artifact_root_count"`
	RecoveryOutcomeCounts       map[string]int        `json:"recovery_outcome_counts"`
	Attempts                    []StateFuzzBatchEntry `json:"attempts"`
}

// BuildStateFuzzBatchReport scans attempt-* roots and validates every link
// from candidate through evaluation, StateSeed, and recovery execution. Early
// execution failures may carry only their recorded StateFuzzAttempt status.
func BuildStateFuzzBatchReport(stateObjective objective.StateObjective, root string) (StateFuzzBatchReport, error) {
	if err := stateObjective.Validate(); err != nil {
		return StateFuzzBatchReport{}, err
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return StateFuzzBatchReport{}, fmt.Errorf("StateFuzz batch root is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return StateFuzzBatchReport{}, fmt.Errorf("read StateFuzz batch root %s: %w", root, err)
	}
	report := StateFuzzBatchReport{
		SchemaVersion:         StateFuzzBatchReportSchema,
		ObjectiveID:           stateObjective.ObjectiveID,
		BatchRoot:             root,
		RecoveryOutcomeCounts: make(map[string]int),
		Attempts:              make([]StateFuzzBatchEntry, 0),
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "attempt-") {
			continue
		}
		report.Attempts = append(report.Attempts, auditStateFuzzRoot(stateObjective, filepath.Join(root, entry.Name())))
	}
	sort.Slice(report.Attempts, func(left, right int) bool {
		if report.Attempts[left].Attempt == report.Attempts[right].Attempt {
			return report.Attempts[left].ArtifactRoot < report.Attempts[right].ArtifactRoot
		}
		return report.Attempts[left].Attempt < report.Attempts[right].Attempt
	})
	for _, attempt := range report.Attempts {
		report.AttemptCount++
		switch attempt.Status {
		case StateFuzzBatchAccepted:
			report.AcceptedCount++
			report.RecoveryOutcomeCounts[attempt.RecoveryOutcome]++
		case StateFuzzBatchRejectedEvaluation:
			report.RejectedEvaluationCount++
		case StateFuzzBatchRejectedSourceBaseline:
			report.RejectedSourceBaselineCount++
		case StateFuzzBatchExecutionFailed:
			report.ExecutionFailureCount++
		case StateFuzzBatchInvalidArtifactRoot:
			report.InvalidArtifactRootCount++
		}
	}
	if err := report.Validate(); err != nil {
		return StateFuzzBatchReport{}, err
	}
	return report, nil
}

func (r StateFuzzBatchReport) Validate() error {
	if r.SchemaVersion != StateFuzzBatchReportSchema || strings.TrimSpace(r.ObjectiveID) == "" || strings.TrimSpace(r.BatchRoot) == "" || r.RecoveryOutcomeCounts == nil {
		return fmt.Errorf("StateFuzz batch report is incomplete")
	}
	accepted := 0
	rejectedEvaluation := 0
	rejectedSourceBaseline := 0
	executionFailures := 0
	invalidRoots := 0
	outcomes := make(map[string]int)
	for _, attempt := range r.Attempts {
		if strings.TrimSpace(attempt.ArtifactRoot) == "" || attempt.Attempt < -1 {
			return fmt.Errorf("StateFuzz batch attempt is incomplete")
		}
		switch attempt.Status {
		case StateFuzzBatchAccepted:
			if strings.TrimSpace(attempt.RecoveryOutcome) == "" || strings.TrimSpace(attempt.Reason) != "" {
				return fmt.Errorf("accepted StateFuzz batch attempt %q lacks a clean recovery outcome", attempt.ArtifactRoot)
			}
			accepted++
			outcomes[attempt.RecoveryOutcome]++
		case StateFuzzBatchRejectedEvaluation:
			if strings.TrimSpace(attempt.Reason) == "" {
				return fmt.Errorf("evaluation-rejected StateFuzz batch attempt %q lacks a reason", attempt.ArtifactRoot)
			}
			rejectedEvaluation++
		case StateFuzzBatchRejectedSourceBaseline:
			if strings.TrimSpace(attempt.Reason) == "" {
				return fmt.Errorf("source-baseline-rejected StateFuzz batch attempt %q lacks a reason", attempt.ArtifactRoot)
			}
			rejectedSourceBaseline++
		case StateFuzzBatchExecutionFailed:
			if strings.TrimSpace(attempt.Reason) == "" {
				return fmt.Errorf("failed StateFuzz batch attempt %q lacks a reason", attempt.ArtifactRoot)
			}
			executionFailures++
		case StateFuzzBatchInvalidArtifactRoot:
			if strings.TrimSpace(attempt.Reason) == "" {
				return fmt.Errorf("invalid StateFuzz artifact root %q lacks a reason", attempt.ArtifactRoot)
			}
			invalidRoots++
		default:
			return fmt.Errorf("unsupported StateFuzz batch attempt status %q", attempt.Status)
		}
	}
	if r.AttemptCount != len(r.Attempts) || r.AcceptedCount != accepted || r.RejectedEvaluationCount != rejectedEvaluation || r.RejectedSourceBaselineCount != rejectedSourceBaseline || r.ExecutionFailureCount != executionFailures || r.InvalidArtifactRootCount != invalidRoots || !reflect.DeepEqual(r.RecoveryOutcomeCounts, outcomes) {
		return fmt.Errorf("StateFuzz batch report aggregates do not match attempt entries")
	}
	return nil
}

func auditStateFuzzRoot(stateObjective objective.StateObjective, root string) StateFuzzBatchEntry {
	entry := StateFuzzBatchEntry{ArtifactRoot: root, Attempt: -1}
	candidate, err := ReadCandidate(filepath.Join(root, "candidate.json"))
	if err != nil || candidate.ValidateFor(stateObjective) != nil {
		return invalidStateFuzzRoot(entry, "missing-or-invalid-candidate")
	}
	entry.Attempt = candidate.Attempt
	entry.CandidateID = candidate.CandidateID

	metadataPath := filepath.Join(root, "statefuzz-attempt.json")
	metadata, hasMetadata, err := readStateFuzzAttemptIfPresent(metadataPath)
	if err != nil {
		return invalidStateFuzzRoot(entry, "missing-or-invalid-attempt-record")
	}
	entry.Legacy = !hasMetadata
	if hasMetadata {
		if !sameArtifactRoot(metadata.ArtifactRoot, root) || metadata.Attempt != candidate.Attempt || metadata.CandidateID != candidate.CandidateID {
			return invalidStateFuzzRoot(entry, "attempt-record-does-not-match-candidate-root")
		}
		if metadata.Status == StateFuzzAttemptExecutionFailed {
			return auditRecordedExecutionFailure(stateObjective, root, entry, candidate, metadata)
		}
	}

	evaluation, err := ReadEvaluation(filepath.Join(root, "evaluation.json"))
	if err != nil || evaluation.ValidateFor(stateObjective) != nil || evaluation.CandidateID != candidate.CandidateID {
		return invalidStateFuzzRoot(entry, "missing-or-invalid-evaluation")
	}
	entry.ProfileRunID = evaluation.ProfileRunID
	if hasMetadata && (metadata.ProfileRunID != evaluation.ProfileRunID || metadata.EligibleForRetention == nil || *metadata.EligibleForRetention != evaluation.EligibleForRetention) {
		return invalidStateFuzzRoot(entry, "attempt-record-does-not-match-evaluation")
	}
	profile, err := objective.ReadProfileRun(filepath.Join(root, "profile-run.json"))
	if err != nil || profile.ValidateFor(stateObjective) != nil || profile.ProfileRunID != evaluation.ProfileRunID || profile.SynthesisCandidateID != candidate.CandidateID {
		return invalidStateFuzzRoot(entry, "candidate-evaluation-profile-lineage-mismatch")
	}
	expectedEvaluation, err := EvaluateProfile(stateObjective, candidate, profile)
	if err != nil || !equalCandidateEvaluations(evaluation, expectedEvaluation) {
		return invalidStateFuzzRoot(entry, "evaluation-is-not-derived-from-profile")
	}

	if hasMetadata {
		switch metadata.Status {
		case StateFuzzAttemptRejectedEvaluation:
			if evaluation.EligibleForRetention || stateFuzzRecoveryArtifactsPresent(root) {
				return invalidStateFuzzRoot(entry, "evaluation-rejection-has-inconsistent-recovery-artifacts")
			}
			entry.Status = StateFuzzBatchRejectedEvaluation
			entry.Reason = metadata.Reason
			return entry
		case StateFuzzAttemptRejectedSourceBaseline:
			if !evaluation.EligibleForRetention || stateFuzzRecoveryArtifactsPresent(root) {
				return invalidStateFuzzRoot(entry, "source-baseline-rejection-has-inconsistent-recovery-artifacts")
			}
			entry.Status = StateFuzzBatchRejectedSourceBaseline
			entry.Reason = metadata.Reason
			return entry
		case StateFuzzAttemptAccepted:
			if !evaluation.EligibleForRetention {
				return invalidStateFuzzRoot(entry, "accepted-attempt-has-ineligible-evaluation")
			}
		}
	} else if !evaluation.EligibleForRetention {
		entry.Status = StateFuzzBatchRejectedEvaluation
		entry.Reason = "legacy-retention-ineligible"
		return entry
	}

	return auditAcceptedStateFuzzRoot(stateObjective, root, entry, candidate, profile)
}

func auditRecordedExecutionFailure(stateObjective objective.StateObjective, root string, entry StateFuzzBatchEntry, candidate SynthesisCandidate, metadata StateFuzzAttempt) StateFuzzBatchEntry {
	evaluationPath := filepath.Join(root, "evaluation.json")
	if _, err := os.Stat(evaluationPath); err == nil {
		evaluation, err := ReadEvaluation(evaluationPath)
		if err != nil {
			return invalidStateFuzzRoot(entry, "execution-failure-has-invalid-evaluation")
		}
		if evaluation.ValidateFor(stateObjective) != nil || evaluation.CandidateID != entry.CandidateID {
			return invalidStateFuzzRoot(entry, "execution-failure-has-invalid-evaluation")
		}
		entry.ProfileRunID = evaluation.ProfileRunID
		if metadata.ProfileRunID != "" && metadata.ProfileRunID != evaluation.ProfileRunID {
			return invalidStateFuzzRoot(entry, "execution-failure-record-does-not-match-evaluation")
		}
		if metadata.EligibleForRetention != nil && *metadata.EligibleForRetention != evaluation.EligibleForRetention {
			return invalidStateFuzzRoot(entry, "execution-failure-record-does-not-match-evaluation")
		}
		profile, err := objective.ReadProfileRun(filepath.Join(root, "profile-run.json"))
		if err != nil || profile.ValidateFor(stateObjective) != nil || profile.ProfileRunID != evaluation.ProfileRunID || profile.SynthesisCandidateID != candidate.CandidateID {
			return invalidStateFuzzRoot(entry, "execution-failure-has-invalid-profile-lineage")
		}
		expectedEvaluation, err := EvaluateProfile(stateObjective, candidate, profile)
		if err != nil || !equalCandidateEvaluations(evaluation, expectedEvaluation) {
			return invalidStateFuzzRoot(entry, "execution-failure-evaluation-is-not-derived-from-profile")
		}
	} else if !os.IsNotExist(err) {
		return invalidStateFuzzRoot(entry, "execution-failure-has-unreadable-evaluation")
	} else if metadata.ProfileRunID != "" || metadata.EligibleForRetention != nil {
		return invalidStateFuzzRoot(entry, "execution-failure-record-has-no-evaluation")
	}
	if stateFuzzRecoveryArtifactsPresent(root) {
		return invalidStateFuzzRoot(entry, "execution-failure-has-unexpected-recovery-artifacts")
	}
	entry.Status = StateFuzzBatchExecutionFailed
	entry.Reason = metadata.Reason
	return entry
}

func auditAcceptedStateFuzzRoot(stateObjective objective.StateObjective, root string, entry StateFuzzBatchEntry, candidate SynthesisCandidate, profile objective.ProfileRun) StateFuzzBatchEntry {
	seed, err := objective.ReadStateSeed(filepath.Join(root, "state-seed.json"))
	if err != nil || seed.ValidateFor(stateObjective) != nil {
		return invalidStateFuzzRoot(entry, "missing-or-invalid-state-seed")
	}
	entry.SeedID = seed.SeedID
	if seed.ProfileRunID != profile.ProfileRunID || seed.SynthesisCandidateID != candidate.CandidateID || seed.TargetID != candidate.TargetID || seed.AdapterID != candidate.AdapterID || seed.RecordedPlanID != profile.RecordedPlanID {
		return invalidStateFuzzRoot(entry, "candidate-profile-seed-lineage-mismatch")
	}
	execution, err := recovery.ReadForkRecoverySetExecution(filepath.Join(root, "recovery-set-execution.json"))
	if err != nil || strings.TrimSpace(execution.RecoverySetID) == "" || execution.SeedID != seed.SeedID || execution.FrontierID != seed.FrontierID || execution.RecordedPlanID != seed.RecordedPlanID || execution.MaterializationHead.ValidateFor(seed) != nil || strings.TrimSpace(execution.Classification.Outcome) == "" {
		return invalidStateFuzzRoot(entry, "seed-recovery-lineage-mismatch")
	}
	entry.Status = StateFuzzBatchAccepted
	entry.RecoveryOutcome = execution.Classification.Outcome
	return entry
}

func readStateFuzzAttemptIfPresent(path string) (StateFuzzAttempt, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return StateFuzzAttempt{}, false, nil
		}
		return StateFuzzAttempt{}, false, err
	}
	attempt, err := ReadStateFuzzAttempt(path)
	if err != nil {
		return StateFuzzAttempt{}, false, err
	}
	return attempt, true, nil
}

func stateFuzzRecoveryArtifactsPresent(root string) bool {
	for _, name := range []string{"state-seed.json", "recovery-set-execution.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func sameArtifactRoot(left string, right string) bool {
	leftPath, leftErr := filepath.Abs(filepath.Clean(left))
	rightPath, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftPath == rightPath
}

func equalCandidateEvaluations(left CandidateEvaluation, right CandidateEvaluation) bool {
	if len(left.ValidatedFrontiers) == 0 {
		left.ValidatedFrontiers = nil
	}
	if len(right.ValidatedFrontiers) == 0 {
		right.ValidatedFrontiers = nil
	}
	if len(left.ObservedEffects) == 0 {
		left.ObservedEffects = nil
	}
	if len(right.ObservedEffects) == 0 {
		right.ObservedEffects = nil
	}
	if len(left.MissingEffects) == 0 {
		left.MissingEffects = nil
	}
	if len(right.MissingEffects) == 0 {
		right.MissingEffects = nil
	}
	if len(left.Feedback) == 0 {
		left.Feedback = nil
	}
	if len(right.Feedback) == 0 {
		right.Feedback = nil
	}
	return reflect.DeepEqual(left, right)
}

func invalidStateFuzzRoot(entry StateFuzzBatchEntry, reason string) StateFuzzBatchEntry {
	entry.Status = StateFuzzBatchInvalidArtifactRoot
	entry.Reason = reason
	entry.RecoveryOutcome = ""
	return entry
}
