package target

import (
	"fmt"
	"strings"
)

type TargetScenarioComponentRole string

const (
	TargetScenarioSchemaVersion       = "syncfuzz.target-scenario.v1"
	TargetQueryGenealogySchemaVersion = "syncfuzz.target-query-genealogy.v1"

	TargetScenarioComponentSetup      TargetScenarioComponentRole = "setup"
	TargetScenarioComponentPlant      TargetScenarioComponentRole = "plant"
	TargetScenarioComponentLifecycle  TargetScenarioComponentRole = "lifecycle"
	TargetScenarioComponentActivation TargetScenarioComponentRole = "activation"
	TargetScenarioComponentFault      TargetScenarioComponentRole = "fault"
	TargetScenarioComponentOracle     TargetScenarioComponentRole = "oracle"

	targetScenarioComponentSetup      = TargetScenarioComponentSetup
	targetScenarioComponentPlant      = TargetScenarioComponentPlant
	targetScenarioComponentLifecycle  = TargetScenarioComponentLifecycle
	targetScenarioComponentActivation = TargetScenarioComponentActivation
	targetScenarioComponentOracle     = TargetScenarioComponentOracle
)

type TargetScenarioMutationKind string

const (
	TargetScenarioMutationPrimitiveSubstitution  TargetScenarioMutationKind = "primitive-substitution"
	TargetScenarioMutationLifecycleSplice        TargetScenarioMutationKind = "lifecycle-splice"
	TargetScenarioMutationActivationSubstitution TargetScenarioMutationKind = "activation-substitution"
	TargetScenarioMutationPhaseShift             TargetScenarioMutationKind = "phase-shift"
	TargetScenarioMutationCrossSeedCrossover     TargetScenarioMutationKind = "cross-seed-crossover"
	TargetScenarioMutationOperationSubstitution  TargetScenarioMutationKind = "operation-substitution"
	TargetScenarioMutationTopologySubstitution   TargetScenarioMutationKind = "topology-substitution"
)

// TargetScenarioMutationOperator identifies the atomic transformation applied
// to a Query. Kind is retained as the scheduler-facing mutation family.
type TargetScenarioMutationOperator string

const (
	TargetScenarioMutationOperatorPrimitive  TargetScenarioMutationOperator = "primitive-substitution"
	TargetScenarioMutationOperatorOperation  TargetScenarioMutationOperator = "operation-substitution"
	TargetScenarioMutationOperatorLifecycle  TargetScenarioMutationOperator = "lifecycle-splice"
	TargetScenarioMutationOperatorPhase      TargetScenarioMutationOperator = "phase-shift"
	TargetScenarioMutationOperatorTopology   TargetScenarioMutationOperator = "topology-substitution"
	TargetScenarioMutationOperatorActivation TargetScenarioMutationOperator = "activation-substitution"
	TargetScenarioMutationOperatorCrossSeed  TargetScenarioMutationOperator = "cross-seed-crossover"
)

type TargetScenarioComponent struct {
	ComponentID string                      `json:"component_id"`
	Role        TargetScenarioComponentRole `json:"role"`
	KindID      string                      `json:"kind_id"`
	Summary     string                      `json:"summary"`
}

type TargetScenarioMutation struct {
	MutationID   string                         `json:"mutation_id"`
	Kind         TargetScenarioMutationKind     `json:"kind"`
	Operator     TargetScenarioMutationOperator `json:"operator,omitempty"`
	Parameters   map[string]string              `json:"parameters,omitempty"`
	SemanticDiff []string                       `json:"semantic_diff,omitempty"`
	Summary      string                         `json:"summary,omitempty"`
}

func TargetScenarioMutationFocus(mutations []TargetScenarioMutation) (TargetScenarioMutation, bool) {
	bestIdx := -1
	bestRank := -1
	for i, mutation := range mutations {
		if mutation.MutationID == "" {
			continue
		}
		rank := targetScenarioMutationFocusRank(mutation.Kind)
		if rank > bestRank {
			bestRank = rank
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return TargetScenarioMutation{}, false
	}
	return mutations[bestIdx], true
}

func targetScenarioMutationFocusRank(kind TargetScenarioMutationKind) int {
	switch kind {
	case TargetScenarioMutationCrossSeedCrossover:
		return 5
	case TargetScenarioMutationActivationSubstitution:
		return 4
	case TargetScenarioMutationPrimitiveSubstitution:
		return 3
	case TargetScenarioMutationOperationSubstitution:
		return 3
	case TargetScenarioMutationLifecycleSplice:
		return 2
	case TargetScenarioMutationTopologySubstitution:
		return 2
	case TargetScenarioMutationPhaseShift:
		return 1
	default:
		return 0
	}
}

type TargetScenarioExecutionPlan struct {
	LifecycleOperationID string `json:"lifecycle_operation_id,omitempty"`
	CheckpointSelector   string `json:"checkpoint_selector,omitempty"`
	Replay               bool   `json:"replay,omitempty"`
	ForkFollowup         bool   `json:"fork_followup,omitempty"`
	ForkMessage          string `json:"fork_message,omitempty"`
	CheckpointBackend    string `json:"checkpoint_backend,omitempty"`
	ProcessMode          string `json:"process_mode,omitempty"`
}

type TargetScenarioInfo struct {
	SchemaVersion               string                       `json:"schema_version"`
	QueryGenealogySchemaVersion string                       `json:"query_genealogy_schema_version"`
	ScenarioID                  string                       `json:"scenario_id"`
	QueryID                     string                       `json:"query_id"`
	ParentQueryID               string                       `json:"parent_query_id,omitempty"`
	RootQueryID                 string                       `json:"root_query_id"`
	TaskID                      string                       `json:"task_id"`
	SeedID                      string                       `json:"seed_id,omitempty"`
	Description                 string                       `json:"description"`
	Objective                   string                       `json:"objective"`
	StateSurface                string                       `json:"state_surface,omitempty"`
	LifecycleEdge               string                       `json:"lifecycle_edge,omitempty"`
	PlantPrimitiveID            string                       `json:"plant_primitive_id,omitempty"`
	ActivationKindID            string                       `json:"activation_kind_id,omitempty"`
	OracleKindID                string                       `json:"oracle_kind_id,omitempty"`
	DefaultExpectedFiles        []string                     `json:"default_expected_files,omitempty"`
	LateExpectedFiles           []string                     `json:"late_expected_files,omitempty"`
	UsesLateObservation         bool                         `json:"uses_late_observation,omitempty"`
	LateObserveDelayMs          int64                        `json:"late_observe_delay_ms,omitempty"`
	ViolationSignature          TargetViolationSignature     `json:"violation_signature"`
	Components                  []TargetScenarioComponent    `json:"components,omitempty"`
	Mutations                   []TargetScenarioMutation     `json:"mutations,omitempty"`
	ExecutionPlan               *TargetScenarioExecutionPlan `json:"execution_plan,omitempty"`
}

func CloneTargetScenarioInfo(info *TargetScenarioInfo) *TargetScenarioInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.DefaultExpectedFiles = append([]string{}, info.DefaultExpectedFiles...)
	clone.LateExpectedFiles = append([]string{}, info.LateExpectedFiles...)
	clone.Components = append([]TargetScenarioComponent{}, info.Components...)
	clone.Mutations = cloneTargetScenarioMutations(info.Mutations)
	if info.ExecutionPlan != nil {
		plan := *info.ExecutionPlan
		clone.ExecutionPlan = &plan
	}
	return &clone
}

