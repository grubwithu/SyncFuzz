// Package profiling defines the evidence contract shared by the Linux
// collector, state probes, frontier analysis, and recovery scheduler.
package profiling

import (
	"fmt"
	"sort"
	"strings"
)

const SchemaVersion = "syncfuzz.profiling.v2"

type StateFamily string

const (
	StateFamilyNamespace        StateFamily = "namespace"
	StateFamilyProcess          StateFamily = "process"
	StateFamilyHandle           StateFamily = "handle"
	StateFamilyIPC              StateFamily = "ipc"
	StateFamilyExecutionContext StateFamily = "execution-context"
	StateFamilyMetadataSecurity StateFamily = "metadata-security"
)

func (f StateFamily) valid() bool {
	switch f {
	case StateFamilyNamespace, StateFamilyProcess, StateFamilyHandle, StateFamilyIPC, StateFamilyExecutionContext, StateFamilyMetadataSecurity:
		return true
	default:
		return false
	}
}

// Valid reports whether f belongs to the bounded profiling state grammar.
// It is exported for the V2 objective IR, whose effect atoms must use the
// same family vocabulary as the normalizer and probes.
func (f StateFamily) Valid() bool {
	return f.valid()
}

// ProfilingScope identifies the isolated runtime whose events may enter a
// profile. The Linux collector will use CgroupID as its primary filter.
type ProfilingScope struct {
	RunID         string `json:"run_id"`
	Environment   string `json:"environment"`
	ContainerID   string `json:"container_id,omitempty"`
	CgroupPath    string `json:"cgroup_path,omitempty"`
	CgroupID      uint64 `json:"cgroup_id,omitempty"`
	CollectorHost string `json:"collector_host,omitempty"`
}

// CheckpointCatalog is emitted by the target adapter. MonotonicNS must share a
// clock domain with RawEvent.MonotonicNS.
type CheckpointCatalog struct {
	SchemaVersion string       `json:"schema_version"`
	RunID         string       `json:"run_id"`
	Checkpoints   []Checkpoint `json:"checkpoints"`
}

type Checkpoint struct {
	CheckpointID string `json:"checkpoint_id"`
	MonotonicNS  uint64 `json:"monotonic_ns"`
	LogicalPhase string `json:"logical_phase,omitempty"`
}

func (c CheckpointCatalog) Validate() error {
	if c.SchemaVersion != "" && c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported checkpoint catalog schema %q", c.SchemaVersion)
	}
	if strings.TrimSpace(c.RunID) == "" {
		return fmt.Errorf("checkpoint catalog run_id is required")
	}
	if len(c.Checkpoints) < 2 {
		return fmt.Errorf("checkpoint catalog requires at least two checkpoints")
	}
	seen := make(map[string]struct{}, len(c.Checkpoints))
	var previous uint64
	for index, checkpoint := range c.Checkpoints {
		if strings.TrimSpace(checkpoint.CheckpointID) == "" {
			return fmt.Errorf("checkpoint %d has an empty checkpoint_id", index)
		}
		if _, ok := seen[checkpoint.CheckpointID]; ok {
			return fmt.Errorf("duplicate checkpoint_id %q", checkpoint.CheckpointID)
		}
		if index > 0 && checkpoint.MonotonicNS <= previous {
			return fmt.Errorf("checkpoint %q is not strictly after the preceding checkpoint", checkpoint.CheckpointID)
		}
		seen[checkpoint.CheckpointID] = struct{}{}
		previous = checkpoint.MonotonicNS
	}
	return nil
}

type RawEventKind string

