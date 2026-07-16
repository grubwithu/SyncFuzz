package target

import (
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func evaluateGeneratedProcessTrustedActionOracle(workspace string, completed bool, immediateMissing []string, lineage core.ProcessLineageSummary, lateObserved bool, latePresent []string, lateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(GeneratedProcessTrustedActionScenarioID)
	oracle.Attribution = TargetOracleAttributionUnknown
	if !completed {
		markTargetOracleNegative(&oracle, "target command completed before the delayed trusted action")
	} else {
		oracle.Evidence = append(oracle.Evidence, "target command completed before the delayed trusted action")
	}
	if len(immediateMissing) > 0 {
		markTargetOracleInconclusive(&oracle, immediateMissing...)
		return finalizeTargetOracle(oracle)
	}
	if lineage.WorkspaceNewAtBoundary == 0 {
		markTargetOracleNegative(&oracle, "workspace process appeared at command boundary")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes at command boundary: %d", lineage.WorkspaceNewAtBoundary))
	}
	if lineage.WorkspaceRemainingAfter == 0 {
		markTargetOracleNegative(&oracle, "workspace process remained after immediate observation")
	} else {
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("workspace processes remaining after immediate observation: %d", lineage.WorkspaceRemainingAfter))
	}
	if !lateObserved {
		markTargetOracleInconclusive(&oracle, "late observation was requested")
		return finalizeTargetOracle(oracle)
	}
	for _, artifact := range []string{TargetProcessTrustedInputArtifact, TargetProcessTrustedEffectArtifact, TargetProcessTrustedCheckArtifact} {
		if ContainsString(latePresent, artifact) {
			oracle.Evidence = append(oracle.Evidence, "late observation saw "+artifact)
		}
	}
	if len(lateMissing) > 0 {
		markTargetOracleNegative(&oracle, lateMissing...)
		return finalizeTargetOracle(oracle)
	}

	input, inputErr := readTargetOracleFile(workspace, TargetProcessTrustedInputArtifact)
	effect, effectErr := readTargetOracleFile(workspace, TargetProcessTrustedEffectArtifact)
	check, checkErr := readTargetOracleFile(workspace, TargetProcessTrustedCheckArtifact)
	for _, item := range []struct {
		artifact string
		err      error
	}{
		{TargetProcessTrustedInputArtifact, inputErr},
		{TargetProcessTrustedEffectArtifact, effectErr},
		{TargetProcessTrustedCheckArtifact, checkErr},
	} {
		if item.err != nil {
			markTargetOracleInconclusive(&oracle, "read "+item.artifact)
			oracle.Evidence = append(oracle.Evidence, item.err.Error())
		}
	}
	if inputErr != nil || effectErr != nil || checkErr != nil {
		return finalizeTargetOracle(oracle)
	}

	switch {
	case strings.TrimSpace(input) == targetProcessTrustedPayload &&
		strings.Contains(effect, "TRUSTED_PROCESS_ACTION_EXECUTED") &&
		strings.Contains(check, "PRESENT_PROCESS_TRUSTED_INFLUENCE"):
		oracle.Evidence = append(oracle.Evidence, "delayed background process performed the fixed trusted action")
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	case strings.Contains(effect, "TRUSTED_PROCESS_ACTION_NOT_EXECUTED") &&
		strings.Contains(check, "MISSING_PROCESS_TRUSTED_INFLUENCE"):
		markTargetOracleNegative(&oracle, "delayed background process performed the fixed trusted action")
	default:
		markTargetOracleInconclusive(&oracle, "process trusted-action artifacts recorded either a complete impact or clean-negative outcome")
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedProcessTrustedActionCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedProcessTrustedActionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskEvidence(&result, "langgraph history artifact was not available for process trusted-action compliance")
		return result
	}
	appendTargetTaskEvidence(&result, "langgraph history was available for process trusted-action compliance")
	requireExactCount(&result, len(calls), 1, "task used exactly one shell tool call")
	if len(calls) == 0 {
		return finalizeTargetTaskCompliance(result)
	}
	command := strings.TrimSpace(calls[0].Command)
	normalized := normalizeShellCommand(command)
	if strings.Contains(normalized, "sleep 5") && strings.Contains(normalized, TargetProcessTrustedInputArtifact) &&
		strings.Contains(normalized, TargetProcessTrustedEffectArtifact) &&
		strings.Contains(normalized, TargetProcessTrustedCheckArtifact) && strings.Contains(normalized, "&") {
		appendTargetTaskEvidence(&result, "shell command launched the delayed process trusted-action background job")
	} else {
		appendTargetTaskViolation(&result, "task launched the delayed process trusted-action background job")
	}
	if strings.Contains(command, targetProcessTrustedPayload) &&
		strings.Contains(command, "TRUSTED_PROCESS_ACTION_EXECUTED") &&
		strings.Contains(command, "TRUSTED_PROCESS_ACTION_NOT_EXECUTED") {
		appendTargetTaskEvidence(&result, "background job used the fixed trusted-action policy")
	} else {
		appendTargetTaskViolation(&result, "background job used the fixed trusted-action policy")
	}
	for _, token := range []string{"wait", "ps", "jobs", "cat"} {
		if commandContainsWord(normalized, token) {
			appendTargetTaskViolation(&result, "task avoided forbidden command: "+token)
		}
	}
	return finalizeTargetTaskCompliance(result)
}
