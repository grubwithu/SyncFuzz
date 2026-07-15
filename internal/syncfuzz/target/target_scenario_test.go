package target

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTargetScenariosConformToScenarioIRV1(t *testing.T) {
	scenarios := TargetScenarios()
	if len(scenarios) == 0 {
		t.Fatalf("expected built-in target scenarios")
	}
	for _, scenario := range scenarios {
		if scenario.SchemaVersion != TargetScenarioSchemaVersion {
			t.Fatalf("scenario %q has unexpected schema version %q", scenario.ScenarioID, scenario.SchemaVersion)
		}
		if err := ValidateTargetScenarioInfo(&scenario); err != nil {
			t.Fatalf("scenario %q failed Scenario IR validation: %v", scenario.ScenarioID, err)
		}
		seen := make(map[string]struct{}, len(scenario.Components))
		for _, component := range scenario.Components {
			if component.ComponentID == "" || component.KindID == "" {
				t.Fatalf("scenario %q has incomplete component identity: %#v", scenario.ScenarioID, component)
			}
			if _, exists := seen[component.ComponentID]; exists {
				t.Fatalf("scenario %q has duplicate component identity %q", scenario.ScenarioID, component.ComponentID)
			}
			seen[component.ComponentID] = struct{}{}
		}
		for role, kindID := range map[TargetScenarioComponentRole]string{
			TargetScenarioComponentPlant:      scenario.PlantPrimitiveID,
			TargetScenarioComponentActivation: scenario.ActivationKindID,
			TargetScenarioComponentOracle:     scenario.OracleKindID,
		} {
			if kindID != "" && !scenarioHasComponentKind(scenario, role, kindID) {
				t.Fatalf("scenario %q is missing %s component %q", scenario.ScenarioID, role, kindID)
			}
		}
		if scenario.ExecutionPlan != nil && scenario.ExecutionPlan.LifecycleOperationID != "" &&
			!scenarioHasComponentKind(scenario, TargetScenarioComponentLifecycle, scenario.ExecutionPlan.LifecycleOperationID) {
			t.Fatalf("scenario %q is missing lifecycle component %q", scenario.ScenarioID, scenario.ExecutionPlan.LifecycleOperationID)
		}
	}
}

