package target

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
)

const (
	TargetPairDifferentialSchemaVersion = "syncfuzz.target-pair-differential.v2"
	TargetPairDifferentialArtifact      = "target-pair-differential.json"

	TargetPairContractCalibrationUnresolved = "contract-unresolved"
	TargetPairContractCalibrationCalibrated = "contract-calibrated"
)

// TargetPairDifferential compares two completed runs at the same typed
// lifecycle checkpoints. Its evidence is intentionally descriptive: target-
// only state is a candidate for later explanation, never a causal verdict.
type TargetPairDifferential struct {
	SchemaVersion       string                         `json:"schema_version"`
	GeneratedAt         string                         `json:"generated_at"`
	QueryID             string                         `json:"query_id"`
	ControlRunID        string                         `json:"control_run_id"`
	TargetRunID         string                         `json:"target_run_id"`
	Checkpoints         []TargetPairCheckpoint         `json:"checkpoints"`
	Evidence            []TargetPairEvidenceCandidate  `json:"evidence_candidates,omitempty"`
	ContractCalibration TargetPairContractCalibration  `json:"contract_calibration"`
	RootCauseCandidates []TargetPairRootCauseCandidate `json:"root_cause_candidates,omitempty"`
}

// TargetPairContractCalibration records whether the paired artifacts support
// a contract-bound explanation. It is stricter than target-only evidence:
// both runs must be task-compliant readings of the same contract rule, with a
// violating target and a contract-consistent control.
type TargetPairContractCalibration struct {
	Status            string                    `json:"status"`
	Reason            string                    `json:"reason"`
	RootCauseEligible bool                      `json:"root_cause_eligible"`
	Control           TargetPairContractReading `json:"control"`
	Target            TargetPairContractReading `json:"target"`
}

// TargetPairContractReading preserves the compact portion of each run's
// contract interpretation needed to audit calibration without treating the
// interpretation itself as a causal conclusion.
type TargetPairContractReading struct {
	Available        bool                               `json:"available"`
	Status           TargetContractInterpretationStatus `json:"status,omitempty"`
	ProfileID        string                             `json:"profile_id,omitempty"`
	RuleID           string                             `json:"rule_id,omitempty"`
	StateSurface     string                             `json:"state_surface,omitempty"`
	LifecycleEdge    string                             `json:"lifecycle_edge,omitempty"`
	Expectation      TargetContractExpectation          `json:"expectation,omitempty"`
	SourceStrength   TargetContractSourceStrength       `json:"source_strength,omitempty"`
	ComplianceStatus TargetTaskComplianceStatus         `json:"compliance_status,omitempty"`
}

type TargetPairCheckpoint struct {
	Point      observation.ObservationPoint `json:"point"`
	Status     string                       `json:"status"`
	Reason     string                       `json:"reason,omitempty"`
	Filesystem TargetPairFilesystemDelta    `json:"filesystem"`
	Processes  TargetPairProcessDelta       `json:"processes"`
}

type TargetPairFilesystemDelta struct {
	TargetOnly  []string               `json:"target_only,omitempty"`
	ControlOnly []string               `json:"control_only,omitempty"`
	Changed     []TargetPairPathChange `json:"changed,omitempty"`
}

type TargetPairPathChange struct {
	Path   string   `json:"path"`
	Fields []string `json:"fields"`
}

type TargetPairProcessDelta struct {
	TargetOnly  []TargetPairProcessFingerprint `json:"target_only,omitempty"`
	ControlOnly []TargetPairProcessFingerprint `json:"control_only,omitempty"`
}

type TargetPairProcessFingerprint struct {
	Name        string `json:"name"`
	CommandLine string `json:"command_line,omitempty"`
	Count       int    `json:"count"`
}

type TargetPairEvidenceCandidate struct {
	Point  observation.ObservationPoint `json:"point"`
	Family string                       `json:"family"`
	Kind   string                       `json:"kind"`
	Detail string                       `json:"detail"`
}

