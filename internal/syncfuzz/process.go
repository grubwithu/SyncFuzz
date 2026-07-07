package syncfuzz

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProcessSnapshot struct {
	Environment    string         `json:"environment"`
	ContainerName  string         `json:"container_name,omitempty"`
	ContainerImage string         `json:"container_image,omitempty"`
	Workspace      string         `json:"workspace,omitempty"`
	CapturedAt     string         `json:"captured_at"`
	Processes      []ProcessEntry `json:"processes"`
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
		Environment:           firstNonEmpty(boundary.Environment, before.Environment, after.Environment),
		ContainerName:         firstNonEmpty(boundary.ContainerName, before.ContainerName, after.ContainerName),
		ContainerImage:        firstNonEmpty(boundary.ContainerImage, before.ContainerImage, after.ContainerImage),
		Workspace:             firstNonEmpty(boundary.Workspace, before.Workspace, after.Workspace),
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

func snapshotLocalProcesses(run *runContext) (ProcessSnapshot, error) {
	workspace, err := filepath.Abs(run.workspace)
	if err != nil {
		return ProcessSnapshot{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	entries, err := readProcEntries("/proc", workspace, false)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return ProcessSnapshot{
		Environment: run.environment,
		Workspace:   run.workspace,
		CapturedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Processes:   entries,
	}, nil
}

func snapshotContainerProcesses(ctx context.Context, run *runContext) (ProcessSnapshot, error) {
	if run.containerName == "" {
		return ProcessSnapshot{}, fmt.Errorf("container process snapshot requires a running container")
	}
	output, err := exec.CommandContext(ctx, "docker", "exec", run.containerName, "bash", "-lc", containerProcessScript()).CombinedOutput()
	if err != nil {
		return ProcessSnapshot{}, fmt.Errorf("snapshot container processes: %w: %s", err, strings.TrimSpace(string(output)))
	}
	entries, err := parseContainerProcessLines(string(output))
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return ProcessSnapshot{
		Environment:    run.environment,
		ContainerName:  run.containerName,
		ContainerImage: run.containerImage,
		Workspace:      "/workspace",
		CapturedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Processes:      entries,
	}, nil
}

func readProcEntries(procRoot string, workspace string, includeAll bool) ([]ProcessEntry, error) {
	dirs, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read proc root: %w", err)
	}

	var entries []ProcessEntry
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(dir.Name())
		if err != nil {
			continue
		}
		entry, err := readProcEntry(procRoot, pid, workspace)
		if err != nil {
			continue
		}
		if includeAll || entry.WorkspaceRelated {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PID < entries[j].PID
	})
	return entries, nil
}

func readProcEntry(procRoot string, pid int, workspace string) (ProcessEntry, error) {
	procDir := filepath.Join(procRoot, strconv.Itoa(pid))
	name, state, ppid, err := readProcStatus(filepath.Join(procDir, "status"))
	if err != nil {
		return ProcessEntry{}, err
	}
	cwd, _ := os.Readlink(filepath.Join(procDir, "cwd"))
	rawCmdline, cmdline := readProcCmdline(filepath.Join(procDir, "cmdline"))
	if rawCmdline == "" {
		rawCmdline = name
	}
	openFDs, hasWorkspaceFD := readProcFDs(procDir, workspace)
	return ProcessEntry{
		PID:              pid,
		PPID:             ppid,
		Name:             name,
		State:            state,
		CWD:              cwd,
		Cmdline:          cmdline,
		RawCmdline:       rawCmdline,
		OpenFDs:          openFDs,
		WorkspaceRelated: isWorkspaceRelated(cwd, workspace) || hasWorkspaceFD,
	}, nil
}

func readProcStatus(path string) (string, string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", 0, err
	}
	defer f.Close()

	var name string
	var state string
	var ppid int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		case strings.HasPrefix(line, "State:"):
			state = strings.TrimSpace(strings.TrimPrefix(line, "State:"))
		case strings.HasPrefix(line, "PPid:"):
			ppid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", 0, err
	}
	if name == "" {
		return "", "", 0, fmt.Errorf("missing process name")
	}
	return name, state, ppid, nil
}

func readProcCmdline(path string) (string, []string) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return "", nil
	}
	raw = bytes.TrimRight(raw, "\x00")
	parts := bytes.Split(raw, []byte{0})
	cmdline := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			cmdline = append(cmdline, string(part))
		}
	}
	return strings.Join(cmdline, " "), cmdline
}

