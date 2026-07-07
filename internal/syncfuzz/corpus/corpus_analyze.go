package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type CorpusAnalyzeOptions struct {
	CorpusDir              string
	Limit                  int
	VerificationResultFile string
}

type CorpusAnalysisFieldStats struct {
	Key          string `json:"key"`
	TotalEntries int    `json:"total_entries"`
}

type CorpusAnalysisSubjectStats struct {
	ExecutionKind         string                     `json:"execution_kind"`
	CaseName              string                     `json:"case_name,omitempty"`
	AdapterID             string                     `json:"adapter_id,omitempty"`
	TargetID              string                     `json:"target_id,omitempty"`
	TaskID                string                     `json:"task_id,omitempty"`
	TotalEntries          int                        `json:"total_entries"`
	TargetOracleSummaries []CorpusAnalysisFieldStats `json:"target_oracle_summaries,omitempty"`
	AttributionSummaries  []CorpusAnalysisFieldStats `json:"attribution_summaries,omitempty"`
	ComplianceSummaries   []CorpusAnalysisFieldStats `json:"compliance_summaries,omitempty"`
	ContractSummaries     []CorpusAnalysisFieldStats `json:"contract_summaries,omitempty"`
}

type CorpusAnalysisResult struct {
	SchemaVersion                string                       `json:"schema_version"`
	CorpusDir                    string                       `json:"corpus_dir"`
	VerificationResultFile       string                       `json:"verification_result_file,omitempty"`
	TotalEntries                 int                          `json:"total_entries"`
	ExecutionSummaries           []CorpusAnalysisFieldStats   `json:"execution_summaries,omitempty"`
	SubjectSummaries             []CorpusAnalysisSubjectStats `json:"subject_summaries,omitempty"`
	TargetOracleSummaries        []CorpusAnalysisFieldStats   `json:"target_oracle_summaries,omitempty"`
	AttributionSummaries         []CorpusAnalysisFieldStats   `json:"attribution_summaries,omitempty"`
	TaskComplianceSummaries      []CorpusAnalysisFieldStats   `json:"task_compliance_summaries,omitempty"`
	ContractSummaries            []CorpusAnalysisFieldStats   `json:"contract_summaries,omitempty"`
	VerificationID               string                       `json:"verification_id,omitempty"`
	VerificationOutcomeSummaries []VerificationOutcomeStats   `json:"verification_outcome_summaries,omitempty"`
	VerificationSubjectSummaries []VerificationSubjectStats   `json:"verification_subject_summaries,omitempty"`
}

type corpusAnalysisSubjectBuilder struct {
	stats        CorpusAnalysisSubjectStats
	oracles      map[string]*CorpusAnalysisFieldStats
	attributions map[string]*CorpusAnalysisFieldStats
	compliances  map[string]*CorpusAnalysisFieldStats
	contracts    map[string]*CorpusAnalysisFieldStats
}

func AnalyzeCorpus(opts CorpusAnalyzeOptions) (*CorpusAnalysisResult, error) {
	if opts.CorpusDir == "" {
		opts.CorpusDir = "corpus"
	}
	entries, err := ListCorpus(opts.CorpusDir, opts.Limit)
	if err != nil {
		return nil, err
	}

	executions := make(map[string]*CorpusAnalysisFieldStats)
	oracles := make(map[string]*CorpusAnalysisFieldStats)
	attributions := make(map[string]*CorpusAnalysisFieldStats)
	compliances := make(map[string]*CorpusAnalysisFieldStats)
	contracts := make(map[string]*CorpusAnalysisFieldStats)
	subjects := make(map[string]*corpusAnalysisSubjectBuilder)

	for _, entry := range entries {
		recordCorpusAnalysisField(executions, entry.EffectiveExecutionKind())
		recordCorpusAnalysisField(oracles, string(entry.TargetOracleStatus))
		recordCorpusAnalysisField(attributions, entry.TargetAttribution)
		recordCorpusAnalysisField(compliances, string(entry.TaskComplianceStatus))
		recordCorpusAnalysisField(contracts, string(entry.ContractStatus))

		subject := corpusAnalysisSubject(subjects, entry)
		subject.stats.TotalEntries++
		recordCorpusAnalysisField(subject.oracles, string(entry.TargetOracleStatus))
		recordCorpusAnalysisField(subject.attributions, entry.TargetAttribution)
		recordCorpusAnalysisField(subject.compliances, string(entry.TaskComplianceStatus))
		recordCorpusAnalysisField(subject.contracts, string(entry.ContractStatus))
	}

	result := &CorpusAnalysisResult{
		SchemaVersion:           "syncfuzz.corpus-analysis.v1",
		CorpusDir:               opts.CorpusDir,
		VerificationResultFile:  opts.VerificationResultFile,
		TotalEntries:            len(entries),
		ExecutionSummaries:      corpusAnalysisFieldStats(executions),
		SubjectSummaries:        corpusAnalysisSubjectStats(subjects),
		TargetOracleSummaries:   corpusAnalysisFieldStats(oracles),
		AttributionSummaries:    corpusAnalysisFieldStats(attributions),
		TaskComplianceSummaries: corpusAnalysisFieldStats(compliances),
		ContractSummaries:       corpusAnalysisFieldStats(contracts),
	}

	if opts.VerificationResultFile != "" {
		verification, err := loadVerificationAnalysis(opts.VerificationResultFile)
		if err != nil {
			return nil, err
		}
		result.VerificationID = verification.VerificationID
		result.VerificationOutcomeSummaries = verification.OutcomeSummaries
		result.VerificationSubjectSummaries = verification.SubjectSummaries
	}
	return result, nil
}

