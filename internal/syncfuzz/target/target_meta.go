package target

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

type TargetTaskInfo struct {
	TaskID               string                     `json:"task_id"`
	ScenarioID           string                     `json:"scenario_id,omitempty"`
	SeedID               string                     `json:"seed_id,omitempty"`
	Description          string                     `json:"description"`
	PlantPrimitiveID     string                     `json:"plant_primitive_id,omitempty"`
	ActivationKindID     string                     `json:"activation_kind_id,omitempty"`
	OracleKindID         string                     `json:"oracle_kind_id,omitempty"`
	DefaultExpectedFiles []string                   `json:"default_expected_files,omitempty"`
	UsesLateObservation  bool                       `json:"uses_late_observation,omitempty"`
	StateSurface         string                     `json:"state_surface,omitempty"`
	LifecycleEdge        string                     `json:"lifecycle_edge,omitempty"`
	LifecycleOperationID string                     `json:"lifecycle_operation_id,omitempty"`
	MutationFocusID      string                     `json:"mutation_focus_id,omitempty"`
	MutationFocusKind    TargetScenarioMutationKind `json:"mutation_focus_kind,omitempty"`
	Mutations            []TargetScenarioMutation   `json:"mutations,omitempty"`
}

type TargetTaskGroupInfo struct {
	GroupID     string   `json:"group_id"`
	Description string   `json:"description"`
	Tasks       []string `json:"tasks"`
}

type TargetScenarioSeedInfo struct {
	SeedID              string   `json:"seed_id"`
	Description         string   `json:"description"`
	Tasks               []string `json:"tasks"`
	PlantPrimitives     []string `json:"plant_primitives,omitempty"`
	LifecycleOperations []string `json:"lifecycle_operations,omitempty"`
	ActivationKinds     []string `json:"activation_kinds,omitempty"`
	OracleKinds         []string `json:"oracle_kinds,omitempty"`
}

func TargetTasks() []TargetTaskInfo {
	scenarios := TargetScenarios()
	tasks := make([]TargetTaskInfo, 0, len(scenarios))
	for _, scenario := range scenarios {
		focus, hasFocus := TargetScenarioMutationFocus(scenario.Mutations)
		tasks = append(tasks, TargetTaskInfo{
			ScenarioID:           scenario.ScenarioID,
			TaskID:               scenario.TaskID,
			SeedID:               scenario.SeedID,
			Description:          scenario.Description,
			PlantPrimitiveID:     scenario.PlantPrimitiveID,
			ActivationKindID:     scenario.ActivationKindID,
			OracleKindID:         scenario.OracleKindID,
			DefaultExpectedFiles: append([]string{}, scenario.DefaultExpectedFiles...),
			UsesLateObservation:  scenario.UsesLateObservation,
			StateSurface:         scenario.StateSurface,
			LifecycleEdge:        scenario.LifecycleEdge,
			Mutations:            append([]TargetScenarioMutation{}, scenario.Mutations...),
		})
		if hasFocus {
			tasks[len(tasks)-1].MutationFocusID = focus.MutationID
			tasks[len(tasks)-1].MutationFocusKind = focus.Kind
		}
		if scenario.ExecutionPlan != nil {
			tasks[len(tasks)-1].LifecycleOperationID = scenario.ExecutionPlan.LifecycleOperationID
		}
	}
	return tasks
}

