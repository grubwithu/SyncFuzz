package syncfuzz

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceResidueTaskSpecsIncludeExpandedStateSurfaces(t *testing.T) {
	tests := []struct {
		taskID    string
		selector  string
		witness   string
		promptHit string
	}{
		{
			taskID:    renameResidueForkTargetTaskID,
			selector:  "before-file-rename",
			witness:   targetRenameResidueForkArtifact,
			promptHit: targetRenameResidueDestArtifact,
		},
		{
			taskID:    modeResidueForkTargetTaskID,
			selector:  "before-file-chmod",
			witness:   targetModeResidueForkArtifact,
			promptHit: "mode 000",
		},
		{
			taskID:    appendResidueForkTargetTaskID,
			selector:  "before-file-append",
			witness:   targetAppendResidueForkArtifact,
			promptHit: "SYNCFUZZ_APPEND_EXTRA_MARKER",
		},
		{
			taskID:    hardlinkResidueForkTargetTaskID,
			selector:  "before-hardlink-create",
			witness:   targetHardlinkResidueForkArtifact,
			promptHit: targetHardlinkResidueLinkArtifact,
		},
		{
			taskID:    fifoResidueForkTargetTaskID,
			selector:  "before-fifo-create",
			witness:   targetFIFOResidueForkArtifact,
			promptHit: targetFIFOResiduePipeArtifact,
		},
		{
			taskID:    openFDResidueForkTargetTaskID,
			selector:  "before-open-fd-hold",
			witness:   targetOpenFDResidueForkArtifact,
			promptHit: targetOpenFDResiduePIDArtifact,
		},
		{
			taskID:    deletedOpenFDForkTargetTaskID,
			selector:  "before-deleted-open-fd-hold",
			witness:   targetDeletedOpenFDForkArtifact,
			promptHit: targetDeletedOpenFDPIDArtifact,
		},
		{
			taskID:    inheritedFDLeakTargetTaskID,
			selector:  "before-inherited-fd-leak-holder",
			witness:   targetInheritedFDLeakForkArtifact,
			promptHit: targetInheritedFDLeakSecretArtifact,
		},
		{
			taskID:    unixListenerResidueForkTargetTaskID,
			selector:  "before-unix-listener-launch",
			witness:   targetUnixListenerForkArtifact,
			promptHit: targetUnixListenerSocketArtifact,
		},
	}

	for _, tt := range tests {
		spec, ok := workspaceResidueTaskSpecByID(tt.taskID)
		if !ok {
			t.Fatalf("expected workspace residue task spec for %s", tt.taskID)
		}
		if spec.CheckpointSelector != tt.selector {
			t.Fatalf("unexpected selector for %s: %#v", tt.taskID, spec)
		}
		if !containsString(spec.ExpectedFiles, tt.witness) || !containsString(spec.ExpectedFiles, langgraphForkArtifact) {
			t.Fatalf("expected witness and fork artifact for %s: %#v", tt.taskID, spec.ExpectedFiles)
		}
		if !strings.Contains(spec.Prompt, tt.promptHit) {
			t.Fatalf("expected prompt for %s to mention %q", tt.taskID, tt.promptHit)
		}
		if !strings.Contains(spec.ForkVerificationMessage, tt.witness) {
			t.Fatalf("expected fork verification message for %s to mention %s", tt.taskID, tt.witness)
		}
	}
}

