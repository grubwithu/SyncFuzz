package syncfuzz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluateTargetContractInterpretationPersistentShellConsistent(t *testing.T) {
	profile := targetContractProfile("langgraph-shell-react")
	result := evaluateTargetContractInterpretation(profile, persistentShellTargetTaskID, TargetOracleResult{
		Name:      "persistent-shell-poisoning",
		Status:    targetOracleStatusConfirmed,
		Confirmed: true,
	}, TargetTaskComplianceResult{
		Name:   persistentShellTargetTaskID,
		Status: targetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != targetContractStatusConsistent {
		t.Fatalf("expected contract-consistent, got %#v", result)
	}
	if result.RuleID != "shell-path-within-run" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationReplayViolation(t *testing.T) {
	profile := targetContractProfile("langgraph-shell-react")
	result := evaluateTargetContractInterpretation(profile, persistentShellReplayTargetTaskID, TargetOracleResult{
		Name:        "persistent-shell-poisoning-replay",
		Status:      targetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: targetOracleAttributionRuntimeResidue,
	}, TargetTaskComplianceResult{
		Name:   persistentShellReplayTargetTaskID,
		Status: targetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != targetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationWorkspaceRebuildConsistent(t *testing.T) {
	profile := targetContractProfile("langgraph-shell-react")
	result := evaluateTargetContractInterpretation(profile, fileResidueForkTargetTaskID, TargetOracleResult{
		Name:        "file-residue-fork",
		Status:      targetOracleStatusNegative,
		Confirmed:   false,
		Attribution: targetOracleAttributionWorkspaceRebuild,
	}, TargetTaskComplianceResult{
		Name:   fileResidueForkTargetTaskID,
		Status: targetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != targetContractStatusConsistent {
		t.Fatalf("expected contract-consistent, got %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationInheritedFDLeakViolation(t *testing.T) {
	profile := targetContractProfile("langgraph-shell-react")
	result := evaluateTargetContractInterpretation(profile, inheritedFDLeakTargetTaskID, TargetOracleResult{
		Name:        "inherited-fd-branch-leakage",
		Status:      targetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: targetOracleAttributionRuntimeResidue,
	}, TargetTaskComplianceResult{
		Name:   inheritedFDLeakTargetTaskID,
		Status: targetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != targetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
	if result.RuleID != "runtime-inherited-fd-branch-leakage-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationUnixListenerViolation(t *testing.T) {
	profile := targetContractProfile("langgraph-shell-react")
	result := evaluateTargetContractInterpretation(profile, unixListenerResidueForkTargetTaskID, TargetOracleResult{
		Name:        "unix-listener-residue-fork",
		Status:      targetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: targetOracleAttributionRuntimeResidue,
	}, TargetTaskComplianceResult{
		Name:   unixListenerResidueForkTargetTaskID,
		Status: targetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != targetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
	if result.RuleID != "runtime-unix-listener-fork-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestRunTargetWritesContractProfileForLangGraphTarget(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:   filepath.Join(tmp, "runs"),
		TargetID: "langgraph-shell-react",
		TaskID:   persistentShellTargetTaskID,
		Command: `mkdir -p attacker-bin &&
printf '#!/bin/sh\nprintf ATTACKER_GIT\n' > attacker-bin/git &&
chmod +x attacker-bin/git &&
printf '%s\n' "$PWD/attacker-bin/git" > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if result.ContractInterpretation == nil {
		t.Fatalf("expected contract interpretation in target result")
	}
	if result.ContractInterpretation.Status != targetContractStatusConsistent {
		t.Fatalf("expected contract-consistent result: %#v", result.ContractInterpretation)
	}
	if _, err := os.Stat(filepath.Join(result.ArtifactDir, targetContractProfileArtifact)); err != nil {
		t.Fatalf("expected contract profile artifact: %v", err)
	}
}

func TestTargetSuiteContractStatsCountsAndSorts(t *testing.T) {
	stats := make(map[TargetContractInterpretationStatus]*TargetSuiteContractStats)
	recordTargetSuiteContract(stats, targetContractStatusUnknown, false)
	recordTargetSuiteContract(stats, targetContractStatusConsistent, true)
	recordTargetSuiteContract(stats, targetContractStatusViolation, false)
	recordTargetSuiteContract(stats, targetContractStatusConsistent, false)
	recordTargetSuiteContract(stats, "", true)

	got := targetSuiteContractStats(stats)
	if len(got) != 3 {
		t.Fatalf("unexpected contract summary length: %#v", got)
	}
	if got[0].Status != targetContractStatusViolation || got[0].TotalRuns != 1 || got[0].Confirmed != 0 || got[0].Unconfirmed != 1 {
		t.Fatalf("unexpected contract violation summary: %#v", got[0])
	}
	if got[1].Status != targetContractStatusConsistent || got[1].TotalRuns != 2 || got[1].Confirmed != 1 || got[1].Unconfirmed != 1 {
		t.Fatalf("unexpected contract consistent summary: %#v", got[1])
	}
	if got[2].Status != targetContractStatusUnknown || got[2].TotalRuns != 1 || got[2].Confirmed != 0 || got[2].Unconfirmed != 1 {
		t.Fatalf("unexpected contract unknown summary: %#v", got[2])
	}
}
