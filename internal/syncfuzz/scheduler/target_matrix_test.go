package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func TestBuildTargetScheduleMatrixExpandsGroupsAndContracts(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "langgraph-shell-react",
		TaskGroups: []string{"shell-lifecycle"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.SchemaVersion != "syncfuzz.target-schedule-matrix.v1" {
		t.Fatalf("unexpected schema version %q", matrix.SchemaVersion)
	}
	if matrix.TargetID != "langgraph-shell-react" {
		t.Fatalf("unexpected target id %q", matrix.TargetID)
	}
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 17 {
		t.Fatalf("unexpected target matrix size: %#v", matrix)
	}
	replay, err := findTargetMatrixCandidate(matrix, target.PersistentShellReplayTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if replay.ContractRuleID != "shell-path-replay-boundary" || replay.ContractExpectation != target.TargetContractExpectationReset {
		t.Fatalf("unexpected replay candidate contract metadata: %#v", replay)
	}
	if replay.SeedID != "shell-path-residue" || replay.PlantPrimitiveID != "shell-path-prepend" {
		t.Fatalf("unexpected replay scenario seed metadata: %#v", replay)
	}
	if replay.ScenarioSchemaVersion != target.TargetScenarioSchemaVersion {
		t.Fatalf("expected frozen Scenario IR schema metadata: %#v", replay)
	}
	if replay.LifecycleOperationID != "checkpoint-replay" || replay.ActivationKindID != "git-resolution" || replay.OracleKindID != "replay-path-residue" {
		t.Fatalf("unexpected replay scenario execution metadata: %#v", replay)
	}
	if replay.Objective == "" {
		t.Fatalf("expected replay objective metadata: %#v", replay)
	}
	if replay.ExecutionPlan == nil || replay.ExecutionPlan.CheckpointSelector != "before-path-export" || !replay.ExecutionPlan.Replay {
		t.Fatalf("expected replay execution plan metadata: %#v", replay)
	}
	if len(replay.Components) < 4 {
		t.Fatalf("expected replay structured components: %#v", replay)
	}
	if replay.MutationFocusID != "lifecycle-splice.checkpoint-replay" || replay.MutationFocusKind != target.TargetScenarioMutationLifecycleSplice {
		t.Fatalf("unexpected replay mutation focus: %#v", replay)
	}
	if len(replay.Mutations) == 0 || replay.Mutations[0].Kind != target.TargetScenarioMutationLifecycleSplice {
		t.Fatalf("expected replay candidate mutations: %#v", replay)
	}
}

func TestBuildTargetScheduleMatrixExpandsSeeds(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		SeedIDs:  []string{"shell-path-residue"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.SeedIDs) != 1 || matrix.SeedIDs[0] != "shell-path-residue" {
		t.Fatalf("unexpected seed ids: %#v", matrix)
	}
	if len(matrix.Tasks) != 3 || matrix.TotalCandidates != 17 {
		t.Fatalf("unexpected seed-expanded matrix size: %#v", matrix)
	}
}

func TestBuildTargetScheduleMatrixAddsMutationFocusDerivedCandidates(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.PersistentShellReplayTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 6 {
		t.Fatalf("expected replay base, prompt/process-mode variants, and generated replay substitutions, got %#v", matrix)
	}
	want := map[string]struct {
		variant   string
		generated bool
	}{
		"langgraph-shell-react/persistent-shell-poisoning-replay":                            {variant: target.TargetPromptVariantBaseID},
		"langgraph-shell-react/persistent-shell-poisoning-replay/lifecycle-boundary":         {variant: target.TargetPromptVariantLifecycleBoundaryID, generated: true},
		"langgraph-shell-react/persistent-shell-poisoning-replay/mutation-focus":             {variant: target.TargetPromptVariantMutationFocusID, generated: true},
		"langgraph-shell-react/persistent-shell-poisoning-replay/phase-shift-single-process": {variant: target.TargetPromptVariantBaseID, generated: true},
		"langgraph-shell-react/persistent-shell-poisoning-replay/primitive-shell-env-export": {variant: target.TargetPromptVariantBaseID, generated: true},
		"langgraph-shell-react/persistent-shell-poisoning-replay/primitive-shell-function-define": {
			variant:   target.TargetPromptVariantBaseID,
			generated: true,
		},
	}
	for _, candidate := range matrix.Candidates {
		expected, ok := want[candidate.CandidateID]
		if !ok {
			t.Fatalf("unexpected candidate id: %#v", candidate)
		}
		if candidate.PromptVariantID != expected.variant {
			t.Fatalf("unexpected prompt variant for %q: %#v", candidate.CandidateID, candidate)
		}
		if candidate.Generated != expected.generated {
			t.Fatalf("unexpected generated flag: %#v", candidate)
		}
		if strings.HasSuffix(candidate.CandidateID, "/phase-shift-single-process") {
			if candidate.ExecutionPlan == nil || candidate.ExecutionPlan.ProcessMode != "single" {
				t.Fatalf("expected executable single-process phase shift: %#v", candidate)
			}
			if candidate.MutationFocusKind != target.TargetScenarioMutationPhaseShift || candidate.MutationFocusID != "phase-shift.process-mode.single-process" {
				t.Fatalf("expected phase-shift mutation provenance: %#v", candidate)
			}
			scenario := targetScenarioForCandidate(candidate)
			if scenario == nil || scenario.ExecutionPlan == nil || scenario.ExecutionPlan.ProcessMode != "single" {
				t.Fatalf("expected generated candidate to produce executable scenario IR: %#v", scenario)
			}
			if len(scenario.Mutations) == 0 || scenario.Mutations[len(scenario.Mutations)-1].MutationID != "phase-shift.process-mode.single-process" {
				t.Fatalf("expected generated scenario IR to retain mutation provenance: %#v", scenario)
			}
		}
	}
}

func TestBuildTargetScheduleMatrixAddsActivationFocusDerivedCandidates(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.DiscardedServerTrustedClientTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 5 {
		t.Fatalf("expected prompt variants plus process-mode phase shift, got %#v", matrix)
	}
	want := map[string]string{
		"langgraph-shell-react/discarded-server-trusted-client":                            target.TargetPromptVariantBaseID,
		"langgraph-shell-react/discarded-server-trusted-client/lifecycle-boundary":         target.TargetPromptVariantLifecycleBoundaryID,
		"langgraph-shell-react/discarded-server-trusted-client/mutation-focus":             target.TargetPromptVariantMutationFocusID,
		"langgraph-shell-react/discarded-server-trusted-client/activation-focus":           target.TargetPromptVariantActivationFocusID,
		"langgraph-shell-react/discarded-server-trusted-client/phase-shift-single-process": target.TargetPromptVariantBaseID,
	}
	for _, candidate := range matrix.Candidates {
		wantVariant, ok := want[candidate.CandidateID]
		if !ok {
			t.Fatalf("unexpected candidate id: %#v", candidate)
		}
		if candidate.PromptVariantID != wantVariant {
			t.Fatalf("unexpected prompt variant for %q: %#v", candidate.CandidateID, candidate)
		}
		if wantVariant == target.TargetPromptVariantActivationFocusID {
			if !candidate.Generated {
				t.Fatalf("activation focus candidate should be generated: %#v", candidate)
			}
			if candidate.ActivationKindID != "trusted-client-consume" {
				t.Fatalf("expected activation metadata to be preserved: %#v", candidate)
			}
		}
	}
}

func TestBuildTargetScheduleMatrixAddsExecutablePrimitiveSubstitutionFamily(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.PersistentShellForkTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	var generated *TargetScheduleCandidate
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		if candidate.ScenarioID == target.GeneratedEnvForkPrimitiveSubstitutionScenarioID {
			generated = candidate
			break
		}
	}
	if generated == nil {
		t.Fatalf("expected generated env fork primitive substitution: %#v", matrix.Candidates)
	}
	if !generated.Generated || generated.PlantPrimitiveID != "shell-env-export" || generated.MutationFocusKind != target.TargetScenarioMutationPrimitiveSubstitution {
		t.Fatalf("unexpected primitive substitution metadata: %#v", generated)
	}
	if generated.ExecutionPlan == nil || generated.ExecutionPlan.CheckpointSelector != "before-env-export" || generated.ExecutionPlan.ProcessMode != "split-process" {
		t.Fatalf("expected executable generated lifecycle plan: %#v", generated)
	}
	if !strings.Contains(generated.Prompt, "SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER") {
		t.Fatalf("expected generated plant prompt: %#v", generated)
	}
	if generated.OracleKindID != "env-residue" || generated.ContractRuleID != "shell-env-generated-fork-boundary" {
		t.Fatalf("expected generated oracle and contract bindings: %#v", generated)
	}
	scenario := targetScenarioForCandidate(*generated)
	if scenario == nil || scenario.ScenarioID != target.GeneratedEnvForkPrimitiveSubstitutionScenarioID || scenario.OracleKindID != "env-residue" {
		t.Fatalf("expected generated candidate to materialize exact Scenario IR: %#v", scenario)
	}
	wantSignature := target.TargetSignatureForScenario(generated.TaskID, scenario)
	if generated.Signature.String() != wantSignature.String() {
		t.Fatalf("unexpected generated signature: got=%s want=%s", generated.Signature.String(), wantSignature.String())
	}

	var functionGenerated *TargetScheduleCandidate
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		if candidate.ScenarioID == target.GeneratedFunctionForkPrimitiveSubstitutionScenarioID {
			functionGenerated = candidate
			break
		}
	}
	if functionGenerated == nil {
		t.Fatalf("expected generated function fork primitive substitution: %#v", matrix.Candidates)
	}
	if functionGenerated.PlantPrimitiveID != "shell-function-define" || functionGenerated.ActivationKindID != "shell-function-invocation" {
		t.Fatalf("unexpected function substitution metadata: %#v", functionGenerated)
	}
	if functionGenerated.ExecutionPlan == nil || functionGenerated.ExecutionPlan.CheckpointSelector != "before-function-define" {
		t.Fatalf("expected executable function fork plan: %#v", functionGenerated)
	}
	if functionGenerated.OracleKindID != "function-residue" || functionGenerated.ContractRuleID != "shell-function-generated-fork-boundary" {
		t.Fatalf("expected generated function oracle and contract bindings: %#v", functionGenerated)
	}
	if !strings.Contains(functionGenerated.Prompt, "syncfuzz_residue_probe()") {
		t.Fatalf("expected generated function plant prompt: %#v", functionGenerated)
	}
}

