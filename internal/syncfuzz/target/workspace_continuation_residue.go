package target

import (
	"fmt"
	"strings"
)

type workspaceContinuationCommandAnalysis struct {
	CallCount              int
	CreateCount            int
	BoundaryCount          int
	VerifyCount            int
	InitialWitnessWrites   int
	LaterWitnessWrites     int
	LaterMutations         int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

func evaluateWorkspaceContinuationTargetTaskCompliance(workspace string, targetID string, taskID string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   taskID,
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

	analysis := analyzeWorkspaceContinuationTaskCommands(taskID, trace.Commands)
	createRequirement, boundaryRequirement, verifyRequirement, laterMutationRequirement, cleanObservationRequirement := workspaceContinuationComplianceRequirements(taskID)

	requireExactCount(&result, analysis.CreateCount, 1, createRequirement)
	requireExactCount(&result, analysis.BoundaryCount, 1, boundaryRequirement)
	requireAtLeastOne(&result, analysis.VerifyCount, verifyRequirement)
	requireExactCount(&result, analysis.InitialWitnessWrites, 0, "initial shell steps did not create "+workspaceContinuationWitnessArtifact(taskID))
	requireAtLeastOne(&result, analysis.LaterWitnessWrites, "later shell step wrote "+workspaceContinuationWitnessArtifact(taskID))
	requireExactCount(&result, analysis.LaterMutations, 0, laterMutationRequirement)
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, cleanObservationRequirement)
	} else {
		appendTargetTaskViolation(&result, cleanObservationRequirement)
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateWorkspaceContinuationTargetOracle(workspace string, targetID string, taskID string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult(taskID)
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

	witnessArtifact := workspaceContinuationWitnessArtifact(taskID)
	witness, err := readTargetOracleFile(workspace, witnessArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+witnessArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")

	witnessKind := classifyWorkspaceContinuationWitness(taskID, witness, &oracle)

	trace, err := loadShellCommandTrace(workspace, targetID)
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, workspaceContinuationTraceRequirement(taskID))
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" artifact was not available for the residue oracle")
		markTargetOracleInconclusive(&oracle, workspaceContinuationTraceRequirement(taskID))
		return finalizeTargetOracle(oracle)
	}

	analysis := analyzeWorkspaceContinuationTaskCommands(taskID, trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for the residue oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))

	createRequirement, boundaryRequirement, _, _, cleanObservationRequirement := workspaceContinuationComplianceRequirements(taskID)
	if analysis.CreateCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the initial setup mutation")
	} else {
		markTargetOracleInconclusive(&oracle, createRequirement)
	}
	if analysis.BoundaryCount > 0 {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" captured the boundary mutation")
	} else {
		markTargetOracleInconclusive(&oracle, boundaryRequirement)
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, trace.Source+" showed the later observation call without reconstructing the residue")
	} else {
		switch {
		case analysis.RebuiltObservationCall:
			oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
			markTargetOracleNegative(&oracle, cleanObservationRequirement)
		default:
			markTargetOracleInconclusive(&oracle, workspaceContinuationTraceRequirement(taskID))
		}
	}

	switch witnessKind {
	case "residue":
		if analysis.CleanObservationCall {
			oracle.Attribution = TargetOracleAttributionRuntimeResidue
		}
	case "clean":
		if analysis.CleanObservationCall {
			markTargetOracleNegative(&oracle, workspaceContinuationPreserveRequirement(taskID))
		}
	}
	return finalizeTargetOracle(oracle)
}