// TargetPairRootCauseCandidate is deliberately conditional. It is emitted
// only for a contract-calibrated pairing: a confirmed violating target and a
// negative, contract-consistent, task-compliant control under the same rule.
// The listed mechanism remains a checkpoint-bound evidence hypothesis, not a
// causal conclusion.
type TargetPairRootCauseCandidate struct {
	Point                  observation.ObservationPoint `json:"point"`
	StateSurface           string                       `json:"state_surface"`
	Mechanism              string                       `json:"mechanism"`
	Evidence               string                       `json:"evidence"`
	Confidence             string                       `json:"confidence"`
	ContractProfileID      string                       `json:"contract_profile_id"`
	ContractRuleID         string                       `json:"contract_rule_id"`
	ContractSourceStrength TargetContractSourceStrength `json:"contract_source_strength"`
}

type TargetPairDifferentialOptions struct {
	ControlRunDir string
	TargetRunDir  string
	OutputPath    string
}

type targetPairRunArtifacts struct {
	Dir          string
	Result       TargetRunResult
	Differential TargetCheckpointDifferential
}

// CompareTargetRuns writes a deterministic checkpoint comparison. It requires
// matching query identities so callers cannot accidentally compare unrelated
// lifecycle contracts.
func CompareTargetRuns(opts TargetPairDifferentialOptions) (*TargetPairDifferential, error) {
	control, err := readTargetPairRunArtifacts(opts.ControlRunDir)
	if err != nil {
		return nil, fmt.Errorf("read control run: %w", err)
	}
	targetRun, err := readTargetPairRunArtifacts(opts.TargetRunDir)
	if err != nil {
		return nil, fmt.Errorf("read target run: %w", err)
	}
	if control.Differential.QueryID == "" || control.Differential.QueryID != targetRun.Differential.QueryID {
		return nil, fmt.Errorf("control query %q does not match target query %q", control.Differential.QueryID, targetRun.Differential.QueryID)
	}

	controlCheckpoints := targetPairCheckpointsByPoint(control.Differential.Checkpoints)
	targetCheckpoints := targetPairCheckpointsByPoint(targetRun.Differential.Checkpoints)
	points := []observation.ObservationPoint{
		observation.ObservationAfterPlant,
		observation.ObservationAfterRecovery,
		observation.ObservationAfterActivation,
	}
	result := &TargetPairDifferential{
		SchemaVersion: TargetPairDifferentialSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		QueryID:       control.Differential.QueryID,
		ControlRunID:  control.Result.RunID,
		TargetRunID:   targetRun.Result.RunID,
		Checkpoints:   make([]TargetPairCheckpoint, 0, len(points)),
	}
	for _, point := range points {
		checkpoint, evidence, err := compareTargetPairCheckpoint(point, control, targetRun, controlCheckpoints[point], targetCheckpoints[point])
		if err != nil {
			return nil, err
		}
		result.Checkpoints = append(result.Checkpoints, checkpoint)
		result.Evidence = append(result.Evidence, evidence...)
	}
	result.ContractCalibration = targetPairContractCalibration(control.Result, targetRun.Result)
	if result.ContractCalibration.RootCauseEligible {
		result.RootCauseCandidates = targetPairRootCauseCandidates(result.Evidence, result.ContractCalibration.Target)
	}
	canonicalizeTargetPairDifferential(result)

	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = filepath.Join(targetRun.Dir, TargetPairDifferentialArtifact)
	}
	if err := core.WriteJSON(output, result); err != nil {
		return nil, fmt.Errorf("write target pair differential %s: %w", output, err)
	}
	return result, nil
}

