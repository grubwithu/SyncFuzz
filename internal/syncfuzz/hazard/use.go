package hazard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const RecoveryUsePlanSchemaVersion = "syncfuzz.recovery-use-plan.v1"
const UnixSocketUseEvidenceSchemaVersion = "syncfuzz.unix-socket-use-evidence.v1"

type RecoveryUseOperation string

const RecoveryUseOperationUnixSocketRoundTrip RecoveryUseOperation = "unix-socket-connect-roundtrip"

func (o RecoveryUseOperation) Valid() bool {
	return o == RecoveryUseOperationUnixSocketRoundTrip
}

// RecoveryUsePlan is derived from a frozen normal workload and a declared
// EnvironmentProgram. It names only a logical resource and a normal request
// (or, for a privacy-preserving target observation, that request's digest).
// Expected listener roles belong to the control oracle, never to this plan.
type RecoveryUsePlan struct {
	SchemaVersion        string               `json:"schema_version"`
	RecoveryUsePlanID    string               `json:"recovery_use_plan_id"`
	WorkloadID           string               `json:"workload_id"`
	EnvironmentProgramID string               `json:"environment_program_id"`
	LogicalName          string               `json:"logical_name"`
	Operation            RecoveryUseOperation `json:"operation"`
	// Request is retained only when SyncFuzz itself executes a local fixture
	// client. Target adapters must not persist application payloads: they build
	// a digest-only plan from their completed-exchange observation instead.
	Request       string `json:"request,omitempty"`
	RequestSHA256 string `json:"request_sha256"`
}

// NewUnixSocketRecoveryUsePlanFromObservedDigest builds the target-side form
// of a use plan. The normal request bytes remain inside the target process;
// the report binds controls through the observer's SHA-256 record only.
func NewUnixSocketRecoveryUsePlanFromObservedDigest(workload Workload, program environment.EnvironmentProgram, requestSHA256 string) (RecoveryUsePlan, error) {
	if err := workload.Validate(); err != nil {
		return RecoveryUsePlan{}, err
	}
	if err := program.Validate(); err != nil {
		return RecoveryUsePlan{}, err
	}
	requestSHA256 = strings.TrimSpace(requestSHA256)
	if !validSHA256(requestSHA256) {
		return RecoveryUsePlan{}, fmt.Errorf("Unix socket observed request digest is invalid")
	}
	plan := RecoveryUsePlan{
		SchemaVersion:        RecoveryUsePlanSchemaVersion,
		WorkloadID:           workload.WorkloadID,
		EnvironmentProgramID: program.ProgramID,
		LogicalName:          program.UnixSocket.LogicalName,
		Operation:            RecoveryUseOperationUnixSocketRoundTrip,
		RequestSHA256:        requestSHA256,
	}
	plan.RecoveryUsePlanID = recoveryUsePlanID(plan)
	if err := plan.ValidateFor(workload, program); err != nil {
		return RecoveryUsePlan{}, err
	}
	return plan, nil
}

func NewUnixSocketRecoveryUsePlan(workload Workload, program environment.EnvironmentProgram, request string) (RecoveryUsePlan, error) {
	if err := workload.Validate(); err != nil {
		return RecoveryUsePlan{}, err
	}
	if err := program.Validate(); err != nil {
		return RecoveryUsePlan{}, err
	}
	request = strings.TrimSpace(request)
	if request == "" {
		return RecoveryUsePlan{}, fmt.Errorf("Unix socket recovery use request is required")
	}
	plan := RecoveryUsePlan{
		SchemaVersion:        RecoveryUsePlanSchemaVersion,
		WorkloadID:           workload.WorkloadID,
		EnvironmentProgramID: program.ProgramID,
		LogicalName:          program.UnixSocket.LogicalName,
		Operation:            RecoveryUseOperationUnixSocketRoundTrip,
		Request:              request + "\n",
	}
	plan.RequestSHA256 = digest(plan.Request)
	plan.RecoveryUsePlanID = recoveryUsePlanID(plan)
	if err := plan.ValidateFor(workload, program); err != nil {
		return RecoveryUsePlan{}, err
	}
	return plan, nil
}

