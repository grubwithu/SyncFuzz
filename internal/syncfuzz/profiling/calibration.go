package profiling

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// CalibrationAuditSchemaVersion identifies the report emitted by the bounded
// V2.2 calibration audit. Its precision and recall are fixture-scoped: they
// measure whether each declared, known-answer link was recovered, not global
// real-world detector precision or state-surface coverage.
const CalibrationAuditSchemaVersion = "syncfuzz.calibration-audit.v1"

type CalibrationKind string

const (
	CalibrationKindCanonicalPath CalibrationKind = "canonical-path"
	CalibrationKindDeletedFD     CalibrationKind = "deleted-fd-identity"
	CalibrationKindUnixSocket    CalibrationKind = "unix-socket-closure"
)

// CalibrationEvidence identifies one observed link used by a fixture-scoped
// calibration result.
type CalibrationEvidence struct {
	EffectID   string               `json:"effect_id"`
	Operation  string               `json:"operation"`
	ResourceID string               `json:"resource_id"`
	Relation   EvidenceLinkRelation `json:"relation"`
}

type CalibrationRunResult struct {
	Kind                CalibrationKind       `json:"kind"`
	RunID               string                `json:"run_id,omitempty"`
	RunDir              string                `json:"run_dir"`
	FrontierID          string                `json:"frontier_id,omitempty"`
	CgroupID            uint64                `json:"cgroup_id,omitempty"`
	ExpectedLinks       int                   `json:"expected_links"`
	ObservedLinks       int                   `json:"observed_links"`
	MatchedLinks        []CalibrationEvidence `json:"matched_links,omitempty"`
	UnexpectedLinks     []CalibrationEvidence `json:"unexpected_links,omitempty"`
	MissingExpectations []string              `json:"missing_expectations,omitempty"`
	ClosureVerified     bool                  `json:"closure_verified,omitempty"`
	Failures            []string              `json:"failures,omitempty"`
	Passed              bool                  `json:"passed"`
}

// CalibrationAuditReport provides a reproducible V2.2 calibration result.
// Precision and recall use only the link candidates selected by each fixture's
// declared oracle, so they cannot be misread as global precision or recall.
type CalibrationAuditReport struct {
	SchemaVersion          string                 `json:"schema_version"`
	Scope                  string                 `json:"scope"`
	ExpectedLinks          int                    `json:"expected_links"`
	ObservedLinks          int                    `json:"observed_links"`
	MatchedLinks           int                    `json:"matched_links"`
	UnexpectedLinks        int                    `json:"unexpected_links"`
	MissingExpectations    int                    `json:"missing_expectations"`
	FixtureScopedPrecision float64                `json:"fixture_scoped_precision"`
	FixtureScopedRecall    float64                `json:"fixture_scoped_recall"`
	Passed                 bool                   `json:"passed"`
	Calibrations           []CalibrationRunResult `json:"calibrations"`
}

type calibrationTargetResult struct {
	RunID       string `json:"run_id"`
	Completed   bool   `json:"completed"`
	Environment string `json:"environment"`
}

type calibrationArtifacts struct {
	runID     string
	cgroupID  uint64
	mapData   CheckpointEffectMap
	catalog   CheckpointCatalog
	summaries map[string]CheckpointStateSummary
}