const (
	RawEventProcessFork   RawEventKind = "process-fork"
	RawEventProcessExec   RawEventKind = "process-exec"
	RawEventProcessExit   RawEventKind = "process-exit"
	RawEventProcessSetSID RawEventKind = "process-setsid"
	RawEventOpenAt        RawEventKind = "openat"
	RawEventDup           RawEventKind = "dup"
	RawEventClose         RawEventKind = "close"
	RawEventUnlinkAt      RawEventKind = "unlinkat"
	RawEventRenameAt2     RawEventKind = "renameat2"
	RawEventLinkAt        RawEventKind = "linkat"
	RawEventSymlinkAt     RawEventKind = "symlinkat"
	RawEventMkdirAt       RawEventKind = "mkdirat"
	RawEventSocket        RawEventKind = "socket"
	RawEventBind          RawEventKind = "bind"
	RawEventListen        RawEventKind = "listen"
	RawEventConnect       RawEventKind = "connect"
	RawEventAccept        RawEventKind = "accept"
	RawEventChdir         RawEventKind = "chdir"
	RawEventChmod         RawEventKind = "chmod"
	RawEventChown         RawEventKind = "chown"
	RawEventSetXAttr      RawEventKind = "setxattr"
)

// ResourceRef is deliberately shared by raw eBPF events and state probes.
// ResourceID is required in state summaries; raw events may only have a path,
// FD, or socket identity while the collector is still resolving the resource.
type ResourceRef struct {
	ResourceID    string      `json:"resource_id,omitempty"`
	Family        StateFamily `json:"family,omitempty"`
	Kind          string      `json:"kind,omitempty"`
	Path          string      `json:"path,omitempty"`
	CanonicalPath string      `json:"canonical_path,omitempty"`
	Device        uint64      `json:"device,omitempty"`
	Inode         uint64      `json:"inode,omitempty"`
	SocketID      string      `json:"socket_id,omitempty"`
	FD            int         `json:"fd,omitempty"`
	HolderPID     uint32      `json:"holder_pid,omitempty"`
	PeerPID       uint32      `json:"peer_pid,omitempty"`
	Deleted       bool        `json:"deleted,omitempty"`
}

type RawEvent struct {
	EventID     string       `json:"event_id"`
	MonotonicNS uint64       `json:"monotonic_ns"`
	Kind        RawEventKind `json:"kind"`
	PID         uint32       `json:"pid"`
	TID         uint32       `json:"tid,omitempty"`
	ParentPID   uint32       `json:"parent_pid,omitempty"`
	CgroupID    uint64       `json:"cgroup_id,omitempty"`
	Result      int64        `json:"result,omitempty"`
	Comm        string       `json:"comm,omitempty"`
	Resource    ResourceRef  `json:"resource,omitempty"`
}

func (e RawEvent) Validate() error {
	if strings.TrimSpace(e.EventID) == "" {
		return fmt.Errorf("raw event has an empty event_id")
	}
	if strings.TrimSpace(string(e.Kind)) == "" {
		return fmt.Errorf("raw event %q has an empty kind", e.EventID)
	}
	return nil
}

type ProcessRef struct {
	PID       uint32 `json:"pid"`
	ParentPID uint32 `json:"parent_pid,omitempty"`
	Comm      string `json:"comm,omitempty"`
}

type NormalizedEffect struct {
	EffectID             string      `json:"effect_id"`
	SourceEventID        string      `json:"source_event_id"`
	MonotonicNS          uint64      `json:"monotonic_ns"`
	Family               StateFamily `json:"family"`
	Operation            string      `json:"operation"`
	Resource             ResourceRef `json:"resource,omitempty"`
	Process              ProcessRef  `json:"process"`
	PersistencePotential bool        `json:"persistence_potential"`
}

type PersistentResource struct {
	Resource ResourceRef `json:"resource"`
	Observed bool        `json:"observed"`
}

type CheckpointStateSummary struct {
	CheckpointID string               `json:"checkpoint_id"`
	MonotonicNS  uint64               `json:"monotonic_ns"`
	Resources    []PersistentResource `json:"resources"`
	Dependencies []ResourceDependency `json:"dependencies,omitempty"`
}

// ResourceDependency makes a probe's closure explicit rather than inferring
// ownership from coincidental path or PID fields. For a Unix endpoint this
// records endpoint -> bound pathname and socket FD -> endpoint/process edges.
type ResourceDependency struct {
	FromResourceID string `json:"from_resource_id"`
	ToResourceID   string `json:"to_resource_id"`
	Relation       string `json:"relation"`
}

