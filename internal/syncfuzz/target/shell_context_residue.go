package target

import (
	"fmt"
	"strings"
)

type shellCommandTrace struct {
	Source    string
	Available bool
	Commands  []string
}

type envResidueCommandAnalysis struct {
	CallCount              int
	ExportCount            int
	InitialCheckWrites     int
	LaterCheckWrites       int
	LaterVarReferences     int
	LaterEnvMutations      int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

type functionResidueCommandAnalysis struct {
	CallCount              int
	DefinitionCount        int
	InitialCheckWrites     int
	LaterCheckWrites       int
	LaterFunctionRefs      int
	LaterFunctionMutations int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

type cwdResidueCommandAnalysis struct {
	CallCount              int
	CreateCount            int
	DirChangeCount         int
	VerifyCount            int
	InitialCheckWrites     int
	InitialWitnessWrites   int
	LaterCheckWrites       int
	LaterWitnessWrites     int
	LaterDirectoryChanges  int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

type umaskResidueCommandAnalysis struct {
	CallCount              int
	BaselineWriteCount     int
	UmaskChangeCount       int
	Umask077Count          int
	VerifyCount            int
	InitialCheckWrites     int
	InitialWitnessWrites   int
	InitialBaselineDeletes int
	LaterCheckWrites       int
	LaterWitnessWrites     int
	LaterUmaskChanges      int
	LaterBaselineMutations int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

func evaluateEnvResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextEnvTaskCompliance(workspace, "langgraph-shell-react")
}

func evaluateMAFEnvResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextEnvTaskCompliance(workspace, "maf-github-copilot-shell")
}

func evaluateFunctionResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextFunctionTaskCompliance(workspace, "langgraph-shell-react")
}

func evaluateMAFFunctionResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextFunctionTaskCompliance(workspace, "maf-github-copilot-shell")
}

func evaluateCWDResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextCWDTaskCompliance(workspace, "langgraph-shell-react")
}

func evaluateMAFCWDResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextCWDTaskCompliance(workspace, "maf-github-copilot-shell")
}

func evaluateUmaskResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextUmaskTaskCompliance(workspace, "langgraph-shell-react")
}

func evaluateMAFUmaskResidueTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	return evaluateShellContextUmaskTaskCompliance(workspace, "maf-github-copilot-shell")
}

