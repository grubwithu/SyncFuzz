package scheduler

import (
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

type TargetMinimizationPlan struct {
	SchemaVersion string                   `json:"schema_version"`
	Applicable    bool                     `json:"applicable"`
	Reason        string                   `json:"reason,omitempty"`
	Strategy      string                   `json:"strategy,omitempty"`
	Steps         []TargetMinimizationStep `json:"steps,omitempty"`
	Preserve      []string                 `json:"preserve,omitempty"`
}

type TargetMinimizationStep struct {
	Order         int                                `json:"order"`
	StepID        string                             `json:"step_id"`
	Kind          string                             `json:"kind"`
	ComponentID   string                             `json:"component_id,omitempty"`
	ComponentKind string                             `json:"component_kind_id,omitempty"`
	ComponentRole target.TargetScenarioComponentRole `json:"component_role,omitempty"`
	MutationKind  target.TargetScenarioMutationKind  `json:"mutation_kind,omitempty"`
	Summary       string                             `json:"summary"`
	Rationale     string                             `json:"rationale,omitempty"`
}

func buildTargetMinimizationPlan(candidate TargetScheduleCandidate, item TargetSuiteRunResult, observation corpus.TargetObservationDetails) *TargetMinimizationPlan {
	plan := &TargetMinimizationPlan{
		SchemaVersion: "syncfuzz.target-minimization-plan.v1",
		Preserve: []string{
			"target_id=" + strings.TrimSpace(item.TargetID),
			"task_id=" + strings.TrimSpace(item.TaskID),
			"oracle=" + strings.TrimSpace(item.OracleKindID),
		},
	}
	if !item.Confirmed {
		plan.Applicable = false
		plan.Reason = "target run was not confirmed"
		return plan
	}
	if observation.Category == corpus.TargetObservationExecutionNotReached {
		plan.Applicable = false
		plan.Reason = "activation was not reached"
		return plan
	}

	plan.Applicable = true
	plan.Strategy = "delete-or-simplify-one-scenario-component-at-a-time while preserving oracle status, attribution, and required artifacts"
	plan.Preserve = append(plan.Preserve,
		"outcome="+string(observation.Category),
		"attribution="+strings.TrimSpace(item.TargetOracle.Attribution),
	)
	for _, artifact := range candidate.DefaultExpectedFiles {
		artifact = strings.TrimSpace(artifact)
		if artifact != "" {
			plan.Preserve = append(plan.Preserve, "artifact="+artifact)
		}
	}

	addStep := func(kind string, role target.TargetScenarioComponentRole, mutationKind target.TargetScenarioMutationKind, summary string, rationale string) {
		plan.Steps = append(plan.Steps, TargetMinimizationStep{
			Order:         len(plan.Steps) + 1,
			StepID:        fmt.Sprintf("m%d", len(plan.Steps)+1),
			Kind:          kind,
			ComponentRole: role,
			MutationKind:  mutationKind,
			Summary:       summary,
			Rationale:     rationale,
		})
	}

	addStep("prompt-reduction", "", "", "remove non-essential prompt wording and keep only the task objective plus required constraints", "checks whether wording style, not scenario semantics, caused the result")

	for _, component := range candidate.Components {
		role := component.Role
		summary := strings.TrimSpace(component.Summary)
		if summary == "" {
			summary = string(role)
		}
		switch string(role) {
		case "setup":
			addStep("component-deletion", role, "", "try deleting setup component: "+summary, "setup should be removable when later components recreate only legitimate prerequisites")
		case "plant":
			addStep("primitive-minimization", role, "", "minimize plant primitive: "+summary, "keeps the state mutation but removes extra writes, helper files, sleeps, and explanatory commands")
		case "lifecycle":
			addStep("lifecycle-tightening", role, "", "tighten lifecycle boundary: "+summary, "preserves the intended checkpoint/replay/fork boundary while removing unrelated turns")
		case "activation":
			addStep("activation-minimization", role, "", "minimize activation step: "+summary, "keeps the smallest command sequence that reaches the observed state")
		case "fault":
			addStep("fault-window-tightening", role, "", "tighten fault component: "+summary, "shrinks the failure timing window while preserving the same causal phase")
		case "oracle":
			addStep("oracle-preservation", role, "", "preserve oracle evidence: "+summary, "keeps only witness files and traces needed to classify the result")
		}
		if len(plan.Steps) > 0 {
			step := &plan.Steps[len(plan.Steps)-1]
			if step.ComponentRole == role {
				step.ComponentID = component.ComponentID
				step.ComponentKind = component.KindID
			}
		}
	}
	if len(candidate.Components) == 0 {
		addStep("artifact-minimization", "", "", "remove generated files that are not expected artifacts, target-task.json, target-result.json, or oracle evidence", "fallback plan for custom tasks without Scenario IR components")
	}
	for _, mutation := range candidate.Mutations {
		if mutation.Kind == "" {
			continue
		}
		summary := strings.TrimSpace(mutation.Summary)
		if summary == "" {
			summary = mutation.MutationID
		}
		addStep("mutation-axis-check", "", mutation.Kind, "toggle mutation axis "+mutation.MutationID+": "+summary, "tests whether this mutation is necessary for the mismatch signature")
	}
	addStep("artifact-replay-check", "", "", "rerun the minimized task through corpus verify or target suite repeat=2", "confirms the minimized case remains reproducible")
	return plan
}
