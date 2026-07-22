package core

import (
	"context"
	"strings"
	"time"
)

// MismatchSignature is the six-tuple deduplication/reproducibility key used
// throughout the prototype: <lifecycle, phase, state class, operation,
// relation, impact>.
type MismatchSignature struct {
	LifecycleEvent string `json:"lifecycle_event"`
	FaultPhase     string `json:"fault_phase"`
	StateClass     string `json:"state_class"`
	Operation      string `json:"operation"`
	Relation       string `json:"mismatch_relation"`
	Impact         string `json:"impact"`
}

func (s MismatchSignature) String() string {
	parts := []string{s.LifecycleEvent, s.FaultPhase, s.StateClass, s.Operation, s.Relation, s.Impact}
	return "<" + strings.Join(parts, ", ") + ">"
}

// RunResult is the per-run verdict emitted by every synthetic testcase.
type RunResult struct {
	RunID           string            `json:"run_id"`
	CaseName        string            `json:"case_name"`
	RunRole         string            `json:"run_role,omitempty"`
	Environment     string            `json:"environment"`
	ContainerImage  string            `json:"container_image,omitempty"`
	FaultPlanID     string            `json:"fault_plan_id,omitempty"`
	PrimitiveID     string            `json:"primitive_id,omitempty"`
	TimingProfileID string            `json:"timing_profile_id,omitempty"`
	Confirmed       bool              `json:"confirmed"`
	Signature       MismatchSignature `json:"signature"`
	Evidence        []string          `json:"evidence"`
	ArtifactDir     string            `json:"artifact_dir"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      string            `json:"finished_at"`
}

// Case is a registry entry for a known-answer synthetic testcase.
type Case struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RunOptions captures only the knobs shared by the deterministic MVP cases.
// Later framework adapters can extend this without changing the CLI shape.
type RunOptions struct {
	CaseName        string
	OutDir          string
	Workspace       string
	Delay           time.Duration
	MockURL         string
	EnvKind         string
	ContainerImage  string
	FaultPlanID     string
	PrimitiveID     string
	RunRole         string
	TimingProfileID string
	FaultPlan       FaultPlan
	Timing          FaultTiming
}

const (
	RunRoleFault   = "fault"
	RunRoleControl = "control"
)

// CommandResult is the captured stdout/stderr of a single shell execution.
type CommandResult struct {
	StdoutStderr string `json:"stdout_stderr"`
}

// PersistentShell is the contract a long-lived shell session exposes to
// testcases. The concrete implementation lives in the environment package so
// core does not depend on exec.
type PersistentShell interface {
	Run(ctx context.Context, command string) ([]string, error)
	Probe(ctx context.Context) (ShellState, error)
	Close()
}

// Environment abstracts local vs container execution backends. Both the
// synthetic cases and the real-target adapter drive runs through it.
type Environment interface {
	Kind() string
	PrepareRun(ctx context.Context, opts RunOptions, started time.Time, workspaceRequired bool) (*RunContext, error)
	ExecShell(ctx context.Context, run *RunContext, command string) (CommandResult, error)
	ExecTargetCommand(ctx context.Context, run *RunContext, command string, envVars map[string]string) ([]byte, error)
	StartPersistentShell(ctx context.Context, run *RunContext) (PersistentShell, error)
	SnapshotProcesses(ctx context.Context, run *RunContext) (ProcessSnapshot, error)
	SnapshotSelectedProcesses(ctx context.Context, run *RunContext, selectors []ProcessSelector) (ProcessSnapshot, error)
}

// ShellState is a small probe of process-local shell state. This is the state
// class most likely to drift when an agent framework checkpoints graph data but
// keeps a long-lived shell process alive.
type ShellState struct {
	PWD           string   `json:"pwd"`
	PATH          string   `json:"path"`
	GitResolution string   `json:"git_resolution"`
	Aliases       []string `json:"aliases"`
	Raw           []string `json:"raw"`
}