func TestBuildTargetScheduleMatrixAddsPortableContinuationPrimitiveSubstitutionFamily(t *testing.T) {
	tests := []struct {
		targetID        string
		expectContracts map[string]string
	}{
		{targetID: "langgraph-shell-react", expectContracts: map[string]string{
			target.GeneratedEnvContinuationPrimitiveSubstitutionScenarioID:      "shell-env-generated-within-run",
			target.GeneratedFunctionContinuationPrimitiveSubstitutionScenarioID: "shell-function-generated-within-run",
			target.GeneratedCWDContinuationPrimitiveSubstitutionScenarioID:      "shell-cwd-generated-within-run",
			target.GeneratedUmaskContinuationPrimitiveSubstitutionScenarioID:    "shell-umask-generated-within-run",
		}},
		{targetID: "maf-github-copilot-shell", expectContracts: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.targetID, func(t *testing.T) {
			matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
				TargetID: tt.targetID,
				Tasks:    []string{target.PersistentShellTargetTaskID},
			})
			if err != nil {
				t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
			}
			if matrix.TotalCandidates != 5 {
				t.Fatalf("expected base plus portable env/function/cwd/umask substitutions, got %#v", matrix)
			}

			found := make(map[string]*TargetScheduleCandidate)
			for idx := range matrix.Candidates {
				candidate := &matrix.Candidates[idx]
				switch candidate.ScenarioID {
				case target.GeneratedEnvContinuationPrimitiveSubstitutionScenarioID,
					target.GeneratedFunctionContinuationPrimitiveSubstitutionScenarioID,
					target.GeneratedCWDContinuationPrimitiveSubstitutionScenarioID,
					target.GeneratedUmaskContinuationPrimitiveSubstitutionScenarioID:
					found[candidate.ScenarioID] = candidate
				}
			}
			if len(found) != 4 {
				t.Fatalf("expected portable continuation substitutions: %#v", matrix.Candidates)
			}
			for scenarioID, candidate := range found {
				if !candidate.Generated || candidate.TaskID != target.PersistentShellTargetTaskID {
					t.Fatalf("unexpected portable continuation metadata: %#v", candidate)
				}
				if candidate.ExecutionPlan == nil || candidate.ExecutionPlan.LifecycleOperationID != "run-continue" || candidate.ExecutionPlan.ForkFollowup || candidate.ExecutionPlan.Replay {
					t.Fatalf("expected executable same-run plan: %#v", candidate)
				}
				if candidate.ContractRuleID != tt.expectContracts[scenarioID] {
					t.Fatalf("unexpected portable continuation contract binding: %#v", candidate)
				}
			}
		})
	}
}

