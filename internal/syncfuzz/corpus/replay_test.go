package corpus_test

import (
	"context"
	"testing"

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
