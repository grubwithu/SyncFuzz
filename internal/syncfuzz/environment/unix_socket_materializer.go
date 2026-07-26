package environment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const EnvironmentMaterializationSchemaVersion = "syncfuzz.environment-materialization.v1"

// Linux sockaddr_un reserves 108 bytes including the trailing NUL. The first
// materializer intentionally supports filesystem-bound names only.
const maxFilesystemUnixSocketPathBytes = 107

type ResolutionStepKind string

const (
	ResolutionStepLogicalName ResolutionStepKind = "logical-name"
	ResolutionStepConfig      ResolutionStepKind = "config"
	ResolutionStepEnvironment ResolutionStepKind = "environment"
	ResolutionStepAlias       ResolutionStepKind = "alias"
	ResolutionStepPathname    ResolutionStepKind = "pathname"
)

func (k ResolutionStepKind) Valid() bool {
	switch k {
	case ResolutionStepLogicalName, ResolutionStepConfig, ResolutionStepEnvironment, ResolutionStepAlias, ResolutionStepPathname:
		return true
	default:
		return false
	}
}

// ResolutionStep is a typed, replayable explanation of how a normal
// workload's logical service name became a Unix pathname. It is not inferred
// from the server response.
type ResolutionStep struct {
	Kind         ResolutionStepKind `json:"kind"`
	From         string             `json:"from"`
	To           string             `json:"to"`
	ArtifactPath string             `json:"artifact_path,omitempty"`
	ValueSHA256  string             `json:"value_sha256,omitempty"`
}

func (s ResolutionStep) Validate() error {
	if !s.Kind.Valid() || strings.TrimSpace(s.From) == "" || strings.TrimSpace(s.To) == "" {
		return fmt.Errorf("resolution step is incomplete")
	}
	if s.ArtifactPath != "" && !validRelativeWorkspacePath(s.ArtifactPath) {
		return fmt.Errorf("resolution step has an unsafe artifact path %q", s.ArtifactPath)
	}
	if s.ValueSHA256 != "" && len(s.ValueSHA256) != 64 {
		return fmt.Errorf("resolution step has an invalid value digest")
	}
	return nil
}

// RunLocalIdentity is meaningful only inside one materialized runtime. It is
// intentionally kept separate from SemanticIdentity so callers cannot compare
// a PID or inode across fresh controls.
type RunLocalIdentity struct {
	EndpointDevice uint64 `json:"endpoint_device"`
	EndpointInode  uint64 `json:"endpoint_inode"`
	SocketDevice   uint64 `json:"socket_device"`
	SocketInode    uint64 `json:"socket_inode"`
	HolderPID      int    `json:"holder_pid"`
	HolderFD       int    `json:"holder_fd"`
}

func (i RunLocalIdentity) Validate() error {
	if i.EndpointInode == 0 || i.SocketInode == 0 || i.HolderPID <= 0 || i.HolderFD < 0 {
		return fmt.Errorf("run-local Unix socket identity is incomplete")
	}
	return nil
}

func (i RunLocalIdentity) SocketID() string {
	return fmt.Sprintf("%d:%d", i.SocketDevice, i.SocketInode)
}

// SemanticIdentity is stable only at the declarative role/provenance level.
// It deliberately excludes bare PID, FD, device, inode, and socket ID.
type SemanticIdentity struct {
	ProgramID        string `json:"program_id"`
	LogicalName      string `json:"logical_name"`
	Role             string `json:"role"`
	ResolutionSHA256 string `json:"resolution_sha256"`
	Creator          string `json:"creator"`
}

func (i SemanticIdentity) Validate() error {
	if strings.TrimSpace(i.ProgramID) == "" || !validTopologyToken(i.LogicalName) || !validTopologyToken(i.Role) || len(i.ResolutionSHA256) != 64 || strings.TrimSpace(i.Creator) == "" {
		return fmt.Errorf("semantic Unix socket identity is incomplete")
	}
	return nil
}

type MaterializedUnixSocketBinding struct {
	Semantic  SemanticIdentity `json:"semantic"`
	Local     RunLocalIdentity `json:"local"`
	Listening bool             `json:"listening"`
}

func (b MaterializedUnixSocketBinding) ValidateFor(program EnvironmentProgram) error {
	if err := b.Semantic.Validate(); err != nil {
		return err
	}
	if err := b.Local.Validate(); err != nil {
		return err
	}
	if b.Semantic.ProgramID != program.ProgramID || b.Semantic.LogicalName != program.UnixSocket.LogicalName || !b.Listening {
		return fmt.Errorf("materialized Unix socket binding does not match environment program")
	}
	return nil
}

