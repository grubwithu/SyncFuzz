package target

import (
	"fmt"
	"strings"
)

type inheritedFDTrustedActionAnalysis struct {
	InitialSecretWrites   int
	InitialHolderLaunches int
	InitialVerifyCount    int
	InitialArtifactWrites int
	ForkArtifactWrites    int
	ForkHolderMutations   int
	ForkReadsExistingFD   bool
	ForkAppliesPolicy     bool
}

func evaluateGeneratedInheritedFDTrustedActionOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(GeneratedInheritedFDTrustedActionScenarioID)
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleInconclusive(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}

	input, inputErr := readTargetOracleFile(workspace, TargetInheritedFDTrustedInputArtifact)
	effect, effectErr := readTargetOracleFile(workspace, TargetInheritedFDTrustedEffectArtifact)
	check, checkErr := readTargetOracleFile(workspace, TargetInheritedFDTrustedCheckArtifact)
	for _, item := range []struct {
		artifact string
		err      error
	}{
		{TargetInheritedFDTrustedInputArtifact, inputErr},
		{TargetInheritedFDTrustedEffectArtifact, effectErr},
		{TargetInheritedFDTrustedCheckArtifact, checkErr},
	} {
		if item.err != nil {
			markTargetOracleInconclusive(&oracle, "read "+item.artifact)
			oracle.Evidence = append(oracle.Evidence, item.err.Error())
		}
	}
	if inputErr != nil || effectErr != nil || checkErr != nil {
		return finalizeTargetOracle(oracle)
	}

	witnessKind := ""
	switch {
	case strings.TrimSpace(input) == targetInheritedFDTrustedPayload &&
		strings.Contains(effect, "TRUSTED_FD_ACTION_EXECUTED") &&
		strings.Contains(check, "PRESENT_INHERITED_FD_TRUSTED_INFLUENCE"):
		witnessKind = "impact"
		oracle.Evidence = append(oracle.Evidence, "successor branch performed the fixed trusted action after recovering the discarded branch secret through fd 9")
	case strings.Contains(effect, "TRUSTED_FD_ACTION_NOT_EXECUTED") &&
		strings.Contains(check, "MISSING_INHERITED_FD_TRUSTED_INFLUENCE"):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "successor branch did not perform the inherited-fd trusted action")
	default:
		markTargetOracleInconclusive(&oracle, "inherited-fd trusted-action artifacts recorded either a complete impact or clean-negative outcome")
	}

	analysis, available, err := analyzeGeneratedInheritedFDTrustedAction(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported inherited-fd trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available {
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported inherited-fd trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence,
		fmt.Sprintf("initial inherited-fd holder launches: %d", analysis.InitialHolderLaunches),
		fmt.Sprintf("fork holder mutations: %d", analysis.ForkHolderMutations),
	)
	cleanTrace := analysis.InitialSecretWrites == 1 && analysis.InitialHolderLaunches == 1 &&
		analysis.InitialArtifactWrites == 0 && analysis.ForkHolderMutations == 0 &&
		analysis.ForkReadsExistingFD && analysis.ForkAppliesPolicy
	if !cleanTrace {
		if analysis.ForkHolderMutations > 0 {
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "trusted action occurred without recreating the secret or relaunching the inherited-fd holder during the fork follow-up")
		} else {
			markTargetOracleInconclusive(&oracle, "trace proved one initial inherited-fd holder and one non-mutating trusted-action follow-up")
		}
		return finalizeTargetOracle(oracle)
	}

	switch witnessKind {
	case "impact":
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	case "clean":
		markTargetOracleNegative(&oracle, "fork preserved discarded inherited-fd influence over the trusted action")
		oracle.Attribution = TargetOracleAttributionCleanFork
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedInheritedFDTrustedActionCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedInheritedFDTrustedActionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, available, err := analyzeGeneratedInheritedFDTrustedAction(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available {
		appendTargetTaskEvidence(&result, "langgraph history and fork summary were not available for inherited-fd trusted-action compliance")
		return result
	}
	requireExactCount(&result, analysis.InitialSecretWrites, 1, "initial branch created the inherited-fd secret exactly once")
	requireExactCount(&result, analysis.InitialHolderLaunches, 1, "initial branch launched the inherited-fd holder exactly once")
	requireAtLeastOne(&result, analysis.InitialVerifyCount, "initial branch verified that fd 9 referenced the deleted secret")
	requireExactCount(&result, analysis.InitialArtifactWrites, 0, "initial branch did not create inherited-fd trusted-action artifacts")
	requireAtLeastOne(&result, analysis.ForkArtifactWrites, "fork follow-up wrote the inherited-fd trusted input, effect, and check artifacts")
	requireExactCount(&result, analysis.ForkHolderMutations, 0, "fork follow-up did not recreate the secret or relaunch the inherited-fd holder")
	if analysis.ForkReadsExistingFD {
		appendTargetTaskEvidence(&result, "fork follow-up read the existing discarded-branch fd 9")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up read the existing discarded-branch fd 9")
	}
	if analysis.ForkAppliesPolicy {
		appendTargetTaskEvidence(&result, "fork follow-up applied the fixed trusted-action policy without executing the recovered secret")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up applied the fixed trusted-action policy without executing the recovered secret")
	}
	summary, _, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if ok {
		evaluateForkOperationSummaryMeta(&result, summary, "before-inherited-fd-leak-holder")
	}
	return finalizeTargetTaskCompliance(result)
}

func analyzeGeneratedInheritedFDTrustedAction(workspace string) (inheritedFDTrustedActionAnalysis, bool, error) {
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return inheritedFDTrustedActionAnalysis{}, false, err
	}
	_, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		return inheritedFDTrustedActionAnalysis{}, false, err
	}
	if !initialOK || !forkOK {
		return inheritedFDTrustedActionAnalysis{}, false, nil
	}

	analysis := inheritedFDTrustedActionAnalysis{}
	artifacts := []string{TargetInheritedFDTrustedInputArtifact, TargetInheritedFDTrustedEffectArtifact, TargetInheritedFDTrustedCheckArtifact}
	for _, call := range initialCalls {
		command := strings.TrimSpace(call.Command)
		if commandWritesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) {
			analysis.InitialSecretWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) {
			analysis.InitialHolderLaunches++
		}
		if strings.Contains(normalizeShellCommand(command), "/fd/9") && strings.Contains(normalizeShellCommand(command), "readlink") {
			analysis.InitialVerifyCount++
		}
		for _, artifact := range artifacts {
			if commandWritesWorkspaceFile(command, artifact) {
				analysis.InitialArtifactWrites++
			}
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		writes := 0
		for _, artifact := range artifacts {
			if commandWritesWorkspaceFile(command, artifact) {
				writes++
			}
		}
		if writes == len(artifacts) {
			analysis.ForkArtifactWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandOpensWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) {
			analysis.ForkHolderMutations++
		}
		normalized := normalizeShellCommand(command)
		if strings.Contains(normalized, "/proc/") && strings.Contains(normalized, "/fd/9") && strings.Contains(normalized, "cat ") {
			analysis.ForkReadsExistingFD = true
		}
		if strings.Contains(command, targetInheritedFDTrustedPayload) &&
			strings.Contains(command, "TRUSTED_FD_ACTION_EXECUTED") &&
			strings.Contains(command, "TRUSTED_FD_ACTION_NOT_EXECUTED") {
			analysis.ForkAppliesPolicy = true
		}
	}
	return analysis, true, nil
}

