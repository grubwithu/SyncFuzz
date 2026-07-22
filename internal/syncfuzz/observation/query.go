package observation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

const LifecycleQuerySchemaVersion = "syncfuzz.lifecycle-query.v1"

// QueryComponent preserves the stable Scenario IR component identity, kind,
// and summary as one relation rather than independent parallel lists.
type QueryComponent struct {
	ComponentID string `json:"component_id"`
	KindID      string `json:"kind_id"`
	Summary     string `json:"summary,omitempty"`
}

// QueryStage is the stable, implementation-facing projection of one
// lifecycle-query stage.
type QueryStage struct {
	Components []QueryComponent `json:"components,omitempty"`
}

// QueryBoundary identifies the semantic edge that can desynchronize logical
// agent state from observed OS state.
type QueryBoundary struct {
	LifecycleEdge string     `json:"lifecycle_edge,omitempty"`
	Stage         QueryStage `json:"stage"`
}

// RecoveryDirective records the adapter-visible controls used to exercise the
// boundary. It is a declaration, not evidence that an adapter actually
// performed framework-native recovery.
type RecoveryDirective struct {
	OperationID        string `json:"operation_id,omitempty"`
	CheckpointSelector string `json:"checkpoint_selector,omitempty"`
	Replay             bool   `json:"replay,omitempty"`
	ForkFollowup       bool   `json:"fork_followup,omitempty"`
	CheckpointBackend  string `json:"checkpoint_backend,omitempty"`
	ProcessMode        string `json:"process_mode,omitempty"`
}

// ViolationHypothesis makes explicit what the later differential oracle is
// intended to decide. It is deliberately not an oracle verdict.
type ViolationHypothesis struct {
	Kind             string `json:"kind"`
	StateSurface     string `json:"state_surface,omitempty"`
	LifecycleEdge    string `json:"lifecycle_edge,omitempty"`
	OracleKindID     string `json:"oracle_kind_id,omitempty"`
	ExpectedRelation string `json:"expected_relation"`
}

// LifecycleQuery is q = <Init, Plant, Boundary, Recovery, Activation,
// Witness>. It is the typed handoff from Scenario IR to resource-footprint
// extraction and observation-plan compilation.
type LifecycleQuery struct {
	SchemaVersion string              `json:"schema_version"`
	QueryID       string              `json:"query_id"`
	ScenarioID    string              `json:"scenario_id,omitempty"`
	TaskID        string              `json:"task_id,omitempty"`
	Init          QueryStage          `json:"init"`
	Plant         QueryStage          `json:"plant"`
	Boundary      QueryBoundary       `json:"boundary"`
	Recovery      RecoveryDirective   `json:"recovery"`
	Activation    QueryStage          `json:"activation"`
	Witness       QueryStage          `json:"witness"`
	Hypothesis    ViolationHypothesis `json:"violation_hypothesis"`
}

// LifecycleQueryFromTargetTask derives a portable query from a persisted
// target task. Historical ad-hoc tasks still receive a minimal query so they
// can use the same footprint and plan schemas.
func LifecycleQueryFromTargetTask(task target.TargetTask) (*LifecycleQuery, error) {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("target task id is required for lifecycle query")
	}
	query := &LifecycleQuery{
		SchemaVersion: LifecycleQuerySchemaVersion,
		QueryID:       taskID,
		TaskID:        taskID,
		Boundary: QueryBoundary{
			LifecycleEdge: "run->recovery",
		},
		Hypothesis: ViolationHypothesis{
			Kind:             "recovery-consistency",
			ExpectedRelation: "compare logical recovery semantics against observed state across the lifecycle boundary",
		},
	}
	if task.Scenario == nil {
		return query, NormalizeLifecycleQuery(query)
	}

	scenario, err := target.NormalizeTargetScenarioInfo(task.Scenario)
	if err != nil {
		return nil, fmt.Errorf("normalize scenario for lifecycle query: %w", err)
	}
	query.QueryID = firstNonEmpty(scenario.ScenarioID, scenario.TaskID, taskID)
	query.ScenarioID = scenario.ScenarioID
	query.TaskID = firstNonEmpty(scenario.TaskID, taskID)
	query.Init = queryStageForRoles(scenario, target.TargetScenarioComponentSetup)
	query.Plant = queryStageForRoles(scenario, target.TargetScenarioComponentPlant)
	query.Boundary = QueryBoundary{
		LifecycleEdge: scenario.LifecycleEdge,
		Stage: queryStageForRoles(
			scenario,
			target.TargetScenarioComponentLifecycle,
			target.TargetScenarioComponentFault,
		),
	}
	query.Activation = queryStageForRoles(scenario, target.TargetScenarioComponentActivation)
	query.Witness = queryStageForRoles(scenario, target.TargetScenarioComponentOracle)
	query.Hypothesis = ViolationHypothesis{
		Kind:             "recovery-consistency",
		StateSurface:     scenario.StateSurface,
		LifecycleEdge:    scenario.LifecycleEdge,
		OracleKindID:     scenario.OracleKindID,
		ExpectedRelation: "compare logical recovery semantics against observed state across the lifecycle boundary",
	}
	if scenario.ExecutionPlan != nil {
		query.Recovery = RecoveryDirective{
			OperationID:        scenario.ExecutionPlan.LifecycleOperationID,
			CheckpointSelector: scenario.ExecutionPlan.CheckpointSelector,
			Replay:             scenario.ExecutionPlan.Replay,
			ForkFollowup:       scenario.ExecutionPlan.ForkFollowup,
			CheckpointBackend:  scenario.ExecutionPlan.CheckpointBackend,
			ProcessMode:        scenario.ExecutionPlan.ProcessMode,
		}
	}
	return query, NormalizeLifecycleQuery(query)
}

