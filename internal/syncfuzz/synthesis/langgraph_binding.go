package synthesis

import (
	"fmt"
	"path/filepath"
	"sort"
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

// InferLangGraphNativeCheckpointManifestPath resolves the target-owned native
// manifest colocated with the ProfileRun's immutable recorded target plan.
// Callers may still supply an explicit manifest path for artifact import, but
// the standard profile-to-recovery workflow must not require a hand-copied
// target run ID.
func InferLangGraphNativeCheckpointManifestPath(run objective.ProfileRun) (string, error) {
	planArtifact := strings.TrimSpace(run.RecordedPlanArtifact)
	if planArtifact == "" {
		return "", fmt.Errorf("LangGraph profile run lacks a recorded target plan artifact")
	}
	return filepath.Join(filepath.Dir(planArtifact), LangGraphNativeCheckpointManifestArtifact), nil
}

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
	BeforeNativeToolLifecycle *LangGraphDurableToolLifecycle      `json:"before_native_tool_lifecycle,omitempty"`
	AfterNativeToolLifecycle  *LangGraphDurableToolLifecycle      `json:"after_native_tool_lifecycle,omitempty"`
	ToolEffectProvenance      *LangGraphToolEffectProvenance      `json:"tool_effect_provenance,omitempty"`
	ObservedWorkspaceFilePath string                              `json:"observed_workspace_file_path,omitempty"`
	ObjectiveEffectIDs        []string                            `json:"objective_effect_ids,omitempty"`
	ManifestArtifact          string                              `json:"manifest_artifact"`
}

// LangGraphNativeFrontierBindingConfig scopes a broad effect atom to an
// adapter-owned observable resource when the objective grammar alone cannot
// distinguish it from target wrapper activity. It is evidence selection, not
// a task-success Oracle: the path comes from the scaffold resource contract.
type LangGraphNativeFrontierBindingConfig struct {
	ObservedWorkspaceFilePath string
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
	if b.BeforeNativeToolLifecycle != nil {
		if err := b.BeforeNativeToolLifecycle.Validate(); err != nil {
			return fmt.Errorf("before native durable tool lifecycle: %w", err)
		}
	}
	if b.AfterNativeToolLifecycle != nil {
		if err := b.AfterNativeToolLifecycle.Validate(); err != nil {
			return fmt.Errorf("after native durable tool lifecycle: %w", err)
		}
	}
	if b.ToolEffectProvenance != nil {
		if err := b.ToolEffectProvenance.Validate(); err != nil {
			return fmt.Errorf("LangGraph tool-effect provenance: %w", err)
		}
		if b.ToolEffectProvenance.FirstEffectMonotonicNS != b.FirstEffectMonotonicNS || b.ToolEffectProvenance.LastEffectMonotonicNS != b.LastEffectMonotonicNS {
			return fmt.Errorf("LangGraph tool-effect provenance does not match the binding effect interval")
		}
	}
	if strings.TrimSpace(b.ObservedWorkspaceFilePath) != "" {
		if _, err := langGraphWorkspaceCanonicalPath(b.ObservedWorkspaceFilePath); err != nil {
			return fmt.Errorf("LangGraph native frontier binding workspace file path: %w", err)
		}
		if len(b.ObjectiveEffectIDs) == 0 {
			return fmt.Errorf("LangGraph native frontier binding has no scoped objective effects")
		}
	}
	seenEffects := make(map[string]struct{}, len(b.ObjectiveEffectIDs))
	for _, effectID := range b.ObjectiveEffectIDs {
		effectID = strings.TrimSpace(effectID)
		if effectID == "" {
			return fmt.Errorf("LangGraph native frontier binding has an empty objective effect ID")
		}
		if _, exists := seenEffects[effectID]; exists {
			return fmt.Errorf("LangGraph native frontier binding repeats objective effect %q", effectID)
		}
		seenEffects[effectID] = struct{}{}
	}
	return nil
}

// BindLangGraphNativeFrontier maps one validated controller frontier to the
// closest exact LangGraph checkpoint persisted before its first required atom
// and the closest one persisted after its last required atom. It rejects an
// ordinary checkpoint-history listing without monotonic persistence evidence.
func BindLangGraphNativeFrontier(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, frontierID string, manifestPath string, manifest LangGraphNativeCheckpointManifest) (LangGraphNativeFrontierBinding, error) {
	return BindLangGraphNativeFrontierWithLifecycleAndConfig(stateObjective, candidate, run, frontierID, manifestPath, manifest, nil, LangGraphNativeFrontierBindingConfig{})
}