func TestBuildTargetScheduleMatrixAddsReplayPrimitiveSubstitutionFamily(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.PersistentShellReplayTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if matrix.TotalCandidates != 6 {
		t.Fatalf("expected replay base, prompt/process-mode variants, and generated env/function replay substitutions, got %#v", matrix)
	}

	found := make(map[string]*TargetScheduleCandidate)
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		switch candidate.ScenarioID {
		case target.GeneratedEnvReplayPrimitiveSubstitutionScenarioID,
			target.GeneratedFunctionReplayPrimitiveSubstitutionScenarioID:
			found[candidate.ScenarioID] = candidate
		}
	}
	if len(found) != 2 {
		t.Fatalf("expected generated replay substitutions: %#v", matrix.Candidates)
	}
	for scenarioID, candidate := range found {
		if !candidate.Generated || candidate.TaskID != target.PersistentShellReplayTargetTaskID {
			t.Fatalf("unexpected replay substitution metadata: %#v", candidate)
		}
		if candidate.ExecutionPlan == nil || !candidate.ExecutionPlan.Replay || candidate.ExecutionPlan.ForkFollowup || candidate.ExecutionPlan.ProcessMode != "split-process" {
			t.Fatalf("expected executable replay plan: %#v", candidate)
		}
		switch scenarioID {
		case target.GeneratedEnvReplayPrimitiveSubstitutionScenarioID:
			if candidate.PlantPrimitiveID != "shell-env-export" || candidate.ContractRuleID != "shell-env-generated-replay-boundary" {
				t.Fatalf("unexpected env replay candidate: %#v", candidate)
			}
		case target.GeneratedFunctionReplayPrimitiveSubstitutionScenarioID:
			if candidate.PlantPrimitiveID != "shell-function-define" || candidate.ContractRuleID != "shell-function-generated-replay-boundary" {
				t.Fatalf("unexpected function replay candidate: %#v", candidate)
			}
		}
	}
}

