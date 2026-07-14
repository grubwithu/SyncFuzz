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
	ExternalServiceObserved     bool     `json:"external_service_observed"`
	ExternalServiceURL          string   `json:"external_service_url"`
	ExternalServiceMode         string   `json:"external_service_mode"`
	AuthorityTokenIssued        bool     `json:"authority_token_issued"`
	AuthorityTokenConsumed      bool     `json:"authority_token_consumed"`
	AuthorityReplayConflict     bool     `json:"authority_replay_conflict_observed"`
	InitialFailureObserved      bool     `json:"initial_failure_observed"`
	PartialCommitObserved       bool     `json:"partial_commit_observed"`
	PendingRequestObserved      bool     `json:"pending_request_observed"`
	ApprovalResponseObserved    bool     `json:"approval_response_observed"`
	ApprovalReplayObserved      bool     `json:"approval_replay_observed"`
	SameInstanceResumeObserved  bool     `json:"same_instance_resume_observed"`
	RehydrateReplayObserved     bool     `json:"rehydrate_replay_observed"`
	OperationID                 string   `json:"operation_id"`
	ApprovalRequestID           string   `json:"approval_request_id"`
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

func evaluateMAFWorkflowHTTPReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-http-effect-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowHTTPReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowHTTPReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow HTTP replay witness: "+witness)
	if !strings.Contains(witness, "DUPLICATE_MAF_WORKFLOW_HTTP_EFFECT") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow replayed an HTTP external service effect")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved HTTP external service replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for HTTP replay oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved HTTP external service replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d HTTP service effect entries", summary.ExternalEffectEntries))
		if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
			mode := strings.TrimSpace(summary.ExternalServiceMode)
			if mode == "" {
				mode = "unknown"
			}
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded calls to an HTTP external service in "+mode+" mode")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded calls to an HTTP external service")
		}
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate HTTP service effects for one operation")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate HTTP service effects for one operation")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowResourceReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-resource-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowResourceReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowResourceReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow resource replay witness: "+witness)
	if !strings.Contains(witness, "DUPLICATE_MAF_WORKFLOW_RESOURCE_EFFECT") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow replayed an external resource creation")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved resource service replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for resource replay oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved resource service replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d resource service entries", summary.ExternalEffectEntries))
		if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
			mode := strings.TrimSpace(summary.ExternalServiceMode)
			if mode == "" {
				mode = "unknown"
			}
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded calls to an HTTP resource service in "+mode+" mode")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded calls to an HTTP resource service")
		}
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate resource creations for one operation")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate resource creations for one operation")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowAuthorityReplayTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-authority-token-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowAuthorityReplayArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowAuthorityReplayArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow authority replay witness: "+witness)
	if !strings.Contains(witness, "AUTHORITY_TOKEN_REPLAY_CONFLICT") || !strings.Contains(witness, targetMAFWorkflowAuthorityMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow observed consumed authority token state")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved authority token replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for authority replay oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved authority token replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d authority service events", summary.ExternalEffectEntries))
		if summary.AuthorityTokenIssued && summary.AuthorityTokenConsumed {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded token issue and first consume before restore")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded token issue and first consume before restore")
		}
		if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
			mode := strings.TrimSpace(summary.ExternalServiceMode)
			if mode == "" {
				mode = "unknown"
			}
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded calls to an authority service in "+mode+" mode")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded calls to an authority service")
		}
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if summary.AuthorityReplayConflict {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded token_already_consumed on replay")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded token_already_consumed on replay")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowPartialCommitTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-partial-commit-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowPartialCommitArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowPartialCommitArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow partial commit witness: "+witness)
	if !strings.Contains(witness, "DUPLICATE_PARTIAL_COMMIT_REPLAY") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow replayed a partially committed parallel effect")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved partial commit replay replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for partial commit oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved partial commit replay replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d external effect entries", summary.ExternalEffectEntries))
		if summary.InitialFailureObserved && summary.PartialCommitObserved {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded initial branch failure after one partial external commit")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded initial branch failure after one partial external commit")
		}
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded restore on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded restore on a recreated workflow object")
		}
		if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate external commits after restore")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate external commits after restore")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowApprovalPendingTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-approval-pending-replay")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowApprovalPendingArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowApprovalPendingArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow approval-pending witness: "+witness)
	if !strings.Contains(witness, "DUPLICATE_APPROVAL_PENDING_REPLAY") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "restored MAF workflow replayed an approved pending request")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved approval-pending replay")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for approval-pending oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved approval-pending replay")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d external effect entries", summary.ExternalEffectEntries))
		if summary.PendingRequestObserved && strings.TrimSpace(summary.ApprovalRequestID) != "" {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded a pending request-info approval")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded a pending request-info approval")
		}
		if summary.Restored && summary.RuntimeObjectRecreated {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded approval response on recreated workflow objects")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded approval response on recreated workflow objects")
		}
		if summary.ApprovalResponseObserved && summary.ApprovalReplayObserved && summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate approved effects from one pending request")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate approved effects from one pending request")
		}
	}

	if oracle.Confirmed && oracle.Attribution == TargetOracleAttributionUnknown {
		oracle.Attribution = TargetOracleAttributionRuntimeResidue
	}
	return finalizeTargetOracle(oracle)
}

