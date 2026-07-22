package target

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

const (
	TargetPairCampaignManifestSchemaVersion = "syncfuzz.target-pair-campaign-manifest.v1"
	TargetPairCampaignResultSchemaVersion   = "syncfuzz.target-pair-campaign-result.v1"
	TargetPairCampaignManifestArtifact      = "target-pair-campaign-manifest.json"
	TargetPairCampaignResultArtifact        = "target-pair-campaign-result.json"
)

type TargetPairControlKind string

const (
	TargetPairControlBaseline         TargetPairControlKind = "baseline"
	TargetPairControlFreshRuntime     TargetPairControlKind = "fresh-runtime"
	TargetPairControlBranchCleanup    TargetPairControlKind = "branch-cleanup"
	TargetPairControlNamespaceRestore TargetPairControlKind = "namespace-restore"
	TargetPairControlCustom           TargetPairControlKind = "custom"
)

// TargetPairCampaignManifest makes counterfactual control selection explicit
// before reports are compared. Each pair must use one typed lifecycle query;
// CompareTargetRuns enforces that invariant when the campaign executes.
type TargetPairCampaignManifest struct {
	SchemaVersion string                   `json:"schema_version"`
	CampaignID    string                   `json:"campaign_id,omitempty"`
	Description   string                   `json:"description,omitempty"`
	Pairs         []TargetPairCampaignPair `json:"pairs"`
}

type TargetPairCampaignPair struct {
	PairID        string                `json:"pair_id"`
	ControlKind   TargetPairControlKind `json:"control_kind"`
	ControlRunDir string                `json:"control_run_dir"`
	TargetRunDir  string                `json:"target_run_dir"`
	Description   string                `json:"description,omitempty"`
}

type TargetPairCampaignOptions struct {
	ManifestPath string
	OutDir       string
}

type TargetPairCampaignResult struct {
	SchemaVersion              string                               `json:"schema_version"`
	CampaignID                 string                               `json:"campaign_id"`
	StartedAt                  string                               `json:"started_at"`
	FinishedAt                 string                               `json:"finished_at"`
	ArtifactDir                string                               `json:"artifact_dir"`
	SourceManifest             string                               `json:"source_manifest"`
	ManifestArtifact           string                               `json:"manifest_artifact"`
	CalibrationSummaryArtifact string                               `json:"calibration_summary_artifact"`
	TotalPairs                 int                                  `json:"total_pairs"`
	RootCauseEligiblePairs     int                                  `json:"root_cause_eligible_pairs"`
	CalibrationCoverage        float64                              `json:"calibration_coverage"`
	ControlKinds               []TargetPairCampaignControlKindStats `json:"control_kinds"`
	Pairs                      []TargetPairCampaignPairResult       `json:"pairs"`
}

type TargetPairCampaignPairResult struct {
	PairID                   string                `json:"pair_id"`
	ControlKind              TargetPairControlKind `json:"control_kind"`
	ControlRunDir            string                `json:"control_run_dir"`
	TargetRunDir             string                `json:"target_run_dir"`
	PairDifferentialArtifact string                `json:"pair_differential_artifact"`
	QueryID                  string                `json:"query_id"`
	CalibrationStatus        string                `json:"calibration_status"`
	CalibrationReason        string                `json:"calibration_reason,omitempty"`
	RootCauseEligible        bool                  `json:"root_cause_eligible"`
	EvidenceCandidates       int                   `json:"evidence_candidates"`
	RootCauseCandidates      int                   `json:"root_cause_candidates"`
}

type TargetPairCampaignControlKindStats struct {
	ControlKind            TargetPairControlKind `json:"control_kind"`
	PairCount              int                   `json:"pair_count"`
	RootCauseEligiblePairs int                   `json:"root_cause_eligible_pairs"`
	RootCauseCandidates    int                   `json:"root_cause_candidates"`
}