func TestBuildTargetScheduleMatrixAddsPortableContinuationActivationSubstitutionFamily(t *testing.T) {
	tests := []struct {
		targetID         string
		expectContractID string
	}{
		{targetID: "langgraph-shell-react", expectContractID: "communication-trusted-action-generated-within-run"},
		{targetID: "maf-github-copilot-shell"},
	}
	for _, tt := range tests {
		t.Run(tt.targetID, func(t *testing.T) {
			matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
				TargetID: tt.targetID,
				Tasks:    []string{target.UnixListenerResidueTargetTaskID},
			})
			if err != nil {
				t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
			}
			if matrix.TotalCandidates != 4 {
				t.Fatalf("expected base prompt variants plus portable trusted-action substitution, got %#v", matrix)
			}

			var generated *TargetScheduleCandidate
			for idx := range matrix.Candidates {
				candidate := &matrix.Candidates[idx]
				if candidate.ScenarioID == target.GeneratedTrustedActionContinuationScenarioID {
					generated = candidate
					break
				}
			}
			if generated == nil {
				t.Fatalf("expected portable trusted-action substitution: %#v", matrix.Candidates)
			}
			if !generated.Generated || generated.TaskID != target.UnixListenerResidueTargetTaskID || generated.PlantPrimitiveID != "workspace-unix-listener" {
				t.Fatalf("unexpected portable trusted-action metadata: %#v", generated)
			}
			if generated.ActivationKindID != "trusted-action-effect" || generated.OracleKindID != "trusted-action-execution" || generated.MutationFocusKind != target.TargetScenarioMutationActivationSubstitution {
				t.Fatalf("unexpected portable trusted-action mutation bindings: %#v", generated)
			}
			if generated.ExecutionPlan == nil || generated.ExecutionPlan.LifecycleOperationID != "run-continue" || generated.ExecutionPlan.ForkFollowup || generated.ExecutionPlan.Replay {
				t.Fatalf("expected executable same-run trusted-action plan: %#v", generated)
			}
			if generated.ContractRuleID != tt.expectContractID {
				t.Fatalf("unexpected portable trusted-action contract binding: %#v", generated)
			}
		})
	}
}

