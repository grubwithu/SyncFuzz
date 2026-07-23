package target

import (
	"context"
	"fmt"
	"sync"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const (
	TargetEBPFResourceScopeArtifact  = "ebpf-resource-scope.json"
	TargetEBPFResourceEventsArtifact = "ebpf-resource-events.jsonl"
)

type TargetResourceProfilingResult struct {
	Collector  string                   `json:"collector"`
	Scope      profiling.ProfilingScope `json:"scope"`
	EventCount int                      `json:"event_count"`
	Events     []profiling.RawEvent     `json:"-"`
}

type targetResourceProfiler struct {
	scope     profiling.ProfilingScope
	collector *profiling.ResourceCollector

	mu       sync.Mutex
	events   []profiling.RawEvent
	readErr  error
	done     chan struct{}
	stopOnce sync.Once
	result   *TargetResourceProfilingResult
	stopErr  error
}

func startTargetResourceProfiler(ctx context.Context, run *core.RunContext) (*targetResourceProfiler, error) {
	if run == nil || run.Environment != "container" || run.ContainerName == "" {
		return nil, fmt.Errorf("start target eBPF resource profiler: a running container environment is required")
	}
	scope, err := environment.ResolveContainerProfilingScope(ctx, run.ContainerName, run.RunID)
	if err != nil {
		return nil, fmt.Errorf("start target eBPF resource profiler: %w", err)
	}
	return startTargetResourceProfilerForScope(scope)
}

func startTargetResourceProfilerForScope(scope profiling.ProfilingScope) (*targetResourceProfiler, error) {
	collector, err := profiling.StartResourceCollector(scope)
	if err != nil {
		return nil, fmt.Errorf("start target eBPF resource profiler: %w", err)
	}
	profiler := &targetResourceProfiler{
		scope:     scope,
		collector: collector,
		done:      make(chan struct{}),
	}
	go profiler.collect()
	return profiler, nil
}

func (p *targetResourceProfiler) Scope() profiling.ProfilingScope {
	if p == nil {
		return profiling.ProfilingScope{}
	}
	return p.scope
}

func (p *targetResourceProfiler) collect() {
	defer close(p.done)
	for {
		event, err := p.collector.Read()
		if err != nil {
			if !profiling.IsResourceCollectorClosed(err) {
				p.mu.Lock()
				p.readErr = err
				p.mu.Unlock()
			}
			return
		}
		p.mu.Lock()
		p.events = append(p.events, event)
		p.mu.Unlock()
	}
}

func (p *targetResourceProfiler) Stop() (*TargetResourceProfilingResult, error) {
	if p == nil {
		return nil, nil
	}
	p.stopOnce.Do(func() {
		if err := p.collector.Close(); err != nil {
			p.stopErr = fmt.Errorf("stop target eBPF resource profiler: %w", err)
			return
		}
		<-p.done
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.readErr != nil {
			p.stopErr = fmt.Errorf("read target eBPF resource profiler: %w", p.readErr)
			return
		}
		events := append([]profiling.RawEvent{}, p.events...)
		p.result = &TargetResourceProfilingResult{
			Collector:  "syncfuzz-ebpf-resource-v1",
			Scope:      p.scope,
			EventCount: len(events),
			Events:     events,
		}
	})
	return p.result, p.stopErr
}