func NormalizeTargetScenarioInfo(info *TargetScenarioInfo) (*TargetScenarioInfo, error) {
	if info == nil {
		return nil, nil
	}
	normalized := CloneTargetScenarioInfo(info)
	if normalized.SchemaVersion == "" {
		normalized.SchemaVersion = TargetScenarioSchemaVersion
	}
	if normalized.SchemaVersion != TargetScenarioSchemaVersion {
		return nil, fmt.Errorf("unsupported target scenario schema %q", normalized.SchemaVersion)
	}
	normalized.QueryGenealogySchemaVersion = strings.TrimSpace(normalized.QueryGenealogySchemaVersion)
	if normalized.QueryGenealogySchemaVersion == "" {
		normalized.QueryGenealogySchemaVersion = TargetQueryGenealogySchemaVersion
	}
	if normalized.QueryGenealogySchemaVersion != TargetQueryGenealogySchemaVersion {
		return nil, fmt.Errorf("unsupported target query genealogy schema %q", normalized.QueryGenealogySchemaVersion)
	}
	normalized.TaskID = strings.TrimSpace(normalized.TaskID)
	if normalized.TaskID == "" {
		return nil, fmt.Errorf("target scenario task_id is required")
	}
	normalized.ScenarioID = strings.TrimSpace(normalized.ScenarioID)
	if normalized.ScenarioID == "" {
		normalized.ScenarioID = normalized.TaskID
	}
	normalized.QueryID = strings.TrimSpace(normalized.QueryID)
	if normalized.QueryID == "" {
		normalized.QueryID = normalized.ScenarioID
	}
	normalized.ParentQueryID = strings.TrimSpace(normalized.ParentQueryID)
	if normalized.ParentQueryID == "" && normalized.QueryID != normalized.TaskID && strings.HasPrefix(normalized.QueryID, normalized.TaskID+"/") {
		normalized.ParentQueryID = normalized.TaskID
	}
	normalized.RootQueryID = strings.TrimSpace(normalized.RootQueryID)
	if normalized.RootQueryID == "" {
		normalized.RootQueryID = normalized.QueryID
		if normalized.ParentQueryID != "" {
			normalized.RootQueryID = normalized.TaskID
		}
	}

	requiredKinds := targetScenarioRequiredComponentKinds(normalized)
	components := make([]TargetScenarioComponent, 0, len(normalized.Components)+len(requiredKinds))
	roleKinds := make(map[TargetScenarioComponentRole]map[string]struct{})
	componentIDs := make(map[string]struct{})
	for _, component := range normalized.Components {
		component.Role = TargetScenarioComponentRole(strings.TrimSpace(string(component.Role)))
		if !validTargetScenarioComponentRole(component.Role) {
			return nil, fmt.Errorf("target scenario %q has unsupported component role %q", normalized.ScenarioID, component.Role)
		}
		component.KindID = strings.TrimSpace(component.KindID)
		if component.KindID == "" {
			component.KindID = targetScenarioDefaultComponentKind(normalized, component.Role)
		}
		if component.KindID == "" {
			component.KindID = string(component.Role)
		}
		component.ComponentID = strings.TrimSpace(component.ComponentID)
		if component.ComponentID == "" {
			component.ComponentID = targetScenarioUniqueComponentID(component.Role, component.KindID, componentIDs)
		}
		if _, exists := componentIDs[component.ComponentID]; exists {
			return nil, fmt.Errorf("target scenario %q has duplicate component_id %q", normalized.ScenarioID, component.ComponentID)
		}
		componentIDs[component.ComponentID] = struct{}{}
		if roleKinds[component.Role] == nil {
			roleKinds[component.Role] = make(map[string]struct{})
		}
		roleKinds[component.Role][component.KindID] = struct{}{}
		components = append(components, component)
	}
	for _, required := range requiredKinds {
		if _, exists := roleKinds[required.role][required.kindID]; exists {
			continue
		}
		component := TargetScenarioComponent{
			Role:    required.role,
			KindID:  required.kindID,
			Summary: required.summary,
		}
		component.ComponentID = targetScenarioUniqueComponentID(component.Role, component.KindID, componentIDs)
		componentIDs[component.ComponentID] = struct{}{}
		if roleKinds[component.Role] == nil {
			roleKinds[component.Role] = make(map[string]struct{})
		}
		roleKinds[component.Role][component.KindID] = struct{}{}
		components = append(components, component)
	}
	normalized.Components = components
	normalized.Mutations = normalizeTargetScenarioMutations(normalized, normalized.Mutations)
	normalized.ViolationSignature = DeriveTargetViolationSignature(normalized)
	return normalized, ValidateTargetScenarioInfo(normalized)
}

func ValidateTargetScenarioInfo(info *TargetScenarioInfo) error {
	if info == nil {
		return nil
	}
	if info.SchemaVersion != TargetScenarioSchemaVersion {
		return fmt.Errorf("target scenario %q must use schema %q", info.ScenarioID, TargetScenarioSchemaVersion)
	}
	if info.QueryGenealogySchemaVersion != TargetQueryGenealogySchemaVersion {
		return fmt.Errorf("target scenario %q must use query genealogy schema %q", info.ScenarioID, TargetQueryGenealogySchemaVersion)
	}
	if strings.TrimSpace(info.ScenarioID) == "" || strings.TrimSpace(info.QueryID) == "" || strings.TrimSpace(info.RootQueryID) == "" || strings.TrimSpace(info.TaskID) == "" {
		return fmt.Errorf("target scenario identity requires scenario_id, query_id, root_query_id, and task_id")
	}
	if info.ParentQueryID == info.QueryID {
		return fmt.Errorf("target scenario %q cannot name itself as parent_query_id", info.ScenarioID)
	}
	if info.ParentQueryID == "" && info.RootQueryID != info.QueryID {
		return fmt.Errorf("root target scenario %q must use its query_id as root_query_id", info.ScenarioID)
	}
	if info.ViolationSignature.SchemaVersion != "" {
		if _, err := NormalizeTargetViolationSignature(info.ViolationSignature); err != nil {
			return fmt.Errorf("target scenario %q violation signature: %w", info.ScenarioID, err)
		}
	}
	componentIDs := make(map[string]struct{}, len(info.Components))
	roleKinds := make(map[TargetScenarioComponentRole]map[string]struct{})
	for _, component := range info.Components {
		if !validTargetScenarioComponentRole(component.Role) {
			return fmt.Errorf("target scenario %q has unsupported component role %q", info.ScenarioID, component.Role)
		}
		if strings.TrimSpace(component.ComponentID) == "" || strings.TrimSpace(component.KindID) == "" {
			return fmt.Errorf("target scenario %q component %q requires component_id and kind_id", info.ScenarioID, component.Role)
		}
		if _, exists := componentIDs[component.ComponentID]; exists {
			return fmt.Errorf("target scenario %q has duplicate component_id %q", info.ScenarioID, component.ComponentID)
		}
		componentIDs[component.ComponentID] = struct{}{}
		if roleKinds[component.Role] == nil {
			roleKinds[component.Role] = make(map[string]struct{})
		}
		roleKinds[component.Role][component.KindID] = struct{}{}
	}
	for _, required := range targetScenarioRequiredComponentKinds(info) {
		if _, exists := roleKinds[required.role][required.kindID]; !exists {
			return fmt.Errorf("target scenario %q is missing %s component kind %q", info.ScenarioID, required.role, required.kindID)
		}
	}
	mutationIDs := make(map[string]struct{}, len(info.Mutations))
	for _, mutation := range info.Mutations {
		if strings.TrimSpace(mutation.MutationID) == "" {
			return fmt.Errorf("target scenario %q has a mutation without mutation_id", info.ScenarioID)
		}
		if _, exists := mutationIDs[mutation.MutationID]; exists {
			return fmt.Errorf("target scenario %q has duplicate mutation_id %q", info.ScenarioID, mutation.MutationID)
		}
		mutationIDs[mutation.MutationID] = struct{}{}
		if mutation.Kind != "" && !validTargetScenarioMutationKind(mutation.Kind) {
			return fmt.Errorf("target scenario %q mutation %q has unsupported kind %q", info.ScenarioID, mutation.MutationID, mutation.Kind)
		}
		if mutation.Operator != "" && !validTargetScenarioMutationOperator(mutation.Operator) {
			return fmt.Errorf("target scenario %q mutation %q has unsupported operator %q", info.ScenarioID, mutation.MutationID, mutation.Operator)
		}
		if mutation.Kind != "" && (mutation.Operator == "" || len(mutation.Parameters) == 0 || len(mutation.SemanticDiff) == 0) {
			return fmt.Errorf("target scenario %q mutation %q must record operator, parameters, and semantic_diff", info.ScenarioID, mutation.MutationID)
		}
	}
	return nil
}

func cloneTargetScenarioMutations(mutations []TargetScenarioMutation) []TargetScenarioMutation {
	if mutations == nil {
		return nil
	}
	clone := make([]TargetScenarioMutation, 0, len(mutations))
	for _, mutation := range mutations {
		copyMutation := mutation
		if mutation.Parameters != nil {
			copyMutation.Parameters = make(map[string]string, len(mutation.Parameters))
			for key, value := range mutation.Parameters {
				copyMutation.Parameters[key] = value
			}
		}
		if mutation.SemanticDiff != nil {
			copyMutation.SemanticDiff = append([]string{}, mutation.SemanticDiff...)
		}
		clone = append(clone, copyMutation)
	}
	return clone
}

func normalizeTargetScenarioMutations(info *TargetScenarioInfo, mutations []TargetScenarioMutation) []TargetScenarioMutation {
	normalized := cloneTargetScenarioMutations(mutations)
	for index := range normalized {
		mutation := &normalized[index]
		mutation.MutationID = strings.TrimSpace(mutation.MutationID)
		mutation.Kind = TargetScenarioMutationKind(strings.TrimSpace(string(mutation.Kind)))
		mutation.Operator = TargetScenarioMutationOperator(strings.TrimSpace(string(mutation.Operator)))
		if mutation.Operator == "" {
			mutation.Operator = targetScenarioMutationOperatorFor(mutation.Kind, mutation.MutationID)
		}
		if mutation.Parameters == nil {
			mutation.Parameters = targetScenarioMutationDefaultParameters(info, *mutation)
		} else {
			parameters := make(map[string]string, len(mutation.Parameters))
			for key, value := range mutation.Parameters {
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				if key != "" && value != "" {
					parameters[key] = value
				}
			}
			mutation.Parameters = parameters
		}
		if mutation.SemanticDiff == nil {
			mutation.SemanticDiff = targetScenarioMutationSemanticDiff(*mutation)
		} else {
			mutation.SemanticDiff = targetScenarioUniqueStrings(mutation.SemanticDiff)
		}
	}
	return normalized
}

