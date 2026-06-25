package syncfuzz

import (
	"path/filepath"
	"sort"
	"time"
)

const (
	agentStateArtifact = "agent-state.json"
	stateTraceArtifact = "state-trace.json"
)

type StateObservation struct {
	Layer       string `json:"layer"`
	StateClass  string `json:"state_class"`
	Phase       string `json:"phase"`
	Artifact    string `json:"artifact"`
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
}

type StateLayerSummary struct {
	Layer        string   `json:"layer"`
	Present      bool     `json:"present"`
	StateClasses []string `json:"state_classes,omitempty"`
	Artifacts    []string `json:"artifacts,omitempty"`
	Phases       []string `json:"phases,omitempty"`
}

type LifecyclePhase struct {
	Phase string `json:"phase"`
	Label string `json:"label"`
}

type AgentStateProjection struct {
	RunID             string            `json:"run_id"`
	CaseName          string            `json:"case_name"`
	Environment       string            `json:"environment"`
	ContainerImage    string            `json:"container_image,omitempty"`
	GeneratedAt       string            `json:"generated_at"`
	Objective         string            `json:"objective"`
	StateClasses      []string          `json:"state_classes"`
	FaultPhases       []string          `json:"fault_phases"`
	Primitives        []string          `json:"primitives"`
	ExpectedSignature MismatchSignature `json:"expected_signature"`
	Confirmed         bool              `json:"confirmed"`
	Evidence          []string          `json:"evidence"`
}

type CrossLayerTrace struct {
	SchemaVersion     string              `json:"schema_version"`
	RunID             string              `json:"run_id"`
	CaseName          string              `json:"case_name"`
	Environment       string              `json:"environment"`
	ContainerImage    string              `json:"container_image,omitempty"`
	GeneratedAt       string              `json:"generated_at"`
	PhaseCatalog      []LifecyclePhase    `json:"phase_catalog"`
	Layers            []StateLayerSummary `json:"layers"`
	Observations      []StateObservation  `json:"observations"`
	ExpectedSignature MismatchSignature   `json:"expected_signature"`
	Confirmed         bool                `json:"confirmed"`
}

func writeCrossLayerArtifacts(run *runContext, manifest CaseManifest, confirmed bool, evidence []string, observations []StateObservation) error {
	generatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	agent := AgentStateProjection{
		RunID:             run.runID,
		CaseName:          run.caseName,
		Environment:       run.environment,
		ContainerImage:    run.containerImage,
		GeneratedAt:       generatedAt,
		Objective:         manifest.Objective,
		StateClasses:      manifest.StateClasses,
		FaultPhases:       manifest.FaultPhases,
		Primitives:        manifest.Primitives,
		ExpectedSignature: manifest.ExpectedSignature,
		Confirmed:         confirmed,
		Evidence:          evidence,
	}
	if err := writeJSON(filepath.Join(run.runDir, agentStateArtifact), agent); err != nil {
		return err
	}

	observations = append([]StateObservation{{
		Layer:       "agent",
		StateClass:  "agent-logical",
		Phase:       "oracle",
		Artifact:    agentStateArtifact,
		Kind:        "agent-state-projection",
		Description: "deterministic projection of the agent-side lifecycle and oracle state",
	}}, observations...)

	trace := buildCrossLayerTrace(run, manifest.ExpectedSignature, confirmed, generatedAt, observations)
	if err := writeJSON(filepath.Join(run.runDir, stateTraceArtifact), trace); err != nil {
		return err
	}
	return run.trace.Write(newEvent(run, "oracle", "cross_layer_trace_written", map[string]any{
		"artifact":          stateTraceArtifact,
		"agent_artifact":    agentStateArtifact,
		"observation_count": len(trace.Observations),
		"layers":            presentLayerNames(trace.Layers),
	}))
}

