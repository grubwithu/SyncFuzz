package target

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mafWorkflowCheckpointArtifactData struct {
	SchemaVersion               string   `json:"schema_version"`
	WorkflowName                string   `json:"workflow_name"`
	CheckpointBackend           string   `json:"checkpoint_backend"`
	CheckpointDir               string   `json:"checkpoint_dir"`
	CheckpointIDs               []string `json:"checkpoint_ids"`
	SelectedCheckpointID        string   `json:"selected_checkpoint_id"`
	SelectedCheckpointIteration int      `json:"selected_checkpoint_iteration"`
	Restored                    bool     `json:"restored"`
	RuntimeObjectRecreated      bool     `json:"runtime_object_recreated"`
	PreRestoreTimedOut          bool     `json:"pre_restore_timed_out"`
	PostRestoreTimedOut         bool     `json:"post_restore_timed_out"`
	EffectWritten               bool     `json:"effect_written"`
	ContinuityObserved          bool     `json:"continuity_observed"`
	DuplicateEffectObserved     bool     `json:"duplicate_effect_observed"`
	ExternalEffectEntries       int      `json:"external_effect_entries"`
	OperationID                 string   `json:"operation_id"`
	PostRestoreOutputs          []string `json:"post_restore_outputs"`
}

func evaluateMAFWorkflowCheckpointTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-checkpoint-continuity")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowContinuityArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowContinuityArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow witness: "+witness)
	if !strings.Contains(witness, "PRESENT_MAF_WORKFLOW_MARKER") || !strings.Contains(witness, targetMAFWorkflowMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow executor observed the planted marker")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved file-checkpoint restore")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for checkpoint oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved file-checkpoint restore")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d file checkpoints", len(summary.CheckpointIDs)))
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if strings.TrimSpace(summary.SelectedCheckpointID) != "" {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary selected a concrete checkpoint id")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary selected a concrete checkpoint id")
		}
		if summary.EffectWritten && summary.ContinuityObserved {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded pre-restore effect and post-restore observation")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded pre-restore effect and post-restore observation")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowExternalReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-external-effect-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowExternalReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowExternalReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow external replay witness: "+witness)
	if !strings.Contains(witness, "DUPLICATE_MAF_WORKFLOW_EXTERNAL_EFFECT") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow replayed the external effect")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved external-effect replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for external replay oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved external-effect replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d external effect entries", summary.ExternalEffectEntries))
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate external effect entries for one operation")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate external effect entries for one operation")
		}
		if strings.TrimSpace(summary.SelectedCheckpointID) != "" {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary selected a concrete checkpoint id")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary selected a concrete checkpoint id")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowCheckpointTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowCheckpointTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded checkpoint restore")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("checkpoint ids: %d", len(summary.CheckpointIDs)))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if strings.TrimSpace(summary.SelectedCheckpointID) != "" {
		appendTargetTaskEvidence(&result, "workflow selected a checkpoint for restore")
	} else {
		appendTargetTaskViolation(&result, "workflow selected a checkpoint for restore")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.EffectWritten && summary.ContinuityObserved {
		appendTargetTaskEvidence(&result, "post-restore executor observed the pre-restore workspace effect")
	} else {
		appendTargetTaskViolation(&result, "post-restore executor observed the pre-restore workspace effect")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFWorkflowExternalReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowExternalReplayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded external-effect replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("external effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "one logical workflow operation produced duplicate external effect entries")
	} else {
		appendTargetTaskViolation(&result, "one logical workflow operation produced duplicate external effect entries")
	}
	return finalizeTargetTaskCompliance(result)
}

func loadMAFWorkflowArtifact(workspace string) (mafWorkflowCheckpointArtifactData, bool, error) {
	path := filepath.Join(workspace, mafWorkflowArtifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mafWorkflowCheckpointArtifactData{}, false, nil
		}
		return mafWorkflowCheckpointArtifactData{}, false, fmt.Errorf("read %s: %w", mafWorkflowArtifact, err)
	}
	var summary mafWorkflowCheckpointArtifactData
	if err := json.Unmarshal(raw, &summary); err != nil {
		return mafWorkflowCheckpointArtifactData{}, true, fmt.Errorf("decode %s: %w", mafWorkflowArtifact, err)
	}
	return summary, true, nil
}