type MaterializationEvent struct {
	Sequence  int                            `json:"sequence"`
	Operation string                         `json:"operation"`
	Role      string                         `json:"role,omitempty"`
	Binding   *MaterializedUnixSocketBinding `json:"binding,omitempty"`
}

// EnvironmentMaterialization is the observed realization of a declarative
// EnvironmentProgram. Its events are local materializer provenance, not a
// substitute for profile-time eBPF W evidence.
type EnvironmentMaterialization struct {
	SchemaVersion   string                        `json:"schema_version"`
	ProgramID       string                        `json:"program_id"`
	Family          EnvironmentResourceFamily     `json:"family"`
	EndpointPath    string                        `json:"endpoint_path"`
	ResolutionSteps []ResolutionStep              `json:"resolution_steps"`
	InitialBinding  MaterializedUnixSocketBinding `json:"initial_binding"`
	ActiveBinding   MaterializedUnixSocketBinding `json:"active_binding"`
	Events          []MaterializationEvent        `json:"events"`
}

func (m EnvironmentMaterialization) ValidateFor(program EnvironmentProgram) error {
	if err := program.Validate(); err != nil {
		return err
	}
	if m.SchemaVersion != EnvironmentMaterializationSchemaVersion || m.ProgramID != program.ProgramID || m.Family != program.Family || m.EndpointPath != program.UnixSocket.EndpointPath || len(m.ResolutionSteps) < 2 || len(m.Events) < 2 {
		return fmt.Errorf("environment materialization is incomplete for program %q", program.ProgramID)
	}
	for _, step := range m.ResolutionSteps {
		if err := step.Validate(); err != nil {
			return err
		}
	}
	if err := m.InitialBinding.ValidateFor(program); err != nil {
		return fmt.Errorf("initial binding: %w", err)
	}
	if err := m.ActiveBinding.ValidateFor(program); err != nil {
		return fmt.Errorf("active binding: %w", err)
	}
	if m.InitialBinding.Semantic.Role != program.UnixSocket.InitialRole || m.ActiveBinding.Semantic.Role != program.UnixSocket.ActiveRole {
		return fmt.Errorf("materialized binding roles do not match environment program")
	}
	resolutionDigest := ResolutionStepsDigest(m.ResolutionSteps)
	if m.InitialBinding.Semantic.ResolutionSHA256 != resolutionDigest || m.ActiveBinding.Semantic.ResolutionSHA256 != resolutionDigest {
		return fmt.Errorf("materialized binding semantic identity does not match resolution steps")
	}
	if program.Mutation.Operator == MutationOperatorRebind && m.InitialBinding.Local.SocketID() == m.ActiveBinding.Local.SocketID() {
		return fmt.Errorf("rebind materialization reused the initial listener socket identity")
	}
	for index, event := range m.Events {
		if event.Sequence != index+1 || strings.TrimSpace(event.Operation) == "" {
			return fmt.Errorf("materialization event %d is invalid", index)
		}
		if event.Binding != nil {
			if err := event.Binding.ValidateFor(program); err != nil {
				return fmt.Errorf("materialization event %d: %w", index, err)
			}
		}
	}
	return nil
}

type ResolvedUnixSocket struct {
	LogicalName          string           `json:"logical_name"`
	EndpointPath         string           `json:"endpoint_path"`
	ResolutionSteps      []ResolutionStep `json:"resolution_steps"`
	absoluteEndpointPath string
}

func (r ResolvedUnixSocket) AbsoluteEndpointPath() string {
	return r.absoluteEndpointPath
}

// UnixSocketMaterialization owns foreground fixture listeners. It is a local
// deterministic materializer intended for calibration and unit-test closure;
// production target materialization must additionally obtain profile-time W
// evidence under the target cgroup.
type UnixSocketMaterialization struct {
	program      EnvironmentProgram
	workspace    string
	environment  map[string]string
	artifact     EnvironmentMaterialization
	initial      *unixSocketServer
	active       *unixSocketServer
	cleanupPaths []string
	closeOnce    sync.Once
	closeErr     error
}

func (m *UnixSocketMaterialization) Program() EnvironmentProgram {
	return m.program
}

