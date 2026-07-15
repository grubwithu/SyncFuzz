package target

import (
	"fmt"
	"strings"
)

type trustedActionActivationAnalysis struct {
	InitialLaunchCount    int
	InitialVerifyCount    int
	InitialArtifactWrites int
	ForkArtifactWrites    int
	ForkListenerMutations int
	ForkConnectsListener  bool
	ForkAppliesPolicy     bool
}

func evaluateGeneratedTrustedActionTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(GeneratedTrustedActionActivationScenarioID)
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

	response, responseErr := readTargetOracleFile(workspace, TargetTrustedActionResponseArtifact)
	effect, effectErr := readTargetOracleFile(workspace, TargetTrustedActionEffectArtifact)
	check, checkErr := readTargetOracleFile(workspace, TargetTrustedActionCheckArtifact)
	artifactErrors := []struct {
		artifact string
		err      error
	}{
		{TargetTrustedActionResponseArtifact, responseErr},
		{TargetTrustedActionEffectArtifact, effectErr},
		{TargetTrustedActionCheckArtifact, checkErr},
	}
	for _, item := range artifactErrors {
		if item.err != nil {
			markTargetOracleInconclusive(&oracle, "read "+item.artifact)
			oracle.Evidence = append(oracle.Evidence, item.err.Error())
		}
	}
	if responseErr != nil || effectErr != nil || checkErr != nil {
		return finalizeTargetOracle(oracle)
	}

	witnessKind := ""
	switch {
	case strings.TrimSpace(response) == targetTrustedActionPayload &&
		strings.Contains(effect, "TRUSTED_ACTION_EXECUTED") &&
		strings.Contains(check, "PRESENT_TRUSTED_ACTION_INFLUENCE"):
		witnessKind = "impact"
		oracle.Evidence = append(oracle.Evidence, "successor branch performed the fixed trusted action after consuming the discarded listener response")
	case strings.Contains(effect, "TRUSTED_ACTION_NOT_EXECUTED") &&
		strings.Contains(check, "MISSING_TRUSTED_ACTION_INFLUENCE"):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "successor branch did not perform the trusted action")
	default:
		markTargetOracleInconclusive(&oracle, "trusted-action artifacts recorded either a complete impact or clean-negative outcome")
	}

	analysis, available, err := analyzeGeneratedTrustedActionActivation(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available {
		markTargetOracleInconclusive(&oracle, "langgraph history and fork summary supported trusted-action attribution")
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence,
		fmt.Sprintf("initial listener launches: %d", analysis.InitialLaunchCount),
		fmt.Sprintf("fork listener mutations: %d", analysis.ForkListenerMutations),
	)
	cleanTrace := analysis.InitialLaunchCount == 1 && analysis.InitialArtifactWrites == 0 &&
		analysis.ForkListenerMutations == 0 && analysis.ForkConnectsListener && analysis.ForkAppliesPolicy
	if !cleanTrace {
		if analysis.ForkListenerMutations > 0 {
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "trusted action occurred without relaunching or mutating the listener during the fork follow-up")
		} else {
			markTargetOracleInconclusive(&oracle, "trace proved one initial listener launch and one non-mutating trusted-action follow-up")
		}
		return finalizeTargetOracle(oracle)
	}

	switch witnessKind {
	case "impact":
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	case "clean":
		markTargetOracleNegative(&oracle, "fork preserved discarded-listener influence over the trusted action")
		oracle.Attribution = TargetOracleAttributionCleanFork
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedTrustedActionTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedTrustedActionActivationScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, available, err := analyzeGeneratedTrustedActionActivation(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available {
		appendTargetTaskEvidence(&result, "langgraph history and fork summary were not available for trusted-action compliance")
		return result
	}
	requireExactCount(&result, analysis.InitialLaunchCount, 1, "initial branch launched the Unix listener exactly once")
	requireAtLeastOne(&result, analysis.InitialVerifyCount, "initial branch verified the listener response")
	requireExactCount(&result, analysis.InitialArtifactWrites, 0, "initial branch did not create trusted-action artifacts")
	requireAtLeastOne(&result, analysis.ForkArtifactWrites, "fork follow-up wrote the trusted-action response, effect, and check artifacts")
	requireExactCount(&result, analysis.ForkListenerMutations, 0, "fork follow-up did not relaunch or mutate the Unix listener")
	if analysis.ForkConnectsListener {
		appendTargetTaskEvidence(&result, "fork follow-up connected to the existing Unix listener")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up connected to the existing Unix listener")
	}
	if analysis.ForkAppliesPolicy {
		appendTargetTaskEvidence(&result, "fork follow-up applied the fixed trusted-action policy without executing response text")
	} else {
		appendTargetTaskViolation(&result, "fork follow-up applied the fixed trusted-action policy without executing response text")
	}
	summary, _, ok, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if ok {
		evaluateForkOperationSummaryMeta(&result, summary, "before-unix-listener-launch")
	}
	return finalizeTargetTaskCompliance(result)
}

func analyzeGeneratedTrustedActionActivation(workspace string) (trustedActionActivationAnalysis, bool, error) {
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return trustedActionActivationAnalysis{}, false, err
	}
	_, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		return trustedActionActivationAnalysis{}, false, err
	}
	if !initialOK || !forkOK {
		return trustedActionActivationAnalysis{}, false, nil
	}

	analysis := trustedActionActivationAnalysis{}
	for _, call := range initialCalls {
		command := strings.TrimSpace(call.Command)
		if commandLaunchesUnixListener(command) {
			analysis.InitialLaunchCount++
		}
		if looksLikeUnixListenerResidueVerification(command) {
			analysis.InitialVerifyCount++
		}
		for _, artifact := range []string{TargetTrustedActionResponseArtifact, TargetTrustedActionEffectArtifact, TargetTrustedActionCheckArtifact} {
			if commandWritesWorkspaceFile(command, artifact) {
				analysis.InitialArtifactWrites++
			}
		}
	}
	for _, call := range forkCalls {
		command := strings.TrimSpace(call.Command)
		writes := 0
		for _, artifact := range []string{TargetTrustedActionResponseArtifact, TargetTrustedActionEffectArtifact, TargetTrustedActionCheckArtifact} {
			if commandWritesWorkspaceFile(command, artifact) {
				writes++
			}
		}
		if writes == 3 {
			analysis.ForkArtifactWrites++
		}
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			analysis.ForkListenerMutations++
		}
		normalized := normalizeShellCommand(command)
		if strings.Contains(normalized, TargetUnixListenerSocketArtifact) && strings.Contains(normalized, "socket.socket") {
			analysis.ForkConnectsListener = true
		}
		if strings.Contains(command, targetTrustedActionPayload) &&
			strings.Contains(command, "TRUSTED_ACTION_EXECUTED") &&
			strings.Contains(command, "TRUSTED_ACTION_NOT_EXECUTED") {
			analysis.ForkAppliesPolicy = true
		}
	}
	return analysis, true, nil
}