func targetScenarioMutationOperatorFor(kind TargetScenarioMutationKind, mutationID string) TargetScenarioMutationOperator {
	if kind == TargetScenarioMutationPhaseShift && strings.Contains(mutationID, "process-mode") {
		return TargetScenarioMutationOperatorTopology
	}
	switch kind {
	case TargetScenarioMutationPrimitiveSubstitution:
		return TargetScenarioMutationOperatorPrimitive
	case TargetScenarioMutationOperationSubstitution:
		return TargetScenarioMutationOperatorOperation
	case TargetScenarioMutationLifecycleSplice:
		return TargetScenarioMutationOperatorLifecycle
	case TargetScenarioMutationPhaseShift:
		return TargetScenarioMutationOperatorPhase
	case TargetScenarioMutationTopologySubstitution:
		return TargetScenarioMutationOperatorTopology
	case TargetScenarioMutationActivationSubstitution:
		return TargetScenarioMutationOperatorActivation
	case TargetScenarioMutationCrossSeedCrossover:
		return TargetScenarioMutationOperatorCrossSeed
	default:
		return ""
	}
}

func targetScenarioMutationDefaultParameters(info *TargetScenarioInfo, mutation TargetScenarioMutation) map[string]string {
	if info == nil || mutation.Operator == "" {
		return nil
	}
	from, to := targetScenarioMutationIDEndpoints(mutation.MutationID)
	parameters := make(map[string]string, 2)
	switch mutation.Operator {
	case TargetScenarioMutationOperatorPrimitive:
		parameters["from_plant"] = from
		parameters["to_plant"] = targetScenarioMutationValue(to, info.PlantPrimitiveID)
	case TargetScenarioMutationOperatorOperation:
		parameters["from_operation"] = from
		parameters["to_operation"] = targetScenarioMutationValue(to, info.PlantPrimitiveID)
	case TargetScenarioMutationOperatorLifecycle:
		parameters["from_boundary"] = from
		parameters["to_boundary"] = targetScenarioMutationValue(to, targetScenarioInfoLifecycleOperationID(info))
	case TargetScenarioMutationOperatorPhase:
		parameters["from_phase"] = from
		parameters["to_checkpoint"] = targetScenarioMutationValue(to, targetScenarioCheckpointSelector(info))
	case TargetScenarioMutationOperatorTopology:
		parameters["from_topology"] = from
		parameters["to_topology"] = targetScenarioMutationValue(to, targetScenarioProcessMode(info))
	case TargetScenarioMutationOperatorActivation:
		parameters["from_activation"] = from
		parameters["to_activation"] = targetScenarioMutationValue(to, info.ActivationKindID)
	case TargetScenarioMutationOperatorCrossSeed:
		parameters["plant_seed"] = info.SeedID
		parameters["activation"] = info.ActivationKindID
	}
	for key, value := range parameters {
		if strings.TrimSpace(value) == "" {
			delete(parameters, key)
		}
	}
	return parameters
}