func (m *UnixSocketMaterialization) Artifact() EnvironmentMaterialization {
	artifact := m.artifact
	artifact.ResolutionSteps = append([]ResolutionStep(nil), m.artifact.ResolutionSteps...)
	artifact.Events = append([]MaterializationEvent(nil), m.artifact.Events...)
	return artifact
}

func (m *UnixSocketMaterialization) ActiveBinding() MaterializedUnixSocketBinding {
	return m.artifact.ActiveBinding
}

func (m *UnixSocketMaterialization) BindingForRole(role string) (MaterializedUnixSocketBinding, bool) {
	if m.artifact.ActiveBinding.Semantic.Role == role {
		return m.artifact.ActiveBinding, true
	}
	if m.artifact.InitialBinding.Semantic.Role == role {
		return m.artifact.InitialBinding, true
	}
	return MaterializedUnixSocketBinding{}, false
}

// ResolveLogicalName repeats the declared resolution operation against the
// actual config, environment, or alias artifact. It rejects a changed value
// rather than silently following an undeclared endpoint.
func (m *UnixSocketMaterialization) ResolveLogicalName(ctx context.Context, logicalName string) (ResolvedUnixSocket, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedUnixSocket{}, err
	}
	if strings.TrimSpace(logicalName) != m.program.UnixSocket.LogicalName {
		return ResolvedUnixSocket{}, fmt.Errorf("logical name %q is not declared by environment program %q", logicalName, m.program.ProgramID)
	}
	binding := m.program.UnixSocket
	endpoint := binding.EndpointPath
	steps := []ResolutionStep{{Kind: ResolutionStepLogicalName, From: binding.LogicalName, To: string(binding.ResolutionMode)}}
	switch binding.ResolutionMode {
	case UnixSocketResolutionDirect:
		steps[0].To = endpoint
	case UnixSocketResolutionConfig:
		artifactPath, err := workspacePath(m.workspace, binding.ResolutionArtifactPath)
		if err != nil {
			return ResolvedUnixSocket{}, err
		}
		contents, err := os.ReadFile(artifactPath)
		if err != nil {
			return ResolvedUnixSocket{}, fmt.Errorf("read Unix socket config %q: %w", binding.ResolutionArtifactPath, err)
		}
		values := map[string]string{}
		if err := json.Unmarshal(contents, &values); err != nil {
			return ResolvedUnixSocket{}, fmt.Errorf("parse Unix socket config %q: %w", binding.ResolutionArtifactPath, err)
		}
		value, ok := values[binding.ResolutionKey]
		if !ok {
			return ResolvedUnixSocket{}, fmt.Errorf("Unix socket config %q lacks key %q", binding.ResolutionArtifactPath, binding.ResolutionKey)
		}
		endpoint = strings.TrimSpace(value)
		steps = append(steps,
			ResolutionStep{Kind: ResolutionStepConfig, From: binding.ResolutionKey, To: endpoint, ArtifactPath: binding.ResolutionArtifactPath, ValueSHA256: digestString(string(contents))},
		)
	case UnixSocketResolutionEnvironment:
		value, ok := m.environment[binding.ResolutionKey]
		if !ok {
			return ResolvedUnixSocket{}, fmt.Errorf("Unix socket environment lacks variable %q", binding.ResolutionKey)
		}
		endpoint = strings.TrimSpace(value)
		steps = append(steps,
			ResolutionStep{Kind: ResolutionStepEnvironment, From: binding.ResolutionKey, To: endpoint, ValueSHA256: digestString(value)},
		)
	case UnixSocketResolutionAlias:
		artifactPath, err := workspacePath(m.workspace, binding.ResolutionArtifactPath)
		if err != nil {
			return ResolvedUnixSocket{}, err
		}
		contents, err := os.ReadFile(artifactPath)
		if err != nil {
			return ResolvedUnixSocket{}, fmt.Errorf("read Unix socket alias %q: %w", binding.ResolutionArtifactPath, err)
		}
		endpoint = strings.TrimSpace(string(contents))
		steps = append(steps,
			ResolutionStep{Kind: ResolutionStepAlias, From: binding.ResolutionKey, To: endpoint, ArtifactPath: binding.ResolutionArtifactPath, ValueSHA256: digestString(string(contents))},
		)
	default:
		return ResolvedUnixSocket{}, fmt.Errorf("unsupported Unix socket resolution mode %q", binding.ResolutionMode)
	}
	if endpoint != binding.EndpointPath || !validRelativeWorkspacePath(endpoint) {
		return ResolvedUnixSocket{}, fmt.Errorf("Unix socket resolution produced undeclared or unsafe endpoint %q", endpoint)
	}
	absEndpoint, err := workspacePath(m.workspace, endpoint)
	if err != nil {
		return ResolvedUnixSocket{}, err
	}
	steps = append(steps, ResolutionStep{Kind: ResolutionStepPathname, From: endpoint, To: "unix-endpoint:" + endpoint})
	return ResolvedUnixSocket{LogicalName: binding.LogicalName, EndpointPath: endpoint, ResolutionSteps: steps, absoluteEndpointPath: absEndpoint}, nil
}