func ValidateCheckpointStateSummaries(catalog CheckpointCatalog, summaries []CheckpointStateSummary) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	if len(summaries) != len(catalog.Checkpoints) {
		return fmt.Errorf("state summaries contain %d checkpoints, catalog contains %d", len(summaries), len(catalog.Checkpoints))
	}
	byID := make(map[string]CheckpointStateSummary, len(summaries))
	for _, summary := range summaries {
		if strings.TrimSpace(summary.CheckpointID) == "" {
			return fmt.Errorf("state summary has an empty checkpoint_id")
		}
		if _, ok := byID[summary.CheckpointID]; ok {
			return fmt.Errorf("duplicate state summary for checkpoint %q", summary.CheckpointID)
		}
		resources := make(map[string]struct{}, len(summary.Resources))
		for _, resource := range summary.Resources {
			if !resource.Observed {
				return fmt.Errorf("resource %q at checkpoint %q is not confirmed observed", resource.Resource.ResourceID, summary.CheckpointID)
			}
			if strings.TrimSpace(resource.Resource.ResourceID) == "" {
				return fmt.Errorf("state resource at checkpoint %q has an empty resource_id", summary.CheckpointID)
			}
			if !resource.Resource.Family.valid() {
				return fmt.Errorf("state resource %q at checkpoint %q has invalid family %q", resource.Resource.ResourceID, summary.CheckpointID, resource.Resource.Family)
			}
			if _, ok := resources[resource.Resource.ResourceID]; ok {
				return fmt.Errorf("duplicate state resource %q at checkpoint %q", resource.Resource.ResourceID, summary.CheckpointID)
			}
			resources[resource.Resource.ResourceID] = struct{}{}
		}
		dependencies := make(map[string]struct{}, len(summary.Dependencies))
		for _, dependency := range summary.Dependencies {
			if strings.TrimSpace(dependency.FromResourceID) == "" || strings.TrimSpace(dependency.ToResourceID) == "" || strings.TrimSpace(dependency.Relation) == "" {
				return fmt.Errorf("state dependency at checkpoint %q is incomplete", summary.CheckpointID)
			}
			if _, ok := resources[dependency.FromResourceID]; !ok {
				return fmt.Errorf("state dependency source %q at checkpoint %q is not an observed resource", dependency.FromResourceID, summary.CheckpointID)
			}
			if _, ok := resources[dependency.ToResourceID]; !ok {
				return fmt.Errorf("state dependency target %q at checkpoint %q is not an observed resource", dependency.ToResourceID, summary.CheckpointID)
			}
			key := dependency.FromResourceID + "\x00" + dependency.ToResourceID + "\x00" + dependency.Relation
			if _, ok := dependencies[key]; ok {
				return fmt.Errorf("duplicate state dependency %q -> %q at checkpoint %q", dependency.FromResourceID, dependency.ToResourceID, summary.CheckpointID)
			}
			dependencies[key] = struct{}{}
		}
		byID[summary.CheckpointID] = summary
	}
	for _, checkpoint := range catalog.Checkpoints {
		summary, ok := byID[checkpoint.CheckpointID]
		if !ok {
			return fmt.Errorf("missing state summary for checkpoint %q", checkpoint.CheckpointID)
		}
		if summary.MonotonicNS != checkpoint.MonotonicNS {
			return fmt.Errorf("state summary %q monotonic_ns does not match checkpoint catalog", checkpoint.CheckpointID)
		}
	}
	return nil
}

func sortedEffects(effects []NormalizedEffect) []NormalizedEffect {
	out := append([]NormalizedEffect{}, effects...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].MonotonicNS == out[j].MonotonicNS {
			return out[i].EffectID < out[j].EffectID
		}
		return out[i].MonotonicNS < out[j].MonotonicNS
	})
	return out
}
