package scheduler

import (
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetCandidateSummary struct {
	Rank                    int                                         `json:"rank"`
	CandidateID             string                                      `json:"candidate_id"`
	TargetID                string                                      `json:"target_id"`
	TaskID                  string                                      `json:"task_id"`
	PromptProfileID         string                                      `json:"prompt_profile_id,omitempty"`
	Runs                    int                                         `json:"runs"`
	Confirmed               int                                         `json:"confirmed"`
	Unconfirmed             int                                         `json:"unconfirmed"`
	Errors                  int                                         `json:"errors"`
	OracleConfirmed         int                                         `json:"oracle_confirmed"`
	OracleNegative          int                                         `json:"oracle_negative"`
	OracleInconclusive      int                                         `json:"oracle_inconclusive"`
	ComplianceCompliant     int                                         `json:"compliance_compliant"`
	ComplianceViolated      int                                         `json:"compliance_violated"`
	ComplianceUnknown       int                                         `json:"compliance_unknown"`
	ComplianceNotApplicable int                                         `json:"compliance_not_applicable"`
	ContractViolations      int                                         `json:"contract_violations"`
	ContractConsistent      int                                         `json:"contract_consistent"`
	ContractUnknown         int                                         `json:"contract_unknown"`
	Score                   int                                         `json:"score"`
	CostPenalty             int                                         `json:"cost_penalty"`
	ReproducibilityRate     float64                                     `json:"reproducibility_rate"`
	TotalDurationMillis     int64                                       `json:"total_duration_ms"`
	AvgDurationMillis       int64                                       `json:"avg_duration_ms"`
	TotalArtifactBytes      int64                                       `json:"total_artifact_bytes"`
	AvgArtifactBytes        int64                                       `json:"avg_artifact_bytes"`
	MaxArtifactBytes        int64                                       `json:"max_artifact_bytes"`
	TotalArtifactFiles      int                                         `json:"total_artifact_files"`
	AvgArtifactFiles        int                                         `json:"avg_artifact_files"`
	Status                  string                                      `json:"status"`
	Attributions            []string                                    `json:"attributions,omitempty"`
	OracleStatuses          []target.TargetOracleStatus                 `json:"oracle_statuses,omitempty"`
	ComplianceStatuses      []target.TargetTaskComplianceStatus         `json:"compliance_statuses,omitempty"`
	ContractStatuses        []target.TargetContractInterpretationStatus `json:"contract_statuses,omitempty"`
}

type targetCandidateAccumulator struct {
	summary            TargetCandidateSummary
	attributions       map[string]struct{}
	oracleStatuses     map[target.TargetOracleStatus]struct{}
	complianceStatuses map[target.TargetTaskComplianceStatus]struct{}
	contractStatuses   map[target.TargetContractInterpretationStatus]struct{}
}

func summarizeTargetCandidates(results []TargetSuiteRunResult) []TargetCandidateSummary {
	accumulators := make(map[string]*targetCandidateAccumulator)
	for _, result := range results {
		if result.CandidateID == "" {
			continue
		}
		accumulator := accumulators[result.CandidateID]
		if accumulator == nil {
			accumulator = &targetCandidateAccumulator{
				summary: TargetCandidateSummary{
					CandidateID:     result.CandidateID,
					TargetID:        result.TargetID,
					TaskID:          result.TaskID,
					PromptProfileID: result.PromptProfileID,
				},
				attributions:       make(map[string]struct{}),
				oracleStatuses:     make(map[target.TargetOracleStatus]struct{}),
				complianceStatuses: make(map[target.TargetTaskComplianceStatus]struct{}),
				contractStatuses:   make(map[target.TargetContractInterpretationStatus]struct{}),
			}
			accumulators[result.CandidateID] = accumulator
		}
		accumulator.observe(result)
	}

	summaries := make([]TargetCandidateSummary, 0, len(accumulators))
	for _, accumulator := range accumulators {
		summary := accumulator.summary
		if summary.Runs > 0 {
			summary.ReproducibilityRate = float64(summary.Confirmed) / float64(summary.Runs)
			summary.AvgDurationMillis = summary.TotalDurationMillis / int64(summary.Runs)
			summary.AvgArtifactBytes = summary.TotalArtifactBytes / int64(summary.Runs)
			summary.AvgArtifactFiles = summary.TotalArtifactFiles / summary.Runs
		}
		summary.Score = targetCandidateScore(summary)
		summary.CostPenalty = targetCandidateCostPenalty(summary)
		summary.Status = targetCandidateStatus(summary)
		summary.Attributions = sortedSet(accumulator.attributions)
		summary.OracleStatuses = sortedTargetOracleStatuses(accumulator.oracleStatuses)
		summary.ComplianceStatuses = sortedTargetComplianceStatuses(accumulator.complianceStatuses)
		summary.ContractStatuses = sortedTargetContractStatuses(accumulator.contractStatuses)
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Score != summaries[j].Score {
			return summaries[i].Score > summaries[j].Score
		}
		if summaries[i].ReproducibilityRate != summaries[j].ReproducibilityRate {
			return summaries[i].ReproducibilityRate > summaries[j].ReproducibilityRate
		}
		if summaries[i].ContractViolations != summaries[j].ContractViolations {
			return summaries[i].ContractViolations > summaries[j].ContractViolations
		}
		if summaries[i].Confirmed != summaries[j].Confirmed {
			return summaries[i].Confirmed > summaries[j].Confirmed
		}
		if summaries[i].ComplianceViolated != summaries[j].ComplianceViolated {
			return summaries[i].ComplianceViolated < summaries[j].ComplianceViolated
		}
		if summaries[i].CostPenalty != summaries[j].CostPenalty {
			return summaries[i].CostPenalty < summaries[j].CostPenalty
		}
		if summaries[i].AvgDurationMillis != summaries[j].AvgDurationMillis {
			return summaries[i].AvgDurationMillis < summaries[j].AvgDurationMillis
		}
		if summaries[i].AvgArtifactBytes != summaries[j].AvgArtifactBytes {
			return summaries[i].AvgArtifactBytes < summaries[j].AvgArtifactBytes
		}
		return summaries[i].CandidateID < summaries[j].CandidateID
	})
	for i := range summaries {
		summaries[i].Rank = i + 1
	}
	return summaries
}

