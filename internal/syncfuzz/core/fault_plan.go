package core

import (
	"fmt"
	"strings"
)

const FaultPlanArtifact = "fault-plan.json"

type FaultPhase string

const (
	FaultPhaseP0 FaultPhase = "P0"
	FaultPhaseP1 FaultPhase = "P1"
	FaultPhaseP2 FaultPhase = "P2"
	FaultPhaseP3 FaultPhase = "P3"
	FaultPhaseP4 FaultPhase = "P4"
	FaultPhaseP5 FaultPhase = "P5"
	FaultPhaseP6 FaultPhase = "P6"
	FaultPhaseP7 FaultPhase = "P7"
	FaultPhaseP8 FaultPhase = "P8"
)

type FaultPlan struct {
	SchemaVersion  string      `json:"schema_version"`
	ID             string      `json:"id"`
	CaseName       string      `json:"case_name"`
	Kind           string      `json:"kind"`
	Lifecycle      string      `json:"lifecycle"`
	InjectPhase    FaultPhase  `json:"inject_phase"`
	Fault          string      `json:"fault"`
	Trigger        string      `json:"trigger"`
	StateLayers    []string    `json:"state_layers"`
	ExpectedImpact string      `json:"expected_impact"`
	Timing         FaultTiming `json:"timing,omitempty"`
	Description    string      `json:"description"`
	Deterministic  bool        `json:"deterministic"`
}

func FaultPlans() []FaultPlan {
	return []FaultPlan{
		{
			ID:             "orphan-process/p5-delayed-child",
			CaseName:       "orphan-process",
			Kind:           "known-answer-fault",
			Lifecycle:      "cancel-recover",
			InjectPhase:    FaultPhaseP5,
			Fault:          "command_return_boundary_before_detached_child_settles",
			Trigger:        "treat shell command return as agent lifecycle boundary while a delayed child process can still write",
			StateLayers:    []string{"agent", "os"},
			ExpectedImpact: "rollback-residue",
			Description:    "Detect a delayed filesystem effect that appears after the agent-visible command return.",
			Deterministic:  true,
		},
		{
			ID:             "action-replay/p5-dropped-receipt",
			CaseName:       "action-replay",
			Kind:           "known-answer-fault",
			Lifecycle:      "replay",
			InjectPhase:    FaultPhaseP5,
			Fault:          "drop_tool_result_after_external_commit",
			Trigger:        "lose the receipt for a committed external effect and replay with a new request id",
			StateLayers:    []string{"agent", "external"},
			ExpectedImpact: "forgotten-external-effect",
			Description:    "Detect duplicated external resources when replay forgets a committed effect.",
			Deterministic:  true,
		},
		{
			ID:             "authority-resurrection/p6-stale-authority",
			CaseName:       "authority-resurrection",
			Kind:           "known-answer-fault",
			Lifecycle:      "replay",
			InjectPhase:    FaultPhaseP6,
			Fault:          "restore_checkpoint_before_authority_consume",
			Trigger:        "restore agent state that treats a consumed single-use token as unused",
			StateLayers:    []string{"agent", "authority"},
			ExpectedImpact: "authority-resurrection",
			Description:    "Detect stale capability reuse after authority state has advanced independently.",
			Deterministic:  true,
		},
		{
			ID:             "persistent-shell-poisoning/p6-reused-shell",
			CaseName:       "persistent-shell-poisoning",
			Kind:           "known-answer-fault",
			Lifecycle:      "replay",
			InjectPhase:    FaultPhaseP6,
			Fault:          "restore_agent_graph_without_restarting_shell",
			Trigger:        "reuse a persistent shell after graph state is restored",
			StateLayers:    []string{"agent", "os"},
			ExpectedImpact: "shell-state-residue",
			Description:    "Detect PATH, cwd, and alias residue in a reused shell process.",
			Deterministic:  true,
		},
		{
			ID:             "partial-filesystem-rollback/p6-naive-restore",
			CaseName:       "partial-filesystem-rollback",
			Kind:           "known-answer-fault",
			Lifecycle:      "rollback",
			InjectPhase:    FaultPhaseP6,
			Fault:          "naive_tracked_content_rollback",
			Trigger:        "restore tracked file bytes while leaving untracked objects and metadata drift",
			StateLayers:    []string{"agent", "os"},
			ExpectedImpact: "partial-filesystem-rollback",
			Description:    "Detect filesystem state classes missed by content-only rollback.",
			Deterministic:  true,
		},
		{
			ID:             "branch-leakage/p6-discard-without-restore",
			CaseName:       "branch-leakage",
			Kind:           "known-answer-fault",
			Lifecycle:      "fork-discard",
			InjectPhase:    FaultPhaseP6,
			Fault:          "discard_branch_without_workspace_restore",
			Trigger:        "discard speculative branch metadata without restoring the underlying workspace",
			StateLayers:    []string{"agent", "os"},
			ExpectedImpact: "branch-leakage",
			Description:    "Detect effects from a discarded branch leaking into the committed branch.",
			Deterministic:  true,
		},
	}
}

func ResolveFaultPlan(caseName string, planID string) (FaultPlan, error) {
	planID = strings.TrimSpace(planID)
	for _, plan := range FaultPlans() {
		if planID == "" {
			if plan.CaseName == caseName {
				return withFaultPlanSchema(plan), nil
			}
			continue
		}
		if plan.ID == planID {
			if plan.CaseName != caseName {
				return FaultPlan{}, fmt.Errorf("fault plan %q belongs to case %q, not %q", planID, plan.CaseName, caseName)
			}
			return withFaultPlanSchema(plan), nil
		}
	}
	if planID != "" {
		return FaultPlan{}, fmt.Errorf("unknown fault plan %q", planID)
	}
	return FaultPlan{}, fmt.Errorf("no default fault plan for case %q", caseName)
}

func withFaultPlanSchema(plan FaultPlan) FaultPlan {
	plan.SchemaVersion = "syncfuzz.fault-plan.v1"
	return plan
}