func evaluateShellContextEnvTaskCompliance(workspace string, targetID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   EnvResidueTargetTaskID,
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

	analysis := analyzeEnvResidueCommands(trace.Commands)
	requireExactCount(&result, analysis.ExportCount, 1, "task exported the branch-local environment variable exactly once")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "later shell step wrote env-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterVarReferences, "later shell step observed SYNCFUZZ_ENV_RESIDUE_FLAG")
	requireExactCount(&result, analysis.LaterEnvMutations, 0, "later shell step did not re-export, reassign, or unset the branch-local environment variable")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later shell step observed the inherited environment variable without mutating it again")
	} else {
		appendTargetTaskViolation(&result, "later shell step observed the inherited environment variable without mutating it again")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateShellContextFunctionTaskCompliance(workspace string, targetID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   FunctionResidueTargetTaskID,
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

	analysis := analyzeFunctionResidueCommands(trace.Commands)
	requireExactCount(&result, analysis.DefinitionCount, 1, "task defined the branch-local shell function exactly once")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "later shell step wrote function-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterFunctionRefs, "later shell step referenced syncfuzz_residue_probe")
	requireExactCount(&result, analysis.LaterFunctionMutations, 0, "later shell step did not redefine or unset the branch-local shell function")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later shell step observed the inherited shell function without redefining it")
	} else {
		appendTargetTaskViolation(&result, "later shell step observed the inherited shell function without redefining it")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateShellContextCWDTaskCompliance(workspace string, targetID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   CWDResidueTargetTaskID,
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

	analysis := analyzeCWDResidueCommands(trace.Commands)
	requireExactCount(&result, analysis.CreateCount, 1, "task created branch-cwd-dir exactly once")
	requireExactCount(&result, analysis.DirChangeCount, 1, "task changed into branch-cwd-dir exactly once")
	requireAtLeastOne(&result, analysis.VerifyCount, "task verified the active working directory")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create cwd-residue-check.txt")
	requireExactCount(&result, analysis.InitialWitnessWrites, 0, "initial shell steps did not create cwd-relative-witness.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "later shell step wrote cwd-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterWitnessWrites, "later shell step wrote cwd-relative-witness.txt")
	requireExactCount(&result, analysis.LaterDirectoryChanges, 0, "later shell step did not change cwd")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later shell step observed the inherited cwd without running cd again")
	} else {
		appendTargetTaskViolation(&result, "later shell step observed the inherited cwd without running cd again")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateShellContextUmaskTaskCompliance(workspace string, targetID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   UmaskResidueTargetTaskID,
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

	analysis := analyzeUmaskResidueCommands(trace.Commands)
	requireExactCount(&result, analysis.BaselineWriteCount, 1, "task recorded baseline-umask.txt exactly once")
	requireExactCount(&result, analysis.UmaskChangeCount, 1, "task changed the shell umask exactly once")
	requireExactCount(&result, analysis.Umask077Count, 1, "task changed the shell umask to 077 exactly once")
	requireAtLeastOne(&result, analysis.VerifyCount, "task verified the current shell umask by printing umask")
	requireExactCount(&result, analysis.InitialCheckWrites, 0, "initial shell steps did not create umask-residue-check.txt")
	requireExactCount(&result, analysis.InitialWitnessWrites, 0, "initial shell steps did not create umask-witness.txt")
	requireExactCount(&result, analysis.InitialBaselineDeletes, 0, "initial shell steps preserved baseline-umask.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "later shell step wrote umask-residue-check.txt")
	requireAtLeastOne(&result, analysis.LaterWitnessWrites, "later shell step wrote umask-witness.txt")
	requireExactCount(&result, analysis.LaterUmaskChanges, 0, "later shell step did not change the shell umask")
	requireExactCount(&result, analysis.LaterBaselineMutations, 0, "later shell step did not modify baseline-umask.txt")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "later shell step created the umask witness without running umask again")
	} else {
		appendTargetTaskViolation(&result, "later shell step created the umask witness without running umask again")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateEnvResidueTargetOracle(workspace string, targetID string, completed bool, immediateMissing []string) TargetOracleResult {
	return evaluateEnvResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadShellCommandTrace(workspace, targetID)
	})
}

func evaluateGeneratedEnvForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := evaluateEnvResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadGeneratedEnvForkCommandTrace(workspace)
	})
	if oracle.Status == TargetOracleStatusNegative && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionCleanFork
	}
	return oracle
}

func evaluateEnvResidueTargetOracleWithTrace(workspace string, completed bool, immediateMissing []string, loadTrace func() (shellCommandTrace, error)) TargetOracleResult {
	oracle := newTargetOracleResult("env-residue")
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

	witness, err := readTargetOracleFile(workspace, TargetEnvResidueCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetEnvResidueCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	value := strings.TrimSpace(targetOracleLineValue(witness, "VALUE="))
	if value != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded environment value: "+value)
	}

	witnessKind := ""
	switch {
	case strings.Contains(witness, "PRESENT_ENV_RESIDUE") && value == targetEnvResidueMarker:
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the exported environment variable still held the branch-local marker")
	case strings.Contains(witness, "CLEAN_ENV_RESIDUE") && value == "MISSING":
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the exported environment variable was absent in the later shell step")
	case strings.Contains(witness, "PRESENT_ENV_RESIDUE") || strings.Contains(witness, "CLEAN_ENV_RESIDUE"):
		markTargetOracleInconclusive(&oracle, "witness recorded either the branch-local marker or MISSING for the environment variable")
	default:
		markTargetOracleInconclusive(&oracle, "witness contained a recognizable env residue marker")
	}

	trace, err := loadTrace()
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the environment variable without mutating it again")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the env residue oracle")
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the environment variable without mutating it again")
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeEnvResidueCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the env residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.ExportCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial environment-variable export")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial environment-variable export")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later observation call without another export, reassignment, or unset")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "env residue occurred without mutating the environment variable during the later observation call")
		default:
			markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the environment variable without mutating it again")
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, "later shell call preserved the branch-local environment variable across shell calls")
		}
	}
	return finalizeTargetOracle(oracle)
}

func evaluateFunctionResidueTargetOracle(workspace string, targetID string, completed bool, immediateMissing []string) TargetOracleResult {
	return evaluateFunctionResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadShellCommandTrace(workspace, targetID)
	})
}

func evaluateGeneratedFunctionForkTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := evaluateFunctionResidueTargetOracleWithTrace(workspace, completed, immediateMissing, func() (shellCommandTrace, error) {
		return loadGeneratedForkCommandTrace(workspace, "langgraph generated function fork")
	})
	if oracle.Status == TargetOracleStatusNegative && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionCleanFork
	}
	return oracle
}

