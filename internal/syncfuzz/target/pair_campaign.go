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
	ManifestPath     string
	RuntimePairPaths []string
	OutDir           string
}

type TargetPairCampaignResult struct {
	SchemaVersion              string                                `json:"schema_version"`
	CampaignID                 string                                `json:"campaign_id"`
	StartedAt                  string                                `json:"started_at"`
	FinishedAt                 string                                `json:"finished_at"`
	ArtifactDir                string                                `json:"artifact_dir"`
	SourceManifest             string                                `json:"source_manifest"`
	RuntimePairArtifacts       []string                              `json:"runtime_pair_artifacts,omitempty"`
	ManifestArtifact           string                                `json:"manifest_artifact"`
	CalibrationSummaryArtifact string                                `json:"calibration_summary_artifact"`
	TotalPairs                 int                                   `json:"total_pairs"`
	RootCauseEligiblePairs     int                                   `json:"root_cause_eligible_pairs"`
	CalibrationCoverage        float64                               `json:"calibration_coverage"`
	ControlKinds               []TargetPairCampaignControlKindStats  `json:"control_kinds"`
	CounterfactualLabels       []TargetPairCampaignLabelStats        `json:"counterfactual_labels"`
	QueryStrata                []TargetPairCampaignQueryStratumStats `json:"query_strata"`
	Pairs                      []TargetPairCampaignPairResult        `json:"pairs"`
}

type TargetPairCampaignPairResult struct {
	PairID                   string                        `json:"pair_id"`
	ControlKind              TargetPairControlKind         `json:"control_kind"`
	ControlRunDir            string                        `json:"control_run_dir"`
	TargetRunDir             string                        `json:"target_run_dir"`
	PairDifferentialArtifact string                        `json:"pair_differential_artifact"`
	QueryID                  string                        `json:"query_id"`
	QueryStratum             TargetPairQueryStratum        `json:"query_stratum"`
	CounterfactualLabel      TargetPairCounterfactualLabel `json:"counterfactual_label"`
	CounterfactualReason     string                        `json:"counterfactual_reason,omitempty"`
	CalibrationStatus        string                        `json:"calibration_status"`
	CalibrationReason        string                        `json:"calibration_reason,omitempty"`
	RootCauseEligible        bool                          `json:"root_cause_eligible"`
	EvidenceCandidates       int                           `json:"evidence_candidates"`
	RootCauseCandidates      int                           `json:"root_cause_candidates"`
}

type TargetPairCampaignControlKindStats struct {
	ControlKind            TargetPairControlKind `json:"control_kind"`
	PairCount              int                   `json:"pair_count"`
	RootCauseEligiblePairs int                   `json:"root_cause_eligible_pairs"`
	RootCauseCandidates    int                   `json:"root_cause_candidates"`
}

type TargetPairCampaignLabelStats struct {
	Label     TargetPairCounterfactualLabel `json:"label"`
	PairCount int                           `json:"pair_count"`
}

// TargetPairCampaignQueryStratumStats aggregates deterministic labels without
// treating those labels as a causal conclusion. A stratum is the target
// query's root, violation signature, mutation axes, and control kind.
type TargetPairCampaignQueryStratumStats struct {
	ControlKind          TargetPairControlKind            `json:"control_kind"`
	RootQueryID          string                           `json:"root_query_id"`
	ViolationSignatureID string                           `json:"violation_signature_id,omitempty"`
	MutationOperators    []TargetScenarioMutationOperator `json:"mutation_operators,omitempty"`
	MutationSemanticDiff []string                         `json:"mutation_semantic_diff,omitempty"`
	PairCount            int                              `json:"pair_count"`
	CounterfactualLabels []TargetPairCampaignLabelStats   `json:"counterfactual_labels"`
}

type targetPairCampaignStratumAccumulator struct {
	stats  TargetPairCampaignQueryStratumStats
	labels map[TargetPairCounterfactualLabel]*TargetPairCampaignLabelStats
}