func loadVerificationAnalysis(path string) (*VerificationResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verification result %q: %w", path, err)
	}
	var result VerificationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode verification result %q: %w", path, err)
	}
	return &result, nil
}

func corpusAnalysisSubject(subjects map[string]*corpusAnalysisSubjectBuilder, entry CorpusEntry) *corpusAnalysisSubjectBuilder {
	key := corpusAnalysisSubjectKey(entry)
	item, ok := subjects[key]
	if ok {
		return item
	}
	stats := CorpusAnalysisSubjectStats{
		ExecutionKind: entry.EffectiveExecutionKind(),
	}
	if entry.EffectiveExecutionKind() == CorpusExecutionTarget {
		stats.AdapterID = entry.AdapterID
		stats.TargetID = entry.TargetID
		stats.TaskID = entry.TaskID
	} else {
		stats.CaseName = entry.CaseName
	}
	item = &corpusAnalysisSubjectBuilder{
		stats:        stats,
		oracles:      make(map[string]*CorpusAnalysisFieldStats),
		attributions: make(map[string]*CorpusAnalysisFieldStats),
		compliances:  make(map[string]*CorpusAnalysisFieldStats),
		contracts:    make(map[string]*CorpusAnalysisFieldStats),
	}
	subjects[key] = item
	return item
}

func corpusAnalysisSubjectKey(entry CorpusEntry) string {
	if entry.EffectiveExecutionKind() == CorpusExecutionTarget {
		return strings.Join([]string{entry.EffectiveExecutionKind(), entry.AdapterID, entry.TargetID, entry.TaskID}, "\x00")
	}
	return strings.Join([]string{entry.EffectiveExecutionKind(), entry.CaseName}, "\x00")
}

func recordCorpusAnalysisField(stats map[string]*CorpusAnalysisFieldStats, key string) {
	if key == "" {
		return
	}
	item, ok := stats[key]
	if !ok {
		item = &CorpusAnalysisFieldStats{Key: key}
		stats[key] = item
	}
	item.TotalEntries++
}

func corpusAnalysisFieldStats(stats map[string]*CorpusAnalysisFieldStats) []CorpusAnalysisFieldStats {
	if len(stats) == 0 {
		return nil
	}
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := stats[keys[i]]
		right := stats[keys[j]]
		if left.TotalEntries != right.TotalEntries {
			return left.TotalEntries > right.TotalEntries
		}
		return left.Key < right.Key
	})
	result := make([]CorpusAnalysisFieldStats, 0, len(keys))
	for _, key := range keys {
		result = append(result, *stats[key])
	}
	return result
}

func corpusAnalysisSubjectStats(subjects map[string]*corpusAnalysisSubjectBuilder) []CorpusAnalysisSubjectStats {
	if len(subjects) == 0 {
		return nil
	}
	keys := make([]string, 0, len(subjects))
	for key := range subjects {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := subjects[keys[i]].stats
		right := subjects[keys[j]].stats
		if left.TotalEntries != right.TotalEntries {
			return left.TotalEntries > right.TotalEntries
		}
		if left.ExecutionKind != right.ExecutionKind {
			return left.ExecutionKind < right.ExecutionKind
		}
		if left.TargetID != right.TargetID {
			return left.TargetID < right.TargetID
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		return left.CaseName < right.CaseName
	})
	result := make([]CorpusAnalysisSubjectStats, 0, len(keys))
	for _, key := range keys {
		builder := subjects[key]
		item := builder.stats
		item.TargetOracleSummaries = corpusAnalysisFieldStats(builder.oracles)
		item.AttributionSummaries = corpusAnalysisFieldStats(builder.attributions)
		item.ComplianceSummaries = corpusAnalysisFieldStats(builder.compliances)
		item.ContractSummaries = corpusAnalysisFieldStats(builder.contracts)
		result = append(result, item)
	}
	return result
}
