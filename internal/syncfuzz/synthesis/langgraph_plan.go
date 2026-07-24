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

type LangGraphForkPlanConfig struct {
	Model                 string
	ContainerImage        string
	RuntimeRoot           string
	PassiveUnixSocketPath string
	PassiveProbeMode      recovery.LangGraphPassiveProbeMode
}

// PrepareLangGraphForkPlan turns a timestamp-validated native binding into an
// immutable recovery plan. It retains source checkpoint IDs as provenance but
// gives a future fresh runtime only structural coordinates to resolve.
func PrepareLangGraphForkPlan(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, binding LangGraphNativeFrontierBinding, config LangGraphForkPlanConfig) (recovery.LangGraphForkPlan, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if err := run.ValidateFor(stateObjective); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if err := binding.Validate(); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if binding.CandidateID != candidate.CandidateID || binding.ProfileRunID != run.ProfileRunID || binding.NativeCheckpointRunID != run.NativeCheckpointRunID || binding.FrontierID == "" || run.AdapterID != recovery.LangGraphForkAdapterID || candidate.AdapterID != recovery.LangGraphForkAdapterID || run.TargetID != candidate.TargetID {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph native binding does not match the candidate/profile recovery identity")
	}
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.ContainerImage) == "" || strings.TrimSpace(config.RuntimeRoot) == "" || strings.TrimSpace(config.PassiveUnixSocketPath) == "" {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph fork plan requires model, container image, runtime root, and passive Unix socket path")
	}
	probeMode := config.PassiveProbeMode.Effective()
	if !probeMode.Valid() {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph fork plan has unsupported passive probe mode %q", config.PassiveProbeMode)
	}
	headCheckpointID, headMonotonicNS, err := langGraphMaterializationHead(run, binding)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	manifest, err := ReadLangGraphNativeCheckpointManifest(binding.ManifestArtifact)
	if err != nil {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("read LangGraph native manifest for materialization head: %w", err)
	}
	headNative, err := langGraphNativeMaterializationHead(manifest, binding, headMonotonicNS)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	runtimeRoot, err := filepath.Abs(strings.TrimSpace(config.RuntimeRoot))
	if err != nil {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("resolve LangGraph runtime root: %w", err)
	}
	sourceWorkspace, err := langGraphProfileWorkspace(run.RecordedPlanArtifact)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	snapshot, err := recovery.CaptureLangGraphWorkspaceSnapshot(sourceWorkspace, strings.TrimSpace(config.PassiveUnixSocketPath))
	if err != nil {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("capture LangGraph recovery source snapshot: %w", err)
	}
	sourceRuntime, err := langGraphSourceRuntime(run)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	socketProbe, err := langGraphUnixSocketProbe(run, binding, headCheckpointID)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	plan := recovery.LangGraphForkPlan{
		SchemaVersion:                   recovery.LangGraphForkPlanSchema,
		RecordedPlanID:                  run.RecordedPlanID,
		AdapterID:                       recovery.LangGraphForkAdapterID,
		TargetID:                        run.TargetID,
		CandidateID:                     candidate.CandidateID,
		Task:                            candidate.Task,
		Model:                           strings.TrimSpace(config.Model),
		ContainerImage:                  strings.TrimSpace(config.ContainerImage),
		RuntimeRoot:                     runtimeRoot,
		PassiveUnixSocketPath:           strings.TrimSpace(config.PassiveUnixSocketPath),
		PassiveProbeMode:                probeMode,
		PassiveObservationID:            "unix-socket-listener-holder-v1:" + strings.TrimSpace(config.PassiveUnixSocketPath),
		MaterializationHeadID:           "materialization-head:" + run.ProfileRunID + ":" + headCheckpointID,
		MaterializationHeadCheckpointID: headCheckpointID,
		SourceThreadID:                  manifest.ThreadID,
		SourceRuntime:                   sourceRuntime,
		WorkspaceSnapshot:               snapshot,
		UnixSocketProbe:                 socketProbe,
		CheckpointCoordinates: map[string]recovery.LangGraphNativeCheckpointCoordinate{
			binding.BeforeProfileCheckpointID: binding.BeforeNativeCoordinate,
			binding.AfterProfileCheckpointID:  binding.AfterNativeCoordinate,
			headCheckpointID:                  nativeCheckpointCoordinate(headNative),
		},
		AgentStateByCheckpoint: map[string]recovery.StatePresence{
			// The timestamp-validated native binding proves the before coordinate
			// was persisted before the effect window and the after coordinate
			// only after it. This is a logical-state projection, not an OS probe.
			binding.BeforeProfileCheckpointID: recovery.StatePresenceAbsent,
			binding.AfterProfileCheckpointID:  recovery.StatePresencePresent,
			headCheckpointID:                  recovery.StatePresencePresent,
		},
	}
	if len(plan.CheckpointCoordinates) != 3 {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph binding does not preserve before, after, and materialization-head coordinates")
	}
	return plan, nil
}

