package syncfuzz

import (
	"context"
	"testing"
)

func TestReplayCorpusEntryReproducesSignature(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := tmp + "/corpus"
	suite := &SuiteResult{
		SuiteID: "suite-test",
		Discoveries: []SuiteDiscovery{
			{
				Kind:      "new-signature",
				Key:       "<replay, after-external-commit, external-effect, duplicate-create, missing-receipt, forgotten-external-effect>",
				CaseName:  "action-replay",
				Iteration: 1,
				RunID:     "run-test",
				Signature: MismatchSignature{
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
	entries, err := WriteCorpus(corpusDir, suite)
	if err != nil {
		t.Fatalf("WriteCorpus failed: %v", err)
	}

	result, err := ReplayCorpusEntry(context.Background(), ReplayOptions{
		CorpusDir: corpusDir,
		EntryID:   entries[0].EntryID,
		OutDir:    tmp + "/runs",
	})
	if err != nil {
		t.Fatalf("ReplayCorpusEntry failed: %v", err)
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
	if result.OutcomeCategory != replayOutcomeReproduced {
		t.Fatalf("expected reproduced outcome category, got %#v", result)
	}
}

func TestClassifyTargetReplayOutcomeExecutionNotReached(t *testing.T) {
	details := classifyTargetReplayOutcome(&TargetRunResult{
		Completed: false,
	}, false)
	if details.Category != replayOutcomeExecutionNotReached {
		t.Fatalf("expected execution-not-reached, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeTaskNoncompliant(t *testing.T) {
	details := classifyTargetReplayOutcome(&TargetRunResult{
		Completed: true,
		TaskCompliance: TargetTaskComplianceResult{
			Status: targetTaskComplianceStatusViolated,
		},
	}, false)
	if details.Category != replayOutcomeTaskNoncompliant {
		t.Fatalf("expected task-noncompliant, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeLifecycleNotTriggered(t *testing.T) {
	details := classifyTargetReplayOutcome(&TargetRunResult{
		Completed: true,
		TaskCompliance: TargetTaskComplianceResult{
			Status: targetTaskComplianceStatusCompliant,
		},
		TargetOracle: TargetOracleResult{
			Status:  targetOracleStatusInconclusive,
			Missing: []string{"langgraph fork summary artifact was present and decodable"},
		},
	}, false)
	if details.Category != replayOutcomeLifecycleNotTriggered {
		t.Fatalf("expected lifecycle-not-triggered, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeStateNotPlanted(t *testing.T) {
	details := classifyTargetReplayOutcome(&TargetRunResult{
		Completed: true,
		TaskCompliance: TargetTaskComplianceResult{
			Status: targetTaskComplianceStatusCompliant,
		},
		TargetOracle: TargetOracleResult{
			Status:  targetOracleStatusInconclusive,
			Missing: []string{"langgraph history captured the initial branch-note.txt creation"},
		},
	}, false)
	if details.Category != replayOutcomeStateNotPlanted {
		t.Fatalf("expected state-not-planted, got %#v", details)
	}
}

func TestClassifyTargetReplayOutcomeCleanNegative(t *testing.T) {
	details := classifyTargetReplayOutcome(&TargetRunResult{
		Completed: true,
		TaskCompliance: TargetTaskComplianceResult{
			Status: targetTaskComplianceStatusCompliant,
		},
		TargetOracle: TargetOracleResult{
			Status:      targetOracleStatusNegative,
			Attribution: targetOracleAttributionCleanFork,
		},
	}, false)
	if details.Category != replayOutcomeCleanNegative {
		t.Fatalf("expected clean-negative, got %#v", details)
	}
}
