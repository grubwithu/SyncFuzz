package observation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func ReadFootprint(path string) (ResourceFootprint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ResourceFootprint{}, fmt.Errorf("read resource footprint %s: %w", path, err)
	}
	var footprint ResourceFootprint
	if err := json.Unmarshal(raw, &footprint); err != nil {
		return ResourceFootprint{}, fmt.Errorf("decode resource footprint %s: %w", path, err)
	}
	if err := NormalizeFootprint(&footprint); err != nil {
		return ResourceFootprint{}, err
	}
	return footprint, nil
}

func WriteFootprint(path string, footprint *ResourceFootprint) error {
	if err := NormalizeFootprint(footprint); err != nil {
		return err
	}
	if err := core.WriteJSON(path, footprint); err != nil {
		return fmt.Errorf("write resource footprint %s: %w", path, err)
	}
	return nil
}

func ReadPlan(path string) (ObservationPlan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ObservationPlan{}, fmt.Errorf("read observation plan %s: %w", path, err)
	}
	var plan ObservationPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return ObservationPlan{}, fmt.Errorf("decode observation plan %s: %w", path, err)
	}
	if err := ValidatePlan(&plan); err != nil {
		return ObservationPlan{}, err
	}
	return plan, nil
}

func WritePlan(path string, plan *ObservationPlan) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	if err := core.WriteJSON(path, plan); err != nil {
		return fmt.Errorf("write observation plan %s: %w", path, err)
	}
	return nil
}

func ReadTargetedProbeReport(path string) (TargetedProbeReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TargetedProbeReport{}, fmt.Errorf("read targeted probe report %s: %w", path, err)
	}
	var report TargetedProbeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return TargetedProbeReport{}, fmt.Errorf("decode targeted probe report %s: %w", path, err)
	}
	if report.SchemaVersion == "" {
		report.SchemaVersion = TargetedProbeReportSchemaVersion
	}
	if report.SchemaVersion != TargetedProbeReportSchemaVersion {
		return TargetedProbeReport{}, fmt.Errorf("unsupported targeted probe report schema %q", report.SchemaVersion)
	}
	report.QueryID = strings.TrimSpace(report.QueryID)
	if report.QueryID == "" {
		return TargetedProbeReport{}, fmt.Errorf("targeted probe report query_id is required")
	}
	return report, nil
}

func ReadFallbackSnapshot(reportPath string, report TargetedProbeReport) (core.Snapshot, error) {
	artifact := strings.TrimSpace(report.FallbackFilesystemArtifact)
	if artifact == "" {
		return core.Snapshot{}, fmt.Errorf("targeted probe report has no fallback filesystem artifact")
	}
	if filepath.IsAbs(artifact) || filepath.Base(artifact) != artifact {
		return core.Snapshot{}, fmt.Errorf("invalid fallback filesystem artifact %q", artifact)
	}
	path := filepath.Join(filepath.Dir(reportPath), artifact)
	raw, err := os.ReadFile(path)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("read fallback filesystem snapshot %s: %w", path, err)
	}
	var snapshot core.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return core.Snapshot{}, fmt.Errorf("decode fallback filesystem snapshot %s: %w", path, err)
	}
	return snapshot, nil
}