// BindLangGraphNativeFrontierWithLifecycle adds optional command-span
// provenance to the native checkpoint binding. A missing or ambiguous span is
// represented by a nil provenance field, never by a guessed tool call.
func BindLangGraphNativeFrontierWithLifecycle(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, frontierID string, manifestPath string, manifest LangGraphNativeCheckpointManifest, lifecycle *LangGraphLifecycleArtifact) (LangGraphNativeFrontierBinding, error) {
	return BindLangGraphNativeFrontierWithLifecycleAndConfig(stateObjective, candidate, run, frontierID, manifestPath, manifest, lifecycle, LangGraphNativeFrontierBindingConfig{})
}

// BindLangGraphNativeFrontierWithLifecycleAndConfig adds a target-owned
// resource scope to lifecycle-aware native binding. The scope prevents a
// generic atom such as handle/open from absorbing wrapper artifact writes in
// the same controller frontier.
func BindLangGraphNativeFrontierWithLifecycleAndConfig(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, frontierID string, manifestPath string, manifest LangGraphNativeCheckpointManifest, lifecycle *LangGraphLifecycleArtifact, config LangGraphNativeFrontierBindingConfig) (LangGraphNativeFrontierBinding, error) {
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
	if err := config.ValidateFor(stateObjective); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	frontier, ok := profileFrontier(run, frontierID)
	if !ok || !frontier.IsFrontier || !frontier.PersistentDelta.Changed() || len(frontier.EvidenceLinks) == 0 || frontier.StartMonotonicNS == 0 || frontier.EndMonotonicNS <= frontier.StartMonotonicNS {
		return LangGraphNativeFrontierBinding{}, fmt.Errorf("profile run has no timestamped validated persistent frontier %q", frontierID)
	}
	firstEffect, lastEffect, objectiveEffectIDs, err := linkedObjectiveEffectWindow(frontier, stateObjective, config)
	if err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	before, after, err := nativeCheckpointsAroundEffect(manifest, frontier, firstEffect, lastEffect)
	if err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	var toolEffectProvenance *LangGraphToolEffectProvenance
	if lifecycle != nil {
		toolEffectProvenance, err = lifecycle.ToolEffectProvenance(firstEffect, lastEffect)
		if err != nil {
			return LangGraphNativeFrontierBinding{}, err
		}
		if toolEffectProvenance != nil && !nativeCheckpointRecordsToolResult(after, *toolEffectProvenance) {
			// The shell span can still be real, but without a durable result in
			// the after checkpoint it cannot support a tool/checkpoint relation.
			toolEffectProvenance = nil
		}
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
		BeforeNativeToolLifecycle: cloneLangGraphDurableToolLifecycle(before.DurableToolLifecycle),
		AfterNativeToolLifecycle:  cloneLangGraphDurableToolLifecycle(after.DurableToolLifecycle),
		ToolEffectProvenance:      toolEffectProvenance,
		ObservedWorkspaceFilePath: strings.TrimSpace(config.ObservedWorkspaceFilePath),
		ObjectiveEffectIDs:        objectiveEffectIDs,
		ManifestArtifact:          manifestPath,
	}
	if err := binding.Validate(); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	return binding, nil
}

func (c LangGraphNativeFrontierBindingConfig) ValidateFor(stateObjective objective.StateObjective) error {
	path := strings.TrimSpace(c.ObservedWorkspaceFilePath)
	if path == "" {
		return nil
	}
	if len(stateObjective.Effects) != 1 || stateObjective.Effects[0].Family != profiling.StateFamilyHandle || stateObjective.Effects[0].Operation != "open" {
		return fmt.Errorf("LangGraph workspace file scope requires a single handle/open objective")
	}
	if _, err := langGraphWorkspaceCanonicalPath(path); err != nil {
		return fmt.Errorf("LangGraph workspace file scope: %w", err)
	}
	return nil
}

