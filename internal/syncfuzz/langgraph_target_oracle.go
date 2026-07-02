package syncfuzz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	langgraphHistoryArtifact    = "langgraph-history.json"
	langgraphSummaryArtifact    = "langgraph-run-summary.json"
	langgraphLifecycleArtifact  = "langgraph-lifecycle.json"
	langgraphCheckpointArtifact = "langgraph-checkpointer.json"
	langgraphReplayArtifact     = "langgraph-replay-summary.json"
	langgraphForkArtifact       = "langgraph-fork-summary.json"
)

type langgraphHistoryCheckpoint struct {
	Index    int                       `json:"index"`
	Messages []langgraphHistoryMessage `json:"messages"`
}

type langgraphHistoryMessage struct {
	Role      string                     `json:"role"`
	Content   string                     `json:"content"`
	ToolCalls []langgraphHistoryToolCall `json:"tool_calls,omitempty"`
}

type langgraphHistoryToolCall struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

type langgraphOperationSummary struct {
	Operation          string                    `json:"operation"`
	Requested          bool                      `json:"requested"`
	CheckpointSelector string                    `json:"checkpoint_selector"`
	CheckpointIndex    int                       `json:"checkpoint_index"`
	CheckpointID       string                    `json:"checkpoint_id"`
	AvailableHistory   int                       `json:"available_history"`
	UserMessage        string                    `json:"user_message,omitempty"`
	Messages           []langgraphHistoryMessage `json:"messages"`
}

type langgraphShellCall struct {
	Command string
	Output  string
}

type persistentShellTranscriptEvidence struct {
	Available   bool
	Confirmed   bool
	Attribution string
	Details     []string
}

type langgraphReplayStateSmugglingEvidence struct {
	Available bool
	Smuggled  bool
	Details   []string
}

func inspectLangGraphPersistentShellEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if len(checkpoints) == 0 {
		return persistentShellTranscriptEvidence{}, nil
	}

	evidence := persistentShellTranscriptEvidence{Available: true}
	for _, checkpoint := range checkpoints {
		calls := buildLangGraphShellCalls(checkpoint.Messages)
		item := evaluateLangGraphPersistentShellCalls(calls)
		if item.Confirmed {
			return item, nil
		}
		if len(item.Details) > len(evidence.Details) {
			evidence.Details = item.Details
		}
	}
	if len(evidence.Details) == 0 {
		evidence.Details = []string{"langgraph transcript was present but did not show persistent-shell poisoning evidence"}
	}
	return evidence, nil
}

func inspectLangGraphReplayPoisonEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, langgraphReplayArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphReplayShellCalls(buildLangGraphShellCalls(summary.Messages))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphReplayStateSmuggling(workspace string) (langgraphReplayStateSmugglingEvidence, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return langgraphReplayStateSmugglingEvidence{}, err
	}
	summary, err := loadLangGraphOperationSummary(workspace, langgraphReplayArtifact)
	if err != nil {
		return langgraphReplayStateSmugglingEvidence{}, err
	}
	if len(checkpoints) == 0 && summary == nil {
		return langgraphReplayStateSmugglingEvidence{}, nil
	}

	evidence := langgraphReplayStateSmugglingEvidence{Available: true}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandWritesPathToExternalHelper(call.Command) {
				evidence.Smuggled = true
				evidence.Details = append(evidence.Details, "langgraph history wrote PATH state to an external helper path")
				return evidence, nil
			}
		}
	}
	if summary == nil {
		return evidence, nil
	}
	for _, call := range buildLangGraphShellCalls(summary.Messages) {
		switch {
		case commandRestoresPathFromExternalHelper(call.Command):
			evidence.Smuggled = true
			evidence.Details = append(evidence.Details, "langgraph replay restored PATH from an external helper path")
			return evidence, nil
		case commandWritesPathToExternalHelper(call.Command):
			evidence.Smuggled = true
			evidence.Details = append(evidence.Details, "langgraph replay wrote PATH state to an external helper path")
			return evidence, nil
		}
	}
	return evidence, nil
}

func inspectLangGraphForkPoisonEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, langgraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkShellCalls(buildLangGraphShellCalls(summary.Messages))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkFileResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, langgraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkFileResidueCalls(buildLangGraphShellCalls(summary.Messages))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkDeleteResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, langgraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkDeleteResidueCalls(buildLangGraphShellCalls(summary.Messages))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkSymlinkResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, langgraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkSymlinkResidueCalls(buildLangGraphShellCalls(summary.Messages))
	evidence.Available = true
	return evidence, nil
}

func loadLangGraphHistory(workspace string) ([]langgraphHistoryCheckpoint, error) {
	path := filepath.Join(workspace, langgraphHistoryArtifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", langgraphHistoryArtifact, err)
	}

	var checkpoints []langgraphHistoryCheckpoint
	if err := json.Unmarshal(raw, &checkpoints); err != nil {
		return nil, fmt.Errorf("decode %s: %w", langgraphHistoryArtifact, err)
	}
	return checkpoints, nil
}

func loadLangGraphOperationSummary(workspace string, artifact string) (*langgraphOperationSummary, error) {
	path := filepath.Join(workspace, artifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", artifact, err)
	}

	var summary langgraphOperationSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, fmt.Errorf("decode %s: %w", artifact, err)
	}
	return &summary, nil
}

func langgraphHistoryShowsPathExport(workspace string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		if langgraphMessagesHavePathExport(checkpoint.Messages) {
			return true, nil
		}
	}
	return false, nil
}

func langgraphMessagesHavePathExport(messages []langgraphHistoryMessage) bool {
	for _, call := range buildLangGraphShellCalls(messages) {
		if looksLikeAttackerPathExport(call.Command) {
			return true
		}
	}
	return false
}

