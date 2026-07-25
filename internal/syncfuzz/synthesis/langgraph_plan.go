package synthesis

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

type LangGraphForkPlanConfig struct {
	Model            string
	ContainerImage   string
	RuntimeRoot      string
	RuntimeContract  recovery.LangGraphRuntimeContract
	ResourceContract recovery.LangGraphRetainedResourceContract
	// PassiveUnixSocketPath and PassiveWorkspaceFilePath are compatibility
	// inputs for older callers. New callers must provide ResourceContract.
	PassiveUnixSocketPath    string
	PassiveWorkspaceFilePath string
	PassiveProbeMode         recovery.LangGraphPassiveProbeMode
}

func (c LangGraphForkPlanConfig) RetainedResourceContract() (recovery.LangGraphRetainedResourceContract, error) {
	hasContract := strings.TrimSpace(c.ResourceContract.SchemaVersion) != ""
	hasSocket := strings.TrimSpace(c.PassiveUnixSocketPath) != ""
	hasWorkspaceFile := strings.TrimSpace(c.PassiveWorkspaceFilePath) != ""
	if hasContract {
		if hasSocket || hasWorkspaceFile {
			return recovery.LangGraphRetainedResourceContract{}, fmt.Errorf("LangGraph fork plan must not mix a retained resource contract with legacy passive resource paths")
		}
		if err := c.ResourceContract.Validate(); err != nil {
			return recovery.LangGraphRetainedResourceContract{}, err
		}
		return c.ResourceContract, nil
	}
	if hasSocket == hasWorkspaceFile {
		return recovery.LangGraphRetainedResourceContract{}, fmt.Errorf("LangGraph fork plan requires exactly one passive Unix socket or workspace file path")
	}
	if hasSocket {
		return recovery.NewLangGraphRetainedResourceContract(recovery.LangGraphRetainedUnixSocket, c.PassiveUnixSocketPath)
	}
	return recovery.NewLangGraphRetainedResourceContract(recovery.LangGraphRetainedWorkspaceFile, c.PassiveWorkspaceFilePath)
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
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.ContainerImage) == "" || strings.TrimSpace(config.RuntimeRoot) == "" {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph fork plan requires model, container image, and runtime root")
	}
	resourceContract, err := config.RetainedResourceContract()
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	hasSocket := resourceContract.Kind == recovery.LangGraphRetainedUnixSocket
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
	toolLifecycleByCheckpoint, err := langGraphForkToolLifecycle(manifest, binding, headCheckpointID, headNative)
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
	sourceRuntime, err := langGraphSourceRuntime(run)
	if err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if config.RuntimeContract.SchemaVersion != "" {
		if err := config.RuntimeContract.Validate(); err != nil {
			return recovery.LangGraphForkPlan{}, err
		}
		if sourceRuntime.ContainerImageID == "" || sourceRuntime.ContainerImageID != config.RuntimeContract.ImageID {
			return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph prepared runtime contract does not match the profiled source image")
		}
	}
	var (
		snapshot             recovery.LangGraphWorkspaceSnapshot
		workspaceTopology    recovery.LangGraphWorkspaceTopology
		socketProbe          recovery.LangGraphUnixSocketProbe
		workspaceFileProbe   *recovery.LangGraphWorkspaceFileProbe
		passiveObservationID string
	)
	snapshot, workspaceTopology, err = recovery.CaptureLangGraphWorkspaceSnapshotForContract(sourceWorkspace, resourceContract)
	if err != nil {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("capture LangGraph recovery source snapshot: %w", err)
	}
	if hasSocket {
		socketProbe, err = langGraphUnixSocketProbe(run, binding, headCheckpointID)
		if err != nil {
			return recovery.LangGraphForkPlan{}, err
		}
		passiveObservationID = "unix-socket-listener-holder-v1:" + resourceContract.WorkspaceRelativePath
	} else {
		if probeMode != recovery.LangGraphPassiveProbeFull {
			return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph workspace file recovery supports only the full passive probe")
		}
		if len(stateObjective.Effects) != 1 || stateObjective.Effects[0].Family != profiling.StateFamilyHandle || stateObjective.Effects[0].Operation != "open" {
			return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph workspace file recovery requires a single handle/open objective")
		}
		if strings.TrimSpace(binding.ObservedWorkspaceFilePath) != resourceContract.WorkspaceRelativePath {
			return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph workspace file recovery path does not match the native frontier binding")
		}
		workspaceFileProbe, err = langGraphWorkspaceFileProbe(run, binding, headCheckpointID, resourceContract.WorkspaceRelativePath)
		if err != nil {
			return recovery.LangGraphForkPlan{}, err
		}
		passiveObservationID = "workspace-file-identity-v1:" + resourceContract.WorkspaceRelativePath
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
		RuntimeContract:                 config.RuntimeContract,
		RuntimeRoot:                     runtimeRoot,
		ResourceContract:                resourceContract,
		WorkspaceTopology:               &workspaceTopology,
		PassiveProbeMode:                probeMode,
		PassiveObservationID:            passiveObservationID,
		MaterializationHeadID:           "materialization-head:" + run.ProfileRunID + ":" + headCheckpointID,
		MaterializationHeadCheckpointID: headCheckpointID,
		SourceThreadID:                  manifest.ThreadID,
		SourceRuntime:                   sourceRuntime,
		WorkspaceSnapshot:               snapshot,
		UnixSocketProbe:                 socketProbe,
		WorkspaceFileProbe:              workspaceFileProbe,
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
		ToolLifecycleByCheckpoint: toolLifecycleByCheckpoint,
		ToolEffectProvenance:      cloneLangGraphToolEffectProvenance(binding.ToolEffectProvenance),
	}
	if hasSocket {
		plan.PassiveUnixSocketPath = resourceContract.WorkspaceRelativePath
	} else {
		plan.PassiveWorkspaceFilePath = resourceContract.WorkspaceRelativePath
	}
	if len(plan.CheckpointCoordinates) != 3 {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph binding does not preserve before, after, and materialization-head coordinates")
	}
	return plan, nil
}

