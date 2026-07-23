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
	TargetEBPFProcessScopeArtifact  = "ebpf-process-scope.json"
	TargetEBPFProcessEventsArtifact = "ebpf-process-events.jsonl"
)

type TargetProcessProfilingResult struct {
	Collector  string                   `json:"collector"`
	Scope      profiling.ProfilingScope `json:"scope"`
	EventCount int                      `json:"event_count"`
	Events     []profiling.RawEvent     `json:"-"`
}

type targetProcessProfiler struct {
	scope     profiling.ProfilingScope
	collector *profiling.ProcessCollector

	mu       sync.Mutex
	events   []profiling.RawEvent
	readErr  error
	done     chan struct{}
	stopOnce sync.Once
	result   *TargetProcessProfilingResult
	stopErr  error
}

func startTargetProcessProfiler(ctx context.Context, run *core.RunContext) (*targetProcessProfiler, error) {
	if run == nil || run.Environment != "container" || run.ContainerName == "" {
		return nil, fmt.Errorf("start target eBPF process profiler: a running container environment is required")
	}
	scope, err := environment.ResolveContainerProfilingScope(ctx, run.ContainerName, run.RunID)
	if err != nil {
		return nil, fmt.Errorf("start target eBPF process profiler: %w", err)
	}
	collector, err := profiling.StartProcessCollector(scope)
	if err != nil {
		return nil, fmt.Errorf("start target eBPF process profiler: %w", err)
	}
	profiler := &targetProcessProfiler{
		scope:     scope,
		collector: collector,
		done:      make(chan struct{}),
	}
	go profiler.collect()
	return profiler, nil
}

func (p *targetProcessProfiler) Scope() profiling.ProfilingScope {
	if p == nil {
		return profiling.ProfilingScope{}
	}
	return p.scope
}

func (p *targetProcessProfiler) collect() {
	defer close(p.done)
	for {
		event, err := p.collector.Read()
		if err != nil {
			if !profiling.IsProcessCollectorClosed(err) {
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

func (p *targetProcessProfiler) Stop() (*TargetProcessProfilingResult, error) {
	if p == nil {
		return nil, nil
	}
	p.stopOnce.Do(func() {
		if err := p.collector.Close(); err != nil {
			p.stopErr = fmt.Errorf("stop target eBPF process profiler: %w", err)
			return
		}
		<-p.done
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.readErr != nil {
			p.stopErr = fmt.Errorf("read target eBPF process profiler: %w", p.readErr)
			return
		}
		events := append([]profiling.RawEvent{}, p.events...)
		p.result = &TargetProcessProfilingResult{
			Collector:  "syncfuzz-ebpf-process-v1",
			Scope:      p.scope,
			EventCount: len(events),
			Events:     events,
		}
	})
	return p.result, p.stopErr
}
