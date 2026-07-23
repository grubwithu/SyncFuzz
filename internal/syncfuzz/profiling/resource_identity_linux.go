//go:build linux

package profiling

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

// enrichResourceEventIdentity adds a best-effort kernel object identity to
// events that still name a live FD when the ring-buffer record is consumed.
// It deliberately leaves the syscall pathname untouched: the host-visible
// procfs target is not comparable with a container-visible /workspace path.
// Failed lookups are expected for short-lived FDs and are represented by an
// absent identity rather than a synthetic match.
func enrichResourceEventIdentity(event RawEvent) RawEvent {
	if !resourceEventHasLiveFD(event.Kind) || event.Resource.FD < 0 {
		return event
	}
	identity, err := resolveProcessFDIdentity(event.PID, event.Resource.FD)
	if err != nil {
		return event
	}
	event.Resource.Device = identity.Device
	event.Resource.Inode = identity.Inode
	event.Resource.SocketID = identity.SocketID
	return event
}

func resourceEventHasLiveFD(kind RawEventKind) bool {
	switch kind {
	case RawEventOpenAt, RawEventDup, RawEventSocket, RawEventBind, RawEventListen, RawEventConnect, RawEventAccept:
		return true
	default:
		return false
	}
}

type processFDIdentity struct {
	Device   uint64
	Inode    uint64
	SocketID string
}

func resolveProcessFDIdentity(pid uint32, fd int) (processFDIdentity, error) {
	if pid == 0 || fd < 0 {
		return processFDIdentity{}, fmt.Errorf("invalid pid/fd %d/%d", pid, fd)
	}
	path := filepath.Join("/proc", fmt.Sprintf("%d", pid), "fd", fmt.Sprintf("%d", fd))
	target, _ := os.Readlink(path)
	info, err := os.Stat(path)
	if err != nil {
		return processFDIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return processFDIdentity{}, fmt.Errorf("unexpected stat type %T", info.Sys())
	}
	return processFDIdentity{
		Device:   uint64(stat.Dev),
		Inode:    uint64(stat.Ino),
		SocketID: core.UnixSocketIDFromTarget(target),
	}, nil
}
