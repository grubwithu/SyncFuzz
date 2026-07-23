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
	unixSockets, err := readWorkspaceUnixSockets("/proc/net/unix", workspace)
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	entries, err := readProcEntries("/proc", workspace, false, unixSocketsByID(unixSockets))
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	return core.ProcessSnapshot{
		Environment: run.Environment,
		Workspace:   run.Workspace,
		CapturedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Processes:   entries,
		UnixSockets: unixSockets,
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
	entries, unixSockets, err := ParseContainerProcessSnapshotLines(string(output))
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
		UnixSockets:    unixSockets,
	}, nil
}

func readProcEntries(procRoot string, workspace string, includeAll bool, unixSockets map[string]core.UnixSocketEntry) ([]core.ProcessEntry, error) {
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
		entry, err := readProcEntry(procRoot, pid, workspace, unixSockets)
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

func readProcEntry(procRoot string, pid int, workspace string, unixSockets map[string]core.UnixSocketEntry) (core.ProcessEntry, error) {
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
	openFDs, hasWorkspaceFD := readProcFDs(procDir, workspace, unixSockets)
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

// readWorkspaceUnixSockets reads the namespace-side identity of Unix-domain
// endpoints. /proc/net/unix supplies the kernel socket inode and the bound
// pathname; FD snapshots later supply the holder process and FD.
func readWorkspaceUnixSockets(path string, workspace string) ([]core.UnixSocketEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read unix socket table: %w", err)
	}
	defer f.Close()

	var entries []core.UnixSocketEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Num") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}
		inode, err := strconv.ParseUint(parts[6], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		socketPath := strings.Join(parts[7:], " ")
		if !isWorkspaceRelatedPathTarget(socketPath, workspace) {
			continue
		}
		entries = append(entries, core.UnixSocketEntry{
			SocketID:         core.UnixSocketID(inode),
			Inode:            inode,
			Path:             socketPath,
			Type:             parts[4],
			State:            parts[5],
			WorkspaceRelated: true,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read unix socket table: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SocketID == entries[j].SocketID {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].SocketID < entries[j].SocketID
	})
	return entries, nil
}

func unixSocketsByID(entries []core.UnixSocketEntry) map[string]core.UnixSocketEntry {
	result := make(map[string]core.UnixSocketEntry, len(entries))
	for _, entry := range entries {
		if entry.SocketID != "" {
			result[entry.SocketID] = entry
		}
	}
	return result
}

func readProcFDs(procDir string, workspace string, unixSockets map[string]core.UnixSocketEntry) ([]core.ProcessFDEntry, bool) {
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
		fdPath := filepath.Join(fdDir, entry.Name())
		target, err := os.Readlink(fdPath)
		if err != nil || strings.TrimSpace(target) == "" {
			continue
		}
		socketID := core.UnixSocketIDFromTarget(target)
		_, workspaceSocket := unixSockets[socketID]
		workspaceRelated := isWorkspaceRelatedPathTarget(target, workspace) || workspaceSocket
		if !workspaceRelated {
			continue
		}
		hasWorkspaceFD = true
		device, inode := readProcFDIdentity(fdPath)
		out = append(out, core.ProcessFDEntry{
			FD:               fd,
			Target:           target,
			Kind:             processFDKind(target),
			Device:           device,
			Inode:            inode,
			SocketID:         socketID,
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
	return `declare -A socket_paths
if [ -r /proc/net/unix ]; then
  while read -r num refcount protocol flags type state inode path; do
    case "$inode" in ''|*[!0-9]*) continue;; esac
    [ -n "$path" ] || continue
    socket_paths["$inode"]=$path
    printf 'U\t%s\t%s\t%s\t%s\n' "$inode" "$type" "$state" "$path"
  done < /proc/net/unix
fi
for d in /proc/[0-9]*; do
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
printf 'P\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$pid" "$ppid" "$state" "$name" "$cwd" "$workspace_related" "$cmd"
for fd_path in "$d"/fd/[0-9]*; do
  [ -L "$fd_path" ] || continue
  fd=${fd_path##*/}
  target=$(readlink "$fd_path" 2>/dev/null || true)
  [ -n "$target" ] || continue
  target_without_deleted=${target%" (deleted)"}
  socket_inode=
  case "$target" in
    socket:\[*\])
      socket_inode=${target#socket:[}
      socket_inode=${socket_inode%]}
      socket_path=${socket_paths[$socket_inode]:-}
      [ -n "$socket_path" ] || continue
      case "$socket_path" in
        /workspace|/workspace/*) ;;
        /*) continue;;
        *)
          case "$cwd" in
            /workspace|/workspace/*) ;;
            *) continue;;
          esac
          ;;
      esac
      ;;
    *)
      case "$target_without_deleted" in
        /workspace|/workspace/*) ;;
        *) continue;;
      esac
      ;;
  esac
  kind=path
  case "$target" in
    socket:\[*\]) kind=socket;;
    pipe:\[*\]) kind=pipe;;
    anon_inode:*) kind=anon_inode;;
    *" (deleted)") kind=deleted-path;;
  esac
  deleted=false
  case "$target" in *" (deleted)") deleted=true;; esac
  identity=$(stat -Lc '%d:%i' "$fd_path" 2>/dev/null || true)
  device=${identity%%:*}
  inode=${identity#*:}
  [ "$identity" = "$device" ] && inode=
  printf 'F\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$pid" "$fd" "$kind" "$deleted" "$device" "$inode" "$target"
done
done`
}

func ParseContainerProcessLines(output string) ([]core.ProcessEntry, error) {
	entries, _, err := ParseContainerProcessSnapshotLines(output)
	return entries, err
}

// ParseContainerProcessSnapshotLines decodes process records together with
// filesystem-bound Unix endpoint records. The older ParseContainerProcessLines
// wrapper remains for callers that only need process lineage.
func ParseContainerProcessSnapshotLines(output string) ([]core.ProcessEntry, []core.UnixSocketEntry, error) {
	processesByPID := make(map[int]core.ProcessEntry)
	processOrder := make([]int, 0)
	fdByPID := make(map[int][]core.ProcessFDEntry)
	unixSocketsByID := make(map[string]core.UnixSocketEntry)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "P\t"):
			entry, err := parseContainerProcessLine(strings.TrimPrefix(line, "P\t"))
			if err != nil {
				return nil, nil, err
			}
			processesByPID[entry.PID] = entry
			processOrder = append(processOrder, entry.PID)
		case strings.HasPrefix(line, "F\t"):
			pid, fd, err := parseContainerFDLine(strings.TrimPrefix(line, "F\t"))
			if err != nil {
				return nil, nil, err
			}
			fdByPID[pid] = append(fdByPID[pid], fd)
		case strings.HasPrefix(line, "U\t"):
			entry, err := parseContainerUnixSocketLine(strings.TrimPrefix(line, "U\t"))
			if err != nil {
				return nil, nil, err
			}
			unixSocketsByID[entry.SocketID] = entry
		default:
			// Keep decoding the pre-identity script format so that a partial
			// container upgrade does not make existing process artifacts unreadable.
			entry, err := parseContainerProcessLine(line)
			if err != nil {
				return nil, nil, err
			}
			processesByPID[entry.PID] = entry
			processOrder = append(processOrder, entry.PID)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read container process output: %w", err)
	}
	entries := make([]core.ProcessEntry, 0, len(processOrder))
	relatedSocketIDs := make(map[string]struct{})
	for _, pid := range processOrder {
		entry, ok := processesByPID[pid]
		if !ok || isContainerProbeProcess(entry.RawCmdline) {
			continue
		}
		entry.OpenFDs = append(entry.OpenFDs, fdByPID[pid]...)
		if len(entry.OpenFDs) > 0 {
			entry.WorkspaceRelated = true
		}
		if !entry.WorkspaceRelated {
			continue
		}
		sort.Slice(entry.OpenFDs, func(i, j int) bool {
			return entry.OpenFDs[i].FD < entry.OpenFDs[j].FD
		})
		for _, fd := range entry.OpenFDs {
			if fd.SocketID != "" {
				relatedSocketIDs[fd.SocketID] = struct{}{}
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PID < entries[j].PID
	})
	unixSockets := make([]core.UnixSocketEntry, 0, len(unixSocketsByID))
	for _, entry := range unixSocketsByID {
		if _, ok := relatedSocketIDs[entry.SocketID]; !ok && !isContainerWorkspaceSocketPath(entry.Path) {
			continue
		}
		entry.WorkspaceRelated = true
		unixSockets = append(unixSockets, entry)
	}
	sort.Slice(unixSockets, func(i, j int) bool {
		if unixSockets[i].SocketID == unixSockets[j].SocketID {
			return unixSockets[i].Path < unixSockets[j].Path
		}
		return unixSockets[i].SocketID < unixSockets[j].SocketID
	})
	return entries, unixSockets, nil
}

func isContainerWorkspaceSocketPath(path string) bool {
	path = strings.TrimSpace(path)
	return path == "/workspace" || strings.HasPrefix(path, "/workspace/")
}

func parseContainerProcessLine(line string) (core.ProcessEntry, error) {
	parts := strings.SplitN(line, "\t", 7)
	if len(parts) != 7 {
		return core.ProcessEntry{}, fmt.Errorf("decode container process line: %q", line)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return core.ProcessEntry{}, fmt.Errorf("decode process pid %q: %w", parts[0], err)
	}
	ppid, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	rawCmdline := strings.TrimSpace(parts[6])
	return core.ProcessEntry{
		PID:              pid,
		PPID:             ppid,
		State:            strings.TrimSpace(parts[2]),
		Name:             strings.TrimSpace(parts[3]),
		CWD:              strings.TrimSpace(parts[4]),
		WorkspaceRelated: strings.TrimSpace(parts[5]) == "true",
		RawCmdline:       rawCmdline,
		Cmdline:          strings.Fields(rawCmdline),
	}, nil
}

func parseContainerFDLine(line string) (int, core.ProcessFDEntry, error) {
	parts := strings.SplitN(line, "\t", 7)
	if len(parts) != 7 {
		return 0, core.ProcessFDEntry{}, fmt.Errorf("decode container fd line: %q", line)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, core.ProcessFDEntry{}, fmt.Errorf("decode fd process pid %q: %w", parts[0], err)
	}
	fd, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, core.ProcessFDEntry{}, fmt.Errorf("decode fd number %q: %w", parts[1], err)
	}
	device, err := parseOptionalUint64(parts[4])
	if err != nil {
		return 0, core.ProcessFDEntry{}, fmt.Errorf("decode fd device %q: %w", parts[4], err)
	}
	inode, err := parseOptionalUint64(parts[5])
	if err != nil {
		return 0, core.ProcessFDEntry{}, fmt.Errorf("decode fd inode %q: %w", parts[5], err)
	}
	return pid, core.ProcessFDEntry{
		FD:               fd,
		Target:           strings.TrimSpace(parts[6]),
		Kind:             strings.TrimSpace(parts[2]),
		Deleted:          strings.TrimSpace(parts[3]) == "true",
		Device:           device,
		Inode:            inode,
		SocketID:         core.UnixSocketIDFromTarget(parts[6]),
		WorkspaceRelated: true,
	}, nil
}

func parseContainerUnixSocketLine(line string) (core.UnixSocketEntry, error) {
	parts := strings.SplitN(line, "\t", 4)
	if len(parts) != 4 {
		return core.UnixSocketEntry{}, fmt.Errorf("decode container unix socket line: %q", line)
	}
	inode, err := parseOptionalUint64(parts[0])
	if err != nil {
		return core.UnixSocketEntry{}, fmt.Errorf("decode unix socket inode %q: %w", parts[0], err)
	}
	if inode == 0 {
		return core.UnixSocketEntry{}, fmt.Errorf("decode unix socket inode %q: must be nonzero", parts[0])
	}
	path := strings.TrimSpace(parts[3])
	if path == "" {
		return core.UnixSocketEntry{}, fmt.Errorf("decode unix socket path: %q", line)
	}
	return core.UnixSocketEntry{
		SocketID:         core.UnixSocketID(inode),
		Inode:            inode,
		Path:             path,
		Type:             strings.TrimSpace(parts[1]),
		State:            strings.TrimSpace(parts[2]),
		WorkspaceRelated: true,
	}, nil
}

func parseOptionalUint64(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func isContainerProbeProcess(rawCmdline string) bool {
	return strings.Contains(rawCmdline, "workspace_related=false") &&
		strings.Contains(rawCmdline, "printf '")
}
