// Package observation derives deterministic, query-specific state-probe plans
// from Scenario IR and normalized run artifacts. It deliberately does not
// depend on an eBPF implementation: a future tracer can contribute the same
// footprint evidence without changing the plan schema.
package observation

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ResourceFootprintSchemaVersion = "syncfuzz.resource-footprint.v1"
	ObservationPlanSchemaVersion   = "syncfuzz.observation-plan.v1"
	ResourceFootprintArtifact      = "resource-footprint.json"
	ObservationPlanArtifact        = "observation-plan.json"
	ObservationRefinedPlanArtifact = "observation-plan-refined.json"
)

// ResourceClass identifies an artifact-visible state family in the A–O
// recovery-consistency model. The first plan compiler focuses on N, Pi, and
// H; E remains covered by the existing external-effect artifact contract.
type ResourceClass string

const (
	ResourceFilesystemNamespace ResourceClass = "filesystem-namespace"
	ResourceProcess             ResourceClass = "process"
	ResourceFileDescriptor      ResourceClass = "file-descriptor"
	ResourceUnixSocket          ResourceClass = "unix-socket"
	ResourceExecutionContext    ResourceClass = "execution-context"
)

// EvidenceSource records why a resource entered a footprint. This keeps IR
// declarations distinct from facts observed in a concrete run.
type EvidenceSource string

const (
	EvidenceScenarioIR     EvidenceSource = "scenario-ir"
	EvidenceFilesystem     EvidenceSource = "filesystem-snapshot"
	EvidenceProcessLineage EvidenceSource = "process-lineage"
	EvidenceStateTrace     EvidenceSource = "state-trace"
	EvidenceFutureOSTracer EvidenceSource = "os-trace"
)

type FootprintEvidence struct {
	Source   EvidenceSource `json:"source"`
	Artifact string         `json:"artifact,omitempty"`
	Phase    string         `json:"phase,omitempty"`
	Detail   string         `json:"detail,omitempty"`
}

type PathFootprint struct {
	Path          string              `json:"path"`
	ResourceClass ResourceClass       `json:"resource_class"`
	Operations    []string            `json:"operations,omitempty"`
	OriginPhase   string              `json:"origin_phase,omitempty"`
	Evidence      []FootprintEvidence `json:"evidence,omitempty"`
}

// ProcessFootprint intentionally avoids PID as its stable identity. Selection
// is based on a process name/cmdline fingerprint and the lifecycle phase in
// which it was observed, so a plan is reusable across runs.
type ProcessFootprint struct {
	Executable  string              `json:"executable,omitempty"`
	CommandLine string              `json:"command_line,omitempty"`
	OriginPhase string              `json:"origin_phase,omitempty"`
	EffectPaths []string            `json:"effect_paths,omitempty"`
	Evidence    []FootprintEvidence `json:"evidence,omitempty"`
}

type ResourceFootprint struct {
	SchemaVersion   string              `json:"schema_version"`
	QueryID         string              `json:"query_id"`
	Query           *LifecycleQuery     `json:"query,omitempty"`
	ScenarioID      string              `json:"scenario_id,omitempty"`
	TaskID          string              `json:"task_id,omitempty"`
	ResourceClasses []ResourceClass     `json:"resource_classes"`
	Paths           []PathFootprint     `json:"paths,omitempty"`
	Processes       []ProcessFootprint  `json:"processes,omitempty"`
	Operations      []string            `json:"operations,omitempty"`
	Evidence        []FootprintEvidence `json:"evidence,omitempty"`
}

// ProbeFamily is a user-space state-probe family. eBPF remains an evidence
// producer, not a per-query dynamically loaded probe family.
type ProbeFamily string

const (
	ProbeFilesystem   ProbeFamily = "filesystem"
	ProbeProcess      ProbeFamily = "process"
	ProbeFD           ProbeFamily = "file-descriptor"
	ProbeUnixSocket   ProbeFamily = "unix-socket"
	ProbeShellContext ProbeFamily = "shell-context"
)

type ObservationPoint string

const (
	ObservationBeforePlant     ObservationPoint = "before-plant"
	ObservationAfterPlant      ObservationPoint = "after-plant"
	ObservationAfterRecovery   ObservationPoint = "after-recovery"
	ObservationAfterActivation ObservationPoint = "after-activation"
)

type ProbePlan struct {
	Family           ProbeFamily        `json:"family"`
	Enabled          bool               `json:"enabled"`
	Paths            []string           `json:"paths,omitempty"`
	ProcessSelectors []ProcessFootprint `json:"process_selectors,omitempty"`
	Fields           []string           `json:"fields"`
}

// ObservationPlan is an executable contract for the next targeted-probe
// milestone. V1 is emitted offline; the runner will consume it in the next
// increment, while full-probe fallback stays mandatory.
type ObservationPlan struct {
	SchemaVersion           string             `json:"schema_version"`
	QueryID                 string             `json:"query_id"`
	Query                   *LifecycleQuery    `json:"query,omitempty"`
	SourceFootprintArtifact string             `json:"source_footprint_artifact,omitempty"`
	Checkpoints             []ObservationPoint `json:"checkpoints"`
	ProbePlans              []ProbePlan        `json:"probe_plans"`
	FallbackFullProbe       bool               `json:"fallback_full_probe"`
	UnplannedResourcePolicy string             `json:"unplanned_resource_policy"`
	ExpansionCount          int                `json:"expansion_count,omitempty"`
	LastExpansionSource     string             `json:"last_expansion_source,omitempty"`
	LastExpansionPaths      []string           `json:"last_expansion_paths,omitempty"`
}

