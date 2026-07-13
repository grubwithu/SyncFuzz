package target

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
	LanggraphReplayArtifact     = "langgraph-replay-summary.json"
	LanggraphForkArtifact       = "langgraph-fork-summary.json"
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
	SawExport   bool
	SawVerify   bool
	SawPath     bool
	SawResolve  bool
	SawMarker   bool
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
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphReplayArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphReplayShellCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphReplayStateSmuggling(workspace string) (langgraphReplayStateSmugglingEvidence, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return langgraphReplayStateSmugglingEvidence{}, err
	}
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphReplayArtifact)
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
	for _, call := range buildLangGraphShellCalls(operationFollowupMessages(summary)) {
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
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkShellCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkFileResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkFileResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkDirectoryResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkDirectoryResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkDeleteResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkDeleteResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkSymlinkResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkSymlinkResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkRenameResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkRenameResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkModeResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkModeResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkAppendResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkAppendResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkHardlinkResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkHardlinkResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkFIFOResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkFIFOResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkOpenFDResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkOpenFDResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkDeletedOpenFDResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkDeletedOpenFDResidueCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkInheritedFDLeakEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkInheritedFDLeakCalls(buildLangGraphShellCalls(operationFollowupMessages(summary)))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkUnixListenerResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkUnixListenerResidueCalls(operationShellCallsWithLifecycle(workspace, summary))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkDiscardedServerTrustedClientEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkDiscardedServerTrustedClientCalls(operationShellCallsWithLifecycle(workspace, summary))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkSocketResponsePoisoningEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkSocketResponsePoisoningCalls(operationShellCallsWithLifecycle(workspace, summary))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkCWDResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkCWDResidueCalls(operationShellCallsWithLifecycle(workspace, summary))
	evidence.Available = true
	return evidence, nil
}

