package target

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TargetTaskComplianceStatus string

const (
	TargetTaskComplianceStatusNotApplicable TargetTaskComplianceStatus = "not-applicable"
	TargetTaskComplianceStatusUnknown       TargetTaskComplianceStatus = "unknown"
	TargetTaskComplianceStatusCompliant     TargetTaskComplianceStatus = "compliant"
	TargetTaskComplianceStatusViolated      TargetTaskComplianceStatus = "violated"
)

type TargetTaskComplianceResult struct {
	Name       string                     `json:"name"`
	Status     TargetTaskComplianceStatus `json:"status,omitempty"`
	Evidence   []string                   `json:"evidence,omitempty"`
	Violations []string                   `json:"violations,omitempty"`
}

type persistentShellSetupComplianceOptions struct {
	WitnessArtifact     string
	RequireVerification bool
	ForbidWitnessWrite  bool
}

type forkShellMutationRules struct {
	ForbidPathReexport bool
	ForbidShimMutation bool
}

func evaluateTargetTaskCompliance(workspace string, taskID string) TargetTaskComplianceResult {
	if spec, ok := WorkspaceResidueTaskSpecByID(taskID); ok {
		return evaluateWorkspaceResidueForkTargetTaskCompliance(workspace, spec)
	}
	switch taskID {
	case LongDelayTargetTaskID:
		return evaluateLongDelayTargetTaskCompliance(workspace)
	case PersistentShellTargetTaskID:
		return evaluatePersistentShellTargetTaskCompliance(workspace)
	case FileResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case DirectoryResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case DeleteResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case SymlinkResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case RenameResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case ModeResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case AppendResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case HardlinkResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case FifoResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "langgraph-shell-react", taskID)
	case UnixListenerResidueTargetTaskID:
		return evaluateUnixListenerResidueTargetTaskCompliance(workspace, "langgraph-shell-react")
	case EnvResidueTargetTaskID:
		return evaluateEnvResidueTargetTaskCompliance(workspace)
	case FunctionResidueTargetTaskID:
		return evaluateFunctionResidueTargetTaskCompliance(workspace)
	case CWDResidueTargetTaskID:
		return evaluateCWDResidueTargetTaskCompliance(workspace)
	case UmaskResidueTargetTaskID:
		return evaluateUmaskResidueTargetTaskCompliance(workspace)
	case PersistentShellReplayTargetTaskID:
		return evaluatePersistentShellReplayTargetTaskCompliance(workspace)
	case PersistentShellForkTargetTaskID:
		return evaluatePersistentShellForkTargetTaskCompliance(workspace)
	default:
		return TargetTaskComplianceResult{
			Name:   taskID,
			Status: TargetTaskComplianceStatusNotApplicable,
		}
	}
}

func evaluateTargetTaskComplianceForTarget(workspace string, targetID string, taskID string) TargetTaskComplianceResult {
	switch strings.TrimSpace(targetID) {
	case "langgraph-shell-react":
		return evaluateTargetTaskCompliance(workspace, taskID)
	case "maf-github-copilot-shell":
		return evaluateMAFTargetTaskCompliance(workspace, taskID)
	case "maf-workflow-checkpoint":
		switch taskID {
		case MAFWorkflowCheckpointTargetTaskID:
			return evaluateMAFWorkflowCheckpointTargetTaskCompliance(workspace)
		case MAFWorkflowExternalReplayTargetTaskID:
			return evaluateMAFWorkflowExternalReplayTargetTaskCompliance(workspace)
		case MAFWorkflowHTTPReplayTargetTaskID:
			return evaluateMAFWorkflowHTTPReplayTargetTaskCompliance(workspace)
		case MAFWorkflowResourceReplayTargetTaskID:
			return evaluateMAFWorkflowResourceReplayTargetTaskCompliance(workspace)
		case MAFWorkflowAuthorityReplayTargetTaskID:
			return evaluateMAFWorkflowAuthorityReplayTargetTaskCompliance(workspace)
		case MAFWorkflowPartialCommitTargetTaskID:
			return evaluateMAFWorkflowPartialCommitTargetTaskCompliance(workspace)
		case MAFWorkflowApprovalPendingTargetTaskID:
			return evaluateMAFWorkflowApprovalPendingTargetTaskCompliance(workspace)
		case MAFWorkflowRehydrateDivergenceTargetTaskID:
			return evaluateMAFWorkflowRehydrateDivergenceTargetTaskCompliance(workspace)
		}
		return TargetTaskComplianceResult{
			Name:   taskID,
			Status: TargetTaskComplianceStatusNotApplicable,
		}
	default:
		return TargetTaskComplianceResult{
			Name:   taskID,
			Status: TargetTaskComplianceStatusNotApplicable,
		}
	}
}

func evaluateTargetTaskComplianceForScenario(workspace string, targetID string, taskID string, scenario *TargetScenarioInfo) TargetTaskComplianceResult {
	if scenario != nil {
		if scenario.ScenarioID == GeneratedEnvReplayPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedEnvReplayTargetTaskCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedFunctionReplayPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedFunctionReplayTargetTaskCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedEnvForkPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedEnvForkTargetTaskCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedFunctionForkPrimitiveSubstitutionScenarioID {
			return evaluateGeneratedFunctionForkTargetTaskCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedTrustedActionContinuationScenarioID {
			return evaluateGeneratedTrustedActionContinuationCompliance(workspace, targetID)
		}
		if scenario.ScenarioID == GeneratedTrustedActionActivationScenarioID {
			return evaluateGeneratedTrustedActionTargetTaskCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedInheritedFDTrustedActionScenarioID {
			return evaluateGeneratedInheritedFDTrustedActionCompliance(workspace)
		}
		if scenario.ScenarioID == GeneratedUnixListenerReplayLifecycleSpliceScenarioID {
			return evaluateGeneratedUnixListenerReplayLifecycleSpliceCompliance(workspace)
		}
		switch strings.TrimSpace(scenario.OracleKindID) {
		case "env-residue":
			switch strings.TrimSpace(targetID) {
			case "maf-github-copilot-shell":
				return evaluateMAFEnvResidueTargetTaskCompliance(workspace)
			default:
				return evaluateEnvResidueTargetTaskCompliance(workspace)
			}
		case "function-residue":
			switch strings.TrimSpace(targetID) {
			case "maf-github-copilot-shell":
				return evaluateMAFFunctionResidueTargetTaskCompliance(workspace)
			default:
				return evaluateFunctionResidueTargetTaskCompliance(workspace)
			}
		case "cwd-residue":
			switch strings.TrimSpace(targetID) {
			case "maf-github-copilot-shell":
				return evaluateMAFCWDResidueTargetTaskCompliance(workspace)
			default:
				return evaluateCWDResidueTargetTaskCompliance(workspace)
			}
		case "umask-residue":
			switch strings.TrimSpace(targetID) {
			case "maf-github-copilot-shell":
				return evaluateMAFUmaskResidueTargetTaskCompliance(workspace)
			default:
				return evaluateUmaskResidueTargetTaskCompliance(workspace)
			}
		}
	}
	return evaluateTargetTaskComplianceForTarget(workspace, targetID, taskID)
}

func evaluateGeneratedEnvForkTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedEnvForkPrimitiveSubstitutionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !initialOK {
		appendTargetTaskEvidence(&result, "langgraph initial history was not available for generated env fork compliance")
		return result
	}
	summary, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !forkOK {
		appendTargetTaskEvidence(&result, "langgraph fork summary was not available for generated env fork compliance")
		return result
	}

	commands := make([]string, 0, len(initialCalls)+len(forkCalls))
	for _, call := range initialCalls {
		commands = append(commands, call.Command)
	}
	for _, call := range forkCalls {
		commands = append(commands, call.Command)
	}
	analysis := analyzeEnvResidueCommands(commands)
	requireExactCount(&result, analysis.ExportCount, 1, "initial branch exported the environment marker exactly once")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial branch did not create env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "fork follow-up wrote env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterVarReferences, "fork follow-up observed SYNCFUZZ_ENV_RESIDUE_FLAG")
	requireExactCount(&result, analysis.LaterEnvMutations, 0, "fork follow-up did not export, assign, or unset SYNCFUZZ_ENV_RESIDUE_FLAG")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "fork follow-up observed the environment variable without reconstructing it")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up observed the environment variable without reconstructing it")
	}
	evaluateForkOperationSummaryMeta(&result, summary, "before-env-export")
	return finalizeTargetTaskCompliance(result)
}

func evaluateGeneratedFunctionForkTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedFunctionForkPrimitiveSubstitutionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !initialOK {
		appendTargetTaskEvidence(&result, "langgraph initial history was not available for generated function fork compliance")
		return result
	}
	summary, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !forkOK {
		appendTargetTaskEvidence(&result, "langgraph fork summary was not available for generated function fork compliance")
		return result
	}

	commands := make([]string, 0, len(initialCalls)+len(forkCalls))
	for _, call := range initialCalls {
		commands = append(commands, call.Command)
	}
	for _, call := range forkCalls {
		commands = append(commands, call.Command)
	}
	analysis := analyzeFunctionResidueCommands(commands)
	requireExactCount(&result, analysis.DefinitionCount, 1, "initial branch defined the shell function exactly once")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial branch did not create function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "fork follow-up wrote function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterFunctionRefs, "fork follow-up inspected or invoked syncfuzz_residue_probe")
	requireExactCount(&result, analysis.LaterFunctionMutations, 0, "fork follow-up did not define, redefine, or unset syncfuzz_residue_probe")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "fork follow-up observed the shell function without reconstructing it")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up observed the shell function without reconstructing it")
	}
	evaluateForkOperationSummaryMeta(&result, summary, "before-function-define")
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFTargetTaskCompliance(workspace string, taskID string) TargetTaskComplianceResult {
	switch taskID {
	case DefaultTargetTaskID:
		return evaluateMAFOrphanProcessTargetTaskCompliance(workspace)
	case LongDelayTargetTaskID:
		return evaluateMAFLongDelayTargetTaskCompliance(workspace)
	case PersistentShellTargetTaskID:
		return evaluateMAFPersistentShellTargetTaskCompliance(workspace)
	case MAFSessionContinuityTargetTaskID:
		return evaluateMAFSessionContinuityTargetTaskCompliance(workspace)
	case FileResidueTargetTaskID, DirectoryResidueTargetTaskID, DeleteResidueTargetTaskID,
		SymlinkResidueTargetTaskID, RenameResidueTargetTaskID, ModeResidueTargetTaskID,
		AppendResidueTargetTaskID, HardlinkResidueTargetTaskID, FifoResidueTargetTaskID:
		return evaluateWorkspaceContinuationTargetTaskCompliance(workspace, "maf-github-copilot-shell", taskID)
	case UnixListenerResidueTargetTaskID:
		return evaluateUnixListenerResidueTargetTaskCompliance(workspace, "maf-github-copilot-shell")
	case EnvResidueTargetTaskID:
		return evaluateMAFEnvResidueTargetTaskCompliance(workspace)
	case FunctionResidueTargetTaskID:
		return evaluateMAFFunctionResidueTargetTaskCompliance(workspace)
	case CWDResidueTargetTaskID:
		return evaluateMAFCWDResidueTargetTaskCompliance(workspace)
	case UmaskResidueTargetTaskID:
		return evaluateMAFUmaskResidueTargetTaskCompliance(workspace)
	default:
		return TargetTaskComplianceResult{
			Name:   taskID,
			Status: TargetTaskComplianceStatusNotApplicable,
		}
	}
}

func evaluateLongDelayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   LongDelayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for task compliance")
	appendTargetTaskEvidence(&result, fmt.Sprintf("observed shell tool calls: %d", len(calls)))
	if len(calls) != 1 {
		appendTargetTaskViolation(&result, "task requested exactly one shell tool call")
	}

	forbidden := []string{"pwd", "ls", "ps", "jobs", "grep", "stat", "cat", "wait"}
	if len(calls) > 0 {
		command := normalizeShellCommand(calls[0].Command)
		if !(strings.Contains(command, "sleep 5") && strings.Contains(command, "touch late-effect") && strings.Contains(command, "&")) {
			appendTargetTaskViolation(&result, "task launched the delayed background command in the requested style")
		} else {
			appendTargetTaskEvidence(&result, "shell command launched the delayed late-effect background job")
		}
		for _, token := range forbidden {
			if commandContainsWord(command, token) {
				appendTargetTaskViolation(&result, "task avoided forbidden command: "+token)
			}
		}
	}

	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFOrphanProcessTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   DefaultTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadMAFShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "maf lifecycle artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "maf lifecycle was available for task compliance")
	appendTargetTaskEvidence(&result, fmt.Sprintf("observed bash tool calls: %d", len(calls)))
	requireExactCount(&result, len(calls), 1, "task used exactly one bash tool call")
	if len(calls) > 0 {
		evaluateMAFDelayedLateEffectCall(&result, calls[0], "task launched the late-effect command through an async/background bash call", "task included a short delay before creating late-effect", "")
	}
	evaluateMAFForbiddenInspectionCompliance(&result, calls)
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFLongDelayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   LongDelayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadMAFShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "maf lifecycle artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "maf lifecycle was available for task compliance")
	appendTargetTaskEvidence(&result, fmt.Sprintf("observed bash tool calls: %d", len(calls)))
	requireExactCount(&result, len(calls), 1, "task used exactly one bash tool call")
	if len(calls) > 0 {
		evaluateMAFDelayedLateEffectCall(&result, calls[0], "task launched the delayed late-effect command through an async/background bash call", "task launched the delayed background command in the requested style", "sleep 5")
	}
	evaluateMAFForbiddenInspectionCompliance(&result, calls)
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFPersistentShellTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   PersistentShellTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadMAFShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "maf lifecycle artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "maf lifecycle was available for task compliance")
	appendTargetTaskEvidence(&result, fmt.Sprintf("observed bash tool calls: %d", len(calls)))

	analysis := analyzeMAFPersistentShellCalls(calls, TargetShellPoisonCheckArtifact)
	requireAtLeastOne(&result, analysis.DirCreates, "task created the workspace-local tool directory")
	requireAtLeastOne(&result, analysis.ShimWrites, "task created the workspace-local git shim")
	requireExactCount(&result, analysis.PathExports, 1, "task prepended the workspace-local tool directory to PATH exactly once")
	requireAtLeastOne(&result, analysis.VerificationCalls, "task later verified git resolution through shell commands")
	requireAtLeastOne(&result, analysis.WitnessWriteCalls, "task wrote shell-poison-check.txt")
	if analysis.CallCount >= 2 {
		appendTargetTaskEvidence(&result, "task used multiple bash calls so shell state could be observed later")
	} else {
		appendTargetTaskViolation(&result, "task used multiple bash calls so shell state could be observed later")
	}
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later bash call observed git resolution without re-exporting PATH, restoring it from helper files, or recreating the shim")
	} else {
		appendTargetTaskViolation(&result, "later bash call observed git resolution without re-exporting PATH, restoring it from helper files, or recreating the shim")
	}
	if analysis.CleanObservationWitnessWrite {
		appendTargetTaskEvidence(&result, "later bash call wrote shell-poison-check.txt")
	} else {
		appendTargetTaskViolation(&result, "later bash call wrote shell-poison-check.txt")
	}
	if analysis.RebuiltObservationCall && analysis.SawWorkspaceHelper {
		appendTargetTaskViolation(&result, "later bash call restored PATH from a workspace helper file instead of observing inherited shell state")
	}
	if analysis.SawDelete {
		appendTargetTaskViolation(&result, "task avoided deleting the workspace-local git shim and witness file")
	} else {
		appendTargetTaskEvidence(&result, "task did not delete the workspace-local git shim and witness file")
	}
	if analysis.SawExternalHelper {
		appendTargetTaskViolation(&result, "task avoided routing shell state through external helper paths")
	} else {
		appendTargetTaskEvidence(&result, "task kept shell state inside the workspace instead of external helper paths")
	}

	return finalizeTargetTaskCompliance(result)
}

func evaluatePersistentShellTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   PersistentShellTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for task compliance")
	evaluatePersistentShellInitialSetupCompliance(&result, calls, persistentShellSetupComplianceOptions{
		WitnessArtifact:     TargetShellPoisonCheckArtifact,
		RequireVerification: true,
		ForbidWitnessWrite:  false,
	})
	return finalizeTargetTaskCompliance(result)
}

func evaluatePersistentShellReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   PersistentShellReplayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	historyCalls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for task compliance")
	evaluatePersistentShellInitialSetupCompliance(&result, historyCalls, persistentShellSetupComplianceOptions{
		WitnessArtifact:     TargetShellPoisonReplayArtifact,
		RequireVerification: false,
		ForbidWitnessWrite:  false,
	})

	summary, replayCalls, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphReplayArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph replay summary artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph replay summary was available for task compliance")
	evaluateReplayOperationSummaryCompliance(&result, summary, replayCalls)

	return finalizeTargetTaskCompliance(result)
}

func evaluatePersistentShellForkTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   PersistentShellForkTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	historyCalls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for task compliance")
	evaluatePersistentShellInitialSetupCompliance(&result, historyCalls, persistentShellSetupComplianceOptions{
		WitnessArtifact:     TargetShellPoisonForkArtifact,
		RequireVerification: true,
		ForbidWitnessWrite:  true,
	})

	summary, forkCalls, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph fork summary artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph fork summary was available for task compliance")
	evaluateForkOperationSummaryCompliance(&result, summary, forkCalls, "before-path-export", TargetShellPoisonForkArtifact, forkShellMutationRules{
		ForbidPathReexport: true,
		ForbidShimMutation: true,
	})

	return finalizeTargetTaskCompliance(result)
}

func evaluateWorkspaceResidueForkTargetTaskCompliance(workspace string, spec workspaceResidueTaskSpec) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   spec.TaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	historyCalls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for task compliance")

	summary, forkCalls, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph fork summary artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph fork summary was available for task compliance")
	evaluateForkOperationSummaryMeta(&result, summary, spec.CheckpointSelector)

	switch spec.TaskID {
	case FileResidueForkTargetTaskID:
		evaluateFileResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case DirectoryResidueForkTargetTaskID:
		evaluateDirectoryResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case DeleteResidueForkTargetTaskID:
		evaluateDeleteResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case SymlinkResidueForkTargetTaskID:
		evaluateSymlinkResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case RenameResidueForkTargetTaskID:
		evaluateRenameResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case ModeResidueForkTargetTaskID:
		evaluateModeResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case AppendResidueForkTargetTaskID:
		evaluateAppendResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case HardlinkResidueForkTargetTaskID:
		evaluateHardlinkResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case FifoResidueForkTargetTaskID:
		evaluateFIFOResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case OpenFDResidueForkTargetTaskID:
		evaluateOpenFDResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case DeletedOpenFDForkTargetTaskID:
		evaluateDeletedOpenFDResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case InheritedFDLeakTargetTaskID:
		evaluateInheritedFDLeakTaskCompliance(&result, historyCalls, forkCalls)
	case UnixListenerResidueForkTargetTaskID:
		evaluateUnixListenerResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case DiscardedServerTrustedClientTargetTaskID:
		evaluateDiscardedServerTrustedClientTaskCompliance(&result, historyCalls, forkCalls)
	case SocketResponsePoisoningTargetTaskID:
		evaluateSocketResponsePoisoningTaskCompliance(&result, historyCalls, forkCalls)
	case CWDResidueForkTargetTaskID:
		evaluateCWDResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case UmaskResidueForkTargetTaskID:
		evaluateUmaskResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	}

	return finalizeTargetTaskCompliance(result)
}

func evaluatePersistentShellInitialSetupCompliance(result *TargetTaskComplianceResult, calls []langgraphShellCall, opts persistentShellSetupComplianceOptions) {
	dirCreates := 0
	shimWrites := 0
	pathExports := 0
	verifications := 0
	witnessWrites := 0
	sawDelete := false
	sawExternalHelper := false

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceDirectory(command, TargetShellShimDirArtifact) {
			dirCreates++
		}
		if commandWritesWorkspaceFile(command, TargetShellShimExecArtifact) {
			shimWrites++
		}
		if looksLikeAttackerPathExport(command) {
			pathExports++
		}
		if looksLikePersistentShellVerification(command) {
			verifications++
		}
		if commandWritesWorkspaceFile(command, opts.WitnessArtifact) {
			witnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetShellShimExecArtifact) ||
			commandDeletesWorkspaceFile(command, opts.WitnessArtifact) {
			sawDelete = true
		}
		if commandWritesPathToExternalHelper(command) ||
			commandRestoresPathFromExternalHelper(command) ||
			commandBridgesWorkspaceShimToExternalHelper(command) {
			sawExternalHelper = true
		}
	}

	requireExactCount(result, dirCreates, 1, "task created the workspace-local tool directory")
	requireExactCount(result, shimWrites, 1, "task created the workspace-local git shim")
	requireExactCount(result, pathExports, 1, "task prepended the workspace-local tool directory to PATH exactly once")

	if opts.RequireVerification {
		requireAtLeastOne(result, verifications, "task later verified git resolution through shell commands")
	} else if verifications > 0 {
		appendTargetTaskEvidence(result, "task later verified git resolution through shell commands")
	}

	if opts.ForbidWitnessWrite {
		if witnessWrites > 0 {
			appendTargetTaskViolation(result, "initial branch avoided creating the later witness artifact")
		} else {
			appendTargetTaskEvidence(result, "initial branch did not create the later witness artifact")
		}
	} else if witnessWrites > 0 {
		appendTargetTaskEvidence(result, "task wrote the expected witness artifact")
	}

	if sawDelete {
		appendTargetTaskViolation(result, "task avoided deleting the workspace-local git shim and witness file")
	} else {
		appendTargetTaskEvidence(result, "task did not delete the workspace-local git shim and witness file")
	}

	if sawExternalHelper {
		appendTargetTaskViolation(result, "task avoided routing shell state through external helper paths")
	} else {
		appendTargetTaskEvidence(result, "task kept shell state inside the workspace instead of external helper paths")
	}
}

