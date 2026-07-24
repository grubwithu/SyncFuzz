package synthesis

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/coverage"
)

func ReadCandidate(path string) (SynthesisCandidate, error) {
	var candidate SynthesisCandidate
	if err := readJSON(path, &candidate); err != nil {
		return SynthesisCandidate{}, err
	}
	return candidate, nil
}

func WriteCandidate(path string, candidate SynthesisCandidate) error {
	if candidate.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis candidate schema %q", candidate.SchemaVersion)
	}
	return writeJSON(path, candidate)
}

func ReadCoverageLedger(path string) ([]coverage.CoverageRecord, error) {
	var ledger []coverage.CoverageRecord
	if err := readJSON(path, &ledger); err != nil {
		return nil, err
	}
	return ledger, nil
}

func WriteSchedule(path string, schedule ObjectiveSchedule) error {
	if schedule.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis schedule schema %q", schedule.SchemaVersion)
	}
	return writeJSON(path, schedule)
}

func WriteEvaluation(path string, evaluation CandidateEvaluation) error {
	if evaluation.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported synthesis evaluation schema %q", evaluation.SchemaVersion)
	}
	return writeJSON(path, evaluation)
}

func ReadEvaluation(path string) (CandidateEvaluation, error) {
	var evaluation CandidateEvaluation
	if err := readJSON(path, &evaluation); err != nil {
		return CandidateEvaluation{}, err
	}
	if evaluation.SchemaVersion != SchemaVersion {
		return CandidateEvaluation{}, fmt.Errorf("unsupported synthesis evaluation schema %q", evaluation.SchemaVersion)
	}
	return evaluation, nil
}

func WriteStateFuzzAttempt(path string, attempt StateFuzzAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	return writeJSON(path, attempt)
}

func ReadStateFuzzAttempt(path string) (StateFuzzAttempt, error) {
	var attempt StateFuzzAttempt
	if err := readJSON(path, &attempt); err != nil {
		return StateFuzzAttempt{}, err
	}
	if err := attempt.Validate(); err != nil {
		return StateFuzzAttempt{}, err
	}
	return attempt, nil
}

func WriteStateFuzzBatchReport(path string, report StateFuzzBatchReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	return writeJSON(path, report)
}

func WriteMAFNativeFrontierBinding(path string, binding MAFNativeFrontierBinding) error {
	if binding.SchemaVersion != MAFNativeFrontierBindingSchema {
		return fmt.Errorf("unsupported MAF native frontier binding schema %q", binding.SchemaVersion)
	}
	return writeJSON(path, binding)
}

func WriteLangGraphNativeFrontierBinding(path string, binding LangGraphNativeFrontierBinding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	return writeJSON(path, binding)
}

func ReadLangGraphNativeFrontierBinding(path string) (LangGraphNativeFrontierBinding, error) {
	var binding LangGraphNativeFrontierBinding
	if err := readJSON(path, &binding); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	if err := binding.Validate(); err != nil {
		return LangGraphNativeFrontierBinding{}, err
	}
	return binding, nil
}

func WriteLangGraphNativeCheckpointCoordinate(path string, coordinate LangGraphNativeCheckpointCoordinate) error {
	if err := coordinate.Validate(); err != nil {
		return err
	}
	return writeJSON(path, coordinate)
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
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
