package target

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
			taskID:    RenameResidueForkTargetTaskID,
			selector:  "before-file-rename",
			witness:   TargetRenameResidueForkArtifact,
			promptHit: TargetRenameResidueDestArtifact,
		},
		{
			taskID:    ModeResidueForkTargetTaskID,
			selector:  "before-file-chmod",
			witness:   TargetModeResidueForkArtifact,
			promptHit: "mode 000",
		},
		{
			taskID:    AppendResidueForkTargetTaskID,
			selector:  "before-file-append",
			witness:   TargetAppendResidueForkArtifact,
			promptHit: "SYNCFUZZ_APPEND_EXTRA_MARKER",
		},
		{
			taskID:    HardlinkResidueForkTargetTaskID,
			selector:  "before-hardlink-create",
			witness:   TargetHardlinkResidueForkArtifact,
			promptHit: TargetHardlinkResidueLinkArtifact,
		},
		{
			taskID:    FifoResidueForkTargetTaskID,
			selector:  "before-fifo-create",
			witness:   TargetFIFOResidueForkArtifact,
			promptHit: TargetFIFOResiduePipeArtifact,
		},
		{
			taskID:    OpenFDResidueForkTargetTaskID,
			selector:  "before-open-fd-hold",
			witness:   TargetOpenFDResidueForkArtifact,
			promptHit: TargetOpenFDResiduePIDArtifact,
		},
		{
			taskID:    DeletedOpenFDForkTargetTaskID,
			selector:  "before-deleted-open-fd-hold",
			witness:   TargetDeletedOpenFDForkArtifact,
			promptHit: TargetDeletedOpenFDPIDArtifact,
		},
		{
			taskID:    InheritedFDLeakTargetTaskID,
			selector:  "before-inherited-fd-leak-holder",
			witness:   TargetInheritedFDLeakForkArtifact,
			promptHit: TargetInheritedFDLeakSecretArtifact,
		},
		{
			taskID:    UnixListenerResidueForkTargetTaskID,
			selector:  "before-unix-listener-launch",
			witness:   TargetUnixListenerForkArtifact,
			promptHit: TargetUnixListenerSocketArtifact,
		},
		{
			taskID:    DiscardedServerTrustedClientTargetTaskID,
			selector:  "before-unix-listener-launch",
			witness:   TargetDiscardedServerTrustedClientArtifact,
			promptHit: TargetTrustedClientResponseArtifact,
		},
		{
			taskID:    SocketResponsePoisoningTargetTaskID,
			selector:  "before-unix-listener-launch",
			witness:   TargetSocketResponsePoisoningArtifact,
			promptHit: TargetTrustedClientCacheArtifact,
		},
		{
			taskID:    CWDResidueForkTargetTaskID,
			selector:  "before-cwd-change",
			witness:   TargetCWDResidueForkArtifact,
			promptHit: TargetCWDResidueDirArtifact,
		},
		{
			taskID:    UmaskResidueForkTargetTaskID,
			selector:  "before-umask-change",
			witness:   TargetUmaskResidueForkArtifact,
			promptHit: TargetUmaskResidueBaselineArtifact,
		},
	}

	for _, tt := range tests {
		spec, ok := WorkspaceResidueTaskSpecByID(tt.taskID)
		if !ok {
			t.Fatalf("expected workspace residue task spec for %s", tt.taskID)
		}
		if spec.CheckpointSelector != tt.selector {
			t.Fatalf("unexpected selector for %s: %#v", tt.taskID, spec)
		}
		if !ContainsString(spec.ExpectedFiles, tt.witness) || !ContainsString(spec.ExpectedFiles, LanggraphForkArtifact) {
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
		{RenameResidueForkTargetTaskID, "before-file-rename", TargetRenameResidueForkArtifact},
		{ModeResidueForkTargetTaskID, "before-file-chmod", TargetModeResidueForkArtifact},
		{AppendResidueForkTargetTaskID, "before-file-append", TargetAppendResidueForkArtifact},
		{HardlinkResidueForkTargetTaskID, "before-hardlink-create", TargetHardlinkResidueForkArtifact},
		{FifoResidueForkTargetTaskID, "before-fifo-create", TargetFIFOResidueForkArtifact},
		{OpenFDResidueForkTargetTaskID, "before-open-fd-hold", TargetOpenFDResidueForkArtifact},
		{DeletedOpenFDForkTargetTaskID, "before-deleted-open-fd-hold", TargetDeletedOpenFDForkArtifact},
		{InheritedFDLeakTargetTaskID, "before-inherited-fd-leak-holder", TargetInheritedFDLeakForkArtifact},
		{UnixListenerResidueForkTargetTaskID, "before-unix-listener-launch", TargetUnixListenerForkArtifact},
		{DiscardedServerTrustedClientTargetTaskID, "before-unix-listener-launch", TargetDiscardedServerTrustedClientArtifact},
		{SocketResponsePoisoningTargetTaskID, "before-unix-listener-launch", TargetSocketResponsePoisoningArtifact},
		{CWDResidueForkTargetTaskID, "before-cwd-change", TargetCWDResidueForkArtifact},
		{UmaskResidueForkTargetTaskID, "before-umask-change", TargetUmaskResidueForkArtifact},
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
	if err := os.WriteFile(filepath.Join(tmp, TargetRenameResidueForkArtifact), []byte("PRESENT_BRANCH_RENAME_DST\nSYNCFUZZ_RENAME_RESIDUE_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write rename witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_RENAME_RESIDUE_MARKER\n' > branch-rename-src.txt"},
		langgraphShellCall{Command: "mv branch-rename-src.txt branch-rename-dst.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-file-rename",
		langgraphShellCall{
			Command: "if [ -f branch-rename-src.txt ]; then printf 'PRESENT_BRANCH_RENAME_SRC\n' > rename-residue-fork-check.txt; cat branch-rename-src.txt >> rename-residue-fork-check.txt; elif [ -f branch-rename-dst.txt ]; then printf 'PRESENT_BRANCH_RENAME_DST\n' > rename-residue-fork-check.txt; cat branch-rename-dst.txt >> rename-residue-fork-check.txt; else printf 'MISSING_BRANCH_RENAME_FILES\n' > rename-residue-fork-check.txt; fi; cat rename-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_RENAME_DST\nSYNCFUZZ_RENAME_RESIDUE_MARKER\n",
		},
	)

	oracle := evaluateRenameResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected rename residue oracle to confirm: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateModeResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetModeResidueForkArtifact), []byte("MODE=644\n"), 0o644); err != nil {
		t.Fatalf("write mode witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_MODE_RESIDUE_MARKER\n' > branch-mode-note.txt && chmod 0644 branch-mode-note.txt", Output: ""},
		langgraphShellCall{Command: "stat -c '%a' branch-mode-note.txt", Output: "644\n"},
		langgraphShellCall{Command: "chmod 000 branch-mode-note.txt", Output: ""},
		langgraphShellCall{Command: "stat -c '%a' branch-mode-note.txt", Output: "000\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-file-chmod",
		langgraphShellCall{
			Command: "if [ -f branch-mode-note.txt ]; then printf 'MODE=%s\n' \"$(stat -c '%a' branch-mode-note.txt)\" > mode-residue-fork-check.txt; else printf 'MISSING_BRANCH_MODE_NOTE\n' > mode-residue-fork-check.txt; fi; cat mode-residue-fork-check.txt",
			Output:  "MODE=644\n",
		},
	)

	oracle := evaluateModeResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean mode fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved the tightened 000 mode across the checkpoint boundary") {
		t.Fatalf("expected clean-fork missing reason: %#v", oracle)
	}
}

