package target

import (
	"fmt"
	"sort"
	"strings"
)

const TargetViolationSignatureSchemaVersion = "syncfuzz.target-violation-signature.v1"

// TargetViolationRelation describes the recovery-consistency relation under
// test. It is a query classification, not a verdict about any individual run.
type TargetViolationRelation string

const (
	TargetViolationResidual         TargetViolationRelation = "residual"
	TargetViolationMissing          TargetViolationRelation = "missing"
	TargetViolationDuplicated       TargetViolationRelation = "duplicated"
	TargetViolationReorderedDelayed TargetViolationRelation = "reordered-delayed"
	TargetViolationRebound          TargetViolationRelation = "rebound"
)

// TargetViolationResourceClass is the OS-facing taxonomy used to classify a
// query independently of its concrete file name or tool prompt.
type TargetViolationResourceClass string

const (
	TargetViolationNamespaceObject    TargetViolationResourceClass = "namespace-object"
	TargetViolationExecutionContext   TargetViolationResourceClass = "execution-context"
	TargetViolationProcess            TargetViolationResourceClass = "process"
	TargetViolationHandleCapability   TargetViolationResourceClass = "handle-capability"
	TargetViolationIPCEndpoint        TargetViolationResourceClass = "ipc-endpoint"
	TargetViolationPermissionMetadata TargetViolationResourceClass = "permission-metadata"
	TargetViolationExternalEffect     TargetViolationResourceClass = "external-effect"
	TargetViolationAuthorityState     TargetViolationResourceClass = "authority-state"
)

type TargetViolationLifecycleBoundary string

const (
	TargetViolationBoundaryContinuation      TargetViolationLifecycleBoundary = "continuation"
	TargetViolationBoundaryCommandReturn     TargetViolationLifecycleBoundary = "command-return"
	TargetViolationBoundaryCheckpointFork    TargetViolationLifecycleBoundary = "checkpoint-fork"
	TargetViolationBoundaryCheckpointReplay  TargetViolationLifecycleBoundary = "checkpoint-replay"
	TargetViolationBoundaryCheckpointRestore TargetViolationLifecycleBoundary = "checkpoint-restore"
	TargetViolationBoundarySessionRestore    TargetViolationLifecycleBoundary = "session-restore"
)

type TargetViolationPersistenceMechanism string

const (
	TargetViolationSharedWorkspace    TargetViolationPersistenceMechanism = "shared-workspace"
	TargetViolationPersistentShell    TargetViolationPersistenceMechanism = "persistent-shell"
	TargetViolationDescendantSurvival TargetViolationPersistenceMechanism = "descendant-survival"
	TargetViolationFDInheritance      TargetViolationPersistenceMechanism = "fd-inheritance"
	TargetViolationNamespaceRebinding TargetViolationPersistenceMechanism = "namespace-rebinding"
	TargetViolationDelayedExecution   TargetViolationPersistenceMechanism = "delayed-execution"
	TargetViolationMissingCleanup     TargetViolationPersistenceMechanism = "missing-cleanup"
	TargetViolationSharedRuntime      TargetViolationPersistenceMechanism = "shared-runtime"
	TargetViolationCheckpointRestore  TargetViolationPersistenceMechanism = "checkpoint-restore"
	TargetViolationRuntimeReexecution TargetViolationPersistenceMechanism = "runtime-reexecution"
)

type TargetViolationConsequence string

const (
	TargetViolationObservation             TargetViolationConsequence = "observation"
	TargetViolationCrossBranchInterference TargetViolationConsequence = "cross-branch-interference"
	TargetViolationServiceImpersonation    TargetViolationConsequence = "service-impersonation"
	TargetViolationSecretCapture           TargetViolationConsequence = "secret-capture"
	TargetViolationStateCorruption         TargetViolationConsequence = "state-corruption"
	TargetViolationDuplicateOperation      TargetViolationConsequence = "duplicate-operation"
	TargetViolationAuthorityConflict       TargetViolationConsequence = "authority-conflict"
)

