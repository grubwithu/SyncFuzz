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
	contract, err := NewLangGraphRetainedResourceContract(LangGraphRetainedUnixSocket, passiveUnixSocketPath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, err
	}
	snapshot, _, err := CaptureLangGraphWorkspaceSnapshotForContract(sourceWorkspace, contract)
	return snapshot, err
}

// CaptureLangGraphWorkspaceFileSnapshot freezes one regular workspace file as
// a retained source node. CloneTo excludes this file and recovery bind-mounts
// it read-only, preserving filesystem identity instead of copying it.
func CaptureLangGraphWorkspaceFileSnapshot(sourceWorkspace, passiveWorkspaceFilePath string) (LangGraphWorkspaceSnapshot, error) {
	contract, err := NewLangGraphRetainedResourceContract(LangGraphRetainedWorkspaceFile, passiveWorkspaceFilePath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, err
	}
	snapshot, _, err := CaptureLangGraphWorkspaceSnapshotForContract(sourceWorkspace, contract)
	return snapshot, err
}

// CaptureLangGraphWorkspaceSnapshotForContract records a source workspace and
// its topology in one pass before recovery begins. The topology is returned on
// a contract violation so callers can persist a structured rejection artifact.
func CaptureLangGraphWorkspaceSnapshotForContract(sourceWorkspace string, contract LangGraphRetainedResourceContract) (LangGraphWorkspaceSnapshot, LangGraphWorkspaceTopology, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(sourceWorkspace))
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, fmt.Errorf("resolve LangGraph source workspace: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, err
	}
	resourcePath, err := workspaceChild(workspace, contract.WorkspaceRelativePath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, err
	}
	resourceInfo, err := os.Lstat(resourcePath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, fmt.Errorf("lstat retained LangGraph resource %s: %w", resourcePath, err)
	}
	if contract.Kind == LangGraphRetainedWorkspaceFile && !resourceInfo.Mode().IsRegular() {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, fmt.Errorf("retained LangGraph resource %s is not a regular workspace file", resourcePath)
	}
	if contract.Kind == LangGraphRetainedUnixSocket && resourceInfo.Mode()&os.ModeSocket == 0 {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, fmt.Errorf("retained LangGraph endpoint %s is not a Unix socket", resourcePath)
	}
	topology, err := InspectLangGraphWorkspaceTopology(workspace, contract)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, LangGraphWorkspaceTopology{}, err
	}
	if err := topology.ValidateForRecovery(); err != nil {
		return LangGraphWorkspaceSnapshot{}, topology, err
	}
	stat, ok := resourceInfo.Sys().(*syscall.Stat_t)
	if !ok || stat.Ino == 0 {
		return LangGraphWorkspaceSnapshot{}, topology, fmt.Errorf("retained LangGraph resource %s lacks inode metadata", resourcePath)
	}
	workspaceDigest, err := digestWorkspaceTree(workspace, contract.WorkspaceRelativePath)
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, topology, err
	}
	checkpointDigest, err := digestWorkspaceTree(filepath.Join(workspace, langGraphCheckpointStoreRelativePath), "")
	if err != nil {
		return LangGraphWorkspaceSnapshot{}, topology, fmt.Errorf("digest LangGraph checkpoint store: %w", err)
	}
	snapshot := LangGraphWorkspaceSnapshot{
		SourceWorkspace:             workspace,
		WorkspaceSHA256:             workspaceDigest,
		CheckpointStoreRelativePath: langGraphCheckpointStoreRelativePath,
		CheckpointStoreSHA256:       checkpointDigest,
	}
	if contract.Kind == LangGraphRetainedWorkspaceFile {
		snapshot.PassiveWorkspaceFilePath = contract.WorkspaceRelativePath
		snapshot.PassiveWorkspaceFileDevice = uint64(stat.Dev)
		snapshot.PassiveWorkspaceFileInode = uint64(stat.Ino)
		snapshot.PassiveWorkspaceFileMode = uint32(resourceInfo.Mode().Perm())
	} else {
		snapshot.PassiveUnixSocketPath = contract.WorkspaceRelativePath
		snapshot.PassiveUnixSocketDevice = uint64(stat.Dev)
		snapshot.PassiveUnixSocketInode = uint64(stat.Ino)
		snapshot.PassiveUnixSocketMode = uint32(resourceInfo.Mode().Perm())
	}
	return snapshot, topology, nil
}

