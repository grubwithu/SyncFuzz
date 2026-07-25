package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

const RelationNoveltyLedgerSchema = "syncfuzz.relation-novelty-ledger.v1"

// RelationNoveltyControl is one complete before/after/head relation. Its
// signature comes from the generic recovery classifier, never from a
// target-specific contract or a natural-language task.
type RelationNoveltyControl struct {
	Name      string                         `json:"name"`
	Class     recovery.RecoveryRelationClass `json:"class"`
	Signature string                         `json:"signature"`
}

// RelationNoveltyRecord is the stable coverage projection of one recovery
// relation report. It deliberately excludes tool-call IDs, shell session IDs,
// command hashes, candidate tasks, and contract decisions: those values are
// per-run audit data rather than relation novelty dimensions.
type RelationNoveltyRecord struct {
	RecordID                   string                              `json:"record_id"`
	SeedID                     string                              `json:"seed_id"`
	ProfileRunID               string                              `json:"profile_run_id"`
	ObjectiveID                string                              `json:"objective_id"`
	AdapterID                  string                              `json:"adapter_id"`
	RecordedPlanID             string                              `json:"recorded_plan_id"`
	EffectScope                []objective.EffectAtom              `json:"effect_scope"`
	CausalEffectEvidenceStatus recovery.CausalEffectEvidenceStatus `json:"causal_effect_evidence_status"`
	CausalToolName             string                              `json:"causal_tool_name,omitempty"`
	Controls                   []RelationNoveltyControl            `json:"controls"`
}

// TupleKey is the relation-coverage key. It intentionally omits provenance
// identities so independent executions of the same relation do not create
// artificial novelty.
func (r RelationNoveltyRecord) TupleKey() string {
	parts := make([]string, 0, len(r.EffectScope)+11)
	parts = append(parts, "effects")
	for _, effect := range r.EffectScope {
		parts = append(parts, effectKey(effect))
	}
	parts = append(parts, "adapter", r.AdapterID, "causal", string(r.CausalEffectEvidenceStatus), r.CausalToolName)
	for _, control := range r.Controls {
		parts = append(parts, control.Name, string(control.Class), control.Signature)
	}
	return strings.Join(parts, "\x00")
}

func (r RelationNoveltyRecord) Validate() error {
	if strings.TrimSpace(r.SeedID) == "" || strings.TrimSpace(r.ProfileRunID) == "" || strings.TrimSpace(r.ObjectiveID) == "" || strings.TrimSpace(r.AdapterID) == "" || strings.TrimSpace(r.RecordedPlanID) == "" || len(r.EffectScope) == 0 {
		return fmt.Errorf("relation novelty record is incomplete")
	}
	if !r.CausalEffectEvidenceStatus.Valid() {
		return fmt.Errorf("relation novelty record has unsupported causal evidence status %q", r.CausalEffectEvidenceStatus)
	}
	if r.CausalEffectEvidenceStatus == recovery.CausalEffectEvidenceProven {
		if strings.TrimSpace(r.CausalToolName) == "" {
			return fmt.Errorf("proven relation novelty record requires a causal tool name")
		}
	} else if strings.TrimSpace(r.CausalToolName) != "" {
		return fmt.Errorf("unknown relation novelty record must not name a causal tool")
	}
	if !sameEffectScope(r.EffectScope, canonicalEffectScope(r.EffectScope)) {
		return fmt.Errorf("relation novelty record effect scope is not canonical")
	}
	expectedControls := []string{"before", "after", "head"}
	if len(r.Controls) != len(expectedControls) {
		return fmt.Errorf("relation novelty record requires before, after, and head controls")
	}
	for index, control := range r.Controls {
		if control.Name != expectedControls[index] || !control.Class.Valid() || strings.TrimSpace(control.Signature) == "" {
			return fmt.Errorf("relation novelty record has invalid %s control", expectedControls[index])
		}
	}
	if r.RecordID != relationNoveltyRecordID(r.SeedID, r.TupleKey()) {
		return fmt.Errorf("relation novelty record has a non-canonical ID")
	}
	return nil
}

// RelationNoveltyLedger retains every distinct seed/tuple observation while
// UniqueTupleCount reports only semantic relation coverage.
type RelationNoveltyLedger struct {
	SchemaVersion string                  `json:"schema_version"`
	Records       []RelationNoveltyRecord `json:"records"`
}