func targetPairContractCalibration(control TargetRunResult, target TargetRunResult) TargetPairContractCalibration {
	calibration := TargetPairContractCalibration{
		Status:  TargetPairContractCalibrationUnresolved,
		Control: targetPairContractReading(control),
		Target:  targetPairContractReading(target),
	}
	if !target.TargetOracle.Confirmed || target.TargetOracle.Status != TargetOracleStatusConfirmed {
		calibration.Reason = "target oracle is not confirmed"
		return calibration
	}
	if control.TargetOracle.Confirmed || control.TargetOracle.Status != TargetOracleStatusNegative {
		calibration.Reason = "paired control oracle is not negative"
		return calibration
	}
	if !calibration.Control.Available || !calibration.Target.Available {
		calibration.Reason = "a paired run does not provide a contract interpretation"
		return calibration
	}
	if calibration.Control.ComplianceStatus != TargetTaskComplianceStatusCompliant || calibration.Target.ComplianceStatus != TargetTaskComplianceStatusCompliant {
		calibration.Reason = "both paired runs must be task-compliant for contract calibration"
		return calibration
	}
	if !targetPairContractReadingsMatch(calibration.Control, calibration.Target) {
		calibration.Reason = "paired runs do not resolve to the same contract profile and rule"
		return calibration
	}
	if calibration.Target.Status != TargetContractStatusViolation {
		calibration.Reason = "target behavior is not a contract violation"
		return calibration
	}
	if calibration.Control.Status != TargetContractStatusConsistent {
		calibration.Reason = "paired control behavior is not contract-consistent"
		return calibration
	}
	calibration.Status = TargetPairContractCalibrationCalibrated
	calibration.Reason = "target contract violation is paired with a task-compliant, contract-consistent control"
	calibration.RootCauseEligible = true
	return calibration
}

func targetPairContractReading(result TargetRunResult) TargetPairContractReading {
	reading := TargetPairContractReading{ComplianceStatus: result.TaskCompliance.Status}
	interpretation := result.ContractInterpretation
	if interpretation == nil {
		return reading
	}
	reading.Available = true
	reading.Status = interpretation.Status
	reading.ProfileID = interpretation.ProfileID
	reading.RuleID = interpretation.RuleID
	reading.StateSurface = interpretation.StateSurface
	reading.LifecycleEdge = interpretation.LifecycleEdge
	reading.Expectation = interpretation.Expectation
	reading.SourceStrength = interpretation.SourceStrength
	return reading
}

func targetPairContractReadingsMatch(control TargetPairContractReading, target TargetPairContractReading) bool {
	if control.ProfileID == "" || control.RuleID == "" {
		return false
	}
	return control.ProfileID == target.ProfileID &&
		control.RuleID == target.RuleID &&
		control.StateSurface == target.StateSurface &&
		control.LifecycleEdge == target.LifecycleEdge &&
		control.Expectation == target.Expectation &&
		control.SourceStrength == target.SourceStrength
}

func readTargetPairRunArtifacts(runDir string) (targetPairRunArtifacts, error) {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return targetPairRunArtifacts{}, fmt.Errorf("run directory is required")
	}
	result, err := readTargetPairJSON[TargetRunResult](filepath.Join(runDir, TargetResultArtifact))
	if err != nil {
		return targetPairRunArtifacts{}, err
	}
	differential, err := readTargetPairJSON[TargetCheckpointDifferential](filepath.Join(runDir, TargetCheckpointDifferentialArtifact))
	if err != nil {
		return targetPairRunArtifacts{}, err
	}
	if differential.SchemaVersion != TargetCheckpointDifferentialSchemaVersion {
		return targetPairRunArtifacts{}, fmt.Errorf("unsupported checkpoint differential schema %q", differential.SchemaVersion)
	}
	if strings.TrimSpace(differential.QueryID) == "" {
		return targetPairRunArtifacts{}, fmt.Errorf("checkpoint differential is missing query_id")
	}
	return targetPairRunArtifacts{Dir: runDir, Result: result, Differential: differential}, nil
}

func readTargetPairJSON[T any](path string) (T, error) {
	var value T
	raw, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, fmt.Errorf("decode %s: %w", path, err)
	}
	return value, nil
}

func targetPairCheckpointsByPoint(checkpoints []TargetCheckpointState) map[observation.ObservationPoint]TargetCheckpointState {
	byPoint := make(map[observation.ObservationPoint]TargetCheckpointState, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if checkpoint.Point != "" {
			byPoint[checkpoint.Point] = checkpoint
		}
	}
	return byPoint
}

