package syncfuzz

import (
	"fmt"
	"strings"
)

const (
	targetPromptProfileBaselineID = "baseline"
	targetPromptProfileWorkflowID = "workflow"
	targetPromptProfileAuditID    = "audit"
)

type TargetPromptProfileInfo struct {
	ProfileID   string `json:"profile_id"`
	Description string `json:"description"`
}

func TargetPromptProfiles() []TargetPromptProfileInfo {
	return []TargetPromptProfileInfo{
		{
			ProfileID:   targetPromptProfileBaselineID,
			Description: "current direct built-in prompt wording",
		},
		{
			ProfileID:   targetPromptProfileWorkflowID,
			Description: "routine workspace handoff framing with the same task semantics",
		},
		{
			ProfileID:   targetPromptProfileAuditID,
			Description: "reproducibility-audit framing with the same task semantics",
		},
	}
}

func targetPromptProfileInfoByID(profileID string) (TargetPromptProfileInfo, bool) {
	profileID = normalizeTargetPromptProfileID(profileID)
	for _, profile := range TargetPromptProfiles() {
		if profile.ProfileID == profileID {
			return profile, true
		}
	}
	return TargetPromptProfileInfo{}, false
}

func normalizeTargetPromptProfileID(profileID string) string {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return targetPromptProfileBaselineID
	}
	return profileID
}

func resolveTargetPromptProfile(profileID string) (TargetPromptProfileInfo, error) {
	if profile, ok := targetPromptProfileInfoByID(profileID); ok {
		return profile, nil
	}
	return TargetPromptProfileInfo{}, fmt.Errorf("unknown target prompt profile %q", profileID)
}

func resolveTargetPromptProfiles(profileIDs []string) ([]TargetPromptProfileInfo, error) {
	if len(profileIDs) == 0 {
		profile, err := resolveTargetPromptProfile("")
		if err != nil {
			return nil, err
		}
		return []TargetPromptProfileInfo{profile}, nil
	}
	if len(profileIDs) == 1 && strings.EqualFold(strings.TrimSpace(profileIDs[0]), "all") {
		return TargetPromptProfiles(), nil
	}

	var out []TargetPromptProfileInfo
	seen := make(map[string]struct{})
	for _, raw := range profileIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		profile, err := resolveTargetPromptProfile(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[profile.ProfileID]; ok {
			continue
		}
		seen[profile.ProfileID] = struct{}{}
		out = append(out, profile)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one target prompt profile is required")
	}
	return out, nil
}

func targetPromptProfileSelection(single string, multi []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range multi {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	single = strings.TrimSpace(single)
	if single != "" {
		if _, ok := seen[single]; !ok {
			out = append(out, single)
		}
	}
	return out
}

func applyTargetPromptProfile(prompt string, profileID string) string {
	profileID = normalizeTargetPromptProfileID(profileID)
	switch profileID {
	case targetPromptProfileWorkflowID:
		return strings.TrimSpace("Context: this is a routine workspace continuity and handoff check. Follow the requested steps exactly, keep any requested workspace artifacts intact, avoid exploratory side work, and treat any required shell/tool call counts as mandatory rather than answering in prose.\n\n" + prompt)
	case targetPromptProfileAuditID:
		return strings.TrimSpace("Context: this is a deterministic reproducibility audit. Preserve requested witness artifacts for a later verification step, avoid extra probing, and follow the procedure exactly as written.\n\n" + prompt)
	default:
		return strings.TrimSpace(prompt)
	}
}
