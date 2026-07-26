package hazard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const RecoveryHazardReportSchemaVersion = "syncfuzz.recovery-hazard-report.v1"
const HistoricalRecoveryEvidenceSchemaVersion = "syncfuzz.historical-recovery-evidence.v1"

type WriteEvidenceMode string

const (
	// Fixture telemetry is explicitly a calibration-only W witness. A target
	// run must use ProfileEBPF before it can report production W evidence.
	WriteEvidenceModeFixtureTelemetry WriteEvidenceMode = "fixture-telemetry"
	WriteEvidenceModeProfileEBPF      WriteEvidenceMode = "profile-ebpf"
)

func (m WriteEvidenceMode) Valid() bool {
	return m == WriteEvidenceModeFixtureTelemetry || m == WriteEvidenceModeProfileEBPF
}

type MaterializationWriteEvidence struct {
	Mode       WriteEvidenceMode `json:"mode"`
	FrontierID string            `json:"frontier_id"`
	Operations []string          `json:"operations"`
}

func (e MaterializationWriteEvidence) ValidateFor(frontierID string) error {
	if !e.Mode.Valid() || strings.TrimSpace(e.FrontierID) == "" || e.FrontierID != frontierID || len(e.Operations) == 0 {
		return fmt.Errorf("materialization W evidence is incomplete")
	}
	seen := map[string]bool{}
	for _, operation := range e.Operations {
		operation = strings.TrimSpace(operation)
		if operation == "" {
			return fmt.Errorf("materialization W evidence contains an empty operation")
		}
		seen[operation] = true
	}
	if !seen["bind"] || !seen["listen"] {
		return fmt.Errorf("Unix socket materialization W evidence requires bind and listen")
	}
	return nil
}

type HazardControlName string

const (
	HazardControlTreatment         HazardControlName = "treatment"
	HazardControlFrontierLocal     HazardControlName = "frontier-local"
	HazardControlHead              HazardControlName = "head"
	HazardControlRetentionAblation HazardControlName = "retention-ablation"
	HazardControlCleanBaseline     HazardControlName = "clean-baseline"
)

func (n HazardControlName) Valid() bool {
	switch n {
	case HazardControlTreatment, HazardControlFrontierLocal, HazardControlHead, HazardControlRetentionAblation, HazardControlCleanBaseline:
		return true
	default:
		return false
	}
}

type HazardStaticOutcome string

const (
	HazardStaticOutcomeResidual      HazardStaticOutcome = "residual"
	HazardStaticOutcomeConsistent    HazardStaticOutcome = "consistent"
	HazardStaticOutcomeInconclusive  HazardStaticOutcome = "inconclusive"
	HazardStaticOutcomeNotApplicable HazardStaticOutcome = "not-applicable"
)

func (o HazardStaticOutcome) Valid() bool {
	switch o {
	case HazardStaticOutcomeResidual, HazardStaticOutcomeConsistent, HazardStaticOutcomeInconclusive, HazardStaticOutcomeNotApplicable:
		return true
	default:
		return false
	}
}

type HistoricalRecoveryEvidenceSource string

const (
	// StateSeed evidence is derived from an actual V2 StateSeed, durable
	// checkpoint recovery set, and executed controls.
	HistoricalRecoveryEvidenceStateSeed HistoricalRecoveryEvidenceSource = "state-seed"
	// CalibrationFixture is deliberately not a StateSeed. It exercises the
	// V3 classifier without entering V2 corpus/coverage or claiming a native
	// target checkpoint recovery.
	HistoricalRecoveryEvidenceCalibrationFixture HistoricalRecoveryEvidenceSource = "calibration-fixture"
)

func (s HistoricalRecoveryEvidenceSource) Valid() bool {
	return s == HistoricalRecoveryEvidenceStateSeed || s == HistoricalRecoveryEvidenceCalibrationFixture
}

type RecoveryCoordinateEvidence struct {
	QueryID             string              `json:"query_id"`
	CheckpointID        string              `json:"checkpoint_id"`
	LogicalCoordinateID string              `json:"logical_coordinate_id,omitempty"`
	RuntimeInstanceID   string              `json:"runtime_instance_id"`
	StaticOutcome       HazardStaticOutcome `json:"static_outcome"`
}