func (p RecoveryUsePlan) ValidateFor(workload Workload, program environment.EnvironmentProgram) error {
	if err := workload.Validate(); err != nil {
		return err
	}
	if err := program.Validate(); err != nil {
		return err
	}
	if p.SchemaVersion != RecoveryUsePlanSchemaVersion || p.WorkloadID != workload.WorkloadID || p.EnvironmentProgramID != program.ProgramID || p.LogicalName != program.UnixSocket.LogicalName || !p.Operation.Valid() || !validSHA256(p.RequestSHA256) || (p.Request != "" && p.RequestSHA256 != digest(p.Request)) || p.RecoveryUsePlanID != recoveryUsePlanID(p) {
		return fmt.Errorf("recovery use plan does not bind one workload and Unix socket environment program")
	}
	return nil
}

func recoveryUsePlanID(plan RecoveryUsePlan) string {
	identity := struct {
		WorkloadID           string               `json:"workload_id"`
		EnvironmentProgramID string               `json:"environment_program_id"`
		LogicalName          string               `json:"logical_name"`
		Operation            RecoveryUseOperation `json:"operation"`
		RequestSHA256        string               `json:"request_sha256"`
	}{
		WorkloadID:           plan.WorkloadID,
		EnvironmentProgramID: plan.EnvironmentProgramID,
		LogicalName:          plan.LogicalName,
		Operation:            plan.Operation,
		RequestSHA256:        plan.RequestSHA256,
	}
	encoded, _ := json.Marshal(identity)
	return "recovery-use-plan:" + digest(string(encoded))
}

type UseEvidenceMode string

const (
	// UseEvidenceModeFixtureRoundTrip is a local calibration observation. It
	// proves real client/server I/O inside the fixture but is not a claim that
	// recovery-time eBPF traced a production target.
	UseEvidenceModeFixtureRoundTrip UseEvidenceMode = "fixture-roundtrip"
	// UseEvidenceModeEBPFResolved is reserved for the future cgroup-scoped
	// `connect -> listener role -> I/O` target collector.
	UseEvidenceModeEBPFResolved UseEvidenceMode = "ebpf-resolved"
)

func (m UseEvidenceMode) Valid() bool {
	return m == UseEvidenceModeFixtureRoundTrip || m == UseEvidenceModeEBPFResolved
}

// UnixSocketUseEvidence records the resource-family-specific resolve/use
// chain. It intentionally does not reuse RecoveryObservation.Evidence strings.
type UnixSocketUseEvidence struct {
	SchemaVersion        string                       `json:"schema_version"`
	EvidenceMode         UseEvidenceMode              `json:"evidence_mode"`
	RecoveryUsePlanID    string                       `json:"recovery_use_plan_id"`
	LogicalName          string                       `json:"logical_name"`
	ResolvedEndpointPath string                       `json:"resolved_endpoint_path"`
	ResolutionSteps      []environment.ResolutionStep `json:"resolution_steps"`
	ConnectObserved      bool                         `json:"connect_observed"`
	IOObserved           bool                         `json:"io_observed"`
	RequestSHA256        string                       `json:"request_sha256"`
	ResponseSHA256       string                       `json:"response_sha256"`
	ListenerRole         string                       `json:"listener_role"`
	ListenerSemantic     environment.SemanticIdentity `json:"listener_semantic"`
	ListenerLocal        environment.RunLocalIdentity `json:"listener_local"`
}

func (e UnixSocketUseEvidence) ValidateFor(workload Workload, plan RecoveryUsePlan, program environment.EnvironmentProgram) error {
	if err := plan.ValidateFor(workload, program); err != nil {
		return err
	}
	return validateUseEvidenceForPlan(e, plan, program)
}

