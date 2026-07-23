package profiling

import (
	"fmt"
	"sort"
)

type effectSpec struct {
	family               StateFamily
	operation            string
	persistencePotential bool
}

// NormalizeRawEvents converts collector-level events into a small, stable OS
// effect grammar. It never claims persistence; that comes only from a state
// summary captured at a checkpoint.
func NormalizeRawEvents(events []RawEvent) ([]NormalizedEffect, error) {
	ordered := append([]RawEvent{}, events...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].MonotonicNS == ordered[j].MonotonicNS {
			return ordered[i].EventID < ordered[j].EventID
		}
		return ordered[i].MonotonicNS < ordered[j].MonotonicNS
	})

	seen := make(map[string]struct{}, len(ordered))
	result := make([]NormalizedEffect, 0, len(ordered))
	for _, event := range ordered {
		if err := event.Validate(); err != nil {
			return nil, err
		}
		if _, ok := seen[event.EventID]; ok {
			return nil, fmt.Errorf("duplicate raw event_id %q", event.EventID)
		}
		seen[event.EventID] = struct{}{}

		specs, ok := rawEventEffectSpecs(event.Kind)
		if !ok {
			return nil, fmt.Errorf("unsupported raw event kind %q", event.Kind)
		}
		for index, spec := range specs {
			resource := event.Resource
			if resource.Family == "" {
				resource.Family = spec.family
			}
			result = append(result, NormalizedEffect{
				EffectID:             fmt.Sprintf("%s/%d", event.EventID, index+1),
				SourceEventID:        event.EventID,
				MonotonicNS:          event.MonotonicNS,
				Family:               spec.family,
				Operation:            spec.operation,
				Resource:             resource,
				Process:              ProcessRef{PID: event.PID, ParentPID: event.ParentPID, Comm: event.Comm},
				PersistencePotential: spec.persistencePotential,
			})
		}
	}
	return sortedEffects(result), nil
}

func rawEventEffectSpecs(kind RawEventKind) ([]effectSpec, bool) {
	switch kind {
	case RawEventProcessFork:
		return []effectSpec{{family: StateFamilyProcess, operation: "spawn", persistencePotential: true}}, true
	case RawEventProcessExec:
		return []effectSpec{{family: StateFamilyProcess, operation: "exec", persistencePotential: true}}, true
	case RawEventProcessExit:
		return []effectSpec{{family: StateFamilyProcess, operation: "exit"}}, true
	case RawEventProcessSetSID:
		return []effectSpec{{family: StateFamilyProcess, operation: "detach", persistencePotential: true}}, true
	case RawEventOpenAt:
		return []effectSpec{{family: StateFamilyHandle, operation: "open", persistencePotential: true}}, true
	case RawEventDup:
		return []effectSpec{{family: StateFamilyHandle, operation: "dup", persistencePotential: true}}, true
	case RawEventClose:
		return []effectSpec{{family: StateFamilyHandle, operation: "close"}}, true
	case RawEventUnlinkAt:
		return []effectSpec{{family: StateFamilyNamespace, operation: "delete", persistencePotential: true}}, true
	case RawEventRenameAt2:
		return []effectSpec{{family: StateFamilyNamespace, operation: "rename", persistencePotential: true}}, true
	case RawEventLinkAt:
		return []effectSpec{{family: StateFamilyNamespace, operation: "link", persistencePotential: true}}, true
	case RawEventSymlinkAt:
		return []effectSpec{{family: StateFamilyNamespace, operation: "symlink", persistencePotential: true}}, true
	case RawEventMkdirAt:
		return []effectSpec{{family: StateFamilyNamespace, operation: "create", persistencePotential: true}}, true
	case RawEventSocket:
		return []effectSpec{{family: StateFamilyHandle, operation: "socket", persistencePotential: true}}, true
	case RawEventBind:
		return []effectSpec{
			{family: StateFamilyNamespace, operation: "rebind", persistencePotential: true},
			{family: StateFamilyIPC, operation: "bind", persistencePotential: true},
		}, true
	case RawEventListen:
		return []effectSpec{
			{family: StateFamilyHandle, operation: "listening-fd", persistencePotential: true},
			{family: StateFamilyIPC, operation: "listen", persistencePotential: true},
		}, true
	case RawEventConnect:
		return []effectSpec{{family: StateFamilyIPC, operation: "connect"}}, true
	case RawEventAccept:
		return []effectSpec{{family: StateFamilyIPC, operation: "accept", persistencePotential: true}}, true
	case RawEventChdir:
		return []effectSpec{{family: StateFamilyExecutionContext, operation: "cwd-change", persistencePotential: true}}, true
	case RawEventChmod:
		return []effectSpec{{family: StateFamilyMetadataSecurity, operation: "chmod", persistencePotential: true}}, true
	case RawEventChown:
		return []effectSpec{{family: StateFamilyMetadataSecurity, operation: "chown", persistencePotential: true}}, true
	case RawEventSetXAttr:
		return []effectSpec{{family: StateFamilyMetadataSecurity, operation: "setxattr", persistencePotential: true}}, true
	default:
		return nil, false
	}
}