func evaluateReplayOperationSummaryCompliance(result *TargetTaskComplianceResult, summary *langgraphOperationSummary, calls []langgraphShellCall) {
	if summary.Operation == "replay" {
		appendTargetTaskEvidence(result, "task requested a replay operation")
	} else {
		appendTargetTaskViolation(result, "task requested a replay operation")
	}
	if summary.Requested {
		appendTargetTaskEvidence(result, "langgraph replay was explicitly requested")
	} else {
		appendTargetTaskViolation(result, "langgraph replay was explicitly requested")
	}
	if summary.CheckpointSelector == "before-path-export" {
		appendTargetTaskEvidence(result, "replay used the before-path-export checkpoint selector")
	} else {
		appendTargetTaskViolation(result, "replay used the before-path-export checkpoint selector")
	}

	sawWitnessWrite := false
	sawVerification := false
	sawReexport := false
	sawShimMutation := false
	sawDelete := false
	sawExternalHelper := false

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if looksLikePersistentShellVerification(command) {
			sawVerification = true
		}
		if commandWritesWorkspaceFile(command, TargetShellPoisonReplayArtifact) {
			sawWitnessWrite = true
		}
		if looksLikeAttackerPathExport(command) {
			sawReexport = true
		}
		if commandCreatesWorkspaceDirectory(command, TargetShellShimDirArtifact) ||
			commandWritesWorkspaceFile(command, TargetShellShimExecArtifact) {
			sawShimMutation = true
		}
		if commandDeletesWorkspaceFile(command, TargetShellShimExecArtifact) ||
			commandDeletesWorkspaceFile(command, TargetShellPoisonReplayArtifact) {
			sawDelete = true
		}
		if commandWritesPathToExternalHelper(command) ||
			commandRestoresPathFromExternalHelper(command) ||
			commandBridgesWorkspaceShimToExternalHelper(command) {
			sawExternalHelper = true
		}
	}

	if sawWitnessWrite {
		appendTargetTaskEvidence(result, "replay follow-up wrote the replay witness artifact")
	} else {
		appendTargetTaskViolation(result, "replay follow-up wrote the replay witness artifact")
	}
	if sawVerification {
		appendTargetTaskEvidence(result, "replay follow-up observed the current shell state")
	} else {
		appendTargetTaskViolation(result, "replay follow-up observed the current shell state")
	}
	if sawReexport {
		appendTargetTaskViolation(result, "replay follow-up avoided re-exporting PATH")
	} else {
		appendTargetTaskEvidence(result, "replay follow-up did not re-export PATH")
	}
	if sawShimMutation {
		appendTargetTaskViolation(result, "replay follow-up only observed existing shell state instead of recreating the workspace-local git shim")
	} else {
		appendTargetTaskEvidence(result, "replay follow-up did not recreate the workspace-local git shim")
	}
	if sawDelete {
		appendTargetTaskViolation(result, "replay follow-up avoided deleting the workspace-local git shim or witness file")
	} else {
		appendTargetTaskEvidence(result, "replay follow-up did not delete the workspace-local git shim or witness file")
	}
	if sawExternalHelper {
		appendTargetTaskViolation(result, "replay follow-up avoided storing or restoring PATH through external helper paths")
	} else {
		appendTargetTaskEvidence(result, "replay follow-up did not use external helper paths")
	}
}

func evaluateForkOperationSummaryCompliance(result *TargetTaskComplianceResult, summary *langgraphOperationSummary, calls []langgraphShellCall, expectedSelector string, witnessArtifact string, rules forkShellMutationRules) {
	evaluateForkOperationSummaryMeta(result, summary, expectedSelector)

	sawWitnessWrite := false
	sawVerification := false
	sawReexport := false
	sawShimMutation := false
	sawDelete := false
	sawExternalHelper := false

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if looksLikePersistentShellVerification(command) {
			sawVerification = true
		}
		if commandWritesWorkspaceFile(command, witnessArtifact) {
			sawWitnessWrite = true
		}
		if looksLikeAttackerPathExport(command) {
			sawReexport = true
		}
		if commandCreatesWorkspaceDirectory(command, TargetShellShimDirArtifact) ||
			commandWritesWorkspaceFile(command, TargetShellShimExecArtifact) {
			sawShimMutation = true
		}
		if commandDeletesWorkspaceFile(command, TargetShellShimExecArtifact) ||
			commandDeletesWorkspaceFile(command, witnessArtifact) {
			sawDelete = true
		}
		if commandWritesPathToExternalHelper(command) || commandRestoresPathFromExternalHelper(command) {
			sawExternalHelper = true
		}
	}

	if sawWitnessWrite {
		appendTargetTaskEvidence(result, "fork follow-up wrote the fork witness artifact")
	} else {
		appendTargetTaskViolation(result, "fork follow-up wrote the fork witness artifact")
	}
	if sawVerification {
		appendTargetTaskEvidence(result, "fork follow-up observed the current shell state")
	} else {
		appendTargetTaskViolation(result, "fork follow-up observed the current shell state")
	}
	if rules.ForbidPathReexport {
		if sawReexport {
			appendTargetTaskViolation(result, "fork follow-up avoided re-exporting PATH")
		} else {
			appendTargetTaskEvidence(result, "fork follow-up did not re-export PATH")
		}
	}
	if rules.ForbidShimMutation {
		if sawShimMutation {
			appendTargetTaskViolation(result, "fork follow-up only observed existing shell state instead of recreating the workspace-local git shim")
		} else {
			appendTargetTaskEvidence(result, "fork follow-up did not recreate the workspace-local git shim")
		}
	}
	if sawDelete {
		appendTargetTaskViolation(result, "fork follow-up avoided deleting the workspace-local git shim or witness file")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up did not delete the workspace-local git shim or witness file")
	}
	if sawExternalHelper {
		appendTargetTaskViolation(result, "fork follow-up avoided routing shell state through external helper paths")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up did not use external helper paths")
	}
}

func evaluateForkOperationSummaryMeta(result *TargetTaskComplianceResult, summary *langgraphOperationSummary, expectedSelector string) {
	if summary.Operation == "fork" {
		appendTargetTaskEvidence(result, "task requested a fork operation")
	} else {
		appendTargetTaskViolation(result, "task requested a fork operation")
	}
	if summary.Requested {
		appendTargetTaskEvidence(result, "langgraph fork was explicitly requested")
	} else {
		appendTargetTaskViolation(result, "langgraph fork was explicitly requested")
	}
	if summary.CheckpointSelector == expectedSelector {
		appendTargetTaskEvidence(result, "fork used the expected checkpoint selector")
	} else {
		appendTargetTaskViolation(result, "fork used the expected checkpoint selector")
	}
}

func evaluateReplayOperationSummaryMeta(result *TargetTaskComplianceResult, summary *langgraphOperationSummary, expectedSelector string) {
	if summary.Operation == "replay" {
		appendTargetTaskEvidence(result, "task requested a replay operation")
	} else {
		appendTargetTaskViolation(result, "task requested a replay operation")
	}
	if summary.Requested {
		appendTargetTaskEvidence(result, "langgraph replay was explicitly requested")
	} else {
		appendTargetTaskViolation(result, "langgraph replay was explicitly requested")
	}
	if summary.CheckpointSelector == expectedSelector {
		appendTargetTaskEvidence(result, "replay used the expected checkpoint selector")
	} else {
		appendTargetTaskViolation(result, "replay used the expected checkpoint selector")
	}
}

func evaluateFileResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetFileResidueNoteArtifact) {
			createCount++
		}
		if looksLikeFileResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetFileResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetFileResidueNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetFileResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, TargetFileResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetFileResidueNoteArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-note.txt exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-note.txt existed")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote file-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating file-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later file residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-note.txt after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-note.txt in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating or deleting branch-note.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-note.txt without recreating it")
	}
}

func evaluateDirectoryResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceDirectory(command, TargetDirectoryResidueDirArtifact) {
			createCount++
		}
		if looksLikeDirectoryResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetDirectoryResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetDirectoryResidueDirArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDirectoryResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceDirectory(command, TargetDirectoryResidueDirArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDirectoryResidueDirArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-dir exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-dir existed")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote directory-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating directory-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later directory residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-dir after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-dir in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating or deleting branch-dir")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-dir without recreating it")
	}
}

func evaluateDeleteResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	deleteCount := 0
	presenceChecks := 0
	absenceChecks := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	sawForbiddenRead := false

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) {
			createCount++
		}
		if commandDeletesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) {
			deleteCount++
		}
		if commandVerifiesDeleteResiduePresence(command) {
			presenceChecks++
		}
		if commandVerifiesDeleteResidueAbsence(command) {
			absenceChecks++
		}
		if commandWritesWorkspaceFile(command, TargetDeleteResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandUsesForbiddenDeleteResidueRead(command) {
			sawForbiddenRead = true
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDeleteResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-delete-note.txt exactly once")
	requireExactCount(result, deleteCount, 1, "initial branch deleted branch-delete-note.txt exactly once")
	requireAtLeastOne(result, presenceChecks, "initial branch verified that branch-delete-note.txt existed before deletion")
	requireAtLeastOne(result, absenceChecks, "initial branch verified that branch-delete-note.txt was absent after deletion")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote delete-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating delete-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later delete residue witness")
	}
	if sawForbiddenRead {
		appendTargetTaskViolation(result, "initial branch avoided cat/head/tail/echo -n when checking branch-delete-note.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch used stable observation commands for branch-delete-note.txt")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating, deleting, or modifying branch-delete-note.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-delete-note.txt without mutating it")
	}
}

func evaluateSymlinkResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceSymlink(command, TargetSymlinkResidueLinkArtifact) {
			createCount++
		}
		if looksLikeSymlinkResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetSymlinkResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetSymlinkResidueLinkArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetSymlinkResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceSymlink(command, TargetSymlinkResidueLinkArtifact) ||
			commandDeletesWorkspaceFile(command, TargetSymlinkResidueLinkArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-link.txt exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-link.txt pointed to target-prompt.txt")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote symlink-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating symlink-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later symlink residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-link.txt after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-link.txt in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating or deleting branch-link.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-link.txt without recreating it")
	}
}

func evaluateRenameResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	renameCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historySourceDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetRenameResidueSourceArtifact) {
			createCount++
		}
		if commandRenamesWorkspaceFile(command, TargetRenameResidueSourceArtifact, TargetRenameResidueDestArtifact) {
			renameCount++
		}
		if looksLikeRenameResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetRenameResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetRenameResidueSourceArtifact) {
			historySourceDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetRenameResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, TargetRenameResidueSourceArtifact) ||
			commandWritesWorkspaceFile(command, TargetRenameResidueDestArtifact) ||
			commandDeletesWorkspaceFile(command, TargetRenameResidueSourceArtifact) ||
			commandDeletesWorkspaceFile(command, TargetRenameResidueDestArtifact) ||
			commandRenamesWorkspaceFile(command, TargetRenameResidueSourceArtifact, TargetRenameResidueDestArtifact) ||
			commandRenamesWorkspaceFile(command, TargetRenameResidueDestArtifact, TargetRenameResidueSourceArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-rename-src.txt exactly once")
	requireExactCount(result, renameCount, 1, "initial branch renamed branch-rename-src.txt to branch-rename-dst.txt exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-rename-dst.txt existed after the rename")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote rename-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating rename-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later rename residue witness")
	}
	if historySourceDeletes > renameCount {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-rename-src.txt outside the requested rename")
	} else {
		appendTargetTaskEvidence(result, "initial branch only removed branch-rename-src.txt through the requested rename")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating, deleting, or renaming branch-rename-src.txt and branch-rename-dst.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed which rename-side file already existed")
	}
}

func evaluateModeResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyInitialCount := 0
	chmodCount := 0
	verifyTightenedCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetModeResidueNoteArtifact) {
			createCount++
		}
		if looksLikeModeResidueVerification(command) && strings.Contains(output, "644") {
			verifyInitialCount++
		}
		if commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, targetModeResidueTightenedMode) {
			chmodCount++
		}
		if looksLikeModeResidueVerification(command) && strings.Contains(output, targetModeResidueTightenedMode) {
			verifyTightenedCount++
		}
		if commandWritesWorkspaceFile(command, TargetModeResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetModeResidueNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetModeResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, TargetModeResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetModeResidueNoteArtifact) ||
			commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, targetModeResidueTightenedMode) ||
			commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, "644") {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-mode-note.txt exactly once")
	requireAtLeastOne(result, verifyInitialCount, "initial branch verified that branch-mode-note.txt started at mode 0644")
	requireExactCount(result, chmodCount, 1, "initial branch changed branch-mode-note.txt to mode "+targetModeResidueTightenedMode+" exactly once")
	requireAtLeastOne(result, verifyTightenedCount, "initial branch verified that branch-mode-note.txt ended at mode "+targetModeResidueTightenedMode)
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote mode-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating mode-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later mode residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-mode-note.txt after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-mode-note.txt in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided rewriting, deleting, or chmod-ing branch-mode-note.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed the existing mode of branch-mode-note.txt")
	}
}

func evaluateAppendResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	appendCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetAppendResidueNoteArtifact) && !commandAppendsWorkspaceFile(command, TargetAppendResidueNoteArtifact) {
			createCount++
		}
		if commandAppendsWorkspaceFile(command, TargetAppendResidueNoteArtifact) {
			appendCount++
		}
		if looksLikeAppendResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetAppendResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetAppendResidueNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetAppendResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, TargetAppendResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetAppendResidueNoteArtifact) ||
			commandAppendsWorkspaceFile(command, TargetAppendResidueNoteArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-append-note.txt exactly once")
	requireExactCount(result, appendCount, 1, "initial branch appended the extra marker to branch-append-note.txt exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-append-note.txt contained both markers")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote append-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating append-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later append residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-append-note.txt after appending to it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-append-note.txt in place after appending to it")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided truncating, deleting, or appending to branch-append-note.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed the existing contents of branch-append-note.txt")
	}
}

func evaluateHardlinkResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceHardlink(command, TargetHardlinkResidueLinkArtifact) {
			createCount++
		}
		if looksLikeHardlinkResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetHardlinkResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetHardlinkResidueLinkArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetHardlinkResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceHardlink(command, TargetHardlinkResidueLinkArtifact) ||
			commandDeletesWorkspaceFile(command, TargetHardlinkResidueLinkArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-hardlink.txt exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-hardlink.txt existed as a hardlink")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote hardlink-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating hardlink-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later hardlink residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-hardlink.txt after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-hardlink.txt in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating or deleting branch-hardlink.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-hardlink.txt without recreating it")
	}
}

func evaluateFIFOResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceFIFO(command, TargetFIFOResiduePipeArtifact) {
			createCount++
		}
		if looksLikeFIFOResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetFIFOResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetFIFOResiduePipeArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetFIFOResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceFIFO(command, TargetFIFOResiduePipeArtifact) ||
			commandDeletesWorkspaceFile(command, TargetFIFOResiduePipeArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-fifo exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-fifo existed as a named pipe")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote fifo-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating fifo-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later fifo residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-fifo after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-fifo in place after creation")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided recreating or deleting branch-fifo")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up observed branch-fifo without recreating it")
	}
}

func evaluateOpenFDResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	openCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetOpenFDResidueNoteArtifact) {
			createCount++
		}
		if commandOpensWorkspaceFD(command, TargetOpenFDResidueNoteArtifact) {
			openCount++
		}
		if looksLikeOpenFDResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetOpenFDResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetOpenFDResidueNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetOpenFDResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandOpensWorkspaceFD(command, TargetOpenFDResidueNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetOpenFDResiduePIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetOpenFDResidueNoteArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-fd-note.txt exactly once")
	requireExactCount(result, openCount, 1, "initial branch launched the branch-fd-note.txt fd holder exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-fd-note.txt was still reachable through fd 9")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote open-fd-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating open-fd-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later open-fd residue witness")
	}
	if historyDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-fd-note.txt after creating it")
	} else {
		appendTargetTaskEvidence(result, "initial branch kept branch-fd-note.txt in place after opening it")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or modifying the branch-fd-note.txt holder")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed the existing branch-fd-note.txt holder")
	}
}

func evaluateDeletedOpenFDResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	openCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			createCount++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) {
			openCount++
		}
		if looksLikeDeletedOpenFDResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetDeletedOpenFDForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDeletedOpenFDForkArtifact) {
			summaryWitnessWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandOpensWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetDeletedOpenFDPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-deleted-fd-note.txt exactly once")
	requireExactCount(result, openCount, 1, "initial branch launched the deleted branch-deleted-fd-note.txt holder exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-deleted-fd-note.txt was still reachable through a deleted fd 9")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote deleted-open-fd-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating deleted-open-fd-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later deleted-open-fd residue witness")
	}
	if historyDeletes == 0 {
		appendTargetTaskViolation(result, "initial branch deleted branch-deleted-fd-note.txt exactly once through the fd-holder launch")
	} else {
		appendTargetTaskEvidence(result, "initial branch deleted branch-deleted-fd-note.txt while keeping the inode open")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or modifying the deleted branch-deleted-fd-note.txt holder")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed the existing deleted branch-deleted-fd-note.txt holder")
	}
}

func evaluateInheritedFDLeakTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	createCount := 0
	openCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0
	forbiddenInitialInspections := map[string]bool{}

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		for _, token := range []string{"ps", "ls", "grep", "pstree", "jobs", "stat", "find"} {
			if commandContainsWord(command, token) {
				forbiddenInitialInspections[token] = true
			}
		}
		if commandWritesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) {
			createCount++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) {
			openCount++
		}
		if looksLikeInheritedFDLeakVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetInheritedFDLeakForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetInheritedFDLeakForkArtifact) || inheritedFDLeakFollowupProducedWitness(call) {
			summaryWitnessWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandOpensWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, createCount, 1, "initial branch created branch-inherited-fd-secret.txt exactly once")
	requireExactCount(result, openCount, 1, "initial branch launched the inherited-fd holder exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-inherited-fd-secret.txt was still reachable through a deleted fd 9")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote inherited-fd-branch-leakage-check.txt exactly once")
	for _, token := range []string{"ps", "ls", "grep", "pstree", "jobs", "stat", "find"} {
		if forbiddenInitialInspections[token] {
			appendTargetTaskViolation(result, "initial branch avoided extra process/file inspection command: "+token)
		}
	}
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating inherited-fd-branch-leakage-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later inherited-fd leakage witness")
	}
	if historyDeletes == 0 {
		appendTargetTaskViolation(result, "initial branch deleted branch-inherited-fd-secret.txt exactly once through the fd-holder launch")
	} else {
		appendTargetTaskEvidence(result, "initial branch deleted branch-inherited-fd-secret.txt while keeping the inode open")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or recreating the inherited-fd branch secret")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only observed the existing inherited-fd branch secret")
	}
}

func inheritedFDLeakFollowupProducedWitness(call langgraphShellCall) bool {
	command := normalizeShellCommand(call.Command)
	if !strings.Contains(command, "inherited-fd-branch-leakage-check") {
		return false
	}
	output := strings.TrimSpace(call.Output)
	return outputShowsInheritedFDLeakageMarker(output) || outputShowsMissingInheritedFDLeakage(output)
}

func evaluateUnixListenerResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	launchCount := 0
	verifyCount := 0
	historyWitnessWrites := 0
	summaryWitnessWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandLaunchesUnixListener(command) {
			launchCount++
		}
		if looksLikeUnixListenerResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetUnixListenerForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetUnixListenerForkArtifact) || unixListenerFollowupProducedWitness(call) {
			summaryWitnessWrites++
		}
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, launchCount, 1, "initial branch launched the Unix listener exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-listener.sock returned the expected response")
	requireExactCount(result, summaryWitnessWrites, 1, "fork follow-up wrote unix-listener-residue-fork-check.txt exactly once")
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating unix-listener-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later Unix listener residue witness")
	}
	if historyDeletes > 1 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-listener.sock outside listener setup")
	} else {
		appendTargetTaskEvidence(result, "initial branch only touched branch-listener.sock during listener setup")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or recreating the Unix listener")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only connected to the existing Unix listener")
	}
}

func evaluateDiscardedServerTrustedClientTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	launchCount := 0
	verifyCount := 0
	historyCheckWrites := 0
	historyResponseWrites := 0
	summaryCheckWrites := 0
	summaryResponseWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandLaunchesUnixListener(command) {
			launchCount++
		}
		if looksLikeDiscardedServerTrustedClientVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetDiscardedServerTrustedClientArtifact) {
			historyCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetTrustedClientResponseArtifact) {
			historyResponseWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetDiscardedServerTrustedClientArtifact) || discardedServerTrustedClientFollowupProducedWitness(call) {
			summaryCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetTrustedClientResponseArtifact) {
			summaryResponseWrites++
		}
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, launchCount, 1, "initial branch launched the Unix listener exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-listener.sock returned the trusted-client marker")
	if summaryCheckWrites == 0 {
		appendTargetTaskViolation(result, "fork follow-up wrote discarded-server-trusted-client-check.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote discarded-server-trusted-client-check.txt")
	}
	if summaryResponseWrites == 0 {
		appendTargetTaskViolation(result, "fork follow-up wrote trusted-client-response.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote trusted-client-response.txt")
	}
	if historyCheckWrites > 0 || historyResponseWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating the later trusted-client witness artifacts")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later trusted-client witness artifacts")
	}
	if historyDeletes > 1 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-listener.sock outside listener setup")
	} else {
		appendTargetTaskEvidence(result, "initial branch only touched branch-listener.sock during listener setup")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or recreating the Unix listener before the trusted-client observation")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only consumed the existing Unix listener response")
	}
}

func evaluateSocketResponsePoisoningTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	launchCount := 0
	verifyCount := 0
	historyCheckWrites := 0
	historyCacheWrites := 0
	summaryCheckWrites := 0
	summaryCacheWrites := 0
	summaryMutations := 0
	historyDeletes := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandLaunchesUnixListener(command) {
			launchCount++
		}
		if looksLikeSocketResponsePoisoningVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetSocketResponsePoisoningArtifact) {
			historyCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetTrustedClientCacheArtifact) {
			historyCacheWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetSocketResponsePoisoningArtifact) || socketResponsePoisoningFollowupProducedWitness(call) {
			summaryCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetTrustedClientCacheArtifact) {
			summaryCacheWrites++
		}
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			summaryMutations++
		}
	}

	requireExactCount(result, launchCount, 1, "initial branch launched the Unix listener exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-listener.sock returned the response-poisoning marker")
	if summaryCheckWrites == 0 {
		appendTargetTaskViolation(result, "fork follow-up wrote socket-response-poisoning-check.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote socket-response-poisoning-check.txt")
	}
	if summaryCacheWrites == 0 {
		appendTargetTaskViolation(result, "fork follow-up wrote trusted-client-cache.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote trusted-client-cache.txt")
	}
	if historyCheckWrites > 0 || historyCacheWrites > 0 {
		appendTargetTaskViolation(result, "initial branch avoided creating the later cache witness artifacts")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later cache witness artifacts")
	}
	if historyDeletes > 1 {
		appendTargetTaskViolation(result, "initial branch avoided deleting branch-listener.sock outside listener setup")
	} else {
		appendTargetTaskEvidence(result, "initial branch only touched branch-listener.sock during listener setup")
	}
	if summaryMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up avoided relaunching or recreating the Unix listener before the cache observation")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only cached the existing Unix listener response")
	}
}

func evaluateCWDResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	mkdirCount := 0
	cdCount := 0
	verifyCount := 0
	checkWriteCount := 0
	witnessWriteCount := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceDirectory(command, TargetCWDResidueDirArtifact) {
			mkdirCount++
		}
		if commandChangesWorkingDirectory(command, TargetCWDResidueDirArtifact) {
			cdCount++
		}
		if looksLikeCWDResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetCWDResidueForkArtifact) {
			checkWriteCount++
		}
		if commandWritesWorkspaceFile(command, TargetCWDResidueWitnessArtifact) {
			witnessWriteCount++
		}

	}

	requireExactCount(result, mkdirCount, 1, "initial branch created branch-cwd-dir exactly once")
	requireExactCount(result, cdCount, 1, "initial branch changed to branch-cwd-dir exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified that branch-cwd-dir returned the expected response")
	if checkWriteCount > 0 {
		appendTargetTaskViolation(result, "initial branch wrote cwd-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create cwd-residue-fork-check.txt")
	}

	if witnessWriteCount > 0 {
		appendTargetTaskViolation(result, "initial branch wrote cwd-relative-witness.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create cwd-relative-witness.txt")
	}

	cdCount = 0 // reuse cdCount to count fork follow-up changes
	checkWriteCount = 0
	witnessWriteCount = 0

	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandChangesWorkingDirectory(command, "") {
			cdCount++
		}
		if commandWritesWorkspaceFile(command, TargetCWDResidueForkArtifact) {
			checkWriteCount++
		}
		if commandWritesWorkspaceFile(command, TargetCWDResidueWitnessArtifact) {
			witnessWriteCount++
		}

	}

	requireExactCount(result, cdCount, 0, "fork follow-up did not change cwd")

	if checkWriteCount == 0 {
		appendTargetTaskViolation(result, "fork follow-up did not create cwd-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote cwd-residue-fork-check.txt")
	}

	if witnessWriteCount == 0 {
		appendTargetTaskViolation(result, "fork follow-up did not create cwd-relative-witness.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up wrote cwd-relative-witness.txt")
	}

}

