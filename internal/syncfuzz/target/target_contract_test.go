package target_test

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

func TestTargetContractProfileIncludesExecutionContextResidueRules(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")

	cwdRule, ok := target.TargetContractRuleFor(profile, target.CWDResidueForkTargetTaskID)
	if !ok {
		t.Fatalf("expected cwd residue contract rule")
	}
	if cwdRule.RuleID != "shell-cwd-fork-boundary" || cwdRule.StateSurface != "shell-session.cwd" {
		t.Fatalf("unexpected cwd residue contract rule: %#v", cwdRule)
	}

	umaskRule, ok := target.TargetContractRuleFor(profile, target.UmaskResidueForkTargetTaskID)
	if !ok {
		t.Fatalf("expected umask residue contract rule")
	}
	if umaskRule.RuleID != "shell-umask-fork-boundary" || umaskRule.StateSurface != "shell-session.umask" {
		t.Fatalf("unexpected umask residue contract rule: %#v", umaskRule)
	}

	trustedClientRule, ok := target.TargetContractRuleFor(profile, target.DiscardedServerTrustedClientTargetTaskID)
	if !ok {
		t.Fatalf("expected trusted-client contract rule")
	}
	if trustedClientRule.RuleID != "communication-trusted-client-fork-boundary" || trustedClientRule.StateSurface != "communication.trusted-client-output" {
		t.Fatalf("unexpected trusted-client contract rule: %#v", trustedClientRule)
	}

	responseCacheRule, ok := target.TargetContractRuleFor(profile, target.SocketResponsePoisoningTargetTaskID)
	if !ok {
		t.Fatalf("expected response-cache contract rule")
	}
	if responseCacheRule.RuleID != "communication-response-cache-fork-boundary" || responseCacheRule.StateSurface != "communication.response-cache" {
		t.Fatalf("unexpected response-cache contract rule: %#v", responseCacheRule)
	}
}

func TestEvaluateTargetContractInterpretationCWDResidueViolation(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.CWDResidueForkTargetTaskID, target.TargetOracleResult{
		Name:        "cwd-residue-fork",
		Status:      target.TargetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: target.TargetOracleAttributionRuntimeResidue,
	}, target.TargetTaskComplianceResult{
		Name:   target.CWDResidueForkTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusViolation {
		t.Fatalf("expected contract-violation, got %#v", result)
	}
	if result.RuleID != "shell-cwd-fork-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationUmaskCleanForkConsistent(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.UmaskResidueForkTargetTaskID, target.TargetOracleResult{
		Name:        "umask-residue-fork",
		Status:      target.TargetOracleStatusNegative,
		Confirmed:   false,
		Attribution: target.TargetOracleAttributionCleanFork,
	}, target.TargetTaskComplianceResult{
		Name:   target.UmaskResidueForkTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusConsistent {
		t.Fatalf("expected contract-consistent, got %#v", result)
	}
	if result.RuleID != "shell-umask-fork-boundary" {
		t.Fatalf("unexpected rule id: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationTrustedClientViolation(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.DiscardedServerTrustedClientTargetTaskID, target.TargetOracleResult{
		Name:        "discarded-server-trusted-client",
		Status:      target.TargetOracleStatusConfirmed,
		Confirmed:   true,
		Attribution: target.TargetOracleAttributionRuntimeResidue,
	}, target.TargetTaskComplianceResult{
		Name:   target.DiscardedServerTrustedClientTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusViolation || result.RuleID != "communication-trusted-client-fork-boundary" {
		t.Fatalf("unexpected trusted-client interpretation: %#v", result)
	}
}

func TestEvaluateTargetContractInterpretationSocketResponsePoisoningConsistent(t *testing.T) {
	profile := target.TargetContractProfileFor("langgraph-shell-react")
	result := target.EvaluateTargetContractInterpretation(profile, target.SocketResponsePoisoningTargetTaskID, target.TargetOracleResult{
		Name:        "socket-response-poisoning",
		Status:      target.TargetOracleStatusNegative,
		Confirmed:   false,
		Attribution: target.TargetOracleAttributionCleanFork,
	}, target.TargetTaskComplianceResult{
		Name:   target.SocketResponsePoisoningTargetTaskID,
		Status: target.TargetTaskComplianceStatusCompliant,
	})
	if result == nil {
		t.Fatalf("expected contract interpretation")
	}
	if result.Status != target.TargetContractStatusConsistent || result.RuleID != "communication-response-cache-fork-boundary" {
		t.Fatalf("unexpected response-cache interpretation: %#v", result)
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
