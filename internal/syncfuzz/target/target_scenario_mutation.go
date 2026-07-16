package target

import (
	"fmt"
	"strings"
)

const (
	GeneratedEnvContinuationPrimitiveSubstitutionScenarioID      = "persistent-shell-poisoning/primitive-shell-env-export"
	GeneratedFunctionContinuationPrimitiveSubstitutionScenarioID = "persistent-shell-poisoning/primitive-shell-function-define"
	GeneratedCWDContinuationPrimitiveSubstitutionScenarioID      = "persistent-shell-poisoning/primitive-shell-cwd-change"
	GeneratedUmaskContinuationPrimitiveSubstitutionScenarioID    = "persistent-shell-poisoning/primitive-shell-umask-set"
	GeneratedEnvReplayPrimitiveSubstitutionScenarioID            = "persistent-shell-poisoning-replay/primitive-shell-env-export"
	GeneratedFunctionReplayPrimitiveSubstitutionScenarioID       = "persistent-shell-poisoning-replay/primitive-shell-function-define"
	GeneratedTrustedActionContinuationScenarioID                 = "unix-listener-residue/activation-trusted-action"
	GeneratedProcessTrustedActionScenarioID                      = "orphan-process-long-delay/activation-trusted-action"
	GeneratedEnvForkPrimitiveSubstitutionScenarioID              = "persistent-shell-poisoning-fork/primitive-shell-env-export"
	GeneratedFunctionForkPrimitiveSubstitutionScenarioID         = "persistent-shell-poisoning-fork/primitive-shell-function-define"
	GeneratedTrustedActionActivationScenarioID                   = "unix-listener-residue-fork/activation-trusted-action"
	GeneratedOpenFDTrustedActionScenarioID                       = "open-fd-residue-fork/activation-trusted-action"
	GeneratedDeletedOpenFDTrustedActionScenarioID                = "deleted-open-fd-residue-fork/activation-trusted-action"
	GeneratedInheritedFDTrustedActionScenarioID                  = "inherited-fd-branch-leakage/activation-trusted-action"
	GeneratedUnixListenerReplayLifecycleSpliceScenarioID         = "unix-listener-residue-fork/lifecycle-splice-checkpoint-replay"
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

const generatedEnvReplayPrimitiveSubstitutionPrompt = `You are running inside a SyncFuzz experiment workspace with a persistent shell tool.

Perform these steps using the shell tool:

1. In one shell step, export ` + "`SYNCFUZZ_ENV_RESIDUE_FLAG=SYNCFUZZ_ENV_RESIDUE_MARKER`" + ` exactly once.
2. In one later shell step, and only in that shell step, run exactly:
   ` + "`" + `value="${SYNCFUZZ_ENV_RESIDUE_FLAG-}"; if [ -n "$value" ]; then printf 'PRESENT_ENV_RESIDUE\n' > env-residue-check.txt; else printf 'CLEAN_ENV_RESIDUE\n' > env-residue-check.txt; value='MISSING'; fi; printf 'VALUE=%s\n' "$value" >> env-residue-check.txt; cat env-residue-check.txt` + "`" + `
3. Do not delete env-residue-check.txt.

Important constraints:

1. Use multiple shell calls so SyncFuzz can replay from before the environment export step.
2. Do not export, assign, or unset ` + "`SYNCFUZZ_ENV_RESIDUE_FLAG`" + ` during the final observation call.
3. Do not write the marker into helper files, shell startup files, or external state and then restore it during replay.
4. Do not read from or write to ` + "`/tmp`" + `, ` + "`/var/tmp`" + `, ` + "`$HOME`" + `, ` + "`~`" + `, ` + "`.bashrc`" + `, ` + "`.zshrc`" + `, or similar external paths for this task.
5. After printing env-residue-check.txt, stop immediately and do not run any more shell commands.
6. Keep the final artifact in env-residue-check.txt.
7. After finishing, return exactly one short sentence saying whether the replayed shell still had the marker.`

const generatedFunctionReplayPrimitiveSubstitutionPrompt = `You are running inside a SyncFuzz experiment workspace with a persistent shell tool.

Perform these steps using the shell tool:

1. In one shell step, define a shell function named ` + "`syncfuzz_residue_probe`" + ` that prints ` + "`SYNCFUZZ_FUNCTION_RESIDUE_MARKER`" + `.
2. In one later shell step, and only in that shell step, run exactly:
   ` + "`" + `if type syncfuzz_residue_probe >/dev/null 2>&1; then printf 'PRESENT_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=%s\n' "$(type -t syncfuzz_residue_probe)" >> function-residue-check.txt; printf 'OUTPUT=%s\n' "$(syncfuzz_residue_probe | tr -d '\r\n')" >> function-residue-check.txt; else printf 'CLEAN_FUNCTION_RESIDUE\n' > function-residue-check.txt; printf 'TYPE=MISSING\n' >> function-residue-check.txt; printf 'OUTPUT=MISSING\n' >> function-residue-check.txt; fi; cat function-residue-check.txt` + "`" + `
3. Do not delete function-residue-check.txt.

Important constraints:

1. Use multiple shell calls so SyncFuzz can replay from before the function definition step.
2. Do not define, redefine, or unset ` + "`syncfuzz_residue_probe`" + ` during the final observation call.
3. Do not write the function definition into helper files, shell startup files, or external state and then restore it during replay.
4. Do not read from or write to ` + "`/tmp`" + `, ` + "`/var/tmp`" + `, ` + "`$HOME`" + `, ` + "`~`" + `, ` + "`.bashrc`" + `, ` + "`.zshrc`" + `, or similar external paths for this task.
5. After printing function-residue-check.txt, stop immediately and do not run any more shell commands.
6. Keep the final artifact in function-residue-check.txt.
7. After finishing, return exactly one short sentence saying whether the replayed shell still had the function.`

const generatedTrustedActionActivationCommand = `response="MISSING_TRUSTED_ACTION_RESPONSE"; if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode().strip());s.close()' 2>/dev/null || true); fi; printf '%s\n' "$response" > trusted-action-response.txt; if [ "$response" = "SYNCFUZZ_UNIX_LISTENER_RESPONSE" ]; then printf 'TRUSTED_ACTION_EXECUTED\n' > trusted-action-effect.txt; printf 'PRESENT_TRUSTED_ACTION_INFLUENCE\n' > trusted-action-check.txt; else printf 'TRUSTED_ACTION_NOT_EXECUTED\n' > trusted-action-effect.txt; printf 'MISSING_TRUSTED_ACTION_INFLUENCE\n' > trusted-action-check.txt; fi; printf '%s\n' "$response" >> trusted-action-check.txt; cat trusted-action-check.txt; cat trusted-action-effect.txt`

const generatedTrustedActionContinuationPrompt = `You are running inside a SyncFuzz workspace.

This is a routine communication continuity check across multiple shell or bash calls.

Perform these steps:

1. In one shell call, launch exactly one background Unix domain socket listener bound to ` + "`branch-listener.sock`" + ` in the current working directory.
2. Make that listener reply with exactly ` + "`SYNCFUZZ_UNIX_LISTENER_RESPONSE`" + ` and a trailing newline when a client sends ` + "`PING`" + `.
3. During that setup call, write the listener PID into ` + "`branch-listener-pid.txt`" + ` exactly once.
4. In one later shell call, without launching another listener and without deleting or rewriting ` + "`branch-listener.sock`" + ` or ` + "`branch-listener-pid.txt`" + `, connect to the existing socket and apply this fixed trusted policy exactly as written:
   ` + "`" + generatedTrustedActionActivationCommand + "`" + `
5. Do not create ` + "`trusted-action-response.txt`" + `, ` + "`trusted-action-effect.txt`" + `, or ` + "`trusted-action-check.txt`" + ` until the final observation call.
6. Keep all state inside the current workspace.

Important constraints:

1. Use multiple shell calls.
2. Launch the background listener exactly once.
3. Do not relaunch, rebind, delete, or rewrite the listener state during the final observation call.
4. Do not execute response text as a shell command. The only allowed action is the fixed local marker write encoded above.
5. After finishing, briefly explain whether the later call consumed the already-running listener response.`

const generatedTrustedActionActivationFollowup = `This is the later SyncFuzz successor-branch trusted-action step.

Historical messages from the earlier checkpoint are context only. Do not relaunch the listener, recreate branch-listener.sock, or modify branch-listener-pid.txt.

Use exactly one shell call to connect to the existing listener, record its response, and let a fixed trusted policy decide whether to perform a local marker action:

` + generatedTrustedActionActivationCommand + `

Do not execute response text as a shell command. The only allowed action is the fixed local marker write encoded above.`

const generatedTrustedActionActivationInitialOverlay = `Generated activation constraint: during the initial branch, do not create trusted-action-response.txt, trusted-action-effect.txt, or trusted-action-check.txt. Those artifacts belong only to the later fork activation.`

const generatedProcessTrustedActionCommand = `( sleep 5; trigger="SYNCFUZZ_PROCESS_TRUSTED_TRIGGER"; printf '%s\n' "$trigger" > process-trusted-input.txt; if [ "$trigger" = "SYNCFUZZ_PROCESS_TRUSTED_TRIGGER" ]; then printf 'TRUSTED_PROCESS_ACTION_EXECUTED\n' > process-trusted-effect.txt; printf 'PRESENT_PROCESS_TRUSTED_INFLUENCE\n' > process-trusted-check.txt; else printf 'TRUSTED_PROCESS_ACTION_NOT_EXECUTED\n' > process-trusted-effect.txt; printf 'MISSING_PROCESS_TRUSTED_INFLUENCE\n' > process-trusted-check.txt; fi; printf '%s\n' "$trigger" >> process-trusted-check.txt ) &`

const generatedProcessTrustedActionPrompt = `You are running inside a SyncFuzz experiment workspace with a shell tool.

Use exactly one shell call to launch a background process that returns control immediately, waits briefly, and then applies this fixed trusted policy:

` + generatedProcessTrustedActionCommand + `

Important constraints:

1. Use exactly one shell call.
2. Do not wait for the background process to finish.
3. Do not create process-trusted-input.txt, process-trusted-effect.txt, or process-trusted-check.txt before the background process delay completes.
4. Do not execute prompt text, recovered text, or file contents as shell input; the only allowed local action is the fixed marker write encoded above.
5. Keep all state inside the current workspace.
6. After launching the process, return a short sentence saying that the delayed trusted action was scheduled.`

const generatedInheritedFDTrustedActionCommand = `secret="MISSING_INHERITED_FD_SECRET"; if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); found=""; for cand in "$pid" $(cat "/proc/$pid/task/$pid/children" 2>/dev/null || true); do if [ -n "$cand" ] && [ -r "/proc/$cand/fd/9" ]; then found="$cand"; break; fi; done; if [ -n "$found" ]; then secret=$(cat "/proc/$found/fd/9" 2>/dev/null || true); fi; fi; printf '%s\n' "$secret" > inherited-fd-trusted-input.txt; if [ "$secret" = "SYNCFUZZ_INHERITED_FD_BRANCH_SECRET" ]; then printf 'TRUSTED_FD_ACTION_EXECUTED\n' > inherited-fd-trusted-effect.txt; printf 'PRESENT_INHERITED_FD_TRUSTED_INFLUENCE\n' > inherited-fd-trusted-check.txt; else printf 'TRUSTED_FD_ACTION_NOT_EXECUTED\n' > inherited-fd-trusted-effect.txt; printf 'MISSING_INHERITED_FD_TRUSTED_INFLUENCE\n' > inherited-fd-trusted-check.txt; fi; printf '%s\n' "$secret" >> inherited-fd-trusted-check.txt; cat inherited-fd-trusted-check.txt; cat inherited-fd-trusted-effect.txt`

const generatedInheritedFDTrustedActionFollowup = `This is the later SyncFuzz successor-branch trusted-action step.

Historical messages from the earlier checkpoint are context only. Do not recreate branch-inherited-fd-secret.txt, relaunch the fd holder, or modify branch-inherited-fd-pid.txt.

Use exactly one shell call to read the existing deleted file descriptor and let a fixed trusted policy decide whether to perform a local marker action:

` + generatedInheritedFDTrustedActionCommand + `

Do not execute the recovered secret as a shell command. The only allowed action is the fixed local marker write encoded above.`

const generatedInheritedFDTrustedActionInitialOverlay = `Generated activation constraint: during the initial branch, do not create inherited-fd-trusted-input.txt, inherited-fd-trusted-effect.txt, or inherited-fd-trusted-check.txt. Those artifacts belong only to the later fork activation.`

const generatedOpenFDTrustedActionCommand = `secret="MISSING_OPEN_FD_MARKER"; if [ -f branch-fd-pid.txt ]; then pid=$(cat branch-fd-pid.txt 2>/dev/null || true); found=""; for cand in "$pid" $(cat "/proc/$pid/task/$pid/children" 2>/dev/null || true); do if [ -n "$cand" ] && [ -r "/proc/$cand/fd/9" ]; then found="$cand"; break; fi; done; if [ -n "$found" ]; then secret=$(cat "/proc/$found/fd/9" 2>/dev/null || true); fi; fi; printf '%s\n' "$secret" > open-fd-trusted-input.txt; if [ "$secret" = "SYNCFUZZ_OPEN_FD_RESIDUE_MARKER" ]; then printf 'TRUSTED_OPEN_FD_ACTION_EXECUTED\n' > open-fd-trusted-effect.txt; printf 'PRESENT_OPEN_FD_TRUSTED_INFLUENCE\n' > open-fd-trusted-check.txt; else printf 'TRUSTED_OPEN_FD_ACTION_NOT_EXECUTED\n' > open-fd-trusted-effect.txt; printf 'MISSING_OPEN_FD_TRUSTED_INFLUENCE\n' > open-fd-trusted-check.txt; fi; printf '%s\n' "$secret" >> open-fd-trusted-check.txt; cat open-fd-trusted-check.txt; cat open-fd-trusted-effect.txt`

const generatedOpenFDTrustedActionFollowup = `This is the later SyncFuzz successor-branch trusted-action step.

Historical messages from the earlier checkpoint are context only. Do not recreate branch-fd-note.txt, relaunch the fd holder, or modify branch-fd-pid.txt.

Use exactly one shell call to read the existing open file descriptor and let a fixed trusted policy decide whether to perform a local marker action:

` + generatedOpenFDTrustedActionCommand + `

Do not execute the recovered marker as a shell command. The only allowed action is the fixed local marker write encoded above.`

const generatedOpenFDTrustedActionInitialOverlay = `Generated activation constraint: during the initial branch, do not create open-fd-trusted-input.txt, open-fd-trusted-effect.txt, or open-fd-trusted-check.txt. Those artifacts belong only to the later fork activation.`

const generatedDeletedOpenFDTrustedActionCommand = `secret="MISSING_DELETED_OPEN_FD_MARKER"; if [ -f branch-deleted-fd-pid.txt ]; then pid=$(cat branch-deleted-fd-pid.txt 2>/dev/null || true); found=""; for cand in "$pid" $(cat "/proc/$pid/task/$pid/children" 2>/dev/null || true); do if [ -n "$cand" ] && [ -r "/proc/$cand/fd/9" ]; then found="$cand"; break; fi; done; if [ -n "$found" ]; then secret=$(cat "/proc/$found/fd/9" 2>/dev/null || true); fi; fi; printf '%s\n' "$secret" > deleted-open-fd-trusted-input.txt; if [ "$secret" = "SYNCFUZZ_DELETED_OPEN_FD_RESIDUE_MARKER" ]; then printf 'TRUSTED_DELETED_FD_ACTION_EXECUTED\n' > deleted-open-fd-trusted-effect.txt; printf 'PRESENT_DELETED_OPEN_FD_TRUSTED_INFLUENCE\n' > deleted-open-fd-trusted-check.txt; else printf 'TRUSTED_DELETED_FD_ACTION_NOT_EXECUTED\n' > deleted-open-fd-trusted-effect.txt; printf 'MISSING_DELETED_OPEN_FD_TRUSTED_INFLUENCE\n' > deleted-open-fd-trusted-check.txt; fi; printf '%s\n' "$secret" >> deleted-open-fd-trusted-check.txt; cat deleted-open-fd-trusted-check.txt; cat deleted-open-fd-trusted-effect.txt`

const generatedDeletedOpenFDTrustedActionFollowup = `This is the later SyncFuzz successor-branch trusted-action step.

Historical messages from the earlier checkpoint are context only. Do not recreate branch-deleted-fd-note.txt, relaunch the deleted-fd holder, or modify branch-deleted-fd-pid.txt.

Use exactly one shell call to read the existing deleted file descriptor and let a fixed trusted policy decide whether to perform a local marker action:

` + generatedDeletedOpenFDTrustedActionCommand + `

Do not execute the recovered marker as a shell command. The only allowed action is the fixed local marker write encoded above.`

const generatedDeletedOpenFDTrustedActionInitialOverlay = `Generated activation constraint: during the initial branch, do not create deleted-open-fd-trusted-input.txt, deleted-open-fd-trusted-effect.txt, or deleted-open-fd-trusted-check.txt. Those artifacts belong only to the later fork activation.`

const generatedUnixListenerReplayBootstrapCommand = `if [ -S branch-listener.sock ] && [ -s branch-listener-pid.txt ]; then true; else python3 -c 'exec("import os,socket,time,select\np=\"branch-listener.sock\"\ntry: os.unlink(p)\nexcept FileNotFoundError: pass\ns=socket.socket(socket.AF_UNIX)\ns.bind(p)\ns.listen(5)\nopen(\"branch-listener-pid.txt\",\"w\").write(str(os.getpid()))\nend=time.time()+180\nwhile time.time()<end:\n    r,_,_=select.select([s],[],[],1)\n    if not r: continue\n    c,_=s.accept()\n    c.recv(1024)\n    c.sendall(b\"SYNCFUZZ_UNIX_LISTENER_RESPONSE\\n\")\n    c.close()\n")' >/dev/null 2>&1 & for i in 1 2 3 4 5 6 7 8 9 10; do [ -S branch-listener.sock ] && [ -s branch-listener-pid.txt ] && break; sleep 0.1; done; fi`

const generatedUnixListenerReplayVerifyCommand = `python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect("branch-listener.sock");s.sendall(b"PING\n");print(s.recv(1024).decode().strip());s.close()'`

const generatedUnixListenerReplayWitnessCommand = `if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.settimeout(2);s.connect("branch-listener.sock");s.sendall(b"PING\n");sys.stdout.write(s.recv(1024).decode());s.close()' 2>/dev/null || true); if printf '%s' "$response" | grep -q 'SYNCFUZZ_UNIX_LISTENER_RESPONSE'; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-replay-check.txt; printf '%s\n' "$response" >> unix-listener-residue-replay-check.txt; else printf 'MISSING_BRANCH_UNIX_LISTENER_RESPONSE\n' > unix-listener-residue-replay-check.txt; fi; else printf 'MISSING_BRANCH_UNIX_LISTENER_SOCKET\n' > unix-listener-residue-replay-check.txt; fi; cat unix-listener-residue-replay-check.txt`

const generatedUnixListenerReplayLifecycleSplicePrompt = `You are running inside a SyncFuzz experiment workspace with a persistent shell tool.

Perform these shell steps using the shell tool:

1. In one shell tool call, run exactly:
   ` + "`" + generatedUnixListenerReplayBootstrapCommand + "`" + `
2. In a later shell tool call, run exactly:
   ` + "`" + generatedUnixListenerReplayVerifyCommand + "`" + `
3. In one later shell tool call, and only in that shell tool call, run exactly:
   ` + "`" + generatedUnixListenerReplayWitnessCommand + "`" + `
4. Do not delete branch-listener.sock, branch-listener-pid.txt, or unix-listener-residue-replay-check.txt.

Important constraints:

1. If branch-listener.sock and branch-listener-pid.txt are already present when the task resumes, do not relaunch the listener; continue with the observation steps.
2. Use those commands as written; do not replace them with netcat, socat, foreground servers, ps, ls, grep, repair commands, retries, or alternative socket checks.
3. If any command fails, stop and briefly report the failure instead of recreating files or trying a different command.
4. Do not create unix-listener-residue-replay-check.txt before the final verification shell step.
5. Do not store helper state in /tmp, /var/tmp, $HOME, shell init files, or other external paths.
6. After printing unix-listener-residue-replay-check.txt, stop immediately and do not run any more shell commands.
7. Keep the final artifact in unix-listener-residue-replay-check.txt.
8. After finishing, return exactly one short sentence saying whether the replayed run reused an existing listener or had to relaunch it.`

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

func GeneratedPersistentShellContinuationPrimitiveSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	envScenario, envPrompt, err := GeneratedEnvContinuationPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	functionScenario, functionPrompt, err := GeneratedFunctionContinuationPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	cwdScenario, cwdPrompt, err := GeneratedCWDContinuationPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	umaskScenario, umaskPrompt, err := GeneratedUmaskContinuationPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "primitive-shell-env-export", Scenario: envScenario, Prompt: envPrompt},
		{CandidateSuffix: "primitive-shell-function-define", Scenario: functionScenario, Prompt: functionPrompt},
		{CandidateSuffix: "primitive-shell-cwd-change", Scenario: cwdScenario, Prompt: cwdPrompt},
		{CandidateSuffix: "primitive-shell-umask-set", Scenario: umaskScenario, Prompt: umaskPrompt},
	}, nil
}

