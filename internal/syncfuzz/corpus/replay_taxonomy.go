package corpus

import (
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type ReplayOutcomeCategory string

const (
	ReplayOutcomeReproduced             ReplayOutcomeCategory = "reproduced"
	ReplayOutcomeSignatureDrift         ReplayOutcomeCategory = "signature-drift"
	ReplayOutcomeExecutionNotReached    ReplayOutcomeCategory = "execution-not-reached"
	ReplayOutcomeTaskNoncompliant       ReplayOutcomeCategory = "task-noncompliant"
	ReplayOutcomeLifecycleNotTriggered  ReplayOutcomeCategory = "lifecycle-not-triggered"
	ReplayOutcomeStateNotPlanted        ReplayOutcomeCategory = "state-not-planted"
	ReplayOutcomeResidueNotObserved     ReplayOutcomeCategory = "residue-not-observed"
	ReplayOutcomeActivationNotTriggered ReplayOutcomeCategory = "activation-not-triggered"
	ReplayOutcomeOracleInconclusive     ReplayOutcomeCategory = "oracle-inconclusive"
	ReplayOutcomeCleanNegative          ReplayOutcomeCategory = "clean-negative"
	ReplayOutcomeError                  ReplayOutcomeCategory = "error"
)

type ReplayOutcomeDetails struct {
	Category ReplayOutcomeCategory
	Reason   string
}

func ClassifyCaseReplayOutcome(runResult *core.RunResult, signatureMatched bool) ReplayOutcomeDetails {
	if runResult == nil {
		return ReplayOutcomeDetails{Category: ReplayOutcomeError, Reason: "case replay did not produce a run result"}
	}
	if runResult.Confirmed && signatureMatched {
		return ReplayOutcomeDetails{Category: ReplayOutcomeReproduced, Reason: "known-answer replay confirmed and matched the expected signature"}
	}
	if runResult.Confirmed && !signatureMatched {
		return ReplayOutcomeDetails{Category: ReplayOutcomeSignatureDrift, Reason: "known-answer replay confirmed but emitted a different mismatch signature"}
	}
	if len(runResult.Evidence) == 0 {
		return ReplayOutcomeDetails{Category: ReplayOutcomeResidueNotObserved, Reason: "known-answer replay did not confirm and emitted no oracle evidence"}
	}
	return ReplayOutcomeDetails{Category: ReplayOutcomeResidueNotObserved, Reason: "known-answer replay did not confirm the expected mismatch"}
}

func ClassifyTargetReplayOutcome(runResult *target.TargetRunResult, signatureMatched bool) ReplayOutcomeDetails {
	if runResult == nil {
		return ReplayOutcomeDetails{Category: ReplayOutcomeError, Reason: "target replay did not produce a run result"}
	}
	if runResult.ExpectationsMet && signatureMatched {
		return ReplayOutcomeDetails{Category: ReplayOutcomeReproduced, Reason: "target replay confirmed and matched the expected signature"}
	}
	if !runResult.Completed {
		return ReplayOutcomeDetails{Category: ReplayOutcomeExecutionNotReached, Reason: "target command did not complete successfully"}
	}
	if runResult.TaskCompliance.Status == target.TargetTaskComplianceStatusViolated {
		return ReplayOutcomeDetails{Category: ReplayOutcomeTaskNoncompliant, Reason: "target replay violated the built-in task contract"}
	}
	if runResult.ExpectationsMet && !signatureMatched {
		return ReplayOutcomeDetails{Category: ReplayOutcomeSignatureDrift, Reason: "target replay confirmed but emitted a different mismatch signature"}
	}
	switch runResult.TargetOracle.Status {
	case target.TargetOracleStatusNegative:
		return ReplayOutcomeDetails{
			Category: ReplayOutcomeCleanNegative,
			Reason:   targetNegativeOutcomeReason(runResult),
		}
	case target.TargetOracleStatusInconclusive:
		return classifyTargetInconclusiveOutcome(runResult)
	case target.TargetOracleStatusConfirmed:
		if !signatureMatched {
			return ReplayOutcomeDetails{Category: ReplayOutcomeSignatureDrift, Reason: "target replay confirmed but emitted a different mismatch signature"}
		}
	}
	return ReplayOutcomeDetails{Category: ReplayOutcomeOracleInconclusive, Reason: "target replay did not map cleanly onto the current replay taxonomy"}
}

func targetNegativeOutcomeReason(runResult *target.TargetRunResult) string {
	switch runResult.TargetOracle.Attribution {
	case target.TargetOracleAttributionCleanReplay:
		return "target replay produced a clean replay outcome"
	case target.TargetOracleAttributionCleanFork:
		return "target replay produced a clean fork outcome"
	case target.TargetOracleAttributionLegitimateReexecution:
		return "target replay re-executed state without preserving residue across the boundary"
	case target.TargetOracleAttributionExternalSmuggling:
		return "target replay depended on external helper state rather than direct runtime residue"
	case target.TargetOracleAttributionWorkspaceRebuild:
		return "target replay rebuilt workspace state instead of preserving residue directly"
	default:
		return "target replay produced a negative oracle outcome"
	}
}

func classifyTargetInconclusiveOutcome(runResult *target.TargetRunResult) ReplayOutcomeDetails {
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"langgraph replay summary artifact was present and decodable",
		"langgraph fork summary artifact was present and decodable",
	) {
		return ReplayOutcomeDetails{
			Category: ReplayOutcomeLifecycleNotTriggered,
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
		return ReplayOutcomeDetails{
			Category: ReplayOutcomeStateNotPlanted,
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
		return ReplayOutcomeDetails{
			Category: ReplayOutcomeResidueNotObserved,
			Reason:   "target replay reached the verification phase but did not observe the expected residue",
		}
	}
	if runResult.TaskCompliance.Status == target.TargetTaskComplianceStatusUnknown {
		return ReplayOutcomeDetails{
			Category: ReplayOutcomeOracleInconclusive,
			Reason:   "target replay lacked enough compliance structure to interpret the oracle outcome confidently",
		}
	}
	return ReplayOutcomeDetails{
		Category: ReplayOutcomeOracleInconclusive,
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
