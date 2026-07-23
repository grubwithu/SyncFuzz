//go:build linux

package profiling

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -no-strip -tags linux resourceMonitor resource_monitor.bpf.c

const resourceMonitorEventSize = 184

// ResourceCollector records successful cgroup-scoped filesystem, FD, and IPC
// syscalls. It keeps only bounded pathname/FD facts; state probes establish
// whether a referenced resource actually persists.
type ResourceCollector struct {
	objects   resourceMonitorObjects
	links     []link.Link
	reader    *ringbuf.Reader
	closeOnce sync.Once
	closeErr  error
}

func StartResourceCollector(scope ProfilingScope) (*ResourceCollector, error) {
	if scope.CgroupID == 0 {
		return nil, fmt.Errorf("start eBPF resource collector: cgroup_id is required")
	}
	if runtime.GOARCH != "amd64" {
		return nil, fmt.Errorf("start eBPF resource collector: syscall decoder currently supports linux/amd64, got %s", runtime.GOARCH)
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("start eBPF resource collector: remove memlock limit (requires BPF privileges): %w", err)
	}

	collector := &ResourceCollector{}
	if err := loadResourceMonitorObjects(&collector.objects, nil); err != nil {
		return nil, fmt.Errorf("start eBPF resource collector: load programs (requires CAP_BPF and CAP_PERFMON or root): %w", err)
	}
	cleanup := func() { _ = collector.Close() }
	key := uint32(0)
	if err := collector.objects.TargetCgroup.Put(key, scope.CgroupID); err != nil {
		cleanup()
		return nil, fmt.Errorf("start eBPF resource collector: set cgroup filter: %w", err)
	}
	attachments := []struct {
		group string
		name  string
		prog  *ebpf.Program
	}{
		{group: "raw_syscalls", name: "sys_enter", prog: collector.objects.SyncfuzzResourceEnter},
		{group: "raw_syscalls", name: "sys_exit", prog: collector.objects.SyncfuzzResourceExit},
	}
	for _, attachment := range attachments {
		if attachment.prog == nil || attachment.prog.FD() < 0 {
			cleanup()
			return nil, fmt.Errorf("start eBPF resource collector: invalid program for %s/%s", attachment.group, attachment.name)
		}
		linked, err := link.Tracepoint(attachment.group, attachment.name, attachment.prog, nil)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("start eBPF resource collector: attach %s/%s (requires tracepoint access): %w", attachment.group, attachment.name, err)
		}
		collector.links = append(collector.links, linked)
	}
	reader, err := ringbuf.NewReader(collector.objects.Events)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("start eBPF resource collector: open ring buffer: %w", err)
	}
	collector.reader = reader
	return collector, nil
}

func (c *ResourceCollector) Read() (RawEvent, error) {
	if c == nil || c.reader == nil {
		return RawEvent{}, fmt.Errorf("read eBPF resource collector: collector is not running")
	}
	record, err := c.reader.Read()
	if err != nil {
		return RawEvent{}, err
	}
	event, err := decodeResourceMonitorEvent(record.RawSample)
	if err != nil {
		return RawEvent{}, err
	}
	return enrichResourceEventIdentity(event), nil
}

func (c *ResourceCollector) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() { c.closeErr = c.close() })
	return c.closeErr
}

func (c *ResourceCollector) close() error {
	var errorsOut []error
	if c.reader != nil {
		if err := c.reader.Close(); err != nil && !errors.Is(err, ringbuf.ErrClosed) {
			errorsOut = append(errorsOut, err)
		}
		c.reader = nil
	}
	for index := len(c.links) - 1; index >= 0; index-- {
		if err := c.links[index].Close(); err != nil {
			errorsOut = append(errorsOut, err)
		}
	}
	c.links = nil
	if err := c.objects.Close(); err != nil {
		errorsOut = append(errorsOut, err)
	}
	return errors.Join(errorsOut...)
}

func IsResourceCollectorClosed(err error) bool {
	return errors.Is(err, ringbuf.ErrClosed)
}

func decodeResourceMonitorEvent(raw []byte) (RawEvent, error) {
	if len(raw) != resourceMonitorEventSize {
		return RawEvent{}, fmt.Errorf("decode eBPF resource event: got %d bytes, want %d", len(raw), resourceMonitorEventSize)
	}
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
	if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, &event); err != nil {
		return RawEvent{}, fmt.Errorf("decode eBPF resource event: %w", err)
	}
	kind, err := resourceRawEventKind(event.Kind)
	if err != nil {
		return RawEvent{}, err
	}
	resource := ResourceRef{Family: resourceFamily(kind), Kind: resourceKind(kind), HolderPID: event.PID}
	if event.FD >= 0 {
		resource.FD = int(event.FD)
	}
	resource.Path = strings.TrimRight(string(event.Path[:]), "\x00")
	return RawEvent{
		EventID:     fmt.Sprintf("ebpf-resource-%d-%d-%d", event.MonotonicNS, event.PID, event.Kind),
		MonotonicNS: event.MonotonicNS,
		Kind:        kind,
		PID:         event.PID,
		CgroupID:    event.CgroupID,
		Result:      event.Result,
		Comm:        strings.TrimRight(string(event.Comm[:]), "\x00"),
		Resource:    resource,
	}, nil
}

func resourceRawEventKind(kind uint32) (RawEventKind, error) {
	switch kind {
	case 1:
		return RawEventOpenAt, nil
	case 2:
		return RawEventClose, nil
	case 3:
		return RawEventDup, nil
	case 4:
		return RawEventUnlinkAt, nil
	case 5:
		return RawEventRenameAt2, nil
	case 6:
		return RawEventLinkAt, nil
	case 7:
		return RawEventSymlinkAt, nil
	case 8:
		return RawEventMkdirAt, nil
	case 9:
		return RawEventSocket, nil
	case 10:
		return RawEventBind, nil
	case 11:
		return RawEventListen, nil
	case 12:
		return RawEventConnect, nil
	case 13:
		return RawEventAccept, nil
	case 14:
		return RawEventChdir, nil
	case 15:
		return RawEventChmod, nil
	case 16:
		return RawEventChown, nil
	case 17:
		return RawEventSetXAttr, nil
	default:
		return "", fmt.Errorf("decode eBPF resource event: unsupported event kind %d", kind)
	}
}

func resourceFamily(kind RawEventKind) StateFamily {
	switch kind {
	case RawEventOpenAt, RawEventDup, RawEventClose, RawEventSocket, RawEventAccept:
		return StateFamilyHandle
	case RawEventBind, RawEventListen, RawEventConnect:
		return StateFamilyIPC
	case RawEventChdir:
		return StateFamilyExecutionContext
	case RawEventChmod, RawEventChown, RawEventSetXAttr:
		return StateFamilyMetadataSecurity
	default:
		return StateFamilyNamespace
	}
}

func resourceKind(kind RawEventKind) string {
	return string(kind)
}
