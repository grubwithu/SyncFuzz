package syncfuzz

import (
	"fmt"
	"path/filepath"
	"strings"
)

type MismatchSignature struct {
	LifecycleEvent string `json:"lifecycle_event"`
	FaultPhase     string `json:"fault_phase"`
	StateClass     string `json:"state_class"`
	Operation      string `json:"operation"`
	Relation       string `json:"mismatch_relation"`
	Impact         string `json:"impact"`
}

// String renders the six-tuple used throughout the prototype:
// <lifecycle, phase, state class, operation, relation, impact>.
func (s MismatchSignature) String() string {
	parts := []string{s.LifecycleEvent, s.FaultPhase, s.StateClass, s.Operation, s.Relation, s.Impact}
	return "<" + strings.Join(parts, ", ") + ">"
}

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

// orphanProcessOracle detects an OS effect that appears only after the command
// has returned. That is the smallest rollback-residue known-answer case.
func orphanProcessOracle(before Snapshot, after Snapshot) (bool, []string) {
	beforeSet := before.Paths()
	var evidence []string

	for _, entry := range after.Files {
		if _, ok := beforeSet[entry.Path]; ok {
			continue
		}
		if entry.Path == "late-effect" {
			evidence = append(evidence, "late-effect appeared after the command returned")
		}
	}

	return len(evidence) > 0, evidence
}

// actionReplayOracle looks for multiple external resources produced by one
// logical operation. The run ID ties resources back to this testcase instance.
func actionReplayOracle(before ExternalState, after ExternalState, runID string) (bool, []string) {
	beforeIDs := make(map[string]struct{}, len(before.Effects.Resources))
	for _, resource := range before.Effects.Resources {
		beforeIDs[resource.ID] = struct{}{}
	}

	var replayed []EffectResource
	for _, resource := range after.Effects.Resources {
		if _, ok := beforeIDs[resource.ID]; ok {
			continue
		}
		if strings.Contains(resource.RequestID, runID) {
			replayed = append(replayed, resource)
		}
	}

	if len(replayed) < 2 {
		return false, nil
	}

	requestIDs := make([]string, 0, len(replayed))
	for _, resource := range replayed {
		requestIDs = append(requestIDs, resource.RequestID)
	}

	return true, []string{
		"external state contains multiple resources for one logical action",
		"request ids: " + strings.Join(requestIDs, ", "),
	}
}

// authorityResurrectionOracle flags a replay that tries to use authority that
// the authority server has already consumed.
func authorityResurrectionOracle(after ExternalState, token string, replay *consumeTokenResponse) (bool, []string) {
	var authorityToken *AuthorityToken
	for i := range after.Authority.Tokens {
		if after.Authority.Tokens[i].Token == token {
			authorityToken = &after.Authority.Tokens[i]
			break
		}
	}
	if authorityToken == nil || !authorityToken.Consumed {
		return false, nil
	}
	if replay == nil || replay.Error != "token_already_consumed" {
		return false, nil
	}

	return true, []string{
		"agent replay attempted to reuse a consumed authority token",
		"authority server rejected stale token with token_already_consumed",
	}
}

// persistentShellPoisoningOracle checks whether process-local shell state still
// reflects the discarded branch after graph-level replay.
func persistentShellPoisoningOracle(before ShellState, after ShellState) (bool, []string) {
	var evidence []string
	if before.PWD != "" && after.PWD != "" && before.PWD != after.PWD {
		evidence = append(evidence, "persistent shell cwd changed from "+before.PWD+" to "+after.PWD)
	}
	if strings.Contains(after.GitResolution, "attacker-bin/git") {
		evidence = append(evidence, "git resolves to attacker-controlled PATH entry: "+after.GitResolution)
	}
	for _, alias := range after.Aliases {
		if strings.Contains(alias, "sfuzz_probe") {
			evidence = append(evidence, "persistent shell alias remains after replay: "+alias)
			break
		}
	}
	return len(evidence) > 0, evidence
}

// partialFilesystemRollbackOracle checks channels commonly missed by naive
// git-like rollback: untracked files, symlinks, and metadata drift.
func partialFilesystemRollbackOracle(before Snapshot, after Snapshot) (bool, []string) {
	beforeSet := before.Paths()
	var evidence []string

	for _, entry := range after.Files {
		beforeEntry, existed := beforeSet[entry.Path]
		if !existed {
			switch entry.Type {
			case "symlink":
				evidence = append(evidence, "symlink residue remains after rollback: "+entry.Path)
			case "file":
				evidence = append(evidence, "untracked file remains after rollback: "+entry.Path)
			default:
				evidence = append(evidence, "new filesystem object remains after rollback: "+entry.Path)
			}
			continue
		}
		if entry.Path == "tracked.txt" && entry.Mode != beforeEntry.Mode {
			evidence = append(evidence, "tracked file mode drift remains after rollback: "+beforeEntry.Mode+" -> "+entry.Mode)
		}
	}

	return len(evidence) > 0, evidence
}

// partialFilesystemRollbackFDOracle detects a deleted workspace inode that
// remains reachable through an open file descriptor after the visible path has
// been recreated by rollback.
func partialFilesystemRollbackFDOracle(after Snapshot, processAfter ProcessSnapshot, workspace string) (bool, []string) {
	afterSet := after.Paths()
	trackedEntry, restored := afterSet["tracked.txt"]
	if !restored || trackedEntry.Type != "file" {
		return false, nil
	}

	trackedTargets := pathCandidates(filepath.Join(filepath.Clean(workspace), "tracked.txt"))
	for _, process := range processAfter.Processes {
		for _, fd := range process.OpenFDs {
			if !fd.WorkspaceRelated || !fd.Deleted {
				continue
			}
			if !pathInCandidates(strings.TrimSuffix(fd.Target, " (deleted)"), trackedTargets) {
				continue
			}
			return true, []string{
				fmt.Sprintf("process %d keeps deleted workspace fd %d to tracked.txt after rollback", process.PID, fd.FD),
				"tracked.txt path was restored while the deleted file descriptor residue remained reachable",
			}
		}
	}

	return false, nil
}

func pathInCandidates(path string, candidates []string) bool {
	for _, candidate := range pathCandidates(path) {
		for _, expected := range candidates {
			if candidate == expected {
				return true
			}
		}
	}
	return false
}

// branchLeakageOracle confirms that the final committed branch state contains
// an artifact that came only from a discarded speculative branch.
func branchLeakageOracle(before Snapshot, after Snapshot) (bool, []string) {
	beforeSet := before.Paths()
	afterSet := after.Paths()

	_, committed := afterSet["committed-branch-b.txt"]
	discardedEntry, discarded := afterSet["discarded-branch-a.txt"]
	if !committed || !discarded {
		return false, nil
	}
	if _, existed := beforeSet["discarded-branch-a.txt"]; existed {
		return false, nil
	}

	return true, []string{
		"committed branch output exists: committed-branch-b.txt",
		"discarded branch artifact leaked into final state: " + discardedEntry.Path,
	}
}
