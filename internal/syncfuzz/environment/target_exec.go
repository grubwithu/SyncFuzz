package environment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

// ExecTargetCommand runs a real-target agent command (bash -lc <command>) with
// the given environment overrides. The local backend runs the command directly
// in the workspace; the container backend runs it inside the Docker sandbox.
func (e LocalEnvironment) ExecTargetCommand(ctx context.Context, run *core.RunContext, command string, envVars map[string]string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = run.Workspace
	cmd.Env = append(os.Environ(), sortedEnv(envVars)...)
	return cmd.CombinedOutput()
}

func (e ContainerEnvironment) ExecTargetCommand(ctx context.Context, run *core.RunContext, command string, envVars map[string]string) ([]byte, error) {
	if run.ContainerName == "" {
		return nil, fmt.Errorf("container target command requires a running container")
	}
	args := []string{"exec", "-w", "/workspace"}
	for _, item := range sortedEnv(envVars) {
		args = append(args, "-e", item)
	}
	args = append(args, run.ContainerName, "bash", "-lc", command)
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

func sortedEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
