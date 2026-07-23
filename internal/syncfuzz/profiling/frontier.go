package profiling

import (
	"fmt"
	"sort"
)

type StateDelta struct {
	Added               []PersistentResource `json:"added,omitempty"`
	Removed             []PersistentResource `json:"removed,omitempty"`
	AddedDependencies   []ResourceDependency `json:"added_dependencies,omitempty"`
	RemovedDependencies []ResourceDependency `json:"removed_dependencies,omitempty"`
}

func (d StateDelta) Changed() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.AddedDependencies) > 0 || len(d.RemovedDependencies) > 0
}

type CheckpointInterval struct {
	FrontierID         string             `json:"frontier_id"`
	BeforeCheckpointID string             `json:"before_checkpoint_id"`
	AfterCheckpointID  string             `json:"after_checkpoint_id"`
	StartMonotonicNS   uint64             `json:"start_monotonic_ns"`
	EndMonotonicNS     uint64             `json:"end_monotonic_ns"`
	Effects            []NormalizedEffect `json:"effects,omitempty"`
	PersistentDelta    StateDelta         `json:"persistent_delta"`
	EvidenceLinks      []EvidenceLink     `json:"evidence_links,omitempty"`
	Score              int                `json:"score"`
	IsFrontier         bool               `json:"is_frontier"`
}

type CheckpointEffectMap struct {
	SchemaVersion string               `json:"schema_version"`
	RunID         string               `json:"run_id"`
	Intervals     []CheckpointInterval `json:"intervals"`
}

// BuildCheckpointEffectMap joins logical checkpoints, normalized collector
// evidence, and probe-confirmed state. A frontier requires both effect
// evidence in the interval and a persistent state delta at its endpoint.
func BuildCheckpointEffectMap(catalog CheckpointCatalog, effects []NormalizedEffect, summaries []CheckpointStateSummary) (*CheckpointEffectMap, error) {
	if err := ValidateCheckpointStateSummaries(catalog, summaries); err != nil {
		return nil, err
	}
	summaryByID := make(map[string]CheckpointStateSummary, len(summaries))
	for _, summary := range summaries {
		summaryByID[summary.CheckpointID] = summary
	}

	orderedEffects := sortedEffects(effects)
	intervals := make([]CheckpointInterval, 0, len(catalog.Checkpoints)-1)
	for index := 0; index+1 < len(catalog.Checkpoints); index++ {
		before := catalog.Checkpoints[index]
		after := catalog.Checkpoints[index+1]
		intervalEffects := effectsBetween(orderedEffects, before.MonotonicNS, after.MonotonicNS)
		delta := diffPersistentState(summaryByID[before.CheckpointID], summaryByID[after.CheckpointID])
		frontier := CheckpointInterval{
			FrontierID:         before.CheckpointID + ".." + after.CheckpointID,
			BeforeCheckpointID: before.CheckpointID,
			AfterCheckpointID:  after.CheckpointID,
			StartMonotonicNS:   before.MonotonicNS,
			EndMonotonicNS:     after.MonotonicNS,
			Effects:            intervalEffects,
			PersistentDelta:    delta,
		}
		frontier.EvidenceLinks = linkEffectsToDelta(intervalEffects, delta)
		frontier.IsFrontier = len(frontier.EvidenceLinks) > 0
		if frontier.IsFrontier {
			frontier.Score = frontierScore(frontier)
		}
		intervals = append(intervals, frontier)
	}
	return &CheckpointEffectMap{SchemaVersion: SchemaVersion, RunID: catalog.RunID, Intervals: intervals}, nil
}

func effectsBetween(effects []NormalizedEffect, start uint64, end uint64) []NormalizedEffect {
	result := make([]NormalizedEffect, 0)
	for _, effect := range effects {
		if effect.MonotonicNS > start && effect.MonotonicNS <= end {
			result = append(result, effect)
		}
	}
	return result
}

