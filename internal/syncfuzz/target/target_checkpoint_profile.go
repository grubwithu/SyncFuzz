package target

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const (
	TargetCheckpointCatalogArtifact        = "checkpoint-catalog.json"
	TargetCheckpointStateSummariesArtifact = "checkpoint-state-summaries.json"
	TargetNormalizedEffectsArtifact        = "normalized-effects.json"
	TargetCheckpointEffectMapArtifact      = "checkpoint-effect-map.json"
)

// targetCheckpointStateSummary projects independently observed workspace and
// process state into the profiling schema. It deliberately excludes zombies:
// their PID is still visible in procfs but no longer represents a live,
// persistent process effect.
func targetCheckpointStateSummary(checkpoint profiling.Checkpoint, workspaceRoot string, snapshot core.Snapshot, processes core.ProcessSnapshot) profiling.CheckpointStateSummary {
	resources := make([]profiling.PersistentResource, 0, len(snapshot.Files)+len(processes.Processes)+len(processes.UnixSockets))
	seen := make(map[string]struct{})
	dependencies := make([]profiling.ResourceDependency, 0)
	seenDependencies := make(map[string]struct{})
	add := func(resource profiling.ResourceRef) {
		if _, ok := seen[resource.ResourceID]; ok {
			return
		}
		seen[resource.ResourceID] = struct{}{}
		resources = append(resources, profiling.PersistentResource{Resource: resource, Observed: true})
	}
	addDependency := func(fromResourceID string, toResourceID string, relation string) {
		if _, ok := seen[fromResourceID]; !ok {
			return
		}
		if _, ok := seen[toResourceID]; !ok {
			return
		}
		dependency := profiling.ResourceDependency{FromResourceID: fromResourceID, ToResourceID: toResourceID, Relation: relation}
		key := targetResourceDependencyKey(dependency)
		if _, ok := seenDependencies[key]; ok {
			return
		}
		seenDependencies[key] = struct{}{}
		dependencies = append(dependencies, dependency)
	}

	socketPathResourceByCanonicalPath := make(map[string]string)
	for _, entry := range snapshot.Files {
		resourceID := "workspace:" + entry.Path
		resource := profiling.ResourceRef{
			ResourceID:    resourceID,
			Family:        profiling.StateFamilyNamespace,
			Kind:          "workspace-" + entry.Type,
			Path:          entry.Path,
			CanonicalPath: profiling.CanonicalWorkspaceResourcePath(workspaceRoot, entry.Path),
		}
		add(resource)
		if entry.Type == "socket" {
			socketPathResourceByCanonicalPath[resource.CanonicalPath] = resourceID
		}
	}

	endpointResourceBySocketID := make(map[string]string)
	for _, socket := range processes.UnixSockets {
		canonicalPath := profiling.CanonicalWorkspaceResourcePath(workspaceRoot, socket.Path)
		pathResourceID, ok := socketPathResourceByCanonicalPath[canonicalPath]
		if !ok || socket.SocketID == "" {
			continue
		}
		endpointResourceID := targetUnixSocketResourceID(socket.SocketID)
		add(profiling.ResourceRef{
			ResourceID:    endpointResourceID,
			Family:        profiling.StateFamilyIPC,
			Kind:          targetUnixSocketKind(socket),
			Path:          socket.Path,
			CanonicalPath: canonicalPath,
			Inode:         socket.Inode,
			SocketID:      socket.SocketID,
		})
		endpointResourceBySocketID[socket.SocketID] = endpointResourceID
		addDependency(endpointResourceID, pathResourceID, "bound-at-path")
	}
	for _, process := range processes.Processes {
		if targetProcessIsZombie(process) {
			continue
		}
		processResourceID := fmt.Sprintf("container-process:%d", process.PID)
		add(profiling.ResourceRef{
			ResourceID: processResourceID,
			Family:     profiling.StateFamilyProcess,
			Kind:       "process",
			HolderPID:  uint32(process.PID),
		})
		for _, fd := range process.OpenFDs {
			fdResourceID := targetFDResourceID(process.PID, fd)
			add(profiling.ResourceRef{
				ResourceID:    fdResourceID,
				Family:        profiling.StateFamilyHandle,
				Kind:          firstNonEmpty(fd.Kind, "fd"),
				Path:          fd.Target,
				CanonicalPath: targetCanonicalFDPath(workspaceRoot, fd.Target),
				Device:        fd.Device,
				Inode:         fd.Inode,
				SocketID:      fd.SocketID,
				FD:            fd.FD,
				HolderPID:     uint32(process.PID),
				Deleted:       fd.Deleted,
			})
			if endpointResourceID, ok := endpointResourceBySocketID[fd.SocketID]; ok {
				addDependency(fdResourceID, endpointResourceID, "references-unix-socket")
				addDependency(fdResourceID, processResourceID, "held-by-process")
			}
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Resource.ResourceID < resources[j].Resource.ResourceID
	})
	sort.Slice(dependencies, func(i, j int) bool {
		return targetResourceDependencyKey(dependencies[i]) < targetResourceDependencyKey(dependencies[j])
	})
	return profiling.CheckpointStateSummary{
		CheckpointID: checkpoint.CheckpointID,
		MonotonicNS:  checkpoint.MonotonicNS,
		Resources:    resources,
		Dependencies: dependencies,
	}
}

func targetFDResourceID(pid int, fd core.ProcessFDEntry) string {
	if fd.Device != 0 && fd.Inode != 0 {
		return fmt.Sprintf("container-fd:%d:%d:device:%d:inode:%d", pid, fd.FD, fd.Device, fd.Inode)
	}
	return fmt.Sprintf("container-fd:%d:%d:%s", pid, fd.FD, fd.Target)
}

func targetCanonicalFDPath(workspaceRoot string, target string) string {
	target = strings.TrimSuffix(strings.TrimSpace(target), " (deleted)")
	if core.UnixSocketIDFromTarget(target) != "" {
		return ""
	}
	return profiling.CanonicalWorkspaceResourcePath(workspaceRoot, target)
}

func targetUnixSocketResourceID(socketID string) string {
	return "unix-socket:" + socketID
}

func targetUnixSocketKind(socket core.UnixSocketEntry) string {
	if socket.Type == "0001" && socket.State == "01" {
		return "unix-listener"
	}
	return "unix-endpoint"
}

func targetResourceDependencyKey(dependency profiling.ResourceDependency) string {
	return dependency.FromResourceID + "\x00" + dependency.ToResourceID + "\x00" + dependency.Relation
}

func targetProcessIsZombie(process core.ProcessEntry) bool {
	return strings.HasPrefix(strings.TrimSpace(process.State), "Z")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
