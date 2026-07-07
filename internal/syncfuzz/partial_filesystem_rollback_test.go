package syncfuzz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPartialFilesystemRollbackOpenFDPrimitiveConfirmsDeletedFDResidue(t *testing.T) {
	tmp := t.TempDir()
	result, err := Run(context.Background(), RunOptions{
		CaseName:    "partial-filesystem-rollback",
		PrimitiveID: "open-fd",
		OutDir:      filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Confirmed {
		t.Fatalf("expected confirmed result, got evidence %#v", result.Evidence)
	}
	if result.Signature.StateClass != "fd" {
		t.Fatalf("expected fd signature, got %#v", result.Signature)
	}

	after := mustReadJSON[Snapshot](t, filepath.Join(result.ArtifactDir, "snapshot-after.json"))
	if _, ok := after.Paths()["tracked.txt"]; !ok {
		t.Fatalf("expected tracked.txt to be restored after rollback")
	}
	if _, ok := after.Paths()["untracked.txt"]; ok {
		t.Fatalf("did not expect untracked file residue for open-fd primitive")
	}
	if _, ok := after.Paths()["link-to-tracked"]; ok {
		t.Fatalf("did not expect symlink residue for open-fd primitive")
	}

	processAfter := mustReadJSON[ProcessSnapshot](t, filepath.Join(result.ArtifactDir, "process-after.json"))
	if !containsDeletedTrackedFD(processAfter, filepath.Join(after.Root, "tracked.txt")) {
		t.Fatalf("expected deleted tracked.txt fd residue, got %#v", processAfter.Processes)
	}
}

func TestRunPartialFilesystemRollbackUntrackedPrimitiveStaysFocused(t *testing.T) {
	tmp := t.TempDir()
	result, err := Run(context.Background(), RunOptions{
		CaseName:    "partial-filesystem-rollback",
		PrimitiveID: "untracked-file",
		OutDir:      filepath.Join(tmp, "runs"),
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Confirmed {
		t.Fatalf("expected confirmed result, got evidence %#v", result.Evidence)
	}

	after := mustReadJSON[Snapshot](t, filepath.Join(result.ArtifactDir, "snapshot-after.json"))
	if _, ok := after.Paths()["untracked.txt"]; !ok {
		t.Fatalf("expected untracked.txt residue for untracked-file primitive")
	}
	if _, ok := after.Paths()["link-to-tracked"]; ok {
		t.Fatalf("did not expect symlink residue for untracked-file primitive")
	}
	if tracked := after.Paths()["tracked.txt"]; tracked.Mode != "-rw-r--r--" {
		t.Fatalf("expected tracked.txt mode reset for untracked-file primitive, got %q", tracked.Mode)
	}
}

func mustReadJSON[T any](t *testing.T, path string) T {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func containsDeletedTrackedFD(snapshot ProcessSnapshot, trackedPath string) bool {
	for _, process := range snapshot.Processes {
		for _, fd := range process.OpenFDs {
			if !fd.Deleted {
				continue
			}
			if strings.TrimSuffix(fd.Target, " (deleted)") == trackedPath {
				return true
			}
		}
	}
	return false
}