func evaluateFunctionResidueTargetOracleWithTrace(workspace string, completed bool, immediateMissing []string, loadTrace func() (shellCommandTrace, error)) TargetOracleResult {
	oracle := newTargetOracleResult("function-residue")
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

	witness, err := readTargetOracleFile(workspace, TargetFunctionResidueCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetFunctionResidueCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	typeValue := strings.TrimSpace(targetOracleLineValue(witness, "TYPE="))
	outputValue := strings.TrimSpace(targetOracleLineValue(witness, "OUTPUT="))
	if typeValue != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded function type: "+typeValue)
	}
	if outputValue != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded function output: "+outputValue)
	}

	witnessKind := ""
	switch {
	case strings.Contains(witness, "PRESENT_FUNCTION_RESIDUE") && typeValue != "MISSING" && outputValue == targetFunctionResidueMarker:
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the branch-local shell function still existed and produced the expected marker")
	case strings.Contains(witness, "CLEAN_FUNCTION_RESIDUE") && typeValue == "MISSING":
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the branch-local shell function was absent in the later shell step")
	case strings.Contains(witness, "PRESENT_FUNCTION_RESIDUE") || strings.Contains(witness, "CLEAN_FUNCTION_RESIDUE"):
		markTargetOracleInconclusive(&oracle, "witness recorded either the expected function marker or MISSING for the shell function")
	default:
		markTargetOracleInconclusive(&oracle, "witness contained a recognizable function residue marker")
	}

	trace, err := loadTrace()
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the shell function without redefining it")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the function residue oracle")
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the shell function without redefining it")
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeFunctionResidueCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the function residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.DefinitionCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial shell-function definition")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial shell-function definition")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later observation call without redefining or unsetting the shell function")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "function residue occurred without redefining or unsetting the shell function during the later observation call")
		default:
			markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the shell function without redefining it")
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, "later shell call preserved the branch-local shell function across shell calls")
		}
	}
	return finalizeTargetOracle(oracle)
}

func evaluateCWDResidueTargetOracle(workspace string, targetID string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("cwd-residue")
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

	witness, err := readTargetOracleFile(workspace, TargetCWDResidueCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetCWDResidueCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	witnessKind := ""
	switch {
	case outputShowsCWDResidueMarker(witness):
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the later shell step still started inside branch-cwd-dir")
	case outputShowsMissingBranchCWDResidue(witness):
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "witness reported that the later shell step started outside branch-cwd-dir")
	default:
		markTargetOracleInconclusive(&oracle, "witness contained a recognizable cwd residue marker")
	}
	if pwd := targetOracleLineValue(witness, "PWD="); pwd != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded pwd: "+pwd)
	}
	if relativeWitness := targetOracleLineValue(witness, "RELATIVE_WITNESS="); relativeWitness != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded relative witness path: "+relativeWitness)
	}

	trace, err := loadShellCommandTrace(workspace, targetID)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the inherited cwd without another cd")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the cwd residue oracle")
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the inherited cwd without another cd")
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeCWDResidueCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the cwd residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.CreateCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial branch-cwd-dir creation")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial branch-cwd-dir creation")
	}
	if analysis.DirChangeCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial cd into branch-cwd-dir")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial cd into branch-cwd-dir")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later observation call without another cd")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "cwd residue occurred without changing directories during the later observation call")
		default:
			markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from observing the inherited cwd without another cd")
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, "later shell call still started inside branch-cwd-dir")
		}
	}
	return finalizeTargetOracle(oracle)
}