// AuditStandardCalibrations evaluates the three known-answer V2.2 fixtures:
// lexical workspace path, deleted open-FD identity, and Unix socket closure.
// It consumes immutable run artifacts and never reads scenario mutations,
// prompts, or target genealogy.
func AuditStandardCalibrations(pathRunDir string, fdRunDir string, socketRunDir string) (*CalibrationAuditReport, error) {
	inputs := []struct {
		kind   CalibrationKind
		runDir string
	}{
		{kind: CalibrationKindCanonicalPath, runDir: pathRunDir},
		{kind: CalibrationKindDeletedFD, runDir: fdRunDir},
		{kind: CalibrationKindUnixSocket, runDir: socketRunDir},
	}
	report := &CalibrationAuditReport{
		SchemaVersion: CalibrationAuditSchemaVersion,
		Scope:         "fixture-scoped link precision/recall; not global detector precision/recall or state-surface coverage",
		Calibrations:  make([]CalibrationRunResult, 0, len(inputs)),
	}
	for _, input := range inputs {
		artifacts, err := loadCalibrationArtifacts(input.runDir)
		if err != nil {
			return nil, fmt.Errorf("load %s calibration: %w", input.kind, err)
		}
		var result CalibrationRunResult
		switch input.kind {
		case CalibrationKindCanonicalPath:
			result = auditCanonicalPathCalibration(input.runDir, artifacts)
		case CalibrationKindDeletedFD:
			result = auditDeletedFDCalibration(input.runDir, artifacts)
		case CalibrationKindUnixSocket:
			result = auditUnixSocketCalibration(input.runDir, artifacts)
		default:
			return nil, fmt.Errorf("unsupported calibration kind %q", input.kind)
		}
		report.Calibrations = append(report.Calibrations, result)
		report.ExpectedLinks += result.ExpectedLinks
		report.ObservedLinks += result.ObservedLinks
		report.MatchedLinks += len(result.MatchedLinks)
		report.UnexpectedLinks += len(result.UnexpectedLinks)
		report.MissingExpectations += len(result.MissingExpectations)
	}
	if report.ObservedLinks > 0 {
		report.FixtureScopedPrecision = float64(report.MatchedLinks) / float64(report.ObservedLinks)
	}
	if report.ExpectedLinks > 0 {
		report.FixtureScopedRecall = float64(report.MatchedLinks) / float64(report.ExpectedLinks)
	}
	report.Passed = report.ExpectedLinks > 0 && report.MissingExpectations == 0 && report.UnexpectedLinks == 0
	for _, result := range report.Calibrations {
		if !result.Passed {
			report.Passed = false
		}
	}
	return report, nil
}

func loadCalibrationArtifacts(runDir string) (calibrationArtifacts, error) {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return calibrationArtifacts{}, fmt.Errorf("run directory is required")
	}
	var targetResult calibrationTargetResult
	if err := readJSON(filepath.Join(runDir, "target-result.json"), &targetResult); err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read target result: %w", err)
	}
	if strings.TrimSpace(targetResult.RunID) == "" || !targetResult.Completed || targetResult.Environment != "container" {
		return calibrationArtifacts{}, fmt.Errorf("target result must be a completed container run")
	}
	var processScope ProfilingScope
	if err := readJSON(filepath.Join(runDir, "ebpf-process-scope.json"), &processScope); err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read process scope: %w", err)
	}
	var resourceScope ProfilingScope
	if err := readJSON(filepath.Join(runDir, "ebpf-resource-scope.json"), &resourceScope); err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read resource scope: %w", err)
	}
	if processScope.RunID != targetResult.RunID || resourceScope.RunID != targetResult.RunID || processScope.CgroupID == 0 || resourceScope.CgroupID == 0 || processScope.CgroupID != resourceScope.CgroupID {
		return calibrationArtifacts{}, fmt.Errorf("process/resource scopes do not share the target run and non-zero cgroup identity")
	}
	catalog, err := ReadCheckpointCatalog(filepath.Join(runDir, "checkpoint-catalog.json"))
	if err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read checkpoint catalog: %w", err)
	}
	if catalog.RunID != targetResult.RunID {
		return calibrationArtifacts{}, fmt.Errorf("checkpoint catalog run %q does not match target run %q", catalog.RunID, targetResult.RunID)
	}
	summaries, err := ReadCheckpointStateSummaries(filepath.Join(runDir, "checkpoint-state-summaries.json"))
	if err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read checkpoint summaries: %w", err)
	}
	if err := ValidateCheckpointStateSummaries(catalog, summaries); err != nil {
		return calibrationArtifacts{}, fmt.Errorf("validate checkpoint summaries: %w", err)
	}
	effectMap, err := ReadCheckpointEffectMap(filepath.Join(runDir, "checkpoint-effect-map.json"))
	if err != nil {
		return calibrationArtifacts{}, fmt.Errorf("read checkpoint effect map: %w", err)
	}
	if effectMap.RunID != targetResult.RunID {
		return calibrationArtifacts{}, fmt.Errorf("checkpoint effect map run %q does not match target run %q", effectMap.RunID, targetResult.RunID)
	}
	summaryByID := make(map[string]CheckpointStateSummary, len(summaries))
	for _, summary := range summaries {
		summaryByID[summary.CheckpointID] = summary
	}
	return calibrationArtifacts{runID: targetResult.RunID, cgroupID: resourceScope.CgroupID, mapData: effectMap, catalog: catalog, summaries: summaryByID}, nil
}

