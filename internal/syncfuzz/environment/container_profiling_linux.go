//go:build linux

package environment

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

// ResolveContainerProfilingScope maps a running Docker container to the
// host-side cgroup identity required by the eBPF collector.
func ResolveContainerProfilingScope(ctx context.Context, containerName string, runID string) (profiling.ProfilingScope, error) {
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Pid}}", containerName).CombinedOutput()
	if err != nil {
		return profiling.ProfilingScope{}, fmt.Errorf("inspect profiling container %q: %w: %s", containerName, err, strings.TrimSpace(string(output)))
	}
	containerID, hostPID, err := parseContainerInspectScope(string(output))
	if err != nil {
		return profiling.ProfilingScope{}, fmt.Errorf("inspect profiling container %q: %w", containerName, err)
	}
	scope, err := profiling.ResolveCgroupV2Scope(hostPID)
	if err != nil {
		return profiling.ProfilingScope{}, err
	}
	scope.RunID = strings.TrimSpace(runID)
	scope.Environment = "container"
	scope.ContainerID = containerID
	return scope, nil
}

func parseContainerInspectScope(raw string) (string, int, error) {
	fields := strings.Fields(raw)
	if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" {
		return "", 0, fmt.Errorf("expected container id and host pid, got %q", strings.TrimSpace(raw))
	}
	hostPID, err := strconv.Atoi(fields[1])
	if err != nil || hostPID <= 0 {
		return "", 0, fmt.Errorf("invalid container host pid %q", fields[1])
	}
	return fields[0], hostPID, nil
}
