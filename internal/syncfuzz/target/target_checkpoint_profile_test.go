package target

import (
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

func TestTargetCheckpointStateSummaryExcludesZombiesAndRecordsObservedResources(t *testing.T) {
	summary := targetCheckpointStateSummary(
		profiling.Checkpoint{CheckpointID: "after-command", MonotonicNS: 42},
		"/workspace",
		core.Snapshot{Files: []core.FileEntry{{Path: "result.txt", Type: "file"}, {Path: "listener.sock", Type: "socket"}}},
		core.ProcessSnapshot{Processes: []core.ProcessEntry{
			{PID: 1, State: "S (sleeping)", OpenFDs: []core.ProcessFDEntry{{FD: 9, Target: "/workspace/secret (deleted)", Kind: "deleted-path", Device: 42, Inode: 99, Deleted: true}}},
			{PID: 2, State: "Z (zombie)"},
		}},
	)
	if summary.CheckpointID != "after-command" || summary.MonotonicNS != 42 {
		t.Fatalf("unexpected summary header %#v", summary)
	}
	byID := make(map[string]profiling.PersistentResource)
	for _, resource := range summary.Resources {
		if !resource.Observed {
			t.Fatalf("resource was not confirmed observed: %#v", resource)
		}
		byID[resource.Resource.ResourceID] = resource
	}
	if file := byID["workspace:result.txt"].Resource; file.Family != profiling.StateFamilyNamespace || file.CanonicalPath != "/workspace/result.txt" {
		t.Fatalf("missing workspace file resource: %#v", byID)
	}
	if byID["workspace:listener.sock"].Resource.Family != profiling.StateFamilyNamespace {
		t.Fatalf("missing workspace socket resource: %#v", byID)
	}
	if _, ok := byID["container-process:2"]; ok {
		t.Fatalf("zombie process must not become a persistent resource: %#v", byID)
	}
	if fd, ok := byID["container-fd:1:9:device:42:inode:99"]; !ok || fd.Resource.Family != profiling.StateFamilyHandle || fd.Resource.CanonicalPath != "/workspace/secret" || !fd.Resource.Deleted {
		t.Fatalf("missing observed handle resource: %#v", byID)
	}
}

func TestTargetCheckpointStateSummaryClosesUnixSocketDependencies(t *testing.T) {
	summary := targetCheckpointStateSummary(
		profiling.Checkpoint{CheckpointID: "after-command", MonotonicNS: 42},
		"/workspace",
		core.Snapshot{Files: []core.FileEntry{{Path: "listener.sock", Type: "socket"}}},
		core.ProcessSnapshot{
			UnixSockets: []core.UnixSocketEntry{{SocketID: "socket:123", Inode: 123, Path: "/workspace/listener.sock", Type: "0001", State: "01", WorkspaceRelated: true}},
			Processes: []core.ProcessEntry{{
				PID:   7,
				State: "S (sleeping)",
				OpenFDs: []core.ProcessFDEntry{{
					FD: 3, Target: "socket:[123]", Kind: "socket", Device: 9, Inode: 123, SocketID: "socket:123", WorkspaceRelated: true,
				}},
			}},
		},
	)
	byID := make(map[string]profiling.PersistentResource)
	for _, resource := range summary.Resources {
		byID[resource.Resource.ResourceID] = resource
	}
	endpoint, ok := byID["unix-socket:socket:123"]
	if !ok || endpoint.Resource.Family != profiling.StateFamilyIPC || endpoint.Resource.Kind != "unix-listener" || endpoint.Resource.SocketID != "socket:123" {
		t.Fatalf("missing Unix endpoint resource: %#v", byID)
	}
	fdID := "container-fd:7:3:device:9:inode:123"
	if fd, ok := byID[fdID]; !ok || fd.Resource.SocketID != "socket:123" {
		t.Fatalf("missing Unix socket FD resource: %#v", byID)
	}
	wantDependencies := map[string]bool{
		"unix-socket:socket:123\x00workspace:listener.sock\x00bound-at-path": true,
		fdID + "\x00unix-socket:socket:123\x00references-unix-socket":        true,
		fdID + "\x00container-process:7\x00held-by-process":                  true,
	}
	for _, dependency := range summary.Dependencies {
		delete(wantDependencies, targetResourceDependencyKey(dependency))
	}
	if len(wantDependencies) != 0 {
		t.Fatalf("missing Unix socket closure dependencies: %#v", wantDependencies)
	}
}
