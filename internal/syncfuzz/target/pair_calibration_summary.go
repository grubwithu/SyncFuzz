package target

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
)

const (
	TargetPairCalibrationSummarySchemaVersion = "syncfuzz.target-pair-calibration-summary.v1"
	TargetPairCalibrationSummaryArtifact      = "target-pair-calibration-summary.json"
)

// TargetPairCalibrationSummary aggregates independently produced pair reports
// for a controlled campaign. It reports calibration coverage and unresolved
// reasons, while preserving every source artifact for later candidate-level
// human review; it derives hypothesis precision only from separate review
// manifests, never from generated evidence alone.
type TargetPairCalibrationSummary struct {
	SchemaVersion          string                                   `json:"schema_version"`
	GeneratedAt            string                                   `json:"generated_at"`
	TotalPairs             int                                      `json:"total_pairs"`
	RootCauseEligiblePairs int                                      `json:"root_cause_eligible_pairs"`
	CalibrationCoverage    float64                                  `json:"calibration_coverage"`
	EvidenceCandidates     int                                      `json:"evidence_candidates"`
	RootCauseCandidates    int                                      `json:"root_cause_candidates"`
	Reports                []TargetPairCalibrationReportRef         `json:"reports"`
	CalibrationStatuses    []TargetPairCalibrationStatusStats       `json:"calibration_statuses"`
	UnresolvedReasons      []TargetPairCalibrationReasonStats       `json:"unresolved_reasons,omitempty"`
	ContractRules          []TargetPairCalibrationContractRuleStats `json:"contract_rules,omitempty"`
	HypothesisReview       *TargetPairHypothesisReviewStats         `json:"hypothesis_review,omitempty"`
}

// TargetPairCalibrationReportRef keeps the campaign summary auditable: each
// aggregate value can be traced back to a concrete pair artifact and its typed
// lifecycle query.
type TargetPairCalibrationReportRef struct {
	Artifact            string `json:"artifact"`
	QueryID             string `json:"query_id"`
	ControlRunID        string `json:"control_run_id"`
	TargetRunID         string `json:"target_run_id"`
	CalibrationStatus   string `json:"calibration_status"`
	CalibrationReason   string `json:"calibration_reason,omitempty"`
	RootCauseEligible   bool   `json:"root_cause_eligible"`
	EvidenceCandidates  int    `json:"evidence_candidates"`
	RootCauseCandidates int    `json:"root_cause_candidates"`
}

type TargetPairCalibrationStatusStats struct {
	Status                 string `json:"status"`
	PairCount              int    `json:"pair_count"`
	RootCauseEligiblePairs int    `json:"root_cause_eligible_pairs"`
	RootCauseCandidates    int    `json:"root_cause_candidates"`
}

type TargetPairCalibrationReasonStats struct {
	Reason    string `json:"reason"`
	PairCount int    `json:"pair_count"`
}

type TargetPairCalibrationContractRuleStats struct {
	ProfileID              string                       `json:"profile_id"`
	RuleID                 string                       `json:"rule_id"`
	StateSurface           string                       `json:"state_surface,omitempty"`
	LifecycleEdge          string                       `json:"lifecycle_edge,omitempty"`
	SourceStrength         TargetContractSourceStrength `json:"source_strength,omitempty"`
	PairCount              int                          `json:"pair_count"`
	RootCauseEligiblePairs int                          `json:"root_cause_eligible_pairs"`
	RootCauseCandidates    int                          `json:"root_cause_candidates"`
}

type TargetPairRootCauseReviewVerdict string

const (
	TargetPairRootCauseReviewSupported    TargetPairRootCauseReviewVerdict = "supported"
	TargetPairRootCauseReviewUnsupported  TargetPairRootCauseReviewVerdict = "unsupported"
	TargetPairRootCauseReviewInconclusive TargetPairRootCauseReviewVerdict = "inconclusive"
)

// TargetPairRootCauseReviewManifest stores independent candidate-level review
// labels. It is separate from generated pair evidence so a campaign cannot
// silently treat its own hypothesis as ground truth.
type TargetPairRootCauseReviewManifest struct {
	SchemaVersion string                               `json:"schema_version"`
	GeneratedAt   string                               `json:"generated_at,omitempty"`
	Reviews       []TargetPairRootCauseCandidateReview `json:"reviews"`
}

type TargetPairRootCauseCandidateReview struct {
	TargetRunID       string                           `json:"target_run_id"`
	Point             observation.ObservationPoint     `json:"point"`
	StateSurface      string                           `json:"state_surface"`
	Mechanism         string                           `json:"mechanism"`
	Evidence          string                           `json:"evidence"`
	ContractProfileID string                           `json:"contract_profile_id"`
	ContractRuleID    string                           `json:"contract_rule_id"`
	Verdict           TargetPairRootCauseReviewVerdict `json:"verdict"`
	Rationale         string                           `json:"rationale,omitempty"`
}

