package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Environment interface {
	Kind() string
	PrepareRun(ctx context.Context, opts RunOptions, started time.Time, workspaceRequired bool) (*runContext, error)
	ExecShell(ctx context.Context, run *runContext, command string) (CommandResult, error)
	StartPersistentShell(ctx context.Context, run *runContext) (*persistentShell, error)
	SnapshotProcesses(ctx context.Context, run *runContext) (ProcessSnapshot, error)
}

type CommandResult struct {
	StdoutStderr string `json:"stdout_stderr"`
}

const defaultContainerImage = "ubuntu:latest"

func newEnvironment(kind string, containerImage string) (Environment, error) {
	kind = normalizedEnvKind(kind)
	switch kind {
	case "local":
		return localEnvironment{}, nil
	case "container":
		return containerEnvironment{image: normalizedContainerImage(containerImage)}, nil
	default:
		return nil, fmt.Errorf("unsupported environment %q", kind)
	}
}

func validateEnvironmentKind(kind string) error {
	_, err := newEnvironment(kind, "")
	return err
}

func normalizedEnvKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "local"
	}
	return strings.TrimSpace(kind)
}

func normalizedContainerImage(image string) string {
	if strings.TrimSpace(image) == "" {
		return defaultContainerImage
	}
	return strings.TrimSpace(image)
}

func containerImageForResult(kind string, image string) string {
	if normalizedEnvKind(kind) != "container" {
		return ""
	}
	return normalizedContainerImage(image)
}

type localEnvironment struct{}

func (e localEnvironment) Kind() string {
	return "local"
}

func (e localEnvironment) PrepareRun(_ context.Context, opts RunOptions, started time.Time, workspaceRequired bool) (*runContext, error) {
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

	trace, err := newEventWriter(filepath.Join(runDir, "trace.jsonl"))
	if err != nil {
		return nil, err
	}

	return &runContext{
		runID:       runID,
		caseName:    opts.CaseName,
		runRole:     opts.RunRole,
		runDir:      runDir,
		workspace:   workspace,
		environment: e.Kind(),
		faultPlan:   opts.faultPlan,
		timing:      opts.timing,
		turnID:      "turn-0001",
		toolCallID:  "tool-0001",
		trace:       trace,
	}, nil
}

func (e localEnvironment) ExecShell(ctx context.Context, run *runContext, command string) (CommandResult, error) {
	if run.workspace == "" {
		return CommandResult{}, fmt.Errorf("local shell execution requires a workspace")
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = run.workspace
	output, err := cmd.CombinedOutput()
	result := CommandResult{StdoutStderr: strings.TrimSpace(string(output))}
	if err != nil {
		return result, fmt.Errorf("execute shell command: %w: %s", err, string(output))
	}
	return result, nil
}

func (e localEnvironment) StartPersistentShell(ctx context.Context, run *runContext) (*persistentShell, error) {
	return startPersistentShell(ctx, run.workspace)
}

func (e localEnvironment) SnapshotProcesses(_ context.Context, run *runContext) (ProcessSnapshot, error) {
	return snapshotLocalProcesses(run)
}

type containerEnvironment struct {
	image string
}

func (e containerEnvironment) Kind() string {
	return "container"
}

func (e containerEnvironment) PrepareRun(ctx context.Context, opts RunOptions, started time.Time, workspaceRequired bool) (*runContext, error) {
	local := localEnvironment{}
	run, err := local.PrepareRun(ctx, opts, started, workspaceRequired)
	if err != nil {
		return nil, err
	}
	run.environment = e.Kind()
	run.containerImage = e.image

	if !workspaceRequired {
		return run, nil
	}

	if err := ensureContainerImage(ctx, e.image); err != nil {
		_ = run.Close()
		return nil, err
	}

	containerName := "syncfuzz-" + run.runID
	workspace, err := filepath.Abs(run.workspace)
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
		e.image,
		"sleep", "infinity",
	}
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		_ = run.Close()
		return nil, fmt.Errorf("start container environment: %w: %s", err, strings.TrimSpace(string(output)))
	}

	run.containerName = containerName
	run.cleanup = func() error {
		cmd := exec.CommandContext(context.Background(), "docker", "stop", "-t", "1", containerName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("stop container %s: %w: %s", containerName, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return run, nil
}

func (e containerEnvironment) ExecShell(ctx context.Context, run *runContext, command string) (CommandResult, error) {
	if run.containerName == "" {
		return CommandResult{}, fmt.Errorf("container shell execution requires a running container")
	}
	args := []string{"exec", "-w", "/workspace", run.containerName, "bash", "-lc", command}
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	result := CommandResult{StdoutStderr: strings.TrimSpace(string(output))}
	if err != nil {
		return result, fmt.Errorf("execute container shell command: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return result, nil
}

func (e containerEnvironment) StartPersistentShell(ctx context.Context, run *runContext) (*persistentShell, error) {
	if run.containerName == "" {
		return nil, fmt.Errorf("container persistent shell requires a running container")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "-w", "/workspace", run.containerName, "bash", "--noprofile", "--norc")
	return startPersistentShellCommand(ctx, cmd)
}

func (e containerEnvironment) SnapshotProcesses(ctx context.Context, run *runContext) (ProcessSnapshot, error) {
	return snapshotContainerProcesses(ctx, run)
}

func ensureContainerImage(ctx context.Context, image string) error {
	output, err := exec.CommandContext(ctx, "docker", "image", "inspect", image).CombinedOutput()
	if err != nil {
		return fmt.Errorf("container image %q is not available locally: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}