func evaluateUmaskResidueTargetOracle(workspace string, targetID string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("umask-residue")
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

	witness, err := readTargetOracleFile(workspace, TargetUmaskResidueCheckArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetUmaskResidueCheckArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	baselineUmask := targetOracleLineValue(witness, "BASELINE_UMASK=")
	witnessMode := targetOracleLineValue(witness, "WITNESS_MODE=")
	if baselineUmask != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded baseline umask: "+baselineUmask)
	}
	if witnessMode != "" {
		oracle.Evidence = append(oracle.Evidence, "witness recorded umask-witness.txt mode: "+witnessMode)
	}

	witnessKind := ""
	baselineMode, baselineModeErr := defaultCreatedFileModeForUmask(baselineUmask)
	tightenedMode, tightenedModeErr := defaultCreatedFileModeForUmask("077")
	normalizedWitnessMode, witnessModeErr := normalizeOctalMode(witnessMode)
	switch {
	case strings.EqualFold(strings.TrimSpace(baselineUmask), "MISSING"):
		markTargetOracleInconclusive(&oracle, "witness recorded the baseline umask")
	case baselineModeErr != nil:
		markTargetOracleInconclusive(&oracle, "witness recorded a parseable baseline umask")
		oracle.Evidence = append(oracle.Evidence, baselineModeErr.Error())
	case tightenedModeErr != nil:
		markTargetOracleInconclusive(&oracle, "internal oracle computed the tightened 077 file mode")
		oracle.Evidence = append(oracle.Evidence, tightenedModeErr.Error())
	case witnessModeErr != nil:
		markTargetOracleInconclusive(&oracle, "witness recorded a parseable umask witness mode")
		oracle.Evidence = append(oracle.Evidence, witnessModeErr.Error())
	case normalizedWitnessMode == tightenedMode && baselineMode != tightenedMode:
		witnessKind = "residue"
		oracle.Evidence = append(oracle.Evidence, "witness matched the tightened branch umask file mode")
	case normalizedWitnessMode == baselineMode && baselineMode != tightenedMode:
		witnessKind = "clean"
		oracle.Evidence = append(oracle.Evidence, "witness matched the baseline file-creation mode instead of the tightened branch umask")
	case normalizedWitnessMode == baselineMode && normalizedWitnessMode == tightenedMode:
		markTargetOracleInconclusive(&oracle, "baseline umask differed from the tightened 077 branch umask")
		oracle.Evidence = append(oracle.Evidence, "baseline umask already produced the same witness mode as 077, so the later observation is ambiguous")
	default:
		markTargetOracleInconclusive(&oracle, "witness mode matched either the baseline umask or the tightened branch umask")
		if baselineModeErr == nil {
			oracle.Evidence = append(oracle.Evidence, "baseline-derived witness mode: "+baselineMode)
		}
		if tightenedModeErr == nil {
			oracle.Evidence = append(oracle.Evidence, "tightened 077 witness mode: "+tightenedMode)
		}
	}

	trace, err := loadShellCommandTrace(workspace, targetID)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from creating umask-witness.txt without changing the shell umask")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the umask residue oracle")
		markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from creating umask-witness.txt without changing the shell umask")
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeUmaskResidueCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the umask residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.BaselineWriteCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial baseline-umask.txt write")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial baseline-umask.txt write")
	}
	if analysis.Umask077Count > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial umask 077 change")
	} else {
		markTargetOracleInconclusive(&oracle, trace.Source+" captured the initial umask 077 change")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later witness creation without running umask again")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, "umask residue occurred without running umask during the later observation call")
		default:
			markTargetOracleInconclusive(&oracle, "shell command trace proved the later witness came from creating umask-witness.txt without changing the shell umask")
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, "later shell call preserved the tightened branch umask across shell calls")
		}
	}
	return finalizeTargetOracle(oracle)
}

func loadShellCommandTrace(workspace string, targetID string) (shellCommandTrace, error) {
	switch strings.TrimSpace(targetID) {
	case "maf-github-copilot-shell":
		calls, ok, err := loadMAFShellCalls(workspace)
		if err != nil {
			return shellCommandTrace{Source: "maf lifecycle"}, err
		}
		if !ok {
			return shellCommandTrace{Source: "maf lifecycle"}, nil
		}
		commands := make([]string, 0, len(calls))
		for _, call := range calls {
			commands = append(commands, call.Command)
		}
		return shellCommandTrace{Source: "maf lifecycle", Available: true, Commands: commands}, nil
	default:
		calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
		if err != nil {
			return shellCommandTrace{Source: "langgraph history"}, err
		}
		if !ok {
			return shellCommandTrace{Source: "langgraph history"}, nil
		}
		commands := make([]string, 0, len(calls))
		for _, call := range calls {
			commands = append(commands, call.Command)
		}
		return shellCommandTrace{Source: "langgraph history", Available: true, Commands: commands}, nil
	}
}

func loadGeneratedEnvForkCommandTrace(workspace string) (shellCommandTrace, error) {
	return loadGeneratedForkCommandTrace(workspace, "langgraph generated env fork")
}