// RunTargetPairCampaign compares the pre-recorded control/target runs listed
// in a manifest, writes one pair report per counterfactual, and then writes a
// campaign calibration summary over exactly those reports.
func RunTargetPairCampaign(opts TargetPairCampaignOptions) (*TargetPairCampaignResult, error) {
	manifestPath := strings.TrimSpace(opts.ManifestPath)
	if manifestPath == "" {
		return nil, fmt.Errorf("pair campaign manifest path is required")
	}
	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		return nil, fmt.Errorf("pair campaign output directory is required")
	}
	manifest, err := readTargetPairCampaignManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create pair campaign directory: %w", err)
	}
	manifestArtifact := filepath.Join(outDir, TargetPairCampaignManifestArtifact)
	if err := core.WriteJSON(manifestArtifact, manifest); err != nil {
		return nil, fmt.Errorf("write pair campaign manifest artifact: %w", err)
	}

	started := time.Now().UTC()
	campaignID := strings.TrimSpace(manifest.CampaignID)
	if campaignID == "" {
		campaignID = fmt.Sprintf("target-pair-campaign-%d", started.UnixNano())
	}
	result := &TargetPairCampaignResult{
		SchemaVersion:    TargetPairCampaignResultSchemaVersion,
		CampaignID:       campaignID,
		StartedAt:        started.Format(time.RFC3339Nano),
		ArtifactDir:      outDir,
		SourceManifest:   manifestPath,
		ManifestArtifact: TargetPairCampaignManifestArtifact,
		Pairs:            make([]TargetPairCampaignPairResult, 0, len(manifest.Pairs)),
	}
	controlStats := make(map[TargetPairControlKind]*TargetPairCampaignControlKindStats)
	for _, pair := range manifest.Pairs {
		pairDir := filepath.Join(outDir, pair.PairID)
		if err := os.MkdirAll(pairDir, 0o755); err != nil {
			return nil, fmt.Errorf("create pair campaign artifact directory for %s: %w", pair.PairID, err)
		}
		reportPath := filepath.Join(pairDir, TargetPairDifferentialArtifact)
		report, err := CompareTargetRuns(TargetPairDifferentialOptions{
			ControlRunDir: pair.ControlRunDir,
			TargetRunDir:  pair.TargetRunDir,
			OutputPath:    reportPath,
		})
		if err != nil {
			return nil, fmt.Errorf("compare pair %s: %w", pair.PairID, err)
		}
		calibration := report.ContractCalibration
		result.TotalPairs++
		if calibration.RootCauseEligible {
			result.RootCauseEligiblePairs++
		}
		result.Pairs = append(result.Pairs, TargetPairCampaignPairResult{
			PairID:                   pair.PairID,
			ControlKind:              pair.ControlKind,
			ControlRunDir:            pair.ControlRunDir,
			TargetRunDir:             pair.TargetRunDir,
			PairDifferentialArtifact: filepath.Join(pair.PairID, TargetPairDifferentialArtifact),
			QueryID:                  report.QueryID,
			CalibrationStatus:        calibration.Status,
			CalibrationReason:        calibration.Reason,
			RootCauseEligible:        calibration.RootCauseEligible,
			EvidenceCandidates:       len(report.Evidence),
			RootCauseCandidates:      len(report.RootCauseCandidates),
		})
		stat := controlStats[pair.ControlKind]
		if stat == nil {
			stat = &TargetPairCampaignControlKindStats{ControlKind: pair.ControlKind}
			controlStats[pair.ControlKind] = stat
		}
		stat.PairCount++
		stat.RootCauseCandidates += len(report.RootCauseCandidates)
		if calibration.RootCauseEligible {
			stat.RootCauseEligiblePairs++
		}
	}
	if result.TotalPairs > 0 {
		result.CalibrationCoverage = float64(result.RootCauseEligiblePairs) / float64(result.TotalPairs)
	}
	for _, stat := range controlStats {
		result.ControlKinds = append(result.ControlKinds, *stat)
	}
	canonicalizeTargetPairCampaignResult(result)

	calibrationSummaryPath := filepath.Join(outDir, TargetPairCalibrationSummaryArtifact)
	if _, err := SummarizeTargetPairCalibrations(TargetPairCalibrationSummaryOptions{
		Inputs:     []string{outDir},
		OutputPath: calibrationSummaryPath,
	}); err != nil {
		return nil, fmt.Errorf("summarize pair campaign calibration: %w", err)
	}
	result.CalibrationSummaryArtifact = TargetPairCalibrationSummaryArtifact
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := core.WriteJSON(filepath.Join(outDir, TargetPairCampaignResultArtifact), result); err != nil {
		return nil, fmt.Errorf("write pair campaign result: %w", err)
	}
	return result, nil
}

