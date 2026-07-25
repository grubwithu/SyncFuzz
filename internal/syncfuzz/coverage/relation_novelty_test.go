package coverage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

func TestRecordRelationNoveltyExcludesEphemeralToolIdentity(t *testing.T) {
	report := testCompleteRelationReport(t, "state-seed:profile-a:C0..C1", "call-a", strings.Repeat("a", 64))
	first, err := RecordRelationNovelty(report)
	if err != nil {
		t.Fatalf("RecordRelationNovelty: %v", err)
	}
	if first.CausalEffectEvidenceStatus != recovery.CausalEffectEvidenceProven || first.CausalToolName != "shell" || len(first.Controls) != 3 {
		t.Fatalf("unexpected novelty record: %#v", first)
	}
	if strings.Contains(first.TupleKey(), "call-a") || strings.Contains(first.TupleKey(), strings.Repeat("a", 64)) {
		t.Fatalf("novelty key must not contain per-run tool identities: %q", first.TupleKey())
	}

	secondReport := testCompleteRelationReport(t, "state-seed:profile-a:C0..C1", "call-b", strings.Repeat("b", 64))
	second, err := RecordRelationNovelty(secondReport)
	if err != nil {
		t.Fatalf("RecordRelationNovelty second report: %v", err)
	}
	if first.RecordID != second.RecordID || first.TupleKey() != second.TupleKey() {
		t.Fatalf("ephemeral tool identity changed novelty: first=%#v second=%#v", first, second)
	}
}

func TestUpdateRelationNoveltyLedgerSeparatesConfidenceFromNovelty(t *testing.T) {
	first := testCompleteRelationReport(t, "state-seed:profile-a:C0..C1", "call-a", strings.Repeat("a", 64))
	second := testCompleteRelationReport(t, "state-seed:profile-b:C0..C1", "call-b", strings.Repeat("b", 64))
	ledger, summary, err := UpdateRelationNoveltyLedger(RelationNoveltyLedger{}, []recovery.RecoveryRelationReport{first, second})
	if err != nil {
		t.Fatalf("UpdateRelationNoveltyLedger: %v", err)
	}
	if summary.InputReportCount != 2 || summary.AddedRecordCount != 2 || summary.RecordCount != 2 || summary.UniqueTupleCount != 1 || summary.ProvenCausalRecordCount != 2 || summary.UnknownCausalRecordCount != 0 {
		t.Fatalf("unexpected novelty summary: %#v", summary)
	}

	_, repeated, err := UpdateRelationNoveltyLedger(ledger, []recovery.RecoveryRelationReport{first})
	if err != nil {
		t.Fatalf("UpdateRelationNoveltyLedger repeat: %v", err)
	}
	if repeated.AddedRecordCount != 0 || repeated.RecordCount != 2 || repeated.UniqueTupleCount != 1 {
		t.Fatalf("repeat report must be idempotent: %#v", repeated)
	}

	path := filepath.Join(t.TempDir(), "relation-novelty-ledger.json")
	if err := WriteRelationNoveltyLedger(path, ledger); err != nil {
		t.Fatalf("WriteRelationNoveltyLedger: %v", err)
	}
	loaded, err := ReadRelationNoveltyLedger(path)
	if err != nil {
		t.Fatalf("ReadRelationNoveltyLedger: %v", err)
	}
	if len(loaded.Records) != 2 || loaded.Records[0].TupleKey() != loaded.Records[1].TupleKey() {
		t.Fatalf("relation novelty ledger round-trip changed records: %#v", loaded)
	}
}

func TestRecordRelationNoveltyRejectsIncompleteOrUnattributedReports(t *testing.T) {
	report := testCompleteRelationReport(t, "state-seed:profile-a:C0..C1", "call-a", strings.Repeat("a", 64))
	report.CausalEffectEvidence = nil
	if _, err := RecordRelationNovelty(report); err == nil || !strings.Contains(err.Error(), "explicit causal") {
		t.Fatalf("expected explicit causal evidence requirement, got %v", err)
	}

	report = testCompleteRelationReport(t, "state-seed:profile-a:C0..C1", "call-a", strings.Repeat("a", 64))
	report.Controls[0].Evidence.Status = recovery.RecoveryEvidenceIncomplete
	if _, err := RecordRelationNovelty(report); err == nil {
		t.Fatal("expected incomplete relation report rejection")
	}
}

func testCompleteRelationReport(t *testing.T, seedID, toolCallID, commandSHA string) recovery.RecoveryRelationReport {
	t.Helper()
	control := func(name, checkpointID string, agent recovery.StatePresence) recovery.RecoveryRelationControl {
		evidence, relation := recovery.DeriveRecoveryRelation(recovery.RecoveryObservation{
			AgentState:         agent,
			OSState:            recovery.StatePresencePresent,
			OSStateOrigin:      recovery.StateOriginResidual,
			EffectMultiplicity: recovery.EffectMultiplicitySingle,
			Evidence:           []string{"exact listener identity"},
		})
		return recovery.RecoveryRelationControl{Name: name, CheckpointID: checkpointID, Evidence: evidence, Relation: relation}
	}
	report := recovery.RecoveryRelationReport{
		SchemaVersion:   recovery.RecoveryRelationReportSchema,
		SeedID:          seedID,
		ObjectiveID:     "ipc.unix-listener.survival",
		ProfileRunID:    "profile-" + seedID,
		FrontierID:      "before-command..after-command",
		EffectScope:     []objective.EffectAtom{{Family: profiling.StateFamilyIPC, Operation: "bind"}},
		SeedResourceIDs: []string{"unix-socket:socket:123"},
		Controls: []recovery.RecoveryRelationControl{
			control("before", "C0", recovery.StatePresenceAbsent),
			control("after", "C1", recovery.StatePresencePresent),
			control("head", "C2", recovery.StatePresencePresent),
		},
		CausalEffectEvidence: &recovery.CausalEffectEvidence{
			Status:         recovery.CausalEffectEvidenceProven,
			AdapterID:      recovery.LangGraphForkAdapterID,
			RecordedPlanID: "recorded-plan:" + seedID,
			LangGraphToolEffectProof: &recovery.LangGraphToolEffectProvenance{
				ToolCallID:                 toolCallID,
				ToolName:                   "shell",
				ShellSessionID:             "shell-1",
				CommandSHA256:              commandSHA,
				CommandStartedMonotonicNS:  100,
				CommandFinishedMonotonicNS: 300,
				FirstEffectMonotonicNS:     150,
				LastEffectMonotonicNS:      160,
			},
		},
		Contract: recovery.ContractEvaluation{
			Status: recovery.ContractNotEvaluated,
			Reason: "relation coverage does not evaluate a framework contract",
		},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("test report is invalid: %v", err)
	}
	return report
}