func (m *UnixSocketMaterialization) Close() error {
	m.closeOnce.Do(func() {
		var failures []string
		if m.active != nil {
			if err := m.active.Close(); err != nil {
				failures = append(failures, err.Error())
			}
		}
		if m.initial != nil && m.initial != m.active {
			if err := m.initial.Close(); err != nil {
				failures = append(failures, err.Error())
			}
		}
		for _, path := range m.cleanupPaths {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				failures = append(failures, fmt.Sprintf("remove %s: %v", path, err))
			}
		}
		if len(failures) > 0 {
			m.closeErr = fmt.Errorf("close Unix socket materialization: %s", strings.Join(failures, "; "))
		}
	})
	return m.closeErr
}

// MaterializeUnixSocketProgram creates the declared config/env/alias chain,
// binds the initial listener, and, for a rebind mutation, unlinks and rebinds
// the same pathname while preserving the first listener in memory.
func MaterializeUnixSocketProgram(ctx context.Context, program EnvironmentProgram, workspace string) (*UnixSocketMaterialization, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := program.Validate(); err != nil {
		return nil, err
	}
	if program.Family != EnvironmentResourceFamilyUnixSocket {
		return nil, fmt.Errorf("environment program %q is not a Unix socket program", program.ProgramID)
	}
	if program.UnixSocket.HolderLifetime != HolderLifetimeForeground {
		return nil, fmt.Errorf("local Unix socket materializer supports foreground holders only, got %q", program.UnixSocket.HolderLifetime)
	}
	absoluteWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve materialization workspace: %w", err)
	}
	if err := os.MkdirAll(absoluteWorkspace, 0o755); err != nil {
		return nil, fmt.Errorf("create materialization workspace: %w", err)
	}
	endpointPath, err := workspacePath(absoluteWorkspace, program.UnixSocket.EndpointPath)
	if err != nil {
		return nil, err
	}
	if len([]byte(endpointPath)) > maxFilesystemUnixSocketPathBytes {
		return nil, fmt.Errorf("Unix socket endpoint path is %d bytes; filesystem Unix sockets allow at most %d bytes", len([]byte(endpointPath)), maxFilesystemUnixSocketPathBytes)
	}
	if err := os.MkdirAll(filepath.Dir(endpointPath), 0o755); err != nil {
		return nil, fmt.Errorf("create Unix socket endpoint parent: %w", err)
	}
	if _, err := os.Lstat(endpointPath); err == nil {
		return nil, fmt.Errorf("refuse to overwrite pre-existing Unix socket endpoint %q", endpointPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect Unix socket endpoint %q: %w", endpointPath, err)
	}

	materialization := &UnixSocketMaterialization{
		program:      program,
		workspace:    absoluteWorkspace,
		environment:  make(map[string]string),
		cleanupPaths: []string{endpointPath},
	}
	if err := materialization.writeResolutionArtifact(); err != nil {
		_ = materialization.Close()
		return nil, err
	}

	initial, err := startUnixSocketServer(endpointPath, program.UnixSocket.InitialRole)
	if err != nil {
		_ = materialization.Close()
		return nil, fmt.Errorf("bind initial Unix listener: %w", err)
	}
	materialization.initial = initial
	initialBinding, err := materializedBinding(program, program.UnixSocket.InitialRole, initial, endpointPath, nil)
	if err != nil {
		_ = materialization.Close()
		return nil, err
	}

	active := initial
	activeBinding := initialBinding
	events := []MaterializationEvent{
		{Sequence: 1, Operation: "bind", Role: initialBinding.Semantic.Role, Binding: &initialBinding},
		{Sequence: 2, Operation: "listen", Role: initialBinding.Semantic.Role, Binding: &initialBinding},
	}
	if program.Mutation.Operator == MutationOperatorRebind {
		if err := os.Remove(endpointPath); err != nil {
			_ = materialization.Close()
			return nil, fmt.Errorf("unlink initial Unix listener pathname for rebind: %w", err)
		}
		events = append(events, MaterializationEvent{Sequence: len(events) + 1, Operation: "unlink", Role: initialBinding.Semantic.Role})
		active, err = startUnixSocketServer(endpointPath, program.UnixSocket.ActiveRole)
		if err != nil {
			_ = materialization.Close()
			return nil, fmt.Errorf("rebind Unix listener: %w", err)
		}
		materialization.active = active
		activeBinding, err = materializedBinding(program, program.UnixSocket.ActiveRole, active, endpointPath, nil)
		if err != nil {
			_ = materialization.Close()
			return nil, err
		}
		events = append(events,
			MaterializationEvent{Sequence: len(events) + 1, Operation: "bind", Role: activeBinding.Semantic.Role, Binding: &activeBinding},
			MaterializationEvent{Sequence: len(events) + 2, Operation: "listen", Role: activeBinding.Semantic.Role, Binding: &activeBinding},
			MaterializationEvent{Sequence: len(events) + 3, Operation: "rebind", Role: activeBinding.Semantic.Role, Binding: &activeBinding},
		)
	} else {
		materialization.active = initial
	}

	resolved, err := materialization.ResolveLogicalName(ctx, program.UnixSocket.LogicalName)
	if err != nil {
		_ = materialization.Close()
		return nil, err
	}
	resolutionDigest := ResolutionStepsDigest(resolved.ResolutionSteps)
	initialBinding.Semantic.ResolutionSHA256 = resolutionDigest
	activeBinding.Semantic.ResolutionSHA256 = resolutionDigest
	for index := range events {
		if events[index].Binding == nil {
			continue
		}
		if events[index].Binding.Semantic.Role == initialBinding.Semantic.Role {
			binding := initialBinding
			events[index].Binding = &binding
		} else {
			binding := activeBinding
			events[index].Binding = &binding
		}
	}
	materialization.artifact = EnvironmentMaterialization{
		SchemaVersion:   EnvironmentMaterializationSchemaVersion,
		ProgramID:       program.ProgramID,
		Family:          program.Family,
		EndpointPath:    program.UnixSocket.EndpointPath,
		ResolutionSteps: append([]ResolutionStep(nil), resolved.ResolutionSteps...),
		InitialBinding:  initialBinding,
		ActiveBinding:   activeBinding,
		Events:          events,
	}
	if err := materialization.artifact.ValidateFor(program); err != nil {
		_ = materialization.Close()
		return nil, err
	}
	return materialization, nil
}

