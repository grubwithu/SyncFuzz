package synthesis

import (
	"fmt"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

// SelectLangGraphEnvironmentFrontier finds the one controller frontier that
// carries the active target EnvironmentProgram listener's linked bind/listen
// effects. It removes a fragile human choice from the E-enabled path without
// using endpoint names or loose time proximity as a substitute for identity.
func SelectLangGraphEnvironmentFrontier(run objective.ProfileRun) (string, error) {
	if run.TargetID != LangGraphSynthesisTargetID || run.AdapterID != LangGraphSynthesisAdapterID {
		return "", fmt.Errorf("environment frontier selection requires a LangGraph synthesis profile run")
	}
	if err := run.CheckpointMap.Validate(); err != nil {
		return "", fmt.Errorf("validate profile checkpoint-effect map: %w", err)
	}
	_, materialization, err := langGraphTargetEnvironmentMaterialization(run.RecordedPlanArtifact)
	if err != nil {
		return "", err
	}
	if materialization == nil {
		return "", fmt.Errorf("LangGraph profile has no target environment materialization")
	}
	activeSocketID := materialization.ActiveListener.SocketID
	if activeSocketID == "" {
		return "", fmt.Errorf("target environment materialization has no active socket identity")
	}
	candidates := make([]string, 0, 1)
	for _, interval := range run.CheckpointMap.Intervals {
		if !interval.IsFrontier || interval.StartMonotonicNS >= uint64(materialization.EffectWindowMonotonicNS.Start) || interval.EndMonotonicNS < uint64(materialization.EffectWindowMonotonicNS.End) {
			continue
		}
		bind, listen := "", ""
		for _, effect := range interval.Effects {
			if effect.Family != profiling.StateFamilyIPC || effect.Resource.SocketID != activeSocketID || effect.MonotonicNS < uint64(materialization.EffectWindowMonotonicNS.Start) || effect.MonotonicNS > uint64(materialization.EffectWindowMonotonicNS.End) {
				continue
			}
			switch effect.Operation {
			case "bind":
				bind = effect.EffectID
			case "listen":
				listen = effect.EffectID
			}
		}
		if bind == "" || listen == "" || !intervalHasExactSocketEvidence(interval, bind) || !intervalHasExactSocketEvidence(interval, listen) {
			continue
		}
		candidates = append(candidates, interval.FrontierID)
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("target environment materialization maps to %d validated active-listener frontiers, expected exactly one", len(candidates))
	}
	return candidates[0], nil
}

func intervalHasExactSocketEvidence(interval profiling.CheckpointInterval, effectID string) bool {
	for _, link := range interval.EvidenceLinks {
		if link.EffectID == effectID && link.Relation == profiling.EvidenceLinkExactSocketID {
			return true
		}
	}
	return false
}
