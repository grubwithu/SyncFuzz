package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
)

// EnvironmentProgramSchemaVersion is deliberately independent from the
// execution-backend schema. An EnvironmentProgram describes the resource
// binding topology SyncFuzz intends to materialize; it is not a probe result.
const EnvironmentProgramSchemaVersion = "syncfuzz.environment-program.v1"

// EnvironmentResourceFamily bounds the first V3 materialization grammar.
// Additional families must get their own typed resolver and identity adapter.
type EnvironmentResourceFamily string

const EnvironmentResourceFamilyUnixSocket EnvironmentResourceFamily = "unix-socket"

func (f EnvironmentResourceFamily) Valid() bool {
	return f == EnvironmentResourceFamilyUnixSocket
}

// UnixSocketResolutionMode records the declared logical-name resolution path.
// It is an EnvironmentProgram property rather than an inferred probe result.
type UnixSocketResolutionMode string

const (
	UnixSocketResolutionDirect      UnixSocketResolutionMode = "direct"
	UnixSocketResolutionConfig      UnixSocketResolutionMode = "config"
	UnixSocketResolutionEnvironment UnixSocketResolutionMode = "environment"
	UnixSocketResolutionAlias       UnixSocketResolutionMode = "alias"
)

func (m UnixSocketResolutionMode) Valid() bool {
	switch m {
	case UnixSocketResolutionDirect, UnixSocketResolutionConfig, UnixSocketResolutionEnvironment, UnixSocketResolutionAlias:
		return true
	default:
		return false
	}
}

// HolderLifetime is part of the mutation vocabulary. The first local Unix
// socket materializer can faithfully materialize foreground holders only; it
// rejects the other values instead of silently claiming their semantics.
type HolderLifetime string

const (
	HolderLifetimeForeground HolderLifetime = "foreground"
	HolderLifetimeChild      HolderLifetime = "child"
	HolderLifetimeDetached   HolderLifetime = "detached"
)

func (l HolderLifetime) Valid() bool {
	switch l {
	case HolderLifetimeForeground, HolderLifetimeChild, HolderLifetimeDetached:
		return true
	default:
		return false
	}
}

// MutationOperator describes one auditable topology change from a parent
// program. It never encodes a prompt, an expected finding, or an exploit
// consequence.
type MutationOperator string

const (
	MutationOperatorBaseline            MutationOperator = "baseline"
	MutationOperatorIncreaseIndirection MutationOperator = "increase-indirection"
	MutationOperatorAddAlias            MutationOperator = "add-alias"
	MutationOperatorRebind              MutationOperator = "rebind"
	MutationOperatorShiftHolderLifetime MutationOperator = "shift-holder-lifetime"
)

func (o MutationOperator) Valid() bool {
	switch o {
	case MutationOperatorBaseline, MutationOperatorIncreaseIndirection, MutationOperatorAddAlias, MutationOperatorRebind, MutationOperatorShiftHolderLifetime:
		return true
	default:
		return false
	}
}

type EnvironmentMutation struct {
	Operator        MutationOperator `json:"operator"`
	ParentProgramID string           `json:"parent_program_id,omitempty"`
}

// EnvironmentNodeKind and EnvironmentProgramEdge make the planned binding
// graph explicit. They are intentionally smaller than profiling.ResourceRef:
// nodes are declarative topology, not observed host identities.
type EnvironmentNodeKind string

const (
	EnvironmentNodeLogicalName EnvironmentNodeKind = "logical-name"
	EnvironmentNodeConfigKey   EnvironmentNodeKind = "config-key"
	EnvironmentNodeEnvVar      EnvironmentNodeKind = "environment-variable"
	EnvironmentNodeAlias       EnvironmentNodeKind = "alias"
	EnvironmentNodePath        EnvironmentNodeKind = "pathname"
	EnvironmentNodeEndpoint    EnvironmentNodeKind = "unix-endpoint"
	EnvironmentNodeCapability  EnvironmentNodeKind = "listening-capability"
	EnvironmentNodeHolder      EnvironmentNodeKind = "holder-process"
)