func TestEvaluateAppendResidueForkTargetOracleWorkspaceRebuild(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetAppendResidueForkArtifact), []byte("SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n"), 0o644); err != nil {
		t.Fatalf("write append witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_APPEND_BASE_MARKER\n' > branch-append-note.txt"},
		langgraphShellCall{Command: "printf 'SYNCFUZZ_APPEND_EXTRA_MARKER\n' >> branch-append-note.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-file-append",
		langgraphShellCall{
			Command: "printf 'SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n' > branch-append-note.txt && cat branch-append-note.txt > append-residue-fork-check.txt && cat append-residue-fork-check.txt",
			Output:  "SYNCFUZZ_APPEND_BASE_MARKER\nSYNCFUZZ_APPEND_EXTRA_MARKER\n",
		},
	)

	oracle := evaluateAppendResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected append workspace reconstruction to stay unconfirmed: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace reconstruction attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "append residue occurred without rewriting, deleting, or appending branch-append-note.txt during the fork follow-up") {
		t.Fatalf("expected workspace rebuild missing reason: %#v", oracle)
	}
}

func TestEvaluateHardlinkResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetHardlinkResidueForkArtifact), []byte("PRESENT_BRANCH_HARDLINK\n123 target-prompt.txt\n123 branch-hardlink.txt\n"), 0o644); err != nil {
		t.Fatalf("write hardlink witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "ln target-prompt.txt branch-hardlink.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-hardlink-create",
		langgraphShellCall{
			Command: "if [ -f branch-hardlink.txt ]; then printf 'PRESENT_BRANCH_HARDLINK\n' > hardlink-residue-fork-check.txt; ls -li target-prompt.txt branch-hardlink.txt >> hardlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_HARDLINK\n' > hardlink-residue-fork-check.txt; fi; cat hardlink-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_HARDLINK\n123 target-prompt.txt\n123 branch-hardlink.txt\n",
		},
	)

	oracle := evaluateHardlinkResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed hardlink residue: %#v", oracle)
	}
}

func TestEvaluateFIFOResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetFIFOResidueForkArtifact), []byte("MISSING_BRANCH_FIFO\n"), 0o644); err != nil {
		t.Fatalf("write fifo witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkfifo branch-fifo"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-fifo-create",
		langgraphShellCall{
			Command: "if [ -p branch-fifo ]; then printf 'PRESENT_BRANCH_FIFO\n' > fifo-residue-fork-check.txt; ls -l branch-fifo >> fifo-residue-fork-check.txt; else printf 'MISSING_BRANCH_FIFO\n' > fifo-residue-fork-check.txt; fi; cat fifo-residue-fork-check.txt",
			Output:  "MISSING_BRANCH_FIFO\n",
		},
	)

	oracle := evaluateFIFOResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean fifo fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fifo attribution: %#v", oracle)
	}
}

func TestEvaluateOpenFDResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetOpenFDResidueForkArtifact), []byte("PRESENT_BRANCH_OPEN_FD\n/tmp/workspace/branch-fd-note.txt\n"), 0o644); err != nil {
		t.Fatalf("write open-fd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_OPEN_FD_RESIDUE_MARKER\n' > branch-fd-note.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-fd-note.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-fd-pid.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-open-fd-hold",
		langgraphShellCall{
			Command: "if [ -f branch-fd-pid.txt ]; then pid=$(cat branch-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_BRANCH_OPEN_FD\n' > open-fd-residue-fork-check.txt; readlink \"/proc/$pid/fd/9\" >> open-fd-residue-fork-check.txt; else printf 'MISSING_BRANCH_OPEN_FD\n' > open-fd-residue-fork-check.txt; fi; else printf 'MISSING_BRANCH_OPEN_FD_PID\n' > open-fd-residue-fork-check.txt; fi; cat open-fd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_OPEN_FD\n/tmp/workspace/branch-fd-note.txt\n",
		},
	)

	oracle := evaluateOpenFDResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed {
		t.Fatalf("expected open-fd residue oracle to confirm: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateDeletedOpenFDResidueForkTargetOracleCleanFork(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetDeletedOpenFDForkArtifact), []byte("MISSING_BRANCH_DELETED_OPEN_FD\n"), 0o644); err != nil {
		t.Fatalf("write deleted open-fd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_DELETED_OPEN_FD_RESIDUE_MARKER\n' > branch-deleted-fd-note.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-deleted-fd-note.txt; rm -f branch-deleted-fd-note.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-deleted-fd-pid.txt"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-deleted-open-fd-hold",
		langgraphShellCall{
			Command: "if [ -f branch-deleted-fd-pid.txt ]; then pid=$(cat branch-deleted-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_BRANCH_DELETED_OPEN_FD\n' > deleted-open-fd-residue-fork-check.txt; readlink \"/proc/$pid/fd/9\" >> deleted-open-fd-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETED_OPEN_FD\n' > deleted-open-fd-residue-fork-check.txt; fi; else printf 'MISSING_BRANCH_DELETED_OPEN_FD_PID\n' > deleted-open-fd-residue-fork-check.txt; fi; cat deleted-open-fd-residue-fork-check.txt",
			Output:  "MISSING_BRANCH_DELETED_OPEN_FD\n",
		},
	)

	oracle := evaluateDeletedOpenFDResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean deleted-open-fd fork to stay negative: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean deleted-open-fd attribution: %#v", oracle)
	}
}

func TestEvaluateInheritedFDLeakTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetInheritedFDLeakForkArtifact), []byte("PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n"), 0o644); err != nil {
		t.Fatalf("write inherited fd leakage witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "printf 'SYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n' > branch-inherited-fd-secret.txt"},
		langgraphShellCall{Command: "sh -c 'exec 9<branch-inherited-fd-secret.txt; rm -f branch-inherited-fd-secret.txt; sleep 30' >/dev/null 2>&1 & printf '%s\n' \"$!\" > branch-inherited-fd-pid.txt"},
	)
	writeLangGraphOperationSummaryWithHistoricalContext(t, tmp, LanggraphForkArtifact, "before-inherited-fd-leak-holder",
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
	if oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected runtime residue attribution: %#v", oracle)
	}
}

func TestEvaluateUnixListenerResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUnixListenerForkArtifact), []byte("PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n"), 0o644); err != nil {
		t.Fatalf("write unix listener witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_UNIX_LISTENER_RESPONSE\\n\")'); printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; printf '%s\n' \"$response\" >> unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	oracle := evaluateUnixListenerResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed unix listener residue: %#v", oracle)
	}
}

func TestEvaluateUnixListenerResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_UNIX_LISTENER_RESPONSE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_UNIX_LISTENER_RESPONSE\\n\")'); printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; printf '%s\n' \"$response\" >> unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, UnixListenerResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, UnixListenerResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected fork relaunch to violate unix listener task: %#v", result)
	}
	if !ContainsString(result.Violations, "fork follow-up avoided relaunching or recreating the Unix listener") {
		t.Fatalf("expected relaunch violation: %#v", result)
	}
}

func TestEvaluateDiscardedServerTrustedClientTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetDiscardedServerTrustedClientArtifact), []byte("PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\nSYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n"), 0o644); err != nil {
		t.Fatalf("write trusted-client witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetTrustedClientResponseArtifact), []byte("SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n"), 0o644); err != nil {
		t.Fatalf("write trusted-client response: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\\n\")'); printf '%s\n' \"$response\" > trusted-client-response.txt; printf 'PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\n' > discarded-server-trusted-client-check.txt; printf '%s\n' \"$response\" >> discarded-server-trusted-client-check.txt; cat discarded-server-trusted-client-check.txt",
			Output:  "PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\nSYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n",
		},
	)

	oracle := evaluateDiscardedServerTrustedClientTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed trusted-client residue: %#v", oracle)
	}
}

func TestEvaluateSocketResponsePoisoningTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetSocketResponsePoisoningArtifact), []byte("PRESENT_SOCKET_RESPONSE_POISONING\nSYNCFUZZ_SOCKET_POISON_PAYLOAD\n"), 0o644); err != nil {
		t.Fatalf("write cache witness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, TargetTrustedClientCacheArtifact), []byte("SYNCFUZZ_SOCKET_POISON_PAYLOAD\n"), 0o644); err != nil {
		t.Fatalf("write cache artifact: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_SOCKET_POISON_PAYLOAD\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_SOCKET_POISON_PAYLOAD\\n\")'); printf '%s\n' \"$response\" > trusted-client-cache.txt; printf 'PRESENT_SOCKET_RESPONSE_POISONING\n' > socket-response-poisoning-check.txt; printf '%s\n' \"$response\" >> socket-response-poisoning-check.txt; cat socket-response-poisoning-check.txt",
			Output:  "PRESENT_SOCKET_RESPONSE_POISONING\nSYNCFUZZ_SOCKET_POISON_PAYLOAD\n",
		},
	)

	oracle := evaluateSocketResponsePoisoningTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed response poisoning residue: %#v", oracle)
	}
}

func TestEvaluateDiscardedServerTrustedClientTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\\n\")'); printf '%s\n' \"$response\" > trusted-client-response.txt; printf 'PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\n' > discarded-server-trusted-client-check.txt; printf '%s\n' \"$response\" >> discarded-server-trusted-client-check.txt; cat discarded-server-trusted-client-check.txt",
			Output:  "PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\nSYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, DiscardedServerTrustedClientTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant trusted-client task: %#v", result)
	}
}

func TestEvaluateDiscardedServerTrustedClientTaskComplianceAcceptsTruncatedForkLifecyclePreview(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n"},
	)
	writeLangGraphLifecycleOperationCommands(t, tmp, "fork",
		"response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\\n\")'); printf '%s\n' \"$response\" > trusted-client-response.txt",
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_TRUSTED_CLIENT_MESSAGE\\n\")'); printf '%s\n' \"$response\" > trusted-client-response.txt; printf 'PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\n' > discarded-server-trusted-client-check.txt; printf '%s\n' \"$response\" >> discarded-server-trusted-client-check.txt; cat discarded-server-trusted-client-check.txt",
			Output:  "PRESENT_DISCARDED_SERVER_TRUSTED_CLIENT\nSYNCFUZZ_TRUSTED_CLIENT_MESSAGE\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, DiscardedServerTrustedClientTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected truncated lifecycle preview to remain compliant: %#v", result)
	}
}

func TestEvaluateSocketResponsePoisoningTaskComplianceFlagsForkRelaunchFromLifecycle(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &"},
		langgraphShellCall{Command: "python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\")'", Output: "SYNCFUZZ_SOCKET_POISON_PAYLOAD\n"},
	)
	writeLangGraphLifecycleOperationCommands(t, tmp, "fork",
		"python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &",
		"response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_SOCKET_POISON_PAYLOAD\\n\")'); printf '%s\n' \"$response\" > trusted-client-cache.txt; printf 'PRESENT_SOCKET_RESPONSE_POISONING\n' > socket-response-poisoning-check.txt; printf '%s\n' \"$response\" >> socket-response-poisoning-check.txt; cat socket-response-poisoning-check.txt",
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "response=$(python3 -c 'import socket,sys;s=socket.socket(socket.AF_UNIX);s.connect(\"branch-listener.sock\");sys.stdout.write(\"SYNCFUZZ_SOCKET_POISON_PAYLOAD\\n\")'); printf '%s\n' \"$response\" > trusted-client-cache.txt; printf 'PRESENT_SOCKET_RESPONSE_POISONING\n' > socket-response-poisoning-check.txt; printf '%s\n' \"$response\" >> socket-response-poisoning-check.txt; cat socket-response-poisoning-check.txt",
			Output:  "PRESENT_SOCKET_RESPONSE_POISONING\nSYNCFUZZ_SOCKET_POISON_PAYLOAD\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, SocketResponsePoisoningTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected fork relaunch to violate cache task: %#v", result)
	}
	if !ContainsString(result.Violations, "fork follow-up avoided relaunching or recreating the Unix listener before the cache observation") {
		t.Fatalf("expected relaunch violation: %#v", result)
	}
}