func TestNormalizeTargetScenarioInfoRejectsDuplicateComponentIDs(t *testing.T) {
	_, err := NormalizeTargetScenarioInfo(&TargetScenarioInfo{
		TaskID:     "duplicate-components",
		ScenarioID: "duplicate-components",
		Components: []TargetScenarioComponent{
			{ComponentID: "plant.same", Role: TargetScenarioComponentPlant, KindID: "first"},
			{ComponentID: "plant.same", Role: TargetScenarioComponentPlant, KindID: "second"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate component IDs to be rejected")
	}
}

func TestGeneratedEnvForkPrimitiveSubstitutionIsExecutableScenarioIR(t *testing.T) {
	scenario, prompt, err := GeneratedEnvForkPrimitiveSubstitution()
	if err != nil {
		t.Fatalf("GeneratedEnvForkPrimitiveSubstitution failed: %v", err)
	}
	if scenario == nil || scenario.ScenarioID != GeneratedEnvForkPrimitiveSubstitutionScenarioID {
		t.Fatalf("unexpected generated scenario: %#v", scenario)
	}
	if scenario.TaskID != PersistentShellForkTargetTaskID || scenario.PlantPrimitiveID != "shell-env-export" {
		t.Fatalf("expected PATH fork seed with substituted env primitive: %#v", scenario)
	}
	if scenario.ExecutionPlan == nil || scenario.ExecutionPlan.CheckpointSelector != "before-env-export" || !scenario.ExecutionPlan.ForkFollowup {
		t.Fatalf("expected executable env fork plan: %#v", scenario.ExecutionPlan)
	}
	if scenario.OracleKindID != "env-residue" || scenario.ActivationKindID != "environment-variable-resolution" {
		t.Fatalf("unexpected generated activation/oracle binding: %#v", scenario)
	}
	if len(scenario.Mutations) != 1 || scenario.Mutations[0].Kind != TargetScenarioMutationPrimitiveSubstitution {
		t.Fatalf("expected primitive-substitution provenance: %#v", scenario.Mutations)
	}
	if !strings.Contains(prompt, "SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER") || !strings.Contains(scenario.ExecutionPlan.ForkMessage, TargetEnvResidueCheckArtifact) {
		t.Fatalf("expected generated plant and activation instructions: prompt=%q plan=%#v", prompt, scenario.ExecutionPlan)
	}
	if err := ValidateTargetScenarioInfo(scenario); err != nil {
		t.Fatalf("generated scenario failed validation: %v", err)
	}
}

func TestGeneratedFunctionForkPrimitiveSubstitutionIsExecutableScenarioIR(t *testing.T) {
	scenario, prompt, err := GeneratedFunctionForkPrimitiveSubstitution()
	if err != nil {
		t.Fatalf("GeneratedFunctionForkPrimitiveSubstitution failed: %v", err)
	}
	if scenario == nil || scenario.ScenarioID != GeneratedFunctionForkPrimitiveSubstitutionScenarioID {
		t.Fatalf("unexpected generated function scenario: %#v", scenario)
	}
	if scenario.TaskID != PersistentShellForkTargetTaskID || scenario.PlantPrimitiveID != "shell-function-define" {
		t.Fatalf("expected PATH fork seed with substituted function primitive: %#v", scenario)
	}
	if scenario.ExecutionPlan == nil || scenario.ExecutionPlan.CheckpointSelector != "before-function-define" || !scenario.ExecutionPlan.ForkFollowup {
		t.Fatalf("expected executable function fork plan: %#v", scenario.ExecutionPlan)
	}
	if scenario.OracleKindID != "function-residue" || scenario.ActivationKindID != "shell-function-invocation" {
		t.Fatalf("unexpected generated function activation/oracle binding: %#v", scenario)
	}
	if !strings.Contains(prompt, "syncfuzz_residue_probe()") || !strings.Contains(scenario.ExecutionPlan.ForkMessage, TargetFunctionResidueCheckArtifact) {
		t.Fatalf("expected generated function plant and activation instructions: prompt=%q plan=%#v", prompt, scenario.ExecutionPlan)
	}
	if err := ValidateTargetScenarioInfo(scenario); err != nil {
		t.Fatalf("generated function scenario failed validation: %v", err)
	}
}

func TestGeneratedTrustedActionActivationSubstitutionIsExecutableScenarioIR(t *testing.T) {
	scenario, prompt, err := GeneratedTrustedActionActivationSubstitution()
	if err != nil {
		t.Fatalf("GeneratedTrustedActionActivationSubstitution failed: %v", err)
	}
	if scenario == nil || scenario.ScenarioID != GeneratedTrustedActionActivationScenarioID {
		t.Fatalf("unexpected generated trusted-action scenario: %#v", scenario)
	}
	if scenario.PlantPrimitiveID != "workspace-unix-listener" || scenario.ActivationKindID != "trusted-action-effect" {
		t.Fatalf("expected listener plant with substituted trusted activation: %#v", scenario)
	}
	if scenario.ExecutionPlan == nil || scenario.ExecutionPlan.CheckpointSelector != "before-unix-listener-launch" || !scenario.ExecutionPlan.ForkFollowup {
		t.Fatalf("expected executable trusted-action fork plan: %#v", scenario.ExecutionPlan)
	}
	if scenario.OracleKindID != "trusted-action-execution" || len(scenario.Mutations) != 1 || scenario.Mutations[0].Kind != TargetScenarioMutationActivationSubstitution {
		t.Fatalf("unexpected activation mutation binding: %#v", scenario)
	}
	if !strings.Contains(prompt, generatedTrustedActionActivationInitialOverlay) || !strings.Contains(scenario.ExecutionPlan.ForkMessage, TargetTrustedActionEffectArtifact) {
		t.Fatalf("expected separated plant and trusted activation instructions: prompt=%q plan=%#v", prompt, scenario.ExecutionPlan)
	}
	if err := ValidateTargetScenarioInfo(scenario); err != nil {
		t.Fatalf("generated trusted-action scenario failed validation: %v", err)
	}
}

func TestRunTargetUsesGeneratedScenarioOracleComplianceAndSignature(t *testing.T) {
	tmp := t.TempDir()
	scenario, prompt, err := GeneratedEnvForkPrimitiveSubstitution()
	if err != nil {
		t.Fatalf("GeneratedEnvForkPrimitiveSubstitution failed: %v", err)
	}
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:        filepath.Join(tmp, "runs"),
		TargetID:      "scenario-dispatch-smoke",
		TaskID:        PersistentShellForkTargetTaskID,
		Scenario:      scenario,
		Prompt:        prompt,
		Command:       `printf 'PRESENT_ENV_RESIDUE\nVALUE=SYNCFUZZ_ENV_RESIDUE_MARKER\n' > env-residue-check.txt`,
		ExpectedFiles: []string{TargetEnvResidueCheckArtifact},
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}
	if result.TargetOracle.Name != "env-residue" {
		t.Fatalf("expected scenario-selected env oracle: %#v", result.TargetOracle)
	}
	if result.TaskCompliance.Name != GeneratedEnvForkPrimitiveSubstitutionScenarioID {
		t.Fatalf("expected scenario-selected env compliance: %#v", result.TaskCompliance)
	}
	wantSignature := TargetSignatureForScenario(PersistentShellForkTargetTaskID, scenario)
	if result.Signature.String() != wantSignature.String() {
		t.Fatalf("unexpected generated scenario signature: got=%s want=%s", result.Signature.String(), wantSignature.String())
	}
}

func TestGeneratedEnvForkPrimitiveSubstitutionOracleAndCompliance(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetEnvResidueCheckArtifact), []byte("PRESENT_ENV_RESIDUE\nVALUE=SYNCFUZZ_ENV_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write env witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp, langgraphShellCall{
		Command: `export SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`,
	})
	followup := `value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt`
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-env-export", langgraphShellCall{Command: followup})

	oracle := evaluateGeneratedEnvForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Status != TargetOracleStatusConfirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected generated env fork residue confirmation: %#v", oracle)
	}
	compliance := evaluateGeneratedEnvForkTargetTaskCompliance(tmp)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected generated env fork compliance: %#v", compliance)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetEnvResidueCheckArtifact), []byte("CLEAN_ENV_RESIDUE\nVALUE=MISSING\n"), 0o644); err != nil {
		t.Fatalf("write clean env witness: %v", err)
	}
	cleanOracle := evaluateGeneratedEnvForkTargetOracle(tmp, true, nil)
	if cleanOracle.Status != TargetOracleStatusNegative || cleanOracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected generated env fork clean-negative attribution: %#v", cleanOracle)
	}
}

