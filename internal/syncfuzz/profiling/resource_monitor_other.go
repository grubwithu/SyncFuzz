//go:build !linux

package profiling

import "fmt"

type ResourceCollector struct{}

func StartResourceCollector(scope ProfilingScope) (*ResourceCollector, error) {
	return nil, fmt.Errorf("start eBPF resource collector: Linux is required")
}

func (c *ResourceCollector) Read() (RawEvent, error) {
	return RawEvent{}, fmt.Errorf("read eBPF resource collector: Linux is required")
}

func (c *ResourceCollector) Close() error { return nil }

func IsResourceCollectorClosed(err error) bool { return false }
