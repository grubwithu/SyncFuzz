package environment_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
)

func TestLocalEnvironmentPrepareRunAndExecShell(t *testing.T) {
	tmp := t.TempDir()
	env, err := environment.NewEnvironment("local", "")
	if err != nil {
		t.Fatalf("environment.NewEnvironment failed: %v", err)
	}

	run, err := env.PrepareRun(context.Background(), core.RunOptions{
		CaseName: "env-test",
		OutDir:   filepath.Join(tmp, "runs"),
	}, time.Unix(1, 2).UTC(), true)
	if err != nil {
		t.Fatalf("PrepareRun failed: %v", err)
	}
	defer run.Close()

	if run.Environment != "local" {
		t.Fatalf("expected local environment, got %q", run.Environment)
	}
	if run.Workspace == "" {
		t.Fatalf("expected workspace")
	}
	if _, err := os.Stat(run.Workspace); err != nil {
		t.Fatalf("expected workspace directory: %v", err)
	}

	result, err := env.ExecShell(context.Background(), run, "printf hello > env-output.txt")
	if err != nil {
		t.Fatalf("ExecShell failed: %v", err)
	}
	if result.StdoutStderr != "" {
		t.Fatalf("expected empty command output, got %q", result.StdoutStderr)
	}
	if _, err := os.Stat(filepath.Join(run.Workspace, "env-output.txt")); err != nil {
		t.Fatalf("expected shell-created file: %v", err)
	}
}

func TestLocalEnvironmentExecTargetCommandTimeoutKillsDescendants(t *testing.T) {
	env := environment.LocalEnvironment{}
	run := &core.RunContext{Workspace: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := env.ExecTargetCommand(ctx, run, "sleep 5 & wait", nil)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected context deadline exceeded, got %v", ctx.Err())
	}
	if elapsed >= time.Second {
		t.Fatalf("target command waited %s for a background descendant after timeout", elapsed)
	}
}

func TestNewEnvironmentSupportsContainerWithDefaultImage(t *testing.T) {
	env, err := environment.NewEnvironment("container", "")
	if err != nil {
		t.Fatalf("environment.NewEnvironment failed: %v", err)
	}
	container, ok := env.(environment.ContainerEnvironment)
	if !ok {
		t.Fatalf("expected environment.ContainerEnvironment, got %T", env)
	}
	if container.Image != environment.DefaultContainerImage {
		t.Fatalf("expected default container image %q, got %q", environment.DefaultContainerImage, container.Image)
	}
}

func TestNewEnvironmentRejectsUnsupportedKind(t *testing.T) {
	if _, err := environment.NewEnvironment("vm", ""); err == nil {
		t.Fatalf("expected unsupported environment error")
	}
}