func (e RecoveryCoordinateEvidence) Validate() error {
	if strings.TrimSpace(e.QueryID) == "" || strings.TrimSpace(e.CheckpointID) == "" || strings.TrimSpace(e.RuntimeInstanceID) == "" || !e.StaticOutcome.Valid() || e.StaticOutcome == HazardStaticOutcomeNotApplicable {
		return fmt.Errorf("recovery coordinate evidence is incomplete")
	}
	return nil
}

func (e RecoveryCoordinateEvidence) EffectiveLogicalCoordinateID() string {
	if strings.TrimSpace(e.LogicalCoordinateID) != "" {
		return e.LogicalCoordinateID
	}
	return e.CheckpointID
}

// HistoricalRecoveryEvidence describes R(C,H) without forcing calibration
// fixtures to masquerade as StateSeeds. NewHistoricalRecoveryEvidenceFromExecution
// is the adapter used for real target runs.
type HistoricalRecoveryEvidence struct {
	SchemaVersion         string                           `json:"schema_version"`
	Source                HistoricalRecoveryEvidenceSource `json:"source"`
	SourceID              string                           `json:"source_id"`
	ProfileRunID          string                           `json:"profile_run_id"`
	FrontierID            string                           `json:"frontier_id"`
	MaterializationHeadID string                           `json:"materialization_head_id"`
	RetentionPolicy       recovery.RetentionPolicy         `json:"retention_policy"`
	Before                RecoveryCoordinateEvidence       `json:"before"`
	After                 RecoveryCoordinateEvidence       `json:"after"`
	Head                  RecoveryCoordinateEvidence       `json:"head"`
}

func (e HistoricalRecoveryEvidence) Validate() error {
	if e.SchemaVersion != HistoricalRecoveryEvidenceSchemaVersion || !e.Source.Valid() || strings.TrimSpace(e.SourceID) == "" || strings.TrimSpace(e.ProfileRunID) == "" || strings.TrimSpace(e.FrontierID) == "" || strings.TrimSpace(e.MaterializationHeadID) == "" || e.RetentionPolicy != recovery.RetentionPolicyRetainRelevantOSState {
		return fmt.Errorf("historical recovery evidence is incomplete")
	}
	for _, coordinate := range []RecoveryCoordinateEvidence{e.Before, e.After, e.Head} {
		if err := coordinate.Validate(); err != nil {
			return err
		}
	}
	if e.Before.CheckpointID == e.After.CheckpointID || e.Before.CheckpointID == e.Head.CheckpointID || e.After.CheckpointID == e.Head.CheckpointID || e.Before.RuntimeInstanceID == e.After.RuntimeInstanceID || e.Before.RuntimeInstanceID == e.Head.RuntimeInstanceID || e.After.RuntimeInstanceID == e.Head.RuntimeInstanceID {
		return fmt.Errorf("historical recovery evidence does not preserve distinct coordinates and runtimes")
	}
	return nil
}

// NewHistoricalRecoveryEvidenceFromExecution maps the existing StateSeed/
// HistoricalRecoverySet/ForkRecoverySetExecution contract into V3's generic
// recovery evidence without changing the V2 relation taxonomy.
func NewHistoricalRecoveryEvidenceFromExecution(seed objective.StateSeed, set recovery.HistoricalRecoverySet, execution recovery.ForkRecoverySetExecution) (HistoricalRecoveryEvidence, error) {
	if err := validateRecoveryExecution(seed, set, execution); err != nil {
		return HistoricalRecoveryEvidence{}, err
	}
	// Classify each observation directly so the before/after/head label does
	// not depend on a fabricated pair ordering.
	classify := func(observation recovery.RecoveryObservation) HazardStaticOutcome {
		return staticOutcome(classifyRecoveryObservation(observation))
	}
	evidence := HistoricalRecoveryEvidence{
		SchemaVersion:         HistoricalRecoveryEvidenceSchemaVersion,
		Source:                HistoricalRecoveryEvidenceStateSeed,
		SourceID:              seed.SeedID,
		ProfileRunID:          seed.ProfileRunID,
		FrontierID:            seed.FrontierID,
		MaterializationHeadID: set.MaterializationHead.HeadID,
		RetentionPolicy:       set.RetentionPolicy,
		Before: RecoveryCoordinateEvidence{
			QueryID:           set.Before.QueryID,
			CheckpointID:      set.Before.CheckpointID,
			RuntimeInstanceID: execution.Before.RuntimeInstanceID,
			StaticOutcome:     classify(execution.Before),
		},
		After: RecoveryCoordinateEvidence{
			QueryID:           set.After.QueryID,
			CheckpointID:      set.After.CheckpointID,
			RuntimeInstanceID: execution.After.RuntimeInstanceID,
			StaticOutcome:     classify(execution.After),
		},
		Head: RecoveryCoordinateEvidence{
			QueryID:           set.Head.QueryID,
			CheckpointID:      set.Head.CheckpointID,
			RuntimeInstanceID: execution.Head.RuntimeInstanceID,
			StaticOutcome:     classify(execution.Head),
		},
	}
	if err := evidence.Validate(); err != nil {
		return HistoricalRecoveryEvidence{}, err
	}
	return evidence, nil
}