func TestBuildTargetScheduleMatrixAddsExecutableActivationSubstitution(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.UnixListenerResidueForkTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	var generated *TargetScheduleCandidate
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		if candidate.ScenarioID == target.GeneratedTrustedActionActivationScenarioID {
			generated = candidate
			break
		}
	}
	if generated == nil {
		t.Fatalf("expected generated trusted-action activation: %#v", matrix.Candidates)
	}
	if !generated.Generated || generated.PlantPrimitiveID != "workspace-unix-listener" || generated.ActivationKindID != "trusted-action-effect" {
		t.Fatalf("unexpected activation substitution metadata: %#v", generated)
	}
	if generated.MutationFocusKind != target.TargetScenarioMutationActivationSubstitution || generated.OracleKindID != "trusted-action-execution" {
		t.Fatalf("expected activation mutation and oracle bindings: %#v", generated)
	}
	if generated.ExecutionPlan == nil || generated.ExecutionPlan.CheckpointSelector != "before-unix-listener-launch" || !strings.Contains(generated.ExecutionPlan.ForkMessage, target.TargetTrustedActionEffectArtifact) {
		t.Fatalf("expected executable trusted-action plan: %#v", generated)
	}
	if generated.ContractRuleID != "communication-trusted-action-generated-fork-boundary" {
		t.Fatalf("expected generated trusted-action contract: %#v", generated)
	}
	scenario := targetScenarioForCandidate(*generated)
	if scenario == nil || scenario.ScenarioID != target.GeneratedTrustedActionActivationScenarioID {
		t.Fatalf("expected exact generated activation Scenario IR: %#v", scenario)
	}
}

func TestBuildTargetScheduleMatrixAddsInheritedFDTrustedActionSubstitution(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.InheritedFDLeakTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	var generated *TargetScheduleCandidate
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		if candidate.ScenarioID == target.GeneratedInheritedFDTrustedActionScenarioID {
			generated = candidate
			break
		}
	}
	if generated == nil {
		t.Fatalf("expected generated inherited-fd trusted-action activation: %#v", matrix.Candidates)
	}
	if !generated.Generated || generated.PlantPrimitiveID != "workspace-inherited-fd-holder" || generated.ActivationKindID != "trusted-secret-action" {
		t.Fatalf("unexpected inherited-fd activation substitution metadata: %#v", generated)
	}
	if generated.MutationFocusKind != target.TargetScenarioMutationActivationSubstitution || generated.OracleKindID != "trusted-action-execution" {
		t.Fatalf("expected inherited-fd activation mutation and oracle bindings: %#v", generated)
	}
	if generated.ExecutionPlan == nil || generated.ExecutionPlan.CheckpointSelector != "before-inherited-fd-leak-holder" || !strings.Contains(generated.ExecutionPlan.ForkMessage, target.TargetInheritedFDTrustedEffectArtifact) {
		t.Fatalf("expected executable inherited-fd trusted-action plan: %#v", generated)
	}
	if generated.ContractRuleID != "capability-inherited-fd-trusted-action-generated-fork-boundary" {
		t.Fatalf("expected generated inherited-fd trusted-action contract: %#v", generated)
	}
}

