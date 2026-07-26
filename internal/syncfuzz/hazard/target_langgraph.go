package hazard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

// LangGraphTargetHazardInput joins two independently profiled, fresh source
// runtimes. The tainted run supplies treatment/frontier/head; the clean run
// supplies retention ablation and clean-head controls. They deliberately do
// not share PIDs, inodes, containers, or native checkpoint IDs.
type LangGraphTargetHazardInput struct {
	Workload Workload

	TaintedSeed              objective.StateSeed
	TaintedRecoverySet       recovery.HistoricalRecoverySet
	TaintedRecoveryExecution recovery.ForkRecoverySetExecution
	TaintedProgram           environment.EnvironmentProgram
	TaintedMaterialization   environment.TargetUnixSocketMaterialization

	CleanSeed              objective.StateSeed
	CleanRecoverySet       recovery.HistoricalRecoverySet
	CleanRecoveryExecution recovery.ForkRecoverySetExecution
	CleanProgram           environment.EnvironmentProgram
	CleanMaterialization   environment.TargetUnixSocketMaterialization
}

// BuildLangGraphTargetRecoveryHazardReport builds the five-control target
// report. It fails closed until every control has a completed-exchange U'
// record and both profiles expose equivalent structural before/after/head
// coordinates. It produces an evidence classification, never a vulnerability
// verdict.
func BuildLangGraphTargetRecoveryHazardReport(input LangGraphTargetHazardInput) (RecoveryHazardReport, error) {
	if err := input.Workload.Validate(); err != nil {
		return RecoveryHazardReport{}, err
	}
	if err := validateTargetHazardSeedPair(input); err != nil {
		return RecoveryHazardReport{}, err
	}
	if err := input.TaintedMaterialization.ValidateFor(input.TaintedProgram); err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("tainted target materialization: %w", err)
	}
	if err := input.CleanMaterialization.ValidateFor(input.CleanProgram); err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("clean target materialization: %w", err)
	}
	if input.TaintedProgram.Mutation.Operator != environment.MutationOperatorRebind || input.TaintedProgram.UnixSocket.InitialRole == input.TaintedProgram.UnixSocket.ActiveRole {
		return RecoveryHazardReport{}, fmt.Errorf("target treatment requires an explicit rebind environment program")
	}
	if input.CleanProgram.UnixSocket.InitialRole != input.CleanProgram.UnixSocket.ActiveRole {
		return RecoveryHazardReport{}, fmt.Errorf("target clean control requires a non-rebinding baseline environment program")
	}
	if err := validateTargetContinuation(input.Workload, input.TaintedRecoverySet, input.CleanRecoverySet); err != nil {
		return RecoveryHazardReport{}, err
	}
	taintedEvidence, err := NewHistoricalRecoveryEvidenceFromExecution(input.TaintedSeed, input.TaintedRecoverySet, input.TaintedRecoveryExecution)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("tainted historical recovery evidence: %w", err)
	}
	cleanEvidence, err := NewHistoricalRecoveryEvidenceFromExecution(input.CleanSeed, input.CleanRecoverySet, input.CleanRecoveryExecution)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("clean historical recovery evidence: %w", err)
	}
	taintedCoordinates, err := langGraphRecoveryCoordinateIDs(input.TaintedSeed, input.TaintedRecoverySet)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("tainted LangGraph coordinates: %w", err)
	}
	cleanCoordinates, err := langGraphRecoveryCoordinateIDs(input.CleanSeed, input.CleanRecoverySet)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("clean LangGraph coordinates: %w", err)
	}
	for _, name := range []string{"before", "after", "head"} {
		if taintedCoordinates[name] != cleanCoordinates[name] {
			return RecoveryHazardReport{}, fmt.Errorf("clean and tainted runs do not have equivalent %s LangGraph recovery coordinate", name)
		}
	}
	taintedEvidence.Before.LogicalCoordinateID = taintedCoordinates["before"]
	taintedEvidence.After.LogicalCoordinateID = taintedCoordinates["after"]
	taintedEvidence.Head.LogicalCoordinateID = taintedCoordinates["head"]
	cleanEvidence.Before.LogicalCoordinateID = cleanCoordinates["before"]
	cleanEvidence.After.LogicalCoordinateID = cleanCoordinates["after"]
	cleanEvidence.Head.LogicalCoordinateID = cleanCoordinates["head"]
	if err := taintedEvidence.Validate(); err != nil {
		return RecoveryHazardReport{}, err
	}
	if err := cleanEvidence.Validate(); err != nil {
		return RecoveryHazardReport{}, err
	}
	requestSHA256, err := targetRecoveryRequestDigest(input)
	if err != nil {
		return RecoveryHazardReport{}, err
	}
	taintedPlan, err := NewUnixSocketRecoveryUsePlanFromObservedDigest(input.Workload, input.TaintedProgram, requestSHA256)
	if err != nil {
		return RecoveryHazardReport{}, err
	}
	cleanPlan, err := NewUnixSocketRecoveryUsePlanFromObservedDigest(input.Workload, input.CleanProgram, requestSHA256)
	if err != nil {
		return RecoveryHazardReport{}, err
	}
	treatmentUse, err := targetUseForObservation(input.Workload, taintedPlan, input.TaintedProgram, input.TaintedMaterialization, input.TaintedRecoveryExecution.Before)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("treatment U': %w", err)
	}
	afterUse, err := targetUseForObservation(input.Workload, taintedPlan, input.TaintedProgram, input.TaintedMaterialization, input.TaintedRecoveryExecution.After)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("frontier-local U': %w", err)
	}
	headUse, err := targetUseForObservation(input.Workload, taintedPlan, input.TaintedProgram, input.TaintedMaterialization, input.TaintedRecoveryExecution.Head)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("head U': %w", err)
	}
	ablationUse, err := targetUseForObservation(input.Workload, cleanPlan, input.CleanProgram, input.CleanMaterialization, input.CleanRecoveryExecution.Before)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("retention-ablation U': %w", err)
	}
	baselineUse, err := targetUseForObservation(input.Workload, cleanPlan, input.CleanProgram, input.CleanMaterialization, input.CleanRecoveryExecution.Head)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("clean-baseline U': %w", err)
	}
	return BuildRecoveryHazardReport(RecoveryHazardReportInput{
		Calibration:      false,
		Workload:         input.Workload,
		RecoveryEvidence: taintedEvidence,
		WriteEvidence: MaterializationWriteEvidence{
			Mode:       WriteEvidenceModeProfileEBPF,
			FrontierID: taintedEvidence.FrontierID,
			Operations: []string{"bind", "listen", "rebind"},
		},
		Environments: []HazardEnvironmentInstance{
			{InstanceID: "tainted-head", Program: input.TaintedProgram, TargetMaterialization: &input.TaintedMaterialization},
			{InstanceID: "clean-head", Program: input.CleanProgram, TargetMaterialization: &input.CleanMaterialization},
		},
		Controls: []RecoveryHazardControl{
			{Name: HazardControlTreatment, CheckpointID: taintedEvidence.Before.CheckpointID, LogicalCoordinateID: taintedCoordinates["before"], RuntimeInstanceID: taintedEvidence.Before.RuntimeInstanceID, StaticOutcome: taintedEvidence.Before.StaticOutcome, EnvironmentInstanceID: "tainted-head", ExpectedRole: input.TaintedProgram.UnixSocket.InitialRole, UsePlan: taintedPlan, UseEvidence: &treatmentUse},
			{Name: HazardControlFrontierLocal, CheckpointID: taintedEvidence.After.CheckpointID, LogicalCoordinateID: taintedCoordinates["after"], RuntimeInstanceID: taintedEvidence.After.RuntimeInstanceID, StaticOutcome: taintedEvidence.After.StaticOutcome, EnvironmentInstanceID: "tainted-head", ExpectedRole: input.TaintedProgram.UnixSocket.ActiveRole, UsePlan: taintedPlan, UseEvidence: &afterUse},
			{Name: HazardControlHead, CheckpointID: taintedEvidence.Head.CheckpointID, LogicalCoordinateID: taintedCoordinates["head"], RuntimeInstanceID: taintedEvidence.Head.RuntimeInstanceID, StaticOutcome: taintedEvidence.Head.StaticOutcome, EnvironmentInstanceID: "tainted-head", ExpectedRole: input.TaintedProgram.UnixSocket.ActiveRole, UsePlan: taintedPlan, UseEvidence: &headUse},
			{Name: HazardControlRetentionAblation, CheckpointID: cleanEvidence.Before.CheckpointID, LogicalCoordinateID: cleanCoordinates["before"], RuntimeInstanceID: cleanEvidence.Before.RuntimeInstanceID, StaticOutcome: HazardStaticOutcomeNotApplicable, EnvironmentInstanceID: "clean-head", ExpectedRole: input.CleanProgram.UnixSocket.ActiveRole, UsePlan: cleanPlan, UseEvidence: &ablationUse},
			{Name: HazardControlCleanBaseline, CheckpointID: cleanEvidence.Head.CheckpointID, LogicalCoordinateID: cleanCoordinates["head"], RuntimeInstanceID: cleanEvidence.Head.RuntimeInstanceID, StaticOutcome: HazardStaticOutcomeNotApplicable, EnvironmentInstanceID: "clean-head", ExpectedRole: input.CleanProgram.UnixSocket.ActiveRole, UsePlan: cleanPlan, UseEvidence: &baselineUse},
		},
	})
}