// ExecuteUnixSocketUse performs the fixed normal recovery use against a
// materialized Unix socket program. It reads the materializer's role-tagged
// calibration response only to identify which declared listener served I/O.
func ExecuteUnixSocketUse(ctx context.Context, materialization *environment.UnixSocketMaterialization, plan RecoveryUsePlan) (UnixSocketUseEvidence, error) {
	if materialization == nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("Unix socket materialization is required")
	}
	program := materialization.Program()
	if plan.SchemaVersion != RecoveryUsePlanSchemaVersion || plan.EnvironmentProgramID != program.ProgramID || plan.LogicalName != program.UnixSocket.LogicalName || !plan.Operation.Valid() || strings.TrimSpace(plan.Request) == "" || plan.RequestSHA256 != digest(plan.Request) || plan.RecoveryUsePlanID != recoveryUsePlanID(plan) {
		return UnixSocketUseEvidence{}, fmt.Errorf("recovery use plan does not bind materialized Unix socket program")
	}
	resolved, err := materialization.ResolveLogicalName(ctx, plan.LogicalName)
	if err != nil {
		return UnixSocketUseEvidence{}, err
	}
	if err := ctx.Err(); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("create Unix socket recovery client: %w", err)
	}
	defer syscall.Close(fd)
	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: resolved.AbsoluteEndpointPath()}); err != nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("connect resolved Unix socket endpoint: %w", err)
	}
	if err := writeAll(fd, []byte(plan.Request)); err != nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("write Unix socket recovery use request: %w", err)
	}
	responseBytes := make([]byte, 4096)
	count, err := syscall.Read(fd, responseBytes)
	if err != nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("read Unix socket recovery use response: %w", err)
	}
	if count == 0 {
		return UnixSocketUseEvidence{}, fmt.Errorf("read Unix socket recovery use response: peer closed without a response")
	}
	responseLine := string(responseBytes[:count])
	response := struct {
		Role          string `json:"role"`
		RequestSHA256 string `json:"request_sha256"`
	}{}
	if err := json.Unmarshal([]byte(responseLine), &response); err != nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("parse Unix socket recovery use response: %w", err)
	}
	if response.RequestSHA256 != plan.RequestSHA256 || !validRole(response.Role) {
		return UnixSocketUseEvidence{}, fmt.Errorf("Unix socket recovery use response did not bind the normal request")
	}
	active := materialization.ActiveBinding()
	if response.Role != active.Semantic.Role {
		return UnixSocketUseEvidence{}, fmt.Errorf("Unix socket response role %q does not match active binding role %q", response.Role, active.Semantic.Role)
	}
	evidence := UnixSocketUseEvidence{
		SchemaVersion:        UnixSocketUseEvidenceSchemaVersion,
		EvidenceMode:         UseEvidenceModeFixtureRoundTrip,
		RecoveryUsePlanID:    plan.RecoveryUsePlanID,
		LogicalName:          plan.LogicalName,
		ResolvedEndpointPath: resolved.EndpointPath,
		ResolutionSteps:      append([]environment.ResolutionStep(nil), resolved.ResolutionSteps...),
		ConnectObserved:      true,
		IOObserved:           true,
		RequestSHA256:        plan.RequestSHA256,
		ResponseSHA256:       digest(responseLine),
		ListenerRole:         response.Role,
		ListenerSemantic:     active.Semantic,
		ListenerLocal:        active.Local,
	}
	// The plan's Workload cannot be reconstructed from IDs alone, so this
	// function validates all binding invariants locally rather than invoking
	// RecoveryUsePlan.ValidateFor again.
	if err := validateUseEvidenceForPlan(evidence, plan, program); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	return evidence, nil
}