func langgraphHistoryShowsWorkspaceFileWrite(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandWritesWorkspaceFile(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceFileDelete(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandDeletesWorkspaceFile(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceSymlinkCreate(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandCreatesWorkspaceSymlink(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func targetRunMarker(workspace string) (string, error) {
	return targetRunMarkerWithPrefix(workspace, "SYNCFUZZ_FILE_RESIDUE_MARKER:")
}

func targetDeleteRunMarker(workspace string) (string, error) {
	return targetRunMarkerWithPrefix(workspace, "SYNCFUZZ_DELETE_RESIDUE_MARKER:")
}

func targetRunMarkerWithPrefix(workspace string, prefix string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(workspace, targetTaskArtifact))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", targetTaskArtifact, err)
	}

	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return "", fmt.Errorf("decode %s: %w", targetTaskArtifact, err)
	}
	if strings.TrimSpace(task.RunID) == "" {
		return "", nil
	}
	return prefix + strings.TrimSpace(task.RunID), nil
}

func buildLangGraphShellCalls(messages []langgraphHistoryMessage) []langgraphShellCall {
	var calls []langgraphShellCall
	pending := -1
	for _, message := range messages {
		switch message.Role {
		case "ai":
			for _, call := range message.ToolCalls {
				if call.Name != "shell" {
					continue
				}
				calls = append(calls, langgraphShellCall{Command: extractLangGraphShellCommand(call.Args)})
				pending = len(calls) - 1
			}
		case "tool":
			if pending >= 0 {
				if calls[pending].Output == "" {
					calls[pending].Output = message.Content
				} else {
					calls[pending].Output += "\n" + message.Content
				}
				pending = -1
			}
		}
	}
	return calls
}

func extractLangGraphShellCommand(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		if command, ok := decoded["command"].(string); ok {
			return command
		}
	}
	return trimmed
}

func evaluateLangGraphPersistentShellCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawExport bool
	var sawLaterVerification bool
	var sawLaterPersistedPath bool
	var sawLaterAttackerResolution bool
	var sawLaterAttackerMarker bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		hasExport := strings.Contains(command, "export PATH=") && strings.Contains(command, "attacker-bin")
		if hasExport {
			sawExport = true
			continue
		}
		if !looksLikePersistentShellVerification(command) {
			continue
		}
		sawLaterVerification = true
		if outputShowsAttackerPathEnv(output) {
			sawLaterPersistedPath = true
		}
		if outputShowsAttackerPath(output) {
			sawLaterAttackerResolution = true
		}
		if outputShowsAttackerMarker(output) {
			sawLaterAttackerMarker = true
		}
		if sawLaterPersistedPath && (sawLaterAttackerResolution || sawLaterAttackerMarker) {
			details := []string{
				"langgraph transcript shows a later shell call without PATH export",
				"later shell call still had attacker-bin at the front of PATH",
			}
			if sawLaterAttackerResolution {
				details = append(details, "later shell call resolved git to attacker-bin/git")
			}
			if sawLaterAttackerMarker {
				details = append(details, "later shell call executed poisoned git and observed ATTACKER_GIT")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: targetOracleAttributionRuntimeResidue,
				Details:     details,
			}
		}
	}

	var details []string
	if sawExport {
		details = append(details, "langgraph transcript captured an earlier PATH export to attacker-bin")
	}
	if sawLaterVerification {
		details = append(details, "langgraph transcript captured a later git verification shell call")
	}
	if sawLaterPersistedPath {
		details = append(details, "later shell call showed attacker-bin in PATH")
	}
	if sawLaterAttackerResolution {
		details = append(details, "later shell call resolved git to attacker-bin/git")
	}
	if sawLaterAttackerMarker {
		details = append(details, "later shell call emitted ATTACKER_GIT")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: targetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphReplayShellCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawExport bool
	var sawVerification bool
	var sawDuplicatedPath bool
	var sawAttackerPathEnv bool
	var sawAttackerResolution bool
	var sawAttackerMarker bool
	var sawCleanObservation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if looksLikeAttackerPathExport(command) {
			sawExport = true
		}
		if !looksLikePersistentShellVerification(command) {
			continue
		}
		sawVerification = true
		if maxAttackerBinEntriesInPath(output) >= 2 {
			sawDuplicatedPath = true
		}
		if replayOutputLooksObserved(output) {
			sawCleanObservation = true
		}
		if outputShowsAttackerPathEnv(output) {
			sawAttackerPathEnv = true
		}
		if outputShowsAttackerPath(output) {
			sawAttackerResolution = true
		}
		if outputShowsAttackerMarker(output) {
			sawAttackerMarker = true
		}
		if (sawDuplicatedPath || (!sawExport && sawAttackerPathEnv)) && (sawAttackerResolution || sawAttackerMarker) {
			details := []string{
				"langgraph replay transcript showed attacker-controlled shell state after replay",
			}
			if sawDuplicatedPath {
				details = append(details, "replay verification showed attacker-bin duplicated inside PATH")
			} else {
				details = append(details, "replay verification still showed attacker-bin in PATH without replaying the export step")
			}
			if sawExport {
				details = append(details, "replay reran the attacker PATH export step")
			}
			if sawAttackerResolution {
				details = append(details, "replay verification resolved git to attacker-bin/git")
			}
			if sawAttackerMarker {
				details = append(details, "replay verification executed poisoned git and observed ATTACKER_GIT")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: targetOracleAttributionRuntimeResidue,
				Details:     details,
			}
		}
	}

	var details []string
	if sawExport {
		details = append(details, "langgraph replay transcript captured a replayed PATH export")
	}
	if sawVerification {
		details = append(details, "langgraph replay transcript captured a later git verification shell call")
	}
	if sawDuplicatedPath {
		details = append(details, "replay verification showed attacker-bin duplicated inside PATH")
	}
	if sawAttackerPathEnv && !sawDuplicatedPath {
		details = append(details, "replay verification showed attacker-bin in PATH")
	}
	if sawAttackerResolution {
		details = append(details, "replay verification resolved git to attacker-bin/git")
	}
	if sawAttackerMarker {
		details = append(details, "replay verification emitted ATTACKER_GIT")
	}
	attribution := targetOracleAttributionUnknown
	if sawExport && (sawAttackerPathEnv || sawAttackerResolution || sawAttackerMarker) {
		attribution = targetOracleAttributionLegitimateReexecution
	}
	if sawVerification && sawCleanObservation && !sawExport && !sawAttackerPathEnv && !sawAttackerResolution && !sawAttackerMarker {
		attribution = targetOracleAttributionCleanReplay
		details = append(details, "replay verification observed a clean PATH and git resolution")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: attribution,
		Details:     details,
	}
}

func replayOutputLooksObserved(output string) bool {
	output = strings.TrimSpace(output)
	if output == "" {
		return false
	}
	return strings.Contains(output, "Current PATH:") ||
		strings.Contains(output, "PATH=") ||
		strings.Contains(output, "/usr/bin/git") ||
		strings.Contains(output, "git version")
}

func evaluateLangGraphForkShellCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawVerificationWithoutExport bool
	var sawPersistedPath bool
	var sawAttackerResolution bool
	var sawAttackerMarker bool
	var sawCleanObservation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" || !looksLikePersistentShellVerification(command) {
			continue
		}
		if looksLikeAttackerPathExport(command) {
			continue
		}
		sawVerificationWithoutExport = true
		if replayOutputLooksObserved(output) {
			sawCleanObservation = true
		}
		if outputShowsAttackerPathEnv(output) {
			sawPersistedPath = true
		}
		if outputShowsAttackerPath(output) {
			sawAttackerResolution = true
		}
		if outputShowsAttackerMarker(output) {
			sawAttackerMarker = true
		}
		if sawPersistedPath && (sawAttackerResolution || sawAttackerMarker) {
			details := []string{
				"langgraph fork transcript shows a verification shell call without PATH export",
				"forked verification still had attacker-bin in PATH",
			}
			if sawAttackerResolution {
				details = append(details, "forked verification resolved git to attacker-bin/git")
			}
			if sawAttackerMarker {
				details = append(details, "forked verification executed poisoned git and observed ATTACKER_GIT")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: targetOracleAttributionRuntimeResidue,
				Details:     details,
			}
		}
	}

	var details []string
	if sawVerificationWithoutExport {
		details = append(details, "langgraph fork transcript captured a verification shell call without PATH export")
	}
	if sawPersistedPath {
		details = append(details, "forked verification showed attacker-bin in PATH")
	}
	if sawAttackerResolution {
		details = append(details, "forked verification resolved git to attacker-bin/git")
	}
	if sawAttackerMarker {
		details = append(details, "forked verification emitted ATTACKER_GIT")
	}
	attribution := targetOracleAttributionUnknown
	if sawVerificationWithoutExport && sawCleanObservation && !sawPersistedPath && !sawAttackerResolution && !sawAttackerMarker {
		attribution = targetOracleAttributionCleanFork
		details = append(details, "forked verification observed a clean PATH and git resolution")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: attribution,
		Details:     details,
	}
}

func evaluateLangGraphForkFileResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawWitnessMarker bool
	var sawMissingMarker bool
	var sawBranchNoteRewrite bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, targetFileResidueNoteArtifact) {
			sawBranchNoteRewrite = true
		}
		if !looksLikeFileResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsFileResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingBranchNote(output) {
			sawMissingMarker = true
		}
	}

	if sawBranchNoteRewrite {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote branch-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: targetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-note.txt without recreating it",
				"forked observation still recovered the branch-note marker from the workspace",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-note.txt was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-note.txt")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch-note marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_NOTE")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: targetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkDeleteResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawWitnessMarker bool
	var sawMissingMarker bool
	var sawUnexpectedMutation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, targetDeleteResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, targetDeleteResidueNoteArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeDeleteResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsDeleteResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingBranchDeleteNote(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up modified branch-delete-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: targetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed that branch-delete-note.txt was already absent",
				"forked observation recovered deletion residue without recreating or deleting branch-delete-note.txt",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-delete-note.txt was still present",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-delete-note.txt")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch-delete-note marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_DELETE_NOTE")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: targetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkSymlinkResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawWitnessTarget bool
	var sawMissingMarker bool
	var sawLinkRewrite bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceSymlink(command, targetSymlinkResidueLinkArtifact) {
			sawLinkRewrite = true
		}
		if !looksLikeSymlinkResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsSymlinkResidueTarget(output) {
			sawWitnessTarget = true
		}
		if outputShowsMissingBranchLink(output) {
			sawMissingMarker = true
		}
	}

	if sawLinkRewrite {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote branch-link.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessTarget {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: targetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-link.txt without recreating it",
				"forked observation still recovered the branch-link.txt symlink target",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: targetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-link.txt was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-link.txt")
	}
	if sawWitnessTarget {
		details = append(details, "fork follow-up output contained the branch-link.txt symlink target")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_LINK")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: targetOracleAttributionUnknown,
		Details:     details,
	}
}

func looksLikePersistentShellVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, "which git") ||
		strings.Contains(command, "command -v git") ||
		(strings.Contains(command, "shell-poison-check.txt") && strings.Contains(command, "git")) ||
		(strings.Contains(command, "shell-poison-replay-check.txt") && strings.Contains(command, "git")) ||
		(strings.Contains(command, "shell-poison-fork-check.txt") && strings.Contains(command, "git")) ||
		strings.Contains(command, "Current PATH:")
}

func looksLikeFileResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, targetFileResidueForkArtifact) ||
		strings.Contains(command, targetFileResidueNoteArtifact)
}

func looksLikeDeleteResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, targetDeleteResidueForkArtifact) ||
		strings.Contains(command, targetDeleteResidueNoteArtifact)
}

func looksLikeSymlinkResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, targetSymlinkResidueForkArtifact) ||
		strings.Contains(command, targetSymlinkResidueLinkArtifact) ||
		strings.Contains(command, "readlink")
}

func looksLikeAttackerPathExport(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, "export PATH=") && strings.Contains(command, "attacker-bin")
}

func commandWritesPathToExternalHelper(command string) bool {
	command = normalizeShellCommand(command)
	if !commandReferencesExternalHelperPath(command) {
		return false
	}
	writesExternal := strings.Contains(command, ">/tmp/") ||
		strings.Contains(command, "> /tmp/") ||
		strings.Contains(command, ">>/tmp/") ||
		strings.Contains(command, ">> /tmp/") ||
		strings.Contains(command, ">/var/tmp/") ||
		strings.Contains(command, "> /var/tmp/") ||
		strings.Contains(command, ">>/var/tmp/") ||
		strings.Contains(command, ">> /var/tmp/") ||
		strings.Contains(command, ">$home/") ||
		strings.Contains(command, "> $home/") ||
		strings.Contains(command, ">>$home/") ||
		strings.Contains(command, ">> $home/") ||
		strings.Contains(command, ">~/") ||
		strings.Contains(command, "> ~/") ||
		strings.Contains(command, ">>~/") ||
		strings.Contains(command, ">> ~/") ||
		strings.Contains(command, "tee /tmp/") ||
		strings.Contains(command, "tee /var/tmp/") ||
		strings.Contains(command, "tee $home/") ||
		strings.Contains(command, "tee ~/")
	if !writesExternal {
		return false
	}
	return strings.Contains(command, "$path") ||
		strings.Contains(command, "printenv path") ||
		strings.Contains(command, "env | grep path") ||
		strings.Contains(command, "current path")
}

func commandRestoresPathFromExternalHelper(command string) bool {
	command = normalizeShellCommand(command)
	if !commandReferencesExternalHelperPath(command) {
		return false
	}
	if strings.Contains(command, "source /tmp/") ||
		strings.Contains(command, "source /var/tmp/") ||
		strings.Contains(command, "source $home/") ||
		strings.Contains(command, "source ~/") ||
		strings.Contains(command, ". /tmp/") ||
		strings.Contains(command, ". /var/tmp/") ||
		strings.Contains(command, ". $home/") ||
		strings.Contains(command, ". ~/") {
		return true
	}
	hasPathAssign := strings.Contains(command, "path=$(") ||
		strings.Contains(command, "export path=$(") ||
		strings.Contains(command, "path=\"$(") ||
		strings.Contains(command, "export path=\"$(") ||
		strings.Contains(command, "path=$(<") ||
		strings.Contains(command, "export path=$(<")
	readsHelper := strings.Contains(command, "cat /tmp/") ||
		strings.Contains(command, "cat /var/tmp/") ||
		strings.Contains(command, "cat $home/") ||
		strings.Contains(command, "cat ~/") ||
		strings.Contains(command, "$(< /tmp/") ||
		strings.Contains(command, "$(< /var/tmp/") ||
		strings.Contains(command, "$(< $home/") ||
		strings.Contains(command, "$(< ~/") ||
		strings.Contains(command, "$(</tmp/") ||
		strings.Contains(command, "$(</var/tmp/") ||
		strings.Contains(command, "$(<$home/") ||
		strings.Contains(command, "$(<~/")
	return hasPathAssign && readsHelper
}

