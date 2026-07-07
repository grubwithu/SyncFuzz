package syncfuzz

import (
	"path/filepath"
	"testing"
)

func TestOrphanProcessOracleConfirmsLateEffect(t *testing.T) {
	before := Snapshot{Files: []FileEntry{}}
	after := Snapshot{Files: []FileEntry{{Path: "late-effect", Type: "file"}}}

	confirmed, evidence := orphanProcessOracle(before, after)
	if !confirmed {
		t.Fatalf("expected oracle to confirm late-effect residue")
	}
	if len(evidence) != 1 {
		t.Fatalf("expected one evidence item, got %d", len(evidence))
	}
}

func TestOrphanProcessOracleIgnoresExistingLateEffect(t *testing.T) {
	before := Snapshot{Files: []FileEntry{{Path: "late-effect", Type: "file"}}}
	after := Snapshot{Files: []FileEntry{{Path: "late-effect", Type: "file"}}}

	confirmed, evidence := orphanProcessOracle(before, after)
	if confirmed {
		t.Fatalf("expected oracle to ignore a pre-existing late-effect")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestActionReplayOracleConfirmsDuplicateExternalEffect(t *testing.T) {
	var before ExternalState
	var after ExternalState
	after.Effects.Resources = []EffectResource{
		{ID: "res_1", RequestID: "req-run123-attempt-1"},
		{ID: "res_2", RequestID: "req-run123-attempt-2"},
	}

	confirmed, evidence := actionReplayOracle(before, after, "run123")
	if !confirmed {
		t.Fatalf("expected oracle to confirm duplicate external effect")
	}
	if len(evidence) != 2 {
		t.Fatalf("expected two evidence items, got %d", len(evidence))
	}
}

func TestActionReplayOracleIgnoresSingleExternalEffect(t *testing.T) {
	var before ExternalState
	var after ExternalState
	after.Effects.Resources = []EffectResource{
		{ID: "res_1", RequestID: "req-run123-attempt-1"},
	}

	confirmed, evidence := actionReplayOracle(before, after, "run123")
	if confirmed {
		t.Fatalf("expected oracle to ignore a single external effect")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestAuthorityResurrectionOracleConfirmsStaleTokenReuse(t *testing.T) {
	var after ExternalState
	after.Authority.Tokens = []AuthorityToken{
		{Token: "tok_1", Consumed: true, ConsumedBy: "deploy-branch-a"},
	}
	replay := &consumeTokenResponse{Error: "token_already_consumed", Accepted: false}

	confirmed, evidence := authorityResurrectionOracle(after, "tok_1", replay)
	if !confirmed {
		t.Fatalf("expected oracle to confirm stale token reuse")
	}
	if len(evidence) != 2 {
		t.Fatalf("expected two evidence items, got %d", len(evidence))
	}
}

func TestAuthorityResurrectionOracleIgnoresUnusedToken(t *testing.T) {
	var after ExternalState
	after.Authority.Tokens = []AuthorityToken{
		{Token: "tok_1", Consumed: false},
	}
	replay := &consumeTokenResponse{Error: "token_already_consumed", Accepted: false}

	confirmed, evidence := authorityResurrectionOracle(after, "tok_1", replay)
	if confirmed {
		t.Fatalf("expected oracle to ignore an unconsumed authority token")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestPersistentShellPoisoningOracleConfirmsResidue(t *testing.T) {
	before := ShellState{PWD: "/workspace", GitResolution: "/usr/bin/git"}
	after := ShellState{
		PWD:           "/workspace/attacker-dir",
		GitResolution: "/workspace/attacker-bin/git",
		Aliases:       []string{"alias sfuzz_probe='echo poisoned-alias'"},
	}

	confirmed, evidence := persistentShellPoisoningOracle(before, after)
	if !confirmed {
		t.Fatalf("expected oracle to confirm persistent shell residue")
	}
	if len(evidence) != 3 {
		t.Fatalf("expected three evidence items, got %d", len(evidence))
	}
}

func TestPersistentShellPoisoningOracleIgnoresCleanShell(t *testing.T) {
	before := ShellState{PWD: "/workspace", GitResolution: "/usr/bin/git"}
	after := ShellState{PWD: "/workspace", GitResolution: "/usr/bin/git"}

	confirmed, evidence := persistentShellPoisoningOracle(before, after)
	if confirmed {
		t.Fatalf("expected oracle to ignore clean shell state")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestPartialFilesystemRollbackOracleConfirmsResidue(t *testing.T) {
	before := Snapshot{Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}
	after := Snapshot{Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rwxr-xr-x"},
		{Path: "untracked.txt", Type: "file", Mode: "-rw-r--r--"},
		{Path: "link-to-tracked", Type: "symlink", Mode: "Lrwxrwxrwx"},
	}}

	confirmed, evidence := partialFilesystemRollbackOracle(before, after)
	if !confirmed {
		t.Fatalf("expected oracle to confirm partial filesystem rollback")
	}
	if len(evidence) != 3 {
		t.Fatalf("expected three evidence items, got %d", len(evidence))
	}
}

func TestPartialFilesystemRollbackOracleIgnoresCleanRollback(t *testing.T) {
	before := Snapshot{Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}
	after := Snapshot{Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}

	confirmed, evidence := partialFilesystemRollbackOracle(before, after)
	if confirmed {
		t.Fatalf("expected oracle to ignore clean rollback")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestPartialFilesystemRollbackFDOracleConfirmsDeletedFDResidue(t *testing.T) {
	after := Snapshot{Root: "/tmp/workspace", Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}
	processAfter := ProcessSnapshot{
		Processes: []ProcessEntry{
			{
				PID:              42,
				WorkspaceRelated: true,
				OpenFDs: []ProcessFDEntry{
					{FD: 9, Target: "/tmp/workspace/tracked.txt (deleted)", Deleted: true, WorkspaceRelated: true},
				},
			},
		},
	}

	confirmed, evidence := partialFilesystemRollbackFDOracle(after, processAfter, "/tmp/workspace")
	if !confirmed {
		t.Fatalf("expected fd oracle to confirm deleted workspace residue")
	}
	if len(evidence) != 2 {
		t.Fatalf("expected two evidence items, got %d", len(evidence))
	}
}

func TestPartialFilesystemRollbackFDOracleIgnoresCleanRollback(t *testing.T) {
	after := Snapshot{Root: "/tmp/workspace", Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}
	processAfter := ProcessSnapshot{
		Processes: []ProcessEntry{
			{
				PID:              42,
				WorkspaceRelated: true,
				OpenFDs: []ProcessFDEntry{
					{FD: 9, Target: "/tmp/workspace/tracked.txt", Deleted: false, WorkspaceRelated: true},
				},
			},
		},
	}

	confirmed, evidence := partialFilesystemRollbackFDOracle(after, processAfter, "/tmp/workspace")
	if confirmed {
		t.Fatalf("expected fd oracle to ignore clean rollback")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}

func TestPartialFilesystemRollbackFDOracleMatchesRelativeWorkspace(t *testing.T) {
	absTarget, err := filepath.Abs(filepath.Join("runs/example/workspace", "tracked.txt"))
	if err != nil {
		t.Fatalf("resolve target path: %v", err)
	}
	after := Snapshot{Root: "runs/example/workspace", Files: []FileEntry{
		{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--"},
	}}
	processAfter := ProcessSnapshot{
		Processes: []ProcessEntry{
			{
				PID:              42,
				WorkspaceRelated: true,
				OpenFDs: []ProcessFDEntry{
					{
						FD:               9,
						Target:           absTarget + " (deleted)",
						Deleted:          true,
						WorkspaceRelated: true,
					},
				},
			},
		},
	}

	confirmed, evidence := partialFilesystemRollbackFDOracle(after, processAfter, "runs/example/workspace")
	if !confirmed {
		t.Fatalf("expected fd oracle to match relative workspace, got evidence %#v", evidence)
	}
}

func TestBranchLeakageOracleConfirmsDiscardedBranchResidue(t *testing.T) {
	before := Snapshot{Files: []FileEntry{
		{Path: "base.txt", Type: "file"},
	}}
	after := Snapshot{Files: []FileEntry{
		{Path: "base.txt", Type: "file"},
		{Path: "discarded-branch-a.txt", Type: "file"},
		{Path: "committed-branch-b.txt", Type: "file"},
	}}

	confirmed, evidence := branchLeakageOracle(before, after)
	if !confirmed {
		t.Fatalf("expected oracle to confirm branch leakage")
	}
	if len(evidence) != 2 {
		t.Fatalf("expected two evidence items, got %d", len(evidence))
	}
}

func TestBranchLeakageOracleRequiresCommittedBranch(t *testing.T) {
	before := Snapshot{Files: []FileEntry{
		{Path: "base.txt", Type: "file"},
	}}
	after := Snapshot{Files: []FileEntry{
		{Path: "base.txt", Type: "file"},
		{Path: "discarded-branch-a.txt", Type: "file"},
	}}

	confirmed, evidence := branchLeakageOracle(before, after)
	if confirmed {
		t.Fatalf("expected oracle to require committed branch output")
	}
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence, got %d", len(evidence))
	}
}