func analyzeWorkspaceContinuationTaskCommands(taskID string, commands []string) workspaceContinuationCommandAnalysis {
	switch taskID {
	case FileResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetFileResidueNoteArtifact)
			},
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetFileResidueNoteArtifact)
			},
			looksLikeFileResidueVerification,
			func(command string) bool { return commandWritesWorkspaceFile(command, TargetFileResidueCheckArtifact) },
			func(command string) bool { return commandMutatesWorkspacePath(command, TargetFileResidueNoteArtifact) },
		)
	case DirectoryResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceDirectory(command, TargetDirectoryResidueDirArtifact)
			},
			func(command string) bool {
				return commandCreatesWorkspaceDirectory(command, TargetDirectoryResidueDirArtifact)
			},
			looksLikeDirectoryResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetDirectoryResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetDirectoryResidueDirArtifact)
			},
		)
	case DeleteResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetDeleteResidueNoteArtifact)
			},
			func(command string) bool {
				return commandDeletesWorkspaceFile(command, TargetDeleteResidueNoteArtifact)
			},
			looksLikeDeleteResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetDeleteResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetDeleteResidueNoteArtifact)
			},
		)
	case SymlinkResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceSymlink(command, TargetSymlinkResidueLinkArtifact)
			},
			func(command string) bool {
				return commandCreatesWorkspaceSymlink(command, TargetSymlinkResidueLinkArtifact)
			},
			looksLikeSymlinkResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetSymlinkResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetSymlinkResidueLinkArtifact)
			},
		)
	case RenameResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetRenameResidueSourceArtifact)
			},
			func(command string) bool {
				return commandRenamesWorkspaceFile(command, TargetRenameResidueSourceArtifact, TargetRenameResidueDestArtifact)
			},
			looksLikeRenameResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetRenameResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetRenameResidueSourceArtifact) ||
					commandMutatesWorkspacePath(command, TargetRenameResidueDestArtifact)
			},
		)
	case ModeResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetModeResidueNoteArtifact)
			},
			func(command string) bool {
				return commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, targetModeResidueTightenedMode)
			},
			looksLikeModeResidueVerification,
			func(command string) bool { return commandWritesWorkspaceFile(command, TargetModeResidueCheckArtifact) },
			func(command string) bool { return commandMutatesWorkspacePath(command, TargetModeResidueNoteArtifact) },
		)
	case AppendResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceResidueFile(command, TargetAppendResidueNoteArtifact)
			},
			func(command string) bool {
				return commandAppendsWorkspaceFile(command, TargetAppendResidueNoteArtifact)
			},
			looksLikeAppendResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetAppendResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetAppendResidueNoteArtifact)
			},
		)
	case HardlinkResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool {
				return commandCreatesWorkspaceHardlink(command, TargetHardlinkResidueLinkArtifact)
			},
			func(command string) bool {
				return commandCreatesWorkspaceHardlink(command, TargetHardlinkResidueLinkArtifact)
			},
			looksLikeHardlinkResidueVerification,
			func(command string) bool {
				return commandWritesWorkspaceFile(command, TargetHardlinkResidueCheckArtifact)
			},
			func(command string) bool {
				return commandMutatesWorkspacePath(command, TargetHardlinkResidueLinkArtifact)
			},
		)
	case FifoResidueTargetTaskID:
		return analyzeWorkspaceContinuationCommands(
			commands,
			func(command string) bool { return commandCreatesWorkspaceFIFO(command, TargetFIFOResiduePipeArtifact) },
			func(command string) bool { return commandCreatesWorkspaceFIFO(command, TargetFIFOResiduePipeArtifact) },
			looksLikeFIFOResidueVerification,
			func(command string) bool { return commandWritesWorkspaceFile(command, TargetFIFOResidueCheckArtifact) },
			func(command string) bool { return commandMutatesWorkspacePath(command, TargetFIFOResiduePipeArtifact) },
		)
	default:
		return workspaceContinuationCommandAnalysis{CallCount: len(commands)}
	}
}