func TestTargetTaskEnvOverridesExpandedResidueTasks(t *testing.T) {
	tests := []struct {
		taskID   string
		selector string
		witness  string
	}{
		{renameResidueForkTargetTaskID, "before-file-rename", targetRenameResidueForkArtifact},
		{modeResidueForkTargetTaskID, "before-file-chmod", targetModeResidueForkArtifact},
		{appendResidueForkTargetTaskID, "before-file-append", targetAppendResidueForkArtifact},
		{hardlinkResidueForkTargetTaskID, "before-hardlink-create", targetHardlinkResidueForkArtifact},
		{fifoResidueForkTargetTaskID, "before-fifo-create", targetFIFOResidueForkArtifact},
		{openFDResidueForkTargetTaskID, "before-open-fd-hold", targetOpenFDResidueForkArtifact},
		{deletedOpenFDForkTargetTaskID, "before-deleted-open-fd-hold", targetDeletedOpenFDForkArtifact},
		{inheritedFDLeakTargetTaskID, "before-inherited-fd-leak-holder", targetInheritedFDLeakForkArtifact},
		{unixListenerResidueForkTargetTaskID, "before-unix-listener-launch", targetUnixListenerForkArtifact},
	}

	for _, tt := range tests {
		env := targetTaskEnvOverrides(tt.taskID)
		if env["SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR"] != tt.selector {
			t.Fatalf("unexpected selector for %s: %#v", tt.taskID, env)
		}
		if env["SYNCFUZZ_LANGGRAPH_PROCESS_MODE"] != "split-process" || env["SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND"] != "disk" {
			t.Fatalf("expected split-process + disk for %s: %#v", tt.taskID, env)
		}
		if !strings.Contains(env["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"], tt.witness) {
			t.Fatalf("expected fork witness message for %s to mention %s", tt.taskID, tt.witness)
		}
	}
}

func TestEvaluateRenameResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetRenameResidueForkArtifact), []byte("PRESENT_BRANCH_RENAME_DST\nSYNCFUZZ_RENAME_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write rename witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_RENAME_RESIDUE_MARKER\n' > branch-rename-src.txt"},
		langgraphShellCall{Command: "mv branch-rename-src.txt branch-rename-dst.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-file-rename",
		langgraphShellCall{
			Command: "if [ -f branch-rename-src.txt ]; then printf 'PRESENT_BRANCH_RENAME_SRC\n' > rename-residue-fork-check.txt; cat branch-rename-src.txt >> rename-residue-fork-check.txt; elif [ -f branch-rename-dst.txt ]; then printf 'PRESENT_BRANCH_RENAME_DST\n' > rename-residue-fork-check.txt; cat branch-rename-dst.txt >> rename-residue-fork-check.txt; else printf 'MISSING_BRANCH_RENAME_FILES\n' > rename-residue-fork-check.txt; fi; cat rename-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_RENAME_DST\nSYNCFUZZ_RENAME_RESIDUE_MARKER\n",
		},
	)

	oracle := evaluateRenameResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected rename residue oracle to confirm: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateModeResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetModeResidueForkArtifact), []byte("MODE=644\n"), 0o644); err != nil {
		t.Fatalf("write mode witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_MODE_RESIDUE_MARKER\n' > branch-mode-note.txt && chmod 0644 branch-mode-note.txt", Output: ""},
		langgraphShellCall{Command: "stat -c '%a' branch-mode-note.txt", Output: "644\n"},
		langgraphShellCall{Command: "chmod 000 branch-mode-note.txt", Output: ""},
		langgraphShellCall{Command: "stat -c '%a' branch-mode-note.txt", Output: "000\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-file-chmod",
		langgraphShellCall{
			Command: "if [ -f branch-mode-note.txt ]; then printf 'MODE=%s\n' \"$(stat -c '%a' branch-mode-note.txt)\" > mode-residue-fork-check.txt; else printf 'MISSING_BRANCH_MODE_NOTE\n' > mode-residue-fork-check.txt; fi; cat mode-residue-fork-check.txt",
			Output:  "MODE=644\n",
		},
	)

	oracle := evaluateModeResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean mode fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "fork preserved the tightened 000 mode across the checkpoint boundary") {
		t.Fatalf("expected clean-fork missing reason: %#v", oracle)
	}
}

func TestEvaluateAppendResidueForkTargetOracleWorkspaceRebuild(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetAppendResidueForkArtifact), []byte("SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write append witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_APPEND_BASE_MARKER\n' > branch-append-note.txt"},
		langgraphShellCall{Command: "printf 'SYNCFUZZ_APPEND_EXTRA_MARKER\n' >> branch-append-note.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-file-append",
		langgraphShellCall{
			Command: "printf 'SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n' > branch-append-note.txt && cat branch-append-note.txt > append-residue-fork-check.txt && cat append-residue-fork-check.txt",
			Output:  "SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n",
		},
	)

	oracle := evaluateAppendResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected append workspace reconstruction to stay unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !containsString(oracle.Missing, "append residue occurred without rewriting, deleting, or appending branch-append-note.txt during the fork follow-up") {
		t.Fatalf("expected workspace rebuild missing reason: %#v", oracle)
	}
}

func TestEvaluateHardlinkResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetHardlinkResidueForkArtifact), []byte("PRESENT_BRANCH_HARDLINK\n123 target-prompt.txt\n123 branch-hardlink.txt\n"), 0o644); err != nil {
		t.Fatalf("write hardlink witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "ln target-prompt.txt branch-hardlink.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-hardlink-create",
		langgraphShellCall{
			Command: "if [ -f branch-hardlink.txt ]; then printf 'PRESENT_BRANCH_HARDLINK\n' > hardlink-residue-fork-check.txt; ls -li target-prompt.txt branch-hardlink.txt >> hardlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_HARDLINK\n' > hardlink-residue-fork-check.txt; fi; cat hardlink-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_HARDLINK\n123 target-prompt.txt\n123 branch-hardlink.txt\n",
		},
	)

	oracle := evaluateHardlinkResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed hardlink residue: %#v", oracle)
	}
}

func TestEvaluateFIFOResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetFIFOResidueForkArtifact), []byte("MISSING_BRANCH_FIFO\n"), 0o644); err != nil {
		t.Fatalf("write fifo witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkfifo branch-fifo"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-fifo-create",
		langgraphShellCall{
			Command: "if [ -p branch-fifo ]; then printf 'PRESENT_BRANCH_FIFO\n' > fifo-residue-fork-check.txt; ls -l branch-fifo >> fifo-residue-fork-check.txt; else printf 'MISSING_BRANCH_FIFO\n' > fifo-residue-fork-check.txt; fi; cat fifo-residue-fork-check.txt",
			Output:  "MISSING_BRANCH_FIFO\n",
		},
	)

	oracle := evaluateFIFOResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean fifo fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean fifo attribution: %#v", oracle)
	}
}

func TestEvaluateOpenFDResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetOpenFDResidueForkArtifact), []byte("PRESENT_BRANCH_OPEN_FD\n/tmp/workspace/branch-fd-note.txt\n"), 0o644); err != nil {
		t.Fatalf("write open-fd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_OPEN_FD_RESIDUE_MARKER\n' > branch-fd-note.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-fd-note.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-fd-pid.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-open-fd-hold",
		langgraphShellCall{
			Command: "if [ -f branch-fd-pid.txt ]; then pid=$(cat branch-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_BRANCH_OPEN_FD\n' > open-fd-residue-fork-check.txt; readlink \"/proc/$pid/fd/9\" >> open-fd-residue-fork-check.txt; else printf 'MISSING_BRANCH_OPEN_FD\n' > open-fd-residue-fork-check.txt; fi; else printf 'MISSING_BRANCH_OPEN_FD_PID\n' > open-fd-residue-fork-check.txt; fi; cat open-fd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_OPEN_FD\n/tmp/workspace/branch-fd-note.txt\n",
		},
	)

	oracle := evaluateOpenFDResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected open-fd residue oracle to confirm: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateDeletedOpenFDResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetDeletedOpenFDForkArtifact), []byte("MISSING_BRANCH_DELETED_OPEN_FD\n"), 0o644); err != nil {
		t.Fatalf("write deleted open-fd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_DELETED_OPEN_FD_RESIDUE_MARKER\n' > branch-deleted-fd-note.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-deleted-fd-note.txt; rm -f branch-deleted-fd-note.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-deleted-fd-pid.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-deleted-open-fd-hold",
		langgraphShellCall{
			Command: "if [ -f branch-deleted-fd-pid.txt ]; then pid=$(cat branch-deleted-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_BRANCH_DELETED_OPEN_FD\n' > deleted-open-fd-residue-fork-check.txt; readlink \"/proc/$pid/fd/9\" >> deleted-open-fd-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETED_OPEN_FD\n' > deleted-open-fd-residue-fork-check.txt; fi; else printf 'MISSING_BRANCH_DELETED_OPEN_FD_PID\n' > deleted-open-fd-residue-fork-check.txt; fi; cat deleted-open-fd-residue-fork-check.txt",
			Output:  "MISSING_BRANCH_DELETED_OPEN_FD\n",
		},
	)

	oracle := evaluateDeletedOpenFDResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean deleted-open-fd fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionCleanFork {
		t.Fatalf("expected clean deleted-open-fd attribution: %#v", oracle)
	}
}

func TestEvaluateInheritedFDLeakTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetInheritedFDLeakForkArtifact), []byte("PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n"), 0o644); err != nil {
		t.Fatalf("write inherited fd leakage witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n' > branch-inherited-fd-secret.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-inherited-fd-pid.txt"},
	)
	writeLangGraphOperationSummaryWithHistoricalContext(t, tmp, langgraphForkArtifact, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "printf 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n' > branch-inherited-fd-secret.txt",
			Output:  "SYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
		"This is the later SyncFuzz successor-branch observation step. Historical messages from the earlier checkpoint are context only; do not repeat them.",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; readlink \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; cat \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; fi; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE_PID\n' > inherited-fd-branch-leakage-check.txt; fi; cat inherited-fd-branch-leakage-check.txt",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	oracle := evaluateInheritedFDLeakTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected inherited fd leakage oracle to confirm: %#v", oracle)
	}
	if oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateUnixListenerResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, targetUnixListenerForkArtifact), []byte("PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write unix listener witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_UNIX_LISTENER_RESPONSE\\n\")'); printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; printf '%s\n' \"$response\" >> unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	oracle := evaluateUnixListenerResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != targetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed unix listener residue: %#v", oracle)
	}
}

func TestEvaluateUnixListenerResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_UNIX_LISTENER_RESPONSE\\n\")'); printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; printf '%s\n' \"$response\" >> unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, unixListenerResidueForkTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant unix listener task: %#v", result)
	}
}

func TestEvaluateUnixListenerResidueForkTaskComplianceFlagsForkRelaunchFromLifecycle(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"},
	)
	writeLangGraphLifecycleOperationCommands(t, tmp, "fork",
		"python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &",
		"if [ -S branch-listener.sock ]; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, unixListenerResidueForkTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected fork relaunch to violate unix listener task: %#v", result)
	}
	if !containsString(result.Violations, "fork follow-up avoided relaunching or recreating the Unix listener") {
		t.Fatalf("expected relaunch violation: %#v", result)
	}
}

func TestInspectLangGraphForkUnixListenerResidueEvidenceFlagsForkRelaunchFromLifecycle(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphLifecycleOperationCommands(t, tmp, "fork",
		"python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &",
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	evidence, err := inspectLangGraphForkUnixListenerResidueEvidence(tmp)
	if err != nil {
		t.Fatalf("inspect unix listener evidence: %v", err)
	}
	if evidence.Attribution != targetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected fork relaunch to be attributed as workspace rebuild: %#v", evidence)
	}
}

func TestEvaluateInheritedFDLeakTaskComplianceUsesLifecycleCommandTrace(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\""},
	)
	writeLangGraphLifecycleCommands(t, tmp,
		"printf '%s\n' 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET' > branch-inherited-fd-secret.txt",
		"sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &",
		"IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\"",
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; readlink \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; cat \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; fi; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE_PID\n' > inherited-fd-branch-leakage-check.txt; fi; cat inherited-fd-branch-leakage-check.txt",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, inheritedFDLeakTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected lifecycle-backed inherited fd task compliance: %#v", result)
	}
}

