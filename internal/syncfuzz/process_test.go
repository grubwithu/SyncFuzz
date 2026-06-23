package syncfuzz

import (
	"context"
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

func containsSleepProcess(snapshot ProcessSnapshot) bool {
	for _, process := range snapshot.Processes {
		if process.WorkspaceRelated && strings.Contains(process.RawCmdline, "sleep 2") {
			return true
		}
	}
	return false
}
