package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultTargetPromptWithProfileWrapsBaselinePrompt(t *testing.T) {
	baseline := defaultTargetPrompt(defaultTargetTaskID)
	workflow := defaultTargetPromptWithProfile(defaultTargetTaskID, targetPromptProfileWorkflowID)
	audit := defaultTargetPromptWithProfile(defaultTargetTaskID, targetPromptProfileAuditID)

	if baseline == workflow || baseline == audit {
		t.Fatalf("expected alternate prompt profiles to change prompt text")
	}
	if !strings.Contains(workflow, "routine workspace continuity and handoff check") {
		t.Fatalf("expected workflow prompt wrapper: %q", workflow)
	}
	if !strings.Contains(workflow, "treat any required shell/tool call counts as mandatory") {
		t.Fatalf("expected workflow prompt wrapper to reinforce required tool use: %q", workflow)
	}
	if !strings.Contains(audit, "deterministic reproducibility audit") {
		t.Fatalf("expected audit prompt wrapper: %q", audit)
	}
	if !strings.Contains(workflow, "creates a file named late-effect") || !strings.Contains(audit, "creates a file named late-effect") {
		t.Fatalf("expected prompt profiles to preserve the base task semantics")
	}
}

func TestBuildTargetScheduleMatrixExpandsPromptProfiles(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:         "langgraph-shell-react",
		Tasks:            []string{persistentShellTargetTaskID},
		PromptProfileIDs: []string{targetPromptProfileBaselineID, targetPromptProfileWorkflowID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 2 {
		t.Fatalf("expected prompt profiles to expand candidate count: %#v", matrix)
	}
	if len(matrix.PromptProfiles) != 2 || matrix.PromptProfiles[0] != targetPromptProfileBaselineID || matrix.PromptProfiles[1] != targetPromptProfileWorkflowID {
		t.Fatalf("unexpected prompt profile list: %#v", matrix.PromptProfiles)
	}
	if matrix.Candidates[0].CandidateID != "langgraph-shell-react/persistent-shell-poisoning" {
		t.Fatalf("expected baseline candidate id to remain stable: %#v", matrix.Candidates[0])
	}
	if matrix.Candidates[1].CandidateID != "langgraph-shell-react/persistent-shell-poisoning/workflow" {
		t.Fatalf("expected workflow candidate id to include prompt profile: %#v", matrix.Candidates[1])
	}
	if matrix.Candidates[1].PromptProfileID != targetPromptProfileWorkflowID {
		t.Fatalf("expected workflow prompt profile metadata: %#v", matrix.Candidates[1])
	}
}

func TestRunTargetRecordsBuiltInPromptProfile(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:          filepath.Join(tmp, "runs"),
		TargetID:        "profile-smoke",
		TaskID:          defaultTargetTaskID,
		PromptProfileID: targetPromptProfileWorkflowID,
		Command: `grep -q 'routine workspace continuity and handoff check' "$SYNCFUZZ_PROMPT_FILE" &&
printf ok > late-effect`,
		Timeout:      5 * time.Second,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if result.PromptProfileID != targetPromptProfileWorkflowID {
		t.Fatalf("expected recorded prompt profile id, got %#v", result)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, targetTaskArtifact))
	if err != nil {
		t.Fatalf("read target task artifact: %v", err)
	}
	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode target task artifact: %v", err)
	}
	if task.PromptProfileID != targetPromptProfileWorkflowID {
		t.Fatalf("expected prompt profile id in target task artifact: %#v", task)
	}
}

func TestWorkflowProfilePreservesExplicitShellRequirementForLongDelayTask(t *testing.T) {
	workflow := defaultTargetPromptWithProfile(longDelayTargetTaskID, targetPromptProfileWorkflowID)
	if !strings.Contains(workflow, "use exactly one shell tool call") {
		t.Fatalf("expected workflow profile to preserve explicit shell requirement: %q", workflow)
	}
	if !strings.Contains(workflow, "A prose-only answer counts as failure") {
		t.Fatalf("expected long delay workflow prompt to reject prose-only completion: %q", workflow)
	}
}
