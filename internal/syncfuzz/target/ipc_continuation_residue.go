package target

import (
	"fmt"
	"strings"
)

type unixListenerResidueCommandAnalysis struct {
	CallCount              int
	LaunchCount            int
	PIDWriteCount          int
	VerifyCount            int
	InitialWitnessWrites   int
	LaterWitnessWrites     int
	LaterListenerMutations int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

func evaluateUnixListenerResidueTargetTaskCompliance(workspace string, targetID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   UnixListenerResidueTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	trace, err := loadShellCommandTrace(workspace, targetID)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !trace.Available {
		appendTargetTaskEvidence(&result, trace.Source+" artifact was not available for task compliance")
		return result
	}
	appendTargetTaskEvidence(&result, trace.Source+" was available for task compliance")
	appendTargetTaskEvidence(&result, fmt.Sprintf("observed shell calls: %d", len(trace.Commands)))

	analysis := analyzeUnixListenerResidueCommands(trace.Commands)
	requireExactCount(&result, analysis.LaunchCount, 1, "task launched the branch Unix listener exactly once")
	requireExactCount(&result, analysis.PIDWriteCount, 1, "task wrote branch-listener-pid.txt exactly once")
	requireAtLeastOne(&result, analysis.VerifyCount, "task later verified that branch-listener.sock still answered the SyncFuzz marker")
	requireExactCount(&result, analysis.InitialWitnessWrites, 0, "initial shell steps did not create unix-listener-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterWitnessWrites, "later shell step wrote unix-listener-residue-check.txt")
	requireExactCount(&result, analysis.LaterListenerMutations, 0, "later shell step did not relaunch or mutate the branch Unix listener")
	if analysis.CallCount >= 2 {
		appendTargetTaskEvidence(&result, "task used multiple shell calls so the existing listener could be observed later")
	} else {
		appendTargetTaskViolation(&result, "task used multiple shell calls so the existing listener could be observed later")
	}
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later shell step connected to the existing Unix listener without relaunching it")
	} else {
		appendTargetTaskViolation(&result, "later shell step connected to the existing Unix listener without relaunching it")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateUnixListenerResidueTargetOracle(workspace string, targetID string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(UnixListenerResidueTargetTaskID)
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

	witness, err := readTargetOracleFile(workspace, TargetUnixListenerResidueCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetUnixListenerResidueCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	witnessKind := ""
	switch {
	case outputShowsUnixListenerResidueMarker(witness):
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the later shell step still reached the existing branch Unix listener")
	case outputShowsMissingUnixListenerResidue(witness):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the later shell step could not reach the existing branch Unix listener")
	default:
		markTargetOracleInconclusive(&oracle, "witness contained a recognizable Unix listener residue marker")
	}

	trace, err := loadShellCommandTrace(workspace, targetID)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the existing Unix listener without relaunching it")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the Unix listener residue oracle")
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the existing Unix listener without relaunching it")
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeUnixListenerResidueCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the Unix listener residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.LaunchCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial branch Unix listener launch")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial branch Unix listener launch")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later observation call without relaunching or mutating the listener")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "Unix listener residue occurred without relaunching or mutating the listener during the later observation call")
		default:
			markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the existing Unix listener without relaunching it")
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, "later shell call preserved the branch Unix listener across shell calls")
		}
	}
	return finalizeTargetOracle(oracle)
}

func analyzeUnixListenerResidueCommands(commands []string) unixListenerResidueCommandAnalysis {
	analysis := unixListenerResidueCommandAnalysis{CallCount: len(commands)}
	launchIndex := -1

	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		launches := commandLaunchesUnixListener(command)
		writesPID := commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			strings.Contains(normalizeShellCommand(command), TargetUnixListenerPIDArtifact)
		verifies := looksLikeUnixListenerResidueVerification(command)
		writesWitness := commandWritesWorkspaceFile(command, TargetUnixListenerResidueCheckArtifact)
		mutates := launches ||
			writesPID ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerPIDArtifact)

		if launches {
			analysis.LaunchCount++
			if launchIndex == -1 {
				launchIndex = idx
			}
		}
		if writesPID {
			analysis.PIDWriteCount++
		}
		if launchIndex == -1 || idx <= launchIndex {
			if writesWitness {
				analysis.InitialWitnessWrites++
			}
			continue
		}
		if verifies {
			analysis.VerifyCount++
		}
		if writesWitness {
			analysis.LaterWitnessWrites++
		}
		if mutates {
			analysis.LaterListenerMutations++
		}
		if verifies && !mutates {
			analysis.CleanObservationCall = true
		}
		if verifies && mutates {
			analysis.RebuiltObservationCall = true
		}
	}

	return analysis
}
