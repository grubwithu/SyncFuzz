package syncfuzz

type TargetContractExpectation string

const (
	targetContractExpectationPreserve    TargetContractExpectation = "preserve"
	targetContractExpectationReset       TargetContractExpectation = "reset"
	targetContractExpectationUnspecified TargetContractExpectation = "unspecified"
)

type TargetContractSourceStrength string

const (
	targetContractSourceStrengthExplicit    TargetContractSourceStrength = "explicit"
	targetContractSourceStrengthImplicit    TargetContractSourceStrength = "implicit"
	targetContractSourceStrengthUnspecified TargetContractSourceStrength = "unspecified"
)

type TargetContractInterpretationStatus string

const (
	targetContractStatusConsistent TargetContractInterpretationStatus = "contract-consistent"
	targetContractStatusViolation  TargetContractInterpretationStatus = "contract-violation"
	targetContractStatusUnknown    TargetContractInterpretationStatus = "contract-unknown"
)

type TargetContractProfile struct {
	SchemaVersion string               `json:"schema_version"`
	ProfileID     string               `json:"profile_id"`
	TargetID      string               `json:"target_id"`
	Description   string               `json:"description"`
	Rules         []TargetContractRule `json:"rules"`
}

type TargetContractRule struct {
	RuleID         string                       `json:"rule_id"`
	TaskID         string                       `json:"task_id"`
	StateSurface   string                       `json:"state_surface"`
	LifecycleEdge  string                       `json:"lifecycle_edge"`
	Expectation    TargetContractExpectation    `json:"expectation"`
	SourceStrength TargetContractSourceStrength `json:"source_strength"`
	EvidenceSource string                       `json:"evidence_source,omitempty"`
	Description    string                       `json:"description,omitempty"`
}

type TargetContractInterpretation struct {
	Status         TargetContractInterpretationStatus `json:"status,omitempty"`
	ProfileID      string                             `json:"profile_id,omitempty"`
	RuleID         string                             `json:"rule_id,omitempty"`
	StateSurface   string                             `json:"state_surface,omitempty"`
	LifecycleEdge  string                             `json:"lifecycle_edge,omitempty"`
	Expectation    TargetContractExpectation          `json:"expectation,omitempty"`
	SourceStrength TargetContractSourceStrength       `json:"source_strength,omitempty"`
	EvidenceSource string                             `json:"evidence_source,omitempty"`
	Summary        string                             `json:"summary,omitempty"`
	Evidence       []string                           `json:"evidence,omitempty"`
	Caveats        []string                           `json:"caveats,omitempty"`
}

type targetContractObservedOutcome string

const (
	targetContractObservedPreserve targetContractObservedOutcome = "preserve"
	targetContractObservedReset    targetContractObservedOutcome = "reset"
)

