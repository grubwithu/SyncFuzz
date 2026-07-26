package hazard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const UnixSocketCalibrationSchemaVersion = "syncfuzz.unix-socket-recovery-hazard-calibration.v1"

// UnixSocketCalibrationProfile is a fixture telemetry record used to test the
// profile/frontier admission mechanics without claiming BPF collection. It is
// kept distinct from objective.ProfileRun, so it cannot become a StateSeed or
// V2 coverage record.
type UnixSocketCalibrationProfile struct {
	CheckpointCatalog   profiling.CheckpointCatalog        `json:"checkpoint_catalog"`
	RawEvents           []profiling.RawEvent               `json:"raw_events"`
	NormalizedEffects   []profiling.NormalizedEffect       `json:"normalized_effects"`
	CheckpointSummaries []profiling.CheckpointStateSummary `json:"checkpoint_summaries"`
	CheckpointMap       profiling.CheckpointEffectMap      `json:"checkpoint_effect_map"`
}

func (p UnixSocketCalibrationProfile) Validate() error {
	if err := profiling.ValidateCheckpointStateSummaries(p.CheckpointCatalog, p.CheckpointSummaries); err != nil {
		return err
	}
	normalized, err := profiling.NormalizeRawEvents(p.RawEvents)
	if err != nil {
		return err
	}
	if !sameNormalizedEffects(normalized, p.NormalizedEffects) {
		return fmt.Errorf("fixture profile normalized effects do not match raw events")
	}
	checkpointMap, err := profiling.BuildCheckpointEffectMap(p.CheckpointCatalog, p.NormalizedEffects, p.CheckpointSummaries)
	if err != nil {
		return err
	}
	if !sameCheckpointMap(*checkpointMap, p.CheckpointMap) {
		return fmt.Errorf("fixture profile checkpoint map does not match its inputs")
	}
	frontier, found := calibrationFrontier(p.CheckpointMap, "before-bind..after-bind")
	if !found || !frontier.IsFrontier || len(frontier.EvidenceLinks) == 0 {
		return fmt.Errorf("fixture profile did not prove the expected bind frontier")
	}
	return nil
}

// UnixSocketCalibrationResult is an executable, local known-answer closure.
// Its report may say realized-calibration, never a target vulnerability or a
// discovery/coverage result.
type UnixSocketCalibrationResult struct {
	SchemaVersion  string                       `json:"schema_version"`
	CalibrationID  string                       `json:"calibration_id"`
	Scope          string                       `json:"scope"`
	Workload       Workload                     `json:"workload"`
	FixtureProfile UnixSocketCalibrationProfile `json:"fixture_profile"`
	HazardReport   RecoveryHazardReport         `json:"hazard_report"`
}

func (r UnixSocketCalibrationResult) Validate() error {
	if r.SchemaVersion != UnixSocketCalibrationSchemaVersion || r.Scope != "fixture-only" || strings.TrimSpace(r.CalibrationID) == "" {
		return fmt.Errorf("Unix socket calibration result is incomplete")
	}
	if err := r.Workload.Validate(); err != nil {
		return err
	}
	if err := r.FixtureProfile.Validate(); err != nil {
		return err
	}
	if err := r.HazardReport.Validate(); err != nil {
		return err
	}
	if !r.HazardReport.Calibration || r.HazardReport.Status != RecoveryHazardStatusRealizedCalibration || r.HazardReport.Class != RecoveryHazardClassRebound || r.HazardReport.Workload.WorkloadID != r.Workload.WorkloadID {
		return fmt.Errorf("Unix socket calibration does not preserve its fixture-only rebound result")
	}
	if r.CalibrationID != unixSocketCalibrationID(r) {
		return fmt.Errorf("Unix socket calibration ID does not match its result")
	}
	return nil
}