func langGraphWorkspaceFileProbe(run objective.ProfileRun, binding LangGraphNativeFrontierBinding, headCheckpointID, workspaceFilePath string) (*recovery.LangGraphWorkspaceFileProbe, error) {
	frontier, ok := profileFrontier(run, binding.FrontierID)
	if !ok {
		return nil, fmt.Errorf("LangGraph profile has no bound frontier %q", binding.FrontierID)
	}
	canonicalPath, err := langGraphWorkspaceCanonicalPath(workspaceFilePath)
	if err != nil {
		return nil, err
	}
	var head *profiling.CheckpointStateSummary
	for index := range run.CheckpointSummaries {
		if run.CheckpointSummaries[index].CheckpointID == headCheckpointID {
			head = &run.CheckpointSummaries[index]
			break
		}
	}
	if head == nil {
		return nil, fmt.Errorf("LangGraph materialization head %q has no state summary", headCheckpointID)
	}
	resourceIDs := make([]string, 0, 1)
	for _, persistent := range head.Resources {
		resource := persistent.Resource
		if resource.Family == profiling.StateFamilyNamespace && resource.Kind == "workspace-file" && resource.CanonicalPath == canonicalPath && resource.ResourceID != "" {
			resourceIDs = append(resourceIDs, resource.ResourceID)
		}
	}
	sort.Strings(resourceIDs)
	if len(resourceIDs) != 1 {
		return nil, fmt.Errorf("LangGraph materialization head does not prove exactly one retained workspace file at %q", canonicalPath)
	}
	linked := make(map[string]struct{})
	boundObjectiveEffects := make(map[string]struct{}, len(binding.ObjectiveEffectIDs))
	for _, effectID := range binding.ObjectiveEffectIDs {
		boundObjectiveEffects[effectID] = struct{}{}
	}
	for _, link := range frontier.EvidenceLinks {
		if link.Relation == profiling.EvidenceLinkExactCanonicalPath && link.ResourceID == resourceIDs[0] {
			if len(boundObjectiveEffects) != 0 {
				if _, ok := boundObjectiveEffects[link.EffectID]; !ok {
					continue
				}
			}
			linked[link.EffectID] = struct{}{}
		}
	}
	effectIDs := make([]string, 0, len(linked))
	for _, effect := range frontier.Effects {
		if _, ok := linked[effect.EffectID]; !ok || effect.Family != profiling.StateFamilyHandle || effect.Operation != "open" {
			continue
		}
		effectIDs = append(effectIDs, effect.EffectID)
	}
	sort.Strings(effectIDs)
	if len(effectIDs) == 0 {
		return nil, fmt.Errorf("LangGraph frontier has no exact canonical-path-linked workspace file open effect for %q", canonicalPath)
	}
	probe := &recovery.LangGraphWorkspaceFileProbe{
		SchemaVersion: recovery.LangGraphWorkspaceFileProbeSchema,
		ResourceID:    resourceIDs[0],
		CanonicalPath: canonicalPath,
		OpenEffectIDs: effectIDs,
	}
	return probe, probe.Validate()
}

