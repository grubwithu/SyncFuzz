// Package synthesis owns V2.4's objective-driven task generation contract.
// It intentionally has no dependency on legacy target scenarios, prompt
// variants, mutation focus, or query genealogy.
package synthesis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const SchemaVersion = "syncfuzz.synthesis.v1"

// AtomFeedback is execution-derived feedback for a follow-up generation
// attempt. It is deliberately expressed in the bounded effect grammar rather
// than in terms of a prior prompt, mutation, or query parent.
type AtomFeedback struct {
	Family    profiling.StateFamily `json:"family"`
	Operation string                `json:"operation"`
	Observed  bool                  `json:"observed"`
	Reason    string                `json:"reason,omitempty"`
}

func (f AtomFeedback) Validate() error {
	if !f.Family.Valid() || strings.TrimSpace(f.Operation) == "" {
		return fmt.Errorf("feedback atom requires a valid family and operation")
	}
	return nil
}

// GeneratorRequest is the complete, bounded input supplied to a task
// generator. ScaffoldArtifact points to a target-owned project scaffold; the
// scheduler never turns a historical testcase into a candidate.
type GeneratorRequest struct {
	SchemaVersion    string                   `json:"schema_version"`
	RequestID        string                   `json:"request_id"`
	Objective        objective.StateObjective `json:"objective"`
	TargetID         string                   `json:"target_id"`
	AdapterID        string                   `json:"adapter_id"`
	ScaffoldArtifact string                   `json:"scaffold_artifact"`
	Attempt          int                      `json:"attempt"`
	Feedback         []AtomFeedback           `json:"feedback,omitempty"`
}

func NewGeneratorRequest(stateObjective objective.StateObjective, targetID string, adapterID string, scaffoldArtifact string, attempt int, feedback []AtomFeedback) (GeneratorRequest, error) {
	request := GeneratorRequest{
		SchemaVersion:    SchemaVersion,
		Objective:        stateObjective,
		TargetID:         strings.TrimSpace(targetID),
		AdapterID:        strings.TrimSpace(adapterID),
		ScaffoldArtifact: strings.TrimSpace(scaffoldArtifact),
		Attempt:          attempt,
		Feedback:         append([]AtomFeedback{}, feedback...),
	}
	request.RequestID = requestID(request)
	if err := request.Validate(); err != nil {
		return GeneratorRequest{}, err
	}
	return request, nil
}

