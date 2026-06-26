package syncfuzz

import (
	"context"
	"fmt"
	"time"
)

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
	RunRole         string
	TimingProfileID string
	faultPlan       FaultPlan
	timing          FaultTiming
}

const (
	RunRoleFault   = "fault"
	RunRoleControl = "control"
)

// Cases is the public registry used by both the CLI and future schedulers.
func Cases() []Case {
	return []Case{
		{
			Name:        "orphan-process",
			Description: "detect a delayed OS effect that survives an agent lifecycle boundary",
		},
		{
			Name:        "action-replay",
			Description: "detect duplicated external effects after a lost response and replay",
		},
		{
			Name:        "authority-resurrection",
			Description: "detect reuse attempts for consumed single-use authority after replay",
		},
		{
			Name:        "persistent-shell-poisoning",
			Description: "detect PATH/cwd/alias residue in a reused persistent shell",
		},
		{
			Name:        "partial-filesystem-rollback",
			Description: "detect untracked, symlink, and metadata residue after naive rollback",
		},
		{
			Name:        "branch-leakage",
			Description: "detect discarded branch effects leaking into committed branch state",
		},
	}
}

// Run normalizes defaults, then dispatches to one known-answer testcase.
// Each testcase emits the same high-level artifacts: trace, state snapshots,
// and a result with a mismatch signature.
func Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if opts.CaseName == "" {
		opts.CaseName = "orphan-process"
	}
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if opts.EnvKind == "" {
		opts.EnvKind = "local"
	}
	timing, err := resolveTimingProfile(opts.TimingProfileID, opts.Delay)
	if err != nil {
		return nil, err
	}
	opts.TimingProfileID = timing.ProfileID
	opts.timing = timing
	runRole, err := normalizeRunRole(opts.RunRole)
	if err != nil {
		return nil, err
	}
	opts.RunRole = runRole
	if err := validateCaseNames([]string{opts.CaseName}); err != nil {
		return nil, err
	}
	faultPlan, err := resolveFaultPlan(opts.CaseName, opts.FaultPlanID)
	if err != nil {
		return nil, err
	}
	faultPlan.Timing = timing
	opts.faultPlan = faultPlan

	switch opts.CaseName {
	case "orphan-process":
		return runOrphanProcess(ctx, opts)
	case "action-replay":
		return runActionReplay(ctx, opts)
	case "authority-resurrection":
		return runAuthorityResurrection(ctx, opts)
	case "persistent-shell-poisoning":
		return runPersistentShellPoisoning(ctx, opts)
	case "partial-filesystem-rollback":
		return runPartialFilesystemRollback(ctx, opts)
	case "branch-leakage":
		return runBranchLeakage(ctx, opts)
	default:
		return nil, fmt.Errorf("unknown case %q", opts.CaseName)
	}
}

func normalizeRunRole(role string) (string, error) {
	switch role {
	case "", RunRoleFault:
		return RunRoleFault, nil
	case RunRoleControl:
		return RunRoleControl, nil
	default:
		return "", fmt.Errorf("unsupported run role %q", role)
	}
}

func isControlRun(opts RunOptions) bool {
	return opts.RunRole == RunRoleControl
}
