package hazard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReadRecoveryHazardReport(path string) (RecoveryHazardReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("read recovery hazard report %s: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var report RecoveryHazardReport
	if err := decoder.Decode(&report); err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("decode recovery hazard report %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return RecoveryHazardReport{}, fmt.Errorf("decode recovery hazard report %s: trailing JSON value", path)
		}
		return RecoveryHazardReport{}, fmt.Errorf("decode recovery hazard report %s: trailing data: %w", path, err)
	}
	if err := report.Validate(); err != nil {
		return RecoveryHazardReport{}, fmt.Errorf("validate recovery hazard report %s: %w", path, err)
	}
	return report, nil
}

func WriteRecoveryHazardReport(path string, report RecoveryHazardReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("recovery hazard report output path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create recovery hazard report directory: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recovery hazard report: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write recovery hazard report %s: %w", path, err)
	}
	return nil
}