func NormalizeFootprint(footprint *ResourceFootprint) error {
	if footprint == nil {
		return fmt.Errorf("resource footprint is required")
	}
	if footprint.SchemaVersion == "" {
		footprint.SchemaVersion = ResourceFootprintSchemaVersion
	}
	if footprint.SchemaVersion != ResourceFootprintSchemaVersion {
		return fmt.Errorf("unsupported resource footprint schema %q", footprint.SchemaVersion)
	}
	footprint.QueryID = strings.TrimSpace(footprint.QueryID)
	if footprint.QueryID == "" {
		return fmt.Errorf("resource footprint query_id is required")
	}
	if footprint.Query != nil {
		if err := NormalizeLifecycleQuery(footprint.Query); err != nil {
			return err
		}
		if footprint.Query.QueryID != footprint.QueryID {
			return fmt.Errorf("resource footprint query id %q does not match lifecycle query id %q", footprint.QueryID, footprint.Query.QueryID)
		}
	}
	footprint.ScenarioID = strings.TrimSpace(footprint.ScenarioID)
	footprint.TaskID = strings.TrimSpace(footprint.TaskID)
	footprint.ResourceClasses = uniqueSortedResourceClasses(footprint.ResourceClasses)
	footprint.Operations = uniqueSortedStrings(footprint.Operations)
	footprint.Evidence = uniqueSortedEvidence(footprint.Evidence)
	for i := range footprint.Paths {
		footprint.Paths[i].Path = strings.TrimSpace(footprint.Paths[i].Path)
		footprint.Paths[i].Operations = uniqueSortedStrings(footprint.Paths[i].Operations)
		footprint.Paths[i].OriginPhase = strings.TrimSpace(footprint.Paths[i].OriginPhase)
		footprint.Paths[i].Evidence = uniqueSortedEvidence(footprint.Paths[i].Evidence)
	}
	footprint.Paths = uniqueSortedPaths(footprint.Paths)
	for i := range footprint.Processes {
		process := &footprint.Processes[i]
		process.Executable = strings.TrimSpace(process.Executable)
		process.CommandLine = strings.TrimSpace(process.CommandLine)
		process.OriginPhase = strings.TrimSpace(process.OriginPhase)
		process.EffectPaths = uniqueSortedStrings(process.EffectPaths)
		process.Evidence = uniqueSortedEvidence(process.Evidence)
	}
	footprint.Processes = uniqueSortedProcesses(footprint.Processes)
	return nil
}

func uniqueSortedResourceClasses(values []ResourceClass) []ResourceClass {
	seen := make(map[ResourceClass]struct{}, len(values))
	for _, value := range values {
		value = ResourceClass(strings.TrimSpace(string(value)))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]ResourceClass, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueSortedEvidence(values []FootprintEvidence) []FootprintEvidence {
	byKey := make(map[string]FootprintEvidence, len(values))
	for _, value := range values {
		value.Source = EvidenceSource(strings.TrimSpace(string(value.Source)))
		value.Artifact = strings.TrimSpace(value.Artifact)
		value.Phase = strings.TrimSpace(value.Phase)
		value.Detail = strings.TrimSpace(value.Detail)
		if value.Source == "" {
			continue
		}
		key := string(value.Source) + "\x00" + value.Artifact + "\x00" + value.Phase + "\x00" + value.Detail
		byKey[key] = value
	}
	out := make([]FootprintEvidence, 0, len(byKey))
	for _, value := range byKey {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		if out[i].Phase != out[j].Phase {
			return out[i].Phase < out[j].Phase
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}

func uniqueSortedPaths(values []PathFootprint) []PathFootprint {
	byKey := make(map[string]PathFootprint, len(values))
	for _, value := range values {
		if value.Path == "" || value.ResourceClass == "" {
			continue
		}
		key := string(value.ResourceClass) + "\x00" + value.Path
		existing, ok := byKey[key]
		if ok {
			existing.Operations = uniqueSortedStrings(append(existing.Operations, value.Operations...))
			existing.Evidence = uniqueSortedEvidence(append(existing.Evidence, value.Evidence...))
			if existing.OriginPhase == "" {
				existing.OriginPhase = value.OriginPhase
			}
			byKey[key] = existing
			continue
		}
		byKey[key] = value
	}
	out := make([]PathFootprint, 0, len(byKey))
	for _, value := range byKey {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].ResourceClass < out[j].ResourceClass
	})
	return out
}

func uniqueSortedProcesses(values []ProcessFootprint) []ProcessFootprint {
	byKey := make(map[string]ProcessFootprint, len(values))
	for _, value := range values {
		key := value.Executable + "\x00" + value.CommandLine + "\x00" + value.OriginPhase
		if key == "\x00\x00" {
			continue
		}
		existing, ok := byKey[key]
		if ok {
			existing.EffectPaths = uniqueSortedStrings(append(existing.EffectPaths, value.EffectPaths...))
			existing.Evidence = uniqueSortedEvidence(append(existing.Evidence, value.Evidence...))
			byKey[key] = existing
			continue
		}
		byKey[key] = value
	}
	out := make([]ProcessFootprint, 0, len(byKey))
	for _, value := range byKey {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Executable != out[j].Executable {
			return out[i].Executable < out[j].Executable
		}
		if out[i].CommandLine != out[j].CommandLine {
			return out[i].CommandLine < out[j].CommandLine
		}
		return out[i].OriginPhase < out[j].OriginPhase
	})
	return out
}
