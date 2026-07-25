package recovery

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

// CausalEffectEvidenceStatus states whether an adapter proved a causal link
// from the profiled effect interval to one durable agent action. It is
// evidence metadata only and never evaluates a framework contract.
type CausalEffectEvidenceStatus string

const (
	CausalEffectEvidenceUnknown CausalEffectEvidenceStatus = "unknown"
	CausalEffectEvidenceProven  CausalEffectEvidenceStatus = "proven"
)

func (s CausalEffectEvidenceStatus) Valid() bool {
	return s == CausalEffectEvidenceUnknown || s == CausalEffectEvidenceProven
}

// CausalEffectEvidence is copied from the immutable recorded adapter plan
// into a recovery relation report. The optional LangGraph field is deliberately
// absent for uncertain or unsupported adapters, rather than inferred from task
// prose or recovery outcomes.
type CausalEffectEvidence struct {
	Status                   CausalEffectEvidenceStatus     `json:"status"`
	AdapterID                string                         `json:"adapter_id"`
	RecordedPlanID           string                         `json:"recorded_plan_id"`
	LangGraphToolEffectProof *LangGraphToolEffectProvenance `json:"langgraph_tool_effect_provenance,omitempty"`
}

func (e CausalEffectEvidence) Validate() error {
	if !e.Status.Valid() || strings.TrimSpace(e.AdapterID) == "" || strings.TrimSpace(e.RecordedPlanID) == "" {
		return fmt.Errorf("causal effect evidence requires status, adapter, and recorded plan identity")
	}
	if e.Status == CausalEffectEvidenceUnknown {
		if e.LangGraphToolEffectProof != nil {
			return fmt.Errorf("unknown causal effect evidence must not carry a tool-effect proof")
		}
		return nil
	}
	if e.AdapterID != LangGraphForkAdapterID || e.LangGraphToolEffectProof == nil {
		return fmt.Errorf("proven causal effect evidence requires a LangGraph tool-effect proof")
	}
	return e.LangGraphToolEffectProof.Validate()
}

func (e CausalEffectEvidence) ValidateFor(seed objective.StateSeed) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.AdapterID != seed.AdapterID || e.RecordedPlanID != seed.RecordedPlanID {
		return fmt.Errorf("causal effect evidence does not match state seed %q", seed.SeedID)
	}
	return nil
}

func unknownCausalEffectEvidence(plan RecordedPlan) CausalEffectEvidence {
	return CausalEffectEvidence{
		Status:         CausalEffectEvidenceUnknown,
		AdapterID:      plan.AdapterID,
		RecordedPlanID: plan.RecordedPlanID,
	}
}

// CausalEffectEvidenceForRecordedPlan reads only adapter-produced immutable
// plan evidence. Missing legacy plan artifacts remain unknown so offline
// relation classification stays usable; malformed present plans are rejected.
func CausalEffectEvidenceForRecordedPlan(plan RecordedPlan) (CausalEffectEvidence, error) {
	if strings.TrimSpace(plan.AdapterID) == "" || strings.TrimSpace(plan.RecordedPlanID) == "" || strings.TrimSpace(plan.ExecutionArtifact) == "" {
		return CausalEffectEvidence{}, fmt.Errorf("recorded plan requires adapter, ID, and execution artifact for causal evidence")
	}
	evidence := unknownCausalEffectEvidence(plan)
	if plan.AdapterID != LangGraphForkAdapterID {
		return evidence, nil
	}
	forkPlan, err := ReadLangGraphForkPlan(plan.ExecutionArtifact)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return evidence, nil
		}
		return CausalEffectEvidence{}, err
	}
	if err := forkPlan.ValidateFor(plan); err != nil {
		return CausalEffectEvidence{}, err
	}
	if forkPlan.ToolEffectProvenance == nil {
		return evidence, nil
	}
	proof := *forkPlan.ToolEffectProvenance
	evidence.Status = CausalEffectEvidenceProven
	evidence.LangGraphToolEffectProof = &proof
	if err := evidence.Validate(); err != nil {
		return CausalEffectEvidence{}, err
	}
	return evidence, nil
}