func targetContractProfile(targetID string) *TargetContractProfile {
	if targetID != "langgraph-shell-react" {
		return nil
	}
	return &TargetContractProfile{
		SchemaVersion: "syncfuzz.target-contract-profile.v1",
		ProfileID:     "langgraph-shell-react.phase5b.v1",
		TargetID:      "langgraph-shell-react",
		Description:   "Contract profile for the official LangGraph create_agent + ShellToolMiddleware target as integrated by SyncFuzz.",
		Rules: []TargetContractRule{
			{
				RuleID:         "shell-path-within-run",
				TaskID:         persistentShellTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "run->continue",
				Expectation:    targetContractExpectationPreserve,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "middleware-runtime",
				Description:    "Later shell calls in the same persistent shell session can still observe earlier PATH changes.",
			},
			{
				RuleID:         "shell-path-replay-boundary",
				TaskID:         persistentShellReplayTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "checkpoint->replay",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Replay from before the PATH export should not preserve later shell-session PATH mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "shell-path-fork-boundary",
				TaskID:         persistentShellForkTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the PATH export should not preserve later shell-session PATH mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-file-fork-boundary",
				TaskID:         fileResidueForkTargetTaskID,
				StateSurface:   "workspace.file",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the file drop should not preserve later workspace file mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-directory-fork-boundary",
				TaskID:         directoryResidueForkTargetTaskID,
				StateSurface:   "workspace.directory",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the directory creation should not preserve later workspace directory mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-delete-fork-boundary",
				TaskID:         deleteResidueForkTargetTaskID,
				StateSurface:   "workspace.file-presence",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the deletion should restore the checkpointed file-presence state rather than preserving later deletion residue.",
			},
			{
				RuleID:         "workspace-symlink-fork-boundary",
				TaskID:         symlinkResidueForkTargetTaskID,
				StateSurface:   "workspace.symlink",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the symlink creation should not preserve later workspace symlink mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-rename-fork-boundary",
				TaskID:         renameResidueForkTargetTaskID,
				StateSurface:   "workspace.filename-binding",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the rename should restore the original filename binding rather than preserving the later renamed destination.",
			},
			{
				RuleID:         "workspace-mode-fork-boundary",
				TaskID:         modeResidueForkTargetTaskID,
				StateSurface:   "workspace.file-mode",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the chmod should restore the earlier file mode rather than preserving the later tightened mode bits.",
			},
			{
				RuleID:         "workspace-append-fork-boundary",
				TaskID:         appendResidueForkTargetTaskID,
				StateSurface:   "workspace.file-content",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the append should restore the earlier file contents rather than preserving later appended content.",
			},
			{
				RuleID:         "workspace-hardlink-fork-boundary",
				TaskID:         hardlinkResidueForkTargetTaskID,
				StateSurface:   "workspace.hardlink",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the hardlink creation should not preserve the later hardlink across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-fifo-fork-boundary",
				TaskID:         fifoResidueForkTargetTaskID,
				StateSurface:   "workspace.fifo",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the fifo creation should not preserve the later named pipe across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-open-fd-fork-boundary",
				TaskID:         openFDResidueForkTargetTaskID,
				StateSurface:   "runtime.open-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the fd-holder launch should not preserve the later open workspace file descriptor across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-deleted-open-fd-fork-boundary",
				TaskID:         deletedOpenFDForkTargetTaskID,
				StateSurface:   "runtime.deleted-open-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the deleted-fd-holder launch should not preserve the later deleted workspace inode capability across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-inherited-fd-branch-leakage-boundary",
				TaskID:         inheritedFDLeakTargetTaskID,
				StateSurface:   "runtime.inherited-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local fd-holder launch should not let the successor branch read discarded branch data through an inherited file descriptor.",
			},
			{
				RuleID:         "runtime-unix-listener-fork-boundary",
				TaskID:         unixListenerResidueForkTargetTaskID,
				StateSurface:   "runtime.unix-listener",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    targetContractExpectationReset,
				SourceStrength: targetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local Unix listener launch should not let the successor branch connect to the discarded branch listener.",
			},
			{
				RuleID:         "child-process-post-return",
				TaskID:         longDelayTargetTaskID,
				StateSurface:   "child-process",
				LifecycleEdge:  "target-command->post-return",
				Expectation:    targetContractExpectationUnspecified,
				SourceStrength: targetContractSourceStrengthUnspecified,
				EvidenceSource: "experiment",
				Description:    "The long-delay orphan-process task is treated as observation-first rather than as a stable recovery-contract check.",
			},
		},
	}
}

func targetContractRule(profile *TargetContractProfile, taskID string) (TargetContractRule, bool) {
	if profile == nil {
		return TargetContractRule{}, false
	}
	for _, rule := range profile.Rules {
		if rule.TaskID == taskID {
			return rule, true
		}
	}
	return TargetContractRule{}, false
}

func evaluateTargetContractInterpretation(profile *TargetContractProfile, taskID string, oracle TargetOracleResult, compliance TargetTaskComplianceResult) *TargetContractInterpretation {
	rule, ok := targetContractRule(profile, taskID)
	if !ok {
		return nil
	}

	result := &TargetContractInterpretation{
		Status:         targetContractStatusUnknown,
		ProfileID:      profile.ProfileID,
		RuleID:         rule.RuleID,
		StateSurface:   rule.StateSurface,
		LifecycleEdge:  rule.LifecycleEdge,
		Expectation:    rule.Expectation,
		SourceStrength: rule.SourceStrength,
		EvidenceSource: rule.EvidenceSource,
	}

	if compliance.Status == targetTaskComplianceStatusViolated {
		result.Summary = "task compliance was violated, so contract interpretation is suspended"
		result.Caveats = append(result.Caveats, "task drift prevented a stable contract reading")
		return result
	}
	if compliance.Status == targetTaskComplianceStatusUnknown {
		result.Caveats = append(result.Caveats, "task compliance was unknown")
	}

	if rule.Expectation == targetContractExpectationUnspecified {
		result.Summary = "the current profile treats this task as observation-first rather than as a stable contract check"
		return result
	}
	if oracle.Status == targetOracleStatusInconclusive {
		result.Summary = "oracle evidence was inconclusive, so contract interpretation remains unknown"
		return result
	}

	outcome, detail, ok := targetContractObservedOutcomeForTask(taskID, oracle)
	if !ok {
		result.Summary = "the observed oracle outcome did not map cleanly onto the current contract rule"
		return result
	}

	result.Evidence = append(result.Evidence, detail)
	expected := targetContractObservedOutcome(rule.Expectation)
	if outcome == expected {
		result.Status = targetContractStatusConsistent
		result.Summary = "observed target behavior matched the selected lifecycle contract"
	} else {
		result.Status = targetContractStatusViolation
		result.Summary = "observed target behavior contradicted the selected lifecycle contract"
	}
	if rule.SourceStrength == targetContractSourceStrengthImplicit {
		result.Caveats = append(result.Caveats, "this rule is inferred from the integrated target and SyncFuzz wrapper semantics rather than from a maintainer-stated guarantee")
	}
	return result
}

