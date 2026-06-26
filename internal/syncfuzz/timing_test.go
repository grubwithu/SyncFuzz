package syncfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveTimingProfileUsesBaselineDelay(t *testing.T) {
	profile, err := resolveTimingProfile("", 1500*time.Millisecond)
	if err != nil {
		t.Fatalf("resolveTimingProfile failed: %v", err)
	}
	if profile.ProfileID != defaultTimingProfileID {
		t.Fatalf("unexpected profile id %q", profile.ProfileID)
	}
	if profile.RecoveryDelay != "1.5s" {
		t.Fatalf("unexpected recovery delay %q", profile.RecoveryDelay)
	}
}

func TestResolveTimingProfileRejectsUnknown(t *testing.T) {
	_, err := resolveTimingProfile("not-a-profile", time.Second)
	if err == nil {
		t.Fatalf("expected unknown timing profile error")
	}
}

func TestRunCarriesTimingProfile(t *testing.T) {
	result, err := Run(context.Background(), RunOptions{
		CaseName:        "action-replay",
		OutDir:          filepath.Join(t.TempDir(), "runs"),
		TimingProfileID: "tight",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.TimingProfileID != "tight" {
		t.Fatalf("unexpected timing profile %q", result.TimingProfileID)
	}
}