func inspectLangGraphForkUmaskResidueEvidence(workspace string) (persistentShellTranscriptEvidence, error) {
	summary, err := loadLangGraphOperationSummary(workspace, LanggraphForkArtifact)
	if err != nil {
		return persistentShellTranscriptEvidence{}, err
	}
	if summary == nil {
		return persistentShellTranscriptEvidence{}, nil
	}
	evidence := evaluateLangGraphForkUmaskResidueCalls(operationShellCallsWithLifecycle(workspace, summary))
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

func operationFollowupMessages(summary *langgraphOperationSummary) []langgraphHistoryMessage {
	if summary == nil {
		return nil
	}
	userMessage := strings.TrimSpace(summary.UserMessage)
	if userMessage == "" {
		return summary.Messages
	}
	userMessage = compactWhitespace(userMessage)
	for i := len(summary.Messages) - 1; i >= 0; i-- {
		message := summary.Messages[i]
		if message.Role != "human" {
			continue
		}
		content := compactWhitespace(message.Content)
		if content == "" {
			continue
		}
		if strings.HasPrefix(userMessage, content) || strings.HasPrefix(content, userMessage) {
			return summary.Messages[i+1:]
		}
	}
	return summary.Messages
}

func operationShellCallsWithLifecycle(workspace string, summary *langgraphOperationSummary) []langgraphShellCall {
	summaryCalls := buildLangGraphShellCalls(operationFollowupMessages(summary))
	if summary == nil {
		return summaryCalls
	}
	lifecycleCalls, ok, err := loadLangGraphLifecycleShellCalls(workspace, summary.Operation)
	if err != nil || !ok || len(lifecycleCalls) == 0 {
		return summaryCalls
	}
	calls := attachShellCallOutputs(lifecycleCalls, summaryCalls)
	for _, call := range summaryCalls {
		if shellCallInSlice(calls, call) {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func shellCallInSlice(calls []langgraphShellCall, candidate langgraphShellCall) bool {
	candidateCommand := strings.TrimSpace(candidate.Command)
	candidateOutput := strings.TrimSpace(candidate.Output)
	for _, call := range calls {
		if strings.TrimSpace(call.Command) == candidateCommand &&
			strings.TrimSpace(call.Output) == candidateOutput {
			return true
		}
	}
	return false
}

func compactWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
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

func langgraphHistoryShowsWorkspaceDirectoryCreate(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandCreatesWorkspaceDirectory(call.Command, name) {
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

func langgraphHistoryShowsWorkspaceRename(workspace string, from string, to string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandRenamesWorkspaceFile(call.Command, from, to) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceModeChange(workspace string, name string, mode string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandChangesWorkspaceFileMode(call.Command, name, mode) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceAppend(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandAppendsWorkspaceFile(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceHardlinkCreate(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandCreatesWorkspaceHardlink(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceFIFOCreate(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandCreatesWorkspaceFIFO(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceOpenFD(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandOpensWorkspaceFD(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkspaceDeletedOpenFD(workspace string, name string) (bool, error) {
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandOpensDeletedWorkspaceFD(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsUnixListenerLaunch(workspace string) (bool, error) {
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return false, err
	}
	if ok {
		for _, call := range calls {
			if commandLaunchesUnixListener(call.Command) {
				return true, nil
			}
		}
	}
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandLaunchesUnixListener(call.Command) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsWorkingDirectoryChange(workspace string, name string) (bool, error) {
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return false, err
	}
	if ok {
		for _, call := range calls {
			if commandChangesWorkingDirectory(call.Command, name) {
				return true, nil
			}
		}
	}
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandChangesWorkingDirectory(call.Command, name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func langgraphHistoryShowsUmaskChange(workspace string, mode string) (bool, error) {
	calls, ok, err := loadPrimaryLangGraphShellCalls(workspace)
	if err != nil {
		return false, err
	}
	if ok {
		for _, call := range calls {
			if commandChangesUmask(call.Command, mode) {
				return true, nil
			}
		}
	}
	checkpoints, err := loadLangGraphHistory(workspace)
	if err != nil {
		return false, err
	}
	for _, checkpoint := range checkpoints {
		for _, call := range buildLangGraphShellCalls(checkpoint.Messages) {
			if commandChangesUmask(call.Command, mode) {
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
	raw, err := os.ReadFile(filepath.Join(workspace, TargetTaskArtifact))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", TargetTaskArtifact, err)
	}

	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return "", fmt.Errorf("decode %s: %w", TargetTaskArtifact, err)
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
		hasExport := looksLikeAttackerPathExport(command)
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
		if sawLaterVerification && (sawLaterAttackerResolution || sawLaterAttackerMarker) {
			details := []string{
				"langgraph transcript captured a later git verification shell call without another PATH export",
			}
			if sawLaterPersistedPath {
				details = append(details, "later shell call still showed the workspace-local tool directory inside PATH")
			}
			if sawLaterAttackerResolution {
				details = append(details, "later shell call resolved git to the workspace-local shim path")
			}
			if sawLaterAttackerMarker {
				details = append(details, "later shell call executed the workspace-local git shim and observed its marker output")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: TargetOracleAttributionRuntimeResidue,
				SawExport:   sawExport,
				SawVerify:   sawLaterVerification,
				SawPath:     sawLaterPersistedPath,
				SawResolve:  sawLaterAttackerResolution,
				SawMarker:   sawLaterAttackerMarker,
				Details:     details,
			}
		}
	}

	var details []string
	if sawExport {
		details = append(details, "langgraph transcript captured an earlier PATH export to the workspace-local tool directory")
	}
	if sawLaterVerification {
		details = append(details, "langgraph transcript captured a later git verification shell call")
	}
	if sawLaterPersistedPath {
		details = append(details, "later shell call showed the workspace-local tool directory in PATH")
	}
	if sawLaterAttackerResolution {
		details = append(details, "later shell call resolved git to the workspace-local shim path")
	}
	if sawLaterAttackerMarker {
		details = append(details, "later shell call emitted the workspace-local shim marker")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		SawExport:   sawExport,
		SawVerify:   sawLaterVerification,
		SawPath:     sawLaterPersistedPath,
		SawResolve:  sawLaterAttackerResolution,
		SawMarker:   sawLaterAttackerMarker,
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
				"langgraph replay transcript showed preserved workspace-local shell state after replay",
			}
			if sawDuplicatedPath {
				details = append(details, "replay verification showed the workspace-local tool directory duplicated inside PATH")
			} else {
				details = append(details, "replay verification still showed the workspace-local tool directory in PATH without replaying the export step")
			}
			if sawExport {
				details = append(details, "replay reran the workspace-local PATH export step")
			}
			if sawAttackerResolution {
				details = append(details, "replay verification resolved git to the workspace-local shim path")
			}
			if sawAttackerMarker {
				details = append(details, "replay verification executed the workspace-local git shim and observed its marker output")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: TargetOracleAttributionRuntimeResidue,
				Details:     details,
			}
		}
	}

	var details []string
	if sawExport {
		details = append(details, "langgraph replay transcript captured a replayed PATH export to the workspace-local tool directory")
	}
	if sawVerification {
		details = append(details, "langgraph replay transcript captured a later git verification shell call")
	}
	if sawDuplicatedPath {
		details = append(details, "replay verification showed the workspace-local tool directory duplicated inside PATH")
	}
	if sawAttackerPathEnv && !sawDuplicatedPath {
		details = append(details, "replay verification showed the workspace-local tool directory in PATH")
	}
	if sawAttackerResolution {
		details = append(details, "replay verification resolved git to the workspace-local shim path")
	}
	if sawAttackerMarker {
		details = append(details, "replay verification emitted the workspace-local shim marker")
	}
	attribution := TargetOracleAttributionUnknown
	if sawExport && (sawAttackerPathEnv || sawAttackerResolution || sawAttackerMarker) {
		attribution = TargetOracleAttributionLegitimateReexecution
	}
	if sawVerification && sawCleanObservation && !sawExport && !sawAttackerPathEnv && !sawAttackerResolution && !sawAttackerMarker {
		attribution = TargetOracleAttributionCleanReplay
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
				"langgraph fork transcript shows a verification shell call without another PATH export",
			}
			if sawPersistedPath {
				details = append(details, "forked verification still had the workspace-local tool directory in PATH")
			}
			if sawAttackerResolution {
				details = append(details, "forked verification resolved git to the workspace-local shim path")
			}
			if sawAttackerMarker {
				details = append(details, "forked verification executed the workspace-local git shim and observed its marker output")
			}
			return persistentShellTranscriptEvidence{
				Available:   true,
				Confirmed:   true,
				Attribution: TargetOracleAttributionRuntimeResidue,
				Details:     details,
			}
		}
	}

	var details []string
	if sawVerificationWithoutExport {
		details = append(details, "langgraph fork transcript captured a verification shell call without PATH export")
	}
	if sawPersistedPath {
		details = append(details, "forked verification showed the workspace-local tool directory in PATH")
	}
	if sawAttackerResolution {
		details = append(details, "forked verification resolved git to the workspace-local shim path")
	}
	if sawAttackerMarker {
		details = append(details, "forked verification emitted the workspace-local shim marker")
	}
	attribution := TargetOracleAttributionUnknown
	if sawVerificationWithoutExport && sawCleanObservation && !sawPersistedPath && !sawAttackerResolution && !sawAttackerMarker {
		attribution = TargetOracleAttributionCleanFork
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
		if commandWritesWorkspaceFile(command, TargetFileResidueNoteArtifact) {
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
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote branch-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
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
			Attribution: TargetOracleAttributionCleanFork,
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
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkDirectoryResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawWitnessMarker bool
	var sawMissingMarker bool
	var sawDirectoryRewrite bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandCreatesWorkspaceDirectory(command, TargetDirectoryResidueDirArtifact) {
			sawDirectoryRewrite = true
		}
		if !looksLikeDirectoryResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsDirectoryResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingBranchDir(output) {
			sawMissingMarker = true
		}
	}

	if sawDirectoryRewrite {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up recreated branch-dir instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-dir without recreating it",
				"forked observation still recovered branch-dir from the workspace",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-dir was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-dir")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch-dir marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_DIR")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
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
		if commandWritesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDeleteResidueNoteArtifact) {
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
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up modified branch-delete-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
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
			Attribution: TargetOracleAttributionCleanFork,
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
		Attribution: TargetOracleAttributionUnknown,
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
		if commandCreatesWorkspaceSymlink(command, TargetSymlinkResidueLinkArtifact) {
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
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote branch-link.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessTarget {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
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
			Attribution: TargetOracleAttributionCleanFork,
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
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkRenameResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawSourceMarker bool
	var sawDestMarker bool
	var sawMissingMarker bool
	var sawUnexpectedMutation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetRenameResidueSourceArtifact) ||
			commandWritesWorkspaceFile(command, TargetRenameResidueDestArtifact) ||
			commandDeletesWorkspaceFile(command, TargetRenameResidueSourceArtifact) ||
			commandDeletesWorkspaceFile(command, TargetRenameResidueDestArtifact) ||
			commandRenamesWorkspaceFile(command, TargetRenameResidueSourceArtifact, TargetRenameResidueDestArtifact) ||
			commandRenamesWorkspaceFile(command, TargetRenameResidueDestArtifact, TargetRenameResidueSourceArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeRenameResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsRenameResidueSource(output) {
			sawSourceMarker = true
		}
		if outputShowsRenameResidueDest(output) {
			sawDestMarker = true
		}
		if outputShowsMissingRenameArtifacts(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up recreated, deleted, or renamed the branch-rename files instead of only observing them",
			},
		}
	}
	if sawObservation && sawDestMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-rename-dst.txt without recreating it",
				"forked observation still recovered the renamed destination file from the workspace",
			},
		}
	}
	if sawObservation && sawSourceMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the original branch-rename-src.txt still existed",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe which rename-side file existed")
	}
	if sawDestMarker {
		details = append(details, "fork follow-up output reported PRESENT_BRANCH_RENAME_DST")
	}
	if sawSourceMarker {
		details = append(details, "fork follow-up output reported PRESENT_BRANCH_RENAME_SRC")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_RENAME_FILES")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkModeResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawTightenedMode bool
	var sawMode644 bool
	var sawMissingMarker bool
	var sawUnexpectedMutation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetModeResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetModeResidueNoteArtifact) ||
			commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, targetModeResidueTightenedMode) ||
			commandChangesWorkspaceFileMode(command, TargetModeResidueNoteArtifact, "644") {
			sawUnexpectedMutation = true
		}
		if !looksLikeModeResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsModeResidue(output, targetModeResidueTightenedMode) {
			sawTightenedMode = true
		}
		if outputShowsModeResidue(output, "644") {
			sawMode644 = true
		}
		if outputShowsMissingBranchModeNote(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote, deleted, or chmod-ed branch-mode-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawTightenedMode {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-mode-note.txt without changing it",
				"forked observation still recovered the tightened " + targetModeResidueTightenedMode + " mode from the workspace",
			},
		}
	}
	if sawObservation && sawMode644 {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-mode-note.txt had the earlier 0644 mode",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe the file mode of branch-mode-note.txt")
	}
	if sawTightenedMode {
		details = append(details, "fork follow-up output reported MODE="+targetModeResidueTightenedMode)
	}
	if sawMode644 {
		details = append(details, "fork follow-up output reported MODE=644")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_MODE_NOTE")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkAppendResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawBaseMarker bool
	var sawExtraMarker bool
	var sawMissingMarker bool
	var sawUnexpectedMutation bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		output := strings.TrimSpace(call.Output)
		if command == "" {
			continue
		}
		if commandWritesWorkspaceFile(command, TargetAppendResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetAppendResidueNoteArtifact) ||
			commandAppendsWorkspaceFile(command, TargetAppendResidueNoteArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeAppendResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsAppendResidueBaseMarker(output) {
			sawBaseMarker = true
		}
		if outputShowsAppendResidueExtraMarker(output) {
			sawExtraMarker = true
		}
		if outputShowsMissingBranchAppendNote(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up rewrote, deleted, or appended branch-append-note.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawBaseMarker && sawExtraMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-append-note.txt without rewriting it",
				"forked observation still recovered the appended extra marker from the workspace",
			},
		}
	}
	if sawObservation && sawBaseMarker && !sawExtraMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed only the base marker in branch-append-note.txt",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-append-note.txt")
	}
	if sawBaseMarker {
		details = append(details, "fork follow-up output contained the base append marker")
	}
	if sawExtraMarker {
		details = append(details, "fork follow-up output contained the appended extra marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_APPEND_NOTE")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkHardlinkResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandCreatesWorkspaceHardlink(command, TargetHardlinkResidueLinkArtifact) ||
			commandDeletesWorkspaceFile(command, TargetHardlinkResidueLinkArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeHardlinkResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsHardlinkResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingBranchHardlink(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up recreated or deleted branch-hardlink.txt instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-hardlink.txt without recreating it",
				"forked observation still recovered the workspace hardlink from the fork workspace",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-hardlink.txt was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-hardlink.txt")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch-hardlink marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_HARDLINK")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkFIFOResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandCreatesWorkspaceFIFO(command, TargetFIFOResiduePipeArtifact) ||
			commandDeletesWorkspaceFile(command, TargetFIFOResiduePipeArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeFIFOResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsFIFOResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingBranchFIFO(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up recreated or deleted branch-fifo instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed branch-fifo without recreating it",
				"forked observation still recovered the named pipe from the fork workspace",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that branch-fifo was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe branch-fifo")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch-fifo marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_BRANCH_FIFO")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkOpenFDResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandOpensWorkspaceFD(command, TargetOpenFDResidueNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetOpenFDResidueNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetOpenFDResiduePIDArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeOpenFDResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsOpenFDResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingOpenFDResidue(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or modified the branch-fd-note.txt holder instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed an existing branch-fd-note.txt holder without relaunching it",
				"forked observation still recovered the open file descriptor from the fork runtime",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the branch-fd-note.txt holder was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe the branch-fd-note.txt holder")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch open-fd marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported a missing branch open-fd holder")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkDeletedOpenFDResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandOpensDeletedWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandOpensWorkspaceFD(command, TargetDeletedOpenFDNoteArtifact) ||
			commandDeletesWorkspaceFile(command, TargetDeletedOpenFDNoteArtifact) ||
			commandWritesWorkspaceFile(command, TargetDeletedOpenFDPIDArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeDeletedOpenFDResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsDeletedOpenFDResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingDeletedOpenFDResidue(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or modified the deleted branch-deleted-fd-note.txt holder instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up observed an existing deleted branch-deleted-fd-note.txt holder without relaunching it",
				"forked observation still recovered the deleted open file descriptor from the fork runtime",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the deleted branch-deleted-fd-note.txt holder was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe the deleted branch-deleted-fd-note.txt holder")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the deleted branch open-fd marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported a missing deleted branch open-fd holder")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkInheritedFDLeakCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandOpensDeletedWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandOpensWorkspaceFD(command, TargetInheritedFDLeakSecretArtifact) ||
			commandDeletesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakSecretArtifact) ||
			commandWritesWorkspaceFile(command, TargetInheritedFDLeakPIDArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeInheritedFDLeakVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsInheritedFDLeakageMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingInheritedFDLeakage(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or recreated the discarded branch fd secret instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up read the discarded branch secret through an existing fd holder",
				"forked observation activated the inherited fd capability from the successor branch",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the inherited fd branch secret was absent",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to observe the inherited fd branch secret")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the inherited fd branch secret marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported a missing inherited fd branch secret")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkUnixListenerResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeUnixListenerResidueVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsUnixListenerResidueMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingUnixListenerResidue(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or recreated the branch Unix listener instead of only observing it",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up connected to an existing branch Unix listener without relaunching it",
				"forked observation still received a response from the discarded branch service",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the branch Unix listener was absent or unreachable",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted to connect to the branch Unix listener")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output contained the branch Unix listener marker")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported a missing branch Unix listener")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkDiscardedServerTrustedClientCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeDiscardedServerTrustedClientVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsDiscardedServerTrustedClientMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingDiscardedServerTrustedClient(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or recreated the branch Unix listener instead of only running the trusted-client observation",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up ran the trusted-client observation without relaunching the branch Unix listener",
				"forked observation still consumed the discarded branch service response",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the trusted-client step could not consume the discarded branch service response",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted a trusted-client observation against branch-listener.sock")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output reported PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_DISCARDED_SERVER_TRUSTED_CLIENT")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkSocketResponsePoisoningCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
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
		if commandLaunchesUnixListener(command) ||
			commandWritesWorkspaceFile(command, TargetUnixListenerPIDArtifact) ||
			commandDeletesWorkspaceFile(command, TargetUnixListenerSocketArtifact) {
			sawUnexpectedMutation = true
		}
		if !looksLikeSocketResponsePoisoningVerification(command) {
			continue
		}
		sawObservation = true
		if outputShowsSocketResponsePoisoningMarker(output) {
			sawWitnessMarker = true
		}
		if outputShowsMissingSocketResponsePoisoning(output) {
			sawMissingMarker = true
		}
	}

	if sawUnexpectedMutation {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up relaunched or recreated the branch Unix listener instead of only running the cache observation",
			},
		}
	}
	if sawObservation && sawWitnessMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   true,
			Attribution: TargetOracleAttributionRuntimeResidue,
			Details: []string{
				"langgraph fork follow-up ran the cache observation without relaunching the branch Unix listener",
				"forked observation still cached the discarded branch service response",
			},
		}
	}
	if sawObservation && sawMissingMarker {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionCleanFork,
			Details: []string{
				"langgraph fork follow-up observed that the cache step could not recover the discarded branch service response",
			},
		}
	}

	var details []string
	if sawObservation {
		details = append(details, "langgraph fork follow-up attempted a cache observation against branch-listener.sock")
	}
	if sawWitnessMarker {
		details = append(details, "fork follow-up output reported PRESENT_SOCKET_RESPONSE_POISONING")
	}
	if sawMissingMarker {
		details = append(details, "fork follow-up output reported MISSING_SOCKET_RESPONSE_POISONING")
	}
	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details:     details,
	}
}

func evaluateLangGraphForkCWDResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawDirectoryChange bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandChangesWorkingDirectory(command, "") {
			sawDirectoryChange = true
		}
		if looksLikeCWDResidueVerification(command) {
			sawObservation = true
		}
	}

	if sawDirectoryChange {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up changed directories instead of only observing the inherited cwd",
			},
		}
	}
	if sawObservation {
		return persistentShellTranscriptEvidence{
			Available: true,
			Confirmed: true,
			Details: []string{
				"langgraph fork follow-up observed the active working directory without running cd again",
			},
		}
	}

	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details: []string{
			"langgraph fork follow-up did not show the expected cwd observation command",
		},
	}
}