// NewFixtureHistoricalRecoveryEvidence is a calibration-only construction.
// It must never be written as a StateSeed or submitted to V2 coverage.
func NewFixtureHistoricalRecoveryEvidence(profileRunID, frontierID, beforeCheckpointID, afterCheckpointID, headCheckpointID string) (HistoricalRecoveryEvidence, error) {
	evidence := HistoricalRecoveryEvidence{
		SchemaVersion:         HistoricalRecoveryEvidenceSchemaVersion,
		Source:                HistoricalRecoveryEvidenceCalibrationFixture,
		SourceID:              "calibration-recovery:" + strings.TrimSpace(profileRunID) + ":" + strings.TrimSpace(frontierID),
		ProfileRunID:          strings.TrimSpace(profileRunID),
		FrontierID:            strings.TrimSpace(frontierID),
		MaterializationHeadID: "materialization-head:" + strings.TrimSpace(profileRunID) + ":" + strings.TrimSpace(headCheckpointID),
		RetentionPolicy:       recovery.RetentionPolicyRetainRelevantOSState,
		Before: RecoveryCoordinateEvidence{
			QueryID:           "calibration-query:" + strings.TrimSpace(beforeCheckpointID),
			CheckpointID:      strings.TrimSpace(beforeCheckpointID),
			RuntimeInstanceID: "calibration-runtime:before",
			StaticOutcome:     HazardStaticOutcomeResidual,
		},
		After: RecoveryCoordinateEvidence{
			QueryID:           "calibration-query:" + strings.TrimSpace(afterCheckpointID),
			CheckpointID:      strings.TrimSpace(afterCheckpointID),
			RuntimeInstanceID: "calibration-runtime:after",
			StaticOutcome:     HazardStaticOutcomeConsistent,
		},
		Head: RecoveryCoordinateEvidence{
			QueryID:           "calibration-query:" + strings.TrimSpace(headCheckpointID),
			CheckpointID:      strings.TrimSpace(headCheckpointID),
			RuntimeInstanceID: "calibration-runtime:head",
			StaticOutcome:     HazardStaticOutcomeConsistent,
		},
	}
	if err := evidence.Validate(); err != nil {
		return HistoricalRecoveryEvidence{}, err
	}
	return evidence, nil
}

type HazardEnvironmentInstance struct {
	InstanceID            string                                       `json:"instance_id"`
	Program               environment.EnvironmentProgram               `json:"program"`
	Materialization       environment.EnvironmentMaterialization       `json:"materialization,omitempty"`
	TargetMaterialization *environment.TargetUnixSocketMaterialization `json:"target_materialization,omitempty"`
}

func (i HazardEnvironmentInstance) Validate() error {
	if strings.TrimSpace(i.InstanceID) == "" {
		return fmt.Errorf("hazard environment instance requires an ID")
	}
	if err := i.Program.Validate(); err != nil {
		return err
	}
	local := i.Materialization.SchemaVersion != ""
	target := i.TargetMaterialization != nil
	if local == target {
		return fmt.Errorf("hazard environment instance must contain exactly one local or target materialization")
	}
	if local {
		return i.Materialization.ValidateFor(i.Program)
	}
	return i.TargetMaterialization.ValidateFor(i.Program)
}

func (i HazardEnvironmentInstance) activeBinding() environment.MaterializedUnixSocketBinding {
	if i.TargetMaterialization != nil {
		return i.TargetMaterialization.ActiveBinding()
	}
	return i.Materialization.ActiveBinding
}

