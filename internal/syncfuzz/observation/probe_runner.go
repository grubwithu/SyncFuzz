package observation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	TargetedProbeReportSchemaVersion = "syncfuzz.targeted-probe-report.v1"
	TargetedProbeReportArtifact      = "targeted-probe-report.json"
)

// TargetedProbeReport records how a plan selected state from the command
// adapter artifacts. Shadow mode projects retained broad snapshots; pruned
// filesystem mode records the final broad fallback separately.
type TargetedProbeReport struct {
	SchemaVersion                    string                    `json:"schema_version"`
	QueryID                          string                    `json:"query_id"`
	PlanArtifact                     string                    `json:"plan_artifact"`
	CollectionMode                   string                    `json:"collection_mode"`
	FullProbeFallbackRequired        bool                      `json:"full_probe_fallback_required"`
	FullProbeFallbackUsed            bool                      `json:"full_probe_fallback_used"`
	FallbackFilesystemArtifact       string                    `json:"fallback_filesystem_artifact,omitempty"`
	UnplannedFallbackFilesystemPaths []string                  `json:"unplanned_fallback_filesystem_paths,omitempty"`
	Checkpoints                      []TargetedProbeCheckpoint `json:"checkpoints"`
}

type TargetedProbeCheckpoint struct {
	Point       ObservationPoint            `json:"point"`
	RunnerPhase string                      `json:"runner_phase,omitempty"`
	Status      string                      `json:"status"`
	Reason      string                      `json:"reason,omitempty"`
	Families    []TargetedProbeFamilyResult `json:"families,omitempty"`
}

type TargetedProbeFamilyResult struct {
	Family             ProbeFamily         `json:"family"`
	Status             string              `json:"status"`
	RequestedPaths     []string            `json:"requested_paths,omitempty"`
	MissingPaths       []string            `json:"missing_paths,omitempty"`
	MatchedPaths       []core.FileEntry    `json:"matched_paths,omitempty"`
	RequestedSelectors []ProcessFootprint  `json:"requested_selectors,omitempty"`
	MatchedProcesses   []core.ProcessEntry `json:"matched_processes,omitempty"`
	Reason             string              `json:"reason,omitempty"`
}

func NewTargetedProbeReport(plan ObservationPlan, planArtifact string) (*TargetedProbeReport, error) {
	if err := ValidatePlan(&plan); err != nil {
		return nil, err
	}
	return &TargetedProbeReport{
		SchemaVersion:             TargetedProbeReportSchemaVersion,
		QueryID:                   plan.QueryID,
		PlanArtifact:              strings.TrimSpace(planArtifact),
		CollectionMode:            "shadow",
		FullProbeFallbackRequired: plan.FallbackFullProbe,
	}, nil
}

// FilesystemPathsForPlan returns the exact object-level filesystem scope for
// an observation plan. Unix-socket paths are also namespace paths and are
// therefore included in the same selected snapshot.
func FilesystemPathsForPlan(plan ObservationPlan) ([]string, error) {
	if err := ValidatePlan(&plan); err != nil {
		return nil, err
	}
	var paths []string
	for _, probe := range plan.ProbePlans {
		if !probe.Enabled {
			continue
		}
		switch probe.Family {
		case ProbeFilesystem, ProbeUnixSocket:
			paths = append(paths, probe.Paths...)
		}
	}
	return normalizeProbePaths(paths), nil
}

// UnplannedFilesystemPaths reports objects present in a broad fallback
// snapshot but not selected by the plan. It is evidence for later plan
// expansion, not itself an oracle finding.
func UnplannedFilesystemPaths(plan ObservationPlan, snapshot core.Snapshot) ([]string, error) {
	paths, err := FilesystemPathsForPlan(plan)
	if err != nil {
		return nil, err
	}
	planned := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		planned[path] = struct{}{}
	}
	unplanned := make([]string, 0)
	for _, entry := range snapshot.Files {
		if _, ok := planned[entry.Path]; !ok {
			unplanned = append(unplanned, entry.Path)
		}
	}
	return uniqueSortedStrings(unplanned), nil
}