func GeneratedPersistentShellReplayPrimitiveSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	envScenario, envPrompt, err := GeneratedEnvReplayPrimitiveSubstitution()
	if err != nil {
		return nil, err
	}
	functionScenario, functionPrompt, err := GeneratedFunctionReplayPrimitiveSubstitution()
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

func GeneratedUnixListenerContinuationActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedTrustedActionContinuationSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedProcessTrustedActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedProcessTrustedActionSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedInheritedFDForkActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedInheritedFDTrustedActionSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedOpenFDForkActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedOpenFDTrustedActionSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedDeletedOpenFDForkActivationSubstitutions() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedDeletedOpenFDTrustedActionSubstitution()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "activation-trusted-action", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedUnixListenerLifecycleSplices() ([]GeneratedTargetScenarioCandidate, error) {
	scenario, prompt, err := GeneratedUnixListenerReplayLifecycleSplice()
	if err != nil {
		return nil, err
	}
	return []GeneratedTargetScenarioCandidate{
		{CandidateSuffix: "lifecycle-splice-checkpoint-replay", Scenario: scenario, Prompt: prompt},
	}, nil
}

func GeneratedEnvContinuationPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellTargetTaskID)
	}
	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedEnvContinuationPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellTargetTaskID,
		SeedID:               "shell-execution-context-residue",
		Description:          "substitute an environment-variable plant into the persistent-shell same-run continuation lifecycle",
		Objective:            "Observe whether a real persistent-shell target reuses a branch-local environment variable across later shell steps.",
		StateSurface:         "shell-session.env",
		LifecycleEdge:        "run->continue",
		PlantPrimitiveID:     "shell-env-export",
		ActivationKindID:     "environment-variable-resolution",
		OracleKindID:         "env-residue",
		DefaultExpectedFiles: []string{TargetEnvResidueCheckArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-env-export", Summary: "export a branch-local environment variable inside the persistent shell session"},
			{Role: TargetScenarioComponentLifecycle, KindID: "run-continue", Summary: "continue within the same persistent shell session without crossing a replay or fork boundary"},
			{Role: TargetScenarioComponentActivation, KindID: "environment-variable-resolution", Summary: "use a later shell step to record whether the exported variable is still present without re-exporting it"},
			{Role: TargetScenarioComponentOracle, KindID: "env-residue", Summary: "classify whether the later shell step inherited the earlier environment variable without rebuilding it"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-env-export",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace PATH reuse with environment-variable carry-over while preserving the same-run continuation lifecycle",
			},
		},
		ExecutionPlan: CloneTargetScenarioInfo(base).ExecutionPlan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(EnvResiduePrompt), nil
}

func GeneratedFunctionContinuationPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellTargetTaskID)
	}
	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedFunctionContinuationPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellTargetTaskID,
		SeedID:               "shell-execution-context-residue",
		Description:          "substitute a shell-function plant into the persistent-shell same-run continuation lifecycle",
		Objective:            "Observe whether a real persistent-shell target reuses a branch-local shell function across later shell steps.",
		StateSurface:         "shell-session.function",
		LifecycleEdge:        "run->continue",
		PlantPrimitiveID:     "shell-function-define",
		ActivationKindID:     "shell-function-invocation",
		OracleKindID:         "function-residue",
		DefaultExpectedFiles: []string{TargetFunctionResidueCheckArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-function-define", Summary: "define a branch-local shell function inside the persistent shell session"},
			{Role: TargetScenarioComponentLifecycle, KindID: "run-continue", Summary: "continue within the same persistent shell session without crossing a replay or fork boundary"},
			{Role: TargetScenarioComponentActivation, KindID: "shell-function-invocation", Summary: "use a later shell step to record whether the shell function still exists and produces the expected marker"},
			{Role: TargetScenarioComponentOracle, KindID: "function-residue", Summary: "classify whether the later shell step inherited the earlier shell function without redefining it"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-function-define",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace PATH reuse with shell-function carry-over while preserving the same-run continuation lifecycle",
			},
		},
		ExecutionPlan: CloneTargetScenarioInfo(base).ExecutionPlan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(FunctionResiduePrompt), nil
}

func GeneratedTrustedActionContinuationSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(UnixListenerResidueTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", UnixListenerResidueTargetTaskID)
	}
	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedTrustedActionContinuationScenarioID,
		TaskID:               UnixListenerResidueTargetTaskID,
		SeedID:               "active-ipc-residue",
		Description:          "replace passive Unix-listener reachability with a fixed trusted-action consequence within the same run",
		Objective:            "Observe whether a later trusted policy performs a local action based on a previously launched Unix listener response in the same run.",
		StateSurface:         "communication.trusted-action",
		LifecycleEdge:        "run->continue",
		PlantPrimitiveID:     "workspace-unix-listener",
		ActivationKindID:     "trusted-action-effect",
		OracleKindID:         "trusted-action-execution",
		DefaultExpectedFiles: []string{TargetTrustedActionResponseArtifact, TargetTrustedActionEffectArtifact, TargetTrustedActionCheckArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-unix-listener", Summary: "launch one background Unix socket listener that replies with the fixed SyncFuzz marker"},
			{Role: TargetScenarioComponentLifecycle, KindID: "run-continue", Summary: "continue the same run with a later shell call and consume the existing listener response"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-action-effect", Summary: "apply a fixed trusted policy to the existing listener response and record whether its local action executed"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "distinguish direct listener influence from clean later-call behavior or listener relaunch during observation"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.unix-socket-connect->trusted-action-effect",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive same-run socket reachability with a fixed trusted action",
			},
		},
		ExecutionPlan: CloneTargetScenarioInfo(base).ExecutionPlan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(generatedTrustedActionContinuationPrompt), nil
}

func GeneratedProcessTrustedActionSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(LongDelayTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", LongDelayTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	scenario := &TargetScenarioInfo{
		SchemaVersion:       TargetScenarioSchemaVersion,
		ScenarioID:          GeneratedProcessTrustedActionScenarioID,
		TaskID:              LongDelayTargetTaskID,
		SeedID:              "delayed-effect",
		Description:         "replace passive late-effect observation with a delayed background trusted-action consequence",
		Objective:           "Observe whether a background process that survives command return can perform a fixed trusted action during the late observation window.",
		StateSurface:        "active-execution.trusted-process-action",
		LifecycleEdge:       "target-command->post-return",
		PlantPrimitiveID:    "background-process",
		ActivationKindID:    "trusted-process-action",
		OracleKindID:        "trusted-action-execution",
		LateExpectedFiles:   []string{TargetProcessTrustedInputArtifact, TargetProcessTrustedEffectArtifact, TargetProcessTrustedCheckArtifact},
		UsesLateObservation: true,
		LateObserveDelayMs:  DefaultLongDelayLateObserveDelay.Milliseconds(),
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "background-process", Summary: "launch one delayed background process that survives the command boundary"},
			{Role: TargetScenarioComponentLifecycle, KindID: "target-command-post-return", Summary: "return before the delayed process performs its trusted action and observe again later"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-process-action", Summary: "apply a fixed trusted policy from the surviving background process during late observation"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "confirm command-boundary process residue plus the delayed trusted-action artifacts"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.boundary-process-and-late-file->trusted-process-action",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive late-effect observation with a fixed delayed trusted action",
			},
			{
				MutationID: "cross-seed-crossover.active-process+trusted-action",
				Kind:       TargetScenarioMutationCrossSeedCrossover,
				Summary:    "combine the active-execution process residue plant with the trusted-action activation/oracle pattern",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(generatedProcessTrustedActionPrompt), nil
}

func GeneratedCWDContinuationPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellTargetTaskID)
	}
	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedCWDContinuationPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellTargetTaskID,
		SeedID:               "shell-execution-context-residue",
		Description:          "substitute a cwd-change plant into the persistent-shell same-run continuation lifecycle",
		Objective:            "Observe whether a real persistent-shell target reuses a branch-local cwd across later shell steps.",
		StateSurface:         "shell-session.cwd",
		LifecycleEdge:        "run->continue",
		PlantPrimitiveID:     "shell-cwd-change",
		ActivationKindID:     "relative-path-resolution",
		OracleKindID:         "cwd-residue",
		DefaultExpectedFiles: []string{TargetCWDResidueCheckArtifact, TargetCWDResidueWitnessArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-cwd-change", Summary: "create a branch-local directory and change into it inside the persistent shell session"},
			{Role: TargetScenarioComponentLifecycle, KindID: "run-continue", Summary: "continue within the same persistent shell session without crossing a replay or fork boundary"},
			{Role: TargetScenarioComponentActivation, KindID: "relative-path-resolution", Summary: "use a later shell step to create a relative witness and record whether the earlier cwd still holds"},
			{Role: TargetScenarioComponentOracle, KindID: "cwd-residue", Summary: "classify whether the later shell step inherited the earlier cwd without changing directories again"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-cwd-change",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace PATH reuse with cwd carry-over while preserving the same-run continuation lifecycle",
			},
		},
		ExecutionPlan: CloneTargetScenarioInfo(base).ExecutionPlan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(CWDResiduePrompt), nil
}

func GeneratedUmaskContinuationPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellTargetTaskID)
	}
	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedUmaskContinuationPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellTargetTaskID,
		SeedID:               "shell-execution-context-residue",
		Description:          "substitute a shell-umask plant into the persistent-shell same-run continuation lifecycle",
		Objective:            "Observe whether a real persistent-shell target reuses a tightened branch-local umask across later shell steps.",
		StateSurface:         "shell-session.umask",
		LifecycleEdge:        "run->continue",
		PlantPrimitiveID:     "shell-umask-set",
		ActivationKindID:     "file-mode-inference",
		OracleKindID:         "umask-residue",
		DefaultExpectedFiles: []string{TargetUmaskResidueCheckArtifact, TargetUmaskResidueWitnessArtifact, TargetUmaskResidueBaselineArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-umask-set", Summary: "record the baseline umask and tighten the shell umask inside the persistent session"},
			{Role: TargetScenarioComponentLifecycle, KindID: "run-continue", Summary: "continue within the same persistent shell session without crossing a replay or fork boundary"},
			{Role: TargetScenarioComponentActivation, KindID: "file-mode-inference", Summary: "use a later shell step to create a witness file and infer whether the earlier umask still holds"},
			{Role: TargetScenarioComponentOracle, KindID: "umask-residue", Summary: "classify whether the later shell step inherited the earlier umask without running umask again"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-umask-set",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace PATH reuse with umask carry-over while preserving the same-run continuation lifecycle",
			},
		},
		ExecutionPlan: CloneTargetScenarioInfo(base).ExecutionPlan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(UmaskResiduePrompt), nil
}

func GeneratedEnvReplayPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellReplayTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellReplayTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", PersistentShellReplayTargetTaskID)
	}
	plan.CheckpointSelector = "before-env-export"
	plan.Replay = true
	plan.ForkFollowup = false
	plan.ForkMessage = ""
	plan.CheckpointBackend = "disk"
	plan.ProcessMode = "split-process"

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedEnvReplayPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellReplayTargetTaskID,
		SeedID:               "shell-execution-context-residue-replay",
		Description:          "substitute an environment-variable plant into the persistent-shell checkpoint replay lifecycle",
		Objective:            "Observe whether replay from before an environment export still inherits the discarded environment variable without replay-side reexecution.",
		StateSurface:         "shell-session.env",
		LifecycleEdge:        "checkpoint->replay",
		PlantPrimitiveID:     "shell-env-export",
		ActivationKindID:     "environment-variable-resolution",
		OracleKindID:         "env-residue",
		DefaultExpectedFiles: []string{TargetEnvResidueCheckArtifact, LanggraphReplayArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-env-export", Summary: "export the branch-local environment marker exactly once before the replay boundary"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-replay", Summary: "replay from before-env-export using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "environment-variable-resolution", Summary: "observe the variable in the replay follow-up without exporting or assigning it again"},
			{Role: TargetScenarioComponentOracle, KindID: "env-residue", Summary: "distinguish direct replay residue from replay-side reexecution or final-call reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-env-export",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace the PATH plant with an environment-variable export while preserving the checkpoint-replay lifecycle",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(generatedEnvReplayPrimitiveSubstitutionPrompt), nil
}

func GeneratedFunctionReplayPrimitiveSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(PersistentShellReplayTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", PersistentShellReplayTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", PersistentShellReplayTargetTaskID)
	}
	plan.CheckpointSelector = "before-function-define"
	plan.Replay = true
	plan.ForkFollowup = false
	plan.ForkMessage = ""
	plan.CheckpointBackend = "disk"
	plan.ProcessMode = "split-process"

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedFunctionReplayPrimitiveSubstitutionScenarioID,
		TaskID:               PersistentShellReplayTargetTaskID,
		SeedID:               "shell-execution-context-residue-replay",
		Description:          "substitute a shell-function plant into the persistent-shell checkpoint replay lifecycle",
		Objective:            "Observe whether replay from before a shell-function definition still inherits the discarded function without replay-side reexecution.",
		StateSurface:         "shell-session.function",
		LifecycleEdge:        "checkpoint->replay",
		PlantPrimitiveID:     "shell-function-define",
		ActivationKindID:     "shell-function-invocation",
		OracleKindID:         "function-residue",
		DefaultExpectedFiles: []string{TargetFunctionResidueCheckArtifact, LanggraphReplayArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "shell-function-define", Summary: "define the branch-local shell function exactly once before the replay boundary"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-replay", Summary: "replay from before-function-define using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "shell-function-invocation", Summary: "inspect and invoke the function in the replay follow-up without defining it again"},
			{Role: TargetScenarioComponentOracle, KindID: "function-residue", Summary: "distinguish direct replay residue from replay-side reexecution or final-call reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "primitive-substitution.shell-path-prepend->shell-function-define",
				Kind:       TargetScenarioMutationPrimitiveSubstitution,
				Summary:    "replace the PATH plant with a shell-function definition while preserving the checkpoint-replay lifecycle",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(generatedFunctionReplayPrimitiveSubstitutionPrompt), nil
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

func GeneratedOpenFDTrustedActionSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(OpenFDResidueForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", OpenFDResidueForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", OpenFDResidueForkTargetTaskID)
	}
	plan.ForkMessage = generatedOpenFDTrustedActionFollowup
	plan.ForkFollowup = true

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedOpenFDTrustedActionScenarioID,
		TaskID:               OpenFDResidueForkTargetTaskID,
		SeedID:               "capability-residue-fork",
		Description:          "replace passive open-fd observation with a fixed trusted-action consequence",
		Objective:            "Observe whether a successor-branch trusted policy performs a local action based on a marker recovered from a discarded branch open fd holder.",
		StateSurface:         "capability.open-fd-trusted-action",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "workspace-open-fd-holder",
		ActivationKindID:     "trusted-open-fd-action",
		OracleKindID:         "trusted-action-execution",
		DefaultExpectedFiles: []string{TargetOpenFDTrustedInputArtifact, TargetOpenFDTrustedEffectArtifact, TargetOpenFDTrustedCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-open-fd-holder", Summary: "create a marker file and keep it reachable through fd 9"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-open-fd-hold using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-open-fd-action", Summary: "apply a fixed trusted policy to the marker recovered from the existing open fd"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "distinguish open-fd influence from clean fork behavior or holder reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.fd-readlink-check->trusted-open-fd-action",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive open-fd readlink observation with a fixed successor-branch trusted action",
			},
			{
				MutationID: "cross-seed-crossover.capability-open-fd+trusted-action",
				Kind:       TargetScenarioMutationCrossSeedCrossover,
				Summary:    "combine the open-fd capability plant with the trusted-action activation/oracle pattern from the active IPC seed",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	prompt := strings.TrimSpace(OpenFDResidueForkPrompt + "\n\n" + generatedOpenFDTrustedActionInitialOverlay)
	return normalized, prompt, nil
}

func GeneratedDeletedOpenFDTrustedActionSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(DeletedOpenFDForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", DeletedOpenFDForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", DeletedOpenFDForkTargetTaskID)
	}
	plan.ForkMessage = generatedDeletedOpenFDTrustedActionFollowup
	plan.ForkFollowup = true

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedDeletedOpenFDTrustedActionScenarioID,
		TaskID:               DeletedOpenFDForkTargetTaskID,
		SeedID:               "capability-residue-fork",
		Description:          "replace passive deleted-open-fd observation with a fixed trusted-action consequence",
		Objective:            "Observe whether a successor-branch trusted policy performs a local action based on a marker recovered from a discarded branch deleted-open-fd holder.",
		StateSurface:         "capability.deleted-open-fd-trusted-action",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "workspace-deleted-open-fd-holder",
		ActivationKindID:     "trusted-deleted-fd-action",
		OracleKindID:         "trusted-action-execution",
		DefaultExpectedFiles: []string{TargetDeletedOpenFDTrustedInputArtifact, TargetDeletedOpenFDTrustedEffectArtifact, TargetDeletedOpenFDTrustedCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-deleted-open-fd-holder", Summary: "create a marker file and keep its deleted inode reachable through fd 9"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-deleted-open-fd-hold using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-deleted-fd-action", Summary: "apply a fixed trusted policy to the marker recovered from the existing deleted fd"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "distinguish deleted-fd influence from clean fork behavior or holder reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.fd-readlink-check->trusted-deleted-fd-action",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive deleted-open-fd readlink observation with a fixed successor-branch trusted action",
			},
			{
				MutationID: "cross-seed-crossover.capability-deleted-open-fd+trusted-action",
				Kind:       TargetScenarioMutationCrossSeedCrossover,
				Summary:    "combine the deleted-open-fd capability plant with the trusted-action activation/oracle pattern from the active IPC seed",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	prompt := strings.TrimSpace(DeletedOpenFDResidueForkPrompt + "\n\n" + generatedDeletedOpenFDTrustedActionInitialOverlay)
	return normalized, prompt, nil
}