func analyzeWorkspaceContinuationCommands(
	commands []string,
	createFn func(string) bool,
	boundaryFn func(string) bool,
	verifyFn func(string) bool,
	witnessFn func(string) bool,
	mutationFn func(string) bool,
) workspaceContinuationCommandAnalysis {
	analysis := workspaceContinuationCommandAnalysis{CallCount: len(commands)}
	boundaryIndex := -1

	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		creates := createFn(command)
		boundary := boundaryFn(command)
		verifies := verifyFn(command)
		writesWitness := witnessFn(command)
		mutates := mutationFn(command)

		if creates {
			analysis.CreateCount++
		}
		if boundary {
			analysis.BoundaryCount++
			if boundaryIndex == -1 {
				boundaryIndex = idx
			}
		}
		if boundaryIndex == -1 || idx <= boundaryIndex {
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
			analysis.LaterMutations++
		}
		observation := verifies
		if observation && !mutates {
			analysis.CleanObservationCall = true
		}
		if observation && mutates {
			analysis.RebuiltObservationCall = true
		}
	}

	return analysis
}

func workspaceContinuationComplianceRequirements(taskID string) (string, string, string, string, string) {
	switch taskID {
	case FileResidueTargetTaskID:
		return "task created branch-note.txt exactly once",
			"task performed the file-drop boundary exactly once",
			"task verified branch-note.txt during a later shell step",
			"later shell step did not recreate or mutate branch-note.txt",
			"later shell step observed branch-note.txt without reconstructing it"
	case DirectoryResidueTargetTaskID:
		return "task created branch-dir exactly once",
			"task performed the directory-create boundary exactly once",
			"task verified branch-dir during a later shell step",
			"later shell step did not recreate or mutate branch-dir",
			"later shell step observed branch-dir without reconstructing it"
	case DeleteResidueTargetTaskID:
		return "task created branch-delete-note.txt exactly once",
			"task deleted branch-delete-note.txt exactly once",
			"task verified branch-delete-note.txt absence during a later shell step",
			"later shell step did not recreate or mutate branch-delete-note.txt",
			"later shell step observed the deleted state without recreating branch-delete-note.txt"
	case SymlinkResidueTargetTaskID:
		return "task created branch-link.txt exactly once",
			"task performed the symlink-create boundary exactly once",
			"task verified branch-link.txt during a later shell step",
			"later shell step did not recreate or mutate branch-link.txt",
			"later shell step observed branch-link.txt without reconstructing it"
	case RenameResidueTargetTaskID:
		return "task created branch-rename-src.txt exactly once",
			"task renamed branch-rename-src.txt to branch-rename-dst.txt exactly once",
			"task verified the rename result during a later shell step",
			"later shell step did not recreate or mutate the rename state",
			"later shell step observed the rename result without reconstructing it"
	case ModeResidueTargetTaskID:
		return "task created branch-mode-note.txt exactly once",
			"task chmod-ed branch-mode-note.txt to mode " + targetModeResidueTightenedMode + " exactly once",
			"task verified branch-mode-note.txt mode during a later shell step",
			"later shell step did not recreate or mutate branch-mode-note.txt",
			"later shell step observed the tightened file mode without reconstructing it"
	case AppendResidueTargetTaskID:
		return "task created branch-append-note.txt exactly once",
			"task appended the extra marker to branch-append-note.txt exactly once",
			"task verified branch-append-note.txt content during a later shell step",
			"later shell step did not recreate or mutate branch-append-note.txt",
			"later shell step observed the appended content without reconstructing it"
	case HardlinkResidueTargetTaskID:
		return "task created branch-hardlink.txt exactly once",
			"task performed the hardlink-create boundary exactly once",
			"task verified branch-hardlink.txt during a later shell step",
			"later shell step did not recreate or mutate branch-hardlink.txt",
			"later shell step observed branch-hardlink.txt without reconstructing it"
	case FifoResidueTargetTaskID:
		return "task created branch-fifo exactly once",
			"task performed the fifo-create boundary exactly once",
			"task verified branch-fifo during a later shell step",
			"later shell step did not recreate or mutate branch-fifo",
			"later shell step observed branch-fifo without reconstructing it"
	default:
		return "task created the workspace object exactly once",
			"task performed the boundary mutation exactly once",
			"task verified the workspace object during a later shell step",
			"later shell step did not recreate or mutate the workspace object",
			"later shell step observed the workspace object without reconstructing it"
	}
}