func TestBuildTargetScheduleMatrixAddsUnixListenerReplayLifecycleSplice(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID: "langgraph-shell-react",
		Tasks:    []string{target.UnixListenerResidueForkTargetTaskID},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	var generated *TargetScheduleCandidate
	for idx := range matrix.Candidates {
		candidate := &matrix.Candidates[idx]
		if candidate.ScenarioID == target.GeneratedUnixListenerReplayLifecycleSpliceScenarioID {
			generated = candidate
			break
		}
	}
	if generated == nil {
		t.Fatalf("expected generated Unix-listener replay lifecycle splice: %#v", matrix.Candidates)
	}
	if !generated.Generated || generated.PlantPrimitiveID != "workspace-unix-listener" || generated.ActivationKindID != "unix-socket-connect" {
		t.Fatalf("unexpected replay splice metadata: %#v", generated)
	}
	if generated.MutationFocusKind != target.TargetScenarioMutationLifecycleSplice || generated.OracleKindID != "workspace-unix-listener-residue" {
		t.Fatalf("expected lifecycle splice mutation and oracle bindings: %#v", generated)
	}
	if generated.ExecutionPlan == nil || generated.ExecutionPlan.CheckpointSelector != "before-unix-listener-launch" || !generated.ExecutionPlan.Replay || generated.ExecutionPlan.ForkFollowup {
		t.Fatalf("expected executable replay splice plan: %#v", generated)
	}
	if generated.LifecycleOperationID != "checkpoint-replay" || generated.LifecycleEdge != "checkpoint->replay" {
		t.Fatalf("expected replay lifecycle metadata: %#v", generated)
	}
	if generated.ContractRuleID != "runtime-unix-listener-generated-replay-boundary" {
		t.Fatalf("expected generated replay splice contract: %#v", generated)
	}
}