// TargetPairHypothesisReviewStats reports precision only over reviewed
// supported/unsupported candidates. Inconclusive reviews remain visible but
// do not silently count as either true or false support.
type TargetPairHypothesisReviewStats struct {
	ReviewManifests        []string `json:"review_manifests"`
	ReviewedCandidates     int      `json:"reviewed_candidates"`
	SupportedCandidates    int      `json:"supported_candidates"`
	UnsupportedCandidates  int      `json:"unsupported_candidates"`
	InconclusiveCandidates int      `json:"inconclusive_candidates"`
	PrecisionDenominator   int      `json:"precision_denominator"`
	PrecisionAvailable     bool     `json:"precision_available"`
	Precision              float64  `json:"precision"`
}

type TargetPairCalibrationSummaryOptions struct {
	Inputs              []string
	ReviewManifestPaths []string
	OutputPath          string
}

// SummarizeTargetPairCalibrations reads pair reports directly or recursively
// from directory roots. Directory walking never follows directory symlinks and
// only accepts the canonical target-pair-differential artifact name.
func SummarizeTargetPairCalibrations(opts TargetPairCalibrationSummaryOptions) (*TargetPairCalibrationSummary, error) {
	reportPaths, err := targetPairCalibrationReportPaths(opts.Inputs)
	if err != nil {
		return nil, err
	}
	summary := &TargetPairCalibrationSummary{
		SchemaVersion: TargetPairCalibrationSummarySchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Reports:       make([]TargetPairCalibrationReportRef, 0, len(reportPaths)),
	}
	statusStats := make(map[string]*TargetPairCalibrationStatusStats)
	reasonStats := make(map[string]*TargetPairCalibrationReasonStats)
	ruleStats := make(map[string]*TargetPairCalibrationContractRuleStats)
	rootCandidates := make(map[string]TargetPairRootCauseCandidate)
	for _, reportPath := range reportPaths {
		report, err := readTargetPairCalibrationReport(reportPath)
		if err != nil {
			return nil, err
		}
		calibration := report.ContractCalibration
		summary.TotalPairs++
		summary.EvidenceCandidates += len(report.Evidence)
		summary.RootCauseCandidates += len(report.RootCauseCandidates)
		if calibration.RootCauseEligible {
			summary.RootCauseEligiblePairs++
		}
		for _, candidate := range report.RootCauseCandidates {
			key := targetPairRootCauseReviewKey(report.TargetRunID, candidate)
			if _, exists := rootCandidates[key]; exists {
				return nil, fmt.Errorf("pair report %s contains duplicate root-cause candidate %q", reportPath, candidate.Evidence)
			}
			rootCandidates[key] = candidate
		}
		summary.Reports = append(summary.Reports, TargetPairCalibrationReportRef{
			Artifact:            reportPath,
			QueryID:             report.QueryID,
			ControlRunID:        report.ControlRunID,
			TargetRunID:         report.TargetRunID,
			CalibrationStatus:   calibration.Status,
			CalibrationReason:   calibration.Reason,
			RootCauseEligible:   calibration.RootCauseEligible,
			EvidenceCandidates:  len(report.Evidence),
			RootCauseCandidates: len(report.RootCauseCandidates),
		})
		status := strings.TrimSpace(calibration.Status)
		if status == "" {
			status = "missing"
		}
		stat := statusStats[status]
		if stat == nil {
			stat = &TargetPairCalibrationStatusStats{Status: status}
			statusStats[status] = stat
		}
		stat.PairCount++
		stat.RootCauseCandidates += len(report.RootCauseCandidates)
		if calibration.RootCauseEligible {
			stat.RootCauseEligiblePairs++
		}
		if status != TargetPairContractCalibrationCalibrated {
			reason := strings.TrimSpace(calibration.Reason)
			if reason == "" {
				reason = "missing calibration reason"
			}
			reasonStat := reasonStats[reason]
			if reasonStat == nil {
				reasonStat = &TargetPairCalibrationReasonStats{Reason: reason}
				reasonStats[reason] = reasonStat
			}
			reasonStat.PairCount++
		}
		if rule := calibration.Target; rule.Available && strings.TrimSpace(rule.ProfileID) != "" && strings.TrimSpace(rule.RuleID) != "" {
			key := targetPairCalibrationRuleKey(rule)
			ruleStat := ruleStats[key]
			if ruleStat == nil {
				ruleStat = &TargetPairCalibrationContractRuleStats{
					ProfileID:      rule.ProfileID,
					RuleID:         rule.RuleID,
					StateSurface:   rule.StateSurface,
					LifecycleEdge:  rule.LifecycleEdge,
					SourceStrength: rule.SourceStrength,
				}
				ruleStats[key] = ruleStat
			}
			ruleStat.PairCount++
			ruleStat.RootCauseCandidates += len(report.RootCauseCandidates)
			if calibration.RootCauseEligible {
				ruleStat.RootCauseEligiblePairs++
			}
		}
	}
	if summary.TotalPairs > 0 {
		summary.CalibrationCoverage = float64(summary.RootCauseEligiblePairs) / float64(summary.TotalPairs)
	}
	for _, stat := range statusStats {
		summary.CalibrationStatuses = append(summary.CalibrationStatuses, *stat)
	}
	for _, stat := range reasonStats {
		summary.UnresolvedReasons = append(summary.UnresolvedReasons, *stat)
	}
	for _, stat := range ruleStats {
		summary.ContractRules = append(summary.ContractRules, *stat)
	}
	if len(opts.ReviewManifestPaths) > 0 {
		reviewStats, err := summarizeTargetPairHypothesisReviews(opts.ReviewManifestPaths, rootCandidates)
		if err != nil {
			return nil, err
		}
		summary.HypothesisReview = &reviewStats
	}
	canonicalizeTargetPairCalibrationSummary(summary)

	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		return nil, fmt.Errorf("summary output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return nil, fmt.Errorf("create target pair calibration summary directory: %w", err)
	}
	if err := core.WriteJSON(output, summary); err != nil {
		return nil, fmt.Errorf("write target pair calibration summary %s: %w", output, err)
	}
	return summary, nil
}