// InspectLangGraphWorkspaceTopology enumerates only source nodes that cannot
// be copied safely by the recovery clone. The configured retained resource is
// the sole permitted special node, when it is a Unix socket.
func InspectLangGraphWorkspaceTopology(sourceWorkspace string, contract LangGraphRetainedResourceContract) (LangGraphWorkspaceTopology, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(sourceWorkspace))
	if err != nil {
		return LangGraphWorkspaceTopology{}, fmt.Errorf("resolve LangGraph source workspace: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return LangGraphWorkspaceTopology{}, err
	}
	rootInfo, err := os.Lstat(workspace)
	if err != nil {
		return LangGraphWorkspaceTopology{}, err
	}
	if !rootInfo.IsDir() {
		return LangGraphWorkspaceTopology{}, fmt.Errorf("LangGraph workspace %s is not a directory", workspace)
	}
	resourcePath, err := workspaceChild(workspace, contract.WorkspaceRelativePath)
	if err != nil {
		return LangGraphWorkspaceTopology{}, err
	}
	resourceInfo, err := os.Lstat(resourcePath)
	if err != nil {
		return LangGraphWorkspaceTopology{}, fmt.Errorf("lstat retained LangGraph resource %s: %w", resourcePath, err)
	}
	if contract.Kind == LangGraphRetainedUnixSocket && resourceInfo.Mode()&os.ModeSocket == 0 {
		return LangGraphWorkspaceTopology{}, fmt.Errorf("retained LangGraph endpoint %s is not a Unix socket", resourcePath)
	}
	if contract.Kind == LangGraphRetainedWorkspaceFile && !resourceInfo.Mode().IsRegular() {
		return LangGraphWorkspaceTopology{}, fmt.Errorf("retained LangGraph resource %s is not a regular workspace file", resourcePath)
	}
	topology := LangGraphWorkspaceTopology{
		SchemaVersion:    LangGraphWorkspaceTopologySchema,
		SourceWorkspace:  workspace,
		ResourceContract: contract,
		UnexpectedNodes:  make([]LangGraphWorkspaceTopologyNode, 0),
	}
	err = filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		if relativePath == "." || relativePath == contract.WorkspaceRelativePath {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			topology.UnexpectedNodes = append(topology.UnexpectedNodes, LangGraphWorkspaceTopologyNode{WorkspaceRelativePath: relativePath, Kind: "symlink"})
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			topology.UnexpectedNodes = append(topology.UnexpectedNodes, LangGraphWorkspaceTopologyNode{WorkspaceRelativePath: relativePath, Kind: langGraphWorkspaceNodeKind(info.Mode())})
		}
		return nil
	})
	if err != nil {
		return LangGraphWorkspaceTopology{}, err
	}
	sort.Slice(topology.UnexpectedNodes, func(left, right int) bool {
		return topology.UnexpectedNodes[left].WorkspaceRelativePath < topology.UnexpectedNodes[right].WorkspaceRelativePath
	})
	if err := topology.Validate(); err != nil {
		return LangGraphWorkspaceTopology{}, err
	}
	return topology, nil
}

func langGraphWorkspaceNodeKind(mode fs.FileMode) string {
	switch {
	case mode&os.ModeSocket != 0:
		return "unix-socket"
	case mode&os.ModeNamedPipe != 0:
		return "fifo"
	case mode&os.ModeDevice != 0 && mode&os.ModeCharDevice != 0:
		return "character-device"
	case mode&os.ModeDevice != 0:
		return "block-device"
	default:
		return "special-file"
	}
}

// VerifySource asserts that the profiled workspace still represents exactly
// the snapshot frozen in the fork plan before a recovery clone is created.
func (s LangGraphWorkspaceSnapshot) VerifySource() error {
	if err := s.Validate(); err != nil {
		return err
	}
	var (
		actual LangGraphWorkspaceSnapshot
		err    error
	)
	if s.PassiveUnixSocketPath != "" {
		actual, err = CaptureLangGraphWorkspaceSnapshot(s.SourceWorkspace, s.PassiveUnixSocketPath)
	} else {
		actual, err = CaptureLangGraphWorkspaceFileSnapshot(s.SourceWorkspace, s.PassiveWorkspaceFilePath)
	}
	if err != nil {
		return err
	}
	if actual.WorkspaceSHA256 != s.WorkspaceSHA256 || actual.CheckpointStoreSHA256 != s.CheckpointStoreSHA256 || actual.PassiveUnixSocketDevice != s.PassiveUnixSocketDevice || actual.PassiveUnixSocketInode != s.PassiveUnixSocketInode || actual.PassiveUnixSocketMode != s.PassiveUnixSocketMode || actual.PassiveWorkspaceFileDevice != s.PassiveWorkspaceFileDevice || actual.PassiveWorkspaceFileInode != s.PassiveWorkspaceFileInode || actual.PassiveWorkspaceFileMode != s.PassiveWorkspaceFileMode {
		return fmt.Errorf("LangGraph source workspace no longer matches the recorded snapshot")
	}
	return nil
}

// CloneTo copies the regular-file portion of this snapshot into destination.
// The retained resource is intentionally excluded and must be bind-mounted by
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
	entries, err := workspaceEntries(s.SourceWorkspace, s.PassiveResourcePath())
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

func (s LangGraphWorkspaceSnapshot) SourcePassiveResourcePath() string {
	return filepath.Join(s.SourceWorkspace, s.PassiveResourcePath())
}

func (s LangGraphWorkspaceSnapshot) PassiveResourcePath() string {
	if s.PassiveUnixSocketPath != "" {
		return s.PassiveUnixSocketPath
	}
	return s.PassiveWorkspaceFilePath
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
