//go:build linux

package profiling

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -no-strip -tags linux processMonitor process_monitor.bpf.c

const processMonitorEventSize = 48

type ProcessCollector struct {
	objects   processMonitorObjects
	links     []link.Link
	reader    *ringbuf.Reader
	closeOnce sync.Once
	closeErr  error
}

// StartProcessCollector attaches process lifecycle tracepoints on the host and
// emits only events whose current cgroup matches scope.CgroupID. The caller is
// responsible for supplying a dedicated container cgroup; a zero value would
// turn a missing scope into a host-wide observation error, so it is rejected.
func StartProcessCollector(scope ProfilingScope) (*ProcessCollector, error) {
	if scope.CgroupID == 0 {
		return nil, fmt.Errorf("start eBPF process collector: cgroup_id is required")
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("start eBPF process collector: remove memlock limit (requires BPF privileges): %w", err)
	}

	collector := &ProcessCollector{}
	if err := loadProcessMonitorObjects(&collector.objects, nil); err != nil {
		return nil, fmt.Errorf("start eBPF process collector: load programs (requires CAP_BPF and CAP_PERFMON or root): %w", err)
	}
	cleanup := func() { _ = collector.Close() }
	key := uint32(0)
	if err := collector.objects.TargetCgroup.Put(key, scope.CgroupID); err != nil {
		cleanup()
		return nil, fmt.Errorf("start eBPF process collector: set cgroup filter: %w", err)
	}

	attachments := []struct {
		group string
		name  string
		prog  *ebpf.Program
	}{
		{group: "sched", name: "sched_process_fork", prog: collector.objects.SyncfuzzProcessFork},
		{group: "sched", name: "sched_process_exec", prog: collector.objects.SyncfuzzProcessExec},
		{group: "sched", name: "sched_process_exit", prog: collector.objects.SyncfuzzProcessExit},
	}
	for _, attachment := range attachments {
		if attachment.prog == nil || attachment.prog.FD() < 0 {
			cleanup()
			return nil, fmt.Errorf("start eBPF process collector: invalid program for %s/%s", attachment.group, attachment.name)
		}
		linked, err := link.Tracepoint(attachment.group, attachment.name, attachment.prog, nil)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("start eBPF process collector: attach %s/%s (requires tracepoint access): %w", attachment.group, attachment.name, err)
		}
		collector.links = append(collector.links, linked)
	}
	reader, err := ringbuf.NewReader(collector.objects.Events)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("start eBPF process collector: open ring buffer: %w", err)
	}
	collector.reader = reader
	return collector, nil
}

// Read blocks until the collector observes an attributed process lifecycle
// event. Closing the collector unblocks Read with ringbuf.ErrClosed.
func (c *ProcessCollector) Read() (RawEvent, error) {
	if c == nil || c.reader == nil {
		return RawEvent{}, fmt.Errorf("read eBPF process collector: collector is not running")
	}
	record, err := c.reader.Read()
	if err != nil {
		return RawEvent{}, err
	}
	return decodeProcessMonitorEvent(record.RawSample)
}

func (c *ProcessCollector) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closeErr = c.close()
	})
	return c.closeErr
}

func (c *ProcessCollector) close() error {
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

func IsProcessCollectorClosed(err error) bool {
	return errors.Is(err, ringbuf.ErrClosed)
}

func decodeProcessMonitorEvent(raw []byte) (RawEvent, error) {
	if len(raw) != processMonitorEventSize {
		return RawEvent{}, fmt.Errorf("decode eBPF process event: got %d bytes, want %d", len(raw), processMonitorEventSize)
	}
	var event struct {
		MonotonicNS uint64
		CgroupID    uint64
		PID         uint32
		ParentPID   uint32
		ChildPID    uint32
		Kind        uint32
		Comm        [16]byte
	}
	if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, &event); err != nil {
		return RawEvent{}, fmt.Errorf("decode eBPF process event: %w", err)
	}
	kind, err := processRawEventKind(event.Kind)
	if err != nil {
		return RawEvent{}, err
	}
	pid := event.PID
	if kind == RawEventProcessFork && event.ChildPID != 0 {
		pid = event.ChildPID
	}
	return RawEvent{
		EventID:     fmt.Sprintf("ebpf-process-%d-%d-%d", event.MonotonicNS, pid, event.Kind),
		MonotonicNS: event.MonotonicNS,
		Kind:        kind,
		PID:         pid,
		ParentPID:   event.ParentPID,
		CgroupID:    event.CgroupID,
		Comm:        strings.TrimRight(string(event.Comm[:]), "\x00"),
		Resource:    ResourceRef{Family: StateFamilyProcess, Kind: "process", HolderPID: pid},
	}, nil
}

func processRawEventKind(kind uint32) (RawEventKind, error) {
	switch kind {
	case 1:
		return RawEventProcessFork, nil
	case 2:
		return RawEventProcessExec, nil
	case 3:
		return RawEventProcessExit, nil
	default:
		return "", fmt.Errorf("decode eBPF process event: unsupported event kind %d", kind)
	}
}