func (m *UnixSocketMaterialization) writeResolutionArtifact() error {
	binding := m.program.UnixSocket
	switch binding.ResolutionMode {
	case UnixSocketResolutionDirect:
		return nil
	case UnixSocketResolutionEnvironment:
		m.environment[binding.ResolutionKey] = binding.EndpointPath
		return nil
	case UnixSocketResolutionConfig:
		path, err := workspacePath(m.workspace, binding.ResolutionArtifactPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create Unix socket config parent: %w", err)
		}
		contents, err := json.Marshal(map[string]string{binding.ResolutionKey: binding.EndpointPath})
		if err != nil {
			return err
		}
		if err := writeExclusiveFile(path, contents, 0o600); err != nil {
			return fmt.Errorf("write Unix socket config: %w", err)
		}
		m.cleanupPaths = append(m.cleanupPaths, path)
		return nil
	case UnixSocketResolutionAlias:
		path, err := workspacePath(m.workspace, binding.ResolutionArtifactPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create Unix socket alias parent: %w", err)
		}
		if err := writeExclusiveFile(path, []byte(binding.EndpointPath+"\n"), 0o600); err != nil {
			return fmt.Errorf("write Unix socket alias: %w", err)
		}
		m.cleanupPaths = append(m.cleanupPaths, path)
		return nil
	default:
		return fmt.Errorf("unsupported Unix socket resolution mode %q", binding.ResolutionMode)
	}
}

