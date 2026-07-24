package synthesis

import (
	"fmt"
	"strings"
)

const StateFuzzAttemptSchema = "syncfuzz.statefuzz-attempt.v1"

type StateFuzzAttemptStatus string

const (
	StateFuzzAttemptAccepted               StateFuzzAttemptStatus = "accepted"
	StateFuzzAttemptRejectedEvaluation     StateFuzzAttemptStatus = "rejected-evaluation"
	StateFuzzAttemptRejectedSourceBaseline StateFuzzAttemptStatus = "rejected-source-baseline"
	StateFuzzAttemptExecutionFailed        StateFuzzAttemptStatus = "execution-failed"
)

// StateFuzzAttempt records the denominator for one generated-candidate trial.
// A rejection is a valid result: it must not be presented as a recovered
// residual or silently disappear from a later generated-candidate aggregate.
type StateFuzzAttempt struct {
	SchemaVersion        string                 `json:"schema_version"`
	Attempt              int                    `json:"attempt"`
	ArtifactRoot         string                 `json:"artifact_root"`
	CandidateID          string                 `json:"candidate_id"`
	ProfileRunID         string                 `json:"profile_run_id"`
	EligibleForRetention *bool                  `json:"eligible_for_retention,omitempty"`
	Status               StateFuzzAttemptStatus `json:"status"`
	Reason               string                 `json:"reason,omitempty"`
}

func (a StateFuzzAttempt) Validate() error {
	if a.SchemaVersion != StateFuzzAttemptSchema || a.Attempt < 0 || strings.TrimSpace(a.ArtifactRoot) == "" || strings.TrimSpace(a.CandidateID) == "" {
		return fmt.Errorf("StateFuzz attempt requires schema, non-negative attempt, artifact root, and candidate ID")
	}
	if !a.Status.Valid() {
		return fmt.Errorf("unsupported StateFuzz attempt status %q", a.Status)
	}
	switch a.Status {
	case StateFuzzAttemptAccepted:
		if strings.TrimSpace(a.ProfileRunID) == "" || a.EligibleForRetention == nil || !*a.EligibleForRetention || strings.TrimSpace(a.Reason) != "" {
			return fmt.Errorf("accepted StateFuzz attempt requires an eligible profile run and no rejection reason")
		}
	case StateFuzzAttemptRejectedEvaluation:
		if strings.TrimSpace(a.ProfileRunID) == "" || a.EligibleForRetention == nil || *a.EligibleForRetention || strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("evaluation-rejected StateFuzz attempt requires an ineligible profile run and reason")
		}
	case StateFuzzAttemptRejectedSourceBaseline:
		if strings.TrimSpace(a.ProfileRunID) == "" || a.EligibleForRetention == nil || !*a.EligibleForRetention || strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("source-baseline-rejected StateFuzz attempt requires an eligible profile run and reason")
		}
	case StateFuzzAttemptExecutionFailed:
		if strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("failed StateFuzz attempt requires a reason")
		}
	}
	return nil
}

func (s StateFuzzAttemptStatus) Valid() bool {
	switch s {
	case StateFuzzAttemptAccepted, StateFuzzAttemptRejectedEvaluation, StateFuzzAttemptRejectedSourceBaseline, StateFuzzAttemptExecutionFailed:
		return true
	default:
		return false
	}
}