func TestEvaluateInheritedFDLeakTaskComplianceFlagsExtraInspection(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\""},
	)
	writeLangGraphLifecycleCommands(t, tmp,
		"printf '%s\n' 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET' > branch-inherited-fd-secret.txt",
		"sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &",
		"IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\"",
		"ps aux",
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); found=\"\"; for cand in \"$pid\" $(cat \"/proc/$pid/task/$pid/children\" 2>/dev/null || true); do if [ -n \"$cand\" ] && [ -r \"/proc/$cand/fd/9\" ]; then found=\"$cand\"; break; fi; done; if [ -n \"$found\" ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; readlink \"/proc/$found/fd/9\" >> inherited-fd-branch-leakage-check.txt; cat \"/proc/$found/fd/9\" >> inherited-fd-branch-leakage-check.txt; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; fi; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE_PID\n' > inherited-fd-branch-leakage-check.txt; fi; cat inherited-fd-branch-leakage-check.txt",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, inheritedFDLeakTargetTaskID)
	if result.Status != targetTaskComplianceStatusViolated {
		t.Fatalf("expected extra inherited-fd inspection to violate compliance: %#v", result)
	}
	if !containsString(result.Violations, "initial branch avoided extra process/file inspection command: ps") {
		t.Fatalf("expected ps inspection violation: %#v", result)
	}
}

func TestEvaluateInheritedFDLeakTaskComplianceAcceptsTruncatedForkCommand(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\""},
	)
	writeLangGraphLifecycleCommands(t, tmp,
		"printf '%s\n' 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET' > branch-inherited-fd-secret.txt",
		"sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; printf \"%s\n\" \"$$\" > branch-inherited-fd-pid.txt; sleep 180' >/dev/null 2>&1 &",
		"IFS= read -r pid < branch-inherited-fd-pid.txt; readlink \"/proc/$pid/fd/9\"",
	)
	writeLangGraphOperationSummary(t, tmp, langgraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.tx",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, inheritedFDLeakTargetTaskID)
	if result.Status != targetTaskComplianceStatusCompliant {
		t.Fatalf("expected truncated fork command with witness output to remain compliant: %#v", result)
	}
}

func writeLangGraphOperationSummaryWithHistoricalContext(t *testing.T, dir string, artifact string, selector string, historicalCall langgraphShellCall, userMessage string, followupCall langgraphShellCall) {
	t.Helper()

	shellMessages := func(call langgraphShellCall) []langgraphHistoryMessage {
		args, err := json.Marshal(map[string]string{"command": call.Command})
		if err != nil {
			t.Fatalf("marshal shell command: %v", err)
		}
		return []langgraphHistoryMessage{
			{
				Role: "ai",
				ToolCalls: []langgraphHistoryToolCall{
					{Name: "shell", Args: string(args)},
				},
			},
			{
				Role:    "tool",
				Content: call.Output,
			},
		}
	}

	messages := append([]langgraphHistoryMessage{}, shellMessages(historicalCall)...)
	messages = append(messages, langgraphHistoryMessage{
		Role:    "human",
		Content: userMessage,
	})
	messages = append(messages, shellMessages(followupCall)...)

	raw, err := json.Marshal(langgraphOperationSummary{
		Operation:          "fork",
		Requested:          true,
		CheckpointSelector: selector,
		CheckpointIndex:    1,
		UserMessage:        userMessage,
		Messages:           messages,
	})
	if err != nil {
		t.Fatalf("marshal langgraph operation summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact), raw, 0o644); err != nil {
		t.Fatalf("write langgraph operation summary: %v", err)
	}
}

func writeLangGraphLifecycleCommands(t *testing.T, dir string, commands ...string) {
	t.Helper()
	writeLangGraphLifecycleOperationCommands(t, dir, "initial_run", commands...)
}

func writeLangGraphLifecycleOperationCommands(t *testing.T, dir string, operation string, commands ...string) {
	t.Helper()

	type lifecycleEvent struct {
		Event          string `json:"event"`
		Operation      string `json:"operation"`
		CommandPreview string `json:"command_preview"`
	}
	events := make([]lifecycleEvent, 0, len(commands))
	for _, command := range commands {
		events = append(events, lifecycleEvent{
			Event:          "shell_command_started",
			Operation:      operation,
			CommandPreview: command,
		})
	}
	raw, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatalf("marshal langgraph lifecycle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, langgraphLifecycleArtifact), raw, 0o644); err != nil {
		t.Fatalf("write langgraph lifecycle: %v", err)
	}
}