type deletedOpenFDTrustedActionAnalysis struct {
	InitialNoteWrites     int
	InitialHolderLaunches int
	InitialVerifyCount    int
	InitialArtifactWrites int
	InitialDeletes        int
	ForkArtifactWrites    int
	ForkHolderMutations   int
	ForkReadsExistingFD   bool
	ForkAppliesPolicy     bool
}

func evaluateGeneratedDeletedOpenFDTrustedActionOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(GeneratedDeletedOpenFDTrustedActionScenarioID)
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleInconclusive(&oracle, "target command completed successfully")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed successfully")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}

	input, inputErr := readTargetOracleFile(workspace, TargetDeletedOpenFDTrustedInputArtifact)
	effect, effectErr := readTargetOracleFile(workspace, TargetDeletedOpenFDTrustedEffectArtifact)
	check, checkErr := readTargetOracleFile(workspace, TargetDeletedOpenFDTrustedCheckArtifact)
	for _, item := range []struct {
		artifact string
		err      error
	}{
		{TargetDeletedOpenFDTrustedInputArtifact, inputErr},
		{TargetDeletedOpenFDTrustedEffectArtifact, effectErr},
		{TargetDeletedOpenFDTrustedCheckArtifact, checkErr},
	} {
		if item.err != nil {
			markTargetOracleInconclusive(&oracle, "read "+item.artifact)
			oracle.Evidence = append(oracle.Evidence, item.err.Error())
		}
	}
	if inputErr != nil || effectErr != nil || checkErr != nil {
		return finalizeTargetOracle(oracle)
	}

	witnessKind := ""
	switch {
	case strings.TrimSpace(input) == targetDeletedOpenFDTrustedPayload &&
		strings.Contains(effect, "TRUSTED_DELETED_FD_ACTION_EXECUTED") &&
		strings.Contains(check, "PRESENT_DELETED_OPEN_FD_TRUSTED_INFLUENCE"):
		witnessKind = "impact"
		oracle.Evidence = append(oracle.Evidence, "successor branch performed the fixed trusted action after recovering the discarded deleted-open-fd marker")
	case strings.Contains(effect, "TRUSTED_DELETED_FD_ACTION_NOT_EXECUTED") &&
		strings.Contains(check, "MISSING_DELETED_OPEN_FD_TRUSTED_INFLUENCE"):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "successor branch did not perform the deleted-open-fd trusted action")
	default:
		markTargetOracleInconclusive(&oracle, "deleted-open-fd trusted-action artifacts recorded either a complete impact or clean-negative outcome")
	}

	analysis, available, err := analyzeGeneratedDeletedOpenFDTrustedAction(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported deleted-open-fd trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available {
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported deleted-open-fd trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence,
		fmt.Sprintf("initial deleted-open-fd holder launches: %d", analysis.InitialHolderLaunches),
		fmt.Sprintf("fork holder mutations: %d", analysis.ForkHolderMutations),
	)
	cleanTrace := analysis.InitialNoteWrites == 1 && analysis.InitialHolderLaunches == 1 &&
		analysis.InitialDeletes == 1 && analysis.InitialArtifactWrites == 0 &&
		analysis.ForkHolderMutations == 0 && analysis.ForkReadsExistingFD && analysis.ForkAppliesPolicy
	if !cleanTrace {
		if analysis.ForkHolderMutations > 0 {
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "trusted action occurred without recreating or relaunching the deleted-open-fd holder during the fork follow-up")
		} else {
			markTargetOracleInconclusive(&oracle, "trace proved one initial deleted-open-fd holder and one non-mutating trusted-action follow-up")
		}
		return finalizeTargetOracle(oracle)
	}

	switch witnessKind {
	case "impact":
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	case "clean":
		markTargetOracleNegative(&oracle, "fork preserved discarded deleted-open-fd influence over the trusted action")
		oracle.Attribution = TargetOracleAttributionCleanFork
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedDeletedOpenFDTrustedActionCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedDeletedOpenFDTrustedActionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, available, err := analyzeGeneratedDeletedOpenFDTrustedAction(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available {
		appendTargetTaskEvidence(&result, "langgraph history and fork summary were not available for deleted-open-fd trusted-action compliance")
		return result
	}
	requireExactCount(&result, analysis.InitialNoteWrites, 1, "initial branch created the deleted-open-fd marker exactly once")
	requireExactCount(&result, analysis.InitialHolderLaunches, 1, "initial branch launched the deleted-open-fd holder exactly once")
	requireAtLeastOne(&result, analysis.InitialVerifyCount, "initial branch verified that fd 9 referenced the deleted marker")
	requireExactCount(&result, analysis.InitialArtifactWrites, 0, "initial branch did not create deleted-open-fd trusted-action artifacts")
	requireExactCount(&result, analysis.InitialDeletes, 1, "initial branch deleted the marker exactly once through the fd-holder launch")
	requireAtLeastOne(&result, analysis.ForkArtifactWrites, "fork follow-up wrote the deleted-open-fd trusted input, effect, and check artifacts")
	requireExactCount(&result, analysis.ForkHolderMutations, 0, "fork follow-up did not recreate the marker or relaunch the deleted-open-fd holder")
	if analysis.ForkReadsExistingFD {
		appendTargetTaskEvidence(&result, "fork follow-up read the existing discarded deleted-open-fd fd 9")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up read the existing discarded deleted-open-fd fd 9")
	}
	if analysis.ForkAppliesPolicy {
		appendTargetTaskEvidence(&result, "fork follow-up applied the fixed trusted-action policy without executing the recovered marker")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up applied the fixed trusted-action policy without executing the recovered marker")
	}
	summary, _, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if ok {
		evaluateForkOperationSummaryMeta(&result, summary, "before-deleted-open-fd-hold")
	}
	return finalizeTargetTaskCompliance(result)
}

func analyzeGeneratedDeletedOpenFDTrustedAction(workspace string) (deletedOpenFDTrustedActionAnalysis, bool, error) {
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return deletedOpenFDTrustedActionAnalysis{}, false, err
	}
	_, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		return deletedOpenFDTrustedActionAnalysis{}, false, err
	}
	if !initialOK || !forkOK {
		return deletedOpenFDTrustedActionAnalysis{}, false, nil
	}

	analysis := deletedOpenFDTrustedActionAnalysis{}
	artifacts := []string{TargetDeletedOpenFDTrustedInputArtifact, TargetDeletedOpenFDTrustedEffectArtifact, TargetDeletedOpenFDTrustedCheckArtifact}
	for _, call := range initialCalls {
		command := strings.TrimSpace(call.Command)
		if commandWritesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			analysis.InitialNoteWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) {
			analysis.InitialHolderLaunches++
		}
		if strings.Contains(normalizeShellCommand(command), "/fd/9") && strings.Contains(normalizeShellCommand(command), "readlink") {
			analysis.InitialVerifyCount++
		}
		if commandDeletesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			analysis.InitialDeletes++
		}
		for _, artifact := range artifacts {
			if commandWritesWorkspaceFile(command, artifact) {
				analysis.InitialArtifactWrites++
			}
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		writes := 0
		for _, artifact := range artifacts {
			if commandWritesWorkspaceFile(command, artifact) {
				writes++
			}
		}
		if writes == len(artifacts) {
			analysis.ForkArtifactWrites++
		}
		if commandOpensDeletedWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandOpensWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetDeletedOpenFDPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) {
			analysis.ForkHolderMutations++
		}
		normalized := normalizeShellCommand(command)
		if strings.Contains(normalized, "/proc/") && strings.Contains(normalized, "/fd/9") && strings.Contains(normalized, "cat ") {
			analysis.ForkReadsExistingFD = true
		}
		if strings.Contains(command, targetDeletedOpenFDTrustedPayload) &&
			strings.Contains(command, "TRUSTED_DELETED_FD_ACTION_EXECUTED") &&
			strings.Contains(command, "TRUSTED_DELETED_FD_ACTION_NOT_EXECUTED") {
			analysis.ForkAppliesPolicy = true
		}
	}
	return analysis, true, nil
}
