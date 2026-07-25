package synthesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const (
	LangGraphLifecycleArtifactName   = "langgraph-lifecycle.json"
	LangGraphLifecycleArtifactSchema = "syncfuzz.langgraph-lifecycle.v1"
)

// LangGraphLifecycleArtifact is target-owned command-lifecycle evidence. The
// command preview is deliberately excluded from this decoder because a causal
// binding needs stable IDs, hashes, and timestamps rather than task prose.
type LangGraphLifecycleArtifact struct {
	SchemaVersion string                    `json:"schema_version"`
	ThreadID      string                    `json:"thread_id"`
	ClockDomain   string                    `json:"clock_domain,omitempty"`
	Events        []LangGraphLifecycleEvent `json:"events"`
}

type LangGraphLifecycleEvent struct {
	Index       int    `json:"index"`
	Event       string `json:"event"`
	MonotonicNS uint64 `json:"monotonic_ns,omitempty"`

	ToolCallID     string `json:"tool_call_id,omitempty"`
	ShellSessionID string `json:"shell_session_id,omitempty"`
	CommandSHA256  string `json:"command_sha256,omitempty"`
}

func InferLangGraphLifecycleArtifactPath(run objective.ProfileRun) (string, error) {
	planArtifact := strings.TrimSpace(run.RecordedPlanArtifact)
	if planArtifact == "" {
		return "", fmt.Errorf("LangGraph profile run lacks a recorded target plan artifact")
	}
	return filepath.Join(filepath.Dir(planArtifact), "workspace", LangGraphLifecycleArtifactName), nil
}

func ReadLangGraphLifecycleArtifact(path string) (LangGraphLifecycleArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LangGraphLifecycleArtifact{}, fmt.Errorf("read LangGraph lifecycle artifact %s: %w", path, err)
	}
	var artifact LangGraphLifecycleArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return LangGraphLifecycleArtifact{}, fmt.Errorf("decode LangGraph lifecycle artifact %s: %w", path, err)
	}
	if err := artifact.Validate(); err != nil {
		return LangGraphLifecycleArtifact{}, err
	}
	return artifact, nil
}

func (a LangGraphLifecycleArtifact) Validate() error {
	if a.SchemaVersion != LangGraphLifecycleArtifactSchema || strings.TrimSpace(a.ThreadID) == "" {
		return fmt.Errorf("LangGraph lifecycle artifact lacks schema or thread identity")
	}
	if a.ClockDomain != "" && a.ClockDomain != "CLOCK_MONOTONIC" {
		return fmt.Errorf("LangGraph lifecycle artifact clock domain %q is unsupported", a.ClockDomain)
	}
	for index, event := range a.Events {
		if event.Index != index || strings.TrimSpace(event.Event) == "" {
			return fmt.Errorf("LangGraph lifecycle artifact has an invalid event at index %d", index)
		}
	}
	return nil
}

// ToolEffectProvenance returns a causal binding only when one complete shell
// command span, recorded in the same monotonic clock domain, contains every
// linked objective effect. Zero or multiple matching spans are unknown rather
// than a reason to guess an attribution.
func (a LangGraphLifecycleArtifact) ToolEffectProvenance(firstEffect, lastEffect uint64) (*recovery.LangGraphToolEffectProvenance, error) {
	if firstEffect == 0 || lastEffect < firstEffect {
		return nil, fmt.Errorf("LangGraph tool-effect provenance requires a valid effect interval")
	}
	if a.ClockDomain != "CLOCK_MONOTONIC" {
		return nil, nil
	}
	type commandStart struct {
		monotonicNS    uint64
		shellSessionID string
		commandSHA256  string
	}
	starts := make(map[string]commandStart)
	var candidates []recovery.LangGraphToolEffectProvenance
	for _, event := range a.Events {
		switch event.Event {
		case "shell_command_started":
			if event.MonotonicNS == 0 || strings.TrimSpace(event.ToolCallID) == "" || strings.TrimSpace(event.ShellSessionID) == "" || strings.TrimSpace(event.CommandSHA256) == "" {
				continue
			}
			if _, exists := starts[event.ToolCallID]; exists {
				return nil, fmt.Errorf("LangGraph lifecycle repeats shell command start for tool call %q", event.ToolCallID)
			}
			starts[event.ToolCallID] = commandStart{
				monotonicNS:    event.MonotonicNS,
				shellSessionID: event.ShellSessionID,
				commandSHA256:  event.CommandSHA256,
			}
		case "shell_command_finished":
			if event.MonotonicNS == 0 || strings.TrimSpace(event.ToolCallID) == "" {
				continue
			}
			start, exists := starts[event.ToolCallID]
			if !exists || event.MonotonicNS < start.monotonicNS {
				continue
			}
			delete(starts, event.ToolCallID)
			if start.monotonicNS > firstEffect || event.MonotonicNS < lastEffect {
				continue
			}
			candidates = append(candidates, recovery.LangGraphToolEffectProvenance{
				ToolCallID:                 event.ToolCallID,
				ToolName:                   "shell",
				ShellSessionID:             start.shellSessionID,
				CommandSHA256:              start.commandSHA256,
				CommandStartedMonotonicNS:  start.monotonicNS,
				CommandFinishedMonotonicNS: event.MonotonicNS,
				FirstEffectMonotonicNS:     firstEffect,
				LastEffectMonotonicNS:      lastEffect,
			})
		}
	}
	if len(candidates) != 1 {
		return nil, nil
	}
	if err := candidates[0].Validate(); err != nil {
		return nil, err
	}
	return &candidates[0], nil
}