func nativeCheckpointRecordsToolResult(checkpoint LangGraphNativeCheckpoint, provenance LangGraphToolEffectProvenance) bool {
	lifecycle := checkpoint.DurableToolLifecycle
	if lifecycle == nil {
		return false
	}
	hasCall := false
	for _, call := range lifecycle.ToolCalls {
		if call.ToolCallID == provenance.ToolCallID && call.ToolName == provenance.ToolName {
			hasCall = true
			break
		}
	}
	if !hasCall {
		return false
	}
	for _, resultID := range lifecycle.ToolResultIDs {
		if resultID == provenance.ToolCallID {
			return true
		}
	}
	return false
}

func cloneLangGraphDurableToolLifecycle(source *LangGraphDurableToolLifecycle) *LangGraphDurableToolLifecycle {
	if source == nil {
		return nil
	}
	clone := source.Clone()
	return &clone
}

func nativeCheckpointCoordinate(checkpoint LangGraphNativeCheckpoint) LangGraphNativeCheckpointCoordinate {
	// A nil slice encodes as JSON null, while the target contract requires an
	// array even when a terminal checkpoint has no next nodes.
	next := make([]string, len(checkpoint.Next))
	copy(next, checkpoint.Next)
	return LangGraphNativeCheckpointCoordinate{
		SchemaVersion:      LangGraphNativeCoordinateSchema,
		SourceCheckpointID: checkpoint.CheckpointID,
		HistoryIndex:       checkpoint.HistoryIndex,
		MessageCount:       checkpoint.MessageCount,
		Next:               next,
	}
}

func linkedObjectiveEffectWindow(frontier profiling.CheckpointInterval, stateObjective objective.StateObjective, config LangGraphNativeFrontierBindingConfig) (uint64, uint64, []string, error) {
	linked := make(map[string]struct{}, len(frontier.EvidenceLinks))
	linkedWorkspaceFileEffects := make(map[string]struct{})
	workspaceFilePath := ""
	if strings.TrimSpace(config.ObservedWorkspaceFilePath) != "" {
		var err error
		workspaceFilePath, err = langGraphWorkspaceCanonicalPath(config.ObservedWorkspaceFilePath)
		if err != nil {
			return 0, 0, nil, err
		}
	}
	effectsByID := make(map[string]profiling.NormalizedEffect, len(frontier.Effects))
	for _, effect := range frontier.Effects {
		effectsByID[effect.EffectID] = effect
	}
	for _, link := range frontier.EvidenceLinks {
		linked[link.EffectID] = struct{}{}
		if workspaceFilePath == "" || link.Relation != profiling.EvidenceLinkExactCanonicalPath {
			continue
		}
		effect, ok := effectsByID[link.EffectID]
		if ok && effect.Resource.CanonicalPath == workspaceFilePath {
			linkedWorkspaceFileEffects[link.EffectID] = struct{}{}
		}
	}
	found := make(map[string]bool, len(stateObjective.Effects))
	objectiveEffectIDs := make([]string, 0)
	var first uint64
	var last uint64
	for _, effect := range frontier.Effects {
		if _, ok := linked[effect.EffectID]; !ok {
			continue
		}
		if workspaceFilePath != "" {
			if _, ok := linkedWorkspaceFileEffects[effect.EffectID]; !ok {
				continue
			}
		}
		for _, atom := range stateObjective.CanonicalEffects() {
			if effect.Family != atom.Family || effect.Operation != atom.Operation {
				continue
			}
			if effect.MonotonicNS == 0 {
				return 0, 0, nil, fmt.Errorf("validated objective effect %q has no monotonic timestamp", effect.EffectID)
			}
			key := string(atom.Family) + "\x00" + atom.Operation
			found[key] = true
			if first == 0 || effect.MonotonicNS < first {
				first = effect.MonotonicNS
			}
			if effect.MonotonicNS > last {
				last = effect.MonotonicNS
			}
			objectiveEffectIDs = append(objectiveEffectIDs, effect.EffectID)
			break
		}
	}
	for _, atom := range stateObjective.CanonicalEffects() {
		key := string(atom.Family) + "\x00" + atom.Operation
		if !found[key] {
			return 0, 0, nil, fmt.Errorf("frontier %q has no linked objective effect %s/%s", frontier.FrontierID, atom.Family, atom.Operation)
		}
	}
	if first <= frontier.StartMonotonicNS || last > frontier.EndMonotonicNS {
		return 0, 0, nil, fmt.Errorf("linked objective effect window does not lie inside frontier %q", frontier.FrontierID)
	}
	sort.Strings(objectiveEffectIDs)
	return first, last, objectiveEffectIDs, nil
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
