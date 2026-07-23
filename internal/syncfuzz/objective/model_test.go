package objective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

func TestPromoteStateSeedRequiresLinkedObjectiveAtoms(t *testing.T) {
	objective := unixListenerObjective()
	run := unixListenerProfileRun()
	seed, err := PromoteStateSeed(objective, run, "C0..C1")
	if err != nil {
		t.Fatalf("PromoteStateSeed returned error: %v", err)
	}
	if err := seed.ValidateFor(objective); err != nil {
		t.Fatalf("promoted seed did not validate: %v", err)
	}
	if seed.FrontierID != "C0..C1" || len(seed.ResourceIDs) != 1 || seed.ResourceIDs[0] != "unix-socket:socket:123" {
		t.Fatalf("unexpected promoted seed: %#v", seed)
	}

	objective.Effects = append(objective.Effects, EffectAtom{Family: profiling.StateFamilyProcess, Operation: "detach"})
	_, err = PromoteStateSeed(objective, run, "C0..C1")
	if err == nil || !strings.Contains(err.Error(), "does not validate objective atom") {
		t.Fatalf("expected missing atom rejection, got %v", err)
	}
}

func TestPromoteStateSeedRejectsUnlinkedFrontier(t *testing.T) {
	objective := unixListenerObjective()
	run := unixListenerProfileRun()
	run.CheckpointMap.Intervals[0].EvidenceLinks = nil
	run.CheckpointMap.Intervals[0].IsFrontier = false
	_, err := PromoteStateSeed(objective, run, "C0..C1")
	if err == nil || !strings.Contains(err.Error(), "linked persistent state evidence") {
		t.Fatalf("expected unlinked frontier rejection, got %v", err)
	}
}

func TestPromoteStateSeedRejectsCalibrationFixture(t *testing.T) {
	run := unixListenerProfileRun()
	run.Kind = ProfileRunKindCalibrationFixture
	run.SynthesisCandidateID = ""
	_, err := PromoteStateSeed(unixListenerObjective(), run, "C0..C1")
	if err == nil || !strings.Contains(err.Error(), "cannot become a StateSeed") {
		t.Fatalf("expected calibration fixture rejection, got %v", err)
	}
}

func TestPromoteStateSeedRequiresSchedulerCandidateProvenance(t *testing.T) {
	run := unixListenerProfileRun()
	run.SynthesisCandidateID = ""
	_, err := PromoteStateSeed(unixListenerObjective(), run, "C0..C1")
	if err == nil || !strings.Contains(err.Error(), "scheduler-issued synthesis candidate ID") {
		t.Fatalf("expected scheduler candidate provenance rejection, got %v", err)
	}
}

func TestImportTargetProfileRunUsesOnlyProfilingArtifacts(t *testing.T) {
	dir := t.TempDir()
	result := map[string]any{
		"run_id":     "target-run-1",
		"adapter_id": "command",
		"target_id":  "command",
		"completed":  true,
		"profiling_analysis": map[string]any{
			"hot_frontiers": 1,
		},
		// Deliberately include legacy-looking data; import must not consume it.
		"scenario": map[string]any{"mutations": []string{"legacy"}},
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode target result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, targetResultArtifact), raw, 0o644); err != nil {
		t.Fatalf("write target result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, targetTaskArtifact), []byte(`{"adapter_id":"command","target_id":"command"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write target task: %v", err)
	}
	effectMap := unixListenerProfileRun().CheckpointMap
	effectMap.RunID = "target-run-1"
	if err := profiling.WriteCheckpointEffectMap(filepath.Join(dir, checkpointMapArtifact), effectMap); err != nil {
		t.Fatalf("write checkpoint map: %v", err)
	}
	run, err := ImportTargetProfileRun(dir, unixListenerObjective().ObjectiveID, ProfileRunKindCalibrationFixture, "")
	if err != nil {
		t.Fatalf("ImportTargetProfileRun returned error: %v", err)
	}
	if run.ProfileRunID != "target-profile:target-run-1" || run.Kind != ProfileRunKindCalibrationFixture || run.RecordedPlanArtifact != filepath.Join(dir, targetTaskArtifact) {
		t.Fatalf("unexpected imported profile run: %#v", run)
	}
}

func TestImportTargetProfileRunRequiresCandidateForSynthesis(t *testing.T) {
	_, err := ImportTargetProfileRun(t.TempDir(), unixListenerObjective().ObjectiveID, ProfileRunKindSynthesisCandidate, "")
	if err == nil || !strings.Contains(err.Error(), "scheduler-issued synthesis candidate ID") {
		t.Fatalf("expected synthesis candidate import rejection, got %v", err)
	}
}

func unixListenerObjective() StateObjective {
	return StateObjective{
		SchemaVersion:    SchemaVersion,
		ObjectiveID:      "ipc.unix-listener.survival",
		Effects:          []EffectAtom{{Family: profiling.StateFamilyIPC, Operation: "bind"}, {Family: profiling.StateFamilyIPC, Operation: "listen"}},
		Lifetime:         "survive-tool-return",
		ResourceRelation: "fixed-path-served-by-descendant",
		Persistence:      "across-checkpoint",
	}
}

func unixListenerProfileRun() ProfileRun {
	return ProfileRun{
		SchemaVersion:        SchemaVersion,
		ProfileRunID:         "profile-1",
		Kind:                 ProfileRunKindSynthesisCandidate,
		SynthesisCandidateID: "synthesis-candidate:ipc-unix-listener:1",
		ObjectiveID:          "ipc.unix-listener.survival",
		TargetID:             "langgraph-shell-react",
		AdapterID:            "langgraph-shell-react",
		RecordedPlanID:       "recorded-plan:profile-1",
		RecordedPlanArtifact: "recorded-plan.json",
		CheckpointMap: profiling.CheckpointEffectMap{
			SchemaVersion: profiling.SchemaVersion,
			RunID:         "run-1",
			Intervals: []profiling.CheckpointInterval{{
				FrontierID:         "C0..C1",
				BeforeCheckpointID: "C0",
				AfterCheckpointID:  "C1",
				Effects: []profiling.NormalizedEffect{
					{EffectID: "bind/2", Family: profiling.StateFamilyIPC, Operation: "bind", PersistencePotential: true},
					{EffectID: "listen/2", Family: profiling.StateFamilyIPC, Operation: "listen", PersistencePotential: true},
				},
				PersistentDelta: profiling.StateDelta{Added: []profiling.PersistentResource{{
					Observed: true,
					Resource: profiling.ResourceRef{ResourceID: "unix-socket:socket:123", Family: profiling.StateFamilyIPC, SocketID: "socket:123"},
				}}},
				EvidenceLinks: []profiling.EvidenceLink{
					{LinkID: "bind/2=>unix", EffectID: "bind/2", ResourceID: "unix-socket:socket:123", Relation: profiling.EvidenceLinkExactSocketID},
					{LinkID: "listen/2=>unix", EffectID: "listen/2", ResourceID: "unix-socket:socket:123", Relation: profiling.EvidenceLinkExactSocketID},
				},
				IsFrontier: true,
			}},
		},
	}
}