func (k EnvironmentNodeKind) Valid() bool {
	switch k {
	case EnvironmentNodeLogicalName, EnvironmentNodeConfigKey, EnvironmentNodeEnvVar, EnvironmentNodeAlias, EnvironmentNodePath, EnvironmentNodeEndpoint, EnvironmentNodeCapability, EnvironmentNodeHolder:
		return true
	default:
		return false
	}
}

type EnvironmentProgramNode struct {
	NodeID       string              `json:"node_id"`
	Kind         EnvironmentNodeKind `json:"kind"`
	Value        string              `json:"value"`
	SemanticRole string              `json:"semantic_role,omitempty"`
}

type EnvironmentProgramEdge struct {
	FromNodeID string `json:"from_node_id"`
	ToNodeID   string `json:"to_node_id"`
	Relation   string `json:"relation"`
}

type ExpectedResourceTouch struct {
	LogicalName string `json:"logical_name"`
	Operation   string `json:"operation"`
}

// UnixSocketBinding is the typed first-family payload of EnvironmentProgram.
// EndpointPath and ResolutionArtifactPath are workspace-relative paths. The
// initial and active role distinguish a pre-rebind listener from the final
// binding without baking any recovery expectation into the program.
type UnixSocketBinding struct {
	LogicalName            string                   `json:"logical_name"`
	ResolutionMode         UnixSocketResolutionMode `json:"resolution_mode"`
	ResolutionKey          string                   `json:"resolution_key,omitempty"`
	ResolutionArtifactPath string                   `json:"resolution_artifact_path,omitempty"`
	EndpointPath           string                   `json:"endpoint_path"`
	InitialRole            string                   `json:"initial_role"`
	ActiveRole             string                   `json:"active_role"`
	HolderLifetime         HolderLifetime           `json:"holder_lifetime"`
}

// EnvironmentProgram is the immutable input-side topology. Nodes and edges
// are canonicalized from UnixSocket so serialized programs cannot silently
// change their declared graph while retaining an ID.
type EnvironmentProgram struct {
	SchemaVersion           string                    `json:"schema_version"`
	ProgramID               string                    `json:"program_id"`
	Family                  EnvironmentResourceFamily `json:"family"`
	Mutation                EnvironmentMutation       `json:"mutation"`
	UnixSocket              UnixSocketBinding         `json:"unix_socket"`
	Nodes                   []EnvironmentProgramNode  `json:"nodes"`
	Edges                   []EnvironmentProgramEdge  `json:"edges"`
	ExpectedResourceTouches []ExpectedResourceTouch   `json:"expected_resource_touches"`
}

type UnixSocketProgramOptions struct {
	ParentProgramID        string
	MutationOperator       MutationOperator
	LogicalName            string
	ResolutionMode         UnixSocketResolutionMode
	ResolutionKey          string
	ResolutionArtifactPath string
	EndpointPath           string
	InitialRole            string
	ActiveRole             string
	HolderLifetime         HolderLifetime
}

// NewUnixSocketProgram creates one canonical declarative program. Callers
// must name all behavioral fields explicitly so the fuzzer cannot smuggle a
// topology change in through defaults.
func NewUnixSocketProgram(options UnixSocketProgramOptions) (EnvironmentProgram, error) {
	operator := options.MutationOperator
	if operator == "" {
		operator = MutationOperatorBaseline
	}
	program := EnvironmentProgram{
		SchemaVersion: EnvironmentProgramSchemaVersion,
		Family:        EnvironmentResourceFamilyUnixSocket,
		Mutation: EnvironmentMutation{
			Operator:        operator,
			ParentProgramID: strings.TrimSpace(options.ParentProgramID),
		},
		UnixSocket: UnixSocketBinding{
			LogicalName:            strings.TrimSpace(options.LogicalName),
			ResolutionMode:         options.ResolutionMode,
			ResolutionKey:          strings.TrimSpace(options.ResolutionKey),
			ResolutionArtifactPath: strings.TrimSpace(options.ResolutionArtifactPath),
			EndpointPath:           strings.TrimSpace(options.EndpointPath),
			InitialRole:            strings.TrimSpace(options.InitialRole),
			ActiveRole:             strings.TrimSpace(options.ActiveRole),
			HolderLifetime:         options.HolderLifetime,
		},
		ExpectedResourceTouches: []ExpectedResourceTouch{{
			LogicalName: strings.TrimSpace(options.LogicalName),
			Operation:   "connect",
		}},
	}
	program.Nodes, program.Edges = canonicalUnixSocketTopology(program.UnixSocket)
	program.ProgramID = environmentProgramID(program)
	if err := program.Validate(); err != nil {
		return EnvironmentProgram{}, err
	}
	return program, nil
}