func auditCanonicalPathCalibration(runDir string, artifacts calibrationArtifacts) CalibrationRunResult {
	result, interval := newCalibrationResult(CalibrationKindCanonicalPath, runDir, artifacts)
	result.ExpectedLinks = 1
	if interval == nil {
		return result
	}
	candidates := selectCandidates(*interval, func(effect NormalizedEffect, resource ResourceRef, link EvidenceLink) bool {
		return resource.ResourceID == "workspace:frontier-marker" && link.Relation == EvidenceLinkExactCanonicalPath
	})
	matchExpectedCandidates(&result, candidates, []calibrationExpectation{{
		label: "open frontier-marker through exact canonical path",
		matches: func(candidate calibrationCandidate) bool {
			return candidate.link.Relation == EvidenceLinkExactCanonicalPath && candidate.effect.Family == StateFamilyHandle && candidate.effect.Operation == "open" && candidate.resource.CanonicalPath == "/workspace/frontier-marker"
		},
	}})
	finalizeCalibrationResult(&result)
	return result
}

func auditDeletedFDCalibration(runDir string, artifacts calibrationArtifacts) CalibrationRunResult {
	result, interval := newCalibrationResult(CalibrationKindDeletedFD, runDir, artifacts)
	result.ExpectedLinks = 1
	if interval == nil {
		return result
	}
	candidates := selectCandidates(*interval, func(effect NormalizedEffect, resource ResourceRef, link EvidenceLink) bool {
		return resource.Family == StateFamilyHandle && resource.Deleted && link.Relation == EvidenceLinkExactDeviceInode
	})
	matchExpectedCandidates(&result, candidates, []calibrationExpectation{{
		label: "dup through exact device/inode to deleted open FD",
		matches: func(candidate calibrationCandidate) bool {
			return candidate.link.Relation == EvidenceLinkExactDeviceInode && candidate.effect.Family == StateFamilyHandle && candidate.effect.Operation == "dup" && candidate.resource.Device != 0 && candidate.resource.Inode != 0 && candidate.effect.Resource.Device == candidate.resource.Device && candidate.effect.Resource.Inode == candidate.resource.Inode
		},
	}})
	finalizeCalibrationResult(&result)
	return result
}

func auditUnixSocketCalibration(runDir string, artifacts calibrationArtifacts) CalibrationRunResult {
	result, interval := newCalibrationResult(CalibrationKindUnixSocket, runDir, artifacts)
	result.ExpectedLinks = 2
	if interval == nil {
		return result
	}
	candidates := selectCandidates(*interval, func(effect NormalizedEffect, resource ResourceRef, link EvidenceLink) bool {
		return resource.Family == StateFamilyIPC && resource.SocketID != "" && link.Relation == EvidenceLinkExactSocketID
	})
	matchExpectedCandidates(&result, candidates, []calibrationExpectation{
		{label: "bind through exact socket ID", matches: socketLinkMatches("bind")},
		{label: "listen through exact socket ID", matches: socketLinkMatches("listen")},
	})
	result.ClosureVerified, result.Failures = verifyUnixSocketClosure(interval.AfterCheckpointID, artifacts, result.Failures)
	finalizeCalibrationResult(&result)
	return result
}

func newCalibrationResult(kind CalibrationKind, runDir string, artifacts calibrationArtifacts) (CalibrationRunResult, *CheckpointInterval) {
	result := CalibrationRunResult{Kind: kind, RunID: artifacts.runID, RunDir: runDir, CgroupID: artifacts.cgroupID}
	frontier, ok := expectedCalibrationFrontier(artifacts.mapData)
	if !ok {
		result.Failures = append(result.Failures, "missing frontier before-command..after-command")
		return result, nil
	}
	result.FrontierID = frontier.FrontierID
	if !frontier.IsFrontier {
		result.Failures = append(result.Failures, "before-command..after-command is not a frontier")
	}
	return result, &frontier
}

