//go:build linux

package profiling

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestDecodeProcessMonitorForkEvent(t *testing.T) {
	var event struct {
		MonotonicNS uint64
		CgroupID    uint64
		PID         uint32
		ParentPID   uint32
		ChildPID    uint32
		Kind        uint32
		Comm        [16]byte
	}
	event.MonotonicNS = 123
	event.CgroupID = 456
	event.PID = 400
	event.ParentPID = 400
	event.ChildPID = 401
	event.Kind = 1
	copy(event.Comm[:], "listener-daemon")
	var encoded bytes.Buffer
	if err := binary.Write(&encoded, binary.LittleEndian, event); err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeProcessMonitorEvent(encoded.Bytes())
	if err != nil {
		t.Fatalf("decodeProcessMonitorEvent returned error: %v", err)
	}
	if decoded.Kind != RawEventProcessFork || decoded.PID != 401 || decoded.ParentPID != 400 || decoded.CgroupID != 456 || decoded.Comm != "listener-daemon" {
		t.Fatalf("unexpected decoded process event: %#v", decoded)
	}
}

func TestDecodeProcessMonitorEventRejectsUnknownKind(t *testing.T) {
	raw := make([]byte, processMonitorEventSize)
	binary.LittleEndian.PutUint32(raw[28:32], 99)
	_, err := decodeProcessMonitorEvent(raw)
	if err == nil || !strings.Contains(err.Error(), "unsupported event kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestStartProcessCollectorRejectsUnscopedCollection(t *testing.T) {
	_, err := StartProcessCollector(ProfilingScope{})
	if err == nil || !strings.Contains(err.Error(), "cgroup_id is required") {
		t.Fatalf("expected required cgroup error, got %v", err)
	}
}

func TestParseCgroupV2Path(t *testing.T) {
	path, err := ParseCgroupV2Path("12:devices:/user.slice\n0::/system.slice/docker-abc.scope\n")
	if err != nil {
		t.Fatalf("ParseCgroupV2Path returned error: %v", err)
	}
	if path != "/system.slice/docker-abc.scope" {
		t.Fatalf("unexpected cgroup path %q", path)
	}
	if _, err := ParseCgroupV2Path("12:devices:/user.slice\n"); err == nil {
		t.Fatal("expected v1-only cgroup data to fail")
	}
}

func TestCgroupV2PathCandidatesIncludeTargetMountNamespace(t *testing.T) {
	paths := cgroupV2PathCandidates(1234, "docker.slice/docker-abc.scope")
	if len(paths) != 2 {
		t.Fatalf("expected collector and target-mount candidates, got %#v", paths)
	}
	if paths[0] != "/sys/fs/cgroup/docker.slice/docker-abc.scope" {
		t.Fatalf("unexpected collector candidate %q", paths[0])
	}
	if paths[1] != "/proc/1234/root/sys/fs/cgroup/docker.slice/docker-abc.scope" {
		t.Fatalf("unexpected target mount candidate %q", paths[1])
	}
}

func TestParseCgroupV2MountInfoAndResolveHybridMount(t *testing.T) {
	mounts := parseCgroupV2MountInfo(`35 24 0:30 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw`)
	if len(mounts) != 1 {
		t.Fatalf("expected one cgroup2 mount, got %#v", mounts)
	}
	if mounts[0].Root != "/" || mounts[0].MountPoint != "/sys/fs/cgroup/unified" {
		t.Fatalf("unexpected cgroup2 mount %#v", mounts[0])
	}
	paths := cgroupV2PathsForMounts(mounts, nil, "/docker.slice/docker-a.scope")
	if len(paths) != 1 || paths[0] != "/sys/fs/cgroup/unified/docker.slice/docker-a.scope" {
		t.Fatalf("unexpected hybrid cgroup path candidates %#v", paths)
	}
	relative, ok := cgroupPathRelativeToMount("/docker.slice/docker-a.scope", mounts[0].Root)
	if !ok || relative != "docker.slice/docker-a.scope" {
		t.Fatalf("unexpected cgroup relative path: relative=%q ok=%t", relative, ok)
	}
}

func TestCgroupPathRelativeToNestedMount(t *testing.T) {
	relative, ok := cgroupPathRelativeToMount("/machine.slice/guest.scope/workload", "/machine.slice/guest.scope")
	if !ok || relative != "workload" {
		t.Fatalf("unexpected nested cgroup relative path: relative=%q ok=%t", relative, ok)
	}
	if _, ok := cgroupPathRelativeToMount("/other.slice/workload", "/machine.slice/guest.scope"); ok {
		t.Fatal("expected unrelated cgroup path to be rejected")
	}
}