// NewUnixSocketUseEvidenceFromTargetRecovery converts the payload-free
// recovery-cgroup evidence into the same typed U' record used by the hazard
// layer. It never derives a result from continuation prose: the caller must
// supply a completed exchange whose request digest matches the frozen use plan.
func NewUnixSocketUseEvidenceFromTargetRecovery(workload Workload, plan RecoveryUsePlan, program environment.EnvironmentProgram, materialization environment.TargetUnixSocketMaterialization, observed recovery.EnvironmentUseEvidence) (UnixSocketUseEvidence, error) {
	if err := plan.ValidateFor(workload, program); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	if err := materialization.ValidateFor(program); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	if err := observed.Validate(); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	expectedEndpoint := "/workspace/" + program.UnixSocket.EndpointPath
	active := materialization.ActiveBinding()
	if observed.Family != "unix-socket" || observed.ProgramID != program.ProgramID || observed.LogicalName != program.UnixSocket.LogicalName || observed.ResolvedEndpointPath != expectedEndpoint || observed.RequestSHA256 != plan.RequestSHA256 || !observed.CompletedExchange || observed.ListenerRole != active.Semantic.Role || observed.ListenerPID != active.Local.HolderPID || observed.ListenerFD != active.Local.HolderFD || observed.ListenerSocketID != materialization.ActiveListener.SocketID || observed.ListenerEndpointDevice != active.Local.EndpointDevice || observed.ListenerEndpointInode != active.Local.EndpointInode || observed.ListenerSocketDevice != active.Local.SocketDevice || observed.ListenerSocketInode != active.Local.SocketInode {
		return UnixSocketUseEvidence{}, fmt.Errorf("target recovery use evidence does not match the approved active environment binding")
	}
	evidence := UnixSocketUseEvidence{
		SchemaVersion:        UnixSocketUseEvidenceSchemaVersion,
		EvidenceMode:         UseEvidenceModeEBPFResolved,
		RecoveryUsePlanID:    plan.RecoveryUsePlanID,
		LogicalName:          plan.LogicalName,
		ResolvedEndpointPath: program.UnixSocket.EndpointPath,
		ResolutionSteps:      append([]environment.ResolutionStep(nil), materialization.ResolutionSteps...),
		ConnectObserved:      true,
		IOObserved:           true,
		RequestSHA256:        observed.RequestSHA256,
		// The target observer intentionally does not retain a response payload.
		// This digest identifies the completed acknowledgement relation instead.
		ResponseSHA256:   digest("acknowledged-response\x00" + observed.ListenerRole + "\x00" + observed.RequestSHA256),
		ListenerRole:     observed.ListenerRole,
		ListenerSemantic: active.Semantic,
		ListenerLocal:    active.Local,
	}
	if err := evidence.ValidateFor(workload, plan, program); err != nil {
		return UnixSocketUseEvidence{}, err
	}
	return evidence, nil
}

func writeAll(fd int, contents []byte) error {
	for len(contents) > 0 {
		written, err := syscall.Write(fd, contents)
		if err != nil {
			return err
		}
		if written == 0 {
			return fmt.Errorf("Unix socket write made no progress")
		}
		contents = contents[written:]
	}
	return nil
}

func validateUseEvidenceForPlan(evidence UnixSocketUseEvidence, plan RecoveryUsePlan, program environment.EnvironmentProgram) error {
	if evidence.SchemaVersion != UnixSocketUseEvidenceSchemaVersion || !evidence.EvidenceMode.Valid() || evidence.RecoveryUsePlanID != plan.RecoveryUsePlanID || evidence.LogicalName != plan.LogicalName || evidence.ResolvedEndpointPath != program.UnixSocket.EndpointPath || len(evidence.ResolutionSteps) < 2 || !evidence.ConnectObserved || !evidence.IOObserved || evidence.RequestSHA256 != plan.RequestSHA256 || len(evidence.ResponseSHA256) != 64 || !validRole(evidence.ListenerRole) {
		return fmt.Errorf("Unix socket use evidence is incomplete")
	}
	for _, step := range evidence.ResolutionSteps {
		if err := step.Validate(); err != nil {
			return err
		}
	}
	if err := evidence.ListenerSemantic.Validate(); err != nil {
		return err
	}
	if err := evidence.ListenerLocal.Validate(); err != nil {
		return err
	}
	if evidence.ListenerSemantic.ProgramID != program.ProgramID || evidence.ListenerSemantic.LogicalName != program.UnixSocket.LogicalName || evidence.ListenerSemantic.Role != evidence.ListenerRole {
		return fmt.Errorf("Unix socket use evidence listener identity does not match its environment program")
	}
	return nil
}

func validRole(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return false
	}
	return true
}

func digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