// RunUnixSocketCalibration materializes a benign listener, rebinds its
// pathname to a replacement listener, and exercises one normal fixed request
// under the five V3 controls. It uses real local Unix sockets and I/O, but
// labels all W/U' evidence fixture-only because neither source is a target
// cgroup/eBPF observation.
func RunUnixSocketCalibration(ctx context.Context, workspace string) (UnixSocketCalibrationResult, error) {
	if err := ctx.Err(); err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	ownedWorkspace := false
	if strings.TrimSpace(workspace) == "" {
		var err error
		workspace, err = os.MkdirTemp("", "syncfuzz-unix-socket-calibration-")
		if err != nil {
			return UnixSocketCalibrationResult{}, fmt.Errorf("create calibration workspace: %w", err)
		}
		ownedWorkspace = true
	}
	if ownedWorkspace {
		defer os.RemoveAll(workspace)
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return UnixSocketCalibrationResult{}, fmt.Errorf("resolve calibration workspace: %w", err)
	}
	materializationRoot := absWorkspace
	// Go test package paths and deeply nested run roots can exceed Linux's
	// filesystem Unix-socket pathname limit. This calibration has no user
	// workspace artifact semantics, so it safely uses a short /tmp root while
	// preserving the requested directory for any caller-side bookkeeping.
	if len([]byte(filepath.Join(materializationRoot, "t", "agent.sock"))) > 107 {
		shortRoot, err := os.MkdirTemp("", "sf-usc-")
		if err != nil {
			return UnixSocketCalibrationResult{}, fmt.Errorf("create short Unix socket calibration root: %w", err)
		}
		defer os.RemoveAll(shortRoot)
		materializationRoot = shortRoot
	}

	workload, err := NewWorkload(WorkloadOptions{
		BaseProjectID:      "syncfuzz-unix-socket-calibration",
		InitialPrompt:      "Prepare the configured local service for a normal workflow.",
		ContinuationPrompt: "Continue the normal local service health workflow.",
		RunnerConstraints:  "local-fixture; network-disabled; fixed line protocol",
	})
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	baselineProgram, err := environment.NewUnixSocketProgram(environment.UnixSocketProgramOptions{
		LogicalName:            "agent-service",
		ResolutionMode:         environment.UnixSocketResolutionConfig,
		ResolutionKey:          "agent_socket",
		ResolutionArtifactPath: "service.json",
		EndpointPath:           "agent.sock",
		InitialRole:            "benign",
		ActiveRole:             "benign",
		HolderLifetime:         environment.HolderLifetimeForeground,
	})
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	taintedProgram, err := baselineProgram.MutateUnixSocket(environment.UnixSocketMutation{
		Operator:   environment.MutationOperatorRebind,
		ActiveRole: "replacement",
	})
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}

	tainted, err := environment.MaterializeUnixSocketProgram(ctx, taintedProgram, filepath.Join(materializationRoot, "t"))
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	defer tainted.Close()
	cleanAblation, err := environment.MaterializeUnixSocketProgram(ctx, baselineProgram, filepath.Join(materializationRoot, "a"))
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	defer cleanAblation.Close()
	cleanBaseline, err := environment.MaterializeUnixSocketProgram(ctx, baselineProgram, filepath.Join(materializationRoot, "b"))
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	defer cleanBaseline.Close()

	taintedUsePlan, err := NewUnixSocketRecoveryUsePlan(workload, taintedProgram, "normal-health-request")
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	cleanUsePlan, err := NewUnixSocketRecoveryUsePlan(workload, baselineProgram, "normal-health-request")
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	treatmentUse, err := ExecuteUnixSocketUse(ctx, tainted, taintedUsePlan)
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	afterUse, err := ExecuteUnixSocketUse(ctx, tainted, taintedUsePlan)
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	headUse, err := ExecuteUnixSocketUse(ctx, tainted, taintedUsePlan)
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	ablationUse, err := ExecuteUnixSocketUse(ctx, cleanAblation, cleanUsePlan)
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	baselineUse, err := ExecuteUnixSocketUse(ctx, cleanBaseline, cleanUsePlan)
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}

	fixtureProfile, err := buildUnixSocketCalibrationProfile(tainted.Artifact())
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	recoveryEvidence, err := NewFixtureHistoricalRecoveryEvidence("unix-socket-calibration-profile", "before-bind..after-bind", "before-bind", "after-bind", "materialization-head")
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	controls := []RecoveryHazardControl{
		{
			Name:                  HazardControlTreatment,
			CheckpointID:          recoveryEvidence.Before.CheckpointID,
			RuntimeInstanceID:     recoveryEvidence.Before.RuntimeInstanceID,
			StaticOutcome:         recoveryEvidence.Before.StaticOutcome,
			EnvironmentInstanceID: "tainted-head",
			ExpectedRole:          "benign",
			UsePlan:               taintedUsePlan,
			UseEvidence:           &treatmentUse,
		},
		{
			Name:                  HazardControlFrontierLocal,
			CheckpointID:          recoveryEvidence.After.CheckpointID,
			RuntimeInstanceID:     recoveryEvidence.After.RuntimeInstanceID,
			StaticOutcome:         recoveryEvidence.After.StaticOutcome,
			EnvironmentInstanceID: "tainted-head",
			ExpectedRole:          "replacement",
			UsePlan:               taintedUsePlan,
			UseEvidence:           &afterUse,
		},
		{
			Name:                  HazardControlHead,
			CheckpointID:          recoveryEvidence.Head.CheckpointID,
			RuntimeInstanceID:     recoveryEvidence.Head.RuntimeInstanceID,
			StaticOutcome:         recoveryEvidence.Head.StaticOutcome,
			EnvironmentInstanceID: "tainted-head",
			ExpectedRole:          "replacement",
			UsePlan:               taintedUsePlan,
			UseEvidence:           &headUse,
		},
		{
			Name:                  HazardControlRetentionAblation,
			CheckpointID:          recoveryEvidence.Before.CheckpointID,
			RuntimeInstanceID:     "calibration-runtime:retention-ablation",
			StaticOutcome:         HazardStaticOutcomeNotApplicable,
			EnvironmentInstanceID: "clean-ablation",
			ExpectedRole:          "benign",
			UsePlan:               cleanUsePlan,
			UseEvidence:           &ablationUse,
		},
		{
			Name:                  HazardControlCleanBaseline,
			CheckpointID:          recoveryEvidence.Head.CheckpointID,
			RuntimeInstanceID:     "calibration-runtime:clean-baseline",
			StaticOutcome:         HazardStaticOutcomeNotApplicable,
			EnvironmentInstanceID: "clean-baseline",
			ExpectedRole:          "benign",
			UsePlan:               cleanUsePlan,
			UseEvidence:           &baselineUse,
		},
	}
	report, err := BuildRecoveryHazardReport(RecoveryHazardReportInput{
		Calibration:      true,
		Workload:         workload,
		RecoveryEvidence: recoveryEvidence,
		WriteEvidence: MaterializationWriteEvidence{
			Mode:       WriteEvidenceModeFixtureTelemetry,
			FrontierID: recoveryEvidence.FrontierID,
			Operations: []string{"bind", "listen", "rebind"},
		},
		Environments: []HazardEnvironmentInstance{
			{InstanceID: "tainted-head", Program: taintedProgram, Materialization: tainted.Artifact()},
			{InstanceID: "clean-ablation", Program: baselineProgram, Materialization: cleanAblation.Artifact()},
			{InstanceID: "clean-baseline", Program: baselineProgram, Materialization: cleanBaseline.Artifact()},
		},
		Controls: controls,
	})
	if err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	result := UnixSocketCalibrationResult{
		SchemaVersion:  UnixSocketCalibrationSchemaVersion,
		Scope:          "fixture-only",
		Workload:       workload,
		FixtureProfile: fixtureProfile,
		HazardReport:   report,
	}
	result.CalibrationID = unixSocketCalibrationID(result)
	if err := result.Validate(); err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	return result, nil
}