func langGraphWorkspaceCanonicalPath(relativePath string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(relativePath))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("LangGraph workspace file path %q is not workspace-relative", relativePath)
	}
	return "/workspace/" + filepath.ToSlash(cleaned), nil
}

func cloneLangGraphToolEffectProvenance(source *LangGraphToolEffectProvenance) *recovery.LangGraphToolEffectProvenance {
	if source == nil {
		return nil
	}
	clone := recovery.LangGraphToolEffectProvenance(*source)
	return &clone
}

func langGraphSourceRuntime(run objective.ProfileRun) (recovery.LangGraphSourceRuntime, error) {
	if run.RetainedRuntime == nil {
		return recovery.LangGraphSourceRuntime{}, fmt.Errorf("LangGraph recovery plan requires a retained source runtime; rerun synthesis execute-langgraph with --retain-runtime")
	}
	if err := run.RetainedRuntime.Validate(); err != nil {
		return recovery.LangGraphSourceRuntime{}, err
	}
	return recovery.LangGraphSourceRuntime{
		SchemaVersion:    run.RetainedRuntime.SchemaVersion,
		Environment:      run.RetainedRuntime.Environment,
		ContainerName:    run.RetainedRuntime.ContainerName,
		ContainerID:      run.RetainedRuntime.ContainerID,
		ContainerImage:   run.RetainedRuntime.ContainerImage,
		ContainerImageID: run.RetainedRuntime.ContainerImageID,
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

// langGraphForkToolLifecycle keeps message-history lifecycle evidence only
// when every coordinate in the fork plan came from a target that recorded it.
// That preserves the distinction between an old artifact with no lifecycle
// ledger and a new checkpoint whose ledger is explicitly empty.
func langGraphForkToolLifecycle(manifest LangGraphNativeCheckpointManifest, binding LangGraphNativeFrontierBinding, headProfileCheckpointID string, head LangGraphNativeCheckpoint) (map[string]recovery.LangGraphDurableToolLifecycle, error) {
	var before LangGraphNativeCheckpoint
	var after LangGraphNativeCheckpoint
	for _, checkpoint := range manifest.NativeCheckpoints {
		switch checkpoint.CheckpointID {
		case binding.BeforeNativeCheckpointID:
			before = checkpoint
		case binding.AfterNativeCheckpointID:
			after = checkpoint
		}
	}
	if before.CheckpointID == "" || after.CheckpointID == "" {
		return nil, fmt.Errorf("LangGraph fork plan cannot resolve lifecycle checkpoint provenance")
	}
	if before.DurableToolLifecycle == nil || after.DurableToolLifecycle == nil || head.DurableToolLifecycle == nil {
		return nil, nil
	}
	if binding.BeforeNativeToolLifecycle != nil && !reflect.DeepEqual(*binding.BeforeNativeToolLifecycle, *before.DurableToolLifecycle) {
		return nil, fmt.Errorf("LangGraph before checkpoint durable tool lifecycle does not match its native manifest")
	}
	if binding.AfterNativeToolLifecycle != nil && !reflect.DeepEqual(*binding.AfterNativeToolLifecycle, *after.DurableToolLifecycle) {
		return nil, fmt.Errorf("LangGraph after checkpoint durable tool lifecycle does not match its native manifest")
	}
	return map[string]recovery.LangGraphDurableToolLifecycle{
		binding.BeforeProfileCheckpointID: before.DurableToolLifecycle.Clone(),
		binding.AfterProfileCheckpointID:  after.DurableToolLifecycle.Clone(),
		headProfileCheckpointID:           head.DurableToolLifecycle.Clone(),
	}, nil
}