// TargetViolationSignature implements the five-dimensional FSE-facing
// taxonomy: mismatch relation, OS resource class, lifecycle boundary,
// persistence mechanism, and activated consequence. A scenario can contain
// multiple relations or mechanisms when one query intentionally composes them.
// It labels a test intent and must never be read as a run-level oracle verdict.
type TargetViolationSignature struct {
	SchemaVersion         string                                `json:"schema_version"`
	SignatureID           string                                `json:"signature_id"`
	Relations             []TargetViolationRelation             `json:"relations"`
	ResourceClasses       []TargetViolationResourceClass        `json:"resource_classes"`
	LifecycleBoundary     TargetViolationLifecycleBoundary      `json:"lifecycle_boundary"`
	PersistenceMechanisms []TargetViolationPersistenceMechanism `json:"persistence_mechanisms"`
	Consequences          []TargetViolationConsequence          `json:"consequences"`
}

// TargetViolationSignatureInfo gives the CLI and experiment artifacts an
// auditable mapping from a concrete executable query to its taxonomy label.
type TargetViolationSignatureInfo struct {
	ScenarioID string                   `json:"scenario_id"`
	SeedID     string                   `json:"seed_id,omitempty"`
	TaskID     string                   `json:"task_id"`
	Signature  TargetViolationSignature `json:"violation_signature"`
}