func materializedBinding(program EnvironmentProgram, role string, server *unixSocketServer, endpointPath string, resolutionSteps []ResolutionStep) (MaterializedUnixSocketBinding, error) {
	local, err := server.identity(endpointPath)
	if err != nil {
		return MaterializedUnixSocketBinding{}, err
	}
	resolutionDigest := ""
	if len(resolutionSteps) > 0 {
		resolutionDigest = ResolutionStepsDigest(resolutionSteps)
	}
	return MaterializedUnixSocketBinding{
		Semantic: SemanticIdentity{
			ProgramID:        program.ProgramID,
			LogicalName:      program.UnixSocket.LogicalName,
			Role:             role,
			ResolutionSHA256: resolutionDigest,
			Creator:          "syncfuzz-unix-socket-materializer",
		},
		Local:     local,
		Listening: true,
	}, nil
}

func workspacePath(workspace string, relative string) (string, error) {
	if !validRelativeWorkspacePath(relative) {
		return "", fmt.Errorf("unsafe workspace-relative path %q", relative)
	}
	path := filepath.Join(workspace, relative)
	relativeToWorkspace, err := filepath.Rel(workspace, path)
	if err != nil || relativeToWorkspace == ".." || strings.HasPrefix(relativeToWorkspace, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path %q escapes materialization workspace", relative)
	}
	return path, nil
}

func digestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// ResolutionStepsDigest is the semantic-identity digest of one declared and
// observed resolution chain. It deliberately omits run-local identities.
func ResolutionStepsDigest(steps []ResolutionStep) string {
	encoded, _ := json.Marshal(steps)
	return digestString(string(encoded))
}

func writeExclusiveFile(path string, contents []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(contents); err != nil {
		return err
	}
	return nil
}

type unixSocketServer struct {
	fd        int
	role      string
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type unixSocketResponse struct {
	Role          string `json:"role"`
	RequestSHA256 string `json:"request_sha256"`
}

func startUnixSocketServer(endpointPath string, role string) (*unixSocketServer, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: endpointPath}); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	if err := syscall.Listen(fd, 16); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	// The materializer owns pathname cleanup. Raw syscalls deliberately avoid
	// net.ListenUnix's socket option setup so this fixture remains usable in
	// restricted test sandboxes.
	server := &unixSocketServer{fd: fd, role: role, closed: make(chan struct{})}
	server.wg.Add(1)
	go server.acceptLoop()
	return server, nil
}

func (s *unixSocketServer) acceptLoop() {
	defer s.wg.Done()
	for {
		connectionFD, _, err := syscall.Accept(s.fd)
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
			}
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				select {
				case <-s.closed:
					return
				case <-time.After(5 * time.Millisecond):
					continue
				}
			}
			return
		}
		if err := syscall.SetNonblock(connectionFD, false); err != nil {
			_ = syscall.Close(connectionFD)
			continue
		}
		s.wg.Add(1)
		go func(fd int) {
			defer s.wg.Done()
			s.handle(fd)
		}(connectionFD)
	}
}

func (s *unixSocketServer) handle(connectionFD int) {
	defer syscall.Close(connectionFD)
	buffer := make([]byte, 4096)
	count, err := syscall.Read(connectionFD, buffer)
	if err != nil || count == 0 {
		return
	}
	response, err := json.Marshal(unixSocketResponse{Role: s.role, RequestSHA256: digestString(string(buffer[:count]))})
	if err != nil {
		return
	}
	_ = writeSocketAll(connectionFD, append(response, '\n'))
}

func (s *unixSocketServer) identity(endpointPath string) (RunLocalIdentity, error) {
	var endpointStat syscall.Stat_t
	if err := syscall.Lstat(endpointPath, &endpointStat); err != nil {
		return RunLocalIdentity{}, fmt.Errorf("stat Unix socket endpoint: %w", err)
	}
	var socketStat syscall.Stat_t
	if err := syscall.Fstat(s.fd, &socketStat); err != nil {
		return RunLocalIdentity{}, fmt.Errorf("stat Unix listener FD: %w", err)
	}
	return RunLocalIdentity{
		EndpointDevice: uint64(endpointStat.Dev),
		EndpointInode:  uint64(endpointStat.Ino),
		SocketDevice:   uint64(socketStat.Dev),
		SocketInode:    uint64(socketStat.Ino),
		HolderPID:      os.Getpid(),
		HolderFD:       s.fd,
	}, nil
}

func (s *unixSocketServer) Close() error {
	var result error
	s.closeOnce.Do(func() {
		close(s.closed)
		result = syscall.Close(s.fd)
		s.wg.Wait()
	})
	return result
}

func writeSocketAll(fd int, contents []byte) error {
	for len(contents) > 0 {
		written, err := syscall.Write(fd, contents)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrUnexpectedEOF
		}
		contents = contents[written:]
	}
	return nil
}