// UnixSocketMutation is the bounded mutator input for the first resource
// family. A resulting program always points back to its parent program ID.
type UnixSocketMutation struct {
	Operator               MutationOperator
	ResolutionMode         UnixSocketResolutionMode
	ResolutionKey          string
	ResolutionArtifactPath string
	ActiveRole             string
	HolderLifetime         HolderLifetime
}

func (p EnvironmentProgram) MutateUnixSocket(mutation UnixSocketMutation) (EnvironmentProgram, error) {
	if err := p.Validate(); err != nil {
		return EnvironmentProgram{}, fmt.Errorf("validate parent environment program: %w", err)
	}
	if p.Family != EnvironmentResourceFamilyUnixSocket {
		return EnvironmentProgram{}, fmt.Errorf("environment program %q is not a Unix socket program", p.ProgramID)
	}
	if !mutation.Operator.Valid() || mutation.Operator == MutationOperatorBaseline {
		return EnvironmentProgram{}, fmt.Errorf("Unix socket mutation requires a non-baseline operator")
	}

	next := p.UnixSocket
	switch mutation.Operator {
	case MutationOperatorIncreaseIndirection:
		if !mutation.ResolutionMode.Valid() || mutation.ResolutionMode == UnixSocketResolutionDirect {
			return EnvironmentProgram{}, fmt.Errorf("increase-indirection requires config, environment, or alias resolution")
		}
		next.ResolutionMode = mutation.ResolutionMode
		next.ResolutionKey = strings.TrimSpace(mutation.ResolutionKey)
		next.ResolutionArtifactPath = strings.TrimSpace(mutation.ResolutionArtifactPath)
		if next.ResolutionMode == p.UnixSocket.ResolutionMode && next.ResolutionKey == p.UnixSocket.ResolutionKey && next.ResolutionArtifactPath == p.UnixSocket.ResolutionArtifactPath {
			return EnvironmentProgram{}, fmt.Errorf("increase-indirection must change the parent resolution binding")
		}
	case MutationOperatorAddAlias:
		next.ResolutionMode = UnixSocketResolutionAlias
		next.ResolutionKey = strings.TrimSpace(mutation.ResolutionKey)
		next.ResolutionArtifactPath = strings.TrimSpace(mutation.ResolutionArtifactPath)
		if next.ResolutionMode == p.UnixSocket.ResolutionMode && next.ResolutionKey == p.UnixSocket.ResolutionKey && next.ResolutionArtifactPath == p.UnixSocket.ResolutionArtifactPath {
			return EnvironmentProgram{}, fmt.Errorf("add-alias must change the parent resolution binding")
		}
	case MutationOperatorRebind:
		next.ActiveRole = strings.TrimSpace(mutation.ActiveRole)
		if next.ActiveRole == p.UnixSocket.ActiveRole {
			return EnvironmentProgram{}, fmt.Errorf("rebind must change the parent active listener role")
		}
	case MutationOperatorShiftHolderLifetime:
		next.HolderLifetime = mutation.HolderLifetime
		if next.HolderLifetime == p.UnixSocket.HolderLifetime {
			return EnvironmentProgram{}, fmt.Errorf("shift-holder-lifetime must change the parent holder lifetime")
		}
	default:
		return EnvironmentProgram{}, fmt.Errorf("unsupported Unix socket mutation operator %q", mutation.Operator)
	}
	return NewUnixSocketProgram(UnixSocketProgramOptions{
		ParentProgramID:        p.ProgramID,
		MutationOperator:       mutation.Operator,
		LogicalName:            next.LogicalName,
		ResolutionMode:         next.ResolutionMode,
		ResolutionKey:          next.ResolutionKey,
		ResolutionArtifactPath: next.ResolutionArtifactPath,
		EndpointPath:           next.EndpointPath,
		InitialRole:            next.InitialRole,
		ActiveRole:             next.ActiveRole,
		HolderLifetime:         next.HolderLifetime,
	})
}

