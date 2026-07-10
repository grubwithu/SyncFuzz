package target

type TargetContractExpectation string

const (
	TargetContractExpectationPreserve    TargetContractExpectation = "preserve"
	TargetContractExpectationReset       TargetContractExpectation = "reset"
	TargetContractExpectationUnspecified TargetContractExpectation = "unspecified"
)

type TargetContractSourceStrength string

const (
	TargetContractSourceStrengthExplicit    TargetContractSourceStrength = "explicit"
	TargetContractSourceStrengthImplicit    TargetContractSourceStrength = "implicit"
	TargetContractSourceStrengthUnspecified TargetContractSourceStrength = "unspecified"
)

type TargetContractInterpretationStatus string

const (
	TargetContractStatusConsistent TargetContractInterpretationStatus = "contract-consistent"
	TargetContractStatusViolation  TargetContractInterpretationStatus = "contract-violation"
	TargetContractStatusUnknown    TargetContractInterpretationStatus = "contract-unknown"
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

func TargetContractProfileFor(targetID string) *TargetContractProfile {
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
				TaskID:         PersistentShellTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "run->continue",
				Expectation:    TargetContractExpectationPreserve,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "middleware-runtime",
				Description:    "Later shell calls in the same persistent shell session can still observe earlier PATH changes.",
			},
			{
				RuleID:         "shell-path-replay-boundary",
				TaskID:         PersistentShellReplayTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "checkpoint->replay",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Replay from before the PATH export should not preserve later shell-session PATH mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "shell-path-fork-boundary",
				TaskID:         PersistentShellForkTargetTaskID,
				StateSurface:   "shell-session.path",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the PATH export should not preserve later shell-session PATH mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-file-fork-boundary",
				TaskID:         FileResidueForkTargetTaskID,
				StateSurface:   "workspace.file",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the file drop should not preserve later workspace file mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-directory-fork-boundary",
				TaskID:         DirectoryResidueForkTargetTaskID,
				StateSurface:   "workspace.directory",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the directory creation should not preserve later workspace directory mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-delete-fork-boundary",
				TaskID:         DeleteResidueForkTargetTaskID,
				StateSurface:   "workspace.file-presence",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the deletion should restore the checkpointed file-presence state rather than preserving later deletion residue.",
			},
			{
				RuleID:         "workspace-symlink-fork-boundary",
				TaskID:         SymlinkResidueForkTargetTaskID,
				StateSurface:   "workspace.symlink",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the symlink creation should not preserve later workspace symlink mutations across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-rename-fork-boundary",
				TaskID:         RenameResidueForkTargetTaskID,
				StateSurface:   "workspace.filename-binding",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the rename should restore the original filename binding rather than preserving the later renamed destination.",
			},
			{
				RuleID:         "workspace-mode-fork-boundary",
				TaskID:         ModeResidueForkTargetTaskID,
				StateSurface:   "workspace.file-mode",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the chmod should restore the earlier file mode rather than preserving the later tightened mode bits.",
			},
			{
				RuleID:         "workspace-append-fork-boundary",
				TaskID:         AppendResidueForkTargetTaskID,
				StateSurface:   "workspace.file-content",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the append should restore the earlier file contents rather than preserving later appended content.",
			},
			{
				RuleID:         "workspace-hardlink-fork-boundary",
				TaskID:         HardlinkResidueForkTargetTaskID,
				StateSurface:   "workspace.hardlink",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the hardlink creation should not preserve the later hardlink across the selected checkpoint boundary.",
			},
			{
				RuleID:         "workspace-fifo-fork-boundary",
				TaskID:         FifoResidueForkTargetTaskID,
				StateSurface:   "workspace.fifo",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the fifo creation should not preserve the later named pipe across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-open-fd-fork-boundary",
				TaskID:         OpenFDResidueForkTargetTaskID,
				StateSurface:   "runtime.open-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the fd-holder launch should not preserve the later open workspace file descriptor across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-deleted-open-fd-fork-boundary",
				TaskID:         DeletedOpenFDForkTargetTaskID,
				StateSurface:   "runtime.deleted-open-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the deleted-fd-holder launch should not preserve the later deleted workspace inode capability across the selected checkpoint boundary.",
			},
			{
				RuleID:         "runtime-inherited-fd-branch-leakage-boundary",
				TaskID:         InheritedFDLeakTargetTaskID,
				StateSurface:   "runtime.inherited-fd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local fd-holder launch should not let the successor branch read discarded branch data through an inherited file descriptor.",
			},
			{
				RuleID:         "runtime-unix-listener-fork-boundary",
				TaskID:         UnixListenerResidueForkTargetTaskID,
				StateSurface:   "runtime.unix-listener",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local Unix listener launch should not let the successor branch connect to the discarded branch listener.",
			},
			{
				RuleID:         "communication-trusted-client-fork-boundary",
				TaskID:         DiscardedServerTrustedClientTargetTaskID,
				StateSurface:   "communication.trusted-client-output",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local Unix listener launch should not let the successor branch trusted client consume the discarded branch response.",
			},
			{
				RuleID:         "communication-response-cache-fork-boundary",
				TaskID:         SocketResponsePoisoningTargetTaskID,
				StateSurface:   "communication.response-cache",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the branch-local Unix listener launch should not let the successor branch cache the discarded branch response.",
			},
			{
				RuleID:         "shell-cwd-fork-boundary",
				TaskID:         CWDResidueForkTargetTaskID,
				StateSurface:   "shell-session.cwd",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the cwd change should not preserve the later cwd across the selected checkpoint boundary.",
			},
			{
				RuleID:         "shell-umask-fork-boundary",
				TaskID:         UmaskResidueForkTargetTaskID,
				StateSurface:   "shell-session.umask",
				LifecycleEdge:  "checkpoint->fork",
				Expectation:    TargetContractExpectationReset,
				SourceStrength: TargetContractSourceStrengthImplicit,
				EvidenceSource: "syncfuzz-wrapper",
				Description:    "Fork from before the umask change should not preserve the later umask across the selected checkpoint boundary.",
			},
			{
				RuleID:         "child-process-post-return",
				TaskID:         LongDelayTargetTaskID,
				StateSurface:   "child-process",
				LifecycleEdge:  "target-command->post-return",
				Expectation:    TargetContractExpectationUnspecified,
				SourceStrength: TargetContractSourceStrengthUnspecified,
				EvidenceSource: "experiment",
				Description:    "The long-delay orphan-process task is treated as observation-first rather than as a stable recovery-contract check.",
			},
		},
	}
}

