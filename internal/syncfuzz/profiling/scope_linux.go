//go:build linux

package profiling

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const cgroupV2Mount = "/sys/fs/cgroup"

type cgroupV2MountInfo struct {
	Root       string
	MountPoint string
}

// ResolveCgroupV2Scope obtains the cgroup-v2 identity for a host process such
// as Docker's container init PID. On cgroup v2, the cgroup directory inode is
// the identity returned by bpf_get_current_cgroup_id, which is the collector's
// kernel-side filter value.
func ResolveCgroupV2Scope(pid int) (ProfilingScope, error) {
	if pid <= 0 {
		return ProfilingScope{}, fmt.Errorf("resolve cgroup scope: pid must be positive")
	}
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return ProfilingScope{}, fmt.Errorf("resolve cgroup scope for pid %d: %w", pid, err)
	}
	relativePath, err := ParseCgroupV2Path(string(raw))
	if err != nil {
		return ProfilingScope{}, fmt.Errorf("resolve cgroup scope for pid %d: %w", pid, err)
	}
	cleanRelativePath := filepath.Clean(strings.TrimPrefix(relativePath, "/"))
	if cleanRelativePath == ".." || strings.HasPrefix(cleanRelativePath, ".."+string(filepath.Separator)) {
		return ProfilingScope{}, fmt.Errorf("resolve cgroup scope for pid %d: unsafe cgroup path %q", pid, relativePath)
	}
	cgroupPaths := cgroupV2PathCandidates(pid, cleanRelativePath)
	cgroupPaths = append(cgroupPaths, cgroupV2PathsFromMountInfo("/proc/self/mountinfo", nil, relativePath)...)
	cgroupPaths = append(cgroupPaths, cgroupV2PathsFromMountInfo(filepath.Join("/proc", strconv.Itoa(pid), "mountinfo"), []string{"/proc", strconv.Itoa(pid), "root"}, relativePath)...)
	attempts := make([]string, 0, len(cgroupPaths))
	seen := make(map[string]struct{}, len(cgroupPaths))
	for _, cgroupPath := range cgroupPaths {
		if _, ok := seen[cgroupPath]; ok {
			continue
		}
		seen[cgroupPath] = struct{}{}
		info, statErr := os.Stat(cgroupPath)
		if statErr != nil {
			attempts = append(attempts, fmt.Sprintf("%s: %v", cgroupPath, statErr))
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Ino == 0 {
			attempts = append(attempts, fmt.Sprintf("%s: cgroup inode unavailable", cgroupPath))
			continue
		}
		return ProfilingScope{CgroupPath: cgroupPath, CgroupID: stat.Ino}, nil
	}
	return ProfilingScope{}, fmt.Errorf("resolve cgroup scope for pid %d: cgroup path %q is unavailable (tried %s)", pid, relativePath, strings.Join(attempts, "; "))
}

// cgroupV2PathCandidates resolves the cgroup directory from two mount
// namespaces. A host-side collector can run inside a container or other mount
// namespace that masks /sys/fs/cgroup, even while Docker reports a valid host
// PID. Following /proc/<pid>/root uses the target process's cgroupfs mount and
// therefore recovers the inode used by bpf_get_current_cgroup_id.
func cgroupV2PathCandidates(pid int, cleanRelativePath string) []string {
	paths := []string{
		filepath.Join(cgroupV2Mount, cleanRelativePath),
		filepath.Join("/proc", strconv.Itoa(pid), "root", "sys", "fs", "cgroup", cleanRelativePath),
	}
	if paths[0] == paths[1] {
		return paths[:1]
	}
	return paths
}

// cgroupV2PathsFromMountInfo finds cgroup2 mountpoints instead of assuming the
// unified hierarchy is mounted directly at /sys/fs/cgroup. Hybrid systems
// commonly mount it at /sys/fs/cgroup/unified. When prefix is non-empty, the
// mountpoint is resolved through a target process's root mount namespace.
func cgroupV2PathsFromMountInfo(mountInfoPath string, prefix []string, cgroupPath string) []string {
	raw, err := os.ReadFile(mountInfoPath)
	if err != nil {
		return nil
	}
	return cgroupV2PathsForMounts(parseCgroupV2MountInfo(string(raw)), prefix, cgroupPath)
}

func cgroupV2PathsForMounts(mounts []cgroupV2MountInfo, prefix []string, cgroupPath string) []string {
	paths := make([]string, 0)
	for _, mount := range mounts {
		relativePath, ok := cgroupPathRelativeToMount(cgroupPath, mount.Root)
		if !ok {
			continue
		}
		components := append([]string{}, prefix...)
		if len(components) == 0 {
			components = append(components, string(filepath.Separator))
		}
		components = append(components, strings.TrimPrefix(mount.MountPoint, "/"))
		if relativePath != "" {
			components = append(components, relativePath)
		}
		paths = append(paths, filepath.Join(components...))
	}
	return paths
}

func parseCgroupV2MountInfo(raw string) []cgroupV2MountInfo {
	mounts := make([]cgroupV2MountInfo, 0)
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), " - ", 2)
		if len(parts) != 2 {
			continue
		}
		before := strings.Fields(parts[0])
		after := strings.Fields(parts[1])
		if len(before) < 5 || len(after) == 0 || after[0] != "cgroup2" {
			continue
		}
		root := unescapeMountInfoPath(before[3])
		mountPoint := unescapeMountInfoPath(before[4])
		if !filepath.IsAbs(root) || !filepath.IsAbs(mountPoint) {
			continue
		}
		mounts = append(mounts, cgroupV2MountInfo{Root: filepath.Clean(root), MountPoint: filepath.Clean(mountPoint)})
	}
	return mounts
}

func cgroupPathRelativeToMount(cgroupPath string, mountRoot string) (string, bool) {
	path := filepath.Clean(cgroupPath)
	root := filepath.Clean(mountRoot)
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return "", false
	}
	if root == "/" {
		return strings.TrimPrefix(path, "/"), true
	}
	if path == root {
		return "", true
	}
	prefix := root + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(path, prefix), true
}

func unescapeMountInfoPath(path string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(path)
}

// ParseCgroupV2Path returns the unified-hierarchy path from procfs cgroup
// data. A v1-only hierarchy is rejected because it cannot provide the cgroup
// filter required by the v2 collector.
func ParseCgroupV2Path(raw string) (string, error) {
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" {
			continue
		}
		path := strings.TrimSpace(parts[2])
		if !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("invalid unified cgroup path %q", path)
		}
		return path, nil
	}
	return "", fmt.Errorf("cgroup v2 unified hierarchy not found")
}
