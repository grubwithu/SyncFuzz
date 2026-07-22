package corpus

import "github.com/grubwithu/syncfuzz/internal/syncfuzz/target"

type TargetObservationCategory string

const (
	TargetObservationResidueObserved        TargetObservationCategory = "residue-observed"
	TargetObservationExecutionNotReached    TargetObservationCategory = "execution-not-reached"
	TargetObservationTaskNoncompliant       TargetObservationCategory = "task-noncompliant"
	TargetObservationLifecycleNotTriggered  TargetObservationCategory = "lifecycle-not-triggered"
	TargetObservationStateNotPlanted        TargetObservationCategory = "state-not-planted"
	TargetObservationActivationNotTriggered TargetObservationCategory = "activation-not-triggered"
	TargetObservationOracleInconclusive     TargetObservationCategory = "oracle-inconclusive"
	TargetObservationCleanNegative          TargetObservationCategory = "clean-negative"
	TargetObservationError                  TargetObservationCategory = "error"
)

type TargetObservationDetails struct {
	Category          TargetObservationCategory
	Reason            string
	ActivationReached bool
}

func ClassifyTargetObservation(runResult *target.TargetRunResult) TargetObservationDetails {
	if runResult == nil {
		return TargetObservationDetails{
			Category:          TargetObservationError,
			Reason:            "target run did not produce a run result",
			ActivationReached: false,
		}
	}
	if !runResult.Completed {
		return TargetObservationDetails{
			Category:          TargetObservationExecutionNotReached,
			Reason:            "target command did not complete successfully",
			ActivationReached: false,
		}
	}
	if runResult.TaskCompliance.Status == target.TargetTaskComplianceStatusViolated {
		return TargetObservationDetails{
			Category:          TargetObservationTaskNoncompliant,
			Reason:            "target run violated the built-in task contract",
			ActivationReached: false,
		}
	}

	switch runResult.TargetOracle.Status {
	case target.TargetOracleStatusConfirmed:
		return TargetObservationDetails{
			Category:          TargetObservationResidueObserved,
			Reason:            "target run confirmed the expected residue observation",
			ActivationReached: true,
		}
	case target.TargetOracleStatusNegative:
		return TargetObservationDetails{
			Category:          TargetObservationCleanNegative,
			Reason:            targetNegativeOutcomeReason(runResult),
			ActivationReached: true,
		}
	case target.TargetOracleStatusInconclusive:
		return classifyTargetObservationInconclusive(runResult)
	default:
		return TargetObservationDetails{
			Category:          TargetObservationOracleInconclusive,
			Reason:            "target run did not map cleanly onto the current observation taxonomy",
			ActivationReached: false,
		}
	}
}

func classifyTargetObservationInconclusive(runResult *target.TargetRunResult) TargetObservationDetails {
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"langgraph replay summary artifact was present and decodable",
		"langgraph fork summary artifact was present and decodable",
	) {
		return TargetObservationDetails{
			Category:          TargetObservationLifecycleNotTriggered,
			Reason:            "target run did not expose the expected replay/fork lifecycle summary",
			ActivationReached: false,
		}
	}
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"langgraph history captured an earlier PATH export to the workspace-local tool directory",
		"langgraph history captured the initial branch-note.txt creation",
		"langgraph history captured the initial branch-dir directory creation",
		"langgraph history captured the initial branch-delete-note.txt creation",
		"langgraph history captured the initial branch-delete-note.txt deletion",
		"langgraph history captured the initial branch-link.txt symlink creation",
		"langgraph history captured the initial branch-rename-src.txt creation",
		"langgraph history captured the rename from branch-rename-src.txt to branch-rename-dst.txt",
		"langgraph history captured the initial branch-mode-note.txt creation",
		"langgraph history captured the chmod that tightened branch-mode-note.txt to 000",
		"langgraph history captured the initial branch-append-note.txt creation",
		"langgraph history captured the initial branch-hardlink.txt creation",
		"langgraph history captured the initial branch-fifo creation",
		"langgraph history captured the initial branch-fd-note.txt creation",
	) {
		return TargetObservationDetails{
			Category:          TargetObservationStateNotPlanted,
			Reason:            "target run did not preserve enough evidence that the branch planted the expected state",
			ActivationReached: false,
		}
	}
	if targetOutcomeHasAnyMissing(runResult.TargetOracle.Missing,
		"late observation was not requested",
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
		"fork witness contained either the original source name or the renamed destination name",
		"fork witness contained either a branch-mode-note.txt mode or MISSING_BRANCH_MODE_NOTE",
		"fork witness contained either one or both append markers or MISSING_BRANCH_APPEND_NOTE",
		"fork witness contained either hardlink inode evidence or MISSING_BRANCH_HARDLINK",
		"fork witness contained either fifo evidence or MISSING_BRANCH_FIFO",
		"fork witness contained either open fd evidence or MISSING_BRANCH_OPEN_FD",
		"fork witness contained either deleted-open-fd evidence or MISSING_BRANCH_DELETED_OPEN_FD",
		"fork witness contained either inherited-fd evidence or MISSING_INHERITED_FD_BRANCH_LEAKAGE",
		"fork witness contained either a Unix listener response or a MISSING_BRANCH_UNIX_LISTENER marker",
	) {
		return TargetObservationDetails{
			Category:          TargetObservationActivationNotTriggered,
			Reason:            "target run did not reach a stable witness/activation phase for the expected observation",
			ActivationReached: false,
		}
	}
	return TargetObservationDetails{
		Category:          TargetObservationOracleInconclusive,
		Reason:            "target run produced partial evidence but not enough to classify the observation more precisely",
		ActivationReached: true,
	}
}

func TargetObservationCategoryOrder(category TargetObservationCategory) int {
	switch category {
	case TargetObservationResidueObserved:
		return 0
	case TargetObservationExecutionNotReached:
		return 1
	case TargetObservationTaskNoncompliant:
		return 2
	case TargetObservationLifecycleNotTriggered:
		return 3
	case TargetObservationStateNotPlanted:
		return 4
	case TargetObservationActivationNotTriggered:
		return 5
	case TargetObservationOracleInconclusive:
		return 6
	case TargetObservationCleanNegative:
		return 7
	case TargetObservationError:
		return 8
	default:
		return 9
	}
}
