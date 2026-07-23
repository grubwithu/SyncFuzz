package profiling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditStandardCalibrationsReportsFixtureScopedMetrics(t *testing.T) {
	root := t.TempDir()
	pathDir := writeCalibrationFixture(t, root, "path-run", calibrationPathMap("path-run"), calibrationPathSummaries())
	fdDir := writeCalibrationFixture(t, root, "fd-run", calibrationFDMap("fd-run"), calibrationFDSummaries())
	socketDir := writeCalibrationFixture(t, root, "socket-run", calibrationSocketMap("socket-run", true), calibrationSocketSummaries())

	report, err := AuditStandardCalibrations(pathDir, fdDir, socketDir)
	if err != nil {
		t.Fatalf("AuditStandardCalibrations returned error: %v", err)
	}
	if !report.Passed || report.ExpectedLinks != 4 || report.ObservedLinks != 4 || report.MatchedLinks != 4 || report.UnexpectedLinks != 0 || report.MissingExpectations != 0 || report.FixtureScopedPrecision != 1 || report.FixtureScopedRecall != 1 {
		t.Fatalf("unexpected calibration report: %#v", report)
	}
	if len(report.Calibrations) != 3 || !report.Calibrations[2].ClosureVerified {
		t.Fatalf("expected verified Unix socket closure: %#v", report.Calibrations)
	}
}

func TestAuditStandardCalibrationsDetectsMissingSocketLink(t *testing.T) {
	root := t.TempDir()
	pathDir := writeCalibrationFixture(t, root, "path-run", calibrationPathMap("path-run"), calibrationPathSummaries())
	fdDir := writeCalibrationFixture(t, root, "fd-run", calibrationFDMap("fd-run"), calibrationFDSummaries())
	socketDir := writeCalibrationFixture(t, root, "socket-run", calibrationSocketMap("socket-run", false), calibrationSocketSummaries())

	report, err := AuditStandardCalibrations(pathDir, fdDir, socketDir)
	if err != nil {
		t.Fatalf("AuditStandardCalibrations returned error: %v", err)
	}
	if report.Passed || report.MissingExpectations != 1 || report.MatchedLinks != 3 || report.FixtureScopedRecall != 0.75 {
		t.Fatalf("expected missing listen link to fail calibration: %#v", report)
	}
}

func writeCalibrationFixture(t *testing.T, root string, runID string, effectMap CheckpointEffectMap, summaries []CheckpointStateSummary) string {
	t.Helper()
	dir := filepath.Join(root, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := writeJSON(filepath.Join(dir, "target-result.json"), calibrationTargetResult{RunID: runID, Completed: true, Environment: "container"}); err != nil {
		t.Fatalf("write target result: %v", err)
	}
	scope := ProfilingScope{RunID: runID, Environment: "container", CgroupID: 42}
	if err := writeJSON(filepath.Join(dir, "ebpf-process-scope.json"), scope); err != nil {
		t.Fatalf("write process scope: %v", err)
	}
	if err := writeJSON(filepath.Join(dir, "ebpf-resource-scope.json"), scope); err != nil {
		t.Fatalf("write resource scope: %v", err)
	}
	catalog := calibrationCatalog(runID)
	if err := writeJSON(filepath.Join(dir, "checkpoint-catalog.json"), catalog); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	if err := writeJSON(filepath.Join(dir, "checkpoint-state-summaries.json"), summaries); err != nil {
		t.Fatalf("write summaries: %v", err)
	}
	if err := WriteCheckpointEffectMap(filepath.Join(dir, "checkpoint-effect-map.json"), effectMap); err != nil {
		t.Fatalf("write effect map: %v", err)
	}
	return dir
}

func calibrationCatalog(runID string) CheckpointCatalog {
	return CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		Checkpoints: []Checkpoint{
			{CheckpointID: "before-command", MonotonicNS: 100},
			{CheckpointID: "after-command", MonotonicNS: 200},
			{CheckpointID: "after-observation", MonotonicNS: 300},
		},
	}
}

func calibrationPathSummaries() []CheckpointStateSummary {
	marker := PersistentResource{Observed: true, Resource: ResourceRef{ResourceID: "workspace:frontier-marker", Family: StateFamilyNamespace, Kind: "workspace-file", Path: "frontier-marker", CanonicalPath: "/workspace/frontier-marker"}}
	return []CheckpointStateSummary{
		{CheckpointID: "before-command", MonotonicNS: 100},
		{CheckpointID: "after-command", MonotonicNS: 200, Resources: []PersistentResource{marker}},
		{CheckpointID: "after-observation", MonotonicNS: 300, Resources: []PersistentResource{marker}},
	}
}

func calibrationPathMap(runID string) CheckpointEffectMap {
	effect := NormalizedEffect{EffectID: "open-marker", Family: StateFamilyHandle, Operation: "open", Resource: ResourceRef{CanonicalPath: "/workspace/frontier-marker"}}
	resource := PersistentResource{Observed: true, Resource: ResourceRef{ResourceID: "workspace:frontier-marker", Family: StateFamilyNamespace, Kind: "workspace-file", Path: "frontier-marker", CanonicalPath: "/workspace/frontier-marker"}}
	return calibrationMap(runID, []NormalizedEffect{effect}, StateDelta{Added: []PersistentResource{resource}}, []EvidenceLink{{LinkID: "open-marker=>workspace:frontier-marker", EffectID: effect.EffectID, ResourceID: resource.Resource.ResourceID, Relation: EvidenceLinkExactCanonicalPath}})
}