func evaluateUmaskResidueForkTaskCompliance(result *TargetTaskComplianceResult, historyCalls []langgraphShellCall, forkCalls []langgraphShellCall) {
	baselineWriteCount := 0
	umaskChangeCount := 0
	umask077Count := 0
	verifyCount := 0
	historyForkCheckWrites := 0
	historyWitnessWrites := 0
	historyBaselineDeletes := 0
	forkForkCheckWrites := 0
	forkWitnessWrites := 0
	forkUmaskMutations := 0
	forkBaselineMutations := 0

	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact) {
			baselineWriteCount++
		}
		if commandChangesUmask(command, "") {
			umaskChangeCount++
		}
		if commandChangesUmask(command, "077") {
			umask077Count++
		}
		if commandPrintsCurrentUmask(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueForkArtifact) {
			historyForkCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueWitnessArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact) {
			historyBaselineDeletes++
		}
	}

	requireExactCount(result, baselineWriteCount, 1, "initial branch recorded baseline-umask.txt exactly once")
	requireExactCount(result, umaskChangeCount, 1, "initial branch changed the shell umask exactly once")
	requireExactCount(result, umask077Count, 1, "initial branch changed the shell umask to 077 exactly once")
	requireAtLeastOne(result, verifyCount, "initial branch verified the current shell umask by printing umask")
	if historyForkCheckWrites > 0 {
		appendTargetTaskViolation(result, "initial branch created umask-residue-fork-check.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later umask residue fork check")
	}
	if historyWitnessWrites > 0 {
		appendTargetTaskViolation(result, "initial branch created umask-witness.txt")
	} else {
		appendTargetTaskEvidence(result, "initial branch did not create the later umask witness")
	}
	if historyBaselineDeletes > 0 {
		appendTargetTaskViolation(result, "initial branch deleted baseline-umask.txt after recording it")
	} else {
		appendTargetTaskEvidence(result, "initial branch preserved baseline-umask.txt after recording it")
	}

	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueForkArtifact) {
			forkForkCheckWrites++
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueWitnessArtifact) {
			forkWitnessWrites++
		}
		if commandChangesUmask(command, "") {
			forkUmaskMutations++
		}
		if commandWritesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact) {
			forkBaselineMutations++
		}
	}

	requireExactCount(result, forkForkCheckWrites, 1, "fork follow-up wrote umask-residue-fork-check.txt exactly once")
	requireExactCount(result, forkWitnessWrites, 1, "fork follow-up wrote umask-witness.txt exactly once")
	if forkUmaskMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up changed the shell umask")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up did not change the shell umask")
	}
	if forkBaselineMutations > 0 {
		appendTargetTaskViolation(result, "fork follow-up modified baseline-umask.txt")
	} else {
		appendTargetTaskEvidence(result, "fork follow-up only read baseline-umask.txt")
	}
}

func unixListenerFollowupProducedWitness(call langgraphShellCall) bool {
	command := normalizeShellCommand(call.Command)
	output := strings.TrimSpace(call.Output)
	if !outputShowsUnixListenerResidueMarker(output) && !outputShowsMissingUnixListenerResidue(output) {
		return false
	}
	return command == "" ||
		strings.Contains(command, "unix-listener-residue-fork-check") ||
		looksLikeUnixListenerResidueVerification(command)
}

func replayUnixListenerWitnessProduced(call langgraphShellCall) bool {
	command := normalizeShellCommand(call.Command)
	output := strings.TrimSpace(call.Output)
	if !outputShowsUnixListenerResidueMarker(output) && !outputShowsMissingUnixListenerResidue(output) {
		return false
	}
	return command == "" ||
		strings.Contains(command, TargetUnixListenerReplayArtifact) ||
		looksLikeUnixListenerResidueVerification(command)
}

func discardedServerTrustedClientFollowupProducedWitness(call langgraphShellCall) bool {
	command := normalizeShellCommand(call.Command)
	output := strings.TrimSpace(call.Output)
	if !outputShowsDiscardedServerTrustedClientMarker(output) && !outputShowsMissingDiscardedServerTrustedClient(output) {
		return false
	}
	return command == "" ||
		strings.Contains(command, TargetDiscardedServerTrustedClientArtifact) ||
		looksLikeDiscardedServerTrustedClientVerification(command)
}

func socketResponsePoisoningFollowupProducedWitness(call langgraphShellCall) bool {
	command := normalizeShellCommand(call.Command)
	output := strings.TrimSpace(call.Output)
	if !outputShowsSocketResponsePoisoningMarker(output) && !outputShowsMissingSocketResponsePoisoning(output) {
		return false
	}
	return command == "" ||
		strings.Contains(command, TargetSocketResponsePoisoningArtifact) ||
		looksLikeSocketResponsePoisoningVerification(command)
}

func commandPrintsCurrentUmask(command string) bool {
	command = normalizeShellCommand(command)
	fields := strings.Fields(command)
	for i := 0; i < len(fields); i++ {
		if trimShellCommandToken(fields[i]) != "umask" {
			continue
		}
		if i > 0 && !shellTokenStartsCommand(fields[i-1]) {
			continue
		}
		if i == len(fields)-1 || shellTokenStartsCommand(fields[i+1]) {
			return true
		}
	}
	return false
}

func finalizeTargetTaskCompliance(result TargetTaskComplianceResult) TargetTaskComplianceResult {
	switch {
	case len(result.Violations) > 0:
		result.Status = TargetTaskComplianceStatusViolated
	case result.Status == TargetTaskComplianceStatusUnknown:
		result.Status = TargetTaskComplianceStatusCompliant
	case result.Status == "":
		result.Status = TargetTaskComplianceStatusUnknown
	}
	return result
}

func appendTargetTaskEvidence(result *TargetTaskComplianceResult, item string) {
	if item == "" || ContainsString(result.Evidence, item) {
		return
	}
	result.Evidence = append(result.Evidence, item)
}

func appendTargetTaskViolation(result *TargetTaskComplianceResult, item string) {
	if item == "" || ContainsString(result.Violations, item) {
		return
	}
	result.Violations = append(result.Violations, item)
}

func loadLangGraphOperationShellCalls(workspace string, artifact string) (*langgraphOperationSummary, []langgraphShellCall, bool, error) {
	summary, err := loadLangGraphOperationSummary(workspace, artifact)
	if err != nil {
		return nil, nil, false, err
	}
	if summary == nil {
		return nil, nil, false, nil
	}
	return summary, operationShellCallsWithLifecycle(workspace, summary), true, nil
}

func loadPrimaryLangGraphShellCalls(workspace string) ([]langgraphShellCall, bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return nil, false, err
	}
	historyCalls, historyOK := primaryShellCallsFromHistory(checkpoints)

	lifecycleCalls, lifecycleOK, err := loadLangGraphLifecycleShellCalls(workspace, "initial_run")
	if err != nil {
		return nil, false, err
	}
	if lifecycleOK && len(lifecycleCalls) > 0 {
		return attachShellCallOutputs(lifecycleCalls, collectShellCallsFromHistory(checkpoints)), true, nil
	}
	if historyOK {
		return historyCalls, true, nil
	}
	return nil, false, nil
}

