package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyzeCorpusSummarizesTargetEntriesAndVerification(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")

	suite, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		TargetID: "local-target",
		Tasks:    []string{defaultTargetTaskID, persistentShellTargetTaskID},
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
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if len(suite.CorpusEntries) != 2 {
		t.Fatalf("expected 2 corpus entries, got %d", len(suite.CorpusEntries))
	}

	verifyResult, err := VerifyCorpus(context.Background(), VerifyOptions{
		CorpusDir: corpusDir,
		OutDir:    filepath.Join(tmp, "verify"),
	})
	if err != nil {
		t.Fatalf("VerifyCorpus failed: %v", err)
	}
	verificationPath := filepath.Join(verifyResult.ArtifactDir, "verification-result.json")

	result, err := AnalyzeCorpus(CorpusAnalyzeOptions{
		CorpusDir:              corpusDir,
		VerificationResultFile: verificationPath,
	})
	if err != nil {
		t.Fatalf("AnalyzeCorpus failed: %v", err)
	}

	if result.TotalEntries != 2 {
		t.Fatalf("expected 2 corpus entries, got %#v", result)
	}
	if len(result.ExecutionSummaries) != 1 || result.ExecutionSummaries[0].Key != corpusExecutionTarget || result.ExecutionSummaries[0].TotalEntries != 2 {
		t.Fatalf("unexpected execution summaries: %#v", result.ExecutionSummaries)
	}
	if len(result.SubjectSummaries) != 2 {
		t.Fatalf("expected 2 subject summaries, got %#v", result.SubjectSummaries)
	}
	if result.VerificationID == "" || len(result.VerificationOutcomeSummaries) == 0 || len(result.VerificationSubjectSummaries) != 2 {
		t.Fatalf("expected embedded verification summaries, got %#v", result)
	}
}

func TestAnalyzeCorpusLoadsVerificationFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "verification-result.json")
	verification := VerificationResult{
		VerificationID: "verify-test",
		OutcomeSummaries: []VerificationOutcomeStats{
			{Category: replayOutcomeReproduced, TotalEntries: 2},
		},
		SubjectSummaries: []VerificationSubjectStats{
			{ExecutionKind: corpusExecutionCase, CaseName: "action-replay", TotalEntries: 2},
		},
	}
	raw, err := json.Marshal(verification)
	if err != nil {
		t.Fatalf("marshal verification result: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write verification result: %v", err)
	}

	result, err := AnalyzeCorpus(CorpusAnalyzeOptions{
		CorpusDir:              t.TempDir(),
		VerificationResultFile: path,
	})
	if err != nil {
		t.Fatalf("AnalyzeCorpus failed: %v", err)
	}
	if result.VerificationID != "verify-test" {
		t.Fatalf("unexpected verification id: %#v", result)
	}
	if len(result.VerificationOutcomeSummaries) != 1 || result.VerificationOutcomeSummaries[0].Category != replayOutcomeReproduced {
		t.Fatalf("unexpected verification outcome summaries: %#v", result.VerificationOutcomeSummaries)
	}
}