// targetRecoveryRequestDigest proves that the normal use is byte-stable
// across all executed controls without copying those bytes out of the target.
// A changing request would add an uncontrolled variable to a five-control
// comparison, so the target report rejects it rather than guessing semantics.
func targetRecoveryRequestDigest(input LangGraphTargetHazardInput) (string, error) {
	observations := []recovery.RecoveryObservation{
		input.TaintedRecoveryExecution.Before,
		input.TaintedRecoveryExecution.After,
		input.TaintedRecoveryExecution.Head,
		input.CleanRecoveryExecution.Before,
		input.CleanRecoveryExecution.After,
		input.CleanRecoveryExecution.Head,
	}
	digest := ""
	for _, observation := range observations {
		if observation.EnvironmentUseEvidence == nil {
			return "", fmt.Errorf("recovery observation %q lacks typed environment use evidence", observation.QueryID)
		}
		if err := observation.EnvironmentUseEvidence.Validate(); err != nil {
			return "", fmt.Errorf("recovery observation %q has invalid typed environment use evidence: %w", observation.QueryID, err)
		}
		observed := observation.EnvironmentUseEvidence.RequestSHA256
		if digest == "" {
			digest = observed
		} else if digest != observed {
			return "", fmt.Errorf("target recovery controls did not issue one byte-stable normal request")
		}
	}
	return digest, nil
}