// RunTargetPairCampaign compares the pre-recorded control/target runs listed
// in a manifest, writes one pair report per counterfactual, and then writes a
// campaign calibration summary over exactly those reports.
func RunTargetPairCampaign(opts TargetPairCampaignOptions) (*TargetPairCampaignResult, error) {
	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		return nil, fmt.Errorf("pair campaign output directory is required")
	}
	manifest, sourceManifest, runtimePairArtifacts, err := readTargetPairCampaignInput(opts)
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
		SchemaVersion:        TargetPairCampaignResultSchemaVersion,
		CampaignID:           campaignID,
		StartedAt:            started.Format(time.RFC3339Nano),
		ArtifactDir:          outDir,
		SourceManifest:       sourceManifest,
		RuntimePairArtifacts: runtimePairArtifacts,
		ManifestArtifact:     TargetPairCampaignManifestArtifact,
		Pairs:                make([]TargetPairCampaignPairResult, 0, len(manifest.Pairs)),
	}
	controlStats := make(map[TargetPairControlKind]*TargetPairCampaignControlKindStats)
	labelStats := make(map[TargetPairCounterfactualLabel]*TargetPairCampaignLabelStats)
	strata := make(map[string]*targetPairCampaignStratumAccumulator)
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
			QueryStratum:             report.QueryStratum,
			CounterfactualLabel:      report.CounterfactualLabel,
			CounterfactualReason:     report.CounterfactualReason,
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
		label := report.CounterfactualLabel
		if label == "" {
			label = TargetPairCounterfactualTargetInconclusive
		}
		labelStat := labelStats[label]
		if labelStat == nil {
			labelStat = &TargetPairCampaignLabelStats{Label: label}
			labelStats[label] = labelStat
		}
		labelStat.PairCount++
		stratum := targetPairCampaignStratum(strata, pair.ControlKind, report.QueryStratum)
		stratum.stats.PairCount++
		stratumLabel := stratum.labels[label]
		if stratumLabel == nil {
			stratumLabel = &TargetPairCampaignLabelStats{Label: label}
			stratum.labels[label] = stratumLabel
		}
		stratumLabel.PairCount++
	}
	if result.TotalPairs > 0 {
		result.CalibrationCoverage = float64(result.RootCauseEligiblePairs) / float64(result.TotalPairs)
	}
	for _, stat := range controlStats {
		result.ControlKinds = append(result.ControlKinds, *stat)
	}
	for _, stat := range labelStats {
		result.CounterfactualLabels = append(result.CounterfactualLabels, *stat)
	}
	for _, accumulator := range strata {
		stat := accumulator.stats
		for _, label := range accumulator.labels {
			stat.CounterfactualLabels = append(stat.CounterfactualLabels, *label)
		}
		result.QueryStrata = append(result.QueryStrata, stat)
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

func readTargetPairCampaignInput(opts TargetPairCampaignOptions) (TargetPairCampaignManifest, string, []string, error) {
	manifestPath := strings.TrimSpace(opts.ManifestPath)
	runtimePairPaths, err := targetPairCampaignRuntimePairPaths(opts.RuntimePairPaths)
	if err != nil {
		return TargetPairCampaignManifest{}, "", nil, err
	}
	if manifestPath == "" && len(runtimePairPaths) == 0 {
		return TargetPairCampaignManifest{}, "", nil, fmt.Errorf("pair campaign requires a manifest path or runtime pair artifacts")
	}
	if manifestPath != "" && len(runtimePairPaths) > 0 {
		return TargetPairCampaignManifest{}, "", nil, fmt.Errorf("pair campaign accepts either a manifest path or runtime pair artifacts, not both")
	}
	if manifestPath != "" {
		manifest, err := readTargetPairCampaignManifest(manifestPath)
		if err != nil {
			return TargetPairCampaignManifest{}, "", nil, err
		}
		return manifest, manifestPath, nil, nil
	}
	manifest, err := readTargetRuntimePairCampaignManifest(runtimePairPaths)
	if err != nil {
		return TargetPairCampaignManifest{}, "", nil, err
	}
	return manifest, strings.Join(runtimePairPaths, ","), runtimePairPaths, nil
}

func targetPairCampaignRuntimePairPaths(paths []string) ([]string, error) {
	unique := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, original := range paths {
		path := strings.TrimSpace(original)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve runtime pair artifact %s: %w", path, err)
		}
		absolute = filepath.Clean(absolute)
		if _, seen := unique[absolute]; seen {
			continue
		}
		unique[absolute] = struct{}{}
		result = append(result, absolute)
	}
	sort.Strings(result)
	return result, nil
}

