package synthesis

import (
	"strings"
	"testing"
)

func TestLangGraphLifecycleToolEffectProvenanceRequiresOneCompleteSpan(t *testing.T) {
	artifact := LangGraphLifecycleArtifact{
		SchemaVersion: LangGraphLifecycleArtifactSchema,
		ThreadID:      "thread-1",
		ClockDomain:   "CLOCK_MONOTONIC",
		Events: []LangGraphLifecycleEvent{
			{Index: 0, Event: "shell_command_started", MonotonicNS: 100, ToolCallID: "call-1", ShellSessionID: "shell-1", CommandSHA256: strings.Repeat("a", 64)},
			{Index: 1, Event: "shell_command_finished", MonotonicNS: 200, ToolCallID: "call-1"},
		},
	}
	provenance, err := artifact.ToolEffectProvenance(120, 180)
	if err != nil {
		t.Fatalf("ToolEffectProvenance returned error: %v", err)
	}
	if provenance == nil || provenance.ToolCallID != "call-1" || provenance.ToolName != "shell" {
		t.Fatalf("unexpected tool-effect provenance: %#v", provenance)
	}

	legacy := artifact
	legacy.Events[0].MonotonicNS = 0
	legacy.Events[1].MonotonicNS = 0
	provenance, err = legacy.ToolEffectProvenance(120, 180)
	if err != nil || provenance != nil {
		t.Fatalf("legacy lifecycle should leave tool-effect provenance unknown, got %#v, %v", provenance, err)
	}

	ambiguous := LangGraphLifecycleArtifact{
		SchemaVersion: LangGraphLifecycleArtifactSchema,
		ThreadID:      "thread-1",
		ClockDomain:   "CLOCK_MONOTONIC",
		Events: []LangGraphLifecycleEvent{
			{Index: 0, Event: "shell_command_started", MonotonicNS: 100, ToolCallID: "call-1", ShellSessionID: "shell-1", CommandSHA256: strings.Repeat("a", 64)},
			{Index: 1, Event: "shell_command_started", MonotonicNS: 110, ToolCallID: "call-2", ShellSessionID: "shell-1", CommandSHA256: strings.Repeat("b", 64)},
			{Index: 2, Event: "shell_command_finished", MonotonicNS: 200, ToolCallID: "call-1"},
			{Index: 3, Event: "shell_command_finished", MonotonicNS: 210, ToolCallID: "call-2"},
		},
	}
	provenance, err = ambiguous.ToolEffectProvenance(120, 180)
	if err != nil || provenance != nil {
		t.Fatalf("ambiguous lifecycle should leave tool-effect provenance unknown, got %#v, %v", provenance, err)
	}
}