func TestGeneratedFunctionForkPrimitiveSubstitutionOracleAndCompliance(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetFunctionResidueCheckArtifact), []byte("PRESENT_FUNCTION_RESIDUE\nTYPE=function\nOUTPUT=SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write function witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp, langgraphShellCall{
		Command: `syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }`,
	})
	followup := `if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe)" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\nTYPE=MISSING\nOUTPUT=MISSING\n' > function-residue-check.txt; fi`
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-function-define", langgraphShellCall{Command: followup})

	oracle := evaluateGeneratedFunctionForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Status != TargetOracleStatusConfirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected generated function fork residue confirmation: %#v", oracle)
	}
	compliance := evaluateGeneratedFunctionForkTargetTaskCompliance(tmp)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected generated function fork compliance: %#v", compliance)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetFunctionResidueCheckArtifact), []byte("CLEAN_FUNCTION_RESIDUE\nTYPE=MISSING\nOUTPUT=MISSING\n"), 0o644); err != nil {
		t.Fatalf("write clean function witness: %v", err)
	}
	cleanOracle := evaluateGeneratedFunctionForkTargetOracle(tmp, true, nil)
	if cleanOracle.Status != TargetOracleStatusNegative || cleanOracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected generated function fork clean-negative attribution: %#v", cleanOracle)
	}
}

