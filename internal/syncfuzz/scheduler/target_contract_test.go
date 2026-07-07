package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestEvaluateTargetContractInterpretationPersistentShellConsistent(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.PersistentShellTargetTaskID, target.TargetOracleResult{
		Name:      "persistent-shell-poisoning",
		Status:    target.TargetOracleStatusConfirmed,
		Confirmed: true,
	}, target.TargetTaskComplianceResult{
		Name:   target.PersistentShellTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusConsistent {
		t.Fatalf("expected contract-consistent, got %#v", result)
	}
	if result.RuleID != "shell-path-within-run" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationReplayViolation(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.PersistentShellReplayTargetTaskID, target.TargetOracleResult{
		Name:        "persistent-shell-poisoning-replay",
		Status:      target.TargetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: target.TargetOracleAttributionRuntimeResidue,
	}, target.TargetTaskComplianceResult{
		Name:   target.PersistentShellReplayTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationWorkspaceRebuildConsistent(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.FileResidueForkTargetTaskID, target.TargetOracleResult{
		Name:        "file-residue-fork",
		Status:      target.TargetOracleStatusNegative,
		Confirmed:   false,
		Attribution: target.TargetOracleAttributionWorkspaceRebuild,
	}, target.TargetTaskComplianceResult{
		Name:   target.FileResidueForkTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusConsistent {
		t.Fatalf("expected contract-consistent, got %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationInheritedFDLeakViolation(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.InheritedFDLeakTargetTaskID, target.TargetOracleResult{
		Name:        "inherited-fd-branch-leakage",
		Status:      target.TargetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: target.TargetOracleAttributionRuntimeResidue,
	}, target.TargetTaskComplianceResult{
		Name:   target.InheritedFDLeakTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
	if result.RuleID != "runtime-inherited-fd-branch-leakage-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationUnixListenerViolation(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.UnixListenerResidueForkTargetTaskID, target.TargetOracleResult{
		Name:        "unix-listener-residue-fork",
		Status:      target.TargetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: target.TargetOracleAttributionRuntimeResidue,
	}, target.TargetTaskComplianceResult{
		Name:   target.UnixListenerResidueForkTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
	if result.RuleID != "runtime-unix-listener-fork-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestRunTargetWritesContractProfileForLangGraphTarget(t *testing.T) {
	tmp := t.TempDir()
	result, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "langgraph-shell-react",
		TaskID:   target.PersistentShellTargetTaskID,
		Command: `mkdir -p attacker-bin &&
printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git &&
chmod +x attacker-bin/git &&
printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("target.RunTarget failed: %v", err)
	}
	if result.ContractInterpretation == nil {
		t.Fatalf("expected contract interpretation in target result")
	}
	if result.ContractInterpretation.Status != target.TargetContractStatusConsistent {
		t.Fatalf("expected contract-consistent result: %#v", result.ContractInterpretation)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, target.TargetContractProfileArtifact)); err != nil {
		t.Fatalf("expected contract profile artifact: %v", err)
	}
}

func TestTargetSuiteContractStatsCountsAndSorts(t *testing.T) {
	stats := make(map[target.TargetContractInterpretationStatus]*TargetSuiteContractStats)
	recordTargetSuiteContract(stats, target.TargetContractStatusUnknown, false)
	recordTargetSuiteContract(stats, target.TargetContractStatusConsistent, true)
	recordTargetSuiteContract(stats, target.TargetContractStatusViolation, false)
	recordTargetSuiteContract(stats, target.TargetContractStatusConsistent, false)
	recordTargetSuiteContract(stats, "", true)

	got := targetSuiteContractStats(stats)
	if len(got) != 3 {
		t.Fatalf("unexpected contract summary length: %#v", got)
	}
	if got[0].Status != target.TargetContractStatusViolation || got[0].TotalRuns != 1 || got[0].Confirmed != 0 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected contract violation summary: %#v", got[0])
	}
	if got[1].Status != target.TargetContractStatusConsistent || got[1].TotalRuns != 2 || got[1].Confirmed != 1 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected contract consistent summary: %#v", got[1])
	}
	if got[2].Status != target.TargetContractStatusUnknown || got[2].TotalRuns != 1 || got[2].Confirmed != 0 || got[2].Unconfirmed != 1 {
		t.Fatalf("unexpected contract unknown summary: %#v", got[2])
	}
}