func buildCrossLayerTrace(run *runContext, signature MismatchSignature, confirmed bool, generatedAt string, observations []StateObservation) CrossLayerTrace {
	sortStateObservations(observations)
	return CrossLayerTrace{
		SchemaVersion:     "syncfuzz.state-trace.v1",
		RunID:             run.runID,
		CaseName:          run.caseName,
		Environment:       run.environment,
		ContainerImage:    run.containerImage,
		GeneratedAt:       generatedAt,
		PhaseCatalog:      lifecyclePhaseCatalog(),
		Layers:            summarizeStateLayers(observations),
		Observations:      observations,
		ExpectedSignature: signature,
		Confirmed:         confirmed,
	}
}

func lifecyclePhaseCatalog() []LifecyclePhase {
	return []LifecyclePhase{
		{Phase: "P0", Label: "before tool intent or testcase setup"},
		{Phase: "P1", Label: "after intent before dispatch"},
		{Phase: "P2", Label: "after shell receives command"},
		{Phase: "P3", Label: "after child process creation"},
		{Phase: "P4", Label: "after first OS, external, or authority effect"},
		{Phase: "P5", Label: "after command finishes before result delivery"},
		{Phase: "P6", Label: "after result delivery before checkpoint persistence"},
		{Phase: "P7", Label: "after checkpoint persistence before acknowledgment"},
		{Phase: "P8", Label: "during replay, resume, or alternate branch commit"},
		{Phase: "oracle", Label: "oracle verdict and artifact finalization"},
	}
}

func summarizeStateLayers(observations []StateObservation) []StateLayerSummary {
	knownLayers := []string{"agent", "os", "external", "authority"}
	byLayer := make(map[string][]StateObservation)
	for _, observation := range observations {
		byLayer[observation.Layer] = append(byLayer[observation.Layer], observation)
	}

	summaries := make([]StateLayerSummary, 0, len(knownLayers))
	for _, layer := range knownLayers {
		layerObservations := byLayer[layer]
		summaries = append(summaries, StateLayerSummary{
			Layer:        layer,
			Present:      len(layerObservations) > 0,
			StateClasses: uniqueObservationValues(layerObservations, func(o StateObservation) string { return o.StateClass }),
			Artifacts:    uniqueObservationValues(layerObservations, func(o StateObservation) string { return o.Artifact }),
			Phases:       uniqueObservationValues(layerObservations, func(o StateObservation) string { return o.Phase }),
		})
	}
	return summaries
}

func presentLayerNames(layers []StateLayerSummary) []string {
	var names []string
	for _, layer := range layers {
		if layer.Present {
			names = append(names, layer.Layer)
		}
	}
	return names
}

func uniqueObservationValues(observations []StateObservation, value func(StateObservation) string) []string {
	seen := make(map[string]struct{})
	for _, observation := range observations {
		item := value(observation)
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		leftRank := phaseRank(out[i])
		rightRank := phaseRank(out[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return out[i] < out[j]
	})
	return out
}

func sortStateObservations(observations []StateObservation) {
	sort.Slice(observations, func(i, j int) bool {
		leftRank := phaseRank(observations[i].Phase)
		rightRank := phaseRank(observations[j].Phase)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if observations[i].Layer != observations[j].Layer {
			return observations[i].Layer < observations[j].Layer
		}
		if observations[i].StateClass != observations[j].StateClass {
			return observations[i].StateClass < observations[j].StateClass
		}
		return observations[i].Artifact < observations[j].Artifact
	})
}

func phaseRank(phase string) int {
	switch phase {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	case "P4":
		return 4
	case "P5":
		return 5
	case "P6":
		return 6
	case "P7":
		return 7
	case "P8":
		return 8
	case "oracle":
		return 99
	default:
		return 50
	}
}

func appendPhase2Artifacts(artifacts []string) []string {
	out := make([]string, 0, len(artifacts)+2)
	inserted := false
	for _, artifact := range artifacts {
		out = appendUniqueStrings(out, artifact)
		if artifact == "trace.jsonl" {
			out = appendUniqueStrings(out, agentStateArtifact, stateTraceArtifact)
			inserted = true
		}
	}
	if !inserted {
		out = appendUniqueStrings(out, agentStateArtifact, stateTraceArtifact)
	}
	return out
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	out := make([]string, 0, len(values)+len(additions))
	for _, value := range append(values, additions...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