func (i HazardEnvironmentInstance) initialBinding() environment.MaterializedUnixSocketBinding {
	if i.TargetMaterialization != nil {
		return i.TargetMaterialization.InitialBinding()
	}
	return i.Materialization.InitialBinding
}

type RecoveryHazardControl struct {
	Name                  HazardControlName      `json:"name"`
	CheckpointID          string                 `json:"checkpoint_id"`
	LogicalCoordinateID   string                 `json:"logical_coordinate_id,omitempty"`
	RuntimeInstanceID     string                 `json:"runtime_instance_id"`
	StaticOutcome         HazardStaticOutcome    `json:"static_outcome"`
	EnvironmentInstanceID string                 `json:"environment_instance_id"`
	ExpectedRole          string                 `json:"expected_role"`
	UsePlan               RecoveryUsePlan        `json:"use_plan"`
	UseEvidence           *UnixSocketUseEvidence `json:"use_evidence,omitempty"`
}

func (c RecoveryHazardControl) EffectiveLogicalCoordinateID() string {
	if strings.TrimSpace(c.LogicalCoordinateID) != "" {
		return c.LogicalCoordinateID
	}
	return c.CheckpointID
}

type RecoveryHazardClass string

const (
	RecoveryHazardClassNone    RecoveryHazardClass = "none"
	RecoveryHazardClassRebound RecoveryHazardClass = "rebound"
)

func (c RecoveryHazardClass) Valid() bool {
	return c == RecoveryHazardClassNone || c == RecoveryHazardClassRebound
}

type RecoveryHazardStatus string

const (
	RecoveryHazardStatusInconclusive        RecoveryHazardStatus = "inconclusive"
	RecoveryHazardStatusNotRealized         RecoveryHazardStatus = "not-realized"
	RecoveryHazardStatusRealizedCalibration RecoveryHazardStatus = "realized-calibration"
	RecoveryHazardStatusRealized            RecoveryHazardStatus = "realized"
)

func (s RecoveryHazardStatus) Valid() bool {
	switch s {
	case RecoveryHazardStatusInconclusive, RecoveryHazardStatusNotRealized, RecoveryHazardStatusRealizedCalibration, RecoveryHazardStatusRealized:
		return true
	default:
		return false
	}
}

// RecoveryHazardReport preserves the required separation:
//   - materialization W is explicit;
//   - recovery R(C,H) is represented independently of a V2 StateSeed when
//     necessary for calibration;
//   - typed U' is per-control use evidence;
//   - final status is an evidence classification, never a vulnerability or a
//     framework-contract verdict.
type RecoveryHazardReport struct {
	SchemaVersion    string                       `json:"schema_version"`
	ReportID         string                       `json:"report_id"`
	Calibration      bool                         `json:"calibration"`
	Workload         Workload                     `json:"workload"`
	RecoveryEvidence HistoricalRecoveryEvidence   `json:"recovery_evidence"`
	WriteEvidence    MaterializationWriteEvidence `json:"write_evidence"`
	Environments     []HazardEnvironmentInstance  `json:"environments"`
	Controls         []RecoveryHazardControl      `json:"controls"`
	Class            RecoveryHazardClass          `json:"class"`
	Status           RecoveryHazardStatus         `json:"status"`
	Reason           string                       `json:"reason"`
}

type RecoveryHazardReportInput struct {
	Calibration      bool
	Workload         Workload
	RecoveryEvidence HistoricalRecoveryEvidence
	WriteEvidence    MaterializationWriteEvidence
	Environments     []HazardEnvironmentInstance
	Controls         []RecoveryHazardControl
}

func BuildRecoveryHazardReport(input RecoveryHazardReportInput) (RecoveryHazardReport, error) {
	report := RecoveryHazardReport{
		SchemaVersion:    RecoveryHazardReportSchemaVersion,
		Calibration:      input.Calibration,
		Workload:         input.Workload,
		RecoveryEvidence: input.RecoveryEvidence,
		WriteEvidence:    input.WriteEvidence,
		Environments:     append([]HazardEnvironmentInstance(nil), input.Environments...),
		Controls:         append([]RecoveryHazardControl(nil), input.Controls...),
	}
	if err := validateReportContext(report); err != nil {
		return RecoveryHazardReport{}, err
	}
	report.Class, report.Status, report.Reason = classifyRecoveryHazard(report)
	report.ReportID = recoveryHazardReportID(report)
	if err := report.Validate(); err != nil {
		return RecoveryHazardReport{}, err
	}
	return report, nil
}

