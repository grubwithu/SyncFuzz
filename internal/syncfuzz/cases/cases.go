package cases

import (
	"context"
	"fmt"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

// Run normalizes defaults, then dispatches to one known-answer testcase.
// Each testcase emits the same high-level artifacts: trace, state snapshots,
// and a result with a mismatch signature.
func Run(ctx context.Context, opts core.RunOptions) (*core.RunResult, error) {
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
	timing, err := core.ResolveTimingProfile(opts.TimingProfileID, opts.Delay)
	if err != nil {
		return nil, err
	}
	opts.TimingProfileID = timing.ProfileID
	opts.Timing = timing
	runRole, err := normalizeRunRole(opts.RunRole)
	if err != nil {
		return nil, err
	}
	opts.RunRole = runRole
	if err := core.ValidateCaseNames([]string{opts.CaseName}); err != nil {
		return nil, err
	}
	faultPlan, err := core.ResolveFaultPlan(opts.CaseName, opts.FaultPlanID)
	if err != nil {
		return nil, err
	}
	faultPlan.Timing = timing
	opts.FaultPlan = faultPlan
	if opts.PrimitiveID != "" {
		if err := validateExecutablePrimitive(opts.CaseName, opts.PrimitiveID); err != nil {
			return nil, err
		}
	}

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

func validateExecutablePrimitive(caseName string, primitiveID string) error {
	primitive, ok := core.PrimitiveByID(primitiveID)
	if !ok {
		return fmt.Errorf("unknown primitive %q", primitiveID)
	}
	if !primitive.Implemented {
		return fmt.Errorf("primitive %q is not implemented", primitiveID)
	}
	if !core.StringInSlice(caseName, primitive.CaseNames) {
		return fmt.Errorf("primitive %q does not apply to case %q", primitiveID, caseName)
	}
	return nil
}

func normalizeRunRole(role string) (string, error) {
	switch role {
	case "", core.RunRoleFault:
		return core.RunRoleFault, nil
	case core.RunRoleControl:
		return core.RunRoleControl, nil
	default:
		return "", fmt.Errorf("unsupported run role %q", role)
	}
}

func isControlRun(opts core.RunOptions) bool {
	return opts.RunRole == core.RunRoleControl
}