func (p EnvironmentProgram) Validate() error {
	if p.SchemaVersion != EnvironmentProgramSchemaVersion || !p.Family.Valid() || !p.Mutation.Operator.Valid() {
		return fmt.Errorf("environment program has an unsupported schema, family, or mutation operator")
	}
	if p.Mutation.Operator == MutationOperatorBaseline {
		if p.Mutation.ParentProgramID != "" {
			return fmt.Errorf("baseline environment program must not name a parent")
		}
	} else if strings.TrimSpace(p.Mutation.ParentProgramID) == "" {
		return fmt.Errorf("mutated environment program requires a parent_program_id")
	}
	if p.Family != EnvironmentResourceFamilyUnixSocket {
		return fmt.Errorf("environment program family %q is not implemented", p.Family)
	}
	if err := p.UnixSocket.ValidateFor(p.Mutation.Operator); err != nil {
		return err
	}
	switch p.Mutation.Operator {
	case MutationOperatorIncreaseIndirection:
		if p.UnixSocket.ResolutionMode == UnixSocketResolutionDirect {
			return fmt.Errorf("increase-indirection mutation must use an indirect resolution mode")
		}
	case MutationOperatorAddAlias:
		if p.UnixSocket.ResolutionMode != UnixSocketResolutionAlias {
			return fmt.Errorf("add-alias mutation must use alias resolution")
		}
	}
	if len(p.ExpectedResourceTouches) != 1 || p.ExpectedResourceTouches[0].LogicalName != p.UnixSocket.LogicalName || p.ExpectedResourceTouches[0].Operation != "connect" {
		return fmt.Errorf("Unix socket environment program must declare exactly one normal connect touch for its logical name")
	}
	expectedNodes, expectedEdges := canonicalUnixSocketTopology(p.UnixSocket)
	if !reflect.DeepEqual(p.Nodes, expectedNodes) || !reflect.DeepEqual(p.Edges, expectedEdges) {
		return fmt.Errorf("environment program topology is not the canonical Unix socket binding graph")
	}
	if p.ProgramID != environmentProgramID(p) {
		return fmt.Errorf("environment program ID does not match its immutable topology")
	}
	return nil
}

func (b UnixSocketBinding) ValidateFor(operator MutationOperator) error {
	if !validTopologyToken(b.LogicalName) || !b.ResolutionMode.Valid() || !validRelativeWorkspacePath(b.EndpointPath) || !validTopologyToken(b.InitialRole) || !validTopologyToken(b.ActiveRole) || !b.HolderLifetime.Valid() {
		return fmt.Errorf("Unix socket environment binding is incomplete or invalid")
	}
	switch b.ResolutionMode {
	case UnixSocketResolutionDirect:
		if b.ResolutionKey != "" || b.ResolutionArtifactPath != "" {
			return fmt.Errorf("direct Unix socket resolution must not declare a key or artifact")
		}
	case UnixSocketResolutionConfig, UnixSocketResolutionAlias:
		if !validTopologyToken(b.ResolutionKey) || !validRelativeWorkspacePath(b.ResolutionArtifactPath) {
			return fmt.Errorf("%s Unix socket resolution requires a safe key and workspace-relative artifact", b.ResolutionMode)
		}
	case UnixSocketResolutionEnvironment:
		if !validTopologyToken(b.ResolutionKey) || b.ResolutionArtifactPath != "" {
			return fmt.Errorf("environment Unix socket resolution requires a safe variable name and no artifact")
		}
	}
	if operator == MutationOperatorRebind && b.InitialRole == b.ActiveRole {
		return fmt.Errorf("rebind mutation must change the active listener role")
	}
	return nil
}