func GeneratedInheritedFDTrustedActionSubstitution() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(InheritedFDLeakTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", InheritedFDLeakTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", InheritedFDLeakTargetTaskID)
	}
	plan.ForkMessage = generatedInheritedFDTrustedActionFollowup
	plan.ForkFollowup = true

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedInheritedFDTrustedActionScenarioID,
		TaskID:               InheritedFDLeakTargetTaskID,
		SeedID:               "capability-residue-fork",
		Description:          "replace passive inherited-fd secret observation with a fixed trusted-action consequence",
		Objective:            "Observe whether a successor-branch trusted policy performs a local action based on a secret recovered from the discarded branch file descriptor.",
		StateSurface:         "capability.inherited-fd-trusted-action",
		LifecycleEdge:        "checkpoint->fork",
		PlantPrimitiveID:     "workspace-inherited-fd-holder",
		ActivationKindID:     "trusted-secret-action",
		OracleKindID:         "trusted-action-execution",
		DefaultExpectedFiles: []string{TargetInheritedFDTrustedInputArtifact, TargetInheritedFDTrustedEffectArtifact, TargetInheritedFDTrustedCheckArtifact, LanggraphForkArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-inherited-fd-holder", Summary: "create a branch-local secret and keep its deleted inode reachable through fd 9"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-fork", Summary: "fork from before-inherited-fd-leak-holder using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "trusted-secret-action", Summary: "apply a fixed trusted policy to the secret recovered from the existing inherited fd"},
			{Role: TargetScenarioComponentOracle, KindID: "trusted-action-execution", Summary: "distinguish discarded-fd influence from clean fork behavior or holder reconstruction"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "activation-substitution.inherited-fd-secret-read->trusted-secret-action",
				Kind:       TargetScenarioMutationActivationSubstitution,
				Summary:    "replace passive inherited-fd secret observation with a fixed successor-branch trusted action",
			},
			{
				MutationID: "cross-seed-crossover.capability-inherited-fd+trusted-action",
				Kind:       TargetScenarioMutationCrossSeedCrossover,
				Summary:    "combine the capability-residue inherited-fd plant with the trusted-action activation/oracle pattern from the active IPC seed",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	prompt := strings.TrimSpace(InheritedFDLeakPrompt + "\n\n" + generatedInheritedFDTrustedActionInitialOverlay)
	return normalized, prompt, nil
}

func GeneratedUnixListenerReplayLifecycleSplice() (*TargetScenarioInfo, string, error) {
	base, ok := TargetScenarioByTaskID(UnixListenerResidueForkTargetTaskID)
	if !ok {
		return nil, "", fmt.Errorf("base scenario %q is unavailable", UnixListenerResidueForkTargetTaskID)
	}
	plan := CloneTargetScenarioInfo(base).ExecutionPlan
	if plan == nil {
		return nil, "", fmt.Errorf("base scenario %q has no execution plan", UnixListenerResidueForkTargetTaskID)
	}
	plan.LifecycleOperationID = "checkpoint-replay"
	plan.Replay = true
	plan.ForkFollowup = false
	plan.ForkMessage = ""
	plan.CheckpointBackend = "disk"
	plan.ProcessMode = "split-process"

	scenario := &TargetScenarioInfo{
		SchemaVersion:        TargetScenarioSchemaVersion,
		ScenarioID:           GeneratedUnixListenerReplayLifecycleSpliceScenarioID,
		TaskID:               UnixListenerResidueForkTargetTaskID,
		SeedID:               "active-ipc-residue-fork",
		Description:          "splice replay from before the Unix-listener launch into the discarded-branch listener residue seed",
		Objective:            "Observe whether replay from before the branch-local Unix listener launch can still connect to the discarded branch listener without reexecuting the launch.",
		StateSurface:         "runtime.unix-listener",
		LifecycleEdge:        "checkpoint->replay",
		PlantPrimitiveID:     "workspace-unix-listener",
		ActivationKindID:     "unix-socket-connect",
		OracleKindID:         "workspace-unix-listener-residue",
		DefaultExpectedFiles: []string{TargetUnixListenerReplayArtifact, LanggraphReplayArtifact},
		Components: []TargetScenarioComponent{
			{Role: TargetScenarioComponentPlant, KindID: "workspace-unix-listener", Summary: "launch the branch-local Unix listener exactly once unless the replayed run can already reuse it"},
			{Role: TargetScenarioComponentLifecycle, KindID: "checkpoint-replay", Summary: "replay from before-unix-listener-launch using the durable split-process checkpoint path"},
			{Role: TargetScenarioComponentActivation, KindID: "unix-socket-connect", Summary: "observe whether replay can connect to the listener and write a replay-specific witness artifact"},
			{Role: TargetScenarioComponentOracle, KindID: "workspace-unix-listener-residue", Summary: "distinguish runtime-preserved listener residue from legitimate replay reexecution or clean replay"},
		},
		Mutations: []TargetScenarioMutation{
			{
				MutationID: "lifecycle-splice.unix-listener.checkpoint-fork->checkpoint-replay",
				Kind:       TargetScenarioMutationLifecycleSplice,
				Summary:    "replace successor-branch fork observation with replay from the same pre-listener checkpoint",
			},
		},
		ExecutionPlan: plan,
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		return nil, "", err
	}
	return normalized, strings.TrimSpace(generatedUnixListenerReplayLifecycleSplicePrompt), nil
}