func TestBuildTargetScheduleMatrixSupportsMAFBaselineGroup(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "maf-github-copilot-shell",
		TaskGroups: []string{"maf-baseline"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.TaskGroups) != 1 || matrix.TaskGroups[0] != "maf-baseline" {
		t.Fatalf("unexpected MAF task groups: %#v", matrix.TaskGroups)
	}
	if len(matrix.Tasks) != 2 {
		t.Fatalf("expected two MAF baseline tasks, got %#v", matrix.Tasks)
	}
	if matrix.TotalCandidates != 3 {
		t.Fatalf("expected base orphan-process plus long-delay base/mutation candidates, got %#v", matrix)
	}
	late, err := findTargetMatrixCandidate(matrix, target.LongDelayTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if late.TargetID != "maf-github-copilot-shell" || late.SeedID != "delayed-effect" {
		t.Fatalf("unexpected MAF long-delay candidate metadata: %#v", late)
	}
	if late.ContractProfileID != "" || late.ContractRuleID != "" {
		t.Fatalf("expected no contract metadata for current MAF baseline: %#v", late)
	}
}

func TestBuildTargetScheduleMatrixSupportsMAFShellContextGroup(t *testing.T) {
	matrix, err := BuildTargetScheduleMatrix(TargetMatrixOptions{
		TargetID:   "maf-github-copilot-shell",
		TaskGroups: []string{"maf-shell-context"},
	})
	if err != nil {
		t.Fatalf("BuildTargetScheduleMatrix failed: %v", err)
	}
	if len(matrix.TaskGroups) != 1 || matrix.TaskGroups[0] != "maf-shell-context" {
		t.Fatalf("unexpected MAF shell-context groups: %#v", matrix.TaskGroups)
	}
	if len(matrix.Tasks) != 7 {
		t.Fatalf("expected seven MAF shell-context tasks, got %#v", matrix.Tasks)
	}
	if matrix.TotalCandidates != 16 {
		t.Fatalf("expected delayed-effect plus expanded same-run shell-context candidates, got %#v", matrix)
	}
	persistent, err := findTargetMatrixCandidate(matrix, target.PersistentShellTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if persistent.TargetID != "maf-github-copilot-shell" || persistent.SeedID != "shell-path-residue" {
		t.Fatalf("unexpected MAF persistent-shell candidate metadata: %#v", persistent)
	}
	cwd, err := findTargetMatrixCandidate(matrix, target.CWDResidueTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if cwd.TargetID != "maf-github-copilot-shell" || cwd.SeedID != "shell-execution-context-residue" {
		t.Fatalf("unexpected MAF cwd residue candidate metadata: %#v", cwd)
	}
	envResidue, err := findTargetMatrixCandidate(matrix, target.EnvResidueTargetTaskID)
	if err != nil {
		t.Fatalf("findTargetMatrixCandidate failed: %v", err)
	}
	if envResidue.TargetID != "maf-github-copilot-shell" || envResidue.SeedID != "shell-execution-context-residue" {
		t.Fatalf("unexpected MAF env residue candidate metadata: %#v", envResidue)
	}
}

func TestRunTargetSuiteMatrixWritesArtifacts(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "matrix-smoke",
		Tasks:        []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:      command,
		ObserveDelay: 10 * time.Millisecond,
		Matrix:       true,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}
	if result.SchedulerMode != suiteSchedulerMatrix {
		t.Fatalf("expected target matrix scheduler, got %q", result.SchedulerMode)
	}
	if result.TotalCandidates != 2 || result.TotalRuns != 2 {
		t.Fatalf("unexpected target matrix counts: %#v", result)
	}
	if result.ScheduleMatrix == "" || result.MatrixResult == "" {
		t.Fatalf("expected target matrix artifacts: %#v", result)
	}
	if len(result.CandidateSummaries) != 2 {
		t.Fatalf("expected two target candidate summaries: %#v", result.CandidateSummaries)
	}
	taskCoverage := findTargetDimensionCoverage(t, result.DimensionCoverage, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 2 || taskCoverage.ConfirmedValues != 2 {
		t.Fatalf("unexpected matrix task coverage: %#v", taskCoverage)
	}
	for _, item := range result.Results {
		if item.CandidateID == "" || item.TargetID != "matrix-smoke" {
			t.Fatalf("expected candidate-aware target run result: %#v", item)
		}
		if item.TaskID == target.PersistentShellTargetTaskID {
			if item.MinimizationPlan == nil || !item.MinimizationPlan.Applicable {
				t.Fatalf("expected applicable minimization plan for confirmed matrix item: %#v", item.MinimizationPlan)
			}
			if len(item.MinimizationPlan.Steps) == 0 {
				t.Fatalf("expected minimization steps: %#v", item.MinimizationPlan)
			}
			if !containsStringPrefix(item.MinimizationPlan.Preserve, "artifact="+target.TargetShellPoisonCheckArtifact) {
				t.Fatalf("expected minimization plan to preserve witness artifact: %#v", item.MinimizationPlan.Preserve)
			}
			if !targetMinimizationPlanHasStepKind(item.MinimizationPlan, "artifact-replay-check") {
				t.Fatalf("expected replay minimization step: %#v", item.MinimizationPlan)
			}
		}
	}
	if _, err := os.Stat(result.ScheduleMatrix); err != nil {
		t.Fatalf("expected target schedule matrix artifact: %v", err)
	}
	if _, err := os.Stat(result.MatrixResult); err != nil {
		t.Fatalf("expected target matrix result artifact: %v", err)
	}
}

func TestBuildTargetMinimizationPlanIncludesMutationAxes(t *testing.T) {
	plan := buildTargetMinimizationPlan(TargetScheduleCandidate{
		TargetID:             "langgraph-shell-react",
		TaskID:               target.PersistentShellReplayTargetTaskID,
		OracleKindID:         "replay-path-residue",
		DefaultExpectedFiles: []string{target.TargetShellPoisonReplayArtifact},
		Components: []target.TargetScenarioComponent{{
			ComponentID: "plant.shell-path-prepend",
			Role:        target.TargetScenarioComponentPlant,
			KindID:      "shell-path-prepend",
			Summary:     "prepend PATH",
		}},
		Mutations: []target.TargetScenarioMutation{{
			MutationID: "lifecycle-splice.checkpoint-replay",
			Kind:       target.TargetScenarioMutationLifecycleSplice,
			Summary:    "resume from an earlier checkpoint",
		}},
	}, TargetSuiteRunResult{
		TargetID:     "langgraph-shell-react",
		TaskID:       target.PersistentShellReplayTargetTaskID,
		OracleKindID: "replay-path-residue",
		Confirmed:    true,
		TargetOracle: target.TargetOracleResult{Attribution: "runtime-preserved-residue"},
	}, corpus.TargetObservationDetails{
		Category:          corpus.TargetObservationResidueObserved,
		ActivationReached: true,
	})
	if plan == nil || !plan.Applicable {
		t.Fatalf("expected applicable minimization plan: %#v", plan)
	}
	if !targetMinimizationPlanHasStepKind(plan, "mutation-axis-check") {
		t.Fatalf("expected mutation-axis minimization step: %#v", plan)
	}
	if !targetMinimizationPlanHasComponent(plan, "plant.shell-path-prepend", "shell-path-prepend") {
		t.Fatalf("expected stable Scenario IR component identity in minimization plan: %#v", plan)
	}
	if !containsStringPrefix(plan.Preserve, "artifact="+target.TargetShellPoisonReplayArtifact) {
		t.Fatalf("expected replay witness artifact in preserve list: %#v", plan.Preserve)
	}
}

func containsStringPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func targetMinimizationPlanHasStepKind(plan *TargetMinimizationPlan, kind string) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if step.Kind == kind {
			return true
		}
	}
	return false
}

func targetMinimizationPlanHasComponent(plan *TargetMinimizationPlan, componentID string, kindID string) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if step.ComponentID == componentID && step.ComponentKind == kindID {
			return true
		}
	}
	return false
}