func canonicalUnixSocketTopology(binding UnixSocketBinding) ([]EnvironmentProgramNode, []EnvironmentProgramEdge) {
	logicalID := "logical:" + binding.LogicalName
	pathID := "path:" + binding.EndpointPath
	endpointID := "endpoint:" + binding.EndpointPath
	capabilityID := "capability:" + binding.LogicalName
	holderID := "holder:" + binding.LogicalName
	nodes := []EnvironmentProgramNode{{NodeID: logicalID, Kind: EnvironmentNodeLogicalName, Value: binding.LogicalName}}
	edges := make([]EnvironmentProgramEdge, 0, 5)
	if binding.ResolutionMode == UnixSocketResolutionDirect {
		edges = append(edges, EnvironmentProgramEdge{FromNodeID: logicalID, ToNodeID: pathID, Relation: "resolves-to"})
	} else {
		kind := EnvironmentNodeConfigKey
		if binding.ResolutionMode == UnixSocketResolutionEnvironment {
			kind = EnvironmentNodeEnvVar
		} else if binding.ResolutionMode == UnixSocketResolutionAlias {
			kind = EnvironmentNodeAlias
		}
		resolutionID := "resolution:" + string(binding.ResolutionMode) + ":" + binding.ResolutionKey
		value := binding.ResolutionKey
		if binding.ResolutionArtifactPath != "" {
			value += "@" + binding.ResolutionArtifactPath
		}
		nodes = append(nodes, EnvironmentProgramNode{NodeID: resolutionID, Kind: kind, Value: value})
		edges = append(edges,
			EnvironmentProgramEdge{FromNodeID: logicalID, ToNodeID: resolutionID, Relation: "resolves-via"},
			EnvironmentProgramEdge{FromNodeID: resolutionID, ToNodeID: pathID, Relation: "resolves-to"},
		)
	}
	nodes = append(nodes,
		EnvironmentProgramNode{NodeID: pathID, Kind: EnvironmentNodePath, Value: binding.EndpointPath},
		EnvironmentProgramNode{NodeID: endpointID, Kind: EnvironmentNodeEndpoint, Value: binding.EndpointPath, SemanticRole: binding.ActiveRole},
		EnvironmentProgramNode{NodeID: capabilityID, Kind: EnvironmentNodeCapability, Value: "unix-listener"},
		EnvironmentProgramNode{NodeID: holderID, Kind: EnvironmentNodeHolder, Value: string(binding.HolderLifetime), SemanticRole: binding.ActiveRole},
	)
	edges = append(edges,
		EnvironmentProgramEdge{FromNodeID: pathID, ToNodeID: endpointID, Relation: "names"},
		EnvironmentProgramEdge{FromNodeID: endpointID, ToNodeID: capabilityID, Relation: "bound-as"},
		EnvironmentProgramEdge{FromNodeID: capabilityID, ToNodeID: holderID, Relation: "held-by"},
	)
	return nodes, edges
}

func environmentProgramID(program EnvironmentProgram) string {
	identity := struct {
		Family     EnvironmentResourceFamily `json:"family"`
		Mutation   EnvironmentMutation       `json:"mutation"`
		UnixSocket UnixSocketBinding         `json:"unix_socket"`
	}{
		Family:     program.Family,
		Mutation:   program.Mutation,
		UnixSocket: program.UnixSocket,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return "environment-program:" + hex.EncodeToString(digest[:])
}

func validRelativeWorkspacePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}
	return cleaned == value
}

func validTopologyToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