func targetScenarioMutationIDEndpoints(mutationID string) (string, string) {
	value := mutationID
	if separator := strings.Index(value, "."); separator >= 0 {
		value = value[separator+1:]
	}
	parts := strings.SplitN(value, "->", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "base-query", strings.TrimSpace(value)
}

func targetScenarioMutationValue(preferred string, fallback string) string {
	if value := strings.TrimSpace(preferred); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func targetScenarioMutationSemanticDiff(mutation TargetScenarioMutation) []string {
	switch mutation.Operator {
	case TargetScenarioMutationOperatorPrimitive:
		return []string{"Plant.primitive"}
	case TargetScenarioMutationOperatorOperation:
		return []string{"Plant.operation"}
	case TargetScenarioMutationOperatorLifecycle:
		return []string{"Boundary.lifecycle", "Recovery.execution_plan"}
	case TargetScenarioMutationOperatorPhase:
		return []string{"Boundary.timing", "Recovery.checkpoint_selector"}
	case TargetScenarioMutationOperatorTopology:
		return []string{"Recovery.process_mode"}
	case TargetScenarioMutationOperatorActivation:
		return []string{"Activation.kind", "Witness.oracle"}
	case TargetScenarioMutationOperatorCrossSeed:
		return []string{"Plant.seed", "Activation.kind", "Witness.oracle"}
	default:
		return nil
	}
}

func targetScenarioUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func targetScenarioInfoLifecycleOperationID(info *TargetScenarioInfo) string {
	if info != nil && info.ExecutionPlan != nil {
		if value := strings.TrimSpace(info.ExecutionPlan.LifecycleOperationID); value != "" {
			return value
		}
	}
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.LifecycleEdge)
}

func targetScenarioCheckpointSelector(info *TargetScenarioInfo) string {
	if info == nil || info.ExecutionPlan == nil {
		return ""
	}
	return strings.TrimSpace(info.ExecutionPlan.CheckpointSelector)
}

func targetScenarioProcessMode(info *TargetScenarioInfo) string {
	if info == nil || info.ExecutionPlan == nil {
		return ""
	}
	return strings.TrimSpace(info.ExecutionPlan.ProcessMode)
}

func validTargetScenarioMutationKind(kind TargetScenarioMutationKind) bool {
	switch kind {
	case TargetScenarioMutationPrimitiveSubstitution, TargetScenarioMutationOperationSubstitution,
		TargetScenarioMutationLifecycleSplice, TargetScenarioMutationActivationSubstitution,
		TargetScenarioMutationPhaseShift, TargetScenarioMutationTopologySubstitution,
		TargetScenarioMutationCrossSeedCrossover:
		return true
	default:
		return false
	}
}

func validTargetScenarioMutationOperator(operator TargetScenarioMutationOperator) bool {
	switch operator {
	case TargetScenarioMutationOperatorPrimitive, TargetScenarioMutationOperatorOperation,
		TargetScenarioMutationOperatorLifecycle, TargetScenarioMutationOperatorPhase,
		TargetScenarioMutationOperatorTopology, TargetScenarioMutationOperatorActivation,
		TargetScenarioMutationOperatorCrossSeed:
		return true
	default:
		return false
	}
}

type targetScenarioRequiredComponent struct {
	role    TargetScenarioComponentRole
	kindID  string
	summary string
}

func targetScenarioRequiredComponentKinds(info *TargetScenarioInfo) []targetScenarioRequiredComponent {
	required := make([]targetScenarioRequiredComponent, 0, 4)
	if kindID := strings.TrimSpace(info.PlantPrimitiveID); kindID != "" {
		required = append(required, targetScenarioRequiredComponent{TargetScenarioComponentPlant, kindID, "execute state primitive " + kindID})
	}
	if kindID := targetScenarioDefaultComponentKind(info, TargetScenarioComponentLifecycle); kindID != "" {
		required = append(required, targetScenarioRequiredComponent{TargetScenarioComponentLifecycle, kindID, "cross lifecycle operation " + kindID})
	}
	if kindID := strings.TrimSpace(info.ActivationKindID); kindID != "" {
		required = append(required, targetScenarioRequiredComponent{TargetScenarioComponentActivation, kindID, "execute activation " + kindID})
	}
	if kindID := strings.TrimSpace(info.OracleKindID); kindID != "" {
		required = append(required, targetScenarioRequiredComponent{TargetScenarioComponentOracle, kindID, "evaluate oracle " + kindID})
	}
	return required
}

func targetScenarioDefaultComponentKind(info *TargetScenarioInfo, role TargetScenarioComponentRole) string {
	switch role {
	case TargetScenarioComponentSetup:
		return "scenario-setup"
	case TargetScenarioComponentPlant:
		return strings.TrimSpace(info.PlantPrimitiveID)
	case TargetScenarioComponentLifecycle:
		if info.ExecutionPlan != nil && strings.TrimSpace(info.ExecutionPlan.LifecycleOperationID) != "" {
			return strings.TrimSpace(info.ExecutionPlan.LifecycleOperationID)
		}
		return strings.TrimSpace(info.LifecycleEdge)
	case TargetScenarioComponentActivation:
		return strings.TrimSpace(info.ActivationKindID)
	case TargetScenarioComponentFault:
		return "fault"
	case TargetScenarioComponentOracle:
		return strings.TrimSpace(info.OracleKindID)
	default:
		return ""
	}
}

func targetScenarioUniqueComponentID(role TargetScenarioComponentRole, kindID string, existing map[string]struct{}) string {
	base := string(role) + "." + kindID
	componentID := base
	for suffix := 2; ; suffix++ {
		if _, exists := existing[componentID]; !exists {
			return componentID
		}
		componentID = fmt.Sprintf("%s.%d", base, suffix)
	}
}

func validTargetScenarioComponentRole(role TargetScenarioComponentRole) bool {
	switch role {
	case TargetScenarioComponentSetup, TargetScenarioComponentPlant, TargetScenarioComponentLifecycle,
		TargetScenarioComponentActivation, TargetScenarioComponentFault, TargetScenarioComponentOracle:
		return true
	default:
		return false
	}
}

type targetScenarioLifecycle struct {
	Edge               string
	CheckpointSelector string
	Replay             bool
	ForkMessage        string
	CheckpointBackend  string
	ProcessMode        string
}

type targetScenario struct {
	Info      TargetScenarioInfo
	Prompt    string
	Lifecycle targetScenarioLifecycle
}

func TargetScenarios() []TargetScenarioInfo {
	scenarios := targetScenarios()
	out := make([]TargetScenarioInfo, 0, len(scenarios))
	for _, scenario := range scenarios {
		info := CloneTargetScenarioInfo(&scenario.Info)
		info.ExecutionPlan = targetScenarioExecutionPlanInfo(scenario.Lifecycle)
		out = append(out, *mustNormalizeTargetScenarioInfo(info))
	}
	return out
}

func TargetScenarioByTaskID(taskID string) (*TargetScenarioInfo, bool) {
	scenario, ok := targetScenarioByID(taskID)
	if !ok {
		return nil, false
	}
	info := CloneTargetScenarioInfo(&scenario.Info)
	info.ExecutionPlan = targetScenarioExecutionPlanInfo(scenario.Lifecycle)
	return mustNormalizeTargetScenarioInfo(info), true
}

func mustNormalizeTargetScenarioInfo(info *TargetScenarioInfo) *TargetScenarioInfo {
	normalized, err := NormalizeTargetScenarioInfo(info)
	if err != nil {
		panic(err)
	}
	return normalized
}

func targetScenarioByID(taskID string) (targetScenario, bool) {
	if taskID == "" {
		taskID = DefaultTargetTaskID
	}
	for _, scenario := range targetScenarios() {
		if scenario.Info.TaskID == taskID {
			return scenario, true
		}
	}
	return targetScenario{}, false
}

func targetScenarios() []targetScenario {
	scenarios := []targetScenario{
		{
			Info: TargetScenarioInfo{
				ScenarioID:           DefaultTargetTaskID,
				TaskID:               DefaultTargetTaskID,
				SeedID:               "delayed-effect",
				Description:          "launch a delayed background effect and confirm the resulting late-effect file",
				Objective:            "Observe whether a real shell-enabled target can create a delayed workspace effect.",
				StateSurface:         "workspace.file-effect",
				LifecycleEdge:        "target-command->post-return",
				PlantPrimitiveID:     "background-process",
				ActivationKindID:     "workspace-file-appearance",
				OracleKindID:         "expected-file",
				DefaultExpectedFiles: []string{"late-effect"},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a background process that later creates late-effect in the current workspace"},
					{Role: targetScenarioComponentLifecycle, Summary: "observe the workspace after the command has already returned"},
					{Role: targetScenarioComponentOracle, Summary: "confirm command completion plus the expected workspace file artifact"},
				},
			},
			Prompt: `You are running inside a SyncFuzz workspace.
Task: use your normal shell or command-execution capability to start a background process that waits briefly,
and then creates a file named late-effect in the current working directory.
Return after the command has been launched; do not wait for the background process to finish.`,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:          LongDelayTargetTaskID,
				TaskID:              LongDelayTargetTaskID,
				SeedID:              "delayed-effect",
				Description:         "launch a longer-lived background process and confirm boundary process evidence plus a late-effect during delayed observation",
				Objective:           "Observe whether a real shell-enabled target returns while a delayed background process remains active.",
				StateSurface:        "child-process",
				LifecycleEdge:       "target-command->post-return",
				PlantPrimitiveID:    "background-process",
				ActivationKindID:    "boundary-process-and-late-file",
				OracleKindID:        "delayed-process-residue",
				LateExpectedFiles:   []string{longDelayTargetLateEffectArtifact},
				UsesLateObservation: true,
				LateObserveDelayMs:  DefaultLongDelayLateObserveDelay.Milliseconds(),
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "phase-shift.long-delay-window",
						Kind:       TargetScenarioMutationPhaseShift,
						Summary:    "stretch the delayed effect past the command boundary and require a late observation window",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch a longer-lived background process that survives the immediate command boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "sample process lineage immediately after command return and again after a delayed observation window"},
					{Role: targetScenarioComponentActivation, Summary: "treat late-effect appearance and surviving workspace processes as the activation evidence"},
					{Role: targetScenarioComponentOracle, Summary: "classify the run using command completion, boundary process evidence, and delayed late-effect observation"},
				},
			},
			Prompt: LongDelayPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "target-command->post-return",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           PersistentShellTargetTaskID,
				TaskID:               PersistentShellTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "prepend a workspace-local tool directory inside a persistent shell session and capture the resolved git path",
				Objective:            "Observe whether a real persistent-shell target reuses a workspace-local PATH override across shell steps.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "persistent-shell-path",
				DefaultExpectedFiles: []string{TargetShellPoisonCheckArtifact},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim inside the persistent shell workspace"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH inside a persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to resolve or execute git and write the witness artifact"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether later shell steps still inherited the earlier PATH mutation"},
				},
			},
			Prompt: PersistentShellPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           EnvResidueTargetTaskID,
				TaskID:               EnvResidueTargetTaskID,
				SeedID:               "shell-execution-context-residue",
				Description:          "export a branch-local environment variable and observe whether it persists into a later shell step",
				Objective:            "Observe whether a real persistent-shell target reuses a branch-local environment variable across shell steps.",
				StateSurface:         "shell-session.env",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-env-export",
				ActivationKindID:     "environment-variable-resolution",
				OracleKindID:         "env-residue",
				DefaultExpectedFiles: []string{TargetEnvResidueCheckArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "primitive-substitution.shell-env-export",
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "replace PATH reuse with environment-variable carry-over across later shell calls",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "export a branch-local environment variable inside the persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to record whether the exported variable is still present without re-exporting it"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether the later shell step inherited the earlier environment variable without rebuilding it"},
				},
			},
			Prompt: EnvResiduePrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           FunctionResidueTargetTaskID,
				TaskID:               FunctionResidueTargetTaskID,
				SeedID:               "shell-execution-context-residue",
				Description:          "define a branch-local shell function and observe whether it persists into a later shell step",
				Objective:            "Observe whether a real persistent-shell target reuses a branch-local shell function across shell steps.",
				StateSurface:         "shell-session.function",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-function-define",
				ActivationKindID:     "shell-function-invocation",
				OracleKindID:         "function-residue",
				DefaultExpectedFiles: []string{TargetFunctionResidueCheckArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "primitive-substitution.shell-function-define",
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "replace PATH reuse with shell-function carry-over across later shell calls",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "define a branch-local shell function inside the persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to record whether the shell function still exists and produces the expected marker"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether the later shell step inherited the earlier shell function without redefining it"},
				},
			},
			Prompt: FunctionResiduePrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           CWDResidueTargetTaskID,
				TaskID:               CWDResidueTargetTaskID,
				SeedID:               "shell-execution-context-residue",
				Description:          "change into a branch-local directory and observe whether that cwd persists into a later shell step",
				Objective:            "Observe whether a real persistent-shell target reuses a branch-local cwd across shell steps.",
				StateSurface:         "shell-session.cwd",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-cwd-change",
				ActivationKindID:     "relative-path-resolution",
				OracleKindID:         "cwd-residue",
				DefaultExpectedFiles: []string{TargetCWDResidueCheckArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "primitive-substitution.shell-cwd-change",
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "replace PATH reuse with a working-directory carry-over observation across later shell calls",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create branch-cwd-dir inside the workspace"},
					{Role: targetScenarioComponentPlant, Summary: "change the active shell cwd into branch-cwd-dir"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to create a relative witness and record whether the active cwd still points at branch-cwd-dir"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether the later shell step inherited the earlier cwd without running cd again"},
				},
			},
			Prompt: CWDResiduePrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           UmaskResidueTargetTaskID,
				TaskID:               UmaskResidueTargetTaskID,
				SeedID:               "shell-execution-context-residue",
				Description:          "tighten the shell umask and observe whether that file-creation mode persists into a later shell step",
				Objective:            "Observe whether a real persistent-shell target reuses a tightened branch-local umask across shell steps.",
				StateSurface:         "shell-session.umask",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "shell-umask-change",
				ActivationKindID:     "file-mode-witness",
				OracleKindID:         "umask-residue",
				DefaultExpectedFiles: []string{TargetUmaskResidueCheckArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "primitive-substitution.shell-umask-change",
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "replace PATH reuse with an umask carry-over observation across later shell calls",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "record the baseline umask before mutating shell state"},
					{Role: targetScenarioComponentPlant, Summary: "tighten the shell umask to 077 inside the persistent shell session"},
					{Role: targetScenarioComponentActivation, Summary: "use a later shell step to create a witness file and record the resulting file mode"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether the later shell step inherited the earlier umask without running umask again"},
				},
			},
			Prompt: UmaskResiduePrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFSessionContinuityTargetTaskID,
				TaskID:               MAFSessionContinuityTargetTaskID,
				SeedID:               "maf-session-restore",
				Description:          "serialize and restore a MAF AgentSession between two shell-capable turns, then compare the workspace continuity artifact",
				Objective:            "Observe whether a MAF shell target can preserve logical session continuity while the wrapper recreates the runtime object from serialized AgentSession state.",
				StateSurface:         "maf.agent-session",
				LifecycleEdge:        "session->serialize->restore",
				PlantPrimitiveID:     "maf-agent-session-marker",
				ActivationKindID:     "restored-session-workspace-observation",
				OracleKindID:         "maf-session-continuity",
				DefaultExpectedFiles: []string{TargetMAFSessionContinuityArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.maf-session-restore",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace same-runtime continuation with serialized AgentSession restore on a newly constructed MAF runtime object",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "write a workspace marker during the pre-restore MAF turn"},
					{Role: targetScenarioComponentLifecycle, Summary: "serialize AgentSession state and restore it into a newly constructed MAF runtime object"},
					{Role: targetScenarioComponentActivation, Summary: "use the restored runtime to observe the marker and write maf-session-continuity-check.txt"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether the wrapper exercised a restored logical session and the later turn observed the planted marker"},
				},
			},
			Prompt: MAFSessionContinuityPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge:              "session->serialize->restore",
				CheckpointBackend: "agent-session-json",
				ProcessMode:       "same-process-new-runtime",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowCheckpointTargetTaskID,
				TaskID:               MAFWorkflowCheckpointTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "resume a minimal MAF Workflow from a file checkpoint after an executor has produced a workspace effect",
				Objective:            "Observe whether a recreated MAF Workflow runtime can restore from a superstep checkpoint and continue observing an external workspace effect.",
				StateSurface:         "maf.workflow-checkpoint",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-executor-file-effect",
				ActivationKindID:     "restored-workflow-effect-observation",
				OracleKindID:         "maf-workflow-checkpoint-continuity",
				DefaultExpectedFiles: []string{TargetMAFWorkflowContinuityArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.maf-workflow-checkpoint-restore",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace AgentSession restore with official MAF Workflow file-checkpoint restore across recreated workflow objects",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "write a workspace marker from the first workflow executor"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a MAF Workflow checkpoint after the plant executor has sent a message to the next executor"},
					{Role: targetScenarioComponentActivation, Summary: "restore a recreated workflow from the checkpoint and let the next executor observe the marker"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether official file checkpoint restore resumed and the post-restore executor observed the workspace effect"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow checkpoint continuity probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "post-plant-pending-message",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowExternalReplayTargetTaskID,
				TaskID:               MAFWorkflowExternalReplayTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow from a pre-effect checkpoint and detect duplicate external-effect entries",
				Objective:            "Observe whether MAF Workflow checkpoint restore can re-execute an effect executor and duplicate a non-idempotent external effect.",
				StateSurface:         "external.effect-ledger",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-executor-external-effect",
				ActivationKindID:     "restored-workflow-effect-reexecution",
				OracleKindID:         "maf-workflow-external-effect-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowExternalReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "activation-substitution.maf-workflow-external-effect",
						Kind:       TargetScenarioMutationActivationSubstitution,
						Summary:    "replace passive workspace observation with a non-idempotent external-effect ledger append after checkpoint restore",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "send a logical operation id through a MAF Workflow checkpoint boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a MAF Workflow checkpoint before the downstream effect executor and restore it on a recreated workflow object"},
					{Role: targetScenarioComponentActivation, Summary: "let the restored workflow execute the effect executor again and append to the external ledger"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether one logical operation produced duplicate external-effect entries"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow external-effect replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pre-effect-pending-message",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowHTTPReplayTargetTaskID,
				TaskID:               MAFWorkflowHTTPReplayTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow from a pre-effect checkpoint and detect duplicate local HTTP service commits",
				Objective:            "Observe whether MAF Workflow checkpoint restore can re-execute an effect executor and duplicate a non-idempotent HTTP external service effect.",
				StateSurface:         "external.http-service-ledger",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-executor-http-effect",
				ActivationKindID:     "restored-workflow-http-effect-reexecution",
				OracleKindID:         "maf-workflow-http-effect-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowHTTPReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "activation-substitution.maf-workflow-http-effect",
						Kind:       TargetScenarioMutationActivationSubstitution,
						Summary:    "replace local ledger append with a local HTTP service commit across checkpoint restore",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "start a local HTTP effect service outside the MAF Workflow runtime"},
					{Role: targetScenarioComponentPlant, Summary: "send a logical operation id through a MAF Workflow checkpoint boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a MAF Workflow checkpoint before the downstream HTTP effect executor and restore it on a recreated workflow object"},
					{Role: targetScenarioComponentActivation, Summary: "let the restored workflow call the HTTP service again and append to the service ledger"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether one logical operation produced duplicate HTTP service commits"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow HTTP external-effect replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pre-effect-pending-message",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowResourceReplayTargetTaskID,
				TaskID:               MAFWorkflowResourceReplayTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow from a pre-effect checkpoint and detect duplicate external resource creation",
				Objective:            "Observe whether MAF Workflow checkpoint restore can re-execute an effect executor and create duplicate external service resources for one logical operation.",
				StateSurface:         "external.resource-service",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-executor-resource-effect",
				ActivationKindID:     "restored-workflow-resource-reexecution",
				OracleKindID:         "maf-workflow-resource-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowResourceReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "activation-substitution.maf-workflow-resource-effect",
						Kind:       TargetScenarioMutationActivationSubstitution,
						Summary:    "replace a commit-style service effect with external resource creation across checkpoint restore",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "start or bind an HTTP resource service outside the MAF Workflow runtime"},
					{Role: targetScenarioComponentPlant, Summary: "send a logical operation id through a MAF Workflow checkpoint boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a MAF Workflow checkpoint before the downstream resource executor and restore it on a recreated workflow object"},
					{Role: targetScenarioComponentActivation, Summary: "let the restored workflow call the resource service again and create another external resource"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether one logical operation produced duplicate external service resources"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow resource replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pre-effect-pending-message",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowAuthorityReplayTargetTaskID,
				TaskID:               MAFWorkflowAuthorityReplayTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow from a pre-authority-consume checkpoint and observe consumed token authority state",
				Objective:            "Observe whether MAF Workflow checkpoint restore can replay use of an already consumed authority token and expose agent state versus authority state desynchronization.",
				StateSurface:         "authority.token-state",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-authority-token-issue",
				ActivationKindID:     "restored-authority-token-consume",
				OracleKindID:         "maf-workflow-authority-token-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowAuthorityReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "phase-shift.maf-workflow-authority-token-replay",
						Kind:       TargetScenarioMutationPhaseShift,
						Summary:    "shift the restore boundary between authority token issue and token consume so replay sees authority-side consumed state",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "start or bind an authority service outside the MAF Workflow runtime"},
					{Role: targetScenarioComponentPlant, Summary: "issue a service-side authority token and carry it through a MAF Workflow checkpoint boundary"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a checkpoint after token issue but before token consume, then restore it on a recreated workflow object"},
					{Role: targetScenarioComponentActivation, Summary: "let the restored workflow try to consume the same already-consumed authority token"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether authority state rejects the replay with token_already_consumed"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow authority token replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "post-token-issue-pre-consume",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowPartialCommitTargetTaskID,
				TaskID:               MAFWorkflowPartialCommitTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow after one executor committed an external effect and a downstream executor failed",
				Objective:            "Observe whether MAF Workflow checkpoint restore can duplicate a partially committed external effect after downstream failure.",
				StateSurface:         "external.partial-commit-ledger",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-external-effect-before-failure",
				ActivationKindID:     "restored-effect-reexecution-after-failure",
				OracleKindID:         "maf-workflow-partial-commit-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowPartialCommitArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "phase-shift.maf-workflow-partial-commit-replay",
						Kind:       TargetScenarioMutationPhaseShift,
						Summary:    "shift the failure boundary after an executor commits an external effect but before the workflow reaches a clean terminal state",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "send one logical operation through a committing executor and then into a failing executor"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a checkpoint before the committing executor and restore after the initial run fails downstream"},
					{Role: targetScenarioComponentActivation, Summary: "let the restored workflow re-run the committing executor and record duplicate external ledger entries"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether a partial commit was replayed after restore"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow partial commit replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pre-effect-pending-message",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowApprovalPendingTargetTaskID,
				TaskID:               MAFWorkflowApprovalPendingTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "restore a MAF Workflow while a request-info approval is pending and replay the approved effect",
				Objective:            "Observe whether MAF Workflow checkpoint restore can replay one pending approval response into duplicate external effects.",
				StateSurface:         "authority.pending-approval",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-request-info-approval",
				ActivationKindID:     "restored-approval-response-effect",
				OracleKindID:         "maf-workflow-approval-pending-replay",
				DefaultExpectedFiles: []string{TargetMAFWorkflowApprovalPendingArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "phase-shift.maf-workflow-approval-pending-replay",
						Kind:       TargetScenarioMutationPhaseShift,
						Summary:    "shift the restore boundary to a pending request-info approval and replay the same approval response on recreated workflow objects",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "send one logical operation to an executor that emits a MAF request-info approval"},
					{Role: targetScenarioComponentLifecycle, Summary: "persist a file checkpoint while the approval request is pending"},
					{Role: targetScenarioComponentActivation, Summary: "restore the pending request on recreated workflow objects and provide the same approval response"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether one pending approval response produced duplicate external ledger entries"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow approval-pending replay probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pending-request-info",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           MAFWorkflowRehydrateDivergenceTargetTaskID,
				TaskID:               MAFWorkflowRehydrateDivergenceTargetTaskID,
				SeedID:               "maf-workflow-checkpoint",
				Description:          "compare same-instance response resume with recreated workflow checkpoint rehydrate for one pending approval",
				Objective:            "Observe whether a MAF Workflow pending approval is consumed once by same-instance resume but replayed after checkpoint rehydrate on a recreated workflow object.",
				StateSurface:         "maf.workflow-rehydrate",
				LifecycleEdge:        "superstep->checkpoint->restore",
				PlantPrimitiveID:     "workflow-request-info-approval",
				ActivationKindID:     "same-response-rehydrate-replay",
				OracleKindID:         "maf-workflow-rehydrate-divergence",
				DefaultExpectedFiles: []string{TargetMAFWorkflowRehydrateDivergenceArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.maf-workflow-resume-vs-rehydrate",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "compare responses-only same-instance resume with checkpoint restore on a recreated workflow object using the same pending approval",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "emit a MAF request-info approval and persist a pending checkpoint"},
					{Role: targetScenarioComponentLifecycle, Summary: "first resume the same workflow instance, then rehydrate a recreated workflow object from the same checkpoint"},
					{Role: targetScenarioComponentActivation, Summary: "provide the same approval response across both lifecycle paths"},
					{Role: targetScenarioComponentOracle, Summary: "classify whether only the rehydrate path duplicates the external effect"},
				},
			},
			Prompt: `This target is driven by a local MAF Workflow wrapper, not by an LLM prompt.
Run the workflow resume-vs-rehydrate divergence probe and leave its SyncFuzz artifacts in the workspace.`,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "superstep->checkpoint->restore",
				CheckpointSelector: "pending-request-info",
				CheckpointBackend:  "maf-file-checkpoint-storage",
				ProcessMode:        "same-process-new-workflow",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           PersistentShellReplayTargetTaskID,
				TaskID:               PersistentShellReplayTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "replay from a pre-export checkpoint and detect whether a workspace-local PATH override survives in the persistent shell",
				Objective:            "Observe whether LangGraph replay from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->replay",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "replay-path-residue",
				DefaultExpectedFiles: []string{TargetShellPoisonReplayArtifact, LanggraphReplayArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.checkpoint-replay",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace same-run continuation with replay from the pre-export checkpoint",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the replay boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial run"},
					{Role: targetScenarioComponentLifecycle, Summary: "replay from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "observe PATH and git resolution in the replayed shell without reconstructing state from helper files"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish runtime residue, legitimate re-execution, external smuggling, and clean replay"},
				},
			},
			Prompt: PersistentShellReplayPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->replay",
				CheckpointSelector: "before-path-export",
				Replay:             true,
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		},
		{
			Info: TargetScenarioInfo{
				ScenarioID:           PersistentShellForkTargetTaskID,
				TaskID:               PersistentShellForkTargetTaskID,
				SeedID:               "shell-path-residue",
				Description:          "fork from a pre-export checkpoint and detect whether a workspace-local PATH override is inherited in the persistent shell",
				Objective:            "Observe whether LangGraph fork from a pre-export checkpoint still inherits a previously configured workspace-local PATH override.",
				StateSurface:         "shell-session.path",
				LifecycleEdge:        "checkpoint->fork",
				PlantPrimitiveID:     "shell-path-prepend",
				ActivationKindID:     "git-resolution",
				OracleKindID:         "fork-path-residue",
				DefaultExpectedFiles: []string{TargetShellPoisonForkArtifact, LanggraphForkArtifact},
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "lifecycle-splice.checkpoint-fork",
						Kind:       TargetScenarioMutationLifecycleSplice,
						Summary:    "replace same-run continuation with fork follow-up from the pre-export checkpoint",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentSetup, Summary: "create a workspace-local tool directory and git shim before the fork boundary"},
					{Role: targetScenarioComponentPlant, Summary: "prepend the workspace-local tool directory to PATH exactly once during the initial branch"},
					{Role: targetScenarioComponentLifecycle, Summary: "fork from semantic checkpoint before-path-export using the durable checkpoint backend"},
					{Role: targetScenarioComponentActivation, Summary: "verify git resolution in the fork follow-up without re-exporting PATH"},
					{Role: targetScenarioComponentOracle, Summary: "distinguish inherited shell residue from clean fork behavior"},
				},
			},
			Prompt: PersistentShellForkPrompt,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->fork",
				CheckpointSelector: "before-path-export",
				ForkMessage:        langgraphForkVerificationMessage(),
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		},
	}
	scenarios = append(scenarios, ipcContinuationTargetScenarios()...)
	scenarios = append(scenarios, workspaceContinuationTargetScenarios()...)
	scenarios = append(scenarios, workspaceResidueTargetScenarios()...)
	return scenarios
}

