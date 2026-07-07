package target_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/scheduler"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestDefaultTargetPromptWithProfileWrapsBaselinePrompt(t *testing.T) {
	baseline := target.DefaultTargetPrompt(target.DefaultTargetTaskID)
	workflow := target.DefaultTargetPromptWithProfile(target.DefaultTargetTaskID, target.TargetPromptProfileWorkflowID)
	audit := target.DefaultTargetPromptWithProfile(target.DefaultTargetTaskID, target.TargetPromptProfileAuditID)

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
	matrix, err := scheduler.BuildTargetScheduleMatrix(scheduler.TargetMatrixOptions{
		TargetID:         "langgraph-shell-react",
		Tasks:            []string{target.PersistentShellTargetTaskID},
		PromptProfileIDs: []string{target.TargetPromptProfileBaselineID, target.TargetPromptProfileWorkflowID},
	})
	if err != nil {
		t.Fatalf("scheduler.BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 2 {
		t.Fatalf("expected prompt profiles to expand candidate count: %#v", matrix)
	}
	if len(matrix.PromptProfiles) != 2 || matrix.PromptProfiles[0] != target.TargetPromptProfileBaselineID || matrix.PromptProfiles[1] != target.TargetPromptProfileWorkflowID {
		t.Fatalf("unexpected prompt profile list: %#v", matrix.PromptProfiles)
	}
	if matrix.Candidates[0].CandidateID != "langgraph-shell-react/persistent-shell-poisoning" {
		t.Fatalf("expected baseline candidate id to remain stable: %#v", matrix.Candidates[0])
	}
	if matrix.Candidates[1].CandidateID != "langgraph-shell-react/persistent-shell-poisoning/workflow" {
		t.Fatalf("expected workflow candidate id to include prompt profile: %#v", matrix.Candidates[1])
	}
	if matrix.Candidates[1].PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected workflow prompt profile metadata: %#v", matrix.Candidates[1])
	}
}

func TestRunTargetRecordsBuiltInPromptProfile(t *testing.T) {
	tmp := t.TempDir()
	result, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:          filepath.Join(tmp, "runs"),
		TargetID:        "profile-smoke",
		TaskID:          target.DefaultTargetTaskID,
		PromptProfileID: target.TargetPromptProfileWorkflowID,
		Command: `grep -q 'routine workspace continuity and handoff check' "$SYNCFUZZ_PROMPT_FILE" &&
printf ok > late-effect`,
		Timeout:      5 * time.Second,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("target.RunTarget failed: %v", err)
	}
	if result.PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected recorded prompt profile id, got %#v", result)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, target.TargetTaskArtifact))
	if err != nil {
		t.Fatalf("read target task artifact: %v", err)
	}
	var task target.TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode target task artifact: %v", err)
	}
	if task.PromptProfileID != target.TargetPromptProfileWorkflowID {
		t.Fatalf("expected prompt profile id in target task artifact: %#v", task)
	}
}

func TestWorkflowProfilePreservesExplicitShellRequirementForLongDelayTask(t *testing.T) {
	workflow := target.DefaultTargetPromptWithProfile(target.LongDelayTargetTaskID, target.TargetPromptProfileWorkflowID)
	if !strings.Contains(workflow, "use exactly one shell tool call") {
		t.Fatalf("expected workflow profile to preserve explicit shell requirement: %q", workflow)
	}
	if !strings.Contains(workflow, "A prose-only answer counts as failure") {
		t.Fatalf("expected long delay workflow prompt to reject prose-only completion: %q", workflow)
	}
}