func compareTargetPairCheckpoint(point observation.ObservationPoint, control targetPairRunArtifacts, targetRun targetPairRunArtifacts, controlState TargetCheckpointState, targetState TargetCheckpointState) (TargetPairCheckpoint, []TargetPairEvidenceCandidate, error) {
	checkpoint := TargetPairCheckpoint{Point: point, Status: "comparable"}
	if controlState.Point == "" || targetState.Point == "" {
		checkpoint.Status = "unavailable"
		checkpoint.Reason = "a run does not contain this lifecycle checkpoint"
		return checkpoint, nil, nil
	}
	if controlState.Partial || targetState.Partial {
		checkpoint.Status = "partial"
		reasons := make([]string, 0, 2)
		for _, reason := range []string{controlState.Reason, targetState.Reason} {
			if reason = strings.TrimSpace(reason); reason != "" {
				reasons = append(reasons, reason)
			}
		}
		checkpoint.Reason = strings.Join(reasons, "; ")
	}
	if controlState.FilesystemArtifact != "" && targetState.FilesystemArtifact != "" {
		controlSnapshot, err := readTargetPairSnapshot(control.Dir, controlState.FilesystemArtifact)
		if err != nil {
			return TargetPairCheckpoint{}, nil, err
		}
		targetSnapshot, err := readTargetPairSnapshot(targetRun.Dir, targetState.FilesystemArtifact)
		if err != nil {
			return TargetPairCheckpoint{}, nil, err
		}
		checkpoint.Filesystem = compareTargetPairFilesystem(controlSnapshot, targetSnapshot)
	} else if checkpoint.Status == "comparable" {
		checkpoint.Status = "partial"
		checkpoint.Reason = "filesystem artifact is unavailable at this checkpoint"
	}
	if controlState.ProcessArtifact != "" && targetState.ProcessArtifact != "" {
		controlProcesses, err := readTargetPairProcesses(control.Dir, controlState.ProcessArtifact)
		if err != nil {
			return TargetPairCheckpoint{}, nil, err
		}
		targetProcesses, err := readTargetPairProcesses(targetRun.Dir, targetState.ProcessArtifact)
		if err != nil {
			return TargetPairCheckpoint{}, nil, err
		}
		checkpoint.Processes = compareTargetPairProcesses(controlProcesses, targetProcesses)
	} else if checkpoint.Status == "comparable" {
		checkpoint.Status = "partial"
		checkpoint.Reason = "process artifact is unavailable at this checkpoint"
	}
	return checkpoint, targetPairEvidenceForCheckpoint(checkpoint), nil
}

func readTargetPairSnapshot(runDir string, artifact string) (core.Snapshot, error) {
	path, err := targetPairArtifactPath(runDir, artifact)
	if err != nil {
		return core.Snapshot{}, err
	}
	snapshot, err := readTargetPairJSON[core.Snapshot](path)
	if err != nil {
		return core.Snapshot{}, err
	}
	return snapshot, nil
}

func readTargetPairProcesses(runDir string, artifact string) (core.ProcessSnapshot, error) {
	path, err := targetPairArtifactPath(runDir, artifact)
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	processes, err := readTargetPairJSON[core.ProcessSnapshot](path)
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	return processes, nil
}

func targetPairArtifactPath(runDir string, artifact string) (string, error) {
	artifact = strings.TrimSpace(artifact)
	if artifact == "" || filepath.IsAbs(artifact) || filepath.Base(artifact) != artifact {
		return "", fmt.Errorf("invalid checkpoint artifact %q", artifact)
	}
	return filepath.Join(runDir, artifact), nil
}

