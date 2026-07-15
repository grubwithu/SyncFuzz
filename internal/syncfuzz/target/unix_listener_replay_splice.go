package target

import (
	"fmt"
	"strings"
)

type unixListenerReplayLifecycleSpliceAnalysis struct {
	InitialLaunchCount   int
	InitialVerifyCount   int
	InitialWitnessWrites int
	ReplayLaunchCount    int
	ReplayVerifyCount    int
	ReplayWitnessWrites  int
	ReplayPIDWrites      int
	ReplayDeleteCount    int
	ReplayWitnessMarker  bool
	ReplayMissingMarker  bool
}

func evaluateGeneratedUnixListenerReplayLifecycleSpliceOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(GeneratedUnixListenerReplayLifecycleSpliceScenarioID)
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

	witness, err := readTargetOracleFile(workspace, TargetUnixListenerReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetUnixListenerReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	switch {
	case outputShowsUnixListenerResidueMarker(witness):
		oracle.Evidence = append(oracle.Evidence, "replay witness received a response from the branch Unix listener")
	case outputShowsMissingUnixListenerResidue(witness):
		oracle.Evidence = append(oracle.Evidence, "replay witness reported that the branch Unix listener was absent")
	default:
		markTargetOracleInconclusive(&oracle, "replay witness contained a recognizable Unix listener marker")
	}

	sawInitialLaunch, err := langgraphHistoryShowsUnixListenerLaunch(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
	} else if sawInitialLaunch {
		oracle.Evidence = append(oracle.Evidence, "langgraph history captured the initial branch Unix listener launch")
	} else {
		markTargetOracleInconclusive(&oracle, "langgraph history captured the initial branch Unix listener launch")
	}

	analysis, _, available, err := analyzeGeneratedUnixListenerReplayLifecycleSplice(workspace)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported Unix listener replay attribution")
		return finalizeTargetOracle(oracle)
	}
	if !available {
		markTargetOracleInconclusive(&oracle, "langgraph replay summary supported Unix listener replay attribution")
		return finalizeTargetOracle(oracle)
	}

	confirmed, attribution, details := classifyUnixListenerReplayLifecycleSplice(analysis)
	if attribution != "" {
		oracle.Attribution = attribution
	}
	oracle.Evidence = append(oracle.Evidence, details...)
	if !confirmed {
		markTargetOracleStatusFromAttribution(&oracle, attribution)
		appendTargetOracleMissing(&oracle, unixListenerReplayAttributionMissingReason(attribution))
	}
	if oracle.Attribution == TargetOracleAttributionRuntimeResidue && !outputShowsUnixListenerResidueMarker(witness) {
		markTargetOracleInconclusive(&oracle, "replay witness preserved the discarded branch Unix listener across the replay boundary")
	}
	if oracle.Attribution == TargetOracleAttributionCleanReplay && !outputShowsMissingUnixListenerResidue(witness) {
		markTargetOracleInconclusive(&oracle, "replay witness showed a clean Unix listener reset")
	}
	return finalizeTargetOracle(oracle)
}

func evaluateGeneratedUnixListenerReplayLifecycleSpliceCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   GeneratedUnixListenerReplayLifecycleSpliceScenarioID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	analysis, summary, available, err := analyzeGeneratedUnixListenerReplayLifecycleSplice(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !available || summary == nil {
		appendTargetTaskEvidence(&result, "langgraph replay summary was not available for Unix listener replay splice compliance")
		return result
	}

	requireExactCount(&result, analysis.InitialLaunchCount, 1, "initial branch launched the Unix listener exactly once")
	requireAtLeastOne(&result, analysis.InitialVerifyCount, "initial branch verified that branch-listener.sock returned the expected response")
	requireAtLeastOne(&result, analysis.ReplayWitnessWrites, "replay phase wrote unix-listener-residue-replay-check.txt")
	requireAtLeastOne(&result, analysis.ReplayVerifyCount, "replay phase observed the current Unix listener state")
	if analysis.ReplayLaunchCount > 1 {
		appendTargetTaskViolation(&result, "replay phase launched the Unix listener at most once")
	} else {
		appendTargetTaskEvidence(&result, "replay phase launched the Unix listener at most once")
	}
	if analysis.ReplayDeleteCount > 0 && analysis.ReplayLaunchCount == 0 {
		appendTargetTaskViolation(&result, "replay phase avoided deleting listener artifacts outside a relaunch attempt")
	} else {
		appendTargetTaskEvidence(&result, "replay phase did not delete listener artifacts outside a relaunch attempt")
	}
	if analysis.ReplayLaunchCount > 0 || analysis.ReplayPIDWrites > 0 {
		appendTargetTaskEvidence(&result, "replay phase legitimately reexecuted the listener launch before observation")
	} else {
		appendTargetTaskEvidence(&result, "replay phase only observed the existing Unix listener state")
	}
	evaluateReplayOperationSummaryMeta(&result, summary, "before-unix-listener-launch")
	return finalizeTargetTaskCompliance(result)
}