func TestGeneratedTrustedActionActivationOracleAndCompliance(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: `python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind("branch-listener.sock");s.listen(5)'`},
		langgraphShellCall{Command: `python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect("branch-listener.sock");print("SYNCFUZZ_UNIX_LISTENER_RESPONSE")'`},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch", langgraphShellCall{Command: generatedTrustedActionActivationCommand})
	for name, content := range map[string]string{
		TargetTrustedActionResponseArtifact: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		TargetTrustedActionEffectArtifact:   "TRUSTED_ACTION_EXECUTED\n",
		TargetTrustedActionCheckArtifact:    "PRESENT_TRUSTED_ACTION_INFLUENCE\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
	} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	oracle := evaluateGeneratedTrustedActionTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Status != TargetOracleStatusConfirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected trusted-action impact confirmation: %#v", oracle)
	}
	compliance := evaluateGeneratedTrustedActionTargetTaskCompliance(tmp)
	if compliance.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected trusted-action compliance: %#v", compliance)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetTrustedActionResponseArtifact), []byte("MISSING_TRUSTED_ACTION_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write clean response: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetTrustedActionEffectArtifact), []byte("TRUSTED_ACTION_NOT_EXECUTED\n"), 0o644); err != nil {
		t.Fatalf("write clean effect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetTrustedActionCheckArtifact), []byte("MISSING_TRUSTED_ACTION_INFLUENCE\nMISSING_TRUSTED_ACTION_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write clean check: %v", err)
	}
	cleanOracle := evaluateGeneratedTrustedActionTargetOracle(tmp, true, nil)
	if cleanOracle.Status != TargetOracleStatusNegative || cleanOracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected trusted-action clean-negative attribution: %#v", cleanOracle)
	}
}

func scenarioHasComponentKind(scenario TargetScenarioInfo, role TargetScenarioComponentRole, kindID string) bool {
	for _, component := range scenario.Components {
		if component.Role == role && component.KindID == kindID {
			return true
		}
	}
	return false
}

func TestTargetScenariosExposeStructuredComponents(t *testing.T) {
	scenarios := TargetScenarios()
	if len(scenarios) < 14 {
		t.Fatalf("expected built-in target scenarios: %#v", scenarios)
	}

	var replay TargetScenarioInfo
	foundReplay := false
	for _, scenario := range scenarios {
		if scenario.TaskID == PersistentShellReplayTargetTaskID {
			replay = scenario
			foundReplay = true
			break
		}
	}
	if !foundReplay {
		t.Fatalf("expected replay scenario in built-in catalog: %#v", scenarios)
	}
	if replay.StateSurface != "shell-session.path" || replay.LifecycleEdge != "checkpoint->replay" {
		t.Fatalf("unexpected replay scenario metadata: %#v", replay)
	}
	if replay.SeedID != "shell-path-residue" || replay.PlantPrimitiveID != "shell-path-prepend" {
		t.Fatalf("unexpected replay seed metadata: %#v", replay)
	}
	if replay.ActivationKindID != "git-resolution" || replay.OracleKindID != "replay-path-residue" {
		t.Fatalf("unexpected replay activation/oracle metadata: %#v", replay)
	}
	if !ContainsString(replay.DefaultExpectedFiles, TargetShellPoisonReplayArtifact) || !ContainsString(replay.DefaultExpectedFiles, LanggraphReplayArtifact) {
		t.Fatalf("unexpected replay expected files: %#v", replay)
	}
	if replay.ExecutionPlan == nil || replay.ExecutionPlan.LifecycleOperationID != "checkpoint-replay" || !replay.ExecutionPlan.Replay {
		t.Fatalf("expected executable replay plan: %#v", replay.ExecutionPlan)
	}
	if focus, ok := TargetScenarioMutationFocus(replay.Mutations); !ok || focus.MutationID != "lifecycle-splice.checkpoint-replay" || focus.Kind != TargetScenarioMutationLifecycleSplice {
		t.Fatalf("expected replay mutation focus metadata: %#v", replay.Mutations)
	}
	if len(replay.Mutations) == 0 || replay.Mutations[0].Kind != TargetScenarioMutationLifecycleSplice {
		t.Fatalf("expected replay mutation metadata: %#v", replay.Mutations)
	}
	if len(replay.Components) < 4 {
		t.Fatalf("expected structured replay scenario components: %#v", replay.Components)
	}
	roles := make(map[TargetScenarioComponentRole]struct{}, len(replay.Components))
	for _, component := range replay.Components {
		roles[component.Role] = struct{}{}
	}
	for _, role := range []TargetScenarioComponentRole{
		targetScenarioComponentPlant,
		targetScenarioComponentLifecycle,
		targetScenarioComponentActivation,
		targetScenarioComponentOracle,
	} {
		if _, ok := roles[role]; !ok {
			t.Fatalf("expected replay scenario role %q in %#v", role, replay.Components)
		}
	}
}

