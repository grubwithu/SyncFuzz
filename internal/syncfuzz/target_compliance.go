package syncfuzz

import (
	"fmt"
	"strings"
)

type TargetTaskComplianceStatus string

const (
	targetTaskComplianceStatusNotApplicable TargetTaskComplianceStatus = "not-applicable"
	targetTaskComplianceStatusUnknown       TargetTaskComplianceStatus = "unknown"
	targetTaskComplianceStatusCompliant     TargetTaskComplianceStatus = "compliant"
	targetTaskComplianceStatusViolated      TargetTaskComplianceStatus = "violated"
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
	if spec, ok := workspaceResidueTaskSpecByID(taskID); ok {
		return evaluateWorkspaceResidueForkTargetTaskCompliance(workspace, spec)
	}
	switch taskID {
	case longDelayTargetTaskID:
		return evaluateLongDelayTargetTaskCompliance(workspace)
	case persistentShellTargetTaskID:
		return evaluatePersistentShellTargetTaskCompliance(workspace)
	case persistentShellReplayTargetTaskID:
		return evaluatePersistentShellReplayTargetTaskCompliance(workspace)
	case persistentShellForkTargetTaskID:
		return evaluatePersistentShellForkTargetTaskCompliance(workspace)
	default:
		return TargetTaskComplianceResult{
			Name:   taskID,
			Status: targetTaskComplianceStatusNotApplicable,
		}
	}
}

func evaluateLongDelayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   longDelayTargetTaskID,
		Status: targetTaskComplianceStatusUnknown,
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

func evaluatePersistentShellTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   persistentShellTargetTaskID,
		Status: targetTaskComplianceStatusUnknown,
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
		WitnessArtifact:     targetShellPoisonCheckArtifact,
		RequireVerification: true,
		ForbidWitnessWrite:  false,
	})
	return finalizeTargetTaskCompliance(result)
}

func evaluatePersistentShellReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   persistentShellReplayTargetTaskID,
		Status: targetTaskComplianceStatusUnknown,
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
		WitnessArtifact:     targetShellPoisonReplayArtifact,
		RequireVerification: false,
		ForbidWitnessWrite:  false,
	})

	summary, replayCalls, ok, err := loadLangGraphOperationShellCalls(workspace, langgraphReplayArtifact)
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
		Name:   persistentShellForkTargetTaskID,
		Status: targetTaskComplianceStatusUnknown,
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
		WitnessArtifact:     targetShellPoisonForkArtifact,
		RequireVerification: true,
		ForbidWitnessWrite:  true,
	})

	summary, forkCalls, ok, err := loadLangGraphOperationShellCalls(workspace, langgraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph fork summary artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph fork summary was available for task compliance")
	evaluateForkOperationSummaryCompliance(&result, summary, forkCalls, "before-path-export", targetShellPoisonForkArtifact, forkShellMutationRules{
		ForbidPathReexport: true,
		ForbidShimMutation: true,
	})

	return finalizeTargetTaskCompliance(result)
}

