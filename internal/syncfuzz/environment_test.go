package syncfuzz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalEnvironmentPrepareRunAndExecShell(t *testing.T) {
	tmp := t.TempDir()
	env, err := newEnvironment("local", "")
	if err != nil {
		t.Fatalf("newEnvironment failed: %v", err)
	}

	run, err := env.PrepareRun(context.Background(), RunOptions{
		CaseName: "env-test",
		OutDir:   filepath.Join(tmp, "runs"),
	}, time.Unix(1, 2).UTC(), true)
	if err != nil {
		t.Fatalf("PrepareRun failed: %v", err)
	}
	defer run.Close()

	if run.environment != "local" {
		t.Fatalf("expected local environment, got %q", run.environment)
	}
	if run.workspace == "" {
		t.Fatalf("expected workspace")
	}
	if _, err := os.Stat(run.workspace); err != nil {
		t.Fatalf("expected workspace directory: %v", err)
	}

	result, err := env.ExecShell(context.Background(), run, "printf hello > env-output.txt")
	if err != nil {
		t.Fatalf("ExecShell failed: %v", err)
	}
	if result.StdoutStderr != "" {
		t.Fatalf("expected empty command output, got %q", result.StdoutStderr)
	}
	if _, err := os.Stat(filepath.Join(run.workspace, "env-output.txt")); err != nil {
		t.Fatalf("expected shell-created file: %v", err)
	}
}

func TestNewEnvironmentSupportsContainerWithDefaultImage(t *testing.T) {
	env, err := newEnvironment("container", "")
	if err != nil {
		t.Fatalf("newEnvironment failed: %v", err)
	}
	container, ok := env.(containerEnvironment)
	if !ok {
		t.Fatalf("expected containerEnvironment, got %T", env)
	}
	if container.image != defaultContainerImage {
		t.Fatalf("expected default container image %q, got %q", defaultContainerImage, container.image)
	}
}

func TestNewEnvironmentRejectsUnsupportedKind(t *testing.T) {
	if _, err := newEnvironment("vm", ""); err == nil {
		t.Fatalf("expected unsupported environment error")
	}
}
