package target

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mafSessionContinuityAnalysis struct {
	CallCount              int
	PlantWrites            int
	CheckWrites            int
	LaterCheckWrites       int
	LaterPlantMutations    int
	CleanObservationCall   bool
	RebuiltObservationCall bool
}

type mafSessionArtifactData struct {
	SchemaVersion             string `json:"schema_version"`
	Discovered                bool   `json:"discovered"`
	Source                    string `json:"source"`
	ServiceSessionID          string `json:"service_session_id"`
	SessionID                 string `json:"session_id"`
	Restored                  bool   `json:"restored"`
	RestoreMode               string `json:"restore_mode"`
	RuntimeObjectRecreated    bool   `json:"runtime_object_recreated"`
	SerializedSessionSHA256   string `json:"serialized_session_sha256"`
	RestoredSessionID         string `json:"restored_session_id"`
	RestoredServiceSessionID  string `json:"restored_service_session_id"`
	PreRestoreResponseSHA256  string `json:"pre_restore_response_sha256"`
	PostRestoreResponseSHA256 string `json:"post_restore_response_sha256"`
}

func evaluateMAFSessionContinuityTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-session-continuity")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFSessionContinuityArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFSessionContinuityArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF session witness: "+witness)
	if !strings.Contains(witness, "PRESENT_MAF_SESSION_MARKER") || !strings.Contains(witness, targetMAFSessionMarker) {
		markTargetOracleNegative(&oracle, "restored MAF turn observed the planted session marker")
		return finalizeTargetOracle(oracle)
	}

	session, ok, err := loadMAFSessionArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf session artifact proved serialized AgentSession restore")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf session artifact was not available for session-continuity oracle")
		markTargetOracleInconclusive(&oracle, "maf session artifact proved serialized AgentSession restore")
	default:
		if session.Restored && session.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf session artifact recorded serialized AgentSession restore on a recreated runtime object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf session artifact proved serialized AgentSession restore")
		}
		if strings.TrimSpace(session.SerializedSessionSHA256) != "" {
			oracle.Evidence = append(oracle.Evidence, "maf session artifact recorded serialized session state")
		} else {
			markTargetOracleInconclusive(&oracle, "maf session artifact recorded serialized session state")
		}
		if strings.TrimSpace(session.SessionID) != "" && session.SessionID == session.RestoredSessionID {
			oracle.Evidence = append(oracle.Evidence, "restored AgentSession preserved the logical session id")
		}
	}

	trace, err := loadShellCommandTrace(workspace, "maf-github-copilot-shell")
	if err != nil {
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf lifecycle proved the witness came from a post-restore shell call")
		return finalizeTargetOracle(oracle)
	}
	if !trace.Available {
		oracle.Evidence = append(oracle.Evidence, "maf lifecycle artifact was not available for session-continuity oracle")
		markTargetOracleInconclusive(&oracle, "maf lifecycle proved the witness came from a post-restore shell call")
		return finalizeTargetOracle(oracle)
	}
	analysis := analyzeMAFSessionContinuityCommands(trace.Commands)
	oracle.Evidence = append(oracle.Evidence, trace.Source+" was available for session-continuity oracle")
	oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("observed shell calls: %d", analysis.CallCount))
	if analysis.PlantWrites > 0 {
		oracle.Evidence = append(oracle.Evidence, "maf lifecycle captured the pre-restore marker write")
	} else {
		markTargetOracleInconclusive(&oracle, "maf lifecycle captured the pre-restore marker write")
	}
	if analysis.CleanObservationCall {
		oracle.Evidence = append(oracle.Evidence, "maf lifecycle showed a later check call without recreating the marker")
	} else if analysis.RebuiltObservationCall {
		oracle.Attribution = TargetOracleAttributionWorkspaceRebuild
		markTargetOracleNegative(&oracle, "post-restore check observed the marker without recreating it")
	} else {
		markTargetOracleInconclusive(&oracle, "maf lifecycle proved the witness came from a post-restore shell call")
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFSessionContinuityTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFSessionContinuityTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	trace, err := loadShellCommandTrace(workspace, "maf-github-copilot-shell")
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

	analysis := analyzeMAFSessionContinuityCommands(trace.Commands)
	requireAtLeastOne(&result, analysis.PlantWrites, "pre-restore shell call wrote maf-session-plant.txt")
	requireAtLeastOne(&result, analysis.CheckWrites, "post-restore shell call wrote maf-session-continuity-check.txt")
	requireAtLeastOne(&result, analysis.LaterCheckWrites, "continuity check happened after the plant step")
	requireExactCount(&result, analysis.LaterPlantMutations, 0, "post-restore check did not recreate maf-session-plant.txt")
	if analysis.CleanObservationCall {
		appendTargetTaskEvidence(&result, "post-restore shell call observed the marker without recreating it")
	} else {
		appendTargetTaskViolation(&result, "post-restore shell call observed the marker without recreating it")
	}

	session, ok, err := loadMAFSessionArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return finalizeTargetTaskCompliance(result)
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf session artifact recorded serialized AgentSession restore")
		return finalizeTargetTaskCompliance(result)
	}
	if session.Restored && session.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "maf session artifact recorded serialized AgentSession restore on a recreated runtime object")
	} else {
		appendTargetTaskViolation(&result, "maf session artifact recorded serialized AgentSession restore on a recreated runtime object")
	}
	if strings.TrimSpace(session.SerializedSessionSHA256) != "" {
		appendTargetTaskEvidence(&result, "maf session artifact recorded serialized session state")
	} else {
		appendTargetTaskViolation(&result, "maf session artifact recorded serialized session state")
	}
	return finalizeTargetTaskCompliance(result)
}

func loadMAFSessionArtifact(workspace string) (mafSessionArtifactData, bool, error) {
	path := filepath.Join(workspace, mafSessionArtifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mafSessionArtifactData{}, false, nil
		}
		return mafSessionArtifactData{}, false, fmt.Errorf("read %s: %w", mafSessionArtifact, err)
	}
	var session mafSessionArtifactData
	if err := json.Unmarshal(raw, &session); err != nil {
		return mafSessionArtifactData{}, true, fmt.Errorf("decode %s: %w", mafSessionArtifact, err)
	}
	return session, true, nil
}

func analyzeMAFSessionContinuityCommands(commands []string) mafSessionContinuityAnalysis {
	analysis := mafSessionContinuityAnalysis{CallCount: len(commands)}
	firstPlantIndex := -1
	for idx, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		writesPlant := commandWritesWorkspaceFile(command, TargetMAFSessionPlantArtifact)
		writesCheck := commandWritesWorkspaceFile(command, TargetMAFSessionContinuityArtifact)
		referencesMarker := strings.Contains(command, TargetMAFSessionPlantArtifact) || strings.Contains(command, targetMAFSessionMarker)

		if writesPlant {
			analysis.PlantWrites++
			if firstPlantIndex == -1 {
				firstPlantIndex = idx
			}
		}
		if writesCheck {
			analysis.CheckWrites++
		}
		if firstPlantIndex == -1 || idx <= firstPlantIndex {
			continue
		}

		if writesCheck {
			analysis.LaterCheckWrites++
		}
		if writesPlant {
			analysis.LaterPlantMutations++
		}
		if writesCheck && referencesMarker && !writesPlant {
			analysis.CleanObservationCall = true
		}
		if writesCheck && writesPlant {
			analysis.RebuiltObservationCall = true
		}
	}
	return analysis
}