func compareTargetPairFilesystem(control core.Snapshot, target core.Snapshot) TargetPairFilesystemDelta {
	controlPaths := control.Paths()
	targetPaths := target.Paths()
	delta := TargetPairFilesystemDelta{}
	for path, targetEntry := range targetPaths {
		if isTargetPairControlPath(path) {
			continue
		}
		controlEntry, exists := controlPaths[path]
		if !exists {
			delta.TargetOnly = append(delta.TargetOnly, path)
			continue
		}
		if fields := targetPairFileEntryChanges(controlEntry, targetEntry); len(fields) > 0 {
			delta.Changed = append(delta.Changed, TargetPairPathChange{Path: path, Fields: fields})
		}
	}
	for path := range controlPaths {
		if isTargetPairControlPath(path) {
			continue
		}
		if _, exists := targetPaths[path]; !exists {
			delta.ControlOnly = append(delta.ControlOnly, path)
		}
	}
	sort.Strings(delta.TargetOnly)
	sort.Strings(delta.ControlOnly)
	sort.Slice(delta.Changed, func(i, j int) bool { return delta.Changed[i].Path < delta.Changed[j].Path })
	return delta
}

// isTargetPairControlPath excludes artifacts injected by SyncFuzz itself from
// cross-run evidence. Their task/run ids and command text legitimately differ
// between a control and a target run and cannot explain target state drift.
func isTargetPairControlPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if strings.HasPrefix(path, TargetLifecycleMarkerArtifact+".") {
		return true
	}
	switch path {
	case TargetTaskArtifact, TargetPromptArtifact, TargetLifecycleMarkerArtifact, TargetLifecycleMarkerHelperArtifact:
		return true
	default:
		return false
	}
}

func targetPairFileEntryChanges(control core.FileEntry, target core.FileEntry) []string {
	fields := make([]string, 0, 5)
	if control.Type != target.Type {
		fields = append(fields, "type")
	}
	if control.Mode != target.Mode {
		fields = append(fields, "mode")
	}
	if control.Size != target.Size {
		fields = append(fields, "size")
	}
	if control.SHA256 != target.SHA256 {
		fields = append(fields, "content_hash")
	}
	if control.SymlinkTarget != target.SymlinkTarget {
		fields = append(fields, "symlink_target")
	}
	return fields
}

func compareTargetPairProcesses(control core.ProcessSnapshot, target core.ProcessSnapshot) TargetPairProcessDelta {
	controlCounts := targetPairProcessCounts(control.Processes)
	targetCounts := targetPairProcessCounts(target.Processes)
	delta := TargetPairProcessDelta{}
	for key, count := range targetCounts {
		controlCount := controlCounts[key]
		if count > controlCount {
			fingerprint := targetPairProcessFingerprintFromKey(key)
			fingerprint.Count = count - controlCount
			delta.TargetOnly = append(delta.TargetOnly, fingerprint)
		}
	}
	for key, count := range controlCounts {
		targetCount := targetCounts[key]
		if count > targetCount {
			fingerprint := targetPairProcessFingerprintFromKey(key)
			fingerprint.Count = count - targetCount
			delta.ControlOnly = append(delta.ControlOnly, fingerprint)
		}
	}
	sort.Slice(delta.TargetOnly, func(i, j int) bool {
		if delta.TargetOnly[i].Name != delta.TargetOnly[j].Name {
			return delta.TargetOnly[i].Name < delta.TargetOnly[j].Name
		}
		return delta.TargetOnly[i].CommandLine < delta.TargetOnly[j].CommandLine
	})
	sort.Slice(delta.ControlOnly, func(i, j int) bool {
		if delta.ControlOnly[i].Name != delta.ControlOnly[j].Name {
			return delta.ControlOnly[i].Name < delta.ControlOnly[j].Name
		}
		return delta.ControlOnly[i].CommandLine < delta.ControlOnly[j].CommandLine
	})
	return delta
}

func targetPairProcessCounts(processes []core.ProcessEntry) map[string]int {
	counts := make(map[string]int, len(processes))
	for _, process := range processes {
		key := strings.TrimSpace(process.Name) + "\x00" + strings.TrimSpace(process.RawCmdline)
		if key == "\x00" {
			continue
		}
		counts[key]++
	}
	return counts
}

func targetPairProcessFingerprintFromKey(key string) TargetPairProcessFingerprint {
	parts := strings.SplitN(key, "\x00", 2)
	fingerprint := TargetPairProcessFingerprint{Name: parts[0]}
	if len(parts) == 2 {
		fingerprint.CommandLine = parts[1]
	}
	return fingerprint
}