func readTargetRuntimePairCampaignManifest(paths []string) (TargetPairCampaignManifest, error) {
	if len(paths) == 0 {
		return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifacts are required")
	}
	manifest := TargetPairCampaignManifest{
		SchemaVersion: TargetPairCampaignManifestSchemaVersion,
		Description:   "Pair campaign assembled from fresh runtime pair artifacts.",
		Pairs:         make([]TargetPairCampaignPair, 0, len(paths)),
	}
	seenIDs := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		runtimePair, err := readTargetPairJSON[TargetRuntimePairResult](path)
		if err != nil {
			return TargetPairCampaignManifest{}, fmt.Errorf("read runtime pair artifact %s: %w", path, err)
		}
		if runtimePair.SchemaVersion != TargetRuntimePairSchemaVersion {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s has unsupported schema %q", path, runtimePair.SchemaVersion)
		}
		pairID := strings.TrimSpace(runtimePair.PairID)
		if !isTargetPairCampaignPairID(pairID) {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s has invalid pair id %q", path, pairID)
		}
		if _, exists := seenIDs[pairID]; exists {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifacts repeat pair id %q", pairID)
		}
		seenIDs[pairID] = struct{}{}
		if !isTargetPairControlKind(runtimePair.ControlKind) {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s has unsupported control kind %q", path, runtimePair.ControlKind)
		}
		if runtimePair.ControlKind == TargetPairControlCustom && strings.TrimSpace(runtimePair.ControlDescription) == "" {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s custom control %q requires a description", path, pairID)
		}
		controlRunDir, err := targetPairCampaignRunDir("", runtimePair.ControlRunDir)
		if err != nil {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s control run: %w", path, err)
		}
		targetRunDir, err := targetPairCampaignRunDir("", runtimePair.TargetRunDir)
		if err != nil {
			return TargetPairCampaignManifest{}, fmt.Errorf("runtime pair artifact %s target run: %w", path, err)
		}
		manifest.Pairs = append(manifest.Pairs, TargetPairCampaignPair{
			PairID:        pairID,
			ControlKind:   runtimePair.ControlKind,
			ControlRunDir: controlRunDir,
			TargetRunDir:  targetRunDir,
			Description:   strings.TrimSpace(runtimePair.ControlDescription),
		})
	}
	return manifest, nil
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
	sort.Slice(result.CounterfactualLabels, func(i, j int) bool {
		return result.CounterfactualLabels[i].Label < result.CounterfactualLabels[j].Label
	})
	for index := range result.QueryStrata {
		sort.Slice(result.QueryStrata[index].CounterfactualLabels, func(i, j int) bool {
			return result.QueryStrata[index].CounterfactualLabels[i].Label < result.QueryStrata[index].CounterfactualLabels[j].Label
		})
	}
	sort.Slice(result.QueryStrata, func(i, j int) bool {
		left, right := result.QueryStrata[i], result.QueryStrata[j]
		if left.ControlKind != right.ControlKind {
			return left.ControlKind < right.ControlKind
		}
		if left.RootQueryID != right.RootQueryID {
			return left.RootQueryID < right.RootQueryID
		}
		if left.ViolationSignatureID != right.ViolationSignatureID {
			return left.ViolationSignatureID < right.ViolationSignatureID
		}
		return strings.Join(targetPairCampaignMutationOperators(left.MutationOperators), ",")+"\x00"+strings.Join(left.MutationSemanticDiff, ",") < strings.Join(targetPairCampaignMutationOperators(right.MutationOperators), ",")+"\x00"+strings.Join(right.MutationSemanticDiff, ",")
	})
}

func targetPairCampaignStratum(
	strata map[string]*targetPairCampaignStratumAccumulator,
	controlKind TargetPairControlKind,
	query TargetPairQueryStratum,
) *targetPairCampaignStratumAccumulator {
	rootQueryID := strings.TrimSpace(query.RootQueryID)
	if rootQueryID == "" {
		rootQueryID = strings.TrimSpace(query.QueryID)
	}
	signatureID := ""
	if query.ViolationSignature != nil {
		signatureID = query.ViolationSignature.SignatureID
	}
	operators := append([]TargetScenarioMutationOperator{}, query.MutationOperators...)
	sort.Slice(operators, func(i, j int) bool { return operators[i] < operators[j] })
	operatorValues := targetPairCampaignMutationOperators(operators)
	semanticDiff := append([]string{}, query.MutationSemanticDiff...)
	sort.Strings(semanticDiff)
	key := strings.Join([]string{string(controlKind), rootQueryID, signatureID, strings.Join(operatorValues, ","), strings.Join(semanticDiff, ",")}, "\x00")
	accumulator := strata[key]
	if accumulator != nil {
		return accumulator
	}
	accumulator = &targetPairCampaignStratumAccumulator{
		stats: TargetPairCampaignQueryStratumStats{
			ControlKind:          controlKind,
			RootQueryID:          rootQueryID,
			ViolationSignatureID: signatureID,
			MutationOperators:    operators,
			MutationSemanticDiff: semanticDiff,
		},
		labels: make(map[TargetPairCounterfactualLabel]*TargetPairCampaignLabelStats),
	}
	strata[key] = accumulator
	return accumulator
}

func targetPairCampaignMutationOperators(values []TargetScenarioMutationOperator) []string {
	operators := make([]string, 0, len(values))
	for _, value := range values {
		if value = TargetScenarioMutationOperator(strings.TrimSpace(string(value))); value != "" {
			operators = append(operators, string(value))
		}
	}
	sort.Strings(operators)
	return operators
}
