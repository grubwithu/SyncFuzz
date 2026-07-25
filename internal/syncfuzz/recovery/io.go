package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteRecoveryPair(path string, pair RecoveryPair) error {
	return writeRecoveryJSON(path, pair)
}

func ReadRecoveryPair(path string) (RecoveryPair, error) {
	file, err := os.Open(path)
	if err != nil {
		return RecoveryPair{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var pair RecoveryPair
	if err := json.NewDecoder(file).Decode(&pair); err != nil {
		return RecoveryPair{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return pair, nil
}

func WriteHistoricalRecoverySet(path string, set HistoricalRecoverySet) error {
	return writeRecoveryJSON(path, set)
}

func ReadHistoricalRecoverySet(path string) (HistoricalRecoverySet, error) {
	file, err := os.Open(path)
	if err != nil {
		return HistoricalRecoverySet{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var set HistoricalRecoverySet
	if err := json.NewDecoder(file).Decode(&set); err != nil {
		return HistoricalRecoverySet{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return set, nil
}

func ReadForkRecoverySetExecution(path string) (ForkRecoverySetExecution, error) {
	file, err := os.Open(path)
	if err != nil {
		return ForkRecoverySetExecution{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var execution ForkRecoverySetExecution
	if err := json.NewDecoder(file).Decode(&execution); err != nil {
		return ForkRecoverySetExecution{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if execution.SchemaVersion != ExecutionSchemaVersion {
		return ForkRecoverySetExecution{}, fmt.Errorf("unsupported recovery-set execution schema %q", execution.SchemaVersion)
	}
	return execution, nil
}

func WriteRecoveryRelationReport(path string, report RecoveryRelationReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	return writeRecoveryJSON(path, report)
}

func ReadRecoveryRelationReport(path string) (RecoveryRelationReport, error) {
	file, err := os.Open(path)
	if err != nil {
		return RecoveryRelationReport{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var report RecoveryRelationReport
	if err := json.NewDecoder(file).Decode(&report); err != nil {
		return RecoveryRelationReport{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := report.Validate(); err != nil {
		return RecoveryRelationReport{}, fmt.Errorf("read %s: %w", path, err)
	}
	return report, nil
}

func WriteLangGraphProbeFidelityReport(path string, report LangGraphProbeFidelityReport) error {
	if report.SchemaVersion != LangGraphProbeFidelityReportSchema {
		return fmt.Errorf("unsupported LangGraph probe fidelity report schema %q", report.SchemaVersion)
	}
	return writeRecoveryJSON(path, report)
}

func WriteLangGraphProbeFidelityAttempt(path string, attempt LangGraphProbeFidelityAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	return writeRecoveryJSON(path, attempt)
}

func ReadLangGraphProbeFidelityAttempt(path string) (LangGraphProbeFidelityAttempt, error) {
	file, err := os.Open(path)
	if err != nil {
		return LangGraphProbeFidelityAttempt{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var attempt LangGraphProbeFidelityAttempt
	if err := json.NewDecoder(file).Decode(&attempt); err != nil {
		return LangGraphProbeFidelityAttempt{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := attempt.Validate(); err != nil {
		return LangGraphProbeFidelityAttempt{}, fmt.Errorf("read %s: %w", path, err)
	}
	return attempt, nil
}

func WriteLangGraphProbeFidelityBatchReport(path string, report LangGraphProbeFidelityBatchReport) error {
	if report.SchemaVersion != LangGraphProbeFidelityBatchReportSchema {
		return fmt.Errorf("unsupported LangGraph probe fidelity batch report schema %q", report.SchemaVersion)
	}
	return writeRecoveryJSON(path, report)
}

// ReadLangGraphProbeFidelityBatchAttempts reads the attempt records emitted by
// the Makefile batch target. Accepted records must resolve to their local
// standard full/pruned layout; rejected and failed attempts intentionally do
// not load recovery artifacts.
func ReadLangGraphProbeFidelityBatchAttempts(root string) ([]LangGraphProbeFidelityBatchAttemptInput, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("LangGraph probe fidelity batch root is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read LangGraph probe fidelity batch root %s: %w", root, err)
	}
	inputs := make([]LangGraphProbeFidelityBatchAttemptInput, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "attempt-") {
			continue
		}
		attemptRoot := filepath.Join(root, entry.Name())
		attempt, err := ReadLangGraphProbeFidelityAttempt(filepath.Join(attemptRoot, "attempt.json"))
		if err != nil {
			return nil, err
		}
		if filepath.Clean(attempt.ArtifactRoot) != filepath.Clean(attemptRoot) {
			return nil, fmt.Errorf("LangGraph probe fidelity attempt %d artifact root %q does not match %q", attempt.AttemptIndex, attempt.ArtifactRoot, attemptRoot)
		}
		input := LangGraphProbeFidelityBatchAttemptInput{Attempt: attempt}
		if attempt.Status == LangGraphProbeFidelityAttemptAccepted {
			trial, err := ReadLangGraphProbeFidelityTrial(attemptRoot)
			if err != nil {
				return nil, fmt.Errorf("read accepted LangGraph probe fidelity attempt %d: %w", attempt.AttemptIndex, err)
			}
			input.Trial = &trial
		}
		inputs = append(inputs, input)
	}
	sort.Slice(inputs, func(left, right int) bool {
		return inputs[left].Attempt.AttemptIndex < inputs[right].Attempt.AttemptIndex
	})
	return inputs, nil
}

func writeRecoveryJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