func TargetContractRuleFor(profile *TargetContractProfile, taskID string) (TargetContractRule, bool) {
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

func EvaluateTargetContractInterpretation(profile *TargetContractProfile, taskID string, oracle TargetOracleResult, compliance TargetTaskComplianceResult) *TargetContractInterpretation {
	rule, ok := TargetContractRuleFor(profile, taskID)
	if !ok {
		return nil
	}

	result := &TargetContractInterpretation{
		Status:         TargetContractStatusUnknown,
		ProfileID:      profile.ProfileID,
		RuleID:         rule.RuleID,
		StateSurface:   rule.StateSurface,
		LifecycleEdge:  rule.LifecycleEdge,
		Expectation:    rule.Expectation,
		SourceStrength: rule.SourceStrength,
		EvidenceSource: rule.EvidenceSource,
	}

	if compliance.Status == TargetTaskComplianceStatusViolated {
		result.Summary = "task compliance was violated, so contract interpretation is suspended"
		result.Caveats = append(result.Caveats, "task drift prevented a stable contract reading")
		return result
	}
	if compliance.Status == TargetTaskComplianceStatusUnknown {
		result.Caveats = append(result.Caveats, "task compliance was unknown")
	}

	if rule.Expectation == TargetContractExpectationUnspecified {
		result.Summary = "the current profile treats this task as observation-first rather than as a stable contract check"
		return result
	}
	if oracle.Status == TargetOracleStatusInconclusive {
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
		result.Status = TargetContractStatusConsistent
		result.Summary = "observed target behavior matched the selected lifecycle contract"
	} else {
		result.Status = TargetContractStatusViolation
		result.Summary = "observed target behavior contradicted the selected lifecycle contract"
	}
	if rule.SourceStrength == TargetContractSourceStrengthImplicit {
		result.Caveats = append(result.Caveats, "this rule is inferred from the integrated target and SyncFuzz wrapper semantics rather than from a maintainer-stated guarantee")
	}
	return result
}

func targetContractObservedOutcomeForTask(taskID string, oracle TargetOracleResult) (targetContractObservedOutcome, string, bool) {
	switch taskID {
	case PersistentShellTargetTaskID:
		switch oracle.Status {
		case TargetOracleStatusConfirmed:
			return targetContractObservedPreserve, "later shell steps still inherited the earlier PATH mutation within the same persistent shell session", true
		case TargetOracleStatusNegative:
			return targetContractObservedReset, "later shell steps no longer showed inherited PATH mutation within the same run", true
		default:
			return "", "", false
		}
	case PersistentShellReplayTargetTaskID:
		switch oracle.Attribution {
		case TargetOracleAttributionRuntimeResidue:
			return targetContractObservedPreserve, "replay follow-up still inherited shell PATH state from after the selected checkpoint", true
		case TargetOracleAttributionCleanReplay, TargetOracleAttributionLegitimateReexecution, TargetOracleAttributionExternalSmuggling:
			return targetContractObservedReset, "replay did not preserve shell PATH state directly across the selected checkpoint boundary", true
		default:
			return "", "", false
		}
	case PersistentShellForkTargetTaskID:
		switch oracle.Attribution {
		case TargetOracleAttributionRuntimeResidue:
			return targetContractObservedPreserve, "fork follow-up still inherited shell PATH state from after the selected checkpoint", true
		case TargetOracleAttributionCleanFork:
			return targetContractObservedReset, "fork did not preserve shell PATH state directly across the selected checkpoint boundary", true
		default:
			return "", "", false
		}
	case FileResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file residue", oracle)
	case DirectoryResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("directory residue", oracle)
	case DeleteResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("deletion residue", oracle)
	case SymlinkResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("symlink residue", oracle)
	case RenameResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("rename residue", oracle)
	case ModeResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file-mode residue", oracle)
	case AppendResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("file-content residue", oracle)
	case HardlinkResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("hardlink residue", oracle)
	case FifoResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("fifo residue", oracle)
	case OpenFDResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("open-fd residue", oracle)
	case DeletedOpenFDForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("deleted-open-fd residue", oracle)
	case InheritedFDLeakTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("inherited-fd branch leakage", oracle)
	case UnixListenerResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("Unix listener residue", oracle)
	case DiscardedServerTrustedClientTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("trusted-client response residue", oracle)
	case SocketResponsePoisoningTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("socket response poisoning residue", oracle)
	case CWDResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("cwd residue", oracle)
	case UmaskResidueForkTargetTaskID:
		return targetContractObservedWorkspaceForkOutcome("umask residue", oracle)
	default:
		return "", "", false
	}
}

func targetContractObservedWorkspaceForkOutcome(name string, oracle TargetOracleResult) (targetContractObservedOutcome, string, bool) {
	switch oracle.Attribution {
	case TargetOracleAttributionRuntimeResidue:
		return targetContractObservedPreserve, "fork preserved " + name + " across the selected checkpoint boundary", true
	case TargetOracleAttributionCleanFork, TargetOracleAttributionWorkspaceRebuild:
		return targetContractObservedReset, "fork did not preserve " + name + " directly across the selected checkpoint boundary", true
	default:
		return "", "", false
	}
}

func TargetContractInterpretationStatusValue(result *TargetContractInterpretation) TargetContractInterpretationStatus {
	if result == nil {
		return ""
	}
	return result.Status
}

func TargetContractInterpretationProfileIDValue(result *TargetContractInterpretation) string {
	if result == nil {
		return ""
	}
	return result.ProfileID
}

func TargetContractInterpretationRuleIDValue(result *TargetContractInterpretation) string {
	if result == nil {
		return ""
	}
	return result.RuleID
}