func (r RecoveryHazardReport) Validate() error {
	if r.SchemaVersion != RecoveryHazardReportSchemaVersion || !r.Class.Valid() || !r.Status.Valid() || strings.TrimSpace(r.Reason) == "" || r.ReportID != recoveryHazardReportID(r) {
		return fmt.Errorf("recovery hazard report has an invalid identity or classification")
	}
	if err := validateReportContext(r); err != nil {
		return err
	}
	class, status, reason := classifyRecoveryHazard(r)
	if r.Class != class || r.Status != status || r.Reason != reason {
		return fmt.Errorf("recovery hazard report classification does not match its evidence")
	}
	return nil
}

func validateReportContext(report RecoveryHazardReport) error {
	if err := report.Workload.Validate(); err != nil {
		return err
	}
	if err := report.RecoveryEvidence.Validate(); err != nil {
		return err
	}
	if report.Calibration != (report.RecoveryEvidence.Source == HistoricalRecoveryEvidenceCalibrationFixture) {
		return fmt.Errorf("recovery hazard report calibration flag does not match recovery evidence source")
	}
	if err := report.WriteEvidence.ValidateFor(report.RecoveryEvidence.FrontierID); err != nil {
		return err
	}
	if report.Calibration && report.WriteEvidence.Mode != WriteEvidenceModeFixtureTelemetry {
		return fmt.Errorf("fixture calibration must use fixture materialization W evidence")
	}
	if !report.Calibration && report.WriteEvidence.Mode != WriteEvidenceModeProfileEBPF {
		return fmt.Errorf("target recovery hazard report must use profile eBPF W evidence")
	}
	if len(report.Environments) == 0 || len(report.Controls) != 5 {
		return fmt.Errorf("recovery hazard report requires materialized environments and all five controls")
	}
	environments := make(map[string]HazardEnvironmentInstance, len(report.Environments))
	for _, instance := range report.Environments {
		if _, found := environments[instance.InstanceID]; found {
			return fmt.Errorf("duplicate hazard environment instance %q", instance.InstanceID)
		}
		if err := instance.Validate(); err != nil {
			return err
		}
		environments[instance.InstanceID] = instance
	}
	seenControls := make(map[HazardControlName]struct{}, len(report.Controls))
	runtimeIDs := make(map[string]struct{}, len(report.Controls))
	for _, control := range report.Controls {
		if !control.Name.Valid() || strings.TrimSpace(control.CheckpointID) == "" || strings.TrimSpace(control.RuntimeInstanceID) == "" || !control.StaticOutcome.Valid() || !validRole(control.ExpectedRole) {
			return fmt.Errorf("recovery hazard control is incomplete")
		}
		if _, found := seenControls[control.Name]; found {
			return fmt.Errorf("duplicate recovery hazard control %q", control.Name)
		}
		seenControls[control.Name] = struct{}{}
		if _, found := runtimeIDs[control.RuntimeInstanceID]; found {
			return fmt.Errorf("recovery hazard controls reused runtime instance %q", control.RuntimeInstanceID)
		}
		runtimeIDs[control.RuntimeInstanceID] = struct{}{}
		instance, found := environments[control.EnvironmentInstanceID]
		if !found {
			return fmt.Errorf("recovery hazard control %q names unknown environment instance %q", control.Name, control.EnvironmentInstanceID)
		}
		if err := control.UsePlan.ValidateFor(report.Workload, instance.Program); err != nil {
			return fmt.Errorf("recovery hazard control %q use plan: %w", control.Name, err)
		}
		if control.UseEvidence != nil {
			if err := control.UseEvidence.ValidateFor(report.Workload, control.UsePlan, instance.Program); err != nil {
				return fmt.Errorf("recovery hazard control %q use evidence: %w", control.Name, err)
			}
			if report.Calibration && control.UseEvidence.EvidenceMode != UseEvidenceModeFixtureRoundTrip {
				return fmt.Errorf("fixture calibration control %q has non-fixture use evidence", control.Name)
			}
			if !report.Calibration && control.UseEvidence.EvidenceMode != UseEvidenceModeEBPFResolved {
				return fmt.Errorf("target recovery control %q has non-eBPF use evidence", control.Name)
			}
			active := instance.activeBinding()
			if control.UseEvidence.ListenerSemantic != active.Semantic || control.UseEvidence.ListenerLocal != active.Local {
				return fmt.Errorf("recovery hazard control %q use evidence does not bind the active materialized listener", control.Name)
			}
		}
	}
	for _, required := range []HazardControlName{HazardControlTreatment, HazardControlFrontierLocal, HazardControlHead, HazardControlRetentionAblation, HazardControlCleanBaseline} {
		if _, found := seenControls[required]; !found {
			return fmt.Errorf("recovery hazard report lacks %s control", required)
		}
	}
	return validateControlCoordinates(report)
}