func TargetTaskByID(taskID string) (TargetTaskInfo, bool) {
	taskID = strings.TrimSpace(taskID)
	for _, task := range TargetTasks() {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return TargetTaskInfo{}, false
}

func TargetTaskGroups() []TargetTaskGroupInfo {
	return []TargetTaskGroupInfo{
		{
			GroupID:     "maf-baseline",
			Description: "current MAF baseline covering the single-step delayed-effect smoke path and the lifecycle-backed long-delay observation path",
			Tasks: []string{
				DefaultTargetTaskID,
				LongDelayTargetTaskID,
			},
		},
		{
			GroupID:     "maf-shell-context",
			Description: "MAF shell-context tasks covering delayed-effect smoke paths plus cross-call PATH, cwd, env, function, and umask residue observation",
			Tasks: []string{
				DefaultTargetTaskID,
				LongDelayTargetTaskID,
				PersistentShellTargetTaskID,
				EnvResidueTargetTaskID,
				FunctionResidueTargetTaskID,
				CWDResidueTargetTaskID,
				UmaskResidueTargetTaskID,
			},
		},
		{
			GroupID:     "phase5a-baseline",
			Description: "current Phase 5A LangGraph baseline covering delayed process, persistent shell, replay/fork shell checks, and workspace residue forks across multiple OS state surfaces",
			Tasks: append([]string{
				LongDelayTargetTaskID,
				PersistentShellTargetTaskID,
				PersistentShellReplayTargetTaskID,
				PersistentShellForkTargetTaskID,
			}, workspaceResidueTaskIDs()...),
		},
		{
			GroupID:     "shell-lifecycle",
			Description: "persistent-shell and replay/fork lifecycle tasks for PATH residue experiments",
			Tasks: []string{
				PersistentShellTargetTaskID,
				PersistentShellReplayTargetTaskID,
				PersistentShellForkTargetTaskID,
			},
		},
		{
			GroupID:     "workspace-residue",
			Description: "fork-based workspace residue tasks covering filesystem objects plus open-fd, inherited-fd, active Unix-listener follow-ups, and cwd/umask residue",
			Tasks:       workspaceResidueTaskIDs(),
		},
	}
}

func TargetScenarioSeeds() []TargetScenarioSeedInfo {
	scenarios := TargetScenarios()
	if len(scenarios) == 0 {
		return nil
	}
	type seedBuilder struct {
		info                TargetScenarioSeedInfo
		plantPrimitives     map[string]struct{}
		lifecycleOperations map[string]struct{}
		activationKinds     map[string]struct{}
		oracleKinds         map[string]struct{}
	}
	builders := make(map[string]*seedBuilder)
	for _, scenario := range scenarios {
		seedID := strings.TrimSpace(scenario.SeedID)
		if seedID == "" {
			continue
		}
		builder := builders[seedID]
		if builder == nil {
			builder = &seedBuilder{
				info: TargetScenarioSeedInfo{
					SeedID:      seedID,
					Description: targetSeedDescription(seedID),
				},
				plantPrimitives:     make(map[string]struct{}),
				lifecycleOperations: make(map[string]struct{}),
				activationKinds:     make(map[string]struct{}),
				oracleKinds:         make(map[string]struct{}),
			}
			builders[seedID] = builder
		}
		builder.info.Tasks = append(builder.info.Tasks, scenario.TaskID)
		if scenario.PlantPrimitiveID != "" {
			builder.plantPrimitives[scenario.PlantPrimitiveID] = struct{}{}
		}
		if scenario.ExecutionPlan != nil && scenario.ExecutionPlan.LifecycleOperationID != "" {
			builder.lifecycleOperations[scenario.ExecutionPlan.LifecycleOperationID] = struct{}{}
		}
		if scenario.ActivationKindID != "" {
			builder.activationKinds[scenario.ActivationKindID] = struct{}{}
		}
		if scenario.OracleKindID != "" {
			builder.oracleKinds[scenario.OracleKindID] = struct{}{}
		}
	}
	seedIDs := make([]string, 0, len(builders))
	for seedID := range builders {
		seedIDs = append(seedIDs, seedID)
	}
	sort.Strings(seedIDs)
	out := make([]TargetScenarioSeedInfo, 0, len(seedIDs))
	for _, seedID := range seedIDs {
		builder := builders[seedID]
		sort.Strings(builder.info.Tasks)
		builder.info.PlantPrimitives = sortedStringSet(builder.plantPrimitives)
		builder.info.LifecycleOperations = sortedStringSet(builder.lifecycleOperations)
		builder.info.ActivationKinds = sortedStringSet(builder.activationKinds)
		builder.info.OracleKinds = sortedStringSet(builder.oracleKinds)
		out = append(out, builder.info)
	}
	return out
}

func TargetScenarioSeedByID(seedID string) (TargetScenarioSeedInfo, bool) {
	seedID = strings.TrimSpace(seedID)
	for _, seed := range TargetScenarioSeeds() {
		if seed.SeedID == seedID {
			return seed, true
		}
	}
	return TargetScenarioSeedInfo{}, false
}

func ExpandTargetTasks(taskIDs, groupIDs []string) ([]string, []string, error) {
	tasks, groups, _, err := ExpandTargetSelection(taskIDs, groupIDs, nil)
	return tasks, groups, err
}

func ExpandTargetSelection(taskIDs, groupIDs, seedIDs []string) ([]string, []string, []string, error) {
	var expanded []string
	var normalizedGroups []string
	var normalizedSeeds []string
	seenTasks := make(map[string]struct{})
	seenGroups := make(map[string]struct{})
	seenSeeds := make(map[string]struct{})
	groupCatalog := make(map[string]TargetTaskGroupInfo, len(TargetTaskGroups()))
	for _, group := range TargetTaskGroups() {
		groupCatalog[group.GroupID] = group
	}
	seedCatalog := make(map[string]TargetScenarioSeedInfo, len(TargetScenarioSeeds()))
	for _, seed := range TargetScenarioSeeds() {
		seedCatalog[seed.SeedID] = seed
	}

	appendTask := func(taskID string) {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			return
		}
		if _, ok := seenTasks[taskID]; ok {
			return
		}
		seenTasks[taskID] = struct{}{}
		expanded = append(expanded, taskID)
	}

	for _, groupID := range groupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		group, ok := groupCatalog[groupID]
		if !ok {
			return nil, nil, nil, fmt.Errorf("unknown target task group %q", groupID)
		}
		if _, ok := seenGroups[groupID]; !ok {
			seenGroups[groupID] = struct{}{}
			normalizedGroups = append(normalizedGroups, groupID)
		}
		for _, taskID := range group.Tasks {
			appendTask(taskID)
		}
	}

	for _, seedID := range seedIDs {
		seedID = strings.TrimSpace(seedID)
		if seedID == "" {
			continue
		}
		seed, ok := seedCatalog[seedID]
		if !ok {
			return nil, nil, nil, fmt.Errorf("unknown target scenario seed %q", seedID)
		}
		if _, ok := seenSeeds[seedID]; !ok {
			seenSeeds[seedID] = struct{}{}
			normalizedSeeds = append(normalizedSeeds, seedID)
		}
		for _, taskID := range seed.Tasks {
			appendTask(taskID)
		}
	}

	for _, taskID := range taskIDs {
		appendTask(taskID)
	}

	if len(expanded) == 0 {
		return []string{DefaultTargetTaskID}, normalizedGroups, normalizedSeeds, nil
	}
	return expanded, normalizedGroups, normalizedSeeds, nil
}

func TargetSignature(taskID string) core.MismatchSignature {
	return core.MismatchSignature{
		LifecycleEvent: "real-target-run",
		FaultPhase:     "target-command",
		StateClass:     "workspace",
		Operation:      taskID,
		Relation:       "observation-only",
		Impact:         "target-adapter",
	}
}

func targetSeedDescription(seedID string) string {
	switch seedID {
	case "delayed-effect":
		return "delayed background effect seed with boundary and late-observation variants"
	case "shell-path-residue":
		return "persistent shell PATH residue seed with same-run, replay, and fork lifecycle variants"
	case "shell-execution-context-residue":
		return "persistent shell execution-context residue seed covering same-run cwd, env, function, and umask carry-over across later shell calls"
	case "workspace-object-residue-fork":
		return "workspace object residue seed expanded through primitive substitution across fork observation"
	case "capability-residue-fork":
		return "resource capability residue seed covering open-fd and inherited-fd fork variants"
	case "active-ipc-residue-fork":
		return "active IPC residue seed for discarded-branch Unix listener reachability, trusted-client consumption, and response caching"
	case "shell-execution-context-residue-fork":
		return "shell execution-context residue seed covering discarded-branch cwd and umask state across fork observation"
	default:
		return "built-in target scenario seed"
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