func summarizeTargetPairHypothesisReviews(manifestPaths []string, candidates map[string]TargetPairRootCauseCandidate) (TargetPairHypothesisReviewStats, error) {
	paths, err := targetPairReviewManifestPaths(manifestPaths)
	if err != nil {
		return TargetPairHypothesisReviewStats{}, err
	}
	stats := TargetPairHypothesisReviewStats{ReviewManifests: paths}
	seen := make(map[string]struct{})
	for _, path := range paths {
		manifest, err := readTargetPairReviewManifest(path)
		if err != nil {
			return TargetPairHypothesisReviewStats{}, err
		}
		for _, review := range manifest.Reviews {
			key := targetPairRootCauseReviewKeyForReview(review)
			if _, exists := seen[key]; exists {
				return TargetPairHypothesisReviewStats{}, fmt.Errorf("duplicate root-cause review for target run %q", review.TargetRunID)
			}
			if _, exists := candidates[key]; !exists {
				return TargetPairHypothesisReviewStats{}, fmt.Errorf("root-cause review for target run %q does not match a candidate in the input reports", review.TargetRunID)
			}
			seen[key] = struct{}{}
			stats.ReviewedCandidates++
			switch review.Verdict {
			case TargetPairRootCauseReviewSupported:
				stats.SupportedCandidates++
			case TargetPairRootCauseReviewUnsupported:
				stats.UnsupportedCandidates++
			case TargetPairRootCauseReviewInconclusive:
				stats.InconclusiveCandidates++
			default:
				return TargetPairHypothesisReviewStats{}, fmt.Errorf("root-cause review for target run %q has unsupported verdict %q", review.TargetRunID, review.Verdict)
			}
		}
	}
	stats.PrecisionDenominator = stats.SupportedCandidates + stats.UnsupportedCandidates
	if stats.PrecisionDenominator > 0 {
		stats.PrecisionAvailable = true
		stats.Precision = float64(stats.SupportedCandidates) / float64(stats.PrecisionDenominator)
	}
	return stats, nil
}

func targetPairReviewManifestPaths(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one review manifest is required")
	}
	seen := make(map[string]struct{})
	paths := make([]string, 0, len(inputs))
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("stat root-cause review manifest %s: %w", input, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("root-cause review manifest must be a file: %s", input)
		}
		if err := addTargetPairCalibrationReportPath(input, seen, &paths); err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one review manifest is required")
	}
	sort.Strings(paths)
	return paths, nil
}

func readTargetPairReviewManifest(path string) (TargetPairRootCauseReviewManifest, error) {
	manifest, err := readTargetPairJSON[TargetPairRootCauseReviewManifest](path)
	if err != nil {
		return TargetPairRootCauseReviewManifest{}, fmt.Errorf("read root-cause review manifest %s: %w", path, err)
	}
	if manifest.SchemaVersion != "syncfuzz.target-pair-root-cause-review.v1" {
		return TargetPairRootCauseReviewManifest{}, fmt.Errorf("root-cause review manifest %s has unsupported schema %q", path, manifest.SchemaVersion)
	}
	return manifest, nil
}

