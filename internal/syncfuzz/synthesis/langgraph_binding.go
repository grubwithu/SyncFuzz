package synthesis

import (
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const (
	LangGraphNativeFrontierBindingSchema = "syncfuzz.langgraph-native-frontier-binding.v1"
	LangGraphNativeCoordinateSchema      = recovery.LangGraphNativeCoordinateSchema
)

type LangGraphNativeCheckpointCoordinate = recovery.LangGraphNativeCheckpointCoordinate

// LangGraphNativeFrontierBinding proves one profile frontier brackets a
// specific pair of durable LangGraph checkpoint writes. Controller checkpoints
// remain profiling evidence; only the two exact native IDs may later be used
// by a LangGraph fork adapter.
type LangGraphNativeFrontierBinding struct {
	SchemaVersion             string                              `json:"schema_version"`
	CandidateID               string                              `json:"candidate_id"`
	ProfileRunID              string                              `json:"profile_run_id"`
	NativeCheckpointRunID     string                              `json:"native_checkpoint_run_id"`
	FrontierID                string                              `json:"frontier_id"`
	BeforeProfileCheckpointID string                              `json:"before_profile_checkpoint_id"`
	AfterProfileCheckpointID  string                              `json:"after_profile_checkpoint_id"`
	FirstEffectMonotonicNS    uint64                              `json:"first_effect_monotonic_ns"`
	LastEffectMonotonicNS     uint64                              `json:"last_effect_monotonic_ns"`
	BeforeNativeCheckpointID  string                              `json:"before_native_checkpoint_id"`
	AfterNativeCheckpointID   string                              `json:"after_native_checkpoint_id"`
	BeforeNativeMonotonicNS   uint64                              `json:"before_native_monotonic_ns"`
	AfterNativeMonotonicNS    uint64                              `json:"after_native_monotonic_ns"`
	BeforeNativeCoordinate    LangGraphNativeCheckpointCoordinate `json:"before_native_coordinate"`
	AfterNativeCoordinate     LangGraphNativeCheckpointCoordinate `json:"after_native_coordinate"`
	ManifestArtifact          string                              `json:"manifest_artifact"`
}

func (b LangGraphNativeFrontierBinding) Validate() error {
	if b.SchemaVersion != LangGraphNativeFrontierBindingSchema || strings.TrimSpace(b.CandidateID) == "" || strings.TrimSpace(b.ProfileRunID) == "" || strings.TrimSpace(b.NativeCheckpointRunID) == "" || strings.TrimSpace(b.FrontierID) == "" || strings.TrimSpace(b.BeforeProfileCheckpointID) == "" || strings.TrimSpace(b.AfterProfileCheckpointID) == "" || strings.TrimSpace(b.BeforeNativeCheckpointID) == "" || strings.TrimSpace(b.AfterNativeCheckpointID) == "" || strings.TrimSpace(b.ManifestArtifact) == "" {
		return fmt.Errorf("LangGraph native frontier binding is incomplete")
	}
	if b.FirstEffectMonotonicNS == 0 || b.LastEffectMonotonicNS < b.FirstEffectMonotonicNS || b.BeforeNativeMonotonicNS == 0 || b.AfterNativeMonotonicNS == 0 {
		return fmt.Errorf("LangGraph native frontier binding has invalid monotonic coordinates")
	}
	if b.BeforeProfileCheckpointID == b.AfterProfileCheckpointID || b.BeforeNativeCheckpointID == b.AfterNativeCheckpointID {
		return fmt.Errorf("LangGraph native frontier binding must retain distinct before/after checkpoints")
	}
	if b.BeforeNativeMonotonicNS >= b.FirstEffectMonotonicNS || b.AfterNativeMonotonicNS <= b.LastEffectMonotonicNS {
		return fmt.Errorf("LangGraph native frontier binding does not bracket the validated effect interval")
	}
	if err := b.BeforeNativeCoordinate.Validate(); err != nil {
		return fmt.Errorf("before native coordinate: %w", err)
	}
	if err := b.AfterNativeCoordinate.Validate(); err != nil {
		return fmt.Errorf("after native coordinate: %w", err)
	}
	if b.BeforeNativeCoordinate.SourceCheckpointID != b.BeforeNativeCheckpointID || b.AfterNativeCoordinate.SourceCheckpointID != b.AfterNativeCheckpointID {
		return fmt.Errorf("LangGraph native frontier binding coordinate does not match native checkpoint ID")
	}
	return nil
}

// BindLangGraphNativeFrontier maps one validated controller frontier to the
// closest exact LangGraph checkpoint persisted before its first required atom
// and the closest one persisted after its last required atom. It rejects an
// ordinary checkpoint-history listing without monotonic persistence evidence.
func BindLangGraphNativeFrontier(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, frontierID string, manifestPath string, manifest LangGraphNativeCheckpointManifest) (LangGraphNativeFrontierBinding, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	if err := run.ValidateFor(stateObjective); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	if run.SynthesisCandidateID != candidate.CandidateID || candidate.TargetID != LangGraphSynthesisTargetID || candidate.AdapterID != LangGraphSynthesisAdapterID || run.TargetID != candidate.TargetID || run.AdapterID != candidate.AdapterID {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("LangGraph native binding requires a matching LangGraph synthesis candidate and profile run")
	}
	if err := manifest.Validate(); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	if manifest.ClockDomain != "CLOCK_MONOTONIC" {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("LangGraph native manifest clock domain %q cannot be joined to controller/eBPF monotonic evidence", manifest.ClockDomain)
	}
	if run.NativeCheckpointRunID != manifest.InitialRuntimeInstanceID {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("profile native checkpoint runtime %q does not match LangGraph manifest runtime %q", run.NativeCheckpointRunID, manifest.InitialRuntimeInstanceID)
	}
	if strings.TrimSpace(manifestPath) == "" {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("LangGraph native binding requires a manifest artifact path")
	}
	frontier, ok := profileFrontier(run, frontierID)
	if !ok || !frontier.IsFrontier || !frontier.PersistentDelta.Changed() || len(frontier.EvidenceLinks) == 0 || frontier.StartMonotonicNS == 0 || frontier.EndMonotonicNS <= frontier.StartMonotonicNS {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("profile run has no timestamped validated persistent frontier %q", frontierID)
	}
	firstEffect, lastEffect, err := linkedObjectiveEffectWindow(frontier, stateObjective)
	if err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	before, after, err := nativeCheckpointsAroundEffect(manifest, frontier, firstEffect, lastEffect)
	if err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	binding := LangGraphNativeFrontierBinding{
		SchemaVersion:             LangGraphNativeFrontierBindingSchema,
		CandidateID:               candidate.CandidateID,
		ProfileRunID:              run.ProfileRunID,
		NativeCheckpointRunID:     run.NativeCheckpointRunID,
		FrontierID:                frontier.FrontierID,
		BeforeProfileCheckpointID: frontier.BeforeCheckpointID,
		AfterProfileCheckpointID:  frontier.AfterCheckpointID,
		FirstEffectMonotonicNS:    firstEffect,
		LastEffectMonotonicNS:     lastEffect,
		BeforeNativeCheckpointID:  before.CheckpointID,
		AfterNativeCheckpointID:   after.CheckpointID,
		BeforeNativeMonotonicNS:   before.PersistedMonotonicNS,
		AfterNativeMonotonicNS:    after.PersistedMonotonicNS,
		BeforeNativeCoordinate:    nativeCheckpointCoordinate(before),
		AfterNativeCoordinate:     nativeCheckpointCoordinate(after),
		ManifestArtifact:          manifestPath,
	}
	if err := binding.Validate(); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	return binding, nil
}

func nativeCheckpointCoordinate(checkpoint LangGraphNativeCheckpoint) LangGraphNativeCheckpointCoordinate {
	return LangGraphNativeCheckpointCoordinate{
		SchemaVersion:      LangGraphNativeCoordinateSchema,
		SourceCheckpointID: checkpoint.CheckpointID,
		HistoryIndex:       checkpoint.HistoryIndex,
		MessageCount:       checkpoint.MessageCount,
		Next:               append([]string(nil), checkpoint.Next...),
	}
}

func linkedObjectiveEffectWindow(frontier profiling.CheckpointInterval, stateObjective objective.StateObjective) (uint64, uint64, error) {
	linked := make(map[string]struct{}, len(frontier.EvidenceLinks))
	for _, link := range frontier.EvidenceLinks {
		linked[link.EffectID] = struct{}{}
	}
	found := make(map[string]bool, len(stateObjective.Effects))
	var first uint64
	var last uint64
	for _, effect := range frontier.Effects {
		if _, ok := linked[effect.EffectID]; !ok {
			continue
		}
		for _, atom := range stateObjective.CanonicalEffects() {
			if effect.Family != atom.Family || effect.Operation != atom.Operation {
				continue
			}
			if effect.MonotonicNS == 0 {
				return 0, 0, fmt.Errorf("validated objective effect %q has no monotonic timestamp", effect.EffectID)
			}
			key := string(atom.Family) + "\x00" + atom.Operation
			found[key] = true
			if first == 0 || effect.MonotonicNS < first {
				first = effect.MonotonicNS
			}
			if effect.MonotonicNS > last {
				last = effect.MonotonicNS
			}
		}
	}
	for _, atom := range stateObjective.CanonicalEffects() {
		key := string(atom.Family) + "\x00" + atom.Operation
		if !found[key] {
			return 0, 0, fmt.Errorf("frontier %q has no linked objective effect %s/%s", frontier.FrontierID, atom.Family, atom.Operation)
		}
	}
	if first <= frontier.StartMonotonicNS || last > frontier.EndMonotonicNS {
		return 0, 0, fmt.Errorf("linked objective effect window does not lie inside frontier %q", frontier.FrontierID)
	}
	return first, last, nil
}

func nativeCheckpointsAroundEffect(manifest LangGraphNativeCheckpointManifest, frontier profiling.CheckpointInterval, firstEffect uint64, lastEffect uint64) (LangGraphNativeCheckpoint, LangGraphNativeCheckpoint, error) {
	var before LangGraphNativeCheckpoint
	var after LangGraphNativeCheckpoint
	for _, checkpoint := range manifest.NativeCheckpoints {
		timestamp := checkpoint.PersistedMonotonicNS
		if timestamp == 0 {
			continue
		}
		if timestamp <= frontier.StartMonotonicNS || timestamp > frontier.EndMonotonicNS {
			continue
		}
		if timestamp < firstEffect && (before.PersistedMonotonicNS == 0 || timestamp > before.PersistedMonotonicNS) {
			before = checkpoint
		}
		if timestamp > lastEffect && (after.PersistedMonotonicNS == 0 || timestamp < after.PersistedMonotonicNS) {
			after = checkpoint
		}
	}
	if before.PersistedMonotonicNS == 0 || after.PersistedMonotonicNS == 0 {
		return LangGraphNativeCheckpoint{}, LangGraphNativeCheckpoint{}, fmt.Errorf("LangGraph native manifest does not prove durable checkpoints bracketing objective effects %d..%d", firstEffect, lastEffect)
	}
	return before, after, nil
}