func (a *targetCandidateAccumulator) observe(result TargetSuiteRunResult) {
	a.summary.Runs++
	a.summary.TotalDurationMillis += result.DurationMillis
	a.summary.TotalArtifactBytes += result.ArtifactBytes
	a.summary.TotalArtifactFiles += result.ArtifactFiles
	if result.ArtifactBytes > a.summary.MaxArtifactBytes {
		a.summary.MaxArtifactBytes = result.ArtifactBytes
	}

	switch {
	case result.Error != "":
		a.summary.Errors++
	case result.Confirmed:
		a.summary.Confirmed++
	default:
		a.summary.Unconfirmed++
	}

	switch result.TargetOracle.Status {
	case target.TargetOracleStatusConfirmed:
		a.summary.OracleConfirmed++
	case target.TargetOracleStatusNegative:
		a.summary.OracleNegative++
	case target.TargetOracleStatusInconclusive:
		a.summary.OracleInconclusive++
	}
	if result.TargetOracle.Status != "" {
		a.oracleStatuses[result.TargetOracle.Status] = struct{}{}
	}
	if result.TargetOracle.Attribution != "" {
		a.attributions[result.TargetOracle.Attribution] = struct{}{}
	}

	switch result.TaskCompliance.Status {
	case target.TargetTaskComplianceStatusCompliant:
		a.summary.ComplianceCompliant++
	case target.TargetTaskComplianceStatusViolated:
		a.summary.ComplianceViolated++
	case target.TargetTaskComplianceStatusUnknown:
		a.summary.ComplianceUnknown++
	case target.TargetTaskComplianceStatusNotApplicable:
		a.summary.ComplianceNotApplicable++
	}
	if result.TaskCompliance.Status != "" {
		a.complianceStatuses[result.TaskCompliance.Status] = struct{}{}
	}

	contractStatus := target.TargetContractInterpretationStatusValue(result.ContractInterpretation)
	switch contractStatus {
	case target.TargetContractStatusViolation:
		a.summary.ContractViolations++
	case target.TargetContractStatusConsistent:
		a.summary.ContractConsistent++
	case target.TargetContractStatusUnknown:
		a.summary.ContractUnknown++
	}
	if contractStatus != "" {
		a.contractStatuses[contractStatus] = struct{}{}
	}
}

func targetCandidateScore(summary TargetCandidateSummary) int {
	return summary.ContractViolations*4 +
		summary.Confirmed*2 +
		summary.OracleInconclusive -
		summary.ComplianceViolated*2 -
		summary.Errors*5
}

func targetCandidateCostPenalty(summary TargetCandidateSummary) int {
	durationPenalty := int(summary.AvgDurationMillis / 1000)
	artifactPenalty := int(summary.AvgArtifactBytes / (1024 * 1024))
	filePenalty := summary.AvgArtifactFiles / 100
	return durationPenalty + artifactPenalty + filePenalty
}

func targetCandidateStatus(summary TargetCandidateSummary) string {
	switch {
	case summary.Runs == 0:
		return "not-run"
	case summary.Errors == summary.Runs:
		return "error"
	case summary.ContractViolations > 0:
		return "contract-violation"
	case summary.Confirmed > 0:
		return "confirmed"
	case summary.OracleInconclusive > 0:
		return "inconclusive"
	case summary.OracleNegative > 0:
		return "negative"
	default:
		return "unknown"
	}
}

func sortedTargetOracleStatuses(values map[target.TargetOracleStatus]struct{}) []target.TargetOracleStatus {
	out := make([]target.TargetOracleStatus, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return targetOracleStatusOrder(out[i]) < targetOracleStatusOrder(out[j])
	})
	return out
}

func sortedTargetComplianceStatuses(values map[target.TargetTaskComplianceStatus]struct{}) []target.TargetTaskComplianceStatus {
	out := make([]target.TargetTaskComplianceStatus, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		li := targetTaskComplianceStatusOrder(out[i])
		lj := targetTaskComplianceStatusOrder(out[j])
		if li != lj {
			return li < lj
		}
		return out[i] < out[j]
	})
	return out
}

func sortedTargetContractStatuses(values map[target.TargetContractInterpretationStatus]struct{}) []target.TargetContractInterpretationStatus {
	out := make([]target.TargetContractInterpretationStatus, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		li := targetContractInterpretationStatusOrder(out[i])
		lj := targetContractInterpretationStatusOrder(out[j])
		if li != lj {
			return li < lj
		}
		return out[i] < out[j]
	})
	return out
}

func targetOracleStatusOrder(status target.TargetOracleStatus) int {
	switch status {
	case target.TargetOracleStatusConfirmed:
		return 0
	case target.TargetOracleStatusInconclusive:
		return 1
	case target.TargetOracleStatusNegative:
		return 2
	default:
		return 3
	}
}