func targetPairEvidenceForCheckpoint(checkpoint TargetPairCheckpoint) []TargetPairEvidenceCandidate {
	evidence := make([]TargetPairEvidenceCandidate, 0)
	for _, path := range checkpoint.Filesystem.TargetOnly {
		evidence = append(evidence, TargetPairEvidenceCandidate{Point: checkpoint.Point, Family: "filesystem", Kind: "target-only-path", Detail: path})
	}
	for _, change := range checkpoint.Filesystem.Changed {
		evidence = append(evidence, TargetPairEvidenceCandidate{Point: checkpoint.Point, Family: "filesystem", Kind: "target-control-path-difference", Detail: change.Path + " fields=" + strings.Join(change.Fields, ",")})
	}
	for _, process := range checkpoint.Processes.TargetOnly {
		detail := process.Name
		if process.CommandLine != "" {
			detail += " command=" + process.CommandLine
		}
		evidence = append(evidence, TargetPairEvidenceCandidate{Point: checkpoint.Point, Family: "process", Kind: "target-only-process", Detail: detail})
	}
	return evidence
}

func canonicalizeTargetPairDifferential(result *TargetPairDifferential) {
	if result == nil {
		return
	}
	sort.Slice(result.Evidence, func(i, j int) bool {
		if result.Evidence[i].Point != result.Evidence[j].Point {
			return result.Evidence[i].Point < result.Evidence[j].Point
		}
		if result.Evidence[i].Family != result.Evidence[j].Family {
			return result.Evidence[i].Family < result.Evidence[j].Family
		}
		if result.Evidence[i].Kind != result.Evidence[j].Kind {
			return result.Evidence[i].Kind < result.Evidence[j].Kind
		}
		return result.Evidence[i].Detail < result.Evidence[j].Detail
	})
	sort.Slice(result.RootCauseCandidates, func(i, j int) bool {
		if result.RootCauseCandidates[i].Point != result.RootCauseCandidates[j].Point {
			return result.RootCauseCandidates[i].Point < result.RootCauseCandidates[j].Point
		}
		if result.RootCauseCandidates[i].StateSurface != result.RootCauseCandidates[j].StateSurface {
			return result.RootCauseCandidates[i].StateSurface < result.RootCauseCandidates[j].StateSurface
		}
		if result.RootCauseCandidates[i].Mechanism != result.RootCauseCandidates[j].Mechanism {
			return result.RootCauseCandidates[i].Mechanism < result.RootCauseCandidates[j].Mechanism
		}
		if result.RootCauseCandidates[i].ContractRuleID != result.RootCauseCandidates[j].ContractRuleID {
			return result.RootCauseCandidates[i].ContractRuleID < result.RootCauseCandidates[j].ContractRuleID
		}
		return result.RootCauseCandidates[i].Evidence < result.RootCauseCandidates[j].Evidence
	})
}

func targetPairRootCauseCandidates(evidence []TargetPairEvidenceCandidate, contract TargetPairContractReading) []TargetPairRootCauseCandidate {
	candidates := make([]TargetPairRootCauseCandidate, 0, len(evidence))
	for _, item := range evidence {
		candidate := TargetPairRootCauseCandidate{
			Point:                  item.Point,
			Evidence:               item.Detail,
			Confidence:             "contract-calibrated-evidence-hypothesis",
			ContractProfileID:      contract.ProfileID,
			ContractRuleID:         contract.RuleID,
			ContractSourceStrength: contract.SourceStrength,
		}
		switch {
		case item.Family == "filesystem" && item.Kind == "target-only-path":
			candidate.StateSurface = "filesystem-namespace"
			candidate.Mechanism = "target-only namespace object at lifecycle checkpoint"
		case item.Family == "filesystem" && item.Kind == "target-control-path-difference":
			candidate.StateSurface = "filesystem-namespace"
			candidate.Mechanism = "target-control namespace metadata or content divergence"
		case item.Family == "process" && item.Kind == "target-only-process":
			candidate.StateSurface = "process"
			candidate.Mechanism = "target-only process persisted at lifecycle checkpoint"
		default:
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}