func evaluateLangGraphForkUmaskResidueCalls(calls []langgraphShellCall) persistentShellTranscriptEvidence {
	var sawObservation bool
	var sawUmaskChange bool

	for _, call := range calls {
		command := strings.TrimSpace(call.Command)
		if command == "" {
			continue
		}
		if commandChangesUmask(command, "") {
			sawUmaskChange = true
		}
		if looksLikeUmaskResidueVerification(command) {
			sawObservation = true
		}
	}

	if sawUmaskChange {
		return persistentShellTranscriptEvidence{
			Available:   true,
			Confirmed:   false,
			Attribution: TargetOracleAttributionWorkspaceRebuild,
			Details: []string{
				"langgraph fork follow-up changed the shell umask instead of only observing the inherited file-creation mode",
			},
		}
	}
	if sawObservation {
		return persistentShellTranscriptEvidence{
			Available: true,
			Confirmed: true,
			Details: []string{
				"langgraph fork follow-up created the umask witness without running umask again",
			},
		}
	}

	return persistentShellTranscriptEvidence{
		Available:   true,
		Confirmed:   false,
		Attribution: TargetOracleAttributionUnknown,
		Details: []string{
			"langgraph fork follow-up did not show the expected umask observation command",
		},
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
	return strings.Contains(command, TargetFileResidueForkArtifact) ||
		strings.Contains(command, TargetFileResidueCheckArtifact) ||
		strings.Contains(command, TargetFileResidueNoteArtifact)
}

func looksLikeDirectoryResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetDirectoryResidueForkArtifact) ||
		strings.Contains(command, TargetDirectoryResidueCheckArtifact) ||
		strings.Contains(command, TargetDirectoryResidueDirArtifact)
}

func looksLikeDeleteResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetDeleteResidueForkArtifact) ||
		strings.Contains(command, TargetDeleteResidueCheckArtifact) ||
		strings.Contains(command, TargetDeleteResidueNoteArtifact)
}

func looksLikeSymlinkResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetSymlinkResidueForkArtifact) ||
		strings.Contains(command, TargetSymlinkResidueCheckArtifact) ||
		strings.Contains(command, TargetSymlinkResidueLinkArtifact) ||
		strings.Contains(command, "readlink")
}

func looksLikeRenameResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetRenameResidueForkArtifact) ||
		strings.Contains(command, TargetRenameResidueCheckArtifact) ||
		strings.Contains(command, TargetRenameResidueSourceArtifact) ||
		strings.Contains(command, TargetRenameResidueDestArtifact)
}

func looksLikeModeResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetModeResidueForkArtifact) ||
		strings.Contains(command, TargetModeResidueCheckArtifact) ||
		(strings.Contains(command, TargetModeResidueNoteArtifact) && strings.Contains(command, "stat"))
}

func looksLikeAppendResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetAppendResidueForkArtifact) ||
		strings.Contains(command, TargetAppendResidueCheckArtifact) ||
		strings.Contains(command, TargetAppendResidueNoteArtifact)
}

func looksLikeHardlinkResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetHardlinkResidueForkArtifact) ||
		strings.Contains(command, TargetHardlinkResidueCheckArtifact) ||
		strings.Contains(command, TargetHardlinkResidueLinkArtifact)
}

func looksLikeFIFOResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetFIFOResidueForkArtifact) ||
		strings.Contains(command, TargetFIFOResidueCheckArtifact) ||
		strings.Contains(command, TargetFIFOResiduePipeArtifact)
}

func looksLikeOpenFDResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetOpenFDResidueForkArtifact) ||
		strings.Contains(command, TargetOpenFDResiduePIDArtifact) ||
		strings.Contains(command, TargetOpenFDResidueNoteArtifact) ||
		strings.Contains(command, "/proc/") ||
		strings.Contains(command, "readlink")
}

func looksLikeDeletedOpenFDResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetDeletedOpenFDForkArtifact) ||
		strings.Contains(command, TargetDeletedOpenFDPIDArtifact) ||
		strings.Contains(command, TargetDeletedOpenFDNoteArtifact) ||
		strings.Contains(command, "/proc/") ||
		strings.Contains(command, "readlink")
}

func looksLikeInheritedFDLeakVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetInheritedFDLeakForkArtifact) ||
		strings.Contains(command, TargetInheritedFDLeakPIDArtifact) ||
		strings.Contains(command, TargetInheritedFDLeakSecretArtifact) ||
		strings.Contains(command, "/proc/") ||
		strings.Contains(command, "readlink")
}

func looksLikeUnixListenerResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetUnixListenerForkArtifact) ||
		strings.Contains(command, TargetUnixListenerSocketArtifact) ||
		strings.Contains(command, "connect(") ||
		strings.Contains(command, "SYNCFUZZ_UNIX_LISTENER_RESPONSE")
}

func looksLikeDiscardedServerTrustedClientVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetDiscardedServerTrustedClientArtifact) ||
		strings.Contains(command, TargetTrustedClientResponseArtifact) ||
		strings.Contains(command, TargetUnixListenerSocketArtifact) ||
		strings.Contains(command, "connect(") ||
		strings.Contains(command, "SYNCFUZZ_TRUSTED_CLIENT_MESSAGE")
}

func looksLikeSocketResponsePoisoningVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetSocketResponsePoisoningArtifact) ||
		strings.Contains(command, TargetTrustedClientCacheArtifact) ||
		strings.Contains(command, TargetUnixListenerSocketArtifact) ||
		strings.Contains(command, "connect(") ||
		strings.Contains(command, "SYNCFUZZ_SOCKET_POISON_PAYLOAD")
}

func looksLikeCWDResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetCWDResidueForkArtifact) ||
		strings.Contains(command, TargetCWDResidueWitnessArtifact) ||
		strings.Contains(command, "pwd")
}

func looksLikeUmaskResidueVerification(command string) bool {
	command = strings.TrimSpace(command)
	return strings.Contains(command, TargetUmaskResidueForkArtifact) ||
		strings.Contains(command, TargetUmaskResidueWitnessArtifact) ||
		strings.Contains(command, TargetUmaskResidueBaselineArtifact)
}

