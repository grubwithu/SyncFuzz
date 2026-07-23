package environment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func TestLocalProcessSnapshotFindsWorkspaceProcess(t *testing.T) {
	tmp := t.TempDir()
	env, err := environment.NewEnvironment("local", "")
	if err != nil {
		t.Fatalf("environment.NewEnvironment failed: %v", err)
	}
	run, err := env.PrepareRun(context.Background(), core.RunOptions{
		CaseName: "process-test",
		OutDir:   filepath.Join(tmp, "runs"),
	}, time.Now().UTC(), true)
	if err != nil {
		t.Fatalf("PrepareRun failed: %v", err)
	}
	defer run.Close()

	if _, err := env.ExecShell(context.Background(), run, "nohup sh -c 'sleep 2' >/dev/null 2>&1 &"); err != nil {
		t.Fatalf("ExecShell failed: %v", err)
	}

	var snapshot core.ProcessSnapshot
	for attempt := 0; attempt < 20; attempt++ {
		snapshot, err = env.SnapshotProcesses(context.Background(), run)
		if err != nil {
			t.Fatalf("SnapshotProcesses failed: %v", err)
		}
		if containsSleepProcess(snapshot) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected workspace sleep process, got %#v", snapshot.Processes)
}

func TestLocalProcessSnapshotFindsWorkspaceFDProcess(t *testing.T) {
	tmp := t.TempDir()
	env, err := environment.NewEnvironment("local", "")
	if err != nil {
		t.Fatalf("environment.NewEnvironment failed: %v", err)
	}
	run, err := env.PrepareRun(context.Background(), core.RunOptions{
		CaseName: "process-fd-test",
		OutDir:   filepath.Join(tmp, "runs"),
	}, time.Now().UTC(), true)
	if err != nil {
		t.Fatalf("PrepareRun failed: %v", err)
	}
	defer run.Close()

	heldFile := filepath.Join(run.Workspace, "held.txt")
	if err := os.WriteFile(heldFile, []byte("held\n"), 0o644); err != nil {
		t.Fatalf("write held file: %v", err)
	}
	command := "nohup sh -c 'cd / && exec 9<" + core.ShellQuote(heldFile) + " && sleep 2' >/dev/null 2>&1 &"
	if _, err := env.ExecShell(context.Background(), run, command); err != nil {
		t.Fatalf("ExecShell failed: %v", err)
	}

	var snapshot core.ProcessSnapshot
	for attempt := 0; attempt < 20; attempt++ {
		snapshot, err = env.SnapshotProcesses(context.Background(), run)
		if err != nil {
			t.Fatalf("SnapshotProcesses failed: %v", err)
		}
		if fd, found := workspaceFD(snapshot, heldFile); found {
			if fd.Device == 0 || fd.Inode == 0 {
				t.Fatalf("workspace FD has no device/inode identity: %#v", fd)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected workspace-related fd process, got %#v", snapshot.Processes)
}

func TestParseContainerProcessLines(t *testing.T) {
	output := "P\t1\t0\tS (sleeping)\tsleep\t/workspace\ttrue\tsleep infinity \nF\t1\t9\tdeleted-path\ttrue\t42\t99\t/workspace/secret (deleted)\n"
	entries, err := environment.ParseContainerProcessLines(output)
	if err != nil {
		t.Fatalf("environment.ParseContainerProcessLines failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if !entries[0].WorkspaceRelated {
		t.Fatalf("expected workspace-related process")
	}
	if entries[0].PID != 1 || entries[0].PPID != 0 || entries[0].Name != "sleep" {
		t.Fatalf("unexpected process entry: %#v", entries[0])
	}
	if len(entries[0].OpenFDs) != 1 {
		t.Fatalf("expected one workspace FD, got %#v", entries[0])
	}
	fd := entries[0].OpenFDs[0]
	if fd.FD != 9 || fd.Device != 42 || fd.Inode != 99 || !fd.Deleted || fd.Target != "/workspace/secret (deleted)" {
		t.Fatalf("unexpected container FD: %#v", fd)
	}
}

func TestParseContainerProcessLinesFiltersProbeProcess(t *testing.T) {
	output := "7\t0\tR (running)\tbash\t/workspace\ttrue\tbash -lc workspace_related=false printf '%s\\t%s\\t%s\\t%s\\t%s\\t%s\\t%s\\n'\n"
	entries, err := environment.ParseContainerProcessLines(output)
	if err != nil {
		t.Fatalf("environment.ParseContainerProcessLines failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected probe process to be filtered, got %#v", entries)
	}
}

func TestParseContainerProcessSnapshotLinesRecordsWorkspaceUnixSocketFD(t *testing.T) {
	output := "U\t123\t0001\t01\tlistener.sock\n" +
		"P\t7\t1\tS (sleeping)\tpython\t/workspace\ttrue\tpython listener.py\n" +
		"F\t7\t3\tsocket\tfalse\t9\t123\tsocket:[123]\n"
	entries, sockets, err := environment.ParseContainerProcessSnapshotLines(output)
	if err != nil {
		t.Fatalf("environment.ParseContainerProcessSnapshotLines failed: %v", err)
	}
	if len(sockets) != 1 || sockets[0].SocketID != "socket:123" || sockets[0].Path != "listener.sock" {
		t.Fatalf("unexpected Unix socket entries: %#v", sockets)
	}
	if len(entries) != 1 || !entries[0].WorkspaceRelated || len(entries[0].OpenFDs) != 1 {
		t.Fatalf("expected socket holder to be workspace-related: %#v", entries)
	}
	fd := entries[0].OpenFDs[0]
	if fd.Kind != "socket" || fd.SocketID != "socket:123" || fd.Inode != 123 {
		t.Fatalf("unexpected Unix socket FD: %#v", fd)
	}
}

func TestWorkspaceRelatedResolvesSymlinkedWorkspace(t *testing.T) {
	tmp := t.TempDir()
	realRoot := filepath.Join(tmp, "real")
	realWorkspace := filepath.Join(realRoot, "workspace")
	if err := os.MkdirAll(realWorkspace, 0o755); err != nil {
		t.Fatalf("create real workspace: %v", err)
	}
	linkRoot := filepath.Join(tmp, "link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	symlinkWorkspace := filepath.Join(linkRoot, "workspace")

	if !environment.IsWorkspaceRelated(realWorkspace, symlinkWorkspace) {
		t.Fatalf("expected real cwd to match symlinked workspace")
	}
	if !environment.IsWorkspaceRelated(filepath.Join(realWorkspace, "subdir"), symlinkWorkspace) {
		t.Fatalf("expected child cwd to match symlinked workspace")
	}
}

func TestAnalyzeProcessLineage(t *testing.T) {
	before := core.ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []core.ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
		},
	}
	boundary := core.ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []core.ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
			{PID: 20, PPID: 10, Name: "sleep", RawCmdline: "sleep 1", WorkspaceRelated: true},
			{PID: 30, PPID: 1, Name: "helper", RawCmdline: "helper", WorkspaceRelated: false},
		},
	}
	after := core.ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []core.ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
			{PID: 30, PPID: 1, Name: "helper", RawCmdline: "helper", WorkspaceRelated: false},
		},
	}

	report := core.AnalyzeProcessLineage(before, boundary, after, "before.json", "boundary.json", "after.json")

	if report.Summary.NewAtBoundary != 2 {
		t.Fatalf("expected 2 new boundary processes, got %#v", report.Summary)
	}
	if report.Summary.RemainingAfter != 1 || !hasProcessPID(report.RemainingAfter, 30) {
		t.Fatalf("expected helper to remain after boundary, got %#v", report.RemainingAfter)
	}
	if report.Summary.ExitedAfter != 1 || !hasProcessPID(report.ExitedAfter, 20) {
		t.Fatalf("expected sleep to exit after boundary, got %#v", report.ExitedAfter)
	}
	if report.Summary.CarriedOverAfter != 1 || !hasProcessPID(report.CarriedOverAfter, 10) {
		t.Fatalf("expected shell to carry over after boundary, got %#v", report.CarriedOverAfter)
	}
	if report.Summary.WorkspaceNewAtBoundary != 1 || report.Summary.WorkspaceCarriedOverAfter != 1 {
		t.Fatalf("unexpected workspace lineage counts: %#v", report.Summary)
	}
	if !hasProcessEdge(report.ParentChildEdges, 10, 20) {
		t.Fatalf("expected parent-child edge from shell to sleep, got %#v", report.ParentChildEdges)
	}
}

