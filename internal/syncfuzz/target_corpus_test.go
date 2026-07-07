package syncfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRunTargetSuiteWritesTargetCorpus(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")

	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		TargetID:     "local-target",
		Tasks:        []string{defaultTargetTaskID, persistentShellTargetTaskID},
		Command:      `case "$SYNCFUZZ_TASK_ID" in orphan-process) printf ok > late-effect ;; persistent-shell-poisoning) mkdir -p attacker-bin && printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git && chmod +x attacker-bin/git && printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt ;; *) exit 9 ;; esac`,
		OutDir:       filepath.Join(tmp, "runs"),
		CorpusDir:    corpusDir,
		Repeat:       1,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}

	if len(result.CorpusEntries) != 2 {
		t.Fatalf("expected 2 target corpus entries, got %d", len(result.CorpusEntries))
	}

	entries, err := ListCorpus(corpusDir, 0)
	if err != nil {
		t.Fatalf("ListCorpus failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 listed entries, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.EffectiveExecutionKind() != corpusExecutionTarget {
			t.Fatalf("expected target execution kind, got %#v", entry)
		}
		if entry.TargetID != "local-target" || entry.TaskID == "" {
			t.Fatalf("expected target metadata, got %#v", entry)
		}
		if entry.TargetOracleStatus == "" {
			t.Fatalf("expected target oracle metadata, got %#v", entry)
		}
		if entry.TaskComplianceStatus == "" {
			t.Fatalf("expected target compliance metadata, got %#v", entry)
		}
	}
}

func TestReplayCorpusEntryReproducesTargetSignature(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")

	suite, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		TargetID:     "local-target",
		Tasks:        []string{persistentShellTargetTaskID},
		Command:      `mkdir -p attacker-bin && printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git && chmod +x attacker-bin/git && printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt`,
		OutDir:       filepath.Join(tmp, "runs"),
		CorpusDir:    corpusDir,
		Repeat:       1,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if len(suite.CorpusEntries) != 1 {
		t.Fatalf("expected 1 corpus entry, got %d", len(suite.CorpusEntries))
	}

	result, err := ReplayCorpusEntry(context.Background(), ReplayOptions{
		CorpusDir: corpusDir,
		EntryID:   suite.CorpusEntries[0].EntryID,
		OutDir:    filepath.Join(tmp, "replays"),
	})
	if err != nil {
		t.Fatalf("ReplayCorpusEntry failed: %v", err)
	}

	if result.ExecutionKind != corpusExecutionTarget {
		t.Fatalf("expected target replay, got %#v", result)
	}
	if !result.Reproduced || !result.SignatureMatched || !result.Confirmed {
		t.Fatalf("expected target replay to reproduce, got %#v", result)
	}
	if result.OutcomeCategory != replayOutcomeReproduced {
		t.Fatalf("expected reproduced target replay outcome, got %#v", result)
	}
	if result.TargetID != "local-target" || result.TaskID != persistentShellTargetTaskID {
		t.Fatalf("expected target metadata to round-trip, got %#v", result)
	}
}

func TestVerifyCorpusSupportsTargetEntries(t *testing.T) {
	tmp := t.TempDir()
	corpusDir := filepath.Join(tmp, "corpus")

	suite, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		TargetID:     "local-target",
		Tasks:        []string{defaultTargetTaskID, persistentShellTargetTaskID},
		Command:      `case "$SYNCFUZZ_TASK_ID" in orphan-process) printf ok > late-effect ;; persistent-shell-poisoning) mkdir -p attacker-bin && printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git && chmod +x attacker-bin/git && printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt ;; *) exit 9 ;; esac`,
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

	result, err := VerifyCorpus(context.Background(), VerifyOptions{
		CorpusDir: corpusDir,
		OutDir:    filepath.Join(tmp, "verify"),
	})
	if err != nil {
		t.Fatalf("VerifyCorpus failed: %v", err)
	}

	if result.TotalEntries != 2 || result.Reproduced != 2 {
		t.Fatalf("expected all target corpus entries to reproduce, got %#v", result)
	}
	if len(result.SubjectSummaries) != 2 {
		t.Fatalf("expected per-task verification summaries, got %#v", result.SubjectSummaries)
	}
	for _, entry := range result.Entries {
		if entry.ExecutionKind != corpusExecutionTarget {
			t.Fatalf("expected target verification entry, got %#v", entry)
		}
	}
}
