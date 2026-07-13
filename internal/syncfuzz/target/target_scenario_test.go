package target

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
