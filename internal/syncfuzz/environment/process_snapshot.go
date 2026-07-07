package environment

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

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func snapshotLocalProcesses(run *core.RunContext) (core.ProcessSnapshot, error) {
	workspace, err := filepath.Abs(run.Workspace)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	entries, err := readProcEntries("/proc", workspace, false)
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	return core.ProcessSnapshot{
		Environment: run.Environment,
		Workspace:   run.Workspace,
		CapturedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Processes:   entries,
	}, nil
}

func snapshotContainerProcesses(ctx context.Context, run *core.RunContext) (core.ProcessSnapshot, error) {
	if run.ContainerName == "" {
		return core.ProcessSnapshot{}, fmt.Errorf("container process snapshot requires a running container")
	}
	output, err := exec.CommandContext(ctx, "docker", "exec", run.ContainerName, "bash", "-lc", containerProcessScript()).CombinedOutput()
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("snapshot container processes: %w: %s", err, strings.TrimSpace(string(output)))
	}
	entries, err := ParseContainerProcessLines(string(output))
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	return core.ProcessSnapshot{
		Environment:    run.Environment,
		ContainerName:  run.ContainerName,
		ContainerImage: run.ContainerImage,
		Workspace:      "/workspace",
		CapturedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Processes:      entries,
	}, nil
}

func readProcEntries(procRoot string, workspace string, includeAll bool) ([]core.ProcessEntry, error) {
	dirs, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read proc root: %w", err)
	}

	var entries []core.ProcessEntry
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

func readProcEntry(procRoot string, pid int, workspace string) (core.ProcessEntry, error) {
	procDir := filepath.Join(procRoot, strconv.Itoa(pid))
	name, state, ppid, err := readProcStatus(filepath.Join(procDir, "status"))
	if err != nil {
		return core.ProcessEntry{}, err
	}
	cwd, _ := os.Readlink(filepath.Join(procDir, "cwd"))
	rawCmdline, cmdline := readProcCmdline(filepath.Join(procDir, "cmdline"))
	if rawCmdline == "" {
		rawCmdline = name
	}
	openFDs, hasWorkspaceFD := readProcFDs(procDir, workspace)
	return core.ProcessEntry{
		PID:              pid,
		PPID:             ppid,
		Name:             name,
		State:            state,
		CWD:              cwd,
		Cmdline:          cmdline,
		RawCmdline:       rawCmdline,
		OpenFDs:          openFDs,
		WorkspaceRelated: IsWorkspaceRelated(cwd, workspace) || hasWorkspaceFD,
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

func readProcFDs(procDir string, workspace string) ([]core.ProcessFDEntry, bool) {
	fdDir := filepath.Join(procDir, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, false
	}

	var out []core.ProcessFDEntry
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
		out = append(out, core.ProcessFDEntry{
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

func IsWorkspaceRelated(cwd string, workspace string) bool {
	if cwd == "" || workspace == "" {
		return false
	}
	for _, cwdCandidate := range core.PathCandidates(cwd) {
		for _, workspaceCandidate := range core.PathCandidates(workspace) {
			if core.IsSameOrChildPath(cwdCandidate, workspaceCandidate) {
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
	for _, targetCandidate := range core.PathCandidates(trimmed) {
		for _, workspaceCandidate := range core.PathCandidates(workspace) {
			if core.IsSameOrChildPath(targetCandidate, workspaceCandidate) {
				return true
			}
		}
	}
	return false
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

func ParseContainerProcessLines(output string) ([]core.ProcessEntry, error) {
	var entries []core.ProcessEntry
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
		entries = append(entries, core.ProcessEntry{
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