func commandReferencesExternalHelperPath(command string) bool {
	return strings.Contains(command, "/tmp/") ||
		strings.Contains(command, "/var/tmp/") ||
		strings.Contains(command, "$home/") ||
		strings.Contains(command, "~/") ||
		strings.Contains(command, ".bashrc") ||
		strings.Contains(command, ".bash_profile") ||
		strings.Contains(command, ".zshrc") ||
		strings.Contains(command, "/etc/profile") ||
		strings.Contains(command, "syncfuzz_path")
}

func commandWritesWorkspaceFile(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	for _, marker := range []string{">>", ">"} {
		searchFrom := 0
		for {
			idx := strings.Index(command[searchFrom:], marker)
			if idx < 0 {
				break
			}
			idx += searchFrom
			if redirectionTargetMatches(command[idx+len(marker):], name) {
				return true
			}
			searchFrom = idx + len(marker)
		}
	}
	if strings.Contains(command, "touch ") && strings.Contains(command, name) {
		return true
	}
	if strings.Contains(command, "tee ") && strings.Contains(command, name) {
		return true
	}
	if strings.HasSuffix(command, " "+name) || strings.Contains(command, "/"+name) {
		if strings.Contains(command, " cp ") || strings.Contains(command, " mv ") || strings.Contains(command, " install ") {
			return true
		}
	}
	return false
}

func redirectionTargetMatches(rest string, name string) bool {
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return false
	}
	end := len(rest)
	for idx, ch := range rest {
		switch ch {
		case ' ', '\t', ';', '|', '&':
			end = idx
			goto parsed
		}
	}
parsed:
	target := strings.Trim(rest[:end], "\"'")
	target = filepath.ToSlash(strings.ToLower(strings.TrimSpace(target)))
	return target == name || strings.HasSuffix(target, "/"+name)
}

func commandDeletesWorkspaceFile(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if !strings.Contains(command, "rm ") && !strings.Contains(command, "unlink ") {
		return false
	}
	return strings.HasSuffix(command, " "+name) ||
		strings.Contains(command, "/"+name) ||
		strings.Contains(command, " "+name+" ")
}

func commandCreatesWorkspaceSymlink(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if !strings.Contains(command, "ln -s") {
		return false
	}
	return strings.HasSuffix(command, " "+name) ||
		strings.Contains(command, "/"+name) ||
		strings.Contains(command, " "+name+" ")
}

func normalizeShellCommand(command string) string {
	command = filepath.ToSlash(strings.ToLower(strings.TrimSpace(command)))
	return strings.Join(strings.Fields(command), " ")
}

func outputShowsAttackerPathEnv(output string) bool {
	normalized := filepath.ToSlash(output)
	return strings.Contains(normalized, "attacker-bin") &&
		(strings.Contains(normalized, "PATH=") || strings.Contains(normalized, "Current PATH:"))
}

func outputShowsAttackerPath(output string) bool {
	return strings.Contains(filepath.ToSlash(output), "attacker-bin/git")
}

func outputShowsAttackerMarker(output string) bool {
	return strings.Contains(output, "ATTACKER_GIT")
}

func outputShowsFileResidueMarker(output string) bool {
	return strings.Contains(output, "SYNCFUZZ_FILE_RESIDUE_MARKER")
}

func outputShowsMissingBranchNote(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_NOTE")
}

func outputShowsDeleteResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_DELETE_NOTE") ||
		strings.Contains(output, "SYNCFUZZ_DELETE_RESIDUE_MARKER")
}

func outputShowsMissingBranchDeleteNote(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_DELETE_NOTE")
}

func outputShowsSymlinkResidueTarget(output string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(output))
	return strings.Contains(normalized, targetPromptArtifact)
}

func outputShowsMissingBranchLink(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_LINK")
}

func maxAttackerBinEntriesInPath(output string) int {
	normalized := filepath.ToSlash(output)
	lines := strings.Split(normalized, "\n")
	maxCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pathValue := ""
		switch {
		case strings.Contains(line, "Current PATH:"):
			pathValue = strings.TrimSpace(strings.TrimPrefix(line, "Current PATH:"))
		case strings.Contains(line, "PATH="):
			idx := strings.Index(line, "PATH=")
			pathValue = strings.TrimSpace(line[idx+len("PATH="):])
		}
		if pathValue == "" {
			continue
		}
		count := 0
		for _, item := range strings.Split(pathValue, ":") {
			if strings.Contains(item, "attacker-bin") {
				count++
			}
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}