func langGraphSourceRuntime(run objective.ProfileRun) (recovery.LangGraphSourceRuntime, error) {
	if run.RetainedRuntime == nil {
		return recovery.LangGraphSourceRuntime{}, fmt.Errorf("LangGraph recovery plan requires a retained source runtime; rerun synthesis execute-langgraph with --retain-runtime")
	}
	if err := run.RetainedRuntime.Validate(); err != nil {
		return recovery.LangGraphSourceRuntime{}, err
	}
	return recovery.LangGraphSourceRuntime{
		SchemaVersion:  run.RetainedRuntime.SchemaVersion,
		Environment:    run.RetainedRuntime.Environment,
		ContainerName:  run.RetainedRuntime.ContainerName,
		ContainerID:    run.RetainedRuntime.ContainerID,
		ContainerImage: run.RetainedRuntime.ContainerImage,
	}, nil
}

func langGraphUnixSocketProbe(run objective.ProfileRun, binding LangGraphNativeFrontierBinding, headCheckpointID string) (recovery.LangGraphUnixSocketProbe, error) {
	frontier, ok := profileFrontier(run, binding.FrontierID)
	if !ok {
		return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph profile has no bound frontier %q", binding.FrontierID)
	}
	var head *profiling.CheckpointStateSummary
	for index := range run.CheckpointSummaries {
		if run.CheckpointSummaries[index].CheckpointID == headCheckpointID {
			head = &run.CheckpointSummaries[index]
			break
		}
	}
	if head == nil {
		return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph materialization head %q has no state summary", headCheckpointID)
	}

	// A frontier can contain abandoned listener attempts. Only a socket that is
	// still present at the materialization head may become a recovery probe.
	liveEndpoints := make(map[string]struct{})
	holdersBySocket := make(map[string][]profiling.ResourceRef)
	seenHolders := make(map[string]map[string]struct{})
	for _, resource := range head.Resources {
		value := resource.Resource
		if value.Family == profiling.StateFamilyIPC && value.Kind == "unix-listener" && strings.HasPrefix(value.ResourceID, "unix-socket:") && value.SocketID != "" && value.ResourceID == "unix-socket:"+value.SocketID {
			liveEndpoints[value.SocketID] = struct{}{}
		}
		if value.Family == profiling.StateFamilyHandle && value.SocketID != "" && value.HolderPID != 0 && value.FD >= 0 {
			key := fmt.Sprintf("%d:%d", value.HolderPID, value.FD)
			if seenHolders[value.SocketID] == nil {
				seenHolders[value.SocketID] = make(map[string]struct{})
			}
			if _, seen := seenHolders[value.SocketID][key]; !seen {
				seenHolders[value.SocketID][key] = struct{}{}
				holdersBySocket[value.SocketID] = append(holdersBySocket[value.SocketID], value)
			}
		}
	}

	linkedResourceByEffect := make(map[string]string, len(frontier.EvidenceLinks))
	for _, link := range frontier.EvidenceLinks {
		if link.Relation == profiling.EvidenceLinkExactSocketID {
			linkedResourceByEffect[link.EffectID] = link.ResourceID
		}
	}
	type endpointEffects struct {
		bind   profiling.NormalizedEffect
		listen profiling.NormalizedEffect
	}
	effectsBySocket := make(map[string]endpointEffects)
	for _, effect := range frontier.Effects {
		resourceID, linked := linkedResourceByEffect[effect.EffectID]
		if !linked || !strings.HasPrefix(resourceID, "unix-socket:") || effect.Family != profiling.StateFamilyIPC {
			continue
		}
		socketID := strings.TrimPrefix(resourceID, "unix-socket:")
		if socketID == "" || effect.Resource.SocketID != socketID {
			continue
		}
		if _, live := liveEndpoints[socketID]; !live {
			continue
		}
		endpoint := effectsBySocket[socketID]
		switch effect.Operation {
		case "bind":
			if endpoint.bind.EffectID != "" {
				return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph frontier records repeated linked Unix bind effects for live endpoint %q", socketID)
			}
			endpoint.bind = effect
		case "listen":
			if endpoint.listen.EffectID != "" {
				return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph frontier records repeated linked Unix listen effects for live endpoint %q", socketID)
			}
			endpoint.listen = effect
		default:
			continue
		}
		effectsBySocket[socketID] = endpoint
	}
	candidates := make([]string, 0, len(effectsBySocket))
	for socketID, endpoint := range effectsBySocket {
		if endpoint.bind.EffectID != "" && endpoint.listen.EffectID != "" {
			candidates = append(candidates, socketID)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph frontier does not prove a linked Unix bind/listen endpoint that remains live at the materialization head")
	}
	if len(candidates) != 1 {
		return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph materialization head retains multiple linked Unix listener endpoints: %s", strings.Join(candidates, ","))
	}
	socketID := candidates[0]
	selected := effectsBySocket[socketID]
	for _, interval := range run.CheckpointMap.Intervals {
		for _, effect := range interval.Effects {
			if effect.Family != profiling.StateFamilyIPC || effect.Resource.SocketID != socketID || (effect.Operation != "bind" && effect.Operation != "listen") {
				continue
			}
			if (effect.Operation == "bind" && effect.EffectID != selected.bind.EffectID) || (effect.Operation == "listen" && effect.EffectID != selected.listen.EffectID) {
				return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph profile records repeated %s for retained socket %q", effect.Operation, socketID)
			}
		}
	}
	holders := holdersBySocket[socketID]
	if len(holders) != 1 {
		return recovery.LangGraphUnixSocketProbe{}, fmt.Errorf("LangGraph materialization head does not prove exactly one live listener holder for %q", socketID)
	}
	probe := recovery.LangGraphUnixSocketProbe{
		SchemaVersion:  recovery.LangGraphUnixSocketProbeSchema,
		SocketID:       socketID,
		HolderPID:      holders[0].HolderPID,
		HolderFD:       holders[0].FD,
		BindEffectID:   selected.bind.EffectID,
		ListenEffectID: selected.listen.EffectID,
	}
	return probe, probe.Validate()
}

func langGraphProfileWorkspace(recordedPlanArtifact string) (string, error) {
	artifact := strings.TrimSpace(recordedPlanArtifact)
	if artifact == "" {
		return "", fmt.Errorf("LangGraph profile has no recorded-plan artifact")
	}
	artifactDir, err := filepath.Abs(filepath.Dir(artifact))
	if err != nil {
		return "", fmt.Errorf("resolve LangGraph profile artifact directory: %w", err)
	}
	return filepath.Join(artifactDir, "workspace"), nil
}

func langGraphMaterializationHead(run objective.ProfileRun, binding LangGraphNativeFrontierBinding) (string, uint64, error) {
	if len(run.CheckpointCatalog.Checkpoints) == 0 || len(run.CheckpointSummaries) == 0 {
		return "", 0, fmt.Errorf("LangGraph recovery plan requires profile checkpoint state summaries for materialization-head evidence")
	}
	head := run.CheckpointCatalog.Checkpoints[len(run.CheckpointCatalog.Checkpoints)-1]
	if head.CheckpointID == binding.BeforeProfileCheckpointID || head.CheckpointID == binding.AfterProfileCheckpointID || head.MonotonicNS <= binding.LastEffectMonotonicNS {
		return "", 0, fmt.Errorf("LangGraph profile has no post-frontier controller checkpoint for materialization head")
	}
	for _, summary := range run.CheckpointSummaries {
		if summary.CheckpointID == head.CheckpointID && len(summary.Resources) > 0 {
			return head.CheckpointID, head.MonotonicNS, nil
		}
	}
	return "", 0, fmt.Errorf("LangGraph materialization head %q has no observed persistent resources", head.CheckpointID)
}

func langGraphNativeMaterializationHead(manifest LangGraphNativeCheckpointManifest, binding LangGraphNativeFrontierBinding, headMonotonicNS uint64) (LangGraphNativeCheckpoint, error) {
	var head LangGraphNativeCheckpoint
	for _, checkpoint := range manifest.NativeCheckpoints {
		timestamp := checkpoint.PersistedMonotonicNS
		if timestamp <= binding.AfterNativeMonotonicNS || timestamp > headMonotonicNS {
			continue
		}
		if head.PersistedMonotonicNS == 0 || timestamp > head.PersistedMonotonicNS {
			head = checkpoint
		}
	}
	if head.PersistedMonotonicNS == 0 {
		return LangGraphNativeCheckpoint{}, fmt.Errorf("LangGraph native manifest has no durable materialization head after frontier %q", binding.FrontierID)
	}
	return head, nil
}