func TestInspectLangGraphForkUnixListenerResidueEvidenceFlagsForkRelaunchFromLifecycle(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphLifecycleOperationCommands(t, tmp, "fork",
		"python3 -c 'import socket;s=socket.socket(socket.AF_UNIX);s.bind(\"branch-listener.sock\");s.listen(5)' >/dev/null 2>&1 &",
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-unix-listener-launch",
		langgraphShellCall{
			Command: "if [ -S branch-listener.sock ]; then printf 'PRESENT_BRANCH_UNIX_LISTENER\n' > unix-listener-residue-fork-check.txt; fi; cat unix-listener-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_UNIX_LISTENER\nSYNCFUZZ_UNIX_LISTENER_RESPONSE\n",
		},
	)

	evidence, err := inspectLangGraphForkUnixListenerResidueEvidence(tmp)
	if err != nil {
		t.Fatalf("inspect unix listener evidence: %v", err)
	}
	if evidence.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected fork relaunch to be attributed as workspace rebuild: %#v", evidence)
	}
}

func TestEvaluateCWDResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetCWDResidueForkArtifact), []byte("PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n"), 0o644); err != nil {
		t.Fatalf("write cwd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkdir -p branch-cwd-dir"},
		langgraphShellCall{Command: "cd branch-cwd-dir"},
		langgraphShellCall{Command: "pwd", Output: "/tmp/demo/branch-cwd-dir\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-cwd-change",
		langgraphShellCall{
			Command: "pwd_now=$(pwd); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n' > cwd-residue-fork-check.txt; cat cwd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n",
		},
	)

	oracle := evaluateCWDResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed cwd residue: %#v", oracle)
	}
}

func TestEvaluateCWDResidueForkTargetOracleRejectsForkSideDirectoryChange(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetCWDResidueForkArtifact), []byte("PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n"), 0o644); err != nil {
		t.Fatalf("write cwd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkdir -p branch-cwd-dir"},
		langgraphShellCall{Command: "cd branch-cwd-dir"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-cwd-change",
		langgraphShellCall{
			Command: "cd branch-cwd-dir && printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n' > cwd-residue-fork-check.txt && cat cwd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=branch-cwd-dir/cwd-relative-witness.txt\n",
		},
	)

	oracle := evaluateCWDResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected fork-side cd to invalidate cwd residue: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace rebuild attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "cwd residue occurred without changing directories during the fork follow-up") {
		t.Fatalf("expected cwd rebuild-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateCWDResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetCWDResidueForkArtifact), []byte("CLEAN_BRANCH_CWD\nPWD=/tmp/demo\nRELATIVE_WITNESS=cwd-relative-witness.txt\n"), 0o644); err != nil {
		t.Fatalf("write cwd witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkdir -p branch-cwd-dir"},
		langgraphShellCall{Command: "cd branch-cwd-dir"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-cwd-change",
		langgraphShellCall{
			Command: "pwd_now=$(pwd); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; printf 'CLEAN_BRANCH_CWD\nPWD=/tmp/demo\nRELATIVE_WITNESS=cwd-relative-witness.txt\n' > cwd-residue-fork-check.txt; cat cwd-residue-fork-check.txt",
			Output:  "CLEAN_BRANCH_CWD\nPWD=/tmp/demo\nRELATIVE_WITNESS=cwd-relative-witness.txt\n",
		},
	)

	oracle := evaluateCWDResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean cwd fork to remain negative: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved branch-cwd-dir as the active cwd across the checkpoint boundary") {
		t.Fatalf("expected clean cwd missing reason: %#v", oracle)
	}
}

func TestEvaluateCWDResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkdir -p branch-cwd-dir"},
		langgraphShellCall{Command: "cd branch-cwd-dir"},
		langgraphShellCall{Command: "pwd", Output: "/tmp/demo/branch-cwd-dir\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-cwd-change",
		langgraphShellCall{
			Command: "pwd_now=$(pwd); printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt; printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=%s\nRELATIVE_WITNESS=cwd-relative-witness.txt\n' \"$pwd_now\" > cwd-residue-fork-check.txt; cat cwd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=cwd-relative-witness.txt\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, CWDResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant cwd task: %#v", result)
	}
}

