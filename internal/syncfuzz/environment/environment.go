package environment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const DefaultContainerImage = "ubuntu:latest"

// NewEnvironment selects a local or container backend. The container backend
// reuses the local run-directory layout and adds a hardened, network-isolated
// Docker sandbox wrapping the same workspace semantics.
func NewEnvironment(kind string, containerImage string) (core.Environment, error) {
	kind = NormalizedEnvKind(kind)
	switch kind {
	case "local":
		return LocalEnvironment{}, nil
	case "container":
		return ContainerEnvironment{Image: NormalizedContainerImage(containerImage)}, nil
	default:
		return nil, fmt.Errorf("unsupported environment %q", kind)
	}
}

func ValidateEnvironmentKind(kind string) error {
	_, err := NewEnvironment(kind, "")
	return err
}

func NormalizedEnvKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "local"
	}
	return strings.TrimSpace(kind)
}

func NormalizedContainerImage(image string) string {
	if strings.TrimSpace(image) == "" {
		return DefaultContainerImage
	}
	return strings.TrimSpace(image)
}

func ContainerImageForResult(kind string, image string) string {
	if NormalizedEnvKind(kind) != "container" {
		return ""
	}
	return NormalizedContainerImage(image)
}

type LocalEnvironment struct{}

func (e LocalEnvironment) Kind() string {
	return "local"
}

func (e LocalEnvironment) PrepareRun(_ context.Context, opts core.RunOptions, started time.Time, workspaceRequired bool) (*core.RunContext, error) {
	runID := fmt.Sprintf("%d", started.UnixNano())
	runDir := filepath.Join(opts.OutDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("create run directory: %w", err)
	}

	workspace := opts.Workspace
	if workspaceRequired {
		if workspace == "" {
			workspace = filepath.Join(runDir, "workspace")
		}
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return nil, fmt.Errorf("create workspace: %w", err)
		}
	}

	trace, err := core.NewEventWriter(filepath.Join(runDir, "trace.jsonl"))
	if err != nil {
		return nil, err
	}

	return &core.RunContext{
		RunID:       runID,
		CaseName:    opts.CaseName,
		RunRole:     opts.RunRole,
		RunDir:      runDir,
		Workspace:   workspace,
		Environment: e.Kind(),
		PrimitiveID: opts.PrimitiveID,
		FaultPlan:   opts.FaultPlan,
		Timing:      opts.Timing,
		TurnID:      "turn-0001",
		ToolCallID:  "tool-0001",
		Trace:       trace,
	}, nil
}

func (e LocalEnvironment) ExecShell(ctx context.Context, run *core.RunContext, command string) (core.CommandResult, error) {
	if run.Workspace == "" {
		return core.CommandResult{}, fmt.Errorf("local shell execution requires a workspace")
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = run.Workspace
	output, err := cmd.CombinedOutput()
	result := core.CommandResult{StdoutStderr: strings.TrimSpace(string(output))}
	if err != nil {
		return result, fmt.Errorf("execute shell command: %w: %s", err, string(output))
	}
	return result, nil
}

func (e LocalEnvironment) StartPersistentShell(ctx context.Context, run *core.RunContext) (core.PersistentShell, error) {
	return startPersistentShell(ctx, run.Workspace)
}

func (e LocalEnvironment) SnapshotProcesses(_ context.Context, run *core.RunContext) (core.ProcessSnapshot, error) {
	return snapshotLocalProcesses(run)
}

type ContainerEnvironment struct {
	Image string
}

func (e ContainerEnvironment) Kind() string {
	return "container"
}

func (e ContainerEnvironment) PrepareRun(ctx context.Context, opts core.RunOptions, started time.Time, workspaceRequired bool) (*core.RunContext, error) {
	local := LocalEnvironment{}
	run, err := local.PrepareRun(ctx, opts, started, workspaceRequired)
	if err != nil {
		return nil, err
	}
	run.Environment = e.Kind()
	run.ContainerImage = e.Image

	if !workspaceRequired {
		return run, nil
	}

	if err := ensureContainerImage(ctx, e.Image); err != nil {
		_ = run.Close()
		return nil, err
	}

	containerName := "syncfuzz-" + run.RunID
	workspace, err := filepath.Abs(run.Workspace)
	if err != nil {
		_ = run.Close()
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}

	args := []string{
		"run",
		"-d",
		"--rm",
		"--name", containerName,
		"--network", "none",
		"--pids-limit", "128",
		"--memory", "256m",
		"--cpus", "1",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v", workspace + ":/workspace",
		"-w", "/workspace",
		e.Image,
		"sleep", "infinity",
	}
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		_ = run.Close()
		return nil, fmt.Errorf("start container environment: %w: %s", err, strings.TrimSpace(string(output)))
	}

	run.ContainerName = containerName
	run.Cleanup = func() error {
		cmd := exec.CommandContext(context.Background(), "docker", "stop", "-t", "1", containerName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("stop container %s: %w: %s", containerName, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return run, nil
}

func (e ContainerEnvironment) ExecShell(ctx context.Context, run *core.RunContext, command string) (core.CommandResult, error) {
	if run.ContainerName == "" {
		return core.CommandResult{}, fmt.Errorf("container shell execution requires a running container")
	}
	args := []string{"exec", "-w", "/workspace", run.ContainerName, "bash", "-lc", command}
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	result := core.CommandResult{StdoutStderr: strings.TrimSpace(string(output))}
	if err != nil {
		return result, fmt.Errorf("execute container shell command: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return result, nil
}

func (e ContainerEnvironment) StartPersistentShell(ctx context.Context, run *core.RunContext) (core.PersistentShell, error) {
	if run.ContainerName == "" {
		return nil, fmt.Errorf("container persistent shell requires a running container")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "-w", "/workspace", run.ContainerName, "bash", "--noprofile", "--norc")
	return startPersistentShellCommand(ctx, cmd)
}

func (e ContainerEnvironment) SnapshotProcesses(ctx context.Context, run *core.RunContext) (core.ProcessSnapshot, error) {
	return snapshotContainerProcesses(ctx, run)
}

func ensureContainerImage(ctx context.Context, image string) error {
	output, err := exec.CommandContext(ctx, "docker", "image", "inspect", image).CombinedOutput()
	if err != nil {
		return fmt.Errorf("container image %q is not available locally: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}