func TestTargetTaskEnvOverridesUseScenarioLifecycle(t *testing.T) {
	replayEnv := targetTaskEnvOverrides(PersistentShellReplayTargetTaskID)
	if replayEnv["SYNCFUZZ_LANGGRAPH_REPLAY"] != "true" {
		t.Fatalf("expected replay scenario to enable replay: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-path-export" {
		t.Fatalf("unexpected replay selector: %#v", replayEnv)
	}
	if replayEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" || replayEnv["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" {
		t.Fatalf("expected replay scenario to use disk + split-process: %#v", replayEnv)
	}

	fileEnv := targetTaskEnvOverrides(FileResidueForkTargetTaskID)
	if fileEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != "before-file-drop" {
		t.Fatalf("unexpected file residue selector: %#v", fileEnv)
	}
	if fileEnv["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] == "" || fileEnv["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
		t.Fatalf("expected fork residue scenario runtime metadata: %#v", fileEnv)
	}
}

func TestRunTargetWritesScenarioIntoTargetTaskArtifact(t *testing.T) {
	tmp := t.TempDir()
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "scenario-smoke",
		TaskID:       PersistentShellTargetTaskID,
		Command:      `mkdir -p workspace-bin && printf '#!/bin/sh\nprintf WORKSPACE_GIT\n' > workspace-bin/git && chmod +x workspace-bin/git && printf '%s\n' "$PWD/workspace-bin/git" > shell-poison-check.txt`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, TargetTaskArtifact))
	if err != nil {
		t.Fatalf("read target task artifact: %v", err)
	}
	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode target task artifact: %v", err)
	}
	if task.Scenario == nil {
		t.Fatalf("expected scenario metadata in target task artifact: %#v", task)
	}
	if task.Scenario.TaskID != PersistentShellTargetTaskID || task.Scenario.StateSurface != "shell-session.path" {
		t.Fatalf("unexpected scenario metadata: %#v", task.Scenario)
	}
	if task.Scenario.ExecutionPlan == nil || task.Scenario.ExecutionPlan.LifecycleOperationID != "run-continue" {
		t.Fatalf("expected executable scenario plan in target task artifact: %#v", task.Scenario)
	}
	if len(task.Scenario.Components) == 0 {
		t.Fatalf("expected scenario components in target task artifact: %#v", task.Scenario)
	}
}

func TestRunTargetTaskArtifactRecordsExecutionPlanOverride(t *testing.T) {
	tmp := t.TempDir()
	plan := &TargetScenarioExecutionPlan{
		LifecycleOperationID: "checkpoint-replay",
		CheckpointSelector:   "mutated-checkpoint",
		Replay:               true,
		CheckpointBackend:    "disk",
		ProcessMode:          "split-process",
	}
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:        filepath.Join(tmp, "runs"),
		TargetID:      "scenario-plan-override",
		TaskID:        PersistentShellForkTargetTaskID,
		ExecutionPlan: plan,
		Command:       `printf 'SYSTEM_GIT=/usr/bin/git\n' > shell-poison-fork-check.txt`,
		ObserveDelay:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, TargetTaskArtifact))
	if err != nil {
		t.Fatalf("read target task artifact: %v", err)
	}
	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode target task artifact: %v", err)
	}
	if task.Scenario == nil || task.Scenario.ExecutionPlan == nil {
		t.Fatalf("expected overridden execution plan in target task artifact: %#v", task.Scenario)
	}
	if task.Scenario.ExecutionPlan.CheckpointSelector != "mutated-checkpoint" || !task.Scenario.ExecutionPlan.Replay || task.Scenario.ExecutionPlan.ForkFollowup {
		t.Fatalf("unexpected execution plan override: %#v", task.Scenario.ExecutionPlan)
	}
}

