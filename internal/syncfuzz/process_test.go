package syncfuzz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalProcessSnapshotFindsWorkspaceProcess(t *testing.T) {
	tmp := t.TempDir()
	env, err := newEnvironment("local", "")
	if err != nil {
		t.Fatalf("newEnvironment failed: %v", err)
	}
	run, err := env.PrepareRun(context.Background(), RunOptions{
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

	var snapshot ProcessSnapshot
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

func TestParseContainerProcessLines(t *testing.T) {
	output := "1\t0\tS (sleeping)\tsleep\t/workspace\ttrue\tsleep infinity \n"
	entries, err := parseContainerProcessLines(output)
	if err != nil {
		t.Fatalf("parseContainerProcessLines failed: %v", err)
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
}

func TestParseContainerProcessLinesFiltersProbeProcess(t *testing.T) {
	output := "7\t0\tR (running)\tbash\t/workspace\ttrue\tbash -lc workspace_related=false printf '%s\\t%s\\t%s\\t%s\\t%s\\t%s\\t%s\\n'\n"
	entries, err := parseContainerProcessLines(output)
	if err != nil {
		t.Fatalf("parseContainerProcessLines failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected probe process to be filtered, got %#v", entries)
	}
}

func TestAnalyzeProcessLineage(t *testing.T) {
	before := ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
		},
	}
	boundary := ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
			{PID: 20, PPID: 10, Name: "sleep", RawCmdline: "sleep 1", WorkspaceRelated: true},
			{PID: 30, PPID: 1, Name: "helper", RawCmdline: "helper", WorkspaceRelated: false},
		},
	}
	after := ProcessSnapshot{
		Environment: "local",
		Workspace:   "/workspace",
		Processes: []ProcessEntry{
			{PID: 10, PPID: 1, Name: "bash", RawCmdline: "bash", WorkspaceRelated: true},
			{PID: 30, PPID: 1, Name: "helper", RawCmdline: "helper", WorkspaceRelated: false},
		},
	}

	report := AnalyzeProcessLineage(before, boundary, after, "before.json", "boundary.json", "after.json")

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
	cases := map[string][]string{
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

	for caseName, artifacts := range cases {
		t.Run(caseName, func(t *testing.T) {
			result, err := Run(context.Background(), RunOptions{
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

func containsSleepProcess(snapshot ProcessSnapshot) bool {
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

func hasProcessPID(processes []ProcessEntry, pid int) bool {
	for _, process := range processes {
		if process.PID == pid {
			return true
		}
	}
	return false
}

func hasProcessEdge(edges []ProcessEdge, parentPID int, childPID int) bool {
	for _, edge := range edges {
		if edge.ParentPID == parentPID && edge.ChildPID == childPID {
			return true
		}
	}
	return false
}