func looksLikeAttackerPathExport(command string) bool {
	command = strings.TrimSpace(command)
	if !strings.Contains(command, "export PATH=") {
		return false
	}
	return strings.Contains(command, TargetShellShimDirArtifact) ||
		strings.Contains(command, TargetShellLegacyShimDirArtifact)
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

func commandWritesPathToWorkspaceHelper(command string) bool {
	command = normalizeShellCommand(command)
	writesWorkspace := strings.Contains(command, "> workspace-bin/") ||
		strings.Contains(command, ">workspace-bin/") ||
		strings.Contains(command, ">> workspace-bin/") ||
		strings.Contains(command, ">>workspace-bin/") ||
		strings.Contains(command, "tee workspace-bin/") ||
		strings.Contains(command, "> attacker-bin/") ||
		strings.Contains(command, ">attacker-bin/") ||
		strings.Contains(command, ">> attacker-bin/") ||
		strings.Contains(command, ">>attacker-bin/") ||
		strings.Contains(command, "tee attacker-bin/")
	if !writesWorkspace {
		return false
	}
	return strings.Contains(command, "$path") ||
		strings.Contains(command, "printenv path") ||
		strings.Contains(command, "env | grep path") ||
		strings.Contains(command, "current path")
}

func commandRestoresPathFromWorkspaceHelper(command string) bool {
	command = normalizeShellCommand(command)
	if strings.Contains(command, "source workspace-bin/") ||
		strings.Contains(command, "source attacker-bin/") ||
		strings.Contains(command, ". workspace-bin/") ||
		strings.Contains(command, ". attacker-bin/") {
		return true
	}
	hasPathAssign := strings.Contains(command, "path=$(") ||
		strings.Contains(command, "export path=$(") ||
		strings.Contains(command, "path=\"$(") ||
		strings.Contains(command, "export path=\"$(") ||
		strings.Contains(command, "path=$(<") ||
		strings.Contains(command, "export path=$(<")
	readsHelper := strings.Contains(command, "cat workspace-bin/") ||
		strings.Contains(command, "cat attacker-bin/") ||
		strings.Contains(command, "$(< workspace-bin/") ||
		strings.Contains(command, "$(< attacker-bin/") ||
		strings.Contains(command, "$( <workspace-bin/") ||
		strings.Contains(command, "$( <attacker-bin/") ||
		strings.Contains(command, "$( < workspace-bin/") ||
		strings.Contains(command, "$( < attacker-bin/") ||
		strings.Contains(command, "$(</workspace-bin/") ||
		strings.Contains(command, "$(</attacker-bin/")
	return hasPathAssign && readsHelper
}

func commandBridgesWorkspaceShimToExternalHelper(command string) bool {
	command = normalizeShellCommand(command)
	if !commandReferencesExternalHelperPath(command) {
		return false
	}
	if !strings.Contains(command, TargetShellShimExecArtifact) {
		return false
	}
	return strings.Contains(command, " ln -s ") ||
		strings.Contains(command, " ln -sf ") ||
		strings.Contains(command, " ln -fs ") ||
		strings.Contains(command, " cp ") ||
		strings.Contains(command, " install ") ||
		strings.Contains(command, " mv ")
}

func commandReferencesExternalHelperPath(command string) bool {
	return strings.Contains(command, "/tmp/") ||
		strings.Contains(command, "/var/tmp/") ||
		strings.Contains(command, "$home/") ||
		strings.Contains(command, "$home/.local/bin/") ||
		strings.Contains(command, "~/.local/bin/") ||
		strings.Contains(command, "~/") ||
		strings.Contains(command, "/.local/bin/") ||
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
		strings.Contains(command, " "+name+" ") ||
		strings.Contains(command, " "+name+";") ||
		strings.Contains(command, " "+name+" &&") ||
		strings.Contains(command, " "+name+" ||")
}

func commandCreatesWorkspaceDirectory(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if !strings.Contains(command, "mkdir ") && !strings.Contains(command, "install -d ") {
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

func commandCreatesWorkspaceHardlink(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if !strings.Contains(command, "ln ") || strings.Contains(command, "ln -s") {
		return false
	}
	return strings.HasSuffix(command, " "+name) ||
		strings.Contains(command, "/"+name) ||
		strings.Contains(command, " "+name+" ")
}

func commandCreatesWorkspaceFIFO(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if !strings.Contains(command, "mkfifo ") {
		return false
	}
	return strings.HasSuffix(command, " "+name) ||
		strings.Contains(command, "/"+name) ||
		strings.Contains(command, " "+name+" ")
}

func commandLaunchesUnixListener(command string) bool {
	command = normalizeShellCommand(command)
	return strings.Contains(command, TargetUnixListenerSocketArtifact) &&
		(strings.Contains(command, "socket.af_unix") || strings.Contains(command, "af_unix")) &&
		(strings.Contains(command, ".bind(") || strings.Contains(command, "bind(")) &&
		(strings.Contains(command, ".listen(") || strings.Contains(command, "listen("))
}

func commandOpensWorkspaceFD(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	return strings.Contains(command, "exec 9<"+name) ||
		(strings.Contains(command, "exec 9<") && strings.Contains(command, "/"+name))
}

func commandOpensDeletedWorkspaceFD(command string, name string) bool {
	return commandOpensWorkspaceFD(command, name) &&
		commandDeletesWorkspaceFile(command, name)
}

func commandRenamesWorkspaceFile(command string, from string, to string) bool {
	command = normalizeShellCommand(command)
	from = filepath.ToSlash(strings.ToLower(strings.TrimSpace(from)))
	to = filepath.ToSlash(strings.ToLower(strings.TrimSpace(to)))
	if !strings.Contains(command, "mv ") && !strings.Contains(command, "rename ") {
		return false
	}
	return (strings.Contains(command, " "+from+" ") || strings.Contains(command, "/"+from+" ")) &&
		(strings.HasSuffix(command, " "+to) || strings.Contains(command, " "+to+" ") || strings.Contains(command, "/"+to))
}

func commandChangesWorkspaceFileMode(command string, name string, mode string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	mode = strings.TrimSpace(strings.ToLower(mode))
	if !strings.Contains(command, "chmod ") {
		return false
	}
	return (strings.Contains(command, "chmod "+mode+" ") || strings.Contains(command, "chmod 0"+mode+" ")) &&
		(strings.HasSuffix(command, " "+name) || strings.Contains(command, "/"+name) || strings.Contains(command, " "+name+" "))
}

func commandChangesWorkingDirectory(command string, name string) bool {
	command = normalizeShellCommand(command)
	fields := strings.Fields(command)
	for i := 0; i < len(fields)-1; i++ {
		if trimShellCommandToken(fields[i]) != "cd" {
			continue
		}
		if i > 0 && !shellTokenStartsCommand(fields[i-1]) {
			continue
		}
		target := trimShellCommandToken(fields[i+1])
		if target == "" {
			continue
		}
		if name == "" {
			return true
		}
		normalizedName := filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
		if target == normalizedName || strings.HasSuffix(target, "/"+normalizedName) {
			return true
		}
	}
	return false
}

func commandChangesUmask(command string, mode string) bool {
	command = normalizeShellCommand(command)
	fields := strings.Fields(command)
	expected := normalizeOctalCommandToken(mode)
	for i := 0; i < len(fields)-1; i++ {
		if trimShellCommandToken(fields[i]) != "umask" {
			continue
		}
		if i > 0 && !shellTokenStartsCommand(fields[i-1]) {
			continue
		}
		candidate := normalizeOctalCommandToken(fields[i+1])
		if candidate == "" {
			continue
		}
		if expected == "" || candidate == expected {
			return true
		}
	}
	return false
}

func commandAppendsWorkspaceFile(command string, name string) bool {
	command = normalizeShellCommand(command)
	name = filepath.ToSlash(strings.ToLower(strings.TrimSpace(name)))
	if strings.Contains(command, ">>") && (strings.HasSuffix(command, " "+name) || strings.Contains(command, "/"+name) || strings.Contains(command, ">>"+name)) {
		return true
	}
	return strings.Contains(command, "tee -a ") && (strings.HasSuffix(command, " "+name) || strings.Contains(command, "/"+name) || strings.Contains(command, " "+name+" "))
}

func normalizeShellCommand(command string) string {
	command = filepath.ToSlash(strings.ToLower(strings.TrimSpace(command)))
	return strings.Join(strings.Fields(command), " ")
}

func trimShellCommandToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "\"'`(){}[];,")
	token = strings.TrimSuffix(token, "&&")
	token = strings.TrimSuffix(token, "||")
	return filepath.ToSlash(strings.ToLower(strings.TrimSpace(token)))
}

func shellTokenStartsCommand(token string) bool {
	normalized := trimShellCommandToken(token)
	raw := strings.TrimSpace(strings.ToLower(token))
	switch normalized {
	case "", "then", "do", "{":
		return true
	}
	return raw == ";" ||
		raw == "&&" ||
		raw == "||" ||
		raw == "(" ||
		strings.HasSuffix(raw, ";") ||
		strings.HasSuffix(raw, "&&") ||
		strings.HasSuffix(raw, "||") ||
		strings.HasSuffix(raw, "(")
}

func normalizeOctalCommandToken(token string) string {
	token = trimShellCommandToken(token)
	if token == "" {
		return ""
	}
	for _, ch := range token {
		if ch < '0' || ch > '7' {
			return ""
		}
	}
	token = strings.TrimLeft(token, "0")
	if token == "" {
		return "0"
	}
	return token
}

func outputShowsAttackerPathEnv(output string) bool {
	normalized := filepath.ToSlash(output)
	if !strings.Contains(normalized, "PATH=") && !strings.Contains(normalized, "Current PATH:") {
		return false
	}
	return strings.Contains(normalized, TargetShellShimDirArtifact) ||
		strings.Contains(normalized, TargetShellLegacyShimDirArtifact)
}

func outputShowsAttackerPath(output string) bool {
	normalized := filepath.ToSlash(output)
	return strings.Contains(normalized, TargetShellShimExecArtifact) ||
		strings.Contains(normalized, TargetShellLegacyShimDirArtifact+"/git")
}

func outputShowsAttackerMarker(output string) bool {
	return strings.Contains(output, targetShellShimMarker) ||
		strings.Contains(output, targetShellLegacyShimMarker)
}

func outputShowsFileResidueMarker(output string) bool {
	return strings.Contains(output, "SYNCFUZZ_FILE_RESIDUE_MARKER")
}

func outputShowsMissingBranchNote(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_NOTE")
}

func outputShowsDirectoryResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_DIR")
}

