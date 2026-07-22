package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProcessSnapshot struct {
	Environment     string            `json:"environment"`
	ContainerName   string            `json:"container_name,omitempty"`
	ContainerImage  string            `json:"container_image,omitempty"`
	Workspace       string            `json:"workspace,omitempty"`
	CapturedAt      string            `json:"captured_at"`
	CollectionScope string            `json:"collection_scope,omitempty"`
	Selectors       []ProcessSelector `json:"selectors,omitempty"`
	Processes       []ProcessEntry    `json:"processes"`
}

// ProcessSelector is a stable plan-level fingerprint. It intentionally does
// not use PIDs, which are run-local and cannot be reused by a later replay.
// When both values are set, a process must match both exactly.
type ProcessSelector struct {
	Executable  string `json:"executable,omitempty"`
	CommandLine string `json:"command_line,omitempty"`
}

// NormalizeProcessSelectors canonicalizes reusable process fingerprints and
// drops empty entries. It is shared by observation-plan consumers and
// environment backends so their selection semantics cannot drift.
func NormalizeProcessSelectors(selectors []ProcessSelector) []ProcessSelector {
	byKey := make(map[string]ProcessSelector, len(selectors))
	for _, selector := range selectors {
		selector.Executable = strings.TrimSpace(selector.Executable)
		selector.CommandLine = strings.TrimSpace(selector.CommandLine)
		if selector.Executable == "" && selector.CommandLine == "" {
			continue
		}
		key := selector.Executable + "\x00" + selector.CommandLine
		byKey[key] = selector
	}
	out := make([]ProcessSelector, 0, len(byKey))
	for _, selector := range byKey {
		out = append(out, selector)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Executable != out[j].Executable {
			return out[i].Executable < out[j].Executable
		}
		return out[i].CommandLine < out[j].CommandLine
	})
	return out
}

// MatchesProcessSelector reports whether process satisfies a plan-level
// fingerprint. A blank field is a wildcard; an empty selector set matches
// nothing because callers must select that broad-collection policy explicitly.
func MatchesProcessSelector(process ProcessEntry, selectors []ProcessSelector) bool {
	for _, selector := range NormalizeProcessSelectors(selectors) {
		if selector.Executable != "" && selector.Executable != process.Name {
			continue
		}
		if selector.CommandLine != "" && selector.CommandLine != process.RawCmdline {
			continue
		}
		return true
	}
	return false
}

type ProcessEntry struct {
	PID              int              `json:"pid"`
	PPID             int              `json:"ppid"`
	Name             string           `json:"name"`
	State            string           `json:"state"`
	CWD              string           `json:"cwd,omitempty"`
	Cmdline          []string         `json:"cmdline,omitempty"`
	RawCmdline       string           `json:"raw_cmdline,omitempty"`
	OpenFDs          []ProcessFDEntry `json:"open_fds,omitempty"`
	WorkspaceRelated bool             `json:"workspace_related"`
}

type ProcessFDEntry struct {
	FD               int    `json:"fd"`
	Target           string `json:"target"`
	Kind             string `json:"kind,omitempty"`
	Deleted          bool   `json:"deleted,omitempty"`
	WorkspaceRelated bool   `json:"workspace_related"`
}

type ProcessLineageReport struct {
	Environment           string                `json:"environment"`
	ContainerName         string                `json:"container_name,omitempty"`
	ContainerImage        string                `json:"container_image,omitempty"`
	Workspace             string                `json:"workspace,omitempty"`
	GeneratedAt           string                `json:"generated_at"`
	BeforeArtifact        string                `json:"before_artifact"`
	BoundaryArtifact      string                `json:"boundary_artifact"`
	AfterArtifact         string                `json:"after_artifact"`
	Summary               ProcessLineageSummary `json:"summary"`
	NewAtBoundary         []ProcessEntry        `json:"new_at_boundary,omitempty"`
	RemainingAfter        []ProcessEntry        `json:"remaining_after,omitempty"`
	ExitedAfter           []ProcessEntry        `json:"exited_after,omitempty"`
	CarriedOverAtBoundary []ProcessEntry        `json:"carried_over_at_boundary,omitempty"`
	CarriedOverAfter      []ProcessEntry        `json:"carried_over_after,omitempty"`
	ParentChildEdges      []ProcessEdge         `json:"parent_child_edges,omitempty"`
}

type ProcessLineageSummary struct {
	BeforeCount                    int `json:"before_count"`
	BoundaryCount                  int `json:"boundary_count"`
	AfterCount                     int `json:"after_count"`
	NewAtBoundary                  int `json:"new_at_boundary"`
	RemainingAfter                 int `json:"remaining_after"`
	ExitedAfter                    int `json:"exited_after"`
	CarriedOverAtBoundary          int `json:"carried_over_at_boundary"`
	CarriedOverAfter               int `json:"carried_over_after"`
	WorkspaceNewAtBoundary         int `json:"workspace_new_at_boundary"`
	WorkspaceRemainingAfter        int `json:"workspace_remaining_after"`
	WorkspaceCarriedOverAtBoundary int `json:"workspace_carried_over_at_boundary"`
	WorkspaceCarriedOverAfter      int `json:"workspace_carried_over_after"`
	ParentChildEdges               int `json:"parent_child_edges"`
}

