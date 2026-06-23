package syncfuzz

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