func expectedCalibrationFrontier(effectMap CheckpointEffectMap) (CheckpointInterval, bool) {
	for _, interval := range effectMap.Intervals {
		if interval.FrontierID == "before-command..after-command" {
			return interval, true
		}
	}
	return CheckpointInterval{}, false
}

type calibrationCandidate struct {
	link     EvidenceLink
	effect   NormalizedEffect
	resource ResourceRef
}

type calibrationExpectation struct {
	label   string
	matches func(calibrationCandidate) bool
}

func selectCandidates(interval CheckpointInterval, include func(NormalizedEffect, ResourceRef, EvidenceLink) bool) []calibrationCandidate {
	effects := make(map[string]NormalizedEffect, len(interval.Effects))
	for _, effect := range interval.Effects {
		effects[effect.EffectID] = effect
	}
	resources := make(map[string]ResourceRef, len(interval.PersistentDelta.Added)+len(interval.PersistentDelta.Removed))
	for _, persistent := range interval.PersistentDelta.Added {
		resources[persistent.Resource.ResourceID] = persistent.Resource
	}
	for _, persistent := range interval.PersistentDelta.Removed {
		resources[persistent.Resource.ResourceID] = persistent.Resource
	}
	candidates := make([]calibrationCandidate, 0)
	for _, link := range interval.EvidenceLinks {
		effect, hasEffect := effects[link.EffectID]
		resource, hasResource := resources[link.ResourceID]
		if !hasEffect || !hasResource || !include(effect, resource, link) {
			continue
		}
		candidates = append(candidates, calibrationCandidate{link: link, effect: effect, resource: resource})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].link.LinkID < candidates[j].link.LinkID })
	return candidates
}

func matchExpectedCandidates(result *CalibrationRunResult, candidates []calibrationCandidate, expectations []calibrationExpectation) {
	result.ObservedLinks = len(candidates)
	matched := make([]bool, len(expectations))
	for _, candidate := range candidates {
		matchIndex := -1
		for index, expectation := range expectations {
			if !matched[index] && expectation.matches(candidate) {
				matchIndex = index
				break
			}
		}
		if matchIndex < 0 {
			result.UnexpectedLinks = append(result.UnexpectedLinks, calibrationEvidence(candidate))
			continue
		}
		matched[matchIndex] = true
		result.MatchedLinks = append(result.MatchedLinks, calibrationEvidence(candidate))
	}
	for index, expectation := range expectations {
		if !matched[index] {
			result.MissingExpectations = append(result.MissingExpectations, expectation.label)
		}
	}
}

func socketLinkMatches(operation string) func(calibrationCandidate) bool {
	return func(candidate calibrationCandidate) bool {
		return candidate.link.Relation == EvidenceLinkExactSocketID && candidate.effect.Family == StateFamilyIPC && candidate.effect.Operation == operation && candidate.resource.Family == StateFamilyIPC && candidate.resource.SocketID != "" && candidate.effect.Resource.SocketID == candidate.resource.SocketID
	}
}

func calibrationEvidence(candidate calibrationCandidate) CalibrationEvidence {
	return CalibrationEvidence{EffectID: candidate.effect.EffectID, Operation: candidate.effect.Operation, ResourceID: candidate.resource.ResourceID, Relation: candidate.link.Relation}
}

func finalizeCalibrationResult(result *CalibrationRunResult) {
	result.Passed = result.ExpectedLinks == len(result.MatchedLinks) && len(result.UnexpectedLinks) == 0 && len(result.MissingExpectations) == 0 && len(result.Failures) == 0
	if result.Kind == CalibrationKindUnixSocket {
		result.Passed = result.Passed && result.ClosureVerified
	}
}