func calibrationFDSummaries() []CheckpointStateSummary {
	fd := PersistentResource{Observed: true, Resource: ResourceRef{ResourceID: "container-fd:43:9:device:2049:inode:51668070", Family: StateFamilyHandle, Kind: "deleted-path", CanonicalPath: "/workspace/held-fd", Device: 2049, Inode: 51668070, FD: 9, HolderPID: 43, Deleted: true}}
	return []CheckpointStateSummary{
		{CheckpointID: "before-command", MonotonicNS: 100},
		{CheckpointID: "after-command", MonotonicNS: 200, Resources: []PersistentResource{fd}},
		{CheckpointID: "after-observation", MonotonicNS: 300, Resources: []PersistentResource{fd}},
	}
}

func calibrationFDMap(runID string) CheckpointEffectMap {
	resource := PersistentResource{Observed: true, Resource: ResourceRef{ResourceID: "container-fd:43:9:device:2049:inode:51668070", Family: StateFamilyHandle, Kind: "deleted-path", CanonicalPath: "/workspace/held-fd", Device: 2049, Inode: 51668070, FD: 9, HolderPID: 43, Deleted: true}}
	effect := NormalizedEffect{EffectID: "dup-held-fd", Family: StateFamilyHandle, Operation: "dup", Resource: ResourceRef{Device: 2049, Inode: 51668070}}
	return calibrationMap(runID, []NormalizedEffect{effect}, StateDelta{Added: []PersistentResource{resource}}, []EvidenceLink{{LinkID: "dup-held-fd=>container-fd", EffectID: effect.EffectID, ResourceID: resource.Resource.ResourceID, Relation: EvidenceLinkExactDeviceInode}})
}

func calibrationSocketSummaries() []CheckpointStateSummary {
	resources := calibrationSocketResources()
	dependencies := calibrationSocketDependencies()
	return []CheckpointStateSummary{
		{CheckpointID: "before-command", MonotonicNS: 100},
		{CheckpointID: "after-command", MonotonicNS: 200, Resources: resources, Dependencies: dependencies},
		{CheckpointID: "after-observation", MonotonicNS: 300, Resources: resources, Dependencies: dependencies},
	}
}

func calibrationSocketResources() []PersistentResource {
	return []PersistentResource{
		{Observed: true, Resource: ResourceRef{ResourceID: "workspace:branch-listener.sock", Family: StateFamilyNamespace, Kind: "workspace-socket", Path: "branch-listener.sock", CanonicalPath: "/workspace/branch-listener.sock"}},
		{Observed: true, Resource: ResourceRef{ResourceID: "unix-socket:socket:9", Family: StateFamilyIPC, Kind: "unix-listener", Path: "branch-listener.sock", CanonicalPath: "/workspace/branch-listener.sock", Inode: 9, SocketID: "socket:9"}},
		{Observed: true, Resource: ResourceRef{ResourceID: "container-fd:43:3:device:8:inode:9", Family: StateFamilyHandle, Kind: "socket", Device: 8, Inode: 9, SocketID: "socket:9", FD: 3, HolderPID: 43}},
		{Observed: true, Resource: ResourceRef{ResourceID: "container-process:43", Family: StateFamilyProcess, Kind: "process", HolderPID: 43}},
	}
}

func calibrationSocketDependencies() []ResourceDependency {
	return []ResourceDependency{
		{FromResourceID: "unix-socket:socket:9", ToResourceID: "workspace:branch-listener.sock", Relation: "bound-at-path"},
		{FromResourceID: "container-fd:43:3:device:8:inode:9", ToResourceID: "unix-socket:socket:9", Relation: "references-unix-socket"},
		{FromResourceID: "container-fd:43:3:device:8:inode:9", ToResourceID: "container-process:43", Relation: "held-by-process"},
	}
}

func calibrationSocketMap(runID string, includeListen bool) CheckpointEffectMap {
	resources := calibrationSocketResources()
	bind := NormalizedEffect{EffectID: "bind-socket", Family: StateFamilyIPC, Operation: "bind", Resource: ResourceRef{SocketID: "socket:9"}}
	listen := NormalizedEffect{EffectID: "listen-socket", Family: StateFamilyIPC, Operation: "listen", Resource: ResourceRef{SocketID: "socket:9"}}
	links := []EvidenceLink{{LinkID: "bind-socket=>endpoint", EffectID: bind.EffectID, ResourceID: "unix-socket:socket:9", Relation: EvidenceLinkExactSocketID}}
	if includeListen {
		links = append(links, EvidenceLink{LinkID: "listen-socket=>endpoint", EffectID: listen.EffectID, ResourceID: "unix-socket:socket:9", Relation: EvidenceLinkExactSocketID})
	}
	return calibrationMap(runID, []NormalizedEffect{bind, listen}, StateDelta{Added: resources, AddedDependencies: calibrationSocketDependencies()}, links)
}

func calibrationMap(runID string, effects []NormalizedEffect, delta StateDelta, links []EvidenceLink) CheckpointEffectMap {
	return CheckpointEffectMap{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		Intervals: []CheckpointInterval{
			{FrontierID: "before-command..after-command", BeforeCheckpointID: "before-command", AfterCheckpointID: "after-command", StartMonotonicNS: 100, EndMonotonicNS: 200, Effects: effects, PersistentDelta: delta, EvidenceLinks: links, IsFrontier: true},
			{FrontierID: "after-command..after-observation", BeforeCheckpointID: "after-command", AfterCheckpointID: "after-observation", StartMonotonicNS: 200, EndMonotonicNS: 300},
		},
	}
}
