//go:build linux

package profiling

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDecodeResourceMonitorOpenAtEvent(t *testing.T) {
	var event struct {
		MonotonicNS uint64
		CgroupID    uint64
		Result      int64
		PID         uint32
		Kind        uint32
		FD          int32
		Comm        [16]byte
		Path        [128]byte
		Reserved    uint32
	}
	event.MonotonicNS = 123
	event.CgroupID = 456
	event.Result = 9
	event.PID = 100
	event.Kind = 1
	event.FD = 9
	copy(event.Comm[:], "touch")
	copy(event.Path[:], "frontier-marker")
	var encoded bytes.Buffer
	if err := binary.Write(&encoded, binary.LittleEndian, event); err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeResourceMonitorEvent(encoded.Bytes())
	if err != nil {
		t.Fatalf("decodeResourceMonitorEvent returned error: %v", err)
	}
	if decoded.Kind != RawEventOpenAt || decoded.Result != 9 || decoded.Resource.FD != 9 || decoded.Resource.Path != "frontier-marker" || decoded.Resource.Family != StateFamilyHandle || decoded.Comm != "touch" {
		t.Fatalf("unexpected decoded resource event: %#v", decoded)
	}
}

func TestDecodeResourceMonitorEventRejectsUnknownKind(t *testing.T) {
	raw := make([]byte, resourceMonitorEventSize)
	binary.LittleEndian.PutUint32(raw[28:32], 99)
	_, err := decodeResourceMonitorEvent(raw)
	if err == nil || !strings.Contains(err.Error(), "unsupported event kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestStartResourceCollectorRejectsUnscopedCollection(t *testing.T) {
	_, err := StartResourceCollector(ProfilingScope{})
	if err == nil || !strings.Contains(err.Error(), "cgroup_id is required") {
		t.Fatalf("expected required cgroup error, got %v", err)
	}
}

func TestEnrichResourceEventIdentityUsesLiveFD(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "identity-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()
	event := enrichResourceEventIdentity(RawEvent{
		Kind: RawEventOpenAt,
		PID:  uint32(os.Getpid()),
		Resource: ResourceRef{
			FD: int(file.Fd()),
		},
	})
	if event.Resource.Device == 0 || event.Resource.Inode == 0 {
		t.Fatalf("expected live device/inode identity, got %#v", event)
	}
}

func TestEnrichResourceEventIdentityUsesUnixSocketID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("Unix socket creation unavailable in this test sandbox: %v", err)
		}
		t.Fatalf("listen Unix socket: %v", err)
	}
	defer listener.Close()
	file, err := listener.File()
	if err != nil {
		t.Fatalf("duplicate listener FD: %v", err)
	}
	defer file.Close()
	event := enrichResourceEventIdentity(RawEvent{
		Kind: RawEventListen,
		PID:  uint32(os.Getpid()),
		Resource: ResourceRef{
			FD: int(file.Fd()),
		},
	})
	if event.Resource.SocketID == "" || event.Resource.Inode == 0 {
		t.Fatalf("expected Unix socket identity, got %#v", event)
	}
}
