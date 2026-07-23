package profiling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRawEventsMapsUnixListenerEffects(t *testing.T) {
	effects, err := NormalizeRawEvents([]RawEvent{
		{EventID: "event-listen", MonotonicNS: 150, Kind: RawEventListen, PID: 42, Resource: ResourceRef{Kind: "unix-socket", Path: "branch-listener.sock"}},
		{EventID: "event-bind", MonotonicNS: 140, Kind: RawEventBind, PID: 42, Resource: ResourceRef{Kind: "unix-socket", Path: "branch-listener.sock"}},
	})
	if err != nil {
		t.Fatalf("NormalizeRawEvents returned error: %v", err)
	}
	if len(effects) != 4 {
		t.Fatalf("expected four normalized effects, got %#v", effects)
	}
	got := []string{
		string(effects[0].Family) + "." + effects[0].Operation,
		string(effects[1].Family) + "." + effects[1].Operation,
		string(effects[2].Family) + "." + effects[2].Operation,
		string(effects[3].Family) + "." + effects[3].Operation,
	}
	want := []string{"namespace.rebind", "ipc.bind", "handle.listening-fd", "ipc.listen"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected effect normalization: got %v want %v", got, want)
	}
	for _, effect := range effects {
		if !effect.PersistencePotential {
			t.Fatalf("expected listener effect to have persistence potential: %#v", effect)
		}
	}
}

func TestBuildCheckpointEffectMapRequiresEffectAndStateDelta(t *testing.T) {
	catalog := CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         "profile-1",
		Checkpoints: []Checkpoint{
			{CheckpointID: "C0", MonotonicNS: 100},
			{CheckpointID: "C1", MonotonicNS: 200},
			{CheckpointID: "C2", MonotonicNS: 300},
		},
	}
	effects, err := NormalizeRawEvents([]RawEvent{
		{EventID: "fork", MonotonicNS: 120, Kind: RawEventProcessFork, PID: 40, Resource: ResourceRef{Kind: "process"}},
		{EventID: "bind", MonotonicNS: 150, Kind: RawEventBind, PID: 42, Resource: ResourceRef{Kind: "unix-socket", Path: "branch-listener.sock"}},
		{EventID: "listen", MonotonicNS: 160, Kind: RawEventListen, PID: 42, Resource: ResourceRef{Kind: "unix-socket", Path: "branch-listener.sock"}},
	})
	if err != nil {
		t.Fatalf("NormalizeRawEvents returned error: %v", err)
	}
	summaries := []CheckpointStateSummary{
		{CheckpointID: "C0", MonotonicNS: 100},
		{CheckpointID: "C1", MonotonicNS: 200, Resources: []PersistentResource{
			{Observed: true, Resource: ResourceRef{ResourceID: "process:42", Family: StateFamilyProcess, Kind: "process", HolderPID: 42}},
			{Observed: true, Resource: ResourceRef{ResourceID: "unix:branch-listener.sock", Family: StateFamilyIPC, Kind: "unix-listener", Path: "branch-listener.sock", HolderPID: 42}},
		}},
		{CheckpointID: "C2", MonotonicNS: 300, Resources: []PersistentResource{
			{Observed: true, Resource: ResourceRef{ResourceID: "process:42", Family: StateFamilyProcess, Kind: "process", HolderPID: 42}},
			{Observed: true, Resource: ResourceRef{ResourceID: "unix:branch-listener.sock", Family: StateFamilyIPC, Kind: "unix-listener", Path: "branch-listener.sock", HolderPID: 42}},
		}},
	}

	result, err := BuildCheckpointEffectMap(catalog, effects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap returned error: %v", err)
	}
	if len(result.Intervals) != 2 {
		t.Fatalf("expected two intervals, got %#v", result.Intervals)
	}
	first := result.Intervals[0]
	if !first.IsFrontier || first.FrontierID != "C0..C1" || len(first.Effects) != 5 || len(first.PersistentDelta.Added) != 2 || first.Score <= 0 {
		t.Fatalf("unexpected first interval: %#v", first)
	}
	second := result.Intervals[1]
	if second.IsFrontier || second.PersistentDelta.Changed() || len(second.Effects) != 0 {
		t.Fatalf("expected non-frontier stable interval, got %#v", second)
	}
	if hot := result.HotFrontiers(); len(hot) != 1 || hot[0].FrontierID != "C0..C1" {
		t.Fatalf("unexpected hot frontiers: %#v", hot)
	}
}

