package core

import "testing"

func TestAnalyzeFilesystemMetadataDetectsDeltas(t *testing.T) {
	before := Snapshot{
		Root:    "/workspace",
		TakenAt: "2026-01-01T00:00:00Z",
		Files: []FileEntry{
			{Path: "tracked.txt", Type: "file", Mode: "-rw-r--r--", Size: 8, SHA256: "before"},
		},
	}
	after := Snapshot{
		Root:    "/workspace",
		TakenAt: "2026-01-01T00:00:01Z",
		Files: []FileEntry{
			{Path: "tracked.txt", Type: "file", Mode: "-rwxr-xr-x", Size: 8, SHA256: "after"},
			{Path: "link-to-tracked", Type: "symlink", Mode: "Lrwxrwxrwx", SymlinkTarget: "tracked.txt"},
			{Path: "untracked.txt", Type: "file", Mode: "-rw-r--r--", Size: 7, SHA256: "new"},
		},
	}

	report := AnalyzeFilesystemMetadata([]FilesystemSnapshotArtifact{
		{Phase: "P0", Artifact: "snapshot-before.json", Snapshot: before},
		{Phase: "P6", Artifact: "snapshot-after.json", Snapshot: after},
	})

	if len(report.Snapshots) != 2 || len(report.Deltas) != 1 {
		t.Fatalf("unexpected report shape: %#v", report)
	}
	if report.Snapshots[1].TypeCounts["symlink"] != 1 {
		t.Fatalf("expected symlink count in after snapshot: %#v", report.Snapshots[1])
	}
	delta := report.Deltas[0]
	if !hasString(delta.Added, "link-to-tracked") || !hasString(delta.Added, "untracked.txt") {
		t.Fatalf("expected added paths, got %#v", delta.Added)
	}
	if !hasString(delta.ContentChanged, "tracked.txt") {
		t.Fatalf("expected tracked content change, got %#v", delta.ContentChanged)
	}
	if !hasMetadataChange(delta.MetadataChanged, "tracked.txt", "mode") {
		t.Fatalf("expected tracked mode metadata change, got %#v", delta.MetadataChanged)
	}
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasMetadataChange(changes []FilesystemMetaChange, path string, field string) bool {
	for _, change := range changes {
		if change.Path == path && change.Field == field {
			return true
		}
	}
	return false
}
