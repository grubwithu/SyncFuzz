package syncfuzz

import (
	"fmt"
	"sort"
	"strings"
)

type MatrixOptions struct {
	Cases            []string
	TimingProfileIDs []string
	IncludePlanned   bool
}

type ScheduleCandidate struct {
	CandidateID     string   `json:"candidate_id"`
	CaseName        string   `json:"case_name"`
	FaultPlanID     string   `json:"fault_plan_id"`
	PrimitiveID     string   `json:"primitive_id"`
	TimingProfileID string   `json:"timing_profile_id"`
	Lifecycle       string   `json:"lifecycle"`
	InjectPhase     string   `json:"inject_phase"`
	StateLayers     []string `json:"state_layers"`
	StateClasses    []string `json:"state_classes"`
	ExpectedImpact  string   `json:"expected_impact"`
	Implemented     bool     `json:"implemented"`
}

type ScheduleMatrix struct {
	SchemaVersion   string              `json:"schema_version"`
	Cases           []string            `json:"cases"`
	TimingProfiles  []string            `json:"timing_profiles"`
	IncludePlanned  bool                `json:"include_planned"`
	TotalCandidates int                 `json:"total_candidates"`
	Candidates      []ScheduleCandidate `json:"candidates"`
}

func BuildScheduleMatrix(opts MatrixOptions) (*ScheduleMatrix, error) {
	if err := validatePrimitiveCatalog(); err != nil {
		return nil, err
	}
	selectedCases := opts.Cases
	if len(selectedCases) == 0 {
		selectedCases = caseNames()
	}
	if err := validateCaseNames(selectedCases); err != nil {
		return nil, err
	}

	timingProfiles, err := resolveMatrixTimingProfiles(opts.TimingProfileIDs)
	if err != nil {
		return nil, err
	}

	var candidates []ScheduleCandidate
	for _, caseName := range selectedCases {
		faultPlan, err := resolveFaultPlan(caseName, "")
		if err != nil {
			return nil, err
		}
		for _, primitive := range primitivesForCase(caseName, opts.IncludePlanned) {
			for _, timing := range timingProfiles {
				candidates = append(candidates, ScheduleCandidate{
					CandidateID:     scheduleCandidateID(caseName, primitive.ID, timing.ProfileID),
					CaseName:        caseName,
					FaultPlanID:     faultPlan.ID,
					PrimitiveID:     primitive.ID,
					TimingProfileID: timing.ProfileID,
					Lifecycle:       faultPlan.Lifecycle,
					InjectPhase:     string(faultPlan.InjectPhase),
					StateLayers:     uniqueStrings(append(append([]string{}, faultPlan.StateLayers...), primitive.StateLayers...)),
					StateClasses:    append([]string{}, primitive.StateClasses...),
					ExpectedImpact:  faultPlan.ExpectedImpact,
					Implemented:     primitive.Implemented,
				})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CandidateID < candidates[j].CandidateID
	})
	return &ScheduleMatrix{
		SchemaVersion:   "syncfuzz.schedule-matrix.v1",
		Cases:           append([]string{}, selectedCases...),
		TimingProfiles:  timingProfileIDs(timingProfiles),
		IncludePlanned:  opts.IncludePlanned,
		TotalCandidates: len(candidates),
		Candidates:      candidates,
	}, nil
}

func resolveMatrixTimingProfiles(profileIDs []string) ([]FaultTiming, error) {
	if len(profileIDs) == 0 {
		return TimingProfiles(), nil
	}
	var out []FaultTiming
	seen := make(map[string]struct{})
	for _, id := range profileIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		profile, err := resolveTimingProfile(id, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, profile)
		seen[id] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one timing profile is required")
	}
	return out, nil
}

func scheduleCandidateID(caseName string, primitiveID string, timingProfileID string) string {
	return strings.Join([]string{caseName, primitiveID, timingProfileID}, "/")
}

func timingProfileIDs(profiles []FaultTiming) []string {
	out := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, profile.ProfileID)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