func TestRunTargetSuiteFeedbackMatrixPreservesUniverseCoverageGaps(t *testing.T) {
	tmp := t.TempDir()
	command := `case "$SYNCFUZZ_TASK_ID" in
orphan-process) printf ok > late-effect ;;
persistent-shell-poisoning) mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt ;;
*) exit 9 ;;
esac`
	result, err := RunTargetSuite(context.Background(), TargetSuiteOptions{
		OutDir:         filepath.Join(tmp, "runs"),
		TargetID:       "matrix-frontier-smoke",
		Tasks:          []string{target.DefaultTargetTaskID, target.PersistentShellTargetTaskID},
		Command:        command,
		ObserveDelay:   10 * time.Millisecond,
		Matrix:         true,
		CandidateLimit: 1,
	})
	if err != nil {
		t.Fatalf("RunTargetSuite failed: %v", err)
	}

	taskCoverage := findTargetDimensionCoverage(t, result.DimensionCoverage, "task_id")
	if taskCoverage.TotalValues != 2 || taskCoverage.ExecutedValues != 1 || len(taskCoverage.MissingValues) != 1 {
		t.Fatalf("expected universe-aware coverage gaps after limited matrix run: %#v", taskCoverage)
	}
	if len(result.FrontierCandidates) != 1 {
		t.Fatalf("expected exactly one next frontier candidate, got %#v", result.FrontierCandidates)
	}
	if result.FrontierCandidates[0].CandidateID == result.Results[0].CandidateID {
		t.Fatalf("expected frontier to point at the remaining candidate, got %#v", result.FrontierCandidates[0])
	}
	if result.FrontierCandidates[0].SelectionMode == "" {
		t.Fatalf("expected frontier recommendation metadata: %#v", result.FrontierCandidates[0])
	}
}