func evaluateMAFWorkflowRehydrateDivergenceTargetOracle(workspace string, completed bool, immediateMissing []string) TargetOracleResult {
	oracle := newTargetOracleResult("maf-workflow-rehydrate-divergence")
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

	witness, err := readTargetOracleFile(workspace, TargetMAFWorkflowRehydrateDivergenceArtifact)
	if err != nil {
		markTargetOracleInconclusive(&oracle, "read "+TargetMAFWorkflowRehydrateDivergenceArtifact)
		oracle.Evidence = append(oracle.Evidence, err.Error())
		return finalizeTargetOracle(oracle)
	}
	oracle.Evidence = append(oracle.Evidence, "immediate expected file checks passed")
	oracle.Evidence = append(oracle.Evidence, "observed MAF workflow rehydrate divergence witness: "+witness)
	if !strings.Contains(witness, "REHYDRATE_DIVERGENCE_REPLAY") || !strings.Contains(witness, targetMAFWorkflowExternalMarker) {
		markTargetOracleNegative(&oracle, "recreated MAF workflow replay diverged from same-instance resume")
		return finalizeTargetOracle(oracle)
	}

	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	switch {
	case err != nil:
		oracle.Evidence = append(oracle.Evidence, err.Error())
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved resume-vs-rehydrate divergence")
	case !ok:
		oracle.Evidence = append(oracle.Evidence, "maf workflow summary artifact was not available for rehydrate divergence oracle")
		markTargetOracleInconclusive(&oracle, "maf workflow summary proved resume-vs-rehydrate divergence")
	default:
		oracle.Evidence = append(oracle.Evidence, fmt.Sprintf("maf workflow summary recorded %d external effect entries", summary.ExternalEffectEntries))
		if summary.PendingRequestObserved && strings.TrimSpace(summary.ApprovalRequestID) != "" {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded a pending request-info approval")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow summary recorded a pending request-info approval")
		}
		if summary.ApprovalResponseObserved && summary.SameInstanceResumeObserved {
			oracle.Evidence = append(oracle.Evidence, "maf workflow same-instance resume consumed the approval once")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow same-instance resume consumed the approval once")
		}
		if summary.Restored && summary.RuntimeObjectRecreated && summary.RehydrateReplayObserved {
			oracle.Evidence = append(oracle.Evidence, "maf workflow rehydrate replay ran on a recreated workflow object")
		} else {
			markTargetOracleInconclusive(&oracle, "maf workflow rehydrate replay ran on a recreated workflow object")
		}
		if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
			oracle.Evidence = append(oracle.Evidence, "maf workflow summary recorded duplicate effects only after checkpoint rehydrate")
		} else {
			markTargetOracleNegative(&oracle, "maf workflow summary recorded duplicate effects only after checkpoint rehydrate")
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

func evaluateMAFWorkflowPartialCommitTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowPartialCommitTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded partial commit replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("external effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.InitialFailureObserved && summary.PartialCommitObserved {
		appendTargetTaskEvidence(&result, "initial workflow run failed after one partial external commit")
	} else {
		appendTargetTaskViolation(&result, "initial workflow run failed after one partial external commit")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "restored workflow duplicated the partially committed external effect")
	} else {
		appendTargetTaskViolation(&result, "restored workflow duplicated the partially committed external effect")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFWorkflowApprovalPendingTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowApprovalPendingTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded approval-pending replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("external effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.PendingRequestObserved && strings.TrimSpace(summary.ApprovalRequestID) != "" {
		appendTargetTaskEvidence(&result, "workflow emitted a pending request-info approval")
	} else {
		appendTargetTaskViolation(&result, "workflow emitted a pending request-info approval")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restored the pending request on recreated workflow objects")
	} else {
		appendTargetTaskViolation(&result, "workflow restored the pending request on recreated workflow objects")
	}
	if summary.ApprovalResponseObserved && summary.ApprovalReplayObserved && summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "restored workflow replayed the approved pending request into duplicate external effects")
	} else {
		appendTargetTaskViolation(&result, "restored workflow replayed the approved pending request into duplicate external effects")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFWorkflowRehydrateDivergenceTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowRehydrateDivergenceTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded resume-vs-rehydrate divergence")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("external effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.PendingRequestObserved && strings.TrimSpace(summary.ApprovalRequestID) != "" {
		appendTargetTaskEvidence(&result, "workflow emitted a pending request-info approval")
	} else {
		appendTargetTaskViolation(&result, "workflow emitted a pending request-info approval")
	}
	if summary.ApprovalResponseObserved && summary.SameInstanceResumeObserved {
		appendTargetTaskEvidence(&result, "same workflow instance consumed the approval once")
	} else {
		appendTargetTaskViolation(&result, "same workflow instance consumed the approval once")
	}
	if summary.Restored && summary.RuntimeObjectRecreated && summary.RehydrateReplayObserved {
		appendTargetTaskEvidence(&result, "recreated workflow object replayed the checkpointed approval response")
	} else {
		appendTargetTaskViolation(&result, "recreated workflow object replayed the checkpointed approval response")
	}
	if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "resume-vs-rehydrate comparison produced duplicate external effects after rehydrate")
	} else {
		appendTargetTaskViolation(&result, "resume-vs-rehydrate comparison produced duplicate external effects after rehydrate")
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

func evaluateMAFWorkflowHTTPReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowHTTPReplayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded HTTP external service replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("HTTP service effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
		appendTargetTaskEvidence(&result, "workflow called a local HTTP external service")
	} else {
		appendTargetTaskViolation(&result, "workflow called a local HTTP external service")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "one logical workflow operation produced duplicate HTTP service effect entries")
	} else {
		appendTargetTaskViolation(&result, "one logical workflow operation produced duplicate HTTP service effect entries")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFWorkflowResourceReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowResourceReplayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded resource service replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("resource service effect entries: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
		appendTargetTaskEvidence(&result, "workflow called an HTTP resource service")
	} else {
		appendTargetTaskViolation(&result, "workflow called an HTTP resource service")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.DuplicateEffectObserved && summary.ExternalEffectEntries >= 2 {
		appendTargetTaskEvidence(&result, "one logical workflow operation produced duplicate resource service effects")
	} else {
		appendTargetTaskViolation(&result, "one logical workflow operation produced duplicate resource service effects")
	}
	return finalizeTargetTaskCompliance(result)
}

func evaluateMAFWorkflowAuthorityReplayTargetTaskCompliance(workspace string) TargetTaskComplianceResult {
	result := TargetTaskComplianceResult{
		Name:   MAFWorkflowAuthorityReplayTargetTaskID,
		Status: TargetTaskComplianceStatusUnknown,
	}
	summary, ok, err := loadMAFWorkflowArtifact(workspace)
	if err != nil {
		appendTargetTaskEvidence(&result, err.Error())
		return result
	}
	if !ok {
		appendTargetTaskViolation(&result, "maf workflow summary artifact recorded authority token replay")
		return finalizeTargetTaskCompliance(result)
	}
	appendTargetTaskEvidence(&result, "maf workflow summary artifact was available")
	appendTargetTaskEvidence(&result, fmt.Sprintf("authority service events: %d", summary.ExternalEffectEntries))
	requireAtLeastOne(&result, len(summary.CheckpointIDs), "workflow created at least one file checkpoint")
	if summary.ExternalServiceObserved && strings.TrimSpace(summary.ExternalServiceURL) != "" {
		appendTargetTaskEvidence(&result, "workflow called an authority service")
	} else {
		appendTargetTaskViolation(&result, "workflow called an authority service")
	}
	if summary.AuthorityTokenIssued && summary.AuthorityTokenConsumed {
		appendTargetTaskEvidence(&result, "workflow issued and consumed an authority token before restore")
	} else {
		appendTargetTaskViolation(&result, "workflow issued and consumed an authority token before restore")
	}
	if summary.Restored && summary.RuntimeObjectRecreated {
		appendTargetTaskEvidence(&result, "workflow restore ran on a recreated workflow object")
	} else {
		appendTargetTaskViolation(&result, "workflow restore ran on a recreated workflow object")
	}
	if summary.AuthorityReplayConflict {
		appendTargetTaskEvidence(&result, "restored workflow replay observed token_already_consumed from authority state")
	} else {
		appendTargetTaskViolation(&result, "restored workflow replay observed token_already_consumed from authority state")
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