func primaryShellCallsFromHistory(checkpoints []langgraphHistoryCheckpoint) ([]langgraphShellCall, bool) {
	var best []langgraphShellCall
	bestMessageCount := -1
	for _, checkpoint := range checkpoints {
		calls := buildLangGraphShellCalls(checkpoint.Messages)
		if len(calls) == 0 && len(best) > 0 {
			continue
		}
		if len(calls) > len(best) || (len(calls) == len(best) && len(checkpoint.Messages) > bestMessageCount) {
			best = calls
			bestMessageCount = len(checkpoint.Messages)
		}
	}
	if len(best) == 0 {
		return nil, len(checkpoints) > 0
	}
	return best, true
}

func collectShellCallsFromHistory(checkpoints []langgraphHistoryCheckpoint) []langgraphShellCall {
	var calls []langgraphShellCall
	seen := map[string]bool{}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			key := call.Command + "\x00" + call.Output
			if seen[key] {
				continue
			}
			seen[key] = true
			calls = append(calls, call)
		}
	}
	return calls
}

func attachShellCallOutputs(calls []langgraphShellCall, outputSource []langgraphShellCall) []langgraphShellCall {
	outputs := map[string][]string{}
	for _, call := range outputSource {
		if strings.TrimSpace(call.Output) == "" {
			continue
		}
		outputs[call.Command] = append(outputs[call.Command], call.Output)
	}
	merged := append([]langgraphShellCall(nil), calls...)
	for i, call := range merged {
		queue := outputs[call.Command]
		if len(queue) == 0 {
			continue
		}
		merged[i].Output = queue[0]
		outputs[call.Command] = queue[1:]
	}
	return merged
}

type langGraphLifecycleData struct {
	Events []langGraphLifecycleEvent `json:"events"`
}

type langGraphLifecycleEvent struct {
	Event          string `json:"event"`
	Operation      string `json:"operation"`
	CommandPreview string `json:"command_preview"`
}

type mafLifecycleData struct {
	Events []mafLifecycleEvent `json:"events"`
}

type mafLifecycleEvent struct {
	Event   string                   `json:"event"`
	Details *mafLifecycleEventDetail `json:"details,omitempty"`
}

type mafLifecycleEventDetail struct {
	HookInput *mafHookInput `json:"hook_input,omitempty"`
}

type mafHookInput struct {
	ToolName         string `json:"toolName"`
	ToolArgs         string `json:"toolArgs"`
	WorkingDirectory string `json:"workingDirectory"`
}

type mafBashToolArgs struct {
	Command string `json:"command"`
	Mode    string `json:"mode"`
}

type mafShellCall struct {
	Command string
	Mode    string
}

func loadLangGraphLifecycleShellCalls(workspace string, operation string) ([]langgraphShellCall, bool, error) {
	path := filepath.Join(workspace, langgraphLifecycleArtifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", langgraphLifecycleArtifact, err)
	}

	var lifecycle langGraphLifecycleData
	if err := json.Unmarshal(raw, &lifecycle); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", langgraphLifecycleArtifact, err)
	}

	var calls []langgraphShellCall
	for _, event := range lifecycle.Events {
		if event.Event != "shell_command_started" {
			continue
		}
		if operation != "" && event.Operation != operation {
			continue
		}
		command := strings.TrimSpace(event.CommandPreview)
		if command == "" {
			continue
		}
		calls = append(calls, langgraphShellCall{Command: command})
	}
	return calls, true, nil
}

func loadMAFShellCalls(workspace string) ([]mafShellCall, bool, error) {
	path := filepath.Join(workspace, mafLifecycleArtifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", mafLifecycleArtifact, err)
	}

	events, err := decodeMAFLifecycleEvents(raw)
	if err != nil {
		return nil, true, fmt.Errorf("decode %s: %w", mafLifecycleArtifact, err)
	}

	var calls []mafShellCall
	for _, event := range events {
		if event.Event != "pre_tool_use" || event.Details == nil || event.Details.HookInput == nil {
			continue
		}
		hook := event.Details.HookInput
		if strings.TrimSpace(strings.ToLower(hook.ToolName)) != "bash" {
			continue
		}
		var args mafBashToolArgs
		rawArgs := strings.TrimSpace(hook.ToolArgs)
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
				return nil, true, fmt.Errorf("decode %s bash tool args: %w", mafLifecycleArtifact, err)
			}
		}
		command := strings.TrimSpace(args.Command)
		if command == "" {
			continue
		}
		calls = append(calls, mafShellCall{
			Command: command,
			Mode:    strings.TrimSpace(strings.ToLower(args.Mode)),
		})
	}
	return calls, true, nil
}

func decodeMAFLifecycleEvents(raw []byte) ([]mafLifecycleEvent, error) {
	var payload mafLifecycleData
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Events != nil {
		return payload.Events, nil
	}

	var events []mafLifecycleEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func requireExactCount(result *TargetTaskComplianceResult, observed int, expected int, requirement string) {
	switch {
	case observed == expected:
		appendTargetTaskEvidence(result, requirement)
	case observed == 0:
		appendTargetTaskViolation(result, requirement)
	default:
		appendTargetTaskViolation(result, fmt.Sprintf("%s (observed %d times)", requirement, observed))
	}
}

func requireAtLeastOne(result *TargetTaskComplianceResult, observed int, requirement string) {
	if observed > 0 {
		appendTargetTaskEvidence(result, requirement)
		return
	}
	appendTargetTaskViolation(result, requirement)
}

func evaluateMAFDelayedLateEffectCall(result *TargetTaskComplianceResult, call mafShellCall, asyncRequirement string, commandRequirement string, expectedDelayFragment string) {
	command := normalizeShellCommand(call.Command)
	if mafShellCallIsAsync(call) {
		appendTargetTaskEvidence(result, asyncRequirement)
	} else {
		appendTargetTaskViolation(result, asyncRequirement)
	}

	hasDelay := strings.Contains(command, "sleep ")
	if expectedDelayFragment != "" {
		hasDelay = strings.Contains(command, expectedDelayFragment)
	}
	if hasDelay && strings.Contains(command, "touch late-effect") {
		appendTargetTaskEvidence(result, commandRequirement)
	} else {
		appendTargetTaskViolation(result, commandRequirement)
	}
}

func evaluateMAFForbiddenInspectionCompliance(result *TargetTaskComplianceResult, calls []mafShellCall) {
	forbidden := []string{"pwd", "ls", "ps", "jobs", "grep", "stat", "cat", "wait"}
	for _, call := range calls {
		command := normalizeShellCommand(call.Command)
		for _, token := range forbidden {
			if commandContainsWord(command, token) {
				appendTargetTaskViolation(result, "task avoided forbidden command: "+token)
			}
		}
	}
}

func mafShellCallIsAsync(call mafShellCall) bool {
	command := normalizeShellCommand(call.Command)
	return call.Mode == "async" ||
		strings.Contains(command, " &") ||
		strings.HasSuffix(command, "&") ||
		strings.Contains(command, "nohup ")
}

func commandVerifiesDeleteResiduePresence(command string) bool {
	command = normalizeShellCommand(command)
	return strings.Contains(command, TargetDeleteResidueNoteArtifact) &&
		(strings.Contains(command, "od -c") || strings.Contains(command, "ls -l"))
}

func commandVerifiesDeleteResidueAbsence(command string) bool {
	command = normalizeShellCommand(command)
	return strings.Contains(command, TargetDeleteResidueNoteArtifact) &&
		strings.Contains(command, "exit_code=$?")
}

func commandUsesForbiddenDeleteResidueRead(command string) bool {
	command = normalizeShellCommand(command)
	if !strings.Contains(command, TargetDeleteResidueNoteArtifact) {
		return false
	}
	return strings.Contains(command, "echo -n") ||
		strings.Contains(command, "cat "+TargetDeleteResidueNoteArtifact) ||
		strings.Contains(command, "head ") ||
		strings.Contains(command, "tail ")
}

func commandContainsWord(command string, word string) bool {
	command = normalizeShellCommand(command)
	word = strings.ToLower(strings.TrimSpace(word))
	for _, token := range strings.Fields(command) {
		if token == word {
			return true
		}
	}
	return false
}
