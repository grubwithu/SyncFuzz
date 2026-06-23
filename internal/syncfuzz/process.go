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
	PID              int      `json:"pid"`
	PPID             int      `json:"ppid"`
	Name             string   `json:"name"`
	State            string   `json:"state"`
	CWD              string   `json:"cwd,omitempty"`
	Cmdline          []string `json:"cmdline,omitempty"`
	RawCmdline       string   `json:"raw_cmdline,omitempty"`
	WorkspaceRelated bool     `json:"workspace_related"`
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
	return ProcessEntry{
		PID:              pid,
		PPID:             ppid,
		Name:             name,
		State:            state,
		CWD:              cwd,
		Cmdline:          cmdline,
		RawCmdline:       rawCmdline,
		WorkspaceRelated: isWorkspaceRelated(cwd, workspace),
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

func isWorkspaceRelated(cwd string, workspace string) bool {
	if cwd == "" || workspace == "" {
		return false
	}
	cleanCWD := filepath.Clean(cwd)
	cleanWorkspace := filepath.Clean(workspace)
	return cleanCWD == cleanWorkspace || strings.HasPrefix(cleanCWD, cleanWorkspace+string(os.PathSeparator))
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