func workspaceContinuationWitnessArtifact(taskID string) string {
	if spec, ok := WorkspaceContinuationTaskSpecByID(taskID); ok && len(spec.ExpectedFiles) > 0 {
		return spec.ExpectedFiles[0]
	}
	return ""
}

func workspaceContinuationTraceRequirement(taskID string) string {
	switch taskID {
	case DeleteResidueTargetTaskID:
		return "shell command trace proved the later witness came from observing the deleted state without recreating the file"
	case RenameResidueTargetTaskID:
		return "shell command trace proved the later witness came from observing the rename state without renaming again"
	case ModeResidueTargetTaskID:
		return "shell command trace proved the later witness came from observing the tightened file mode without chmod again"
	case AppendResidueTargetTaskID:
		return "shell command trace proved the later witness came from observing appended content without appending again"
	default:
		return "shell command trace proved the later witness came from observing pre-existing workspace state without reconstructing it"
	}
}

func workspaceContinuationPreserveRequirement(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "later shell call preserved branch-note.txt across shell calls"
	case DirectoryResidueTargetTaskID:
		return "later shell call preserved branch-dir across shell calls"
	case DeleteResidueTargetTaskID:
		return "later shell call preserved the deleted state of branch-delete-note.txt across shell calls"
	case SymlinkResidueTargetTaskID:
		return "later shell call preserved branch-link.txt across shell calls"
	case RenameResidueTargetTaskID:
		return "later shell call preserved the branch-rename-dst.txt binding across shell calls"
	case ModeResidueTargetTaskID:
		return "later shell call preserved the tightened file mode across shell calls"
	case AppendResidueTargetTaskID:
		return "later shell call preserved the appended content across shell calls"
	case HardlinkResidueTargetTaskID:
		return "later shell call preserved branch-hardlink.txt across shell calls"
	case FifoResidueTargetTaskID:
		return "later shell call preserved branch-fifo across shell calls"
	default:
		return "later shell call preserved the workspace residue across shell calls"
	}
}