func diffPersistentState(before CheckpointStateSummary, after CheckpointStateSummary) StateDelta {
	beforeByID := make(map[string]PersistentResource, len(before.Resources))
	for _, resource := range before.Resources {
		beforeByID[resource.Resource.ResourceID] = resource
	}
	afterByID := make(map[string]PersistentResource, len(after.Resources))
	for _, resource := range after.Resources {
		afterByID[resource.Resource.ResourceID] = resource
	}

	delta := StateDelta{}
	for resourceID, resource := range afterByID {
		if _, ok := beforeByID[resourceID]; !ok {
			delta.Added = append(delta.Added, resource)
		}
	}
	for resourceID, resource := range beforeByID {
		if _, ok := afterByID[resourceID]; !ok {
			delta.Removed = append(delta.Removed, resource)
		}
	}
	beforeDependencies := resourceDependenciesByKey(before.Dependencies)
	afterDependencies := resourceDependenciesByKey(after.Dependencies)
	for key, dependency := range afterDependencies {
		if _, ok := beforeDependencies[key]; !ok {
			delta.AddedDependencies = append(delta.AddedDependencies, dependency)
		}
	}
	for key, dependency := range beforeDependencies {
		if _, ok := afterDependencies[key]; !ok {
			delta.RemovedDependencies = append(delta.RemovedDependencies, dependency)
		}
	}
	sort.Slice(delta.Added, func(i, j int) bool { return delta.Added[i].Resource.ResourceID < delta.Added[j].Resource.ResourceID })
	sort.Slice(delta.Removed, func(i, j int) bool {
		return delta.Removed[i].Resource.ResourceID < delta.Removed[j].Resource.ResourceID
	})
	sort.Slice(delta.AddedDependencies, func(i, j int) bool {
		return resourceDependencyKey(delta.AddedDependencies[i]) < resourceDependencyKey(delta.AddedDependencies[j])
	})
	sort.Slice(delta.RemovedDependencies, func(i, j int) bool {
		return resourceDependencyKey(delta.RemovedDependencies[i]) < resourceDependencyKey(delta.RemovedDependencies[j])
	})
	return delta
}

func resourceDependenciesByKey(dependencies []ResourceDependency) map[string]ResourceDependency {
	result := make(map[string]ResourceDependency, len(dependencies))
	for _, dependency := range dependencies {
		result[resourceDependencyKey(dependency)] = dependency
	}
	return result
}

func resourceDependencyKey(dependency ResourceDependency) string {
	return dependency.FromResourceID + "\x00" + dependency.ToResourceID + "\x00" + dependency.Relation
}

func frontierScore(interval CheckpointInterval) int {
	score := 0
	effectsByID := make(map[string]NormalizedEffect, len(interval.Effects))
	for _, effect := range interval.Effects {
		effectsByID[effect.EffectID] = effect
	}
	resourcesByID := make(map[string]PersistentResource, len(interval.PersistentDelta.Added)+len(interval.PersistentDelta.Removed))
	for _, resource := range interval.PersistentDelta.Added {
		resourcesByID[resource.Resource.ResourceID] = resource
	}
	for _, resource := range interval.PersistentDelta.Removed {
		resourcesByID[resource.Resource.ResourceID] = resource
	}
	for _, link := range interval.EvidenceLinks {
		if effect, ok := effectsByID[link.EffectID]; ok {
			score += effectWeight(effect)
		}
		if resource, ok := resourcesByID[link.ResourceID]; ok {
			score += persistentResourceWeight(resource.Resource.Family)
		}
	}
	return score
}

func effectWeight(effect NormalizedEffect) int {
	switch {
	case effect.Family == StateFamilyIPC && effect.Operation == "bind":
		return 7
	case effect.Family == StateFamilyIPC && effect.Operation == "listen":
		return 7
	case effect.Family == StateFamilyNamespace && effect.Operation == "rebind":
		return 6
	case effect.Family == StateFamilyProcess && effect.Operation == "detach":
		return 6
	case effect.Family == StateFamilyHandle && (effect.Operation == "dup" || effect.Operation == "listening-fd"):
		return 5
	case effect.Family == StateFamilyNamespace && (effect.Operation == "delete" || effect.Operation == "rename" || effect.Operation == "symlink"):
		return 4
	case effect.Family == StateFamilyProcess && effect.Operation == "spawn":
		return 3
	case effect.PersistencePotential:
		return 2
	default:
		return 1
	}
}

func persistentResourceWeight(family StateFamily) int {
	switch family {
	case StateFamilyIPC:
		return 8
	case StateFamilyHandle:
		return 6
	case StateFamilyProcess:
		return 5
	case StateFamilyNamespace:
		return 4
	default:
		return 2
	}
}

func (m CheckpointEffectMap) HotFrontiers() []CheckpointInterval {
	frontiers := make([]CheckpointInterval, 0)
	for _, interval := range m.Intervals {
		if interval.IsFrontier {
			frontiers = append(frontiers, interval)
		}
	}
	sort.Slice(frontiers, func(i, j int) bool {
		if frontiers[i].Score == frontiers[j].Score {
			return frontiers[i].FrontierID < frontiers[j].FrontierID
		}
		return frontiers[i].Score > frontiers[j].Score
	})
	return frontiers
}

func (m CheckpointEffectMap) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported checkpoint effect map schema %q", m.SchemaVersion)
	}
	return nil
}