func (l RelationNoveltyLedger) Validate() error {
	if l.SchemaVersion != RelationNoveltyLedgerSchema {
		return fmt.Errorf("unsupported relation novelty ledger schema %q", l.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(l.Records))
	for _, record := range l.Records {
		if err := record.Validate(); err != nil {
			return err
		}
		if _, exists := seen[record.RecordID]; exists {
			return fmt.Errorf("relation novelty ledger repeats record %q", record.RecordID)
		}
		seen[record.RecordID] = struct{}{}
	}
	return nil
}

type RelationNoveltySummary struct {
	InputReportCount         int `json:"input_report_count"`
	AddedRecordCount         int `json:"added_record_count"`
	RecordCount              int `json:"record_count"`
	UniqueTupleCount         int `json:"unique_tuple_count"`
	ProvenCausalRecordCount  int `json:"proven_causal_record_count"`
	UnknownCausalRecordCount int `json:"unknown_causal_record_count"`
}

// RecordRelationNovelty projects a complete recovery relation into a stable
// coverage record. Contract status is deliberately not read or copied.
func RecordRelationNovelty(report recovery.RecoveryRelationReport) (RelationNoveltyRecord, error) {
	if err := report.Validate(); err != nil {
		return RelationNoveltyRecord{}, err
	}
	if report.CausalEffectEvidence == nil {
		return RelationNoveltyRecord{}, fmt.Errorf("recovery relation report has no explicit causal effect evidence")
	}
	if err := report.CausalEffectEvidence.Validate(); err != nil {
		return RelationNoveltyRecord{}, err
	}
	controls := make([]RelationNoveltyControl, 0, len(report.Controls))
	for _, control := range report.Controls {
		if control.Evidence.Status != recovery.RecoveryEvidenceComplete || control.Relation.Class == recovery.RecoveryRelationUnknown {
			return RelationNoveltyRecord{}, fmt.Errorf("recovery relation report control %q is not complete relation evidence", control.Name)
		}
		controls = append(controls, RelationNoveltyControl{Name: control.Name, Class: control.Relation.Class, Signature: control.Relation.Signature})
	}
	record := RelationNoveltyRecord{
		SeedID:                     report.SeedID,
		ProfileRunID:               report.ProfileRunID,
		ObjectiveID:                report.ObjectiveID,
		AdapterID:                  report.CausalEffectEvidence.AdapterID,
		RecordedPlanID:             report.CausalEffectEvidence.RecordedPlanID,
		EffectScope:                canonicalEffectScope(report.EffectScope),
		CausalEffectEvidenceStatus: report.CausalEffectEvidence.Status,
		Controls:                   controls,
	}
	if report.CausalEffectEvidence.LangGraphToolEffectProof != nil {
		record.CausalToolName = report.CausalEffectEvidence.LangGraphToolEffectProof.ToolName
	}
	record.RecordID = relationNoveltyRecordID(record.SeedID, record.TupleKey())
	if err := record.Validate(); err != nil {
		return RelationNoveltyRecord{}, err
	}
	return record, nil
}

// UpdateRelationNoveltyLedger merges reports into a durable ledger. A repeated
// classification of the same seed and semantic tuple is idempotent, while a
// new seed with the same tuple increases confidence but not novelty coverage.
func UpdateRelationNoveltyLedger(existing RelationNoveltyLedger, reports []recovery.RecoveryRelationReport) (RelationNoveltyLedger, RelationNoveltySummary, error) {
	if existing.SchemaVersion == "" {
		existing.SchemaVersion = RelationNoveltyLedgerSchema
	}
	if err := existing.Validate(); err != nil {
		return RelationNoveltyLedger{}, RelationNoveltySummary{}, err
	}
	ledger := RelationNoveltyLedger{
		SchemaVersion: existing.SchemaVersion,
		Records:       append([]RelationNoveltyRecord(nil), existing.Records...),
	}
	seen := make(map[string]struct{}, len(ledger.Records))
	for _, record := range ledger.Records {
		seen[record.RecordID] = struct{}{}
	}
	summary := RelationNoveltySummary{InputReportCount: len(reports)}
	for _, report := range reports {
		record, err := RecordRelationNovelty(report)
		if err != nil {
			return RelationNoveltyLedger{}, RelationNoveltySummary{}, err
		}
		if _, exists := seen[record.RecordID]; exists {
			continue
		}
		seen[record.RecordID] = struct{}{}
		ledger.Records = append(ledger.Records, record)
		summary.AddedRecordCount++
	}
	sort.Slice(ledger.Records, func(left, right int) bool {
		return ledger.Records[left].RecordID < ledger.Records[right].RecordID
	})
	if err := ledger.Validate(); err != nil {
		return RelationNoveltyLedger{}, RelationNoveltySummary{}, err
	}
	summary.RecordCount = len(ledger.Records)
	uniqueTuples := make(map[string]struct{}, len(ledger.Records))
	for _, record := range ledger.Records {
		uniqueTuples[record.TupleKey()] = struct{}{}
		switch record.CausalEffectEvidenceStatus {
		case recovery.CausalEffectEvidenceProven:
			summary.ProvenCausalRecordCount++
		case recovery.CausalEffectEvidenceUnknown:
			summary.UnknownCausalRecordCount++
		}
	}
	summary.UniqueTupleCount = len(uniqueTuples)
	return ledger, summary, nil
}

func canonicalEffectScope(effects []objective.EffectAtom) []objective.EffectAtom {
	result := append([]objective.EffectAtom(nil), effects...)
	sort.Slice(result, func(left, right int) bool { return effectKey(result[left]) < effectKey(result[right]) })
	return result
}

func sameEffectScope(left, right []objective.EffectAtom) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func effectKey(effect objective.EffectAtom) string {
	return string(effect.Family) + "/" + effect.Operation
}

func relationNoveltyRecordID(seedID, tupleKey string) string {
	digest := sha256.Sum256([]byte(seedID + "\x00" + tupleKey))
	return "relation-novelty:" + hex.EncodeToString(digest[:])
}
