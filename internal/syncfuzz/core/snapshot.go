package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxHashBytes = 16 << 20

// FileEntry is a compact filesystem state projection. It is not a full backup:
// it records enough metadata to detect residue and to explain a mismatch.
type FileEntry struct {
	Path          string `json:"path"`
	Type          string `json:"type"`
	Mode          string `json:"mode"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256,omitempty"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	ModTime       string `json:"mod_time"`
}

// Snapshot captures the observable workspace state at one lifecycle boundary.
type Snapshot struct {
	Root    string      `json:"root"`
	TakenAt string      `json:"taken_at"`
	Files   []FileEntry `json:"files"`
}

type FilesystemSnapshotArtifact struct {
	Phase    string
	Artifact string
	Snapshot Snapshot
}

type FilesystemMetadataReport struct {
	Root        string                      `json:"root"`
	GeneratedAt string                      `json:"generated_at"`
	Snapshots   []FilesystemSnapshotSummary `json:"snapshots"`
	Deltas      []FilesystemMetadataDelta   `json:"deltas"`
}

type FilesystemSnapshotSummary struct {
	Phase           string         `json:"phase"`
	Artifact        string         `json:"artifact"`
	TakenAt         string         `json:"taken_at"`
	TotalEntries    int            `json:"total_entries"`
	TotalBytes      int64          `json:"total_bytes"`
	TypeCounts      map[string]int `json:"type_counts"`
	ModeCounts      map[string]int `json:"mode_counts"`
	HashableFiles   int            `json:"hashable_files"`
	ExecutableFiles []string       `json:"executable_files,omitempty"`
	Symlinks        []string       `json:"symlinks,omitempty"`
}

type FilesystemMetadataDelta struct {
	FromPhase       string                 `json:"from_phase"`
	ToPhase         string                 `json:"to_phase"`
	FromArtifact    string                 `json:"from_artifact"`
	ToArtifact      string                 `json:"to_artifact"`
	Added           []string               `json:"added,omitempty"`
	Removed         []string               `json:"removed,omitempty"`
	ContentChanged  []string               `json:"content_changed,omitempty"`
	MetadataChanged []FilesystemMetaChange `json:"metadata_changed,omitempty"`
}

type FilesystemMetaChange struct {
	Path   string `json:"path"`
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (s Snapshot) Paths() map[string]FileEntry {
	out := make(map[string]FileEntry, len(s.Files))
	for _, entry := range s.Files {
		out[entry.Path] = entry
	}
	return out
}

func SnapshotFilesystem(root string) (Snapshot, error) {
	files := make([]FileEntry, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		item := FileEntry{
			Path:    rel,
			Type:    fileType(info),
			Mode:    info.Mode().String(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Do not follow symlinks during snapshotting; the symlink itself is
			// often the security-relevant object.
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			item.SymlinkTarget = target
		} else if info.Mode().IsRegular() && info.Size() <= maxHashBytes {
			// Hash small regular files so oracle/debug output can distinguish
			// content changes without storing file contents in artifacts.
			sum, err := hashFile(path)
			if err != nil {
				return err
			}
			item.SHA256 = sum
		}

		files = append(files, item)
		return nil
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot %s: %w", root, err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return Snapshot{
		Root:    root,
		TakenAt: time.Now().UTC().Format(time.RFC3339Nano),
		Files:   files,
	}, nil
}

func AnalyzeFilesystemMetadata(snapshots []FilesystemSnapshotArtifact) FilesystemMetadataReport {
	report := FilesystemMetadataReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Snapshots:   make([]FilesystemSnapshotSummary, 0, len(snapshots)),
	}
	if len(snapshots) > 0 {
		report.Root = snapshots[0].Snapshot.Root
	}
	for _, snapshot := range snapshots {
		report.Snapshots = append(report.Snapshots, summarizeFilesystemSnapshot(snapshot))
	}
	for i := 1; i < len(snapshots); i++ {
		report.Deltas = append(report.Deltas, compareFilesystemSnapshots(snapshots[i-1], snapshots[i]))
	}
	return report
}

func summarizeFilesystemSnapshot(snapshot FilesystemSnapshotArtifact) FilesystemSnapshotSummary {
	summary := FilesystemSnapshotSummary{
		Phase:        snapshot.Phase,
		Artifact:     snapshot.Artifact,
		TakenAt:      snapshot.Snapshot.TakenAt,
		TotalEntries: len(snapshot.Snapshot.Files),
		TypeCounts:   make(map[string]int),
		ModeCounts:   make(map[string]int),
	}
	for _, entry := range snapshot.Snapshot.Files {
		summary.TypeCounts[entry.Type]++
		summary.ModeCounts[entry.Mode]++
		summary.TotalBytes += entry.Size
		if entry.SHA256 != "" {
			summary.HashableFiles++
		}
		if entry.Type == "symlink" {
			summary.Symlinks = append(summary.Symlinks, entry.Path)
		}
		if entry.Type == "file" && strings.Contains(entry.Mode, "x") {
			summary.ExecutableFiles = append(summary.ExecutableFiles, entry.Path)
		}
	}
	sort.Strings(summary.ExecutableFiles)
	sort.Strings(summary.Symlinks)
	return summary
}

func compareFilesystemSnapshots(from FilesystemSnapshotArtifact, to FilesystemSnapshotArtifact) FilesystemMetadataDelta {
	fromPaths := from.Snapshot.Paths()
	toPaths := to.Snapshot.Paths()
	delta := FilesystemMetadataDelta{
		FromPhase:    from.Phase,
		ToPhase:      to.Phase,
		FromArtifact: from.Artifact,
		ToArtifact:   to.Artifact,
	}

	for path, toEntry := range toPaths {
		fromEntry, existed := fromPaths[path]
		if !existed {
			delta.Added = append(delta.Added, path)
			continue
		}
		if fromEntry.SHA256 != "" && toEntry.SHA256 != "" && fromEntry.SHA256 != toEntry.SHA256 {
			delta.ContentChanged = append(delta.ContentChanged, path)
		}
		delta.MetadataChanged = append(delta.MetadataChanged, metadataChanges(fromEntry, toEntry)...)
	}
	for path := range fromPaths {
		if _, exists := toPaths[path]; !exists {
			delta.Removed = append(delta.Removed, path)
		}
	}

	sort.Strings(delta.Added)
	sort.Strings(delta.Removed)
	sort.Strings(delta.ContentChanged)
	sort.Slice(delta.MetadataChanged, func(i, j int) bool {
		if delta.MetadataChanged[i].Path != delta.MetadataChanged[j].Path {
			return delta.MetadataChanged[i].Path < delta.MetadataChanged[j].Path
		}
		return delta.MetadataChanged[i].Field < delta.MetadataChanged[j].Field
	})
	return delta
}

func metadataChanges(before FileEntry, after FileEntry) []FilesystemMetaChange {
	var changes []FilesystemMetaChange
	if before.Type != after.Type {
		changes = append(changes, FilesystemMetaChange{Path: before.Path, Field: "type", Before: before.Type, After: after.Type})
	}
	if before.Mode != after.Mode {
		changes = append(changes, FilesystemMetaChange{Path: before.Path, Field: "mode", Before: before.Mode, After: after.Mode})
	}
	if before.Size != after.Size {
		changes = append(changes, FilesystemMetaChange{Path: before.Path, Field: "size", Before: fmt.Sprintf("%d", before.Size), After: fmt.Sprintf("%d", after.Size)})
	}
	if before.SymlinkTarget != after.SymlinkTarget {
		changes = append(changes, FilesystemMetaChange{Path: before.Path, Field: "symlink_target", Before: before.SymlinkTarget, After: after.SymlinkTarget})
	}
	return changes
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileType(info os.FileInfo) string {
	mode := info.Mode()
	switch {
	case mode.IsRegular():
		return "file"
	case mode.IsDir():
		return "dir"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	case mode&os.ModeSocket != 0:
		return "socket"
	case mode&os.ModeNamedPipe != 0:
		return "fifo"
	case mode&os.ModeDevice != 0:
		return "device"
	default:
		return "other"
	}
}