func outputShowsMissingBranchDir(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_DIR")
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
	return strings.Contains(normalized, TargetPromptArtifact)
}

func outputShowsMissingBranchLink(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_LINK")
}

func outputShowsRenameResidueSource(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_RENAME_SRC") &&
		strings.Contains(output, "SYNCFUZZ_RENAME_RESIDUE_MARKER")
}

func outputShowsRenameResidueDest(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_RENAME_DST") &&
		strings.Contains(output, "SYNCFUZZ_RENAME_RESIDUE_MARKER")
}

func outputShowsMissingRenameArtifacts(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_RENAME_FILES")
}

func outputShowsModeResidue(output string, mode string) bool {
	return strings.Contains(output, "MODE="+mode)
}

func outputShowsMissingBranchModeNote(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_MODE_NOTE")
}

func outputShowsAppendResidueBaseMarker(output string) bool {
	return strings.Contains(output, "SYNCFUZZ_APPEND_BASE_MARKER")
}

func outputShowsAppendResidueExtraMarker(output string) bool {
	return strings.Contains(output, "SYNCFUZZ_APPEND_EXTRA_MARKER")
}

func outputShowsMissingBranchAppendNote(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_APPEND_NOTE")
}

func outputShowsHardlinkResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_HARDLINK")
}

func outputShowsMissingBranchHardlink(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_HARDLINK")
}

func outputShowsFIFOResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_FIFO")
}

func outputShowsMissingBranchFIFO(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_FIFO")
}

func outputShowsOpenFDResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_OPEN_FD") &&
		strings.Contains(output, TargetOpenFDResidueNoteArtifact)
}

func outputShowsMissingOpenFDResidue(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_OPEN_FD")
}

func outputShowsDeletedOpenFDResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_DELETED_OPEN_FD") &&
		strings.Contains(output, TargetDeletedOpenFDNoteArtifact) &&
		strings.Contains(output, "(deleted)")
}

func outputShowsMissingDeletedOpenFDResidue(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_DELETED_OPEN_FD")
}

func outputShowsInheritedFDLeakageMarker(output string) bool {
	return strings.Contains(output, "PRESENT_INHERITED_FD_BRANCH_LEAKAGE") &&
		strings.Contains(output, "SYNCFUZZ_INHERITED_FD_BRANCH_SECRET")
}

func outputShowsMissingInheritedFDLeakage(output string) bool {
	return strings.Contains(output, "MISSING_INHERITED_FD_BRANCH_LEAKAGE")
}

func outputShowsUnixListenerResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_UNIX_LISTENER") &&
		strings.Contains(output, "SYNCFUZZ_UNIX_LISTENER_RESPONSE")
}

func outputShowsMissingUnixListenerResidue(output string) bool {
	return strings.Contains(output, "MISSING_BRANCH_UNIX_LISTENER")
}

func outputShowsDiscardedServerTrustedClientMarker(output string) bool {
	return strings.Contains(output, "PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT") &&
		strings.Contains(output, "SYNCFUZZ_TRUSTED_CLIENT_MESSAGE")
}

func outputShowsMissingDiscardedServerTrustedClient(output string) bool {
	return strings.Contains(output, "MISSING_DISCARDED_SERVER_TRUSTED_CLIENT")
}

func outputShowsSocketResponsePoisoningMarker(output string) bool {
	return strings.Contains(output, "PRESENT_SOCKET_RESPONSE_POISONING") &&
		strings.Contains(output, "SYNCFUZZ_SOCKET_POISON_PAYLOAD")
}

func outputShowsMissingSocketResponsePoisoning(output string) bool {
	return strings.Contains(output, "MISSING_SOCKET_RESPONSE_POISONING")
}

func outputShowsCWDResidueMarker(output string) bool {
	return strings.Contains(output, "PRESENT_BRANCH_CWD_RESIDUE")
}

func outputShowsMissingBranchCWDResidue(output string) bool {
	return strings.Contains(output, "CLEAN_BRANCH_CWD")
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
			if strings.Contains(item, TargetShellShimDirArtifact) ||
				strings.Contains(item, TargetShellLegacyShimDirArtifact) {
				count++
			}
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}
