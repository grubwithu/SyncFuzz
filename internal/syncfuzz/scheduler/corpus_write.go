package scheduler

import (
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

// WriteCorpus registers interesting synthetic-case discoveries. It builds
// compact corpus entries from suite discoveries and delegates storage to the
// corpus package, which has no dependency on scheduler result types.
func WriteCorpus(corpusDir string, suite *SuiteResult) ([]corpus.CorpusEntry, error) {
	if corpusDir == "" || suite == nil || len(suite.Discoveries) == 0 {
		return nil, nil
	}
	entries := make([]corpus.CorpusEntry, 0, len(suite.Discoveries))
	for _, discovery := range suite.Discoveries {
		entries = append(entries, corpus.CorpusEntry{
			ExecutionKind:      corpus.CorpusExecutionCase,
			EntryID:            corpus.CorpusEntryID(discovery.Kind, discovery.CaseName, discovery.RunID),
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
			Score:              corpus.CorpusScore(discovery.Kind),
			Signature:          discovery.Signature,
			Differential:       discovery.Differential,
			SecurityRelevant:   discovery.SecurityRelevant,
			DifferentialReport: discovery.DifferentialReport,
			ArtifactDir:        discovery.ArtifactDir,
		})
	}
	if err := corpus.AppendCorpusEntries(corpusDir, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// WriteTargetCorpus registers confirmed real-target runs. It is the target-track
// mirror of WriteCorpus: build entries from confirmed suite results, then
// delegate storage. Contract-status extraction uses target-package helpers.
func WriteTargetCorpus(corpusDir string, suite *TargetSuiteResult) ([]corpus.CorpusEntry, error) {
	if corpusDir == "" || suite == nil || len(suite.Results) == 0 {
		return nil, nil
	}
	entries := make([]corpus.CorpusEntry, 0, suite.Confirmed)
	for _, item := range suite.Results {
		if !item.Confirmed || item.RunID == "" || item.ArtifactDir == "" {
			continue
		}
		entries = append(entries, corpus.CorpusEntry{
			ExecutionKind:        corpus.CorpusExecutionTarget,
			EntryID:              corpus.TargetCorpusEntryID(suite.TargetID, item.TaskID, item.RunID),
			SuiteID:              suite.SuiteID,
			RunID:                item.RunID,
			CandidateID:          item.CandidateID,
			CaseName:             "",
			Iteration:            item.Iteration,
			Kind:                 "target-confirmed",
			Key:                  item.Signature.String(),
			Score:                corpus.CorpusScore("target-confirmed"),
			Signature:            item.Signature,
			AdapterID:            suite.AdapterID,
			TargetID:             suite.TargetID,
			TaskID:               item.TaskID,
			PromptProfileID:      item.PromptProfileID,
			TargetOracleStatus:   item.TargetOracle.Status,
			TargetAttribution:    item.TargetOracle.Attribution,
			TaskComplianceStatus: item.TaskCompliance.Status,
			ContractStatus:       target.TargetContractInterpretationStatusValue(item.ContractInterpretation),
			ContractProfileID:    target.TargetContractInterpretationProfileIDValue(item.ContractInterpretation),
			ContractRuleID:       target.TargetContractInterpretationRuleIDValue(item.ContractInterpretation),
			ArtifactDir:          item.ArtifactDir,
		})
	}
	if err := corpus.AppendCorpusEntries(corpusDir, entries); err != nil {
		return nil, err
	}
	return entries, nil
}
