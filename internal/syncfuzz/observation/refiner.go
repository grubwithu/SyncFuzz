package observation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

// RefinePlanFromFallback expands a plan exactly once from target-observable
// objects found by the final broad fallback. It is intentionally conservative:
// a second refinement is rejected so the policy remains
// expand-once-then-full-probe rather than unbounded plan drift.
func RefinePlanFromFallback(plan ObservationPlan, report TargetedProbeReport, fallback core.Snapshot) (*ObservationPlan, error) {
	if err := ValidatePlan(&plan); err != nil {
		return nil, err
	}
	if report.SchemaVersion != "" && report.SchemaVersion != TargetedProbeReportSchemaVersion {
		return nil, fmt.Errorf("unsupported targeted probe report schema %q", report.SchemaVersion)
	}
	if strings.TrimSpace(report.QueryID) == "" {
		return nil, fmt.Errorf("targeted probe report query_id is required")
	}
	if report.QueryID != plan.QueryID {
		return nil, fmt.Errorf("targeted probe report query id %q does not match observation plan query id %q", report.QueryID, plan.QueryID)
	}
	if !report.FullProbeFallbackUsed || strings.TrimSpace(report.FallbackFilesystemArtifact) == "" {
		return nil, fmt.Errorf("targeted probe report has no usable full filesystem fallback")
	}
	if plan.ExpansionCount >= 1 {
		return nil, fmt.Errorf("observation plan %q already used its one permitted fallback expansion", plan.QueryID)
	}

	byPath := fallback.Paths()
	added := make([]string, 0, len(report.UnplannedFallbackFilesystemPaths))
	for _, path := range uniqueSortedStrings(report.UnplannedFallbackFilesystemPaths) {
		entry, exists := byPath[path]
		if !exists {
			continue
		}
		if addFallbackEntryToPlan(&plan, entry) {
			added = append(added, entry.Path)
		}
	}
	if len(added) > 0 {
		plan.ExpansionCount++
		plan.LastExpansionSource = report.FallbackFilesystemArtifact
		plan.LastExpansionPaths = uniqueSortedStrings(added)
	}
	canonicalizePlan(&plan)
	if err := ValidatePlan(&plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func addFallbackEntryToPlan(plan *ObservationPlan, entry core.FileEntry) bool {
	if plan == nil || strings.TrimSpace(entry.Path) == "" {
		return false
	}
	added := addPlanPath(plan, ProbeFilesystem, entry.Path)
	if entry.Type != "socket" {
		return added
	}
	added = addPlanPath(plan, ProbeUnixSocket, entry.Path) || added
	ensurePlanProbe(plan, ProbeProcess)
	ensurePlanProbe(plan, ProbeFD)
	return added
}

func addPlanPath(plan *ObservationPlan, family ProbeFamily, path string) bool {
	probe := ensurePlanProbe(plan, family)
	path = filepathToSlash(path)
	for _, existing := range probe.Paths {
		if existing == path {
			return false
		}
	}
	probe.Paths = append(probe.Paths, path)
	return true
}

func ensurePlanProbe(plan *ObservationPlan, family ProbeFamily) *ProbePlan {
	for i := range plan.ProbePlans {
		if plan.ProbePlans[i].Family == family {
			plan.ProbePlans[i].Enabled = true
			if len(plan.ProbePlans[i].Fields) == 0 {
				plan.ProbePlans[i].Fields = probeFields(family)
			}
			return &plan.ProbePlans[i]
		}
	}
	plan.ProbePlans = append(plan.ProbePlans, ProbePlan{Family: family, Enabled: true, Fields: probeFields(family)})
	return &plan.ProbePlans[len(plan.ProbePlans)-1]
}

func canonicalizePlan(plan *ObservationPlan) {
	if plan == nil {
		return
	}
	for i := range plan.ProbePlans {
		plan.ProbePlans[i].Paths = normalizeProbePaths(plan.ProbePlans[i].Paths)
		plan.ProbePlans[i].ProcessSelectors = uniqueSortedProcesses(plan.ProbePlans[i].ProcessSelectors)
		plan.ProbePlans[i].Fields = uniqueSortedStrings(plan.ProbePlans[i].Fields)
	}
	order := map[ProbeFamily]int{
		ProbeFilesystem:   0,
		ProbeProcess:      1,
		ProbeFD:           2,
		ProbeUnixSocket:   3,
		ProbeShellContext: 4,
	}
	sort.Slice(plan.ProbePlans, func(i, j int) bool {
		return order[plan.ProbePlans[i].Family] < order[plan.ProbePlans[j].Family]
	})
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}