func analyzeGeneratedUnixListenerReplayLifecycleSplice(workspace string) (unixListenerReplayLifecycleSpliceAnalysis, *langgraphOperationSummary, bool, error) {
	historyCalls, historyOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return unixListenerReplayLifecycleSpliceAnalysis{}, nil, false, err
	}
	summary, replayCalls, replayOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphReplayArtifact)
	if err != nil {
		return unixListenerReplayLifecycleSpliceAnalysis{}, nil, false, err
	}
	if !historyOK || !replayOK || summary == nil {
		return unixListenerReplayLifecycleSpliceAnalysis{}, summary, false, nil
	}

	analysis := unixListenerReplayLifecycleSpliceAnalysis{}
	for _, call := range historyCalls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandLaunchesUnixListener(command) {
			analysis.InitialLaunchCount++
		}
		if looksLikeUnixListenerResidueVerification(command) {
			analysis.InitialVerifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetUnixListenerReplayArtifact) {
			analysis.InitialWitnessWrites++
		}
	}
	for _, call := range replayCalls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" && !replayUnixListenerWitnessProduced(call) {
			continue
		}
		if commandLaunchesUnixListener(command) {
			analysis.ReplayLaunchCount++
		}
		if commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) {
			analysis.ReplayPIDWrites++
		}
		if !commandLaunchesUnixListener(command) &&
			(commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) ||
				commandDeletesWorkspaceFile(command, TargetUnixListenerPIDArtifact)) {
			analysis.ReplayDeleteCount++
		}
		if looksLikeUnixListenerResidueVerification(command) {
			analysis.ReplayVerifyCount++
		}
		if commandWritesWorkspaceFile(command, TargetUnixListenerReplayArtifact) || replayUnixListenerWitnessProduced(call) {
			analysis.ReplayWitnessWrites++
		}
		if outputShowsUnixListenerResidueMarker(output) {
			analysis.ReplayWitnessMarker = true
		}
		if outputShowsMissingUnixListenerResidue(output) {
			analysis.ReplayMissingMarker = true
		}
	}
	return analysis, summary, true, nil
}

func classifyUnixListenerReplayLifecycleSplice(analysis unixListenerReplayLifecycleSpliceAnalysis) (bool, string, []string) {
	if analysis.ReplayLaunchCount > 0 || analysis.ReplayPIDWrites > 0 {
		details := []string{
			fmt.Sprintf("replay launched the Unix listener %d time(s) before final observation", analysis.ReplayLaunchCount),
			"langgraph replay transcript showed a listener relaunch instead of direct checkpoint-boundary preservation",
		}
		if analysis.ReplayWitnessMarker {
			details = append(details, "replay still connected successfully after reexecuting the listener launch")
		}
		return false, TargetOracleAttributionLegitimateReexecution, details
	}
	if analysis.ReplayVerifyCount > 0 && analysis.ReplayWitnessMarker {
		return true, TargetOracleAttributionRuntimeResidue, []string{
			"langgraph replay follow-up connected to an existing branch Unix listener without relaunching it",
			"replayed observation still received a response from the discarded branch service",
		}
	}
	if analysis.ReplayVerifyCount > 0 && analysis.ReplayMissingMarker {
		return false, TargetOracleAttributionCleanReplay, []string{
			"langgraph replay follow-up observed that the branch Unix listener was absent or unreachable",
		}
	}
	var details []string
	if analysis.ReplayVerifyCount > 0 {
		details = append(details, "langgraph replay transcript captured a Unix listener verification step")
	}
	if analysis.ReplayWitnessWrites > 0 {
		details = append(details, "replay wrote the Unix listener replay witness artifact")
	}
	return false, TargetOracleAttributionUnknown, details
}

func unixListenerReplayAttributionMissingReason(attribution string) string {
	switch attribution {
	case TargetOracleAttributionLegitimateReexecution:
		return "replay preserved the discarded branch Unix listener across the replay boundary without relaunching it"
	case TargetOracleAttributionCleanReplay:
		return "replay preserved the discarded branch Unix listener across the replay boundary"
	case TargetOracleAttributionUnknown:
		return "langgraph replay summary showed a Unix listener verification step without proving direct listener preservation"
	default:
		return ""
	}
}