func queryStageForRoles(scenario *target.TargetScenarioInfo, roles ...target.TargetScenarioComponentRole) QueryStage {
	if scenario == nil {
		return QueryStage{}
	}
	wanted := make(map[target.TargetScenarioComponentRole]struct{}, len(roles))
	for _, role := range roles {
		wanted[role] = struct{}{}
	}
	components := make([]target.TargetScenarioComponent, 0, len(scenario.Components))
	for _, component := range scenario.Components {
		if _, ok := wanted[component.Role]; ok {
			components = append(components, component)
		}
	}
	sort.Slice(components, func(i, j int) bool {
		if components[i].ComponentID != components[j].ComponentID {
			return components[i].ComponentID < components[j].ComponentID
		}
		return components[i].KindID < components[j].KindID
	})
	stage := QueryStage{}
	for _, component := range components {
		stage.Components = append(stage.Components, QueryComponent{
			ComponentID: component.ComponentID,
			KindID:      component.KindID,
			Summary:     component.Summary,
		})
	}
	normalizeQueryStage(&stage)
	return stage
}

func NormalizeLifecycleQuery(query *LifecycleQuery) error {
	if query == nil {
		return fmt.Errorf("lifecycle query is required")
	}
	if query.SchemaVersion == "" {
		query.SchemaVersion = LifecycleQuerySchemaVersion
	}
	if query.SchemaVersion != LifecycleQuerySchemaVersion {
		return fmt.Errorf("unsupported lifecycle query schema %q", query.SchemaVersion)
	}
	query.QueryID = strings.TrimSpace(query.QueryID)
	if query.QueryID == "" {
		return fmt.Errorf("lifecycle query id is required")
	}
	query.ScenarioID = strings.TrimSpace(query.ScenarioID)
	query.TaskID = strings.TrimSpace(query.TaskID)
	normalizeQueryStage(&query.Init)
	normalizeQueryStage(&query.Plant)
	query.Boundary.LifecycleEdge = strings.TrimSpace(query.Boundary.LifecycleEdge)
	normalizeQueryStage(&query.Boundary.Stage)
	query.Recovery.OperationID = strings.TrimSpace(query.Recovery.OperationID)
	query.Recovery.CheckpointSelector = strings.TrimSpace(query.Recovery.CheckpointSelector)
	query.Recovery.CheckpointBackend = strings.TrimSpace(query.Recovery.CheckpointBackend)
	query.Recovery.ProcessMode = strings.TrimSpace(query.Recovery.ProcessMode)
	normalizeQueryStage(&query.Activation)
	normalizeQueryStage(&query.Witness)
	query.Hypothesis.Kind = strings.TrimSpace(query.Hypothesis.Kind)
	query.Hypothesis.StateSurface = strings.TrimSpace(query.Hypothesis.StateSurface)
	query.Hypothesis.LifecycleEdge = strings.TrimSpace(query.Hypothesis.LifecycleEdge)
	query.Hypothesis.OracleKindID = strings.TrimSpace(query.Hypothesis.OracleKindID)
	query.Hypothesis.ExpectedRelation = strings.TrimSpace(query.Hypothesis.ExpectedRelation)
	if query.Hypothesis.Kind == "" {
		query.Hypothesis.Kind = "recovery-consistency"
	}
	if query.Hypothesis.ExpectedRelation == "" {
		query.Hypothesis.ExpectedRelation = "compare logical recovery semantics against observed state across the lifecycle boundary"
	}
	return nil
}

func normalizeQueryStage(stage *QueryStage) {
	if stage == nil {
		return
	}
	byKey := make(map[string]QueryComponent, len(stage.Components))
	for _, component := range stage.Components {
		component.ComponentID = strings.TrimSpace(component.ComponentID)
		component.KindID = strings.TrimSpace(component.KindID)
		component.Summary = strings.TrimSpace(component.Summary)
		if component.ComponentID == "" && component.KindID == "" {
			continue
		}
		key := component.ComponentID + "\x00" + component.KindID + "\x00" + component.Summary
		byKey[key] = component
	}
	stage.Components = make([]QueryComponent, 0, len(byKey))
	for _, component := range byKey {
		stage.Components = append(stage.Components, component)
	}
	sort.Slice(stage.Components, func(i, j int) bool {
		if stage.Components[i].ComponentID != stage.Components[j].ComponentID {
			return stage.Components[i].ComponentID < stage.Components[j].ComponentID
		}
		if stage.Components[i].KindID != stage.Components[j].KindID {
			return stage.Components[i].KindID < stage.Components[j].KindID
		}
		return stage.Components[i].Summary < stage.Components[j].Summary
	})
}