func readProcFDs(procDir string, workspace string) ([]ProcessFDEntry, bool) {
	fdDir := filepath.Join(procDir, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, false
	}

	var out []ProcessFDEntry
	hasWorkspaceFD := false
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil || strings.TrimSpace(target) == "" {
			continue
		}
		workspaceRelated := isWorkspaceRelatedPathTarget(target, workspace)
		if !workspaceRelated {
			continue
		}
		hasWorkspaceFD = true
		out = append(out, ProcessFDEntry{
			FD:               fd,
			Target:           target,
			Kind:             processFDKind(target),
			Deleted:          strings.HasSuffix(strings.TrimSpace(target), " (deleted)"),
			WorkspaceRelated: workspaceRelated,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FD < out[j].FD
	})
	return out, hasWorkspaceFD
}

func isWorkspaceRelated(cwd string, workspace string) bool {
	if cwd == "" || workspace == "" {
		return false
	}
	for _, cwdCandidate := range pathCandidates(cwd) {
		for _, workspaceCandidate := range pathCandidates(workspace) {
			if isSameOrChildPath(cwdCandidate, workspaceCandidate) {
				return true
			}
		}
	}
	return false
}

func isWorkspaceRelatedPathTarget(target string, workspace string) bool {
	if target == "" || workspace == "" {
		return false
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(target), " (deleted)")
	if trimmed == "" {
		return false
	}
	for _, targetCandidate := range pathCandidates(trimmed) {
		for _, workspaceCandidate := range pathCandidates(workspace) {
			if isSameOrChildPath(targetCandidate, workspaceCandidate) {
				return true
			}
		}
	}
	return false
}

func pathCandidates(path string) []string {
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
	return uniqueStrings(candidates)
}

func isSameOrChildPath(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

func processFDKind(target string) string {
	target = strings.TrimSpace(target)
	switch {
	case strings.HasPrefix(target, "socket:["):
		return "socket"
	case strings.HasPrefix(target, "pipe:["):
		return "pipe"
	case strings.HasPrefix(target, "anon_inode:"):
		return "anon_inode"
	case strings.HasSuffix(target, " (deleted)"):
		return "deleted-path"
	default:
		return "path"
	}
}

func containerProcessScript() string {
	return `for d in /proc/[0-9]*; do
pid=${d##*/}
[ -r "$d/status" ] || continue
name=
state=
ppid=
while IFS= read -r line; do
  case "$line" in
    Name:*) name=${line#Name:}; name=${name#	};;
    State:*) state=${line#State:}; state=${state#	};;
    PPid:*) ppid=${line#PPid:}; ppid=${ppid#	};;
  esac
done < "$d/status"
cwd=$(readlink "$d/cwd" 2>/dev/null || true)
cmd=$(tr '\000\t\n' '   ' < "$d/cmdline" 2>/dev/null || true)
workspace_related=false
case "$cwd" in
  /workspace|/workspace/*) workspace_related=true;;
esac
printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$pid" "$ppid" "$state" "$name" "$cwd" "$workspace_related" "$cmd"
done`
}

func parseContainerProcessLines(output string) ([]ProcessEntry, error) {
	var entries []ProcessEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 7)
		if len(parts) != 7 {
			return nil, fmt.Errorf("decode container process line: %q", line)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("decode process pid %q: %w", parts[0], err)
		}
		ppid, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		rawCmdline := strings.TrimSpace(parts[6])
		if isContainerProbeProcess(rawCmdline) {
			continue
		}
		entries = append(entries, ProcessEntry{
			PID:              pid,
			PPID:             ppid,
			State:            strings.TrimSpace(parts[2]),
			Name:             strings.TrimSpace(parts[3]),
			CWD:              strings.TrimSpace(parts[4]),
			WorkspaceRelated: strings.TrimSpace(parts[5]) == "true",
			RawCmdline:       rawCmdline,
			Cmdline:          strings.Fields(rawCmdline),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read container process output: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PID < entries[j].PID
	})
	return entries, nil
}

func isContainerProbeProcess(rawCmdline string) bool {
	return strings.Contains(rawCmdline, "workspace_related=false") &&
		strings.Contains(rawCmdline, "printf '%s\\t%s\\t%s\\t%s\\t%s\\t%s\\t%s\\n'")
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