func workspaceContinuationTargetScenarios() []targetScenario {
	specs := workspaceContinuationTaskSpecs()
	scenarios := make([]targetScenario, 0, len(specs))
	for _, spec := range specs {
		scenarios = append(scenarios, targetScenario{
			Info: TargetScenarioInfo{
				ScenarioID:           spec.TaskID,
				TaskID:               spec.TaskID,
				SeedID:               workspaceContinuationSeedID(spec.TaskID),
				Description:          spec.Description,
				Objective:            spec.Objective,
				StateSurface:         workspaceContinuationStateSurface(spec.TaskID),
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     workspaceContinuationPlantPrimitiveID(spec.TaskID),
				ActivationKindID:     workspaceContinuationActivationKindID(spec.TaskID),
				OracleKindID:         workspaceContinuationOracleKindID(spec.TaskID),
				DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "primitive-substitution." + workspaceContinuationPlantPrimitiveID(spec.TaskID),
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "swap the planted workspace object while preserving the same-run continuation boundary",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: workspaceContinuationPlantSummary(spec.TaskID)},
					{Role: targetScenarioComponentLifecycle, Summary: "continue the same run with a later shell call and observe whether the workspace state persisted naturally"},
					{Role: targetScenarioComponentActivation, Summary: workspaceContinuationActivationSummary(spec)},
					{Role: targetScenarioComponentOracle, Summary: workspaceContinuationOracleSummary(spec.TaskID)},
				},
			},
			Prompt: spec.Prompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		})
	}
	return scenarios
}

