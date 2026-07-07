package corpus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

const (
	CorpusExecutionCase   = "case"
	CorpusExecutionTarget = "target"
)

// CorpusEntry is the compact scheduling/minimization handle. The artifact_dir
// keeps the original PoC location; the corpus entry is never a copy of the full
// run artifact tree.
type CorpusEntry struct {
	ExecutionKind        string                                    `json:"execution_kind,omitempty"`
	EntryID              string                                    `json:"entry_id"`
	SuiteID              string                                    `json:"suite_id"`
	RunID                string                                    `json:"run_id"`
	PairID               string                                    `json:"pair_id,omitempty"`
	ControlRunID         string                                    `json:"control_run_id,omitempty"`
	FaultRunID           string                                    `json:"fault_run_id,omitempty"`
	CandidateID          string                                    `json:"candidate_id,omitempty"`
	CaseName             string                                    `json:"case_name"`
	FaultPlanID          string                                    `json:"fault_plan_id,omitempty"`
	PrimitiveID          string                                    `json:"primitive_id,omitempty"`
	TimingProfileID      string                                    `json:"timing_profile_id,omitempty"`
	Iteration            int                                       `json:"iteration"`
	Kind                 string                                    `json:"kind"`
	Key                  string                                    `json:"key"`
	Score                int                                       `json:"score"`
	Signature            core.MismatchSignature                    `json:"signature"`
	Differential         bool                                      `json:"differential,omitempty"`
	SecurityRelevant     bool                                      `json:"security_relevant,omitempty"`
	DifferentialReport   string                                    `json:"differential_report,omitempty"`
	AdapterID            string                                    `json:"adapter_id,omitempty"`
	TargetID             string                                    `json:"target_id,omitempty"`
	TaskID               string                                    `json:"task_id,omitempty"`
	PromptProfileID      string                                    `json:"prompt_profile_id,omitempty"`
	TargetOracleStatus   target.TargetOracleStatus                 `json:"target_oracle_status,omitempty"`
	TargetAttribution    string                                    `json:"target_attribution,omitempty"`
	TaskComplianceStatus target.TargetTaskComplianceStatus         `json:"task_compliance_status,omitempty"`
	ContractStatus       target.TargetContractInterpretationStatus `json:"contract_status,omitempty"`
	ContractProfileID    string                                    `json:"contract_profile_id,omitempty"`
	ContractRuleID       string                                    `json:"contract_rule_id,omitempty"`
	ArtifactDir          string                                    `json:"artifact_dir"`
	RecordedAt           string                                    `json:"recorded_at"`
}

// AppendCorpusEntries writes the given entries to the corpus index and per-entry
// JSON files. It is the single I/O path used by both the synthetic and target
// schedulers, which are responsible for building entries from their own result
// types. This split keeps the corpus package free of scheduler/result types and
// breaks what would otherwise be a corpus<->scheduler import cycle.
func AppendCorpusEntries(corpusDir string, entries []CorpusEntry) error {
	if corpusDir == "" || len(entries) == 0 {
		return nil
	}
	entriesDir := filepath.Join(corpusDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return fmt.Errorf("create corpus entries directory: %w", err)
	}

	indexPath := filepath.Join(corpusDir, "index.jsonl")
	index, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open corpus index: %w", err)
	}
	defer index.Close()

	recordedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range entries {
		if entries[i].RecordedAt == "" {
			entries[i].RecordedAt = recordedAt
		}
		entryPath := filepath.Join(entriesDir, entries[i].EntryID+".json")
		if err := core.WriteJSON(entryPath, entries[i]); err != nil {
			return err
		}
		raw, err := json.Marshal(entries[i])
		if err != nil {
			return err
		}
		if _, err := index.Write(append(raw, '\n')); err != nil {
			return fmt.Errorf("append corpus index: %w", err)
		}
	}
	return nil
}

func ListCorpus(corpusDir string, limit int) ([]CorpusEntry, error) {
	if corpusDir == "" {
		corpusDir = "corpus"
	}
	indexPath := filepath.Join(corpusDir, "index.jsonl")
	index, err := os.Open(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []CorpusEntry{}, nil
		}
		return nil, fmt.Errorf("open corpus index: %w", err)
	}
	defer index.Close()

	var entries []CorpusEntry
	scanner := bufio.NewScanner(index)
	for scanner.Scan() {
		if limit > 0 && len(entries) >= limit {
			break
		}
		var entry CorpusEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode corpus index entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read corpus index: %w", err)
	}
	return entries, nil
}

func ShowCorpusEntry(corpusDir string, entryID string) (*CorpusEntry, error) {
	if corpusDir == "" {
		corpusDir = "corpus"
	}
	if entryID == "" {
		return nil, fmt.Errorf("entry id is required")
	}
	entryPath := filepath.Join(corpusDir, "entries", entryID+".json")
	raw, err := os.ReadFile(entryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return findCorpusEntryByPrefix(corpusDir, entryID)
		}
		return nil, fmt.Errorf("read corpus entry %q: %w", entryID, err)
	}

	var entry CorpusEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("decode corpus entry %q: %w", entryID, err)
	}
	return &entry, nil
}

func findCorpusEntryByPrefix(corpusDir string, prefix string) (*CorpusEntry, error) {
	entries, err := ListCorpus(corpusDir, 0)
	if err != nil {
		return nil, err
	}

	var matches []CorpusEntry
	for _, entry := range entries {
		if strings.HasPrefix(entry.EntryID, prefix) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("corpus entry %q not found", prefix)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("corpus entry prefix %q is ambiguous (%d matches)", prefix, len(matches))
	}
	return &matches[0], nil
}

// CorpusEntryID derives a filesystem-safe entry id for a synthetic-case
// discovery.
func CorpusEntryID(kind string, caseName string, runID string) string {
	key := strings.NewReplacer(
		"<", "",
		">", "",
		",", "",
		" ", "-",
		"/", "-",
		":", "-",
	).Replace(kind + "-" + caseName + "-" + runID)
	return strings.Trim(key, "-")
}

// TargetCorpusEntryID derives a filesystem-safe entry id for a confirmed
// real-target run.
func TargetCorpusEntryID(targetID string, taskID string, runID string) string {
	key := strings.NewReplacer(
		"<", "",
		">", "",
		",", "",
		" ", "-",
		"/", "-",
		":", "-",
	).Replace("target-confirmed-" + targetID + "-" + taskID + "-" + runID)
	return strings.Trim(key, "-")
}

// CorpusScore maps a discovery kind to its ranking weight.
func CorpusScore(kind string) int {
	switch kind {
	case "new-signature":
		return 10
	case "new-impact":
		return 5
	case "new-state-class":
		return 3
	case "target-confirmed":
		return 4
	default:
		return 1
	}
}

func (e CorpusEntry) EffectiveExecutionKind() string {
	if e.ExecutionKind == "" {
		return CorpusExecutionCase
	}
	return e.ExecutionKind
}

func (e CorpusEntry) Subject() string {
	if e.EffectiveExecutionKind() == CorpusExecutionTarget {
		if e.TargetID != "" && e.TaskID != "" {
			return e.TargetID + "/" + e.TaskID
		}
		if e.TaskID != "" {
			return e.TaskID
		}
		if e.TargetID != "" {
			return e.TargetID
		}
	}
	return e.CaseName
}
