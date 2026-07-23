//go:build !linux

package profiling

import "fmt"

type ProcessCollector struct{}

func StartProcessCollector(ProfilingScope) (*ProcessCollector, error) {
	return nil, fmt.Errorf("eBPF process collector requires Linux")
}

func (c *ProcessCollector) Read() (RawEvent, error) {
	return RawEvent{}, fmt.Errorf("eBPF process collector requires Linux")
}

func (c *ProcessCollector) Close() error { return nil }

func IsProcessCollectorClosed(error) bool { return false }