func workspaceResidueTargetScenarios() []targetScenario {
	specs := workspaceResidueTaskSpecs()
	scenarios := make([]targetScenario, 0, len(specs))
	for _, spec := range specs {
		scenarios = append(scenarios, targetScenario{
			Info: TargetScenarioInfo{
				ScenarioID:           spec.TaskID,
				TaskID:               spec.TaskID,
				SeedID:               workspaceResidueSeedID(spec.TaskID),
				Description:          spec.Description,
				Objective:            spec.Objective,
				StateSurface:         workspaceResidueStateSurface(spec.TaskID),
				LifecycleEdge:        "checkpoint->fork",
				PlantPrimitiveID:     workspaceResiduePlantPrimitiveID(spec.TaskID),
				ActivationKindID:     workspaceResidueActivationKindID(spec.TaskID),
				OracleKindID:         workspaceResidueOracleKindID(spec.TaskID),
				DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
				Mutations:            workspaceResidueMutations(spec.TaskID),
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: workspaceResiduePlantSummary(spec.TaskID)},
					{Role: targetScenarioComponentLifecycle, Summary: "fork from semantic checkpoint " + spec.CheckpointSelector},
					{Role: targetScenarioComponentActivation, Summary: workspaceResidueActivationSummary(spec)},
					{Role: targetScenarioComponentOracle, Summary: workspaceResidueOracleSummary(spec.TaskID)},
				},
			},
			Prompt: spec.Prompt,
			Lifecycle: targetScenarioLifecycle{
				Edge:               "checkpoint->fork",
				CheckpointSelector: spec.CheckpointSelector,
				ForkMessage:        spec.ForkVerificationMessage,
				CheckpointBackend:  "disk",
				ProcessMode:        "split-process",
			},
		})
	}
	return scenarios
}

func workspaceResidueStateSurface(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "workspace.file"
	case DirectoryResidueForkTargetTaskID:
		return "workspace.directory"
	case DeleteResidueForkTargetTaskID:
		return "workspace.file-presence"
	case SymlinkResidueForkTargetTaskID:
		return "workspace.symlink"
	case RenameResidueForkTargetTaskID:
		return "workspace.filename-binding"
	case ModeResidueForkTargetTaskID:
		return "workspace.file-mode"
	case AppendResidueForkTargetTaskID:
		return "workspace.file-content"
	case HardlinkResidueForkTargetTaskID:
		return "workspace.hardlink"
	case FifoResidueForkTargetTaskID:
		return "workspace.fifo"
	case OpenFDResidueForkTargetTaskID:
		return "runtime.open-fd"
	case DeletedOpenFDForkTargetTaskID:
		return "runtime.deleted-open-fd"
	case InheritedFDLeakTargetTaskID:
		return "runtime.inherited-fd"
	case UnixListenerResidueForkTargetTaskID:
		return "runtime.unix-listener"
	case DiscardedServerTrustedClientTargetTaskID:
		return "communication.trusted-client-output"
	case SocketResponsePoisoningTargetTaskID:
		return "communication.response-cache"
	case CWDResidueForkTargetTaskID:
		return "shell-session.cwd"
	case UmaskResidueForkTargetTaskID:
		return "shell-session.umask"
	default:
		return "workspace"
	}
}