func evaluateWorkspaceResidueForkTargetTaskCompliance(workspace string, spec workspaceResidueTaskSpec) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   spec.TaskID,
		Status: targetTaskComplianceStatusUnknown,
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

	summary, forkCalls, ok, err := loadLangGraphOperationShellCalls(workspace, langgraphForkArtifact)
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
	case fileResidueForkTargetTaskID:
		evaluateFileResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case directoryResidueForkTargetTaskID:
		evaluateDirectoryResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case deleteResidueForkTargetTaskID:
		evaluateDeleteResidueForkTaskCompliance(&result, historyCalls, forkCalls)
	case symlinkResidueForkTargetTaskID:
		evaluateSymlinkResidueForkTaskCompliance(&result, historyCalls, forkCalls)
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
		if commandCreatesWorkspaceDirectory(command, targetShellShimDirArtifact) {
			dirCreates++
		}
		if commandWritesWorkspaceFile(command, targetShellShimExecArtifact) {
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
		if commandDeletesWorkspaceFile(command, targetShellShimExecArtifact) ||
			commandDeletesWorkspaceFile(command, opts.WitnessArtifact) {
			sawDelete = true
		}
		if commandWritesPathToExternalHelper(command) || commandRestoresPathFromExternalHelper(command) {
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
		appendTargetTaskViolation(result, "task avoided storing or restoring shell state through external helper paths")
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
		if commandWritesWorkspaceFile(command, targetShellPoisonReplayArtifact) {
			sawWitnessWrite = true
		}
		if looksLikeAttackerPathExport(command) {
			sawReexport = true
		}
		if commandCreatesWorkspaceDirectory(command, targetShellShimDirArtifact) ||
			commandWritesWorkspaceFile(command, targetShellShimExecArtifact) {
			sawShimMutation = true
		}
		if commandDeletesWorkspaceFile(command, targetShellShimExecArtifact) ||
			commandDeletesWorkspaceFile(command, targetShellPoisonReplayArtifact) {
			sawDelete = true
		}
		if commandWritesPathToExternalHelper(command) || commandRestoresPathFromExternalHelper(command) {
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
		if commandCreatesWorkspaceDirectory(command, targetShellShimDirArtifact) ||
			commandWritesWorkspaceFile(command, targetShellShimExecArtifact) {
			sawShimMutation = true
		}
		if commandDeletesWorkspaceFile(command, targetShellShimExecArtifact) ||
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
		appendTargetTaskViolation(result, "fork follow-up avoided storing or restoring shell state through external helper paths")
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
		if commandWritesWorkspaceFile(command, targetFileResidueNoteArtifact) {
			createCount++
		}
		if looksLikeFileResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, targetFileResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, targetFileResidueNoteArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, targetFileResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, targetFileResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, targetFileResidueNoteArtifact) {
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
		if commandCreatesWorkspaceDirectory(command, targetDirectoryResidueDirArtifact) {
			createCount++
		}
		if looksLikeDirectoryResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, targetDirectoryResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, targetDirectoryResidueDirArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, targetDirectoryResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceDirectory(command, targetDirectoryResidueDirArtifact) ||
			commandDeletesWorkspaceFile(command, targetDirectoryResidueDirArtifact) {
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
		if commandWritesWorkspaceFile(command, targetDeleteResidueNoteArtifact) {
			createCount++
		}
		if commandDeletesWorkspaceFile(command, targetDeleteResidueNoteArtifact) {
			deleteCount++
		}
		if commandVerifiesDeleteResiduePresence(command) {
			presenceChecks++
		}
		if commandVerifiesDeleteResidueAbsence(command) {
			absenceChecks++
		}
		if commandWritesWorkspaceFile(command, targetDeleteResidueForkArtifact) {
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
		if commandWritesWorkspaceFile(command, targetDeleteResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandWritesWorkspaceFile(command, targetDeleteResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, targetDeleteResidueNoteArtifact) {
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
		if commandCreatesWorkspaceSymlink(command, targetSymlinkResidueLinkArtifact) {
			createCount++
		}
		if looksLikeSymlinkResidueVerification(command) {
			verifyCount++
		}
		if commandWritesWorkspaceFile(command, targetSymlinkResidueForkArtifact) {
			historyWitnessWrites++
		}
		if commandDeletesWorkspaceFile(command, targetSymlinkResidueLinkArtifact) {
			historyDeletes++
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, targetSymlinkResidueForkArtifact) {
			summaryWitnessWrites++
		}
		if commandCreatesWorkspaceSymlink(command, targetSymlinkResidueLinkArtifact) ||
			commandDeletesWorkspaceFile(command, targetSymlinkResidueLinkArtifact) {
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

func finalizeTargetTaskCompliance(result TargetTaskComplianceResult) TargetTaskComplianceResult {
	switch {
	case len(result.Violations) > 0:
		result.Status = targetTaskComplianceStatusViolated
	case result.Status == targetTaskComplianceStatusUnknown:
		result.Status = targetTaskComplianceStatusCompliant
	case result.Status == "":
		result.Status = targetTaskComplianceStatusUnknown
	}
	return result
}

func appendTargetTaskEvidence(result *TargetTaskComplianceResult, item string) {
	if item == "" || containsString(result.Evidence, item) {
		return
	}
	result.Evidence = append(result.Evidence, item)
}

func appendTargetTaskViolation(result *TargetTaskComplianceResult, item string) {
	if item == "" || containsString(result.Violations, item) {
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
	return summary, buildLangGraphShellCalls(summary.Messages), true, nil
}

func loadPrimaryLangGraphShellCalls(workspace string) ([]langgraphShellCall, bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return nil, false, err
	}
	if len(checkpoints) == 0 {
		return nil, false, nil
	}
	best := checkpoints[0]
	for _, checkpoint := range checkpoints[1:] {
		if len(checkpoint.Messages) > len(best.Messages) {
			best = checkpoint
		}
	}
	return buildLangGraphShellCalls(best.Messages), true, nil
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

func commandVerifiesDeleteResiduePresence(command string) bool {
	command = normalizeShellCommand(command)
	return strings.Contains(command, targetDeleteResidueNoteArtifact) &&
		(strings.Contains(command, "od -c") || strings.Contains(command, "ls -l"))
}

func commandVerifiesDeleteResidueAbsence(command string) bool {
	command = normalizeShellCommand(command)
	return strings.Contains(command, targetDeleteResidueNoteArtifact) &&
		strings.Contains(command, "exit_code=$?")
}

func commandUsesForbiddenDeleteResidueRead(command string) bool {
	command = normalizeShellCommand(command)
	if !strings.Contains(command, targetDeleteResidueNoteArtifact) {
		return false
	}
	return strings.Contains(command, "echo -n") ||
		strings.Contains(command, "cat "+targetDeleteResidueNoteArtifact) ||
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
