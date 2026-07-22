package observation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// CompilePlan applies a small, explicit dependency closure. V1 deliberately
// uses static rules: the contribution is query-specific state-probe pruning,
// not dynamic eBPF program generation.
func CompilePlan(footprint ResourceFootprint) (*ObservationPlan, error) {
	if err := NormalizeFootprint(&footprint); err != nil {
		return nil, err
	}
	classes := resourceDependencyClosure(footprint.ResourceClasses)
	plan := &ObservationPlan{
		SchemaVersion:           ObservationPlanSchemaVersion,
		QueryID:                 footprint.QueryID,
		Query:                   footprint.Query,
		SourceFootprintArtifact: ResourceFootprintArtifact,
		Checkpoints: []ObservationPoint{
			ObservationBeforePlant,
			ObservationAfterPlant,
			ObservationAfterRecovery,
			ObservationAfterActivation,
		},
		FallbackFullProbe:       true,
		UnplannedResourcePolicy: "expand-once-then-full-probe",
	}
	pathsByClass := make(map[ResourceClass][]string)
	for _, path := range footprint.Paths {
		pathsByClass[path.ResourceClass] = append(pathsByClass[path.ResourceClass], filepath.ToSlash(path.Path))
	}
	for _, family := range orderedProbeFamilies(classes) {
		probe := ProbePlan{Family: family, Enabled: true, Fields: probeFields(family)}
		switch family {
		case ProbeFilesystem:
			probe.Paths = mergePaths(pathsByClass[ResourceFilesystemNamespace], pathsByClass[ResourceUnixSocket])
		case ProbeUnixSocket:
			probe.Paths = mergePaths(pathsByClass[ResourceUnixSocket])
		case ProbeProcess, ProbeFD:
			probe.ProcessSelectors = append([]ProcessFootprint{}, footprint.Processes...)
		}
		plan.ProbePlans = append(plan.ProbePlans, probe)
	}
	return plan, nil
}

func resourceDependencyClosure(initial []ResourceClass) []ResourceClass {
	seen := make(map[ResourceClass]struct{}, len(initial)+3)
	queue := append([]ResourceClass{}, initial...)
	for len(queue) > 0 {
		class := queue[0]
		queue = queue[1:]
		if _, exists := seen[class]; exists {
			continue
		}
		seen[class] = struct{}{}
		for _, dependency := range resourceDependencies(class) {
			if _, exists := seen[dependency]; !exists {
				queue = append(queue, dependency)
			}
		}
	}
	classes := make([]ResourceClass, 0, len(seen))
	for class := range seen {
		classes = append(classes, class)
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i] < classes[j] })
	return classes
}

func resourceDependencies(class ResourceClass) []ResourceClass {
	switch class {
	case ResourceUnixSocket:
		return []ResourceClass{ResourceFilesystemNamespace, ResourceProcess, ResourceFileDescriptor}
	case ResourceFileDescriptor:
		return []ResourceClass{ResourceProcess}
	default:
		return nil
	}
}

func orderedProbeFamilies(classes []ResourceClass) []ProbeFamily {
	wanted := make(map[ProbeFamily]struct{}, len(classes))
	for _, class := range classes {
		switch class {
		case ResourceFilesystemNamespace:
			wanted[ProbeFilesystem] = struct{}{}
		case ResourceProcess:
			wanted[ProbeProcess] = struct{}{}
		case ResourceFileDescriptor:
			wanted[ProbeFD] = struct{}{}
		case ResourceUnixSocket:
			wanted[ProbeUnixSocket] = struct{}{}
		case ResourceExecutionContext:
			wanted[ProbeShellContext] = struct{}{}
		}
	}
	order := []ProbeFamily{ProbeFilesystem, ProbeProcess, ProbeFD, ProbeUnixSocket, ProbeShellContext}
	out := make([]ProbeFamily, 0, len(wanted))
	for _, family := range order {
		if _, exists := wanted[family]; exists {
			out = append(out, family)
		}
	}
	return out
}

func probeFields(family ProbeFamily) []string {
	switch family {
	case ProbeFilesystem:
		return []string{"exists", "type", "inode", "mode", "size", "content_hash", "symlink_target"}
	case ProbeProcess:
		return []string{"alive", "ppid", "start_time", "process_group", "session", "command_line"}
	case ProbeFD:
		return []string{"fd_number", "target", "kind", "socket_inode", "open_state"}
	case ProbeUnixSocket:
		return []string{"exists", "inode", "listening", "owner_pid", "peer_pid", "connectivity"}
	case ProbeShellContext:
		return []string{"cwd", "path", "environment", "functions", "umask"}
	default:
		return []string{"unsupported"}
	}
}

func mergePaths(groups ...[]string) []string {
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, path := range group {
			if path != "" {
				seen[path] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func ValidatePlan(plan *ObservationPlan) error {
	if plan == nil {
		return fmt.Errorf("observation plan is required")
	}
	if plan.SchemaVersion == "" {
		plan.SchemaVersion = ObservationPlanSchemaVersion
	}
	if plan.SchemaVersion != ObservationPlanSchemaVersion {
		return fmt.Errorf("unsupported observation plan schema %q", plan.SchemaVersion)
	}
	if plan.QueryID == "" {
		return fmt.Errorf("observation plan query_id is required")
	}
	if plan.Query != nil {
		if err := NormalizeLifecycleQuery(plan.Query); err != nil {
			return err
		}
		if plan.Query.QueryID != plan.QueryID {
			return fmt.Errorf("observation plan query id %q does not match lifecycle query id %q", plan.QueryID, plan.Query.QueryID)
		}
	}
	if len(plan.Checkpoints) == 0 {
		return fmt.Errorf("observation plan checkpoints are required")
	}
	if len(plan.ProbePlans) == 0 {
		return fmt.Errorf("observation plan requires at least one probe family")
	}
	if plan.ExpansionCount < 0 {
		return fmt.Errorf("observation plan expansion_count must not be negative")
	}
	plan.LastExpansionSource = strings.TrimSpace(plan.LastExpansionSource)
	plan.LastExpansionPaths = uniqueSortedStrings(plan.LastExpansionPaths)
	return nil
}
