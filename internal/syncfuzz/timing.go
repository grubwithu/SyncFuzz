package syncfuzz

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const defaultTimingProfileID = "baseline"

type FaultTiming struct {
	ProfileID        string `json:"profile_id"`
	Description      string `json:"description"`
	RecoveryDelay    string `json:"recovery_delay"`
	OrphanChildDelay string `json:"orphan_child_delay,omitempty"`
	ReplayDelay      string `json:"replay_delay,omitempty"`
}

func TimingProfiles() []FaultTiming {
	return []FaultTiming{
		{
			ProfileID:        defaultTimingProfileID,
			Description:      "default stable timing; recovery delay follows --delay",
			OrphanChildDelay: "1s",
			ReplayDelay:      "0s",
		},
		{
			ProfileID:        "tight",
			Description:      "compressed deterministic timing for faster scheduler sweeps",
			RecoveryDelay:    "750ms",
			OrphanChildDelay: "250ms",
			ReplayDelay:      "0s",
		},
		{
			ProfileID:        "wide",
			Description:      "wider deterministic timing window for late-effect and replay boundaries",
			RecoveryDelay:    "2500ms",
			OrphanChildDelay: "1500ms",
			ReplayDelay:      "250ms",
		},
	}
}

func resolveTimingProfile(profileID string, fallbackRecoveryDelay time.Duration) (FaultTiming, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		profileID = defaultTimingProfileID
	}
	for _, profile := range TimingProfiles() {
		if profile.ProfileID != profileID {
			continue
		}
		if profile.RecoveryDelay == "" {
			profile.RecoveryDelay = fallbackRecoveryDelay.String()
		}
		if _, err := profile.recoveryDelayDuration(); err != nil {
			return FaultTiming{}, err
		}
		if _, err := profile.orphanChildDelayDuration(); err != nil {
			return FaultTiming{}, err
		}
		if _, err := profile.replayDelayDuration(); err != nil {
			return FaultTiming{}, err
		}
		return profile, nil
	}
	return FaultTiming{}, fmt.Errorf("unknown timing profile %q", profileID)
}

func (t FaultTiming) recoveryDelayDuration() (time.Duration, error) {
	return parseTimingDuration(t.RecoveryDelay)
}

func (t FaultTiming) orphanChildDelayDuration() (time.Duration, error) {
	return parseTimingDuration(t.OrphanChildDelay)
}

func (t FaultTiming) replayDelayDuration() (time.Duration, error) {
	if t.ReplayDelay == "" {
		return 0, nil
	}
	return parseTimingDuration(t.ReplayDelay)
}

func parseTimingDuration(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("timing duration is required")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse timing duration %q: %w", value, err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("timing duration %q must not be negative", value)
	}
	return duration, nil
}

func shellSleepDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0"
	}
	if duration%time.Second == 0 {
		return strconv.FormatInt(int64(duration/time.Second), 10)
	}
	return strconv.FormatFloat(duration.Seconds(), 'f', 3, 64)
}