type ProcessEdge struct {
	ParentPID      int    `json:"parent_pid"`
	ParentName     string `json:"parent_name"`
	ParentCmdline  string `json:"parent_cmdline,omitempty"`
	ChildPID       int    `json:"child_pid"`
	ChildName      string `json:"child_name"`
	ChildCmdline   string `json:"child_cmdline,omitempty"`
	ChildWorkspace bool   `json:"child_workspace_related"`
}

func AnalyzeProcessLineage(before ProcessSnapshot, boundary ProcessSnapshot, after ProcessSnapshot, beforeArtifact string, boundaryArtifact string, afterArtifact string) ProcessLineageReport {
	beforeByPID := processEntriesByPID(before.Processes)
	boundaryByPID := processEntriesByPID(boundary.Processes)
	afterByPID := processEntriesByPID(after.Processes)

	var newAtBoundary []ProcessEntry
	var carriedOverAtBoundary []ProcessEntry
	for _, process := range boundary.Processes {
		if _, existed := beforeByPID[process.PID]; existed {
			carriedOverAtBoundary = append(carriedOverAtBoundary, process)
			continue
		}
		newAtBoundary = append(newAtBoundary, process)
	}

	var remainingAfter []ProcessEntry
	var exitedAfter []ProcessEntry
	for _, process := range newAtBoundary {
		if afterProcess, stillRunning := afterByPID[process.PID]; stillRunning {
			remainingAfter = append(remainingAfter, afterProcess)
			continue
		}
		exitedAfter = append(exitedAfter, process)
	}

	var carriedOverAfter []ProcessEntry
	for _, process := range carriedOverAtBoundary {
		if afterProcess, stillRunning := afterByPID[process.PID]; stillRunning {
			carriedOverAfter = append(carriedOverAfter, afterProcess)
		}
	}

	edges := processParentChildEdges(boundaryByPID, boundary.Processes)
	return ProcessLineageReport{
		Environment:           FirstNonEmpty(boundary.Environment, before.Environment, after.Environment),
		ContainerName:         FirstNonEmpty(boundary.ContainerName, before.ContainerName, after.ContainerName),
		ContainerImage:        FirstNonEmpty(boundary.ContainerImage, before.ContainerImage, after.ContainerImage),
		Workspace:             FirstNonEmpty(boundary.Workspace, before.Workspace, after.Workspace),
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		BeforeArtifact:        beforeArtifact,
		BoundaryArtifact:      boundaryArtifact,
		AfterArtifact:         afterArtifact,
		NewAtBoundary:         newAtBoundary,
		RemainingAfter:        remainingAfter,
		ExitedAfter:           exitedAfter,
		CarriedOverAtBoundary: carriedOverAtBoundary,
		CarriedOverAfter:      carriedOverAfter,
		ParentChildEdges:      edges,
		Summary: ProcessLineageSummary{
			BeforeCount:                    len(before.Processes),
			BoundaryCount:                  len(boundary.Processes),
			AfterCount:                     len(after.Processes),
			NewAtBoundary:                  len(newAtBoundary),
			RemainingAfter:                 len(remainingAfter),
			ExitedAfter:                    len(exitedAfter),
			CarriedOverAtBoundary:          len(carriedOverAtBoundary),
			CarriedOverAfter:               len(carriedOverAfter),
			WorkspaceNewAtBoundary:         countWorkspaceProcesses(newAtBoundary),
			WorkspaceRemainingAfter:        countWorkspaceProcesses(remainingAfter),
			WorkspaceCarriedOverAtBoundary: countWorkspaceProcesses(carriedOverAtBoundary),
			WorkspaceCarriedOverAfter:      countWorkspaceProcesses(carriedOverAfter),
			ParentChildEdges:               len(edges),
		},
	}
}

func processEntriesByPID(processes []ProcessEntry) map[int]ProcessEntry {
	entries := make(map[int]ProcessEntry, len(processes))
	for _, process := range processes {
		entries[process.PID] = process
	}
	return entries
}

func processParentChildEdges(processesByPID map[int]ProcessEntry, processes []ProcessEntry) []ProcessEdge {
	var edges []ProcessEdge
	for _, child := range processes {
		parent, ok := processesByPID[child.PPID]
		if !ok {
			continue
		}
		edges = append(edges, ProcessEdge{
			ParentPID:      parent.PID,
			ParentName:     parent.Name,
			ParentCmdline:  parent.RawCmdline,
			ChildPID:       child.PID,
			ChildName:      child.Name,
			ChildCmdline:   child.RawCmdline,
			ChildWorkspace: child.WorkspaceRelated,
		})
	}
	return edges
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func PathCandidates(path string) []string {
	path = strings.TrimSuffix(strings.TrimSpace(path), " (deleted)")
	if path == "" {
		return nil
	}
	candidates := []string{filepath.Clean(path)}
	if abs, err := filepath.Abs(path); err == nil {
		candidates = append(candidates, filepath.Clean(abs))
	}
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, filepath.Clean(realPath))
	}
	return UniqueStrings(candidates)
}

func IsSameOrChildPath(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}