func classifyWorkspaceContinuationWitness(taskID string, witness string, oracle *TargetOracleResult) string {
	switch taskID {
	case FileResidueTargetTaskID:
		switch {
		case strings.Contains(witness, targetFileResidueMarker):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-note.txt still contained the expected marker")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_NOTE"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-note.txt was absent in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness contained a recognizable file residue marker")
		}
	case DirectoryResidueTargetTaskID:
		switch {
		case strings.Contains(witness, "PRESENT_BRANCH_DIR"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-dir still existed in the later shell step")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_DIR"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-dir was absent in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness contained a recognizable directory residue marker")
		}
	case DeleteResidueTargetTaskID:
		switch {
		case strings.Contains(witness, "MISSING_BRANCH_DELETE_NOTE"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-delete-note.txt still remained deleted in the later shell step")
			return "residue"
		case strings.Contains(witness, "PRESENT_BRANCH_DELETE_NOTE"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-delete-note.txt was present again in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness contained a recognizable delete residue marker")
		}
	case SymlinkResidueTargetTaskID:
		switch {
		case strings.TrimSpace(witness) == "target-prompt.txt":
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-link.txt still pointed at target-prompt.txt")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_LINK"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-link.txt was absent in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness contained a recognizable symlink residue marker")
		}
	case RenameResidueTargetTaskID:
		dst := strings.TrimSpace(targetOracleLineValue(witness, "DST_PRESENT="))
		src := strings.TrimSpace(targetOracleLineValue(witness, "SRC_PRESENT="))
		if dst != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded destination presence: "+dst)
		}
		if src != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded source presence: "+src)
		}
		switch {
		case dst == "yes" && src == "no":
			oracle.Evidence = append(oracle.Evidence, "witness reported that the renamed destination state still held in the later shell step")
			return "residue"
		case dst == "no" && src == "yes":
			oracle.Evidence = append(oracle.Evidence, "witness reported that the original source binding was present again in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness recorded a recognizable rename residue state")
		}
	case ModeResidueTargetTaskID:
		mode := strings.TrimSpace(targetOracleLineValue(witness, "MODE="))
		if mode != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded file mode: "+mode)
		}
		switch {
		case mode == targetModeResidueTightenedMode:
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-mode-note.txt still had mode "+targetModeResidueTightenedMode)
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_MODE_NOTE"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-mode-note.txt was absent in the later shell step")
			return "clean"
		case mode != "":
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-mode-note.txt no longer had the tightened "+targetModeResidueTightenedMode+" mode")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness recorded a recognizable mode residue state")
		}
	case AppendResidueTargetTaskID:
		base := strings.TrimSpace(targetOracleLineValue(witness, "BASE_COUNT="))
		appended := strings.TrimSpace(targetOracleLineValue(witness, "APPEND_COUNT="))
		if base != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded base marker count: "+base)
		}
		if appended != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded appended marker count: "+appended)
		}
		switch {
		case base == "1" && appended == "1":
			oracle.Evidence = append(oracle.Evidence, "witness reported that the appended marker still remained in branch-append-note.txt")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_APPEND_NOTE"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-append-note.txt was absent in the later shell step")
			return "clean"
		case base == "1" && appended == "0":
			oracle.Evidence = append(oracle.Evidence, "witness reported that the appended marker was missing in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness recorded a recognizable append residue state")
		}
	case HardlinkResidueTargetTaskID:
		targetInode := strings.TrimSpace(targetOracleLineValue(witness, "TARGET_INODE="))
		linkInode := strings.TrimSpace(targetOracleLineValue(witness, "LINK_INODE="))
		if targetInode != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded target inode: "+targetInode)
		}
		if linkInode != "" {
			oracle.Evidence = append(oracle.Evidence, "witness recorded hardlink inode: "+linkInode)
		}
		switch {
		case strings.Contains(witness, "PRESENT_BRANCH_HARDLINK") && targetInode != "" && targetInode == linkInode:
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-hardlink.txt still shared the target inode")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_HARDLINK"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-hardlink.txt was absent or no longer shared the target inode")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness recorded a recognizable hardlink residue state")
		}
	case FifoResidueTargetTaskID:
		switch {
		case strings.Contains(witness, "PRESENT_BRANCH_FIFO"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-fifo still existed as a named pipe")
			return "residue"
		case strings.Contains(witness, "MISSING_BRANCH_FIFO"):
			oracle.Evidence = append(oracle.Evidence, "witness reported that branch-fifo was absent in the later shell step")
			return "clean"
		default:
			markTargetOracleInconclusive(oracle, "witness contained a recognizable fifo residue marker")
		}
	}
	return ""
}

func commandCreatesWorkspaceResidueFile(command string, name string) bool {
	return commandWritesWorkspaceFile(command, name) && !commandAppendsWorkspaceFile(command, name)
}

func commandMutatesWorkspacePath(command string, name string) bool {
	command = normalizeShellCommand(command)
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if commandCreatesWorkspaceResidueFile(command, normalizedName) ||
		commandWritesWorkspaceFile(command, normalizedName) ||
		commandAppendsWorkspaceFile(command, normalizedName) ||
		commandDeletesWorkspaceFile(command, normalizedName) ||
		commandCreatesWorkspaceDirectory(command, normalizedName) ||
		commandCreatesWorkspaceSymlink(command, normalizedName) ||
		commandCreatesWorkspaceHardlink(command, normalizedName) ||
		commandCreatesWorkspaceFIFO(command, normalizedName) {
		return true
	}
	if strings.Contains(command, "chmod ") && commandMentionsWorkspacePath(command, normalizedName) {
		return true
	}
	if strings.Contains(command, "mv ") && commandMentionsWorkspacePath(command, normalizedName) {
		return true
	}
	return false
}

func commandMentionsWorkspacePath(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.HasSuffix(command, " "+name) ||
		strings.Contains(command, " "+name+" ") ||
		strings.Contains(command, "/"+name) ||
		strings.Contains(command, name+";") ||
		strings.Contains(command, name+" &&") ||
		strings.Contains(command, name+" ||")
}