// DeriveTargetViolationSignature derives a stable taxonomy label from the
// executable Scenario IR metadata. The derivation is intentionally local and
// deterministic, so generated mutations are reclassified after their plant,
// boundary, or activation metadata changes.
func DeriveTargetViolationSignature(info *TargetScenarioInfo) TargetViolationSignature {
	primitive := targetViolationNormalizedField(info, func(value *TargetScenarioInfo) string { return value.PlantPrimitiveID })
	activation := targetViolationNormalizedField(info, func(value *TargetScenarioInfo) string { return value.ActivationKindID })
	oracle := targetViolationNormalizedField(info, func(value *TargetScenarioInfo) string { return value.OracleKindID })
	stateSurface := targetViolationNormalizedField(info, func(value *TargetScenarioInfo) string { return value.StateSurface })
	lifecycle := targetViolationLifecycleValue(info)

	signature := TargetViolationSignature{
		SchemaVersion:     TargetViolationSignatureSchemaVersion,
		LifecycleBoundary: targetViolationBoundaryFor(lifecycle),
		Relations:         []TargetViolationRelation{TargetViolationResidual},
		ResourceClasses:   []TargetViolationResourceClass{TargetViolationNamespaceObject},
		PersistenceMechanisms: []TargetViolationPersistenceMechanism{
			TargetViolationSharedWorkspace,
		},
		Consequences: []TargetViolationConsequence{TargetViolationObservation},
	}

	switch {
	case targetViolationContainsAny(primitive, "authority-token") || targetViolationContainsAny(stateSurface, "authority."):
		signature.Relations = []TargetViolationRelation{TargetViolationMissing}
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationAuthorityState}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationCheckpointRestore, TargetViolationRuntimeReexecution}
		signature.Consequences = []TargetViolationConsequence{TargetViolationAuthorityConflict}
	case targetViolationContainsAny(primitive, "external-effect", "http-effect", "resource-effect", "external-effect-before-failure") || targetViolationContainsAny(stateSurface, "external."):
		signature.Relations = []TargetViolationRelation{TargetViolationDuplicated}
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationExternalEffect}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationCheckpointRestore, TargetViolationRuntimeReexecution}
		signature.Consequences = []TargetViolationConsequence{TargetViolationDuplicateOperation}
	case targetViolationContainsAny(primitive, "background-process"):
		signature.Relations = []TargetViolationRelation{TargetViolationReorderedDelayed}
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationProcess}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationDelayedExecution, TargetViolationDescendantSurvival}
		signature.Consequences = []TargetViolationConsequence{TargetViolationObservation}
	case targetViolationContainsAny(primitive, "open-fd", "inherited-fd"):
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationHandleCapability}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationFDInheritance, TargetViolationMissingCleanup}
		if targetViolationContainsAny(activation, "secret-read", "trusted-action") {
			signature.Consequences = []TargetViolationConsequence{TargetViolationSecretCapture}
		}
	case targetViolationContainsAny(primitive, "unix-listener"):
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationIPCEndpoint}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationDescendantSurvival, TargetViolationMissingCleanup}
		signature.Consequences = []TargetViolationConsequence{TargetViolationObservation}
		if targetViolationContainsAny(activation, "trusted-client", "trusted-action") {
			signature.Consequences = []TargetViolationConsequence{TargetViolationCrossBranchInterference, TargetViolationStateCorruption}
		}
		if targetViolationContainsAny(activation, "cache") || targetViolationContainsAny(oracle, "response-poisoning") {
			signature.Relations = []TargetViolationRelation{TargetViolationResidual, TargetViolationRebound}
			signature.PersistenceMechanisms = append(signature.PersistenceMechanisms, TargetViolationNamespaceRebinding)
			signature.Consequences = []TargetViolationConsequence{TargetViolationStateCorruption}
		}
	case targetViolationContainsAny(primitive, "shell-") || targetViolationContainsAny(stateSurface, "shell-session"):
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationExecutionContext}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationPersistentShell}
	case targetViolationContainsAny(primitive, "file-chmod"):
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationPermissionMetadata}
	case targetViolationContainsAny(primitive, "workflow-"):
		signature.ResourceClasses = []TargetViolationResourceClass{TargetViolationNamespaceObject}
		signature.PersistenceMechanisms = []TargetViolationPersistenceMechanism{TargetViolationCheckpointRestore}
	}

	if targetViolationContainsAny(lifecycle, "checkpoint-fork") {
		signature.PersistenceMechanisms = append(signature.PersistenceMechanisms, TargetViolationSharedRuntime, TargetViolationMissingCleanup)
	}
	if targetViolationContainsAny(lifecycle, "checkpoint-replay", "checkpoint-restore", "session-restore") {
		signature.PersistenceMechanisms = append(signature.PersistenceMechanisms, TargetViolationCheckpointRestore)
	}
	if targetViolationContainsAny(activation, "trusted-client") && targetViolationContainsAny(primitive, "unix-listener") {
		signature.Consequences = append(signature.Consequences, TargetViolationServiceImpersonation)
	}
	return mustNormalizeTargetViolationSignature(signature)
}