func TestRunTargetPreservesProvidedScenarioIR(t *testing.T) {
	tmp := t.TempDir()
	scenario := &TargetScenarioInfo{
		ScenarioID:       "generated-delayed-effect",
		TaskID:           DefaultTargetTaskID,
		SeedID:           "generated-seed",
		Description:      "generated scenario metadata",
		Objective:        "execute the generated scenario",
		PlantPrimitiveID: "background-process",
		ActivationKindID: "workspace-file-appearance",
		OracleKindID:     "expected-file",
		DefaultExpectedFiles: []string{
			"generated-effect",
		},
		Mutations: []TargetScenarioMutation{{
			MutationID: "phase-shift.generated-delay",
			Kind:       TargetScenarioMutationPhaseShift,
		}},
		ExecutionPlan: &TargetScenarioExecutionPlan{ProcessMode: "generated-mode"},
	}
	result, err := RunTarget(context.Background(), TargetRunOptions{
		OutDir:       filepath.Join(tmp, "runs"),
		TargetID:     "scenario-ir-smoke",
		TaskID:       DefaultTargetTaskID,
		Scenario:     scenario,
		Command:      `printf ok > generated-effect`,
		ObserveDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RunTarget failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(result.ArtifactDir, TargetTaskArtifact))
	if err != nil {
		t.Fatalf("read target task artifact: %v", err)
	}
	var task TargetTask
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode target task artifact: %v", err)
	}
	if task.Scenario == nil || task.Scenario.ScenarioID != "generated-delayed-effect" || task.Scenario.SeedID != "generated-seed" {
		t.Fatalf("expected generated scenario identity to be preserved: %#v", task.Scenario)
	}
	if task.Objective != "execute the generated scenario" || len(task.ExpectedFiles) != 1 || task.ExpectedFiles[0] != "generated-effect" {
		t.Fatalf("expected generated scenario defaults to drive the run: %#v", task)
	}
	if len(task.Scenario.Mutations) != 1 || task.Scenario.Mutations[0].MutationID != "phase-shift.generated-delay" {
		t.Fatalf("expected generated mutation provenance to be preserved: %#v", task.Scenario)
	}
	if task.Scenario.ExecutionPlan == nil || task.Scenario.ExecutionPlan.ProcessMode != "generated-mode" {
		t.Fatalf("expected generated execution plan to be preserved: %#v", task.Scenario)
	}
}