func targetUseForObservation(workload Workload, plan RecoveryUsePlan, program environment.EnvironmentProgram, materialization environment.TargetUnixSocketMaterialization, observation recovery.RecoveryObservation) (UnixSocketUseEvidence, error) {
	if observation.EnvironmentUseEvidence == nil {
		return UnixSocketUseEvidence{}, fmt.Errorf("recovery observation %q lacks typed environment use evidence", observation.QueryID)
	}
	return NewUnixSocketUseEvidenceFromTargetRecovery(workload, plan, program, materialization, *observation.EnvironmentUseEvidence)
}

func validateTargetHazardSeedPair(input LangGraphTargetHazardInput) error {
	for _, seed := range []objective.StateSeed{input.TaintedSeed, input.CleanSeed} {
		if err := seed.Validate(); err != nil {
			return err
		}
		if seed.AdapterID != recovery.LangGraphForkAdapterID || seed.TargetID != "langgraph-shell-react" {
			return fmt.Errorf("target hazard report requires LangGraph target StateSeeds")
		}
	}
	if input.TaintedSeed.ObjectiveID != input.CleanSeed.ObjectiveID || input.TaintedSeed.SynthesisCandidateID != input.CleanSeed.SynthesisCandidateID || input.TaintedSeed.NativeCheckpointRunID == input.CleanSeed.NativeCheckpointRunID {
		return fmt.Errorf("target hazard report requires distinct clean/tainted runs for the same objective and synthesis candidate")
	}
	if !seedValidatesBindListen(input.TaintedSeed) || !seedValidatesBindListen(input.CleanSeed) {
		return fmt.Errorf("target hazard report requires StateSeeds validated for Unix bind/listen")
	}
	return nil
}

func seedValidatesBindListen(seed objective.StateSeed) bool {
	bind, listen := false, false
	for _, effect := range seed.ValidatedEffects {
		if effect.Family != profiling.StateFamilyIPC {
			continue
		}
		bind = bind || effect.Operation == "bind"
		listen = listen || effect.Operation == "listen"
	}
	return bind && listen
}

func validateTargetContinuation(workload Workload, tainted, clean recovery.HistoricalRecoverySet) error {
	if tainted.ContinuationQuery == nil || clean.ContinuationQuery == nil || tainted.ContinuationQuery.Query != workload.ContinuationPrompt || clean.ContinuationQuery.Query != workload.ContinuationPrompt || tainted.ContinuationQuery.ContinuationQueryID != clean.ContinuationQuery.ContinuationQueryID || tainted.ContinuationQuery.QuerySHA256 != clean.ContinuationQuery.QuerySHA256 {
		return fmt.Errorf("target hazard report requires one identical frozen workload continuation")
	}
	return nil
}

func langGraphRecoveryCoordinateIDs(seed objective.StateSeed, set recovery.HistoricalRecoverySet) (map[string]string, error) {
	plan, err := recovery.ReadLangGraphForkPlan(seed.RecordedPlanArtifact)
	if err != nil {
		return nil, err
	}
	queries := map[string]recovery.RecoveryQuery{"before": set.Before, "after": set.After, "head": set.Head}
	identities := make(map[string]string, len(queries))
	for name, query := range queries {
		coordinate, found := plan.CheckpointCoordinates[query.CheckpointID]
		if !found {
			return nil, fmt.Errorf("fork plan has no native coordinate for %s checkpoint %q", name, query.CheckpointID)
		}
		if err := coordinate.Validate(); err != nil {
			return nil, err
		}
		identity := fmt.Sprintf("langgraph-coordinate:v1:%d:%d:%s", coordinate.HistoryIndex, coordinate.MessageCount, strings.Join(coordinate.Next, "\x1f"))
		digest := sha256.Sum256([]byte(identity))
		identities[name] = "langgraph-coordinate:" + hex.EncodeToString(digest[:])
	}
	if identities["before"] == identities["after"] || identities["before"] == identities["head"] || identities["after"] == identities["head"] {
		return nil, fmt.Errorf("LangGraph recovery set does not expose three distinct structural coordinates")
	}
	return identities, nil
}