func targetContractObservedOutcomeForTask(taskID string, oracle TargetOracleResult) (targetContractObservedOutcome, string, bool) {
	switch taskID {
	case persistentShellTargetTaskID:
		switch oracle.Status {
		case targetOracleStatusConfirmed:
			return targetContractObservedPreserve, "later shell steps still inherited the earlier PATH mutation within the same persistent shell session", true
		case targetOracleStatusNegative:
			return targetContractObservedReset, "later shell steps no longer showed inherited PATH mutation within the same run", true
		default:
			return "", "", false
		}
	case persistentShellReplayTargetTaskID:
		switch oracle.Attribution {
		case targetOracleAttributionRuntimeResidue:
			return targetContractObservedPreserve, "replay follow-up still inherited shell PATH state from after the selected checkpoint", true
		case targetOracleAttributionCleanReplay, targetOracleAttributionLegitimateReexecution, targetOracleAttributionExternalSmuggling:
			return targetContractObservedReset, "replay did not preserve shell PATH state directly across the selected checkpoint boundary", true
		default:
			return "", "", false
		}
	case persistentShellForkTargetTaskID:
		switch oracle.Attribution {
		case targetOracleAttributionRuntimeResidue:
			return targetContractObservedPreserve, "fork follow-up still inherited shell PATH state from after the selected checkpoint", true
		case targetOracleAttributionCleanFork:
			return targetContractObservedReset, "fork did not preserve shell PATH state directly across the selected checkpoint boundary", true
		default:
			return "", "", false
		}
	case fileResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file residue", oracle)
	case directoryResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("directory residue", oracle)
	case deleteResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("deletion residue", oracle)
	case symlinkResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("symlink residue", oracle)
	case renameResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("rename residue", oracle)
	case modeResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file-mode residue", oracle)
	case appendResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file-content residue", oracle)
	case hardlinkResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("hardlink residue", oracle)
	case fifoResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("fifo residue", oracle)
	case openFDResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("open-fd residue", oracle)
	case deletedOpenFDForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("deleted-open-fd residue", oracle)
	case inheritedFDLeakTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("inherited-fd branch leakage", oracle)
	case unixListenerResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("Unix listener residue", oracle)
	default:
		return "", "", false
	}
}

func targetContractObservedWorkspaceForkOutcome(name string, oracle TargetOracleResult) (targetContractObservedOutcome, string, bool) {
	switch oracle.Attribution {
	case targetOracleAttributionRuntimeResidue:
		return targetContractObservedPreserve, "fork preserved " + name + " across the selected checkpoint boundary", true
	case targetOracleAttributionCleanFork, targetOracleAttributionWorkspaceRebuild:
		return targetContractObservedReset, "fork did not preserve " + name + " directly across the selected checkpoint boundary", true
	default:
		return "", "", false
	}
}

func targetContractInterpretationStatusValue(result *TargetContractInterpretation) TargetContractInterpretationStatus {
	if result == nil {
		return ""
	}
	return result.Status
}

func targetContractInterpretationProfileIDValue(result *TargetContractInterpretation) string {
	if result == nil {
		return ""
	}
	return result.ProfileID
}

func targetContractInterpretationRuleIDValue(result *TargetContractInterpretation) string {
	if result == nil {
		return ""
	}
	return result.RuleID
}