func TestBuildCheckpointEffectMapRejectsUnconfirmedSummaryResource(t *testing.T) {
	catalog := CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         "profile-1",
		Checkpoints:   []Checkpoint{{CheckpointID: "C0", MonotonicNS: 1}, {CheckpointID: "C1", MonotonicNS: 2}},
	}
	_, err := BuildCheckpointEffectMap(catalog, nil, []CheckpointStateSummary{
		{CheckpointID: "C0", MonotonicNS: 1},
		{CheckpointID: "C1", MonotonicNS: 2, Resources: []PersistentResource{{Resource: ResourceRef{ResourceID: "x", Family: StateFamilyProcess}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not confirmed observed") {
		t.Fatalf("expected unconfirmed resource error, got %v", err)
	}
}

func TestReadRawEventsJSONLAndWriteArtifacts(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "events.jsonl")
	encoded, err := json.Marshal(RawEvent{EventID: "fork", MonotonicNS: 10, Kind: RawEventProcessFork, PID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := ReadRawEventsJSONL(rawPath)
	if err != nil {
		t.Fatalf("ReadRawEventsJSONL returned error: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "fork" {
		t.Fatalf("unexpected decoded events: %#v", events)
	}

	outputPath := filepath.Join(dir, "effects.json")
	if err := WriteNormalizedEffects(outputPath, []NormalizedEffect{{EffectID: "fork/1", Family: StateFamilyProcess, Operation: "spawn"}}); err != nil {
		t.Fatalf("WriteNormalizedEffects returned error: %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected output artifact: %v", err)
	}
}

func TestCheckpointRecorderUsesStrictlyOrderedMonotonicCheckpoints(t *testing.T) {
	recorder, err := NewCheckpointRecorder("run-1")
	if err != nil {
		t.Fatalf("NewCheckpointRecorder returned error: %v", err)
	}
	first, err := recorder.Mark("before-command", "P1")
	if err != nil {
		t.Fatalf("mark first checkpoint: %v", err)
	}
	second, err := recorder.Mark("after-command", "P5")
	if err != nil {
		t.Fatalf("mark second checkpoint: %v", err)
	}
	if first.MonotonicNS == 0 || second.MonotonicNS <= first.MonotonicNS {
		t.Fatalf("checkpoints are not monotonic: first=%#v second=%#v", first, second)
	}
	catalog := recorder.Catalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("catalog did not validate: %v", err)
	}
	if _, err := recorder.Mark("before-command", "P1"); err == nil {
		t.Fatal("expected duplicate checkpoint id to fail")
	}
}

func TestBuildCheckpointEffectMapRequiresLinkedResourceEvidence(t *testing.T) {
	catalog := CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         "profile-link",
		Checkpoints:   []Checkpoint{{CheckpointID: "C0", MonotonicNS: 1}, {CheckpointID: "C1", MonotonicNS: 2}},
	}
	summaries := []CheckpointStateSummary{
		{CheckpointID: "C0", MonotonicNS: 1},
		{CheckpointID: "C1", MonotonicNS: 2, Resources: []PersistentResource{{
			Observed: true,
			Resource: ResourceRef{ResourceID: "workspace:marker", Family: StateFamilyNamespace, Path: "marker", CanonicalPath: "/workspace/marker"},
		}}},
	}
	linkedEffects := []NormalizedEffect{{
		EffectID: "open-marker/1", MonotonicNS: 2, Family: StateFamilyHandle, Operation: "open",
		Resource: ResourceRef{Path: "marker", CanonicalPath: "/workspace/marker"}, PersistencePotential: true,
	}}
	linked, err := BuildCheckpointEffectMap(catalog, linkedEffects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap(linked) returned error: %v", err)
	}
	interval := linked.Intervals[0]
	if !interval.IsFrontier || len(interval.EvidenceLinks) != 1 || interval.EvidenceLinks[0].Relation != EvidenceLinkExactCanonicalPath {
		t.Fatalf("expected exact canonical evidence link, got %#v", interval)
	}

	unlinkedEffects := []NormalizedEffect{{
		EffectID: "open-other/1", MonotonicNS: 2, Family: StateFamilyHandle, Operation: "open",
		Resource: ResourceRef{Path: "other", CanonicalPath: "/workspace/other"}, PersistencePotential: true,
	}}
	unlinked, err := BuildCheckpointEffectMap(catalog, unlinkedEffects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap(unlinked) returned error: %v", err)
	}
	if unlinked.Intervals[0].IsFrontier || len(unlinked.Intervals[0].EvidenceLinks) != 0 {
		t.Fatalf("unlinked interval must not become a frontier: %#v", unlinked.Intervals[0])
	}
}

func TestBuildCheckpointEffectMapLinksExactDeviceInodeOnly(t *testing.T) {
	catalog := CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         "profile-identity-link",
		Checkpoints:   []Checkpoint{{CheckpointID: "C0", MonotonicNS: 1}, {CheckpointID: "C1", MonotonicNS: 2}},
	}
	summaries := []CheckpointStateSummary{
		{CheckpointID: "C0", MonotonicNS: 1},
		{CheckpointID: "C1", MonotonicNS: 2, Resources: []PersistentResource{{
			Observed: true,
			Resource: ResourceRef{ResourceID: "fd:deleted", Family: StateFamilyHandle, Device: 42, Inode: 99, Deleted: true},
		}}},
	}
	effects := []NormalizedEffect{{
		EffectID: "open-deleted/1", MonotonicNS: 2, Family: StateFamilyHandle, Operation: "open",
		Resource: ResourceRef{Device: 42, Inode: 99}, PersistencePotential: true,
	}}
	linked, err := BuildCheckpointEffectMap(catalog, effects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap(identity) returned error: %v", err)
	}
	interval := linked.Intervals[0]
	if !interval.IsFrontier || len(interval.EvidenceLinks) != 1 || interval.EvidenceLinks[0].Relation != EvidenceLinkExactDeviceInode {
		t.Fatalf("expected exact device/inode evidence link, got %#v", interval)
	}

	effects[0].Resource.Device = 43
	unlinked, err := BuildCheckpointEffectMap(catalog, effects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap(different device) returned error: %v", err)
	}
	if unlinked.Intervals[0].IsFrontier || len(unlinked.Intervals[0].EvidenceLinks) != 0 {
		t.Fatalf("different device must not link by inode alone: %#v", unlinked.Intervals[0])
	}
}

func TestBuildCheckpointEffectMapLinksUnixSocketIdentity(t *testing.T) {
	catalog := CheckpointCatalog{
		SchemaVersion: SchemaVersion,
		RunID:         "profile-unix-socket-link",
		Checkpoints:   []Checkpoint{{CheckpointID: "C0", MonotonicNS: 1}, {CheckpointID: "C1", MonotonicNS: 2}},
	}
	summaries := []CheckpointStateSummary{
		{CheckpointID: "C0", MonotonicNS: 1},
		{CheckpointID: "C1", MonotonicNS: 2, Resources: []PersistentResource{{
			Observed: true,
			Resource: ResourceRef{ResourceID: "unix-socket:socket:123", Family: StateFamilyIPC, Kind: "unix-listener", SocketID: "socket:123"},
		}}},
	}
	effects := []NormalizedEffect{{
		EffectID: "bind-listener/2", MonotonicNS: 2, Family: StateFamilyIPC, Operation: "bind",
		Resource: ResourceRef{SocketID: "socket:123"}, PersistencePotential: true,
	}}
	linked, err := BuildCheckpointEffectMap(catalog, effects, summaries)
	if err != nil {
		t.Fatalf("BuildCheckpointEffectMap(socket) returned error: %v", err)
	}
	interval := linked.Intervals[0]
	if !interval.IsFrontier || len(interval.EvidenceLinks) != 1 || interval.EvidenceLinks[0].Relation != EvidenceLinkExactSocketID {
		t.Fatalf("expected exact socket evidence link, got %#v", interval)
	}
}

func TestCanonicalizeWorkspaceEventPaths(t *testing.T) {
	events := CanonicalizeWorkspaceEventPaths([]RawEvent{
		{EventID: "relative", Kind: RawEventOpenAt, Resource: ResourceRef{Path: "marker"}},
		{EventID: "absolute", Kind: RawEventOpenAt, Resource: ResourceRef{Path: "/etc/passwd"}},
	}, "/workspace")
	if events[0].Resource.CanonicalPath != "/workspace/marker" || events[1].Resource.CanonicalPath != "/etc/passwd" {
		t.Fatalf("unexpected canonicalized events: %#v", events)
	}
}