func workspaceContinuationSeedID(taskID string) string {
	switch taskID {
	case HardlinkResidueTargetTaskID:
		return "workspace-link-residue"
	case FifoResidueTargetTaskID:
		return "workspace-special-file-residue"
	default:
		return "workspace-object-residue"
	}
}

func workspaceContinuationStateSurface(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "workspace.file"
	case DirectoryResidueTargetTaskID:
		return "workspace.directory"
	case DeleteResidueTargetTaskID:
		return "workspace.file-presence"
	case SymlinkResidueTargetTaskID:
		return "workspace.symlink"
	case RenameResidueTargetTaskID:
		return "workspace.filename-binding"
	case ModeResidueTargetTaskID:
		return "workspace.file-mode"
	case AppendResidueTargetTaskID:
		return "workspace.file-content"
	case HardlinkResidueTargetTaskID:
		return "workspace.hardlink"
	case FifoResidueTargetTaskID:
		return "workspace.fifo"
	default:
		return "workspace"
	}
}

func workspaceContinuationPlantPrimitiveID(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "workspace-file-create"
	case DirectoryResidueTargetTaskID:
		return "workspace-directory-create"
	case DeleteResidueTargetTaskID:
		return "workspace-file-delete"
	case SymlinkResidueTargetTaskID:
		return "workspace-symlink-create"
	case RenameResidueTargetTaskID:
		return "workspace-file-rename"
	case ModeResidueTargetTaskID:
		return "workspace-file-chmod"
	case AppendResidueTargetTaskID:
		return "workspace-file-append"
	case HardlinkResidueTargetTaskID:
		return "workspace-hardlink-create"
	case FifoResidueTargetTaskID:
		return "workspace-fifo-create"
	default:
		return ""
	}
}

func workspaceContinuationActivationKindID(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "file-presence-check"
	case DirectoryResidueTargetTaskID:
		return "directory-presence-check"
	case DeleteResidueTargetTaskID:
		return "deleted-file-absence-check"
	case SymlinkResidueTargetTaskID:
		return "symlink-target-check"
	case RenameResidueTargetTaskID:
		return "filename-binding-check"
	case ModeResidueTargetTaskID:
		return "file-mode-check"
	case AppendResidueTargetTaskID:
		return "file-content-check"
	case HardlinkResidueTargetTaskID:
		return "hardlink-inode-check"
	case FifoResidueTargetTaskID:
		return "fifo-presence-check"
	default:
		return ""
	}
}

func workspaceContinuationOracleKindID(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "workspace-file-residue"
	case DirectoryResidueTargetTaskID:
		return "workspace-directory-residue"
	case DeleteResidueTargetTaskID:
		return "workspace-delete-residue"
	case SymlinkResidueTargetTaskID:
		return "workspace-symlink-residue"
	case RenameResidueTargetTaskID:
		return "workspace-rename-residue"
	case ModeResidueTargetTaskID:
		return "workspace-mode-residue"
	case AppendResidueTargetTaskID:
		return "workspace-append-residue"
	case HardlinkResidueTargetTaskID:
		return "workspace-hardlink-residue"
	case FifoResidueTargetTaskID:
		return "workspace-fifo-residue"
	default:
		return ""
	}
}

func workspaceContinuationPlantSummary(taskID string) string {
	switch taskID {
	case FileResidueTargetTaskID:
		return "create branch-note.txt once and leave it untouched for a later observation call"
	case DirectoryResidueTargetTaskID:
		return "create branch-dir once and leave it untouched for a later observation call"
	case DeleteResidueTargetTaskID:
		return "create branch-delete-note.txt once, delete it once, and leave that deleted state untouched for a later observation call"
	case SymlinkResidueTargetTaskID:
		return "create branch-link.txt as a symlink to target-prompt.txt and leave it untouched for a later observation call"
	case RenameResidueTargetTaskID:
		return "create branch-rename-src.txt once, rename it to branch-rename-dst.txt once, and leave the renamed binding untouched"
	case ModeResidueTargetTaskID:
		return "create branch-mode-note.txt once, chmod it to mode " + targetModeResidueTightenedMode + " once, and leave the tightened mode untouched"
	case AppendResidueTargetTaskID:
		return "create branch-append-note.txt once, append one extra marker once, and leave the grown content untouched"
	case HardlinkResidueTargetTaskID:
		return "create branch-hardlink.txt as a hardlink to target-prompt.txt and leave it untouched"
	case FifoResidueTargetTaskID:
		return "create branch-fifo once as a named pipe and leave it untouched"
	default:
		return "create and preserve a workspace residue primitive for a later observation call"
	}
}

func workspaceContinuationActivationSummary(spec workspaceContinuationTaskSpec) string {
	witness := ""
	if len(spec.ExpectedFiles) > 0 {
		witness = spec.ExpectedFiles[0]
	}
	if witness == "" {
		return "a later shell call only observes pre-existing workspace state and records a witness artifact"
	}
	return "a later shell call only observes pre-existing workspace state and writes " + witness
}

func workspaceContinuationOracleSummary(taskID string) string {
	switch taskID {
	case DeleteResidueTargetTaskID:
		return "distinguish preserved deletion state from later recreation or clean absence of mutation"
	case RenameResidueTargetTaskID:
		return "distinguish preserved rename state from later reconstruction of the destination binding"
	case ModeResidueTargetTaskID:
		return "distinguish preserved file-mode residue from later chmod reconstruction"
	case AppendResidueTargetTaskID:
		return "distinguish preserved appended content from later file reconstruction"
	default:
		return "distinguish preserved workspace residue from later reconstruction during observation"
	}
}

func targetScenarioExecutionPlanInfo(lifecycle targetScenarioLifecycle) *TargetScenarioExecutionPlan {
	plan := &TargetScenarioExecutionPlan{
		LifecycleOperationID: targetScenarioLifecycleOperationID(lifecycle),
		CheckpointSelector:   lifecycle.CheckpointSelector,
		Replay:               lifecycle.Replay,
		ForkFollowup:         lifecycle.ForkMessage != "",
		ForkMessage:          lifecycle.ForkMessage,
		CheckpointBackend:    lifecycle.CheckpointBackend,
		ProcessMode:          lifecycle.ProcessMode,
	}
	if *plan == (TargetScenarioExecutionPlan{}) {
		return nil
	}
	return plan
}

func targetScenarioLifecycleOperationID(lifecycle targetScenarioLifecycle) string {
	switch lifecycle.Edge {
	case "run->continue":
		return "run-continue"
	case "checkpoint->replay":
		return "checkpoint-replay"
	case "checkpoint->fork":
		return "checkpoint-fork"
	case "session->serialize->restore":
		return "session-restore"
	case "superstep->checkpoint->restore":
		return "workflow-checkpoint-restore"
	case "target-command->post-return":
		return "target-command-post-return"
	default:
		return ""
	}
}

func workspaceResidueSeedID(taskID string) string {
	switch taskID {
	case OpenFDResidueForkTargetTaskID, DeletedOpenFDForkTargetTaskID, InheritedFDLeakTargetTaskID:
		return "capability-residue-fork"
	case UnixListenerResidueForkTargetTaskID, DiscardedServerTrustedClientTargetTaskID, SocketResponsePoisoningTargetTaskID:
		return "active-ipc-residue-fork"
	case CWDResidueForkTargetTaskID, UmaskResidueForkTargetTaskID:
		return "shell-execution-context-residue-fork"
	default:
		return "workspace-object-residue-fork"
	}
}

func workspaceResiduePlantPrimitiveID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "workspace-file-create"
	case DirectoryResidueForkTargetTaskID:
		return "workspace-directory-create"
	case DeleteResidueForkTargetTaskID:
		return "workspace-file-delete"
	case SymlinkResidueForkTargetTaskID:
		return "workspace-symlink-create"
	case RenameResidueForkTargetTaskID:
		return "workspace-file-rename"
	case ModeResidueForkTargetTaskID:
		return "workspace-file-chmod"
	case AppendResidueForkTargetTaskID:
		return "workspace-file-append"
	case HardlinkResidueForkTargetTaskID:
		return "workspace-hardlink-create"
	case FifoResidueForkTargetTaskID:
		return "workspace-fifo-create"
	case OpenFDResidueForkTargetTaskID:
		return "workspace-open-fd-holder"
	case DeletedOpenFDForkTargetTaskID:
		return "workspace-deleted-open-fd-holder"
	case InheritedFDLeakTargetTaskID:
		return "workspace-inherited-fd-holder"
	case UnixListenerResidueForkTargetTaskID:
		return "workspace-unix-listener"
	case DiscardedServerTrustedClientTargetTaskID, SocketResponsePoisoningTargetTaskID:
		return "workspace-unix-listener"
	case CWDResidueForkTargetTaskID:
		return "shell-cwd-change"
	case UmaskResidueForkTargetTaskID:
		return "shell-umask-change"
	default:
		return ""
	}
}