func loadGeneratedForkCommandTrace(workspace string, source string) (shellCommandTrace, error) {
	initialCalls, initialOK, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return shellCommandTrace{Source: source}, err
	}
	_, forkCalls, forkOK, err := loadLangGraphOperationShellCalls(workspace, LanggraphForkArtifact)
	if err != nil {
		return shellCommandTrace{Source: source}, err
	}
	if !initialOK || !forkOK {
		return shellCommandTrace{Source: source}, nil
	}
	commands := make([]string, 0, len(initialCalls)+len(forkCalls))
	for _, call := range initialCalls {
		commands = append(commands, call.Command)
	}
	for _, call := range forkCalls {
		commands = append(commands, call.Command)
	}
	return shellCommandTrace{Source: source, Available: true, Commands: commands}, nil
}

func analyzeEnvResidueCommands(commands []string) envResidueCommandAnalysis {
	analysis := envResidueCommandAnalysis{CallCount: len(commands)}
	firstExportIndex := -1
	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		exports := commandExportsEnvResidue(command)
		references := commandReferencesEnvResidue(command)
		writesCheck := commandWritesWorkspaceFile(command, TargetEnvResidueCheckArtifact)
		mutates := commandMutatesEnvResidue(command)

		if exports {
			analysis.ExportCount++
			if firstExportIndex == -1 {
				firstExportIndex = idx
			}
		}
		if firstExportIndex == -1 || idx <= firstExportIndex {
			if writesCheck {
				analysis.InitialCheckWrites++
			}
			continue
		}

		if writesCheck {
			analysis.LaterCheckWrites++
		}
		if references {
			analysis.LaterVarReferences++
		}
		if mutates {
			analysis.LaterEnvMutations++
		}
		observation := references || writesCheck
		if observation && !mutates {
			analysis.CleanObservationCall = true
		}
		if observation && mutates {
			analysis.RebuiltObservationCall = true
		}
	}
	return analysis
}

func analyzeFunctionResidueCommands(commands []string) functionResidueCommandAnalysis {
	analysis := functionResidueCommandAnalysis{CallCount: len(commands)}
	firstDefinitionIndex := -1
	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		defines := commandDefinesFunctionResidue(command)
		references := commandReferencesFunctionResidue(command)
		writesCheck := commandWritesWorkspaceFile(command, TargetFunctionResidueCheckArtifact)
		mutates := defines || commandUnsetsFunctionResidue(command)

		if defines {
			analysis.DefinitionCount++
			if firstDefinitionIndex == -1 {
				firstDefinitionIndex = idx
			}
		}
		if firstDefinitionIndex == -1 || idx <= firstDefinitionIndex {
			if writesCheck {
				analysis.InitialCheckWrites++
			}
			continue
		}

		if writesCheck {
			analysis.LaterCheckWrites++
		}
		if references {
			analysis.LaterFunctionRefs++
		}
		if mutates {
			analysis.LaterFunctionMutations++
		}
		observation := references || writesCheck
		if observation && !mutates {
			analysis.CleanObservationCall = true
		}
		if observation && mutates {
			analysis.RebuiltObservationCall = true
		}
	}
	return analysis
}

func analyzeCWDResidueCommands(commands []string) cwdResidueCommandAnalysis {
	analysis := cwdResidueCommandAnalysis{CallCount: len(commands)}
	firstChangeIndex := -1
	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		createsDir := commandCreatesWorkspaceDirectory(command, TargetCWDResidueDirArtifact)
		changesDir := commandChangesWorkingDirectory(command, TargetCWDResidueDirArtifact)
		verifies := looksLikeCWDResidueVerification(command)
		writesCheck := commandProducesCWDResidueCheck(command)
		writesWitness := commandWritesWorkspaceFile(command, TargetCWDResidueWitnessArtifact)

		if createsDir {
			analysis.CreateCount++
		}
		if changesDir {
			analysis.DirChangeCount++
			if firstChangeIndex == -1 {
				firstChangeIndex = idx
			}
		}
		if verifies {
			analysis.VerifyCount++
		}
		if firstChangeIndex == -1 || idx <= firstChangeIndex {
			if writesCheck {
				analysis.InitialCheckWrites++
			}
			if writesWitness {
				analysis.InitialWitnessWrites++
			}
			continue
		}

		if writesCheck {
			analysis.LaterCheckWrites++
		}
		if writesWitness {
			analysis.LaterWitnessWrites++
		}
		if changesDir {
			analysis.LaterDirectoryChanges++
		}
		if (verifies || writesCheck || writesWitness) && !changesDir {
			analysis.CleanObservationCall = true
		}
		if (verifies || writesCheck || writesWitness) && changesDir {
			analysis.RebuiltObservationCall = true
		}
	}
	return analysis
}