func readTargetPairCampaignManifest(path string) (TargetPairCampaignManifest, error) {
	path = strings.TrimSpace(path)
	manifest, err := readTargetPairJSON[TargetPairCampaignManifest](path)
	if err != nil {
		return TargetPairCampaignManifest{}, fmt.Errorf("read pair campaign manifest %s: %w", path, err)
	}
	if manifest.SchemaVersion != TargetPairCampaignManifestSchemaVersion {
		return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s has unsupported schema %q", path, manifest.SchemaVersion)
	}
	if len(manifest.Pairs) == 0 {
		return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s has no pairs", path)
	}
	manifestDir := filepath.Dir(path)
	seenIDs := make(map[string]struct{}, len(manifest.Pairs))
	for index := range manifest.Pairs {
		pair := &manifest.Pairs[index]
		pair.PairID = strings.TrimSpace(pair.PairID)
		if !isTargetPairCampaignPairID(pair.PairID) {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s has invalid pair id %q", path, pair.PairID)
		}
		if _, exists := seenIDs[pair.PairID]; exists {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s repeats pair id %q", path, pair.PairID)
		}
		seenIDs[pair.PairID] = struct{}{}
		if !isTargetPairControlKind(pair.ControlKind) {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s has unsupported control kind %q", path, pair.ControlKind)
		}
		if pair.ControlKind == TargetPairControlCustom && strings.TrimSpace(pair.Description) == "" {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s custom control %q requires a description", path, pair.PairID)
		}
		controlDir, err := targetPairCampaignRunDir(manifestDir, pair.ControlRunDir)
		if err != nil {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s control run for %s: %w", path, pair.PairID, err)
		}
		targetDir, err := targetPairCampaignRunDir(manifestDir, pair.TargetRunDir)
		if err != nil {
			return TargetPairCampaignManifest{}, fmt.Errorf("pair campaign manifest %s target run for %s: %w", path, pair.PairID, err)
		}
		pair.ControlRunDir = controlDir
		pair.TargetRunDir = targetDir
	}
	return manifest, nil
}

func isTargetPairCampaignPairID(value string) bool {
	if value == "" || filepath.IsAbs(value) || filepath.Base(value) != value {
		return false
	}
	for _, runeValue := range value {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= 'A' && runeValue <= 'Z') || (runeValue >= '0' && runeValue <= '9') || runeValue == '-' || runeValue == '_' {
			continue
		}
		return false
	}
	return true
}

func isTargetPairControlKind(value TargetPairControlKind) bool {
	switch value {
	case TargetPairControlBaseline, TargetPairControlFreshRuntime, TargetPairControlBranchCleanup, TargetPairControlNamespaceRestore, TargetPairControlCustom:
		return true
	default:
		return false
	}
}

func targetPairCampaignRunDir(manifestDir string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("run directory is required")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(manifestDir, value)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func canonicalizeTargetPairCampaignResult(result *TargetPairCampaignResult) {
	if result == nil {
		return
	}
	sort.Slice(result.ControlKinds, func(i, j int) bool {
		return result.ControlKinds[i].ControlKind < result.ControlKinds[j].ControlKind
	})
}