// TargetViolationSignatures returns the taxonomy mapping for every built-in
// executable Scenario IR, including generated scenario metadata once it has
// been lowered into a schedule candidate.
func TargetViolationSignatures() []TargetViolationSignatureInfo {
	scenarios := TargetScenarios()
	out := make([]TargetViolationSignatureInfo, 0, len(scenarios))
	for _, scenario := range scenarios {
		out = append(out, TargetViolationSignatureInfo{
			ScenarioID: scenario.ScenarioID,
			SeedID:     scenario.SeedID,
			TaskID:     scenario.TaskID,
			Signature:  scenario.ViolationSignature,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScenarioID != out[j].ScenarioID {
			return out[i].ScenarioID < out[j].ScenarioID
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

// TargetViolationSignatureForTask resolves the normalized Scenario IR label
// for a run result without inventing a taxonomy for an unknown custom task.
func TargetViolationSignatureForTask(taskID string, scenario *TargetScenarioInfo) *TargetViolationSignature {
	if scenario != nil {
		signature, err := NormalizeTargetViolationSignature(scenario.ViolationSignature)
		if err == nil {
			return &signature
		}
	}
	if builtin, ok := TargetScenarioByTaskID(taskID); ok {
		signature := builtin.ViolationSignature
		return &signature
	}
	return nil
}

func mustNormalizeTargetViolationSignature(signature TargetViolationSignature) TargetViolationSignature {
	normalized, err := NormalizeTargetViolationSignature(signature)
	if err != nil {
		panic(err)
	}
	return normalized
}

func NormalizeTargetViolationSignature(signature TargetViolationSignature) (TargetViolationSignature, error) {
	if signature.SchemaVersion == "" {
		signature.SchemaVersion = TargetViolationSignatureSchemaVersion
	}
	if signature.SchemaVersion != TargetViolationSignatureSchemaVersion {
		return TargetViolationSignature{}, fmt.Errorf("unsupported target violation signature schema %q", signature.SchemaVersion)
	}
	signature.Relations = targetViolationUniqueRelations(signature.Relations)
	signature.ResourceClasses = targetViolationUniqueResourceClasses(signature.ResourceClasses)
	signature.LifecycleBoundary = TargetViolationLifecycleBoundary(strings.TrimSpace(string(signature.LifecycleBoundary)))
	signature.PersistenceMechanisms = targetViolationUniquePersistenceMechanisms(signature.PersistenceMechanisms)
	signature.Consequences = targetViolationUniqueConsequences(signature.Consequences)
	if len(signature.Relations) == 0 || len(signature.ResourceClasses) == 0 || signature.LifecycleBoundary == "" || len(signature.PersistenceMechanisms) == 0 || len(signature.Consequences) == 0 {
		return TargetViolationSignature{}, fmt.Errorf("target violation signature requires all five taxonomy dimensions")
	}
	for _, relation := range signature.Relations {
		if !validTargetViolationRelation(relation) {
			return TargetViolationSignature{}, fmt.Errorf("unsupported target violation relation %q", relation)
		}
	}
	for _, resourceClass := range signature.ResourceClasses {
		if !validTargetViolationResourceClass(resourceClass) {
			return TargetViolationSignature{}, fmt.Errorf("unsupported target violation resource class %q", resourceClass)
		}
	}
	if !validTargetViolationLifecycleBoundary(signature.LifecycleBoundary) {
		return TargetViolationSignature{}, fmt.Errorf("unsupported target violation lifecycle boundary %q", signature.LifecycleBoundary)
	}
	for _, mechanism := range signature.PersistenceMechanisms {
		if !validTargetViolationPersistenceMechanism(mechanism) {
			return TargetViolationSignature{}, fmt.Errorf("unsupported target violation persistence mechanism %q", mechanism)
		}
	}
	for _, consequence := range signature.Consequences {
		if !validTargetViolationConsequence(consequence) {
			return TargetViolationSignature{}, fmt.Errorf("unsupported target violation consequence %q", consequence)
		}
	}
	signature.SignatureID = targetViolationSignatureID(signature)
	return signature, nil
}

func targetViolationSignatureID(signature TargetViolationSignature) string {
	return "relation=" + targetViolationJoin(signature.Relations) +
		"|resource=" + targetViolationJoin(signature.ResourceClasses) +
		"|boundary=" + string(signature.LifecycleBoundary) +
		"|mechanism=" + targetViolationJoin(signature.PersistenceMechanisms) +
		"|consequence=" + targetViolationJoin(signature.Consequences)
}

func targetViolationBoundaryFor(value string) TargetViolationLifecycleBoundary {
	switch {
	case targetViolationContainsAny(value, "checkpoint-fork"):
		return TargetViolationBoundaryCheckpointFork
	case targetViolationContainsAny(value, "checkpoint-replay"):
		return TargetViolationBoundaryCheckpointReplay
	case targetViolationContainsAny(value, "checkpoint-restore"):
		return TargetViolationBoundaryCheckpointRestore
	case targetViolationContainsAny(value, "session-restore"):
		return TargetViolationBoundarySessionRestore
	case targetViolationContainsAny(value, "post-return"):
		return TargetViolationBoundaryCommandReturn
	default:
		return TargetViolationBoundaryContinuation
	}
}

func targetViolationLifecycleValue(info *TargetScenarioInfo) string {
	if info == nil {
		return ""
	}
	if info.ExecutionPlan != nil && strings.TrimSpace(info.ExecutionPlan.LifecycleOperationID) != "" {
		return strings.ToLower(strings.TrimSpace(info.ExecutionPlan.LifecycleOperationID))
	}
	return strings.ToLower(strings.TrimSpace(info.LifecycleEdge))
}

func targetViolationNormalizedField(info *TargetScenarioInfo, value func(*TargetScenarioInfo) string) string {
	if info == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value(info)))
}

func targetViolationContainsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func targetViolationUniqueRelations(values []TargetViolationRelation) []TargetViolationRelation {
	seen := make(map[TargetViolationRelation]struct{}, len(values))
	for _, value := range values {
		value = TargetViolationRelation(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]TargetViolationRelation, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func targetViolationUniqueResourceClasses(values []TargetViolationResourceClass) []TargetViolationResourceClass {
	seen := make(map[TargetViolationResourceClass]struct{}, len(values))
	for _, value := range values {
		value = TargetViolationResourceClass(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]TargetViolationResourceClass, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func targetViolationUniquePersistenceMechanisms(values []TargetViolationPersistenceMechanism) []TargetViolationPersistenceMechanism {
	seen := make(map[TargetViolationPersistenceMechanism]struct{}, len(values))
	for _, value := range values {
		value = TargetViolationPersistenceMechanism(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]TargetViolationPersistenceMechanism, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func targetViolationUniqueConsequences(values []TargetViolationConsequence) []TargetViolationConsequence {
	seen := make(map[TargetViolationConsequence]struct{}, len(values))
	for _, value := range values {
		value = TargetViolationConsequence(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]TargetViolationConsequence, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func validTargetViolationRelation(value TargetViolationRelation) bool {
	switch value {
	case TargetViolationResidual, TargetViolationMissing, TargetViolationDuplicated, TargetViolationReorderedDelayed, TargetViolationRebound:
		return true
	default:
		return false
	}
}

func validTargetViolationResourceClass(value TargetViolationResourceClass) bool {
	switch value {
	case TargetViolationNamespaceObject, TargetViolationExecutionContext, TargetViolationProcess, TargetViolationHandleCapability, TargetViolationIPCEndpoint, TargetViolationPermissionMetadata, TargetViolationExternalEffect, TargetViolationAuthorityState:
		return true
	default:
		return false
	}
}

func validTargetViolationLifecycleBoundary(value TargetViolationLifecycleBoundary) bool {
	switch value {
	case TargetViolationBoundaryContinuation, TargetViolationBoundaryCommandReturn, TargetViolationBoundaryCheckpointFork, TargetViolationBoundaryCheckpointReplay, TargetViolationBoundaryCheckpointRestore, TargetViolationBoundarySessionRestore:
		return true
	default:
		return false
	}
}

func validTargetViolationPersistenceMechanism(value TargetViolationPersistenceMechanism) bool {
	switch value {
	case TargetViolationSharedWorkspace, TargetViolationPersistentShell, TargetViolationDescendantSurvival, TargetViolationFDInheritance, TargetViolationNamespaceRebinding, TargetViolationDelayedExecution, TargetViolationMissingCleanup, TargetViolationSharedRuntime, TargetViolationCheckpointRestore, TargetViolationRuntimeReexecution:
		return true
	default:
		return false
	}
}

func validTargetViolationConsequence(value TargetViolationConsequence) bool {
	switch value {
	case TargetViolationObservation, TargetViolationCrossBranchInterference, TargetViolationServiceImpersonation, TargetViolationSecretCapture, TargetViolationStateCorruption, TargetViolationDuplicateOperation, TargetViolationAuthorityConflict:
		return true
	default:
		return false
	}
}

func targetViolationJoin[T ~string](values []T) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	return strings.Join(parts, "+")
}