func validateControlCoordinates(report RecoveryHazardReport) error {
	controls := controlsByName(report.Controls)
	recoveryEvidence := report.RecoveryEvidence
	if controls[HazardControlTreatment].CheckpointID != recoveryEvidence.Before.CheckpointID || controls[HazardControlFrontierLocal].CheckpointID != recoveryEvidence.After.CheckpointID || controls[HazardControlHead].CheckpointID != recoveryEvidence.Head.CheckpointID {
		return fmt.Errorf("recovery hazard controls do not preserve before/after/head and clean counterfactual coordinates")
	}
	if controls[HazardControlTreatment].EffectiveLogicalCoordinateID() != recoveryEvidence.Before.EffectiveLogicalCoordinateID() || controls[HazardControlFrontierLocal].EffectiveLogicalCoordinateID() != recoveryEvidence.After.EffectiveLogicalCoordinateID() || controls[HazardControlHead].EffectiveLogicalCoordinateID() != recoveryEvidence.Head.EffectiveLogicalCoordinateID() || controls[HazardControlRetentionAblation].EffectiveLogicalCoordinateID() != recoveryEvidence.Before.EffectiveLogicalCoordinateID() || controls[HazardControlCleanBaseline].EffectiveLogicalCoordinateID() != recoveryEvidence.Head.EffectiveLogicalCoordinateID() {
		return fmt.Errorf("recovery hazard controls do not preserve comparable before/after/head coordinates")
	}
	if controls[HazardControlTreatment].RuntimeInstanceID != recoveryEvidence.Before.RuntimeInstanceID || controls[HazardControlFrontierLocal].RuntimeInstanceID != recoveryEvidence.After.RuntimeInstanceID || controls[HazardControlHead].RuntimeInstanceID != recoveryEvidence.Head.RuntimeInstanceID {
		return fmt.Errorf("recovery hazard controls do not bind recovery execution runtime evidence")
	}
	if controls[HazardControlTreatment].StaticOutcome != recoveryEvidence.Before.StaticOutcome || controls[HazardControlFrontierLocal].StaticOutcome != recoveryEvidence.After.StaticOutcome || controls[HazardControlHead].StaticOutcome != recoveryEvidence.Head.StaticOutcome || controls[HazardControlRetentionAblation].StaticOutcome != HazardStaticOutcomeNotApplicable || controls[HazardControlCleanBaseline].StaticOutcome != HazardStaticOutcomeNotApplicable {
		return fmt.Errorf("recovery hazard controls have invalid static relation roles")
	}
	return nil
}

