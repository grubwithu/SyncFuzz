package syncfuzz

import "strings"

type ReplayOutcomeCategory string

const (
	replayOutcomeReproduced             ReplayOutcomeCategory = "reproduced"
	replayOutcomeSignatureDrift         ReplayOutcomeCategory = "signature-drift"
	replayOutcomeExecutionNotReached    ReplayOutcomeCategory = "execution-not-reached"
	replayOutcomeTaskNoncompliant       ReplayOutcomeCategory = "task-noncompliant"
	replayOutcomeLifecycleNotTriggered  ReplayOutcomeCategory = "lifecycle-not-triggered"
	replayOutcomeStateNotPlanted        ReplayOutcomeCategory = "state-not-planted"
	replayOutcomeResidueNotObserved     ReplayOutcomeCategory = "residue-not-observed"
	replayOutcomeActivationNotTriggered ReplayOutcomeCategory = "activation-not-triggered"
	replayOutcomeOracleInconclusive     ReplayOutcomeCategory = "oracle-inconclusive"
	replayOutcomeCleanNegative          ReplayOutcomeCategory = "clean-negative"
	replayOutcomeError                  ReplayOutcomeCategory = "error"
)

type replayOutcomeDetails struct {
	Category ReplayOutcomeCategory
	Reason   string
}

func classifyCaseReplayOutcome(runResult *RunResult, signatureMatched bool) replayOutcomeDetails {
	if runResult == nil {
		return replayOutcomeDetails{Category: replayOutcomeError, Reason: "case replay did not produce a run result"}
	}
	if runResult.Confirmed && signatureMatched {
		return replayOutcomeDetails{Category: replayOutcomeReproduced, Reason: "known-answer replay confirmed and matched the expected signature"}
	}
	if runResult.Confirmed && !signatureMatched {
		return replayOutcomeDetails{Category: replayOutcomeSignatureDrift, Reason: "known-answer replay confirmed but emitted a different mismatch signature"}
	}
	if len(runResult.Evidence) == 0 {
		return replayOutcomeDetails{Category: replayOutcomeResidueNotObserved, Reason: "known-answer replay did not confirm and emitted no oracle evidence"}
	}
	return replayOutcomeDetails{Category: replayOutcomeResidueNotObserved, Reason: "known-answer replay did not confirm the expected mismatch"}
}

func classifyTargetReplayOutcome(runResult *TargetRunResult, signatureMatched bool) replayOutcomeDetails {
	if runResult == nil {
		return replayOutcomeDetails{Category: replayOutcomeError, Reason: "target replay did not produce a run result"}
	}
	if runResult.ExpectationsMet && signatureMatched {
		return replayOutcomeDetails{Category: replayOutcomeReproduced, Reason: "target replay confirmed and matched the expected signature"}
	}
	if !runResult.Completed {
		return replayOutcomeDetails{Category: replayOutcomeExecutionNotReached, Reason: "target command did not complete successfully"}
	}
	if runResult.TaskCompliance.Status == targetTaskComplianceStatusViolated {
		return replayOutcomeDetails{Category: replayOutcomeTaskNoncompliant, Reason: "target replay violated the built-in task contract"}
	}
	if runResult.ExpectationsMet && !signatureMatched {
		return replayOutcomeDetails{Category: replayOutcomeSignatureDrift, Reason: "target replay confirmed but emitted a different mismatch signature"}
	}
	switch runResult.TargetOracle.Status {
	case targetOracleStatusNegative:
		return replayOutcomeDetails{
			Category: replayOutcomeCleanNegative,
			Reason:   targetNegativeOutcomeReason(runResult),
		}
	case targetOracleStatusInconclusive:
		return classifyTargetInconclusiveOutcome(runResult)
	case targetOracleStatusConfirmed:
		if !signatureMatched {
			return replayOutcomeDetails{Category: replayOutcomeSignatureDrift, Reason: "target replay confirmed but emitted a different mismatch signature"}
		}
	}
	return replayOutcomeDetails{Category: replayOutcomeOracleInconclusive, Reason: "target replay did not map cleanly onto the current replay taxonomy"}
}

func targetNegativeOutcomeReason(runResult *TargetRunResult) string {
	switch runResult.TargetOracle.Attribution {
	case targetOracleAttributionCleanReplay:
		return "target replay produced a clean replay outcome"
	case targetOracleAttributionCleanFork:
		return "target replay produced a clean fork outcome"
	case targetOracleAttributionLegitimateReexecution:
		return "target replay re-executed state without preserving residue across the boundary"
	case targetOracleAttributionExternalSmuggling:
		return "target replay depended on external helper state rather than direct runtime residue"
	case targetOracleAttributionWorkspaceRebuild:
		return "target replay rebuilt workspace state instead of preserving residue directly"
	default:
		return "target replay produced a negative oracle outcome"
	}
}

func classifyTargetInconclusiveOutcome(runResult *TargetRunResult) replayOutcomeDetails {
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"langgraph replay summary artifact was present and decodable",
		"langgraph fork summary artifact was present and decodable",
	) {
		return replayOutcomeDetails{
			Category: replayOutcomeLifecycleNotTriggered,
			Reason:   "target replay did not expose the expected replay/fork lifecycle summary",
		}
	}
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"langgraph history captured an earlier PATH export to the workspace-local tool directory",
		"langgraph history captured the initial branch-note.txt creation",
		"langgraph history captured the initial branch-dir directory creation",
		"langgraph history captured the initial branch-delete-note.txt creation",
		"langgraph history captured the initial branch-delete-note.txt deletion",
		"langgraph history captured the initial branch-link.txt symlink creation",
	) {
		return replayOutcomeDetails{
			Category: replayOutcomeStateNotPlanted,
			Reason:   "target replay did not preserve enough evidence that the initial branch planted the expected state",
		}
	}
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"workspace process appeared at command boundary",
		"workspace process remained after immediate observation",
		"replay witness preserved the workspace-local PATH override across replay",
		"replay witness resolved or executed the workspace-local git shim",
		"fork witness still showed the workspace-local tool directory inside PATH",
		"fork witness resolved or executed the workspace-local git shim",
		"fork witness contained a recognizable branch-note marker",
		"fork witness contained a recognizable branch-dir marker",
		"fork witness contained either the branch-delete-note marker or MISSING_BRANCH_DELETE_NOTE",
		"fork witness contained a recognizable branch-link.txt target",
	) {
		return replayOutcomeDetails{
			Category: replayOutcomeResidueNotObserved,
			Reason:   "target replay reached the verification phase but did not observe the expected residue",
		}
	}
	if runResult.TaskCompliance.Status == targetTaskComplianceStatusUnknown {
		return replayOutcomeDetails{
			Category: replayOutcomeOracleInconclusive,
			Reason:   "target replay lacked enough compliance structure to interpret the oracle outcome confidently",
		}
	}
	return replayOutcomeDetails{
		Category: replayOutcomeOracleInconclusive,
		Reason:   "target replay produced partial evidence but not enough to classify the failure more precisely",
	}
}

func targetOutcomeHasAnyMissing(items []string, needles ...string) bool {
	for _, item := range items {
		for _, needle := range needles {
			if strings.Contains(item, needle) {
				return true
			}
		}
	}
	return false
}
