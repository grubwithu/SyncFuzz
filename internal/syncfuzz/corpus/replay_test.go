package corpus_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/scheduler"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestReplayCorpusEntryReproducesSignature(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := tmp + "/corpus"
	suite := &scheduler.SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []scheduler.SuiteDiscovery{
			{
				Kind:      "new-signature",
				Key:       "<replay, after-external-commit, external-effect, duplicate-create, missing-receipt, forgotten-external-effect>",
				CaseName:  "action-replay",
				Iteration: 1,
				RunID:     "run-test",
				Signature: core.MismatchSignature{
					LifecycleEvent: "replay",
					FaultPhase:     "after-external-commit",
					StateClass:     "external-effect",
					Operation:      "duplicate-create",
					Relation:       "missing-receipt",
					Impact:         "forgotten-external-effect",
				},
				ArtifactDir: "runs/suite-test/run-test",
			},
		},
	}
	entries, err := scheduler.WriteCorpus(corpusDir, suite)
	if err != nil {
		t.Fatalf("scheduler.WriteCorpus failed: %v", err)
	}

	result, err := corpus.ReplayCorpusEntry(context.Background(), corpus.ReplayOptions{
		CorpusDir: corpusDir,
		EntryID:   entries[0].EntryID,
		OutDir:    tmp + "/runs",
	})
	if err != nil {
		t.Fatalf("corpus.ReplayCorpusEntry failed: %v", err)
	}

	if !result.Reproduced {
		t.Fatalf("expected replay to reproduce signature")
	}
	if !result.SignatureMatched {
		t.Fatalf("expected replay signature to match")
	}
	if result.RunArtifactDir == "" {
		t.Fatalf("expected run artifact directory")
	}
	if result.OutcomeCategory != corpus.ReplayOutcomeReproduced {
		t.Fatalf("expected reproduced outcome category, got %#v", result)
	}
}

func TestReplayTargetCorpusEntryPreservesStoredExecutionPlan(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "source-target-run")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create source artifact directory: %v", err)
	}
	signature := target.TargetSignature(target.DefaultTargetTaskID)
	if err := core.WriteJSON(filepath.Join(artifactDir, target.TargetTaskArtifact), target.TargetTask{
		SchemaVersion:  "syncfuzz.target-task.v1",
		RunID:          "source-run",
		AdapterID:      target.DefaultTargetAdapterID,
		TargetID:       "execution-plan-replay-target",
		TaskID:         target.DefaultTargetTaskID,
		Prompt:         "write the process mode witness",
		Command:        `printf '%s' "$SYNCFUZZ_LANGGRAPH_PROCESS_MODE" > late-effect`,
		TimeoutMillis:  int64(time.Second / time.Millisecond),
		ObserveDelayMs: 1,
		Environment:    "local",
		ExpectedFiles:  []string{"late-effect"},
		Scenario: &target.TargetScenarioInfo{
			TaskID: target.DefaultTargetTaskID,
			ExecutionPlan: &target.TargetScenarioExecutionPlan{
				ProcessMode: "mutated-mode",
			},
		},
	}); err != nil {
		t.Fatalf("write source target task: %v", err)
	}
	corpusDir := filepath.Join(tmp, "corpus")
	entry := corpus.CorpusEntry{
		ExecutionKind: corpus.CorpusExecutionTarget,
		EntryID:       "target-execution-plan-replay",
		SuiteID:       "suite-test",
		RunID:         "source-run",
		Kind:          "target-confirmed",
		Signature:     signature,
		AdapterID:     target.DefaultTargetAdapterID,
		TargetID:      "execution-plan-replay-target",
		TaskID:        target.DefaultTargetTaskID,
		ArtifactDir:   artifactDir,
	}
	if err := corpus.AppendCorpusEntries(corpusDir, []corpus.CorpusEntry{entry}); err != nil {
		t.Fatalf("append target corpus entry: %v", err)
	}

	result, err := corpus.ReplayCorpusEntry(context.Background(), corpus.ReplayOptions{
		CorpusDir: corpusDir,
		EntryID:   entry.EntryID,
		OutDir:    filepath.Join(tmp, "replays"),
	})
	if err != nil {
		t.Fatalf("ReplayCorpusEntry failed: %v", err)
	}
	witness, err := os.ReadFile(filepath.Join(result.RunArtifactDir, "workspace", "late-effect"))
	if err != nil {
		t.Fatalf("read replay witness: %v", err)
	}
	if string(witness) != "mutated-mode" {
		t.Fatalf("expected stored execution plan during replay, got %q", witness)
	}
}

func TestClassifyTargetReplayOutcomeExecutionNotReached(t *testing.T) {
	details := corpus.ClassifyTargetReplayOutcome(&target.TargetRunResult{
		Completed: false,
	}, false)
	if details.Category != corpus.ReplayOutcomeExecutionNotReached {
		t.Fatalf("expected execution-not-reached, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeTaskNoncompliant(t *testing.T) {
	details := corpus.ClassifyTargetReplayOutcome(&target.TargetRunResult{
		Completed: true,
		TaskCompliance: target.TargetTaskComplianceResult{
			Status: target.TargetTaskComplianceStatusViolated,
		},
	}, false)
	if details.Category != corpus.ReplayOutcomeTaskNoncompliant {
		t.Fatalf("expected task-noncompliant, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeLifecycleNotTriggered(t *testing.T) {
	details := corpus.ClassifyTargetReplayOutcome(&target.TargetRunResult{
		Completed: true,
		TaskCompliance: target.TargetTaskComplianceResult{
			Status: target.TargetTaskComplianceStatusCompliant,
		},
		TargetOracle: target.TargetOracleResult{
			Status:  target.TargetOracleStatusInconclusive,
			Missing: []string{"langgraph fork summary artifact was present and decodable"},
		},
	}, false)
	if details.Category != corpus.ReplayOutcomeLifecycleNotTriggered {
		t.Fatalf("expected lifecycle-not-triggered, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeStateNotPlanted(t *testing.T) {
	details := corpus.ClassifyTargetReplayOutcome(&target.TargetRunResult{
		Completed: true,
		TaskCompliance: target.TargetTaskComplianceResult{
			Status: target.TargetTaskComplianceStatusCompliant,
		},
		TargetOracle: target.TargetOracleResult{
			Status:  target.TargetOracleStatusInconclusive,
			Missing: []string{"langgraph history captured the initial branch-note.txt creation"},
		},
	}, false)
	if details.Category != corpus.ReplayOutcomeStateNotPlanted {
		t.Fatalf("expected state-not-planted, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeCleanNegative(t *testing.T) {
	details := corpus.ClassifyTargetReplayOutcome(&target.TargetRunResult{
		Completed: true,
		TaskCompliance: target.TargetTaskComplianceResult{
			Status: target.TargetTaskComplianceStatusCompliant,
		},
		TargetOracle: target.TargetOracleResult{
			Status:      target.TargetOracleStatusNegative,
			Attribution: target.TargetOracleAttributionCleanFork,
		},
	}, false)
	if details.Category != corpus.ReplayOutcomeCleanNegative {
		t.Fatalf("expected clean-negative, got %#v", details)
	}
}