func classifyRecoveryHazard(report RecoveryHazardReport) (RecoveryHazardClass, RecoveryHazardStatus, string) {
	controls := controlsByName(report.Controls)
	treatment := controls[HazardControlTreatment]
	frontier := controls[HazardControlFrontierLocal]
	head := controls[HazardControlHead]
	ablation := controls[HazardControlRetentionAblation]
	baseline := controls[HazardControlCleanBaseline]
	if treatment.StaticOutcome != HazardStaticOutcomeResidual || frontier.StaticOutcome != HazardStaticOutcomeConsistent || head.StaticOutcome != HazardStaticOutcomeConsistent {
		return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "historical recovery static controls do not localize a residual before the write frontier"
	}
	ordered := []RecoveryHazardControl{treatment, frontier, head, ablation, baseline}
	for _, control := range ordered {
		if control.UseEvidence == nil {
			return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "typed recovery-time resolve/use evidence is incomplete"
		}
	}
	if treatment.UseEvidence.ListenerRole == treatment.ExpectedRole {
		return RecoveryHazardClassNone, RecoveryHazardStatusNotRealized, "historical treatment resolved to the role expected by restored logical state"
	}
	if frontier.UseEvidence.ListenerRole != frontier.ExpectedRole || head.UseEvidence.ListenerRole != head.ExpectedRole {
		return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "frontier-local or head control did not resolve the role expected after the write frontier"
	}
	if ablation.UseEvidence.ListenerRole != ablation.ExpectedRole || baseline.UseEvidence.ListenerRole != baseline.ExpectedRole {
		return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "clean retention or clean baseline control did not establish the normal role"
	}
	if treatment.UseEvidence.ListenerSemantic.Role == treatment.ExpectedRole || treatment.UseEvidence.ListenerSemantic.Role != treatment.UseEvidence.ListenerRole {
		return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "treatment listener semantic identity is incomplete or non-divergent"
	}
	tainted := environmentForControl(report, treatment)
	if tainted == nil || tainted.Program.Mutation.Operator != environment.MutationOperatorRebind || tainted.initialBinding().Semantic.Role != treatment.ExpectedRole || tainted.activeBinding().Semantic.Role != treatment.UseEvidence.ListenerRole {
		return RecoveryHazardClassNone, RecoveryHazardStatusInconclusive, "treatment lacks a verified rebind materialization witness"
	}
	if report.RecoveryEvidence.Source == HistoricalRecoveryEvidenceStateSeed && report.WriteEvidence.Mode == WriteEvidenceModeProfileEBPF && everyUseEvidenceMode(ordered, UseEvidenceModeEBPFResolved) {
		return RecoveryHazardClassRebound, RecoveryHazardStatusRealized, "historical recovery resolved and used a rebound Unix listener role"
	}
	return RecoveryHazardClassRebound, RecoveryHazardStatusRealizedCalibration, "fixture calibration resolved and used a rebound Unix listener role"
}

func everyUseEvidenceMode(controls []RecoveryHazardControl, mode UseEvidenceMode) bool {
	for _, control := range controls {
		if control.UseEvidence == nil || control.UseEvidence.EvidenceMode != mode {
			return false
		}
	}
	return true
}

func controlsByName(controls []RecoveryHazardControl) map[HazardControlName]RecoveryHazardControl {
	result := make(map[HazardControlName]RecoveryHazardControl, len(controls))
	for _, control := range controls {
		result[control.Name] = control
	}
	return result
}

func environmentForControl(report RecoveryHazardReport, control RecoveryHazardControl) *HazardEnvironmentInstance {
	for index := range report.Environments {
		if report.Environments[index].InstanceID == control.EnvironmentInstanceID {
			return &report.Environments[index]
		}
	}
	return nil
}

func recoveryHazardReportID(report RecoveryHazardReport) string {
	// A report ID is provenance, not a coarse finding key. In particular, two
	// clean runs may have the same semantic roles while carrying different
	// native checkpoint runs and run-local identities. Hash the complete
	// immutable evidence payload (except ReportID itself) so artifacts cannot
	// be substituted under a stable-looking five-control identifier.
	identity := struct {
		SchemaVersion    string                       `json:"schema_version"`
		Calibration      bool                         `json:"calibration"`
		Workload         Workload                     `json:"workload"`
		RecoveryEvidence HistoricalRecoveryEvidence   `json:"recovery_evidence"`
		WriteEvidence    MaterializationWriteEvidence `json:"write_evidence"`
		Environments     []HazardEnvironmentInstance  `json:"environments"`
		Controls         []RecoveryHazardControl      `json:"controls"`
		Class            RecoveryHazardClass          `json:"class"`
		Status           RecoveryHazardStatus         `json:"status"`
		Reason           string                       `json:"reason"`
	}{
		SchemaVersion:    report.SchemaVersion,
		Calibration:      report.Calibration,
		Workload:         report.Workload,
		RecoveryEvidence: report.RecoveryEvidence,
		WriteEvidence:    report.WriteEvidence,
		Environments:     report.Environments,
		Controls:         report.Controls,
		Class:            report.Class,
		Status:           report.Status,
		Reason:           report.Reason,
	}
	encoded, _ := json.Marshal(identity) // All fields are concrete JSON values.
	return "recovery-hazard-report:" + digest(string(encoded))
}

