package recovery

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
	"syscall"
)

const langGraphCheckpointStoreRelativePath = "langgraph-checkpoints"

// CaptureLangGraphWorkspaceSnapshot records a source workspace without
// dereferencing links or accepting unmodelled special files. This keeps a
// recovery clone from following task-controlled paths outside its workspace.
func CaptureLangGraphWorkspaceSnapshot(sourceWorkspace, passiveUnixSocketPath string) (LangGraphWorkspaceSnapshot, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(sourceWorkspace))
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("resolve LangGraph source workspace: %w", err)
	}
	if strings.TrimSpace(passiveUnixSocketPath) == "" || filepath.IsAbs(passiveUnixSocketPath) {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("LangGraph snapshot requires a workspace-relative passive Unix socket path")
	}
	socketPath, err := workspaceChild(workspace, passiveUnixSocketPath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, err
	}
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("lstat retained LangGraph socket %s: %w", socketPath, err)
	}
	if socketInfo.Mode()&os.ModeSocket == 0 {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("retained LangGraph endpoint %s is not a Unix socket", socketPath)
	}
	stat, ok := socketInfo.Sys().(*syscall.Stat_t)
	if !ok || stat.Ino == 0 {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("retained LangGraph socket %s lacks inode metadata", socketPath)
	}
	workspaceDigest, err := digestWorkspaceTree(workspace, passiveUnixSocketPath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, err
	}
	checkpointDigest, err := digestWorkspaceTree(filepath.Join(workspace, langGraphCheckpointStoreRelativePath), "")
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, fmt.Errorf("digest LangGraph checkpoint store: %w", err)
	}
	return LangGraphWorkspaceSnapshot{
		SourceWorkspace:             workspace,
		WorkspaceSHA256:             workspaceDigest,
		CheckpointStoreRelativePath: langGraphCheckpointStoreRelativePath,
		CheckpointStoreSHA256:       checkpointDigest,
		PassiveUnixSocketPath:       filepath.Clean(passiveUnixSocketPath),
		PassiveUnixSocketDevice:     uint64(stat.Dev),
		PassiveUnixSocketInode:      uint64(stat.Ino),
		PassiveUnixSocketMode:       uint32(socketInfo.Mode().Perm()),
	}, nil
}

// VerifySource asserts that the profiled workspace still represents exactly
// the snapshot frozen in the fork plan before a recovery clone is created.
func (s LangGraphWorkspaceSnapshot) VerifySource() error {
	if err := s.Validate(); err != nil {
		return err
	}
	actual, err := CaptureLangGraphWorkspaceSnapshot(s.SourceWorkspace, s.PassiveUnixSocketPath)
	if err != nil {
		return err
	}
	if actual.WorkspaceSHA256 != s.WorkspaceSHA256 || actual.CheckpointStoreSHA256 != s.CheckpointStoreSHA256 || actual.PassiveUnixSocketDevice != s.PassiveUnixSocketDevice || actual.PassiveUnixSocketInode != s.PassiveUnixSocketInode || actual.PassiveUnixSocketMode != s.PassiveUnixSocketMode {
		return fmt.Errorf("LangGraph source workspace no longer matches the recorded snapshot")
	}
	return nil
}

// CloneTo copies the regular-file portion of this snapshot into destination.
// The retained socket is intentionally excluded and must be bind-mounted by
// the recovery container at the same workspace-relative path.
func (s LangGraphWorkspaceSnapshot) CloneTo(destination string) error {
	if err := s.VerifySource(); err != nil {
		return err
	}
	destination, err := filepath.Abs(strings.TrimSpace(destination))
	if err != nil {
		return fmt.Errorf("resolve LangGraph recovery workspace: %w", err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	entries, err := workspaceEntries(s.SourceWorkspace, s.PassiveUnixSocketPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		target := filepath.Join(destination, entry.relativePath)
		if entry.info.IsDir() {
			if err := os.MkdirAll(target, entry.info.Mode().Perm()); err != nil {
				return err
			}
			if err := os.Chmod(target, entry.info.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyRegularFile(entry.path, target, entry.info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func (s LangGraphWorkspaceSnapshot) SourceSocketPath() string {
	return filepath.Join(s.SourceWorkspace, s.PassiveUnixSocketPath)
}

type workspaceEntry struct {
	path         string
	relativePath string
	info         fs.FileInfo
}

func digestWorkspaceTree(root, excludedRelativePath string) (string, error) {
	entries, err := workspaceEntries(root, excludedRelativePath)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, entry := range entries {
		if entry.relativePath == "." {
			continue
		}
		if _, err := io.WriteString(hash, entry.relativePath+"\x00"+entry.info.Mode().String()+"\x00"); err != nil {
			return "", err
		}
		if entry.info.Mode().IsRegular() {
			file, err := os.Open(entry.path)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return "", copyErr
			}
			if closeErr != nil {
				return "", closeErr
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func workspaceEntries(root, excludedRelativePath string) ([]workspaceEntry, error) {
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("LangGraph workspace %s is not a directory", root)
	}
	excluded := filepath.Clean(excludedRelativePath)
	if excluded == "." {
		excluded = ""
	}
	entries := make([]workspaceEntry, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excluded != "" && relativePath == excluded {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("LangGraph workspace snapshot rejects symlink %s", relativePath)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("LangGraph workspace snapshot rejects special file %s", relativePath)
		}
		entries = append(entries, workspaceEntry{path: path, relativePath: relativePath, info: info})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].relativePath < entries[right].relativePath })
	return entries, nil
}

func workspaceChild(root, relativePath string) (string, error) {
	candidate := filepath.Join(root, filepath.Clean(relativePath))
	if relativePath == "." || filepath.Clean(relativePath) == ".." {
		return "", fmt.Errorf("invalid LangGraph workspace-relative path %q", relativePath)
	}
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolvedCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if resolvedCandidate != resolvedRoot && !strings.HasPrefix(resolvedCandidate, resolvedRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("LangGraph workspace-relative path %q escapes %s", relativePath, root)
	}
	return resolvedCandidate, nil
}

func copyRegularFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(destination, mode)
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