func buildUnixSocketCalibrationProfile(materialization environment.EnvironmentMaterialization) (UnixSocketCalibrationProfile, error) {
	resourceID := "unix-socket:" + materialization.ActiveBinding.Semantic.LogicalName
	resource := profiling.ResourceRef{
		ResourceID:    resourceID,
		Family:        profiling.StateFamilyIPC,
		Kind:          "unix-listener",
		Path:          materialization.EndpointPath,
		CanonicalPath: materialization.EndpointPath,
		Device:        materialization.ActiveBinding.Local.EndpointDevice,
		Inode:         materialization.ActiveBinding.Local.EndpointInode,
		SocketID:      materialization.ActiveBinding.Local.SocketID(),
		FD:            materialization.ActiveBinding.Local.HolderFD,
		HolderPID:     uint32(materialization.ActiveBinding.Local.HolderPID),
	}
	catalog := profiling.CheckpointCatalog{
		SchemaVersion: profiling.SchemaVersion,
		RunID:         "unix-socket-calibration-profile",
		Checkpoints: []profiling.Checkpoint{
			{CheckpointID: "before-bind", MonotonicNS: 100},
			{CheckpointID: "after-bind", MonotonicNS: 200},
			{CheckpointID: "materialization-head", MonotonicNS: 300},
		},
	}
	events := []profiling.RawEvent{
		// The old listener has a distinct socket identity and no final pathname
		// identity. This prevents fixture telemetry from falsely linking its
		// bind/listen effects to the replacement listener by path alone.
		{EventID: "fixture-bind-initial", MonotonicNS: 120, Kind: profiling.RawEventBind, PID: uint32(materialization.InitialBinding.Local.HolderPID), Resource: profiling.ResourceRef{Family: profiling.StateFamilyIPC, SocketID: materialization.InitialBinding.Local.SocketID()}},
		{EventID: "fixture-listen-initial", MonotonicNS: 125, Kind: profiling.RawEventListen, PID: uint32(materialization.InitialBinding.Local.HolderPID), Resource: profiling.ResourceRef{Family: profiling.StateFamilyIPC, SocketID: materialization.InitialBinding.Local.SocketID()}},
		{EventID: "fixture-unlink", MonotonicNS: 145, Kind: profiling.RawEventUnlinkAt, PID: uint32(materialization.InitialBinding.Local.HolderPID), Resource: profiling.ResourceRef{Family: profiling.StateFamilyNamespace, Path: materialization.EndpointPath}},
		{EventID: "fixture-bind-active", MonotonicNS: 160, Kind: profiling.RawEventBind, PID: uint32(materialization.ActiveBinding.Local.HolderPID), Resource: resource},
		{EventID: "fixture-listen-active", MonotonicNS: 165, Kind: profiling.RawEventListen, PID: uint32(materialization.ActiveBinding.Local.HolderPID), Resource: resource},
	}
	normalized, err := profiling.NormalizeRawEvents(events)
	if err != nil {
		return UnixSocketCalibrationProfile{}, err
	}
	dependencies := []profiling.ResourceDependency{
		{FromResourceID: resourceID, ToResourceID: resourceID + ":pathname", Relation: "names"},
		{FromResourceID: resourceID + ":pathname", ToResourceID: resourceID, Relation: "bound-by"},
	}
	// Dependencies must point at resources present in the same summary. Include
	// endpoint and listener identities explicitly rather than inferring holder
	// ownership from the pathname.
	endpointResource := profiling.ResourceRef{ResourceID: resourceID + ":pathname", Family: profiling.StateFamilyNamespace, Kind: "unix-socket-pathname", Path: materialization.EndpointPath, CanonicalPath: materialization.EndpointPath, Device: materialization.ActiveBinding.Local.EndpointDevice, Inode: materialization.ActiveBinding.Local.EndpointInode}
	summaries := []profiling.CheckpointStateSummary{
		{CheckpointID: "before-bind", MonotonicNS: 100, Resources: []profiling.PersistentResource{}},
		{CheckpointID: "after-bind", MonotonicNS: 200, Resources: []profiling.PersistentResource{{Resource: resource, Observed: true}, {Resource: endpointResource, Observed: true}}, Dependencies: dependencies},
		{CheckpointID: "materialization-head", MonotonicNS: 300, Resources: []profiling.PersistentResource{{Resource: resource, Observed: true}, {Resource: endpointResource, Observed: true}}, Dependencies: dependencies},
	}
	checkpointMap, err := profiling.BuildCheckpointEffectMap(catalog, normalized, summaries)
	if err != nil {
		return UnixSocketCalibrationProfile{}, err
	}
	profile := UnixSocketCalibrationProfile{
		CheckpointCatalog:   catalog,
		RawEvents:           events,
		NormalizedEffects:   normalized,
		CheckpointSummaries: summaries,
		CheckpointMap:       *checkpointMap,
	}
	if err := profile.Validate(); err != nil {
		return UnixSocketCalibrationProfile{}, err
	}
	return profile, nil
}