func validateRecoveryExecution(seed objective.StateSeed, set recovery.HistoricalRecoverySet, execution recovery.ForkRecoverySetExecution) error {
	if err := seed.Validate(); err != nil {
		return err
	}
	if err := set.ValidateFor(seed); err != nil {
		return err
	}
	if execution.SchemaVersion != recovery.ExecutionSchemaVersion || execution.RecoverySetID != set.RecoverySetID || execution.SeedID != seed.SeedID || execution.FrontierID != seed.FrontierID || execution.RecordedPlanID != set.RecordedPlanID || execution.RetentionPolicy != set.RetentionPolicy || execution.MaterializationHead.HeadID != set.MaterializationHead.HeadID {
		return fmt.Errorf("recovery evidence does not bind one historical recovery set execution")
	}
	plan := recovery.RecordedPlan{
		SchemaVersion:         recovery.SchemaVersion,
		RecordedPlanID:        seed.RecordedPlanID,
		AdapterID:             seed.AdapterID,
		TargetID:              seed.TargetID,
		ExecutionArtifact:     seed.RecordedPlanArtifact,
		PassiveObservationID:  set.PassiveObservationID,
		MaterializationHeadID: set.MaterializationHead.HeadID,
		RetentionPolicy:       set.RetentionPolicy,
	}
	for _, candidate := range []struct {
		query       recovery.RecoveryQuery
		observation recovery.RecoveryObservation
	}{
		{query: set.Before, observation: execution.Before},
		{query: set.After, observation: execution.After},
		{query: set.Head, observation: execution.Head},
	} {
		if err := candidate.observation.ValidateFor(candidate.query, plan); err != nil {
			return err
		}
	}
	if execution.Before.RuntimeInstanceID == execution.After.RuntimeInstanceID || execution.Before.RuntimeInstanceID == execution.Head.RuntimeInstanceID || execution.After.RuntimeInstanceID == execution.Head.RuntimeInstanceID {
		return fmt.Errorf("historical recovery execution reused a runtime instance")
	}
	expected := recovery.ClassifyForkRecoverySet(execution.Before, execution.After, execution.Head)
	if execution.Classification != expected {
		return fmt.Errorf("historical recovery execution classification does not match its observations")
	}
	return nil
}

func staticOutcome(value string) HazardStaticOutcome {
	switch value {
	case "residual":
		return HazardStaticOutcomeResidual
	case "consistent":
		return HazardStaticOutcomeConsistent
	default:
		return HazardStaticOutcomeInconclusive
	}
}

// classifyRecoveryObservation mirrors recovery's deliberately small static
// classifier without exporting or re-labeling that taxonomy. It exists only
// to embed a V2 execution as evidence in this higher-level report.
func classifyRecoveryObservation(observation recovery.RecoveryObservation) string {
	if !observation.AgentState.Valid() || !observation.OSState.Valid() || !observation.OSStateOrigin.Valid() || !observation.EffectMultiplicity.Valid() || observation.AgentState == recovery.StatePresenceUnknown || observation.OSState == recovery.StatePresenceUnknown || observation.OSStateOrigin == recovery.StateOriginUnknown || observation.EffectMultiplicity == recovery.EffectMultiplicityUnknown || len(observation.Evidence) == 0 {
		return "inconclusive"
	}
	if observation.EffectMultiplicity == recovery.EffectMultiplicityDuplicate {
		return "duplicate"
	}
	if observation.OSState == recovery.StatePresencePresent && observation.OSStateOrigin == recovery.StateOriginReconstructed {
		return "reconstruction"
	}
	if observation.AgentState == recovery.StatePresenceAbsent && observation.OSState == recovery.StatePresencePresent && observation.OSStateOrigin == recovery.StateOriginResidual {
		return "residual"
	}
	if observation.AgentState == recovery.StatePresencePresent && observation.OSState == recovery.StatePresenceAbsent {
		return "missing"
	}
	if observation.AgentState == observation.OSState && (observation.OSState == recovery.StatePresenceAbsent || observation.OSStateOrigin == recovery.StateOriginResidual || observation.OSStateOrigin == recovery.StateOriginNone) {
		return "consistent"
	}
	return "inconclusive"
}