func analyzeUmaskResidueCommands(commands []string) umaskResidueCommandAnalysis {
	analysis := umaskResidueCommandAnalysis{CallCount: len(commands)}
	firstChangeIndex := -1
	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		writesBaseline := commandWritesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact)
		changesUmask := commandChangesUmask(command, "")
		changesUmask077 := commandChangesUmask(command, "077")
		verifies := commandPrintsCurrentUmask(command)
		writesCheck := commandWritesWorkspaceFile(command, TargetUmaskResidueCheckArtifact)
		writesWitness := commandWritesWorkspaceFile(command, TargetUmaskResidueWitnessArtifact)
		baselineMutation := writesBaseline || commandDeletesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact)

		if writesBaseline {
			analysis.BaselineWriteCount++
		}
		if changesUmask {
			analysis.UmaskChangeCount++
		}
		if changesUmask077 {
			analysis.Umask077Count++
			if firstChangeIndex == -1 {
				firstChangeIndex = idx
			}
		}
		if verifies {
			analysis.VerifyCount++
		}
		if firstChangeIndex == -1 || idx <= firstChangeIndex {
			if commandDeletesWorkspaceFile(command, TargetUmaskResidueBaselineArtifact) {
				analysis.InitialBaselineDeletes++
			}
			if writesCheck {
				analysis.InitialCheckWrites++
			}
			if writesWitness {
				analysis.InitialWitnessWrites++
			}
			continue
		}

		if writesCheck {
			analysis.LaterCheckWrites++
		}
		if writesWitness {
			analysis.LaterWitnessWrites++
		}
		if changesUmask {
			analysis.LaterUmaskChanges++
		}
		if baselineMutation {
			analysis.LaterBaselineMutations++
		}
		observation := looksLikeUmaskResidueVerification(command) || writesCheck || writesWitness
		if observation && !changesUmask && !baselineMutation {
			analysis.CleanObservationCall = true
		}
		if observation && (changesUmask || baselineMutation) {
			analysis.RebuiltObservationCall = true
		}
	}
	return analysis
}

func commandProducesCWDResidueCheck(command string) bool {
	if commandWritesWorkspaceFile(command, TargetCWDResidueCheckArtifact) {
		return true
	}
	command = normalizeShellCommand(command)
	return strings.Contains(command, TargetCWDResidueCheckArtifact) &&
		(strings.Contains(command, "present_branch_cwd_residue") ||
			strings.Contains(command, "clean_branch_cwd"))
}

func commandExportsEnvResidue(command string) bool {
	command = normalizeShellCommand(command)
	varName := strings.ToLower(targetEnvResidueVarName)
	marker := strings.ToLower(targetEnvResidueMarker)
	return strings.Contains(command, "export "+varName+"="+marker) ||
		strings.Contains(command, "declare -x "+varName+"="+marker) ||
		strings.Contains(command, varName+"="+marker)
}

func commandMutatesEnvResidue(command string) bool {
	command = normalizeShellCommand(command)
	varName := strings.ToLower(targetEnvResidueVarName)
	return strings.Contains(command, "export "+varName+"=") ||
		strings.Contains(command, "declare -x "+varName+"=") ||
		strings.Contains(command, varName+"=") ||
		strings.Contains(command, "unset "+varName) ||
		strings.Contains(command, "unset -v "+varName)
}

func commandReferencesEnvResidue(command string) bool {
	command = normalizeShellCommand(command)
	varName := strings.ToLower(targetEnvResidueVarName)
	return strings.Contains(command, "${"+varName) ||
		strings.Contains(command, "$"+varName) ||
		strings.Contains(command, "printenv "+varName) ||
		strings.Contains(command, "env | grep "+varName)
}

func commandDefinesFunctionResidue(command string) bool {
	command = normalizeShellCommand(command)
	name := strings.ToLower(targetFunctionResidueName)
	return strings.Contains(command, name+"()") ||
		strings.Contains(command, "function "+name)
}

func commandUnsetsFunctionResidue(command string) bool {
	command = normalizeShellCommand(command)
	name := strings.ToLower(targetFunctionResidueName)
	return strings.Contains(command, "unset -f "+name) ||
		strings.Contains(command, "unset "+name)
}

func commandReferencesFunctionResidue(command string) bool {
	command = normalizeShellCommand(command)
	name := strings.ToLower(targetFunctionResidueName)
	return strings.Contains(command, "type "+name) ||
		strings.Contains(command, "type -t "+name) ||
		strings.Contains(command, name)
}