// CaptureTargetedProbeCheckpoint projects one broad snapshot pair through a
// plan. A nil snapshot indicates that the command adapter cannot expose that
// semantic checkpoint yet; that limitation is emitted as artifact evidence.
func CaptureTargetedProbeCheckpoint(plan ObservationPlan, point ObservationPoint, runnerPhase string, filesystem *core.Snapshot, processes *core.ProcessSnapshot, reason string) (TargetedProbeCheckpoint, error) {
	if err := ValidatePlan(&plan); err != nil {
		return TargetedProbeCheckpoint{}, err
	}
	checkpoint := TargetedProbeCheckpoint{
		Point:       point,
		RunnerPhase: strings.TrimSpace(runnerPhase),
		Status:      "observed",
		Reason:      strings.TrimSpace(reason),
	}
	for _, probe := range plan.ProbePlans {
		if !probe.Enabled {
			continue
		}
		result := TargetedProbeFamilyResult{Family: probe.Family, Status: "observed"}
		switch probe.Family {
		case ProbeFilesystem, ProbeUnixSocket:
			result.RequestedPaths = normalizeProbePaths(probe.Paths)
			if filesystem == nil {
				result.Status = "unavailable"
				result.Reason = "no filesystem artifact at this adapter-visible checkpoint"
				checkpoint.Status = "partial"
			} else {
				result.MatchedPaths, result.MissingPaths = matchProbePaths(*filesystem, result.RequestedPaths, probe.Family)
			}
		case ProbeProcess, ProbeFD:
			result.RequestedSelectors = append([]ProcessFootprint{}, probe.ProcessSelectors...)
			if processes == nil {
				result.Status = "unavailable"
				result.Reason = "no process artifact at this adapter-visible checkpoint"
				checkpoint.Status = "partial"
			} else {
				result.MatchedProcesses = matchProbeProcesses(*processes, probe.ProcessSelectors)
			}
		case ProbeShellContext:
			result.Status = "unavailable"
			result.Reason = "the command adapter has no normalized shell-context snapshot"
			checkpoint.Status = "partial"
		default:
			result.Status = "unavailable"
			result.Reason = "unsupported probe family"
			checkpoint.Status = "partial"
		}
		checkpoint.Families = append(checkpoint.Families, result)
	}
	if len(checkpoint.Families) == 0 {
		checkpoint.Status = "unavailable"
		if checkpoint.Reason == "" {
			checkpoint.Reason = "observation plan has no enabled probe family"
		}
	}
	return checkpoint, nil
}

func (report *TargetedProbeReport) AddCheckpoint(checkpoint TargetedProbeCheckpoint) error {
	if report == nil {
		return fmt.Errorf("targeted probe report is required")
	}
	if report.SchemaVersion == "" {
		report.SchemaVersion = TargetedProbeReportSchemaVersion
	}
	if report.SchemaVersion != TargetedProbeReportSchemaVersion {
		return fmt.Errorf("unsupported targeted probe report schema %q", report.SchemaVersion)
	}
	if report.QueryID == "" {
		return fmt.Errorf("targeted probe report query_id is required")
	}
	report.Checkpoints = append(report.Checkpoints, checkpoint)
	return nil
}

func normalizeProbePaths(paths []string) []string {
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if path == "." || path == "" || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			continue
		}
		values = append(values, path)
	}
	return uniqueSortedStrings(values)
}

func matchProbePaths(snapshot core.Snapshot, requested []string, family ProbeFamily) ([]core.FileEntry, []string) {
	byPath := snapshot.Paths()
	matched := make([]core.FileEntry, 0, len(requested))
	missing := make([]string, 0)
	for _, path := range requested {
		entry, exists := byPath[path]
		if !exists || (family == ProbeUnixSocket && entry.Type != "socket") {
			missing = append(missing, path)
			continue
		}
		matched = append(matched, entry)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Path < matched[j].Path })
	sort.Strings(missing)
	return matched, missing
}

func matchProbeProcesses(snapshot core.ProcessSnapshot, selectors []ProcessFootprint) []core.ProcessEntry {
	matched := make([]core.ProcessEntry, 0)
	for _, process := range snapshot.Processes {
		if !process.WorkspaceRelated {
			continue
		}
		if len(selectors) == 0 || matchesAnyProcessSelector(process, selectors) {
			matched = append(matched, process)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].PID != matched[j].PID {
			return matched[i].PID < matched[j].PID
		}
		return matched[i].RawCmdline < matched[j].RawCmdline
	})
	return matched
}

func matchesAnyProcessSelector(process core.ProcessEntry, selectors []ProcessFootprint) bool {
	for _, selector := range selectors {
		if selector.Executable != "" && selector.Executable != process.Name {
			continue
		}
		if selector.CommandLine != "" && selector.CommandLine != process.RawCmdline {
			continue
		}
		return true
	}
	return false
}
