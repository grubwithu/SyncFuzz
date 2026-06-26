package syncfuzz

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CorpusEntry struct {
	EntryID            string            `json:"entry_id"`
	SuiteID            string            `json:"suite_id"`
	RunID              string            `json:"run_id"`
	PairID             string            `json:"pair_id,omitempty"`
	ControlRunID       string            `json:"control_run_id,omitempty"`
	FaultRunID         string            `json:"fault_run_id,omitempty"`
	CandidateID        string            `json:"candidate_id,omitempty"`
	CaseName           string            `json:"case_name"`
	FaultPlanID        string            `json:"fault_plan_id,omitempty"`
	PrimitiveID        string            `json:"primitive_id,omitempty"`
	TimingProfileID    string            `json:"timing_profile_id,omitempty"`
	Iteration          int               `json:"iteration"`
	Kind               string            `json:"kind"`
	Key                string            `json:"key"`
	Score              int               `json:"score"`
	Signature          MismatchSignature `json:"signature"`
	Differential       bool              `json:"differential,omitempty"`
	SecurityRelevant   bool              `json:"security_relevant,omitempty"`
	DifferentialReport string            `json:"differential_report,omitempty"`
	ArtifactDir        string            `json:"artifact_dir"`
	RecordedAt         string            `json:"recorded_at"`
}

// WriteCorpus registers interesting discoveries without copying the full run
// artifact tree. The artifact_dir keeps the original PoC location, while the
// corpus entry becomes the compact scheduling/minimization handle.
func WriteCorpus(corpusDir string, suite *SuiteResult) ([]CorpusEntry, error) {
	if corpusDir == "" || suite == nil || len(suite.Discoveries) == 0 {
		return nil, nil
	}
	entriesDir := filepath.Join(corpusDir, "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create corpus entries directory: %w", err)
	}

	indexPath := filepath.Join(corpusDir, "index.jsonl")
	index, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open corpus index: %w", err)
	}
	defer index.Close()

	recordedAt := time.Now().UTC().Format(time.RFC3339Nano)
	entries := make([]CorpusEntry, 0, len(suite.Discoveries))
	for _, discovery := range suite.Discoveries {
		entry := CorpusEntry{
			EntryID:            corpusEntryID(discovery),
			SuiteID:            suite.SuiteID,
			RunID:              discovery.RunID,
			PairID:             discovery.PairID,
			ControlRunID:       discovery.ControlRunID,
			FaultRunID:         discovery.FaultRunID,
			CandidateID:        discovery.CandidateID,
			CaseName:           discovery.CaseName,
			FaultPlanID:        discovery.FaultPlanID,
			PrimitiveID:        discovery.PrimitiveID,
			TimingProfileID:    discovery.TimingProfileID,
			Iteration:          discovery.Iteration,
			Kind:               discovery.Kind,
			Key:                discovery.Key,
			Score:              corpusScore(discovery.Kind),
			Signature:          discovery.Signature,
			Differential:       discovery.Differential,
			SecurityRelevant:   discovery.SecurityRelevant,
			DifferentialReport: discovery.DifferentialReport,
			ArtifactDir:        discovery.ArtifactDir,
			RecordedAt:         recordedAt,
		}
		entries = append(entries, entry)

		entryPath := filepath.Join(entriesDir, entry.EntryID+".json")
		if err := writeJSON(entryPath, entry); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		if _, err := index.Write(append(raw, '\n')); err != nil {
			return nil, fmt.Errorf("append corpus index: %w", err)
		}
	}
	return entries, nil
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

func corpusEntryID(discovery SuiteDiscovery) string {
	key := strings.NewReplacer(
		"<", "",
		">", "",
		",", "",
		" ", "-",
		"/", "-",
		":", "-",
	).Replace(discovery.Kind + "-" + discovery.CaseName + "-" + discovery.RunID)
	return strings.Trim(key, "-")
}

func corpusScore(kind string) int {
	switch kind {
	case "new-signature":
		return 10
	case "new-impact":
		return 5
	case "new-state-class":
		return 3
	default:
		return 1
	}
}