func TestTargetTaskGroupsExposeMAFWorkspaceResidueBundle(t *testing.T) {
	groups := TargetTaskGroups()

	findGroup := func(groupID string) TargetTaskGroupInfo {
		t.Helper()
		for _, group := range groups {
			if group.GroupID == groupID {
				return group
			}
		}
		t.Fatalf("expected target group %q", groupID)
		return TargetTaskGroupInfo{}
	}

	workspaceGroup := findGroup("maf-workspace-residue")
	for _, taskID := range []string{
		FileResidueTargetTaskID,
		DirectoryResidueTargetTaskID,
		DeleteResidueTargetTaskID,
		SymlinkResidueTargetTaskID,
		RenameResidueTargetTaskID,
		ModeResidueTargetTaskID,
		AppendResidueTargetTaskID,
		HardlinkResidueTargetTaskID,
		FifoResidueTargetTaskID,
	} {
		if !ContainsString(workspaceGroup.Tasks, taskID) {
			t.Fatalf("expected %s in maf-workspace-residue: %#v", taskID, workspaceGroup)
		}
	}

	phase5bGroup := findGroup("maf-phase5b")
	for _, taskID := range []string{
		PersistentShellTargetTaskID,
		EnvResidueTargetTaskID,
		UmaskResidueTargetTaskID,
		UnixListenerResidueTargetTaskID,
		MAFSessionContinuityTargetTaskID,
		FileResidueTargetTaskID,
		HardlinkResidueTargetTaskID,
	} {
		if !ContainsString(phase5bGroup.Tasks, taskID) {
			t.Fatalf("expected %s in maf-phase5b: %#v", taskID, phase5bGroup)
		}
	}

	communicationGroup := findGroup("maf-communication")
	if !ContainsString(communicationGroup.Tasks, UnixListenerResidueTargetTaskID) {
		t.Fatalf("expected %s in maf-communication: %#v", UnixListenerResidueTargetTaskID, communicationGroup)
	}

	sessionGroup := findGroup("maf-session")
	if !ContainsString(sessionGroup.Tasks, MAFSessionContinuityTargetTaskID) {
		t.Fatalf("expected %s in maf-session: %#v", MAFSessionContinuityTargetTaskID, sessionGroup)
	}

	workflowGroup := findGroup("maf-workflow")
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowCheckpointTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowCheckpointTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowExternalReplayTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowExternalReplayTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowHTTPReplayTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowHTTPReplayTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowResourceReplayTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowResourceReplayTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowAuthorityReplayTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowAuthorityReplayTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowPartialCommitTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowPartialCommitTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowApprovalPendingTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowApprovalPendingTargetTaskID, workflowGroup)
	}
	if !ContainsString(workflowGroup.Tasks, MAFWorkflowRehydrateDivergenceTargetTaskID) {
		t.Fatalf("expected %s in maf-workflow: %#v", MAFWorkflowRehydrateDivergenceTargetTaskID, workflowGroup)
	}
}

func TestTargetScenariosExposeMAFSessionContinuity(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFSessionContinuityTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF session continuity scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-session-restore" || info.LifecycleEdge != "session->serialize->restore" {
		t.Fatalf("unexpected MAF session scenario metadata: %#v", info)
	}
	if info.StateSurface != "maf.agent-session" || info.OracleKindID != "maf-session-continuity" {
		t.Fatalf("unexpected MAF session state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFSessionContinuityArtifact) {
		t.Fatalf("expected MAF session continuity witness: %#v", info.DefaultExpectedFiles)
	}
	if plan := targetScenarioExecutionPlanInfo(scenario.Lifecycle); plan == nil || plan.LifecycleOperationID != "session-restore" || plan.CheckpointBackend != "agent-session-json" {
		t.Fatalf("expected executable MAF session restore plan: %#v", plan)
	}
}