func TestEvaluateCWDResidueForkTaskComplianceFlagsForkDirectoryChange(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "mkdir -p branch-cwd-dir"},
		langgraphShellCall{Command: "cd branch-cwd-dir"},
		langgraphShellCall{Command: "pwd", Output: "/tmp/demo/branch-cwd-dir\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-cwd-change",
		langgraphShellCall{
			Command: "cd branch-cwd-dir && printf '%s\n' 'SYNCFUZZ_CWD_RESIDUE_WITNESS' > cwd-relative-witness.txt && printf 'PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=cwd-relative-witness.txt\n' > cwd-residue-fork-check.txt && cat cwd-residue-fork-check.txt",
			Output:  "PRESENT_BRANCH_CWD_RESIDUE\nPWD=/tmp/demo/branch-cwd-dir\nRELATIVE_WITNESS=cwd-relative-witness.txt\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, CWDResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected fork-side cd to violate cwd compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "fork follow-up did not change cwd (observed 1 times)") {
		t.Fatalf("expected cwd-mutation violation: %#v", result)
	}
}

func TestEvaluateUmaskResidueForkTargetOracleConfirmed(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueForkArtifact), []byte("BASELINE_UMASK=022\nWITNESS_MODE=600\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
		langgraphShellCall{Command: "umask", Output: "0077\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=022\nWITNESS_MODE=600\n' > umask-residue-fork-check.txt; cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=022\nWITNESS_MODE=600\n",
		},
	)

	oracle := evaluateUmaskResidueForkTargetOracle(tmp, true, nil)
	if !oracle.Confirmed || oracle.Attribution != TargetOracleAttributionRuntimeResidue {
		t.Fatalf("expected confirmed umask residue: %#v", oracle)
	}
}

func TestEvaluateUmaskResidueForkTargetOracleRejectsForkSideUmaskChange(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueForkArtifact), []byte("BASELINE_UMASK=022\nWITNESS_MODE=600\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "umask 077 && : > umask-witness.txt && printf 'BASELINE_UMASK=022\nWITNESS_MODE=600\n' > umask-residue-fork-check.txt && cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=022\nWITNESS_MODE=600\n",
		},
	)

	oracle := evaluateUmaskResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected fork-side umask change to invalidate residue: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionWorkspaceRebuild {
		t.Fatalf("expected workspace rebuild attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "umask residue occurred without running umask during the fork follow-up") {
		t.Fatalf("expected umask rebuild-specific missing reason: %#v", oracle)
	}
}

func TestEvaluateUmaskResidueForkTargetOracleRecordsCleanForkNegative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueForkArtifact), []byte("BASELINE_UMASK=022\nWITNESS_MODE=644\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=022\nWITNESS_MODE=644\n' > umask-residue-fork-check.txt; cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=022\nWITNESS_MODE=644\n",
		},
	)

	oracle := evaluateUmaskResidueForkTargetOracle(tmp, true, nil)
	if oracle.Confirmed {
		t.Fatalf("expected clean umask fork to remain negative: %#v", oracle)
	}
	if oracle.Attribution != TargetOracleAttributionCleanFork {
		t.Fatalf("expected clean fork attribution: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "fork preserved the tightened branch umask across the checkpoint boundary") {
		t.Fatalf("expected clean umask missing reason: %#v", oracle)
	}
}

func TestEvaluateUmaskResidueForkTargetOracleMarksAmbiguousBaselineInconclusive(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, TargetUmaskResidueForkArtifact), []byte("BASELINE_UMASK=077\nWITNESS_MODE=600\n"), 0o644); err != nil {
		t.Fatalf("write umask witness: %v", err)
	}
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=077\nWITNESS_MODE=600\n' > umask-residue-fork-check.txt; cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=077\nWITNESS_MODE=600\n",
		},
	)

	oracle := evaluateUmaskResidueForkTargetOracle(tmp, true, nil)
	if oracle.Status != TargetOracleStatusInconclusive {
		t.Fatalf("expected ambiguous baseline to be inconclusive: %#v", oracle)
	}
	if !ContainsString(oracle.Missing, "baseline umask differed from the tightened 077 branch umask") {
		t.Fatalf("expected ambiguous baseline missing reason: %#v", oracle)
	}
}