func TestWorkspaceSeedsWriteProcessArtifacts(t *testing.T) {
	tmp := t.TempDir()
	caseArtifacts := map[string][]string{
		"persistent-shell-poisoning": {
			"process-before.json",
			"process-after-mutation.json",
			"process-after-replay.json",
			"process-lineage.json",
		},
		"branch-leakage": {
			"process-before.json",
			"process-branch-a.json",
			"process-after.json",
			"process-lineage.json",
		},
		"partial-filesystem-rollback": {
			"process-before.json",
			"process-mutated.json",
			"process-after.json",
			"process-lineage.json",
		},
	}

	for caseName, artifacts := range caseArtifacts {
		t.Run(caseName, func(t *testing.T) {
			result, err := cases.Run(context.Background(), core.RunOptions{
				CaseName: caseName,
				OutDir:   filepath.Join(tmp, "runs"),
			})
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if !result.Confirmed {
				t.Fatalf("expected confirmed result")
			}
			for _, artifact := range artifacts {
				if !fileExists(filepath.Join(result.ArtifactDir, artifact)) {
					t.Fatalf("expected process artifact %s", artifact)
				}
			}
		})
	}
}

func containsSleepProcess(snapshot core.ProcessSnapshot) bool {
	for _, process := range snapshot.Processes {
		if process.WorkspaceRelated && strings.Contains(process.RawCmdline, "sleep 2") {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasProcessPID(processes []core.ProcessEntry, pid int) bool {
	for _, process := range processes {
		if process.PID == pid {
			return true
		}
	}
	return false
}

func hasProcessEdge(edges []core.ProcessEdge, parentPID int, childPID int) bool {
	for _, edge := range edges {
		if edge.ParentPID == parentPID && edge.ChildPID == childPID {
			return true
		}
	}
	return false
}

func containsWorkspaceFDProcess(snapshot core.ProcessSnapshot, target string) bool {
	_, found := workspaceFD(snapshot, target)
	return found
}

func workspaceFD(snapshot core.ProcessSnapshot, target string) (core.ProcessFDEntry, bool) {
	for _, process := range snapshot.Processes {
		if !process.WorkspaceRelated {
			continue
		}
		for _, fd := range process.OpenFDs {
			if fd.WorkspaceRelated && strings.TrimSuffix(fd.Target, " (deleted)") == target {
				return fd, true
			}
		}
	}
	return core.ProcessFDEntry{}, false
}