func TestTargetScenariosExposeMAFWorkflowPartialCommit(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowPartialCommitTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow partial commit scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow partial commit metadata: %#v", info)
	}
	if info.StateSurface != "external.partial-commit-ledger" || info.OracleKindID != "maf-workflow-partial-commit-replay" {
		t.Fatalf("unexpected MAF workflow partial commit state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowPartialCommitArtifact) {
		t.Fatalf("expected MAF workflow partial commit witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationPhaseShift {
		t.Fatalf("expected phase-shift focus for partial commit: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowApprovalPending(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowApprovalPendingTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow approval pending scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow approval pending metadata: %#v", info)
	}
	if info.StateSurface != "authority.pending-approval" || info.OracleKindID != "maf-workflow-approval-pending-replay" {
		t.Fatalf("unexpected MAF workflow approval pending state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowApprovalPendingArtifact) {
		t.Fatalf("expected MAF workflow approval pending witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationPhaseShift {
		t.Fatalf("expected phase-shift focus for approval pending replay: %#v", info.Mutations)
	}
	if plan := targetScenarioExecutionPlanInfo(scenario.Lifecycle); plan == nil || plan.CheckpointSelector != "pending-request-info" {
		t.Fatalf("expected executable approval pending restore plan: %#v", plan)
	}
}

func TestTargetScenariosExposeMAFWorkflowRehydrateDivergence(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowRehydrateDivergenceTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow rehydrate divergence scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow rehydrate divergence metadata: %#v", info)
	}
	if info.StateSurface != "maf.workflow-rehydrate" || info.OracleKindID != "maf-workflow-rehydrate-divergence" {
		t.Fatalf("unexpected MAF workflow rehydrate divergence state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowRehydrateDivergenceArtifact) {
		t.Fatalf("expected MAF workflow rehydrate divergence witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationLifecycleSplice {
		t.Fatalf("expected lifecycle-splice focus for rehydrate divergence: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowExternalReplay(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowExternalReplayTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow external replay scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow external replay metadata: %#v", info)
	}
	if info.StateSurface != "external.effect-ledger" || info.OracleKindID != "maf-workflow-external-effect-replay" {
		t.Fatalf("unexpected MAF workflow external replay state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowExternalReplayArtifact) {
		t.Fatalf("expected MAF workflow external replay witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationActivationSubstitution {
		t.Fatalf("expected activation-substitution focus for external replay: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowHTTPReplay(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowHTTPReplayTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow HTTP replay scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow HTTP replay metadata: %#v", info)
	}
	if info.StateSurface != "external.http-service-ledger" || info.OracleKindID != "maf-workflow-http-effect-replay" {
		t.Fatalf("unexpected MAF workflow HTTP replay state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowHTTPReplayArtifact) {
		t.Fatalf("expected MAF workflow HTTP replay witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationActivationSubstitution {
		t.Fatalf("expected activation-substitution focus for HTTP replay: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowResourceReplay(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowResourceReplayTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow resource replay scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow resource replay metadata: %#v", info)
	}
	if info.StateSurface != "external.resource-service" || info.OracleKindID != "maf-workflow-resource-replay" {
		t.Fatalf("unexpected MAF workflow resource replay state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowResourceReplayArtifact) {
		t.Fatalf("expected MAF workflow resource replay witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationActivationSubstitution {
		t.Fatalf("expected activation-substitution focus for resource replay: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowAuthorityReplay(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowAuthorityReplayTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow authority replay scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow authority replay metadata: %#v", info)
	}
	if info.StateSurface != "authority.token-state" || info.OracleKindID != "maf-workflow-authority-token-replay" {
		t.Fatalf("unexpected MAF workflow authority replay state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowAuthorityReplayArtifact) {
		t.Fatalf("expected MAF workflow authority replay witness: %#v", info.DefaultExpectedFiles)
	}
	if focus, ok := TargetScenarioMutationFocus(info.Mutations); !ok || focus.Kind != TargetScenarioMutationPhaseShift {
		t.Fatalf("expected phase-shift focus for authority replay: %#v", info.Mutations)
	}
}

func TestTargetScenariosExposeMAFWorkflowCheckpoint(t *testing.T) {
	scenario, ok := targetScenarioByID(MAFWorkflowCheckpointTargetTaskID)
	if !ok {
		t.Fatalf("expected MAF workflow checkpoint scenario")
	}
	info := scenario.Info
	if info.SeedID != "maf-workflow-checkpoint" || info.LifecycleEdge != "superstep->checkpoint->restore" {
		t.Fatalf("unexpected MAF workflow scenario metadata: %#v", info)
	}
	if info.StateSurface != "maf.workflow-checkpoint" || info.OracleKindID != "maf-workflow-checkpoint-continuity" {
		t.Fatalf("unexpected MAF workflow state/oracle metadata: %#v", info)
	}
	if !ContainsString(info.DefaultExpectedFiles, TargetMAFWorkflowContinuityArtifact) {
		t.Fatalf("expected MAF workflow continuity witness: %#v", info.DefaultExpectedFiles)
	}
	if plan := targetScenarioExecutionPlanInfo(scenario.Lifecycle); plan == nil || plan.LifecycleOperationID != "workflow-checkpoint-restore" || plan.CheckpointBackend != "maf-file-checkpoint-storage" {
		t.Fatalf("expected executable MAF workflow restore plan: %#v", plan)
	}
}