func verifyUnixSocketClosure(afterCheckpointID string, artifacts calibrationArtifacts, failures []string) (bool, []string) {
	after, ok := artifacts.summaries[afterCheckpointID]
	if !ok {
		return false, append(failures, "missing post-frontier state summary")
	}
	endpoint, fd, processID, pathID, ok := findUnixSocketClosure(after)
	if !ok {
		return false, append(failures, "post-frontier summary lacks endpoint/path/FD/process dependency closure")
	}
	if !hasDependency(after.Dependencies, endpoint.ResourceID, pathID, "bound-at-path") || !hasDependency(after.Dependencies, fd.ResourceID, endpoint.ResourceID, "references-unix-socket") || !hasDependency(after.Dependencies, fd.ResourceID, processID, "held-by-process") {
		return false, append(failures, "post-frontier summary lacks a required Unix socket dependency edge")
	}
	followingID, ok := nextCheckpointID(artifacts.catalog, afterCheckpointID)
	if !ok {
		return false, append(failures, "no immediate post-frontier checkpoint for persistence validation")
	}
	following, ok := artifacts.summaries[followingID]
	if !ok {
		return false, append(failures, "missing immediate post-frontier state summary")
	}
	if !summaryContainsUnixSocketClosure(following, endpoint, fd, processID, pathID) {
		return false, append(failures, "Unix socket closure does not persist to the immediate post-frontier checkpoint")
	}
	return true, failures
}

func findUnixSocketClosure(summary CheckpointStateSummary) (ResourceRef, ResourceRef, string, string, bool) {
	resources := resourcesByID(summary)
	for _, endpoint := range resources {
		if endpoint.Family != StateFamilyIPC || endpoint.SocketID == "" {
			continue
		}
		for _, fd := range resources {
			if fd.Family != StateFamilyHandle || fd.SocketID != endpoint.SocketID || fd.HolderPID == 0 {
				continue
			}
			processID := fmt.Sprintf("container-process:%d", fd.HolderPID)
			process, hasProcess := resources[processID]
			if !hasProcess || process.Family != StateFamilyProcess {
				continue
			}
			for _, path := range resources {
				if path.Family == StateFamilyNamespace && path.CanonicalPath != "" && path.CanonicalPath == endpoint.CanonicalPath {
					return endpoint, fd, processID, path.ResourceID, true
				}
			}
		}
	}
	return ResourceRef{}, ResourceRef{}, "", "", false
}

func summaryContainsUnixSocketClosure(summary CheckpointStateSummary, endpoint ResourceRef, fd ResourceRef, processID string, pathID string) bool {
	resources := resourcesByID(summary)
	if _, ok := resources[endpoint.ResourceID]; !ok {
		return false
	}
	if _, ok := resources[fd.ResourceID]; !ok {
		return false
	}
	if _, ok := resources[processID]; !ok {
		return false
	}
	if _, ok := resources[pathID]; !ok {
		return false
	}
	return hasDependency(summary.Dependencies, endpoint.ResourceID, pathID, "bound-at-path") && hasDependency(summary.Dependencies, fd.ResourceID, endpoint.ResourceID, "references-unix-socket") && hasDependency(summary.Dependencies, fd.ResourceID, processID, "held-by-process")
}

func resourcesByID(summary CheckpointStateSummary) map[string]ResourceRef {
	resources := make(map[string]ResourceRef, len(summary.Resources))
	for _, persistent := range summary.Resources {
		resources[persistent.Resource.ResourceID] = persistent.Resource
	}
	return resources
}

func hasDependency(dependencies []ResourceDependency, fromResourceID string, toResourceID string, relation string) bool {
	for _, dependency := range dependencies {
		if dependency.FromResourceID == fromResourceID && dependency.ToResourceID == toResourceID && dependency.Relation == relation {
			return true
		}
	}
	return false
}

func nextCheckpointID(catalog CheckpointCatalog, checkpointID string) (string, bool) {
	for index, checkpoint := range catalog.Checkpoints {
		if checkpoint.CheckpointID == checkpointID && index+1 < len(catalog.Checkpoints) {
			return catalog.Checkpoints[index+1].CheckpointID, true
		}
	}
	return "", false
}

func WriteCalibrationAudit(path string, report CalibrationAuditReport) error {
	return writeJSON(path, report)
}