func (r GeneratorRequest) Validate() error {
	if r.SchemaVersion != "" && r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis request schema %q", r.SchemaVersion)
	}
	if err := r.Objective.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.TargetID) == "" || strings.TrimSpace(r.AdapterID) == "" || strings.TrimSpace(r.ScaffoldArtifact) == "" || r.Attempt < 0 {
		return fmt.Errorf("synthesis request requires ID, target, adapter, scaffold, and non-negative attempt")
	}
	seen := make(map[string]struct{}, len(r.Feedback))
	for _, feedback := range r.Feedback {
		if err := feedback.Validate(); err != nil {
			return err
		}
		key := effectKey(objective.EffectAtom{Family: feedback.Family, Operation: feedback.Operation})
		if _, ok := seen[key]; ok {
			return fmt.Errorf("synthesis request has duplicate feedback atom %s/%s", feedback.Family, feedback.Operation)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// GeneratorResponse is the only accepted stdout schema for a task generator.
// The scheduler, not the generator, assigns target identity, candidate ID, and
// execution provenance.
type GeneratorResponse struct {
	SchemaVersion string `json:"schema_version"`
	Task          string `json:"task"`
}

func (r GeneratorResponse) Validate() error {
	if r.SchemaVersion != "" && r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis generator response schema %q", r.SchemaVersion)
	}
	if strings.TrimSpace(r.Task) == "" {
		return fmt.Errorf("synthesis generator response requires a natural task")
	}
	return nil
}

// SynthesisCandidate is a generated natural task attempt, not a recovery
// query. It has no mutation focus, prompt variant, parent ID, or testcase
// primitive. Only a later execution can promote it into a StateSeed.
type SynthesisCandidate struct {
	SchemaVersion    string `json:"schema_version"`
	CandidateID      string `json:"candidate_id"`
	ObjectiveID      string `json:"objective_id"`
	TargetID         string `json:"target_id"`
	AdapterID        string `json:"adapter_id"`
	ScaffoldArtifact string `json:"scaffold_artifact"`
	GeneratorID      string `json:"generator_id"`
	Attempt          int    `json:"attempt"`
	Task             string `json:"task"`
}

func NewCandidate(request GeneratorRequest, generatorID string, response GeneratorResponse) (SynthesisCandidate, error) {
	if err := request.Validate(); err != nil {
		return SynthesisCandidate{}, err
	}
	if err := response.Validate(); err != nil {
		return SynthesisCandidate{}, err
	}
	if strings.TrimSpace(generatorID) == "" {
		return SynthesisCandidate{}, fmt.Errorf("synthesis candidate requires a generator ID")
	}
	candidate := SynthesisCandidate{
		SchemaVersion:    SchemaVersion,
		ObjectiveID:      request.Objective.ObjectiveID,
		TargetID:         request.TargetID,
		AdapterID:        request.AdapterID,
		ScaffoldArtifact: request.ScaffoldArtifact,
		GeneratorID:      strings.TrimSpace(generatorID),
		Attempt:          request.Attempt,
		Task:             strings.TrimSpace(response.Task),
	}
	candidate.CandidateID = candidateID(candidate)
	if err := candidate.ValidateFor(request.Objective); err != nil {
		return SynthesisCandidate{}, err
	}
	return candidate, nil
}

func (c SynthesisCandidate) ValidateFor(stateObjective objective.StateObjective) error {
	if c.SchemaVersion != "" && c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis candidate schema %q", c.SchemaVersion)
	}
	if err := stateObjective.Validate(); err != nil {
		return err
	}
	if c.ObjectiveID != stateObjective.ObjectiveID || strings.TrimSpace(c.CandidateID) == "" || strings.TrimSpace(c.TargetID) == "" || strings.TrimSpace(c.AdapterID) == "" || strings.TrimSpace(c.ScaffoldArtifact) == "" || strings.TrimSpace(c.GeneratorID) == "" || strings.TrimSpace(c.Task) == "" || c.Attempt < 0 {
		return fmt.Errorf("synthesis candidate does not match objective %q or lacks execution identity", stateObjective.ObjectiveID)
	}
	if c.CandidateID != candidateID(c) {
		return fmt.Errorf("synthesis candidate %q has a non-canonical ID", c.CandidateID)
	}
	return nil
}

func requestID(request GeneratorRequest) string {
	values := []string{request.Objective.ObjectiveID, request.TargetID, request.AdapterID, request.ScaffoldArtifact, strconv.Itoa(request.Attempt)}
	feedback := append([]AtomFeedback{}, request.Feedback...)
	sort.Slice(feedback, func(i, j int) bool {
		left := effectKey(objective.EffectAtom{Family: feedback[i].Family, Operation: feedback[i].Operation})
		right := effectKey(objective.EffectAtom{Family: feedback[j].Family, Operation: feedback[j].Operation})
		return left < right
	})
	for _, item := range feedback {
		values = append(values, string(item.Family), item.Operation, strconv.FormatBool(item.Observed), item.Reason)
	}
	return "synthesis-request:" + shortDigest(strings.Join(values, "\x00"))
}

func candidateID(candidate SynthesisCandidate) string {
	return "synthesis-candidate:" + shortDigest(strings.Join([]string{
		candidate.ObjectiveID,
		candidate.TargetID,
		candidate.AdapterID,
		candidate.ScaffoldArtifact,
		candidate.GeneratorID,
		strconv.Itoa(candidate.Attempt),
		candidate.Task,
	}, "\x00"))
}

func effectKey(atom objective.EffectAtom) string {
	return string(atom.Family) + "\x00" + atom.Operation
}

func shortDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