func TestEvaluateUmaskResidueForkTaskComplianceCompliant(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
		langgraphShellCall{Command: "umask", Output: "0077\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' \"$baseline\" > umask-residue-fork-check.txt; printf 'WITNESS_MODE=%s\n' \"$mode\" >> umask-residue-fork-check.txt; cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=022\nWITNESS_MODE=600\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, UmaskResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
		t.Fatalf("expected compliant umask task: %#v", result)
	}
}

func TestEvaluateUmaskResidueForkTaskComplianceFlagsForkUmaskChange(t *testing.T) {
	tmp := t.TempDir()
	writeLangGraphShellHistory(t, tmp,
		langgraphShellCall{Command: "umask > baseline-umask.txt"},
		langgraphShellCall{Command: "umask 077"},
		langgraphShellCall{Command: "umask", Output: "0077\n"},
	)
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-umask-change",
		langgraphShellCall{
			Command: "umask 077 && baseline=$(tr -d ' \t\r\n' < baseline-umask.txt); : > umask-witness.txt; mode=$(stat -c '%a' umask-witness.txt 2>/dev/null || true); printf 'BASELINE_UMASK=%s\n' \"$baseline\" > umask-residue-fork-check.txt; printf 'WITNESS_MODE=%s\n' \"$mode\" >> umask-residue-fork-check.txt; cat umask-residue-fork-check.txt",
			Output:  "BASELINE_UMASK=022\nWITNESS_MODE=600\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, UmaskResidueForkTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected fork-side umask change to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "fork follow-up changed the shell umask") {
		t.Fatalf("expected umask-mutation violation: %#v", result)
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
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); if [ -n \"$pid\" ] && [ -r \"/proc/$pid/fd/9\" ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; readlink \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; cat \"/proc/$pid/fd/9\" >> inherited-fd-branch-leakage-check.txt; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; fi; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE_PID\n' > inherited-fd-branch-leakage-check.txt; fi; cat inherited-fd-branch-leakage-check.txt",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, InheritedFDLeakTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then pid=$(cat branch-inherited-fd-pid.txt 2>/dev/null || true); found=\"\"; for cand in \"$pid\" $(cat \"/proc/$pid/task/$pid/children\" 2>/dev/null || true); do if [ -n \"$cand\" ] && [ -r \"/proc/$cand/fd/9\" ]; then found=\"$cand\"; break; fi; done; if [ -n \"$found\" ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; readlink \"/proc/$found/fd/9\" >> inherited-fd-branch-leakage-check.txt; cat \"/proc/$found/fd/9\" >> inherited-fd-branch-leakage-check.txt; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.txt; fi; else printf 'MISSING_INHERITED_FD_BRANCH_LEAKAGE_PID\n' > inherited-fd-branch-leakage-check.txt; fi; cat inherited-fd-branch-leakage-check.txt",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, InheritedFDLeakTargetTaskID)
	if result.Status != TargetTaskComplianceStatusViolated {
		t.Fatalf("expected extra inherited-fd inspection to violate compliance: %#v", result)
	}
	if !ContainsString(result.Violations, "initial branch avoided extra process/file inspection command: ps") {
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
	writeLangGraphOperationSummary(t, tmp, LanggraphForkArtifact, "fork", true, "before-inherited-fd-leak-holder",
		langgraphShellCall{
			Command: "if [ -f branch-inherited-fd-pid.txt ]; then printf 'PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n' > inherited-fd-branch-leakage-check.tx",
			Output:  "PRESENT_INHERITED_FD_BRANCH_LEAKAGE\n/tmp/workspace/branch-inherited-fd-secret.txt (deleted)\nSYNCFUZZ_INHERITED_FD_BRANCH_SECRET\n",
		},
	)

	result := evaluateTargetTaskCompliance(tmp, InheritedFDLeakTargetTaskID)
	if result.Status != TargetTaskComplianceStatusCompliant {
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
