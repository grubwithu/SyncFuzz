package target

import "fmt"

func evaluateGeneratedEnvReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := evaluateEnvResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadGeneratedReplayCommandTrace(workspace, "langgraph generated env replay")
	})

	analysis, summary, available, err := analyzeGeneratedEnvReplay(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported generated env replay attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available || summary == nil {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported generated env replay attribution")
		return finalizeTargetOracle(oracle)
	}
	preObservationMutations, observationMutations, mutationInfoAvailable, err := generatedEnvReplayMutationShape(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph replay summary distinguished replay-side reexecution from final-call reconstruction")
		return finalizeTargetOracle(oracle)
	}
	if !mutationInfoAvailable {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary distinguished replay-side reexecution from final-call reconstruction")
		return finalizeTargetOracle(oracle)
	}

	switch {
	case observationMutations > 0:
		oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
		oracle.Missing = nil
		markTargetOracleNegative(&oracle, "replay preserved the discarded environment variable directly across the selected checkpoint boundary")
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observation mutated the environment variable while writing env-residue-check.txt")
	case preObservationMutations > 0:
		oracle.Attribution = TargetOracleAttributionLegitimateReexecution
		oracle.Missing = nil
		markTargetOracleNegative(&oracle, "replay preserved the discarded environment variable directly across the selected checkpoint boundary")
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("langgraph replay re-executed the environment-variable mutation %d time(s) before the final observation", preObservationMutations))
	case oracle.Status == TargetOracleStatusConfirmed:
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observed the environment variable without replay-side re-export")
	case oracle.Status == TargetOracleStatusNegative && oracle.Attribution == TargetOracleAttributionUnknown && analysis.CleanObservationCall:
		oracle.Attribution = TargetOracleAttributionCleanReplay
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observed that the environment variable was absent without replay-side re-export")
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedFunctionReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := evaluateFunctionResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadGeneratedReplayCommandTrace(workspace, "langgraph generated function replay")
	})

	analysis, summary, available, err := analyzeGeneratedFunctionReplay(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported generated function replay attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available || summary == nil {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported generated function replay attribution")
		return finalizeTargetOracle(oracle)
	}
	preObservationMutations, observationMutations, mutationInfoAvailable, err := generatedFunctionReplayMutationShape(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph replay summary distinguished replay-side reexecution from final-call reconstruction")
		return finalizeTargetOracle(oracle)
	}
	if !mutationInfoAvailable {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary distinguished replay-side reexecution from final-call reconstruction")
		return finalizeTargetOracle(oracle)
	}

	switch {
	case observationMutations > 0:
		oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
		oracle.Missing = nil
		markTargetOracleNegative(&oracle, "replay preserved the discarded shell function directly across the selected checkpoint boundary")
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observation redefined or unset the shell function while writing function-residue-check.txt")
	case preObservationMutations > 0:
		oracle.Attribution = TargetOracleAttributionLegitimateReexecution
		oracle.Missing = nil
		markTargetOracleNegative(&oracle, "replay preserved the discarded shell function directly across the selected checkpoint boundary")
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("langgraph replay re-executed the shell-function mutation %d time(s) before the final observation", preObservationMutations))
	case oracle.Status == TargetOracleStatusConfirmed:
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observed the shell function without replay-side redefinition")
	case oracle.Status == TargetOracleStatusNegative && oracle.Attribution == TargetOracleAttributionUnknown && analysis.CleanObservationCall:
		oracle.Attribution = TargetOracleAttributionCleanReplay
		oracle.Evidence = append(oracle.Evidence, "langgraph replay observed that the shell function was absent without replay-side redefinition")
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedEnvReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedEnvReplayPrimitiveSubstitutionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, summary, available, err := analyzeGeneratedEnvReplay(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available || summary == nil {
		appendTargetTaskEvidence(&result, "langgraph replay summary was not available for generated env replay compliance")
		return result
	}

	requireExactCount(&result, analysis.ExportCount, 1, "task exported the branch-local environment variable exactly once across initial run and replay")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "replay phase wrote env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterVarReferences, "replay phase observed SYNCFUZZ_ENV_RESIDUE_FLAG")
	requireExactCount(&result, analysis.LaterEnvMutations, 0, "replay phase did not re-export, reassign, or unset the branch-local environment variable")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "replay phase observed the environment variable without mutating it again")
	} else {
		appendTargetTaskViolation(&result, "replay phase observed the environment variable without mutating it again")
	}
	evaluateReplayOperationSummaryMeta(&result, summary, "before-env-export")
	return finalizeTargetTaskCompliance(result)
}

func evaluateGeneratedFunctionReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedFunctionReplayPrimitiveSubstitutionScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, summary, available, err := analyzeGeneratedFunctionReplay(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available || summary == nil {
		appendTargetTaskEvidence(&result, "langgraph replay summary was not available for generated function replay compliance")
		return result
	}

	requireExactCount(&result, analysis.DefinitionCount, 1, "task defined the branch-local shell function exactly once across initial run and replay")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "replay phase wrote function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterFunctionRefs, "replay phase referenced syncfuzz_residue_probe")
	requireExactCount(&result, analysis.LaterFunctionMutations, 0, "replay phase did not redefine or unset the branch-local shell function")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "replay phase observed the shell function without redefining it")
	} else {
		appendTargetTaskViolation(&result, "replay phase observed the shell function without redefining it")
	}
	evaluateReplayOperationSummaryMeta(&result, summary, "before-function-define")
	return finalizeTargetTaskCompliance(result)
}

func analyzeGeneratedEnvReplay(workspace string) (envResidueCommandAnalysis, *langgraphOperationSummary, bool, error) {
	trace, summary, available, err := loadGeneratedReplayTrace(workspace, "langgraph generated env replay")
	if err != nil {
		return envResidueCommandAnalysis{}, nil, false, err
	}
	if !available {
		return envResidueCommandAnalysis{}, summary, false, nil
	}
	return analyzeEnvResidueCommands(trace.Commands), summary, true, nil
}

func analyzeGeneratedFunctionReplay(workspace string) (functionResidueCommandAnalysis, *langgraphOperationSummary, bool, error) {
	trace, summary, available, err := loadGeneratedReplayTrace(workspace, "langgraph generated function replay")
	if err != nil {
		return functionResidueCommandAnalysis{}, nil, false, err
	}
	if !available {
		return functionResidueCommandAnalysis{}, summary, false, nil
	}
	return analyzeFunctionResidueCommands(trace.Commands), summary, true, nil
}

func loadGeneratedReplayCommandTrace(workspace string, source string) (shellCommandTrace, error) {
	trace, _, available, err := loadGeneratedReplayTrace(workspace, source)
	if err != nil {
		return shellCommandTrace{Source: source}, err
	}
	if !available {
		return shellCommandTrace{Source: source}, nil
	}
	return trace, nil
}

func loadGeneratedReplayTrace(workspace string, source string) (shellCommandTrace, *langgraphOperationSummary, bool, error) {
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return shellCommandTrace{Source: source}, nil, false, err
	}
	summary, replayCalls, replayOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphReplayArtifact)
	if err != nil {
		return shellCommandTrace{Source: source}, nil, false, err
	}
	if !initialOK || !replayOK || summary == nil {
		return shellCommandTrace{Source: source}, summary, false, nil
	}
	commands := make([]string, 0, len(initialCalls)+len(replayCalls))
	for _, call := range initialCalls {
		commands = append(commands, call.Command)
	}
	for _, call := range replayCalls {
		commands = append(commands, call.Command)
	}
	return shellCommandTrace{Source: source, Available: true, Commands: commands}, summary, true, nil
}

func generatedEnvReplayMutationShape(workspace string) (int, int, bool, error) {
	_, replayCalls, replayOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphReplayArtifact)
	if err != nil {
		return 0, 0, false, err
	}
	if !replayOK {
		return 0, 0, false, nil
	}
	preObservationMutations := 0
	observationMutations := 0
	for _, call := range replayCalls {
		command := call.Command
		if command == "" || !commandMutatesEnvResidue(command) {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetEnvResidueCheckArtifact) {
			observationMutations++
		} else {
			preObservationMutations++
		}
	}
	return preObservationMutations, observationMutations, true, nil
}

func generatedFunctionReplayMutationShape(workspace string) (int, int, bool, error) {
	_, replayCalls, replayOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphReplayArtifact)
	if err != nil {
		return 0, 0, false, err
	}
	if !replayOK {
		return 0, 0, false, nil
	}
	preObservationMutations := 0
	observationMutations := 0
	for _, call := range replayCalls {
		command := call.Command
		if command == "" {
			continue
		}
		mutates := commandDefinesFunctionResidue(command) || commandUnsetsFunctionResidue(command)
		if !mutates {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetFunctionResidueCheckArtifact) {
			observationMutations++
		} else {
			preObservationMutations++
		}
	}
	return preObservationMutations, observationMutations, true, nil
}