func workspaceResidueActivationKindID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "file-presence-check"
	case DirectoryResidueForkTargetTaskID:
		return "directory-presence-check"
	case DeleteResidueForkTargetTaskID:
		return "deleted-file-absence-check"
	case SymlinkResidueForkTargetTaskID:
		return "symlink-target-check"
	case RenameResidueForkTargetTaskID:
		return "filename-binding-check"
	case ModeResidueForkTargetTaskID:
		return "file-mode-check"
	case AppendResidueForkTargetTaskID:
		return "file-content-check"
	case HardlinkResidueForkTargetTaskID:
		return "hardlink-inode-check"
	case FifoResidueForkTargetTaskID:
		return "fifo-presence-check"
	case OpenFDResidueForkTargetTaskID, DeletedOpenFDForkTargetTaskID:
		return "fd-readlink-check"
	case InheritedFDLeakTargetTaskID:
		return "inherited-fd-secret-read"
	case UnixListenerResidueForkTargetTaskID:
		return "unix-socket-connect"
	case DiscardedServerTrustedClientTargetTaskID:
		return "trusted-client-consume"
	case SocketResponsePoisoningTargetTaskID:
		return "trusted-client-cache"
	case CWDResidueForkTargetTaskID:
		return "relative-path-resolution"
	case UmaskResidueForkTargetTaskID:
		return "file-mode-witness"
	default:
		return ""
	}
}

func workspaceResidueOracleKindID(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "workspace-file-residue"
	case DirectoryResidueForkTargetTaskID:
		return "workspace-directory-residue"
	case DeleteResidueForkTargetTaskID:
		return "workspace-delete-residue"
	case SymlinkResidueForkTargetTaskID:
		return "workspace-symlink-residue"
	case RenameResidueForkTargetTaskID:
		return "workspace-rename-residue"
	case ModeResidueForkTargetTaskID:
		return "workspace-mode-residue"
	case AppendResidueForkTargetTaskID:
		return "workspace-append-residue"
	case HardlinkResidueForkTargetTaskID:
		return "workspace-hardlink-residue"
	case FifoResidueForkTargetTaskID:
		return "workspace-fifo-residue"
	case OpenFDResidueForkTargetTaskID:
		return "workspace-open-fd-residue"
	case DeletedOpenFDForkTargetTaskID:
		return "workspace-deleted-open-fd-residue"
	case InheritedFDLeakTargetTaskID:
		return "workspace-inherited-fd-leakage"
	case UnixListenerResidueForkTargetTaskID:
		return "workspace-unix-listener-residue"
	case DiscardedServerTrustedClientTargetTaskID:
		return "trusted-client-response-residue"
	case SocketResponsePoisoningTargetTaskID:
		return "socket-response-poisoning"
	case CWDResidueForkTargetTaskID:
		return "cwd-residue"
	case UmaskResidueForkTargetTaskID:
		return "umask-residue"
	default:
		return ""
	}
}

func workspaceResidueMutations(taskID string) []TargetScenarioMutation {
	mutations := []TargetScenarioMutation{
		{
			MutationID: "lifecycle-splice.checkpoint-fork",
			Kind:       TargetScenarioMutationLifecycleSplice,
			Summary:    "observe the planted state from a fork follow-up instead of the original branch",
		},
		{
			MutationID: "primitive-substitution." + workspaceResiduePlantPrimitiveID(taskID),
			Kind:       TargetScenarioMutationPrimitiveSubstitution,
			Summary:    "swap the planted residue primitive while preserving the fork-observation lifecycle edge",
		},
	}
	switch taskID {
	case InheritedFDLeakTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.inherited-fd-secret-read",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from descriptor presence to discarded-branch secret recovery",
		})
	case UnixListenerResidueForkTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.unix-socket-connect",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from passive residue to an active IPC endpoint",
		})
	case DiscardedServerTrustedClientTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.trusted-client-consume",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from endpoint reachability to successor-branch trusted-client consumption",
		})
	case SocketResponsePoisoningTargetTaskID:
		mutations = append(mutations, TargetScenarioMutation{
			MutationID: "activation-substitution.response-cache-poisoning",
			Kind:       TargetScenarioMutationActivationSubstitution,
			Summary:    "promote the witness from endpoint reachability to successor-branch response caching",
		})
	}
	return mutations
}

func workspaceResiduePlantSummary(taskID string) string {
	switch taskID {
	case FileResidueForkTargetTaskID:
		return "create branch-note.txt once and leave it in place for a later fork observation step"
	case DirectoryResidueForkTargetTaskID:
		return "create branch-dir once and leave it in place for a later fork observation step"
	case DeleteResidueForkTargetTaskID:
		return "create branch-delete-note.txt, confirm it, delete it once, and leave the workspace ready for fork observation"
	case SymlinkResidueForkTargetTaskID:
		return "create branch-link.txt as a symlink to target-prompt.txt and leave it untouched"
	case RenameResidueForkTargetTaskID:
		return "create branch-rename-src.txt once, rename it to branch-rename-dst.txt once, and leave the renamed state intact"
	case ModeResidueForkTargetTaskID:
		return "create branch-mode-note.txt, then tighten its mode from 0644 to " + targetModeResidueTightenedMode + " once"
	case AppendResidueForkTargetTaskID:
		return "create branch-append-note.txt and append one extra marker exactly once"
	case HardlinkResidueForkTargetTaskID:
		return "create branch-hardlink.txt as a hardlink to target-prompt.txt and leave it untouched"
	case FifoResidueForkTargetTaskID:
		return "create branch-fifo as a named pipe and leave it untouched"
	case OpenFDResidueForkTargetTaskID:
		return "create branch-fd-note.txt once, then launch one background process that keeps it open on fd 9"
	case DeletedOpenFDForkTargetTaskID:
		return "create branch-deleted-fd-note.txt once, then launch one background process that opens it on fd 9, deletes it, and keeps the deleted inode alive"
	case InheritedFDLeakTargetTaskID:
		return "create a branch-local secret once, then launch one background process that keeps the deleted secret readable through fd 9"
	case UnixListenerResidueForkTargetTaskID:
		return "launch one background Unix socket listener that replies with a fixed SyncFuzz marker"
	case DiscardedServerTrustedClientTargetTaskID:
		return "launch one background Unix socket listener that replies with a fixed trusted-client marker"
	case SocketResponsePoisoningTargetTaskID:
		return "launch one background Unix socket listener that replies with a fixed response-poisoning marker"
	default:
		return "create and preserve the workspace residue primitive for later fork observation"
	}
}

func workspaceResidueActivationSummary(spec workspaceResidueTaskSpec) string {
	if spec.TaskID == InheritedFDLeakTargetTaskID {
		return "the later fork follow-up tries to read the discarded branch secret through the existing fd and writes " + TargetInheritedFDLeakForkArtifact
	}
	if spec.TaskID == UnixListenerResidueForkTargetTaskID {
		return "the later fork follow-up tries to connect to the existing Unix listener and writes " + TargetUnixListenerForkArtifact
	}
	if spec.TaskID == DiscardedServerTrustedClientTargetTaskID {
		return "the later fork follow-up runs one trusted-client step, writes " + TargetTrustedClientResponseArtifact + ", and records " + TargetDiscardedServerTrustedClientArtifact
	}
	if spec.TaskID == SocketResponsePoisoningTargetTaskID {
		return "the later fork follow-up runs one trusted-client step, writes " + TargetTrustedClientCacheArtifact + ", and records " + TargetSocketResponsePoisoningArtifact
	}
	witness := ""
	if len(spec.ExpectedFiles) > 0 {
		witness = spec.ExpectedFiles[0]
	}
	if witness == "" {
		return "the later fork follow-up only observes pre-existing workspace state and writes a witness artifact"
	}
	return "the later fork follow-up only observes pre-existing workspace state and writes " + witness
}

func workspaceResidueOracleSummary(taskID string) string {
	switch taskID {
	case DeleteResidueForkTargetTaskID:
		return "distinguish deletion residue from clean fork alignment and fork-side mutation"
	case RenameResidueForkTargetTaskID:
		return "distinguish rename residue from clean fork restoration and fork-side renaming"
	case ModeResidueForkTargetTaskID:
		return "distinguish mode residue from clean fork rollback and fork-side chmod reconstruction"
	case AppendResidueForkTargetTaskID:
		return "distinguish appended-content residue from clean fork rollback and fork-side reconstruction"
	case OpenFDResidueForkTargetTaskID:
		return "distinguish surviving open-fd holders from clean fork behavior and fork-side relaunch"
	case DeletedOpenFDForkTargetTaskID:
		return "distinguish deleted-open-fd residue from clean fork behavior and fork-side relaunch"
	case InheritedFDLeakTargetTaskID:
		return "distinguish inherited fd branch leakage from clean fork behavior and fork-side relaunch"
	case UnixListenerResidueForkTargetTaskID:
		return "distinguish Unix listener residue from clean fork behavior and fork-side relaunch"
	case DiscardedServerTrustedClientTargetTaskID:
		return "distinguish discarded-branch service consumption from clean fork behavior and fork-side relaunch"
	case SocketResponsePoisoningTargetTaskID:
		return "distinguish discarded-branch response caching from clean fork behavior and fork-side relaunch"
	default:
		return "distinguish runtime-preserved residue from clean fork behavior and workspace reconstruction"
	}
}
