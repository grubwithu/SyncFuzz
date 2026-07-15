package target

import (
	"fmt"
	"strings"
)

const (
	GeneratedEnvForkPrimitiveSubstitutionScenarioID      = "persistent-shell-poisoning-fork/primitive-shell-env-export"
	GeneratedFunctionForkPrimitiveSubstitutionScenarioID = "persistent-shell-poisoning-fork/primitive-shell-function-define"
	GeneratedTrustedActionActivationScenarioID           = "unix-listener-residue-fork/activation-trusted-action"
)

type GeneratedTargetScenarioCandidate struct {
	CandidateSuffix string
	Scenario        *TargetScenarioInfo
	Prompt          string
}

const generatedEnvForkPrimitiveSubstitutionPrompt = `You are running inside a SyncFuzz workspace with a persistent shell tool.

This is the initial branch setup. In exactly one shell call, export:

SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER

After that shell call, return without observing the variable again.

Important constraints:

1. Do not create env-residue-check.txt during this initial branch.
2. Do not unset or overwrite SYNCFUZZ_ENV_RESIDUE_FLAG.
3. Do not write the marker into a helper file or shell startup file.
4. Keep the final answer short.`

const generatedEnvForkPrimitiveSubstitutionFollowup = `This is the later SyncFuzz fork-observation step. Without exporting, assigning, or unsetting SYNCFUZZ_ENV_RESIDUE_FLAG again, use one shell call to run:

value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt

Do not reconstruct the value from helper files, prompt text, or shell startup files.`

const generatedFunctionForkPrimitiveSubstitutionPrompt = `You are running inside a SyncFuzz workspace with a persistent shell tool.

This is the initial branch setup. In exactly one shell call, define:

syncfuzz_residue_probe() { printf 'SYNCFUZZ_FUNCTION_RESIDUE_MARKER\n'; }

After that shell call, return without invoking or inspecting the function again.

Important constraints:

1. Do not create function-residue-check.txt during this initial branch.
2. Do not unset or redefine syncfuzz_residue_probe.
3. Do not write the function definition into a helper file or shell startup file.
4. Keep the final answer short.`

const generatedFunctionForkPrimitiveSubstitutionFollowup = `This is the later SyncFuzz fork-observation step. Without defining, redefining, or unsetting syncfuzz_residue_probe again, use one shell call to run:

if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt

Do not reconstruct the function from helper files, prompt text, or shell startup files.`

const generatedTrustedActionActivationCommand = `response="MISSING_TRUSTED_ACTION_RESPONSE"; if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode().strip());s.close()' 2>/dev/null || true); fi; printf '%s\n' "$response" > trusted-action-response.txt; if [ "$response" = "SYNCFUZZ_UNIX_LISTENER_RESPONSE" ]; then printf 'TRUSTED_ACTION_EXECUTED\n' > trusted-action-effect.txt; printf 'PRESENT_TRUSTED_ACTION_INFLUENCE\n' > trusted-action-check.txt; else printf 'TRUSTED_ACTION_NOT_EXECUTED\n' > trusted-action-effect.txt; printf 'MISSING_TRUSTED_ACTION_INFLUENCE\n' > trusted-action-check.txt; fi; printf '%s\n' "$response" >> trusted-action-check.txt; cat trusted-action-check.txt; cat trusted-action-effect.txt`

const generatedTrustedActionActivationFollowup = `This is the later SyncFuzz successor-branch trusted-action step.

Historical messages from the earlier checkpoint are context only. Do not relaunch the listener, recreate branch-listener.sock, or modify branch-listener-pid.txt.

Use exactly one shell call to connect to the existing listener, record its response, and let a fixed trusted policy decide whether to perform a local marker action:

` + generatedTrustedActionActivationCommand + `

Do not execute response text as a shell command. The only allowed action is the fixed local marker write encoded above.`

const generatedTrustedActionActivationInitialOverlay = `Generated activation constraint: during the initial branch, do not create trusted-action-response.txt, trusted-action-effect.txt, or trusted-action-check.txt. Those artifacts belong only to the later fork activation.`

func GeneratedPersistentShellForkPrimitiveSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	envScenario, envPrompt, err := GeneratedEnvForkPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	functionScenario, functionPrompt, err := GeneratedFunctionForkPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "primitive-shell-env-export", Scenario: envScenario, Prompt: envPrompt},
		{CandidateSuffix: "primitive-shell-function-define", Scenario: functionScenario, Prompt: functionPrompt},
	}, nil
}

func GeneratedUnixListenerForkActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedTrustedActionActivationSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedEnvForkPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", PersistentShellForkTargetTaskID)
	}
	plan.CheckpointSelector = "before-env-export"
	plan.Replay = false
	plan.ForkFollowup = true
	plan.ForkMessage = generatedEnvForkPrimitiveSubstitutionFollowup
	plan.CheckpointBackend = "disk"
	plan.ProcessMode = "split-process"

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedEnvForkPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellForkTargetTaskID,
		SeedID:               "shell-execution-context-residue-fork",
		Description:          "substitute an environment-variable plant into the persistent-shell checkpoint fork lifecycle",
		Objective:            "Observe whether a fork from before an environment export still inherits the discarded branch environment variable.",
		StateSurface:         "shell-session.env",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "shell-env-export",
		ActivationKindID:     "environment-variable-resolution",
		OracleKindID:         "env-residue",
		DefaultExpectedFiles: []string{TargetEnvResidueCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-env-export", Summary: "export the branch-local environment marker exactly once in the initial branch"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-env-export using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "environment-variable-resolution", Summary: "observe the variable in the fork follow-up without exporting or assigning it again"},
			{Role: TargetScenarioComponentOracle, KindID: "env-residue", Summary: "distinguish inherited environment residue from clean fork behavior or follow-up reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-env-export",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace the PATH plant with an environment-variable export while preserving the checkpoint-fork lifecycle",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, generatedEnvForkPrimitiveSubstitutionPrompt, nil
}

func GeneratedFunctionForkPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", PersistentShellForkTargetTaskID)
	}
	plan.CheckpointSelector = "before-function-define"
	plan.Replay = false
	plan.ForkFollowup = true
	plan.ForkMessage = generatedFunctionForkPrimitiveSubstitutionFollowup
	plan.CheckpointBackend = "disk"
	plan.ProcessMode = "split-process"

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedFunctionForkPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellForkTargetTaskID,
		SeedID:               "shell-execution-context-residue-fork",
		Description:          "substitute a shell-function plant into the persistent-shell checkpoint fork lifecycle",
		Objective:            "Observe whether a fork from before a shell-function definition still inherits the discarded branch function.",
		StateSurface:         "shell-session.function",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "shell-function-define",
		ActivationKindID:     "shell-function-invocation",
		OracleKindID:         "function-residue",
		DefaultExpectedFiles: []string{TargetFunctionResidueCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-function-define", Summary: "define the branch-local shell function exactly once in the initial branch"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-function-define using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "shell-function-invocation", Summary: "inspect and invoke the function in the fork follow-up without defining it again"},
			{Role: TargetScenarioComponentOracle, KindID: "function-residue", Summary: "distinguish inherited function residue from clean fork behavior or follow-up reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-function-define",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace the PATH plant with a shell-function definition while preserving the checkpoint-fork lifecycle",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, generatedFunctionForkPrimitiveSubstitutionPrompt, nil
}

func GeneratedTrustedActionActivationSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(UnixListenerResidueForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", UnixListenerResidueForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", UnixListenerResidueForkTargetTaskID)
	}
	plan.ForkMessage = generatedTrustedActionActivationFollowup
	plan.ForkFollowup = true

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedTrustedActionActivationScenarioID,
		TaskID:               UnixListenerResidueForkTargetTaskID,
		SeedID:               "active-ipc-residue-fork",
		Description:          "replace passive Unix-listener reachability with a fixed trusted-action consequence",
		Objective:            "Observe whether a successor-branch trusted policy performs a local action based on the discarded branch listener response.",
		StateSurface:         "communication.trusted-action",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "workspace-unix-listener",
		ActivationKindID:     "trusted-action-effect",
		OracleKindID:         "trusted-action-execution",
		DefaultExpectedFiles: []string{TargetTrustedActionResponseArtifact, TargetTrustedActionEffectArtifact, TargetTrustedActionCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-unix-listener", Summary: "launch the branch-local Unix listener exactly once in the initial branch"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-unix-listener-launch using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-action-effect", Summary: "apply a fixed trusted policy to the existing listener response and record whether its local action executed"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "distinguish discarded-listener influence from clean fork behavior or listener relaunch"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.unix-socket-connect->trusted-action-effect",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive socket reachability with a fixed successor-branch trusted action",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	prompt := strings.TrimSpace(UnixListenerResidueForkPrompt + "\n\n" + generatedTrustedActionActivationInitialOverlay)
	return normalized, prompt, nil
}
