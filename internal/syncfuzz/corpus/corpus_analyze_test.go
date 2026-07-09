package corpus_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/scheduler"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestAnalyzeCorpusSummarizesTargetEntriesAndVerification(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")

	suite, err := scheduler.RunTargetSuite(context.Background(), scheduler.TargetSuiteOptions{
		TargetID: "local-target",
		Tasks:    []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command: `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p attacker-bin && printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git && chmod +x attacker-bin/git && printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`,
		OutDir:       filepath.Join(tmp, "runs"),
		CorpusDir:    corpusDir,
		Repeat:       1,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("scheduler.RunTargetSuite failed: %v", err)
	}
	if len(suite.CorpusEntries) != 2 {
		t.Fatalf("expected 2 corpus entries, got %d", len(suite.CorpusEntries))
	}

	verifyResult, err := corpus.VerifyCorpus(context.Background(), corpus.VerifyOptions{
		CorpusDir: corpusDir,
		OutDir:    filepath.Join(tmp, "verify"),
	})
	if err != nil {
		t.Fatalf("corpus.VerifyCorpus failed: %v", err)
	}
	verificationPath := filepath.Join(verifyResult.ArtifactDir, "verification-result.json")

	result, err := corpus.AnalyzeCorpus(corpus.CorpusAnalyzeOptions{
		CorpusDir:              corpusDir,
		VerificationResultFile: verificationPath,
	})
	if err != nil {
		t.Fatalf("corpus.AnalyzeCorpus failed: %v", err)
	}

	if result.TotalEntries != 2 {
		t.Fatalf("expected 2 corpus entries, got %#v", result)
	}
	if len(result.ExecutionSummaries) != 1 || result.ExecutionSummaries[0].Key != corpus.CorpusExecutionTarget || result.ExecutionSummaries[0].TotalEntries != 2 {
		t.Fatalf("unexpected execution summaries: %#v", result.ExecutionSummaries)
	}
	if len(result.SubjectSummaries) != 2 {
		t.Fatalf("expected 2 subject summaries, got %#v", result.SubjectSummaries)
	}
	if len(result.TargetOutcomeSummaries) == 0 || len(result.ActivationSummaries) == 0 {
		t.Fatalf("expected target outcome/activation summaries, got %#v", result)
	}
	if result.VerificationID == "" || len(result.VerificationOutcomeSummaries) == 0 || len(result.VerificationSubjectSummaries) != 2 {
		t.Fatalf("expected embedded verification summaries, got %#v", result)
	}
}

func TestAnalyzeCorpusLoadsVerificationFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verification-result.json")
	verification := corpus.VerificationResult{
		VerificationID: "verify-test",
		OutcomeSummaries: []corpus.VerificationOutcomeStats{
			{Category: corpus.ReplayOutcomeReproduced, TotalEntries: 2},
		},
		SubjectSummaries: []corpus.VerificationSubjectStats{
			{ExecutionKind: corpus.CorpusExecutionCase, CaseName: "action-replay", TotalEntries: 2},
		},
	}
	raw, err := json.Marshal(verification)
	if err != nil {
		t.Fatalf("marshal verification result: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write verification result: %v", err)
	}

	result, err := corpus.AnalyzeCorpus(corpus.CorpusAnalyzeOptions{
		CorpusDir:              t.TempDir(),
		VerificationResultFile: path,
	})
	if err != nil {
		t.Fatalf("corpus.AnalyzeCorpus failed: %v", err)
	}
	if result.VerificationID != "verify-test" {
		t.Fatalf("unexpected verification id: %#v", result)
	}
	if len(result.VerificationOutcomeSummaries) != 1 || result.VerificationOutcomeSummaries[0].Category != corpus.ReplayOutcomeReproduced {
		t.Fatalf("unexpected verification outcome summaries: %#v", result.VerificationOutcomeSummaries)
	}
}