func targetPairRootCauseReviewKey(targetRunID string, candidate TargetPairRootCauseCandidate) string {
	return strings.Join([]string{
		strings.TrimSpace(targetRunID),
		string(candidate.Point),
		strings.TrimSpace(candidate.StateSurface),
		strings.TrimSpace(candidate.Mechanism),
		strings.TrimSpace(candidate.Evidence),
		strings.TrimSpace(candidate.ContractProfileID),
		strings.TrimSpace(candidate.ContractRuleID),
	}, "\x00")
}

func targetPairRootCauseReviewKeyForReview(review TargetPairRootCauseCandidateReview) string {
	return strings.Join([]string{
		strings.TrimSpace(review.TargetRunID),
		string(review.Point),
		strings.TrimSpace(review.StateSurface),
		strings.TrimSpace(review.Mechanism),
		strings.TrimSpace(review.Evidence),
		strings.TrimSpace(review.ContractProfileID),
		strings.TrimSpace(review.ContractRuleID),
	}, "\x00")
}

func targetPairCalibrationReportPaths(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one pair report or directory is required")
	}
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("stat pair report input %s: %w", input, err)
		}
		if !info.IsDir() {
			if err := addTargetPairCalibrationReportPath(input, seen, &paths); err != nil {
				return nil, err
			}
			continue
		}
		if err := filepath.WalkDir(input, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Name() != TargetPairDifferentialArtifact {
				return nil
			}
			return addTargetPairCalibrationReportPath(path, seen, &paths)
		}); err != nil {
			return nil, fmt.Errorf("walk pair report directory %s: %w", input, err)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no %s artifacts found", TargetPairDifferentialArtifact)
	}
	sort.Strings(paths)
	return paths, nil
}

func addTargetPairCalibrationReportPath(path string, seen map[string]struct{}, paths *[]string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve pair report path %s: %w", path, err)
	}
	abs = filepath.Clean(abs)
	if _, ok := seen[abs]; ok {
		return nil
	}
	seen[abs] = struct{}{}
	*paths = append(*paths, abs)
	return nil
}

func readTargetPairCalibrationReport(path string) (TargetPairDifferential, error) {
	report, err := readTargetPairJSON[TargetPairDifferential](path)
	if err != nil {
		return TargetPairDifferential{}, fmt.Errorf("read pair report %s: %w", path, err)
	}
	if report.SchemaVersion != TargetPairDifferentialSchemaVersion {
		return TargetPairDifferential{}, fmt.Errorf("pair report %s has unsupported schema %q", path, report.SchemaVersion)
	}
	if strings.TrimSpace(report.QueryID) == "" {
		return TargetPairDifferential{}, fmt.Errorf("pair report %s is missing query_id", path)
	}
	calibration := report.ContractCalibration
	if calibration.RootCauseEligible && calibration.Status != TargetPairContractCalibrationCalibrated {
		return TargetPairDifferential{}, fmt.Errorf("pair report %s marks root-cause eligibility without contract calibration", path)
	}
	if calibration.Status == TargetPairContractCalibrationCalibrated && !calibration.RootCauseEligible {
		return TargetPairDifferential{}, fmt.Errorf("pair report %s has calibrated status without root-cause eligibility", path)
	}
	if len(report.RootCauseCandidates) > 0 && !calibration.RootCauseEligible {
		return TargetPairDifferential{}, fmt.Errorf("pair report %s has root-cause candidates without contract calibration", path)
	}
	return report, nil
}

func targetPairCalibrationRuleKey(reading TargetPairContractReading) string {
	return strings.Join([]string{
		reading.ProfileID,
		reading.RuleID,
		reading.StateSurface,
		reading.LifecycleEdge,
		string(reading.SourceStrength),
	}, "\x00")
}

func canonicalizeTargetPairCalibrationSummary(summary *TargetPairCalibrationSummary) {
	if summary == nil {
		return
	}
	sort.Slice(summary.Reports, func(i, j int) bool {
		return summary.Reports[i].Artifact < summary.Reports[j].Artifact
	})
	sort.Slice(summary.CalibrationStatuses, func(i, j int) bool {
		return summary.CalibrationStatuses[i].Status < summary.CalibrationStatuses[j].Status
	})
	sort.Slice(summary.UnresolvedReasons, func(i, j int) bool {
		return summary.UnresolvedReasons[i].Reason < summary.UnresolvedReasons[j].Reason
	})
	sort.Slice(summary.ContractRules, func(i, j int) bool {
		left := summary.ContractRules[i]
		right := summary.ContractRules[j]
		if left.ProfileID != right.ProfileID {
			return left.ProfileID < right.ProfileID
		}
		if left.RuleID != right.RuleID {
			return left.RuleID < right.RuleID
		}
		if left.StateSurface != right.StateSurface {
			return left.StateSurface < right.StateSurface
		}
		if left.LifecycleEdge != right.LifecycleEdge {
			return left.LifecycleEdge < right.LifecycleEdge
		}
		return left.SourceStrength < right.SourceStrength
	})
}