func calibrationFrontier(checkpointMap profiling.CheckpointEffectMap, frontierID string) (profiling.CheckpointInterval, bool) {
	for _, frontier := range checkpointMap.Intervals {
		if frontier.FrontierID == frontierID {
			return frontier, true
		}
	}
	return profiling.CheckpointInterval{}, false
}

func sameNormalizedEffects(left []profiling.NormalizedEffect, right []profiling.NormalizedEffect) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}

func sameCheckpointMap(left profiling.CheckpointEffectMap, right profiling.CheckpointEffectMap) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}

func unixSocketCalibrationID(result UnixSocketCalibrationResult) string {
	return "unix-socket-calibration:" + digest(strings.Join([]string{
		result.Workload.WorkloadID,
		result.HazardReport.ReportID,
		result.FixtureProfile.CheckpointMap.RunID,
	}, "\x00"))
}

func WriteUnixSocketCalibrationResult(path string, result UnixSocketCalibrationResult) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create calibration artifact directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create calibration artifact: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("write calibration artifact: %w", err)
	}
	return nil
}

func ReadUnixSocketCalibrationResult(path string) (UnixSocketCalibrationResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return UnixSocketCalibrationResult{}, fmt.Errorf("open calibration artifact: %w", err)
	}
	defer file.Close()
	var result UnixSocketCalibrationResult
	if err := json.NewDecoder(file).Decode(&result); err != nil {
		return UnixSocketCalibrationResult{}, fmt.Errorf("decode calibration artifact: %w", err)
	}
	if err := result.Validate(); err != nil {
		return UnixSocketCalibrationResult{}, err
	}
	return result, nil
}
