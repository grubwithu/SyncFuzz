package core_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func TestResolveTimingProfileUsesBaselineDelay(t *testing.T) {
	profile, err := core.ResolveTimingProfile("", 1500*time.Millisecond)
	if err != nil {
		t.Fatalf("ResolveTimingProfile failed: %v", err)
	}
	if profile.ProfileID != core.DefaultTimingProfileID {
		t.Fatalf("unexpected profile id %q", profile.ProfileID)
	}
	if profile.RecoveryDelay != "1.5s" {
		t.Fatalf("unexpected recovery delay %q", profile.RecoveryDelay)
	}
}

func TestResolveTimingProfileRejectsUnknown(t *testing.T) {
	_, err := core.ResolveTimingProfile("not-a-profile", time.Second)
	if err == nil {
		t.Fatalf("expected unknown timing profile error")
	}
}

func TestRunCarriesTimingProfile(t *testing.T) {
	result, err := cases.Run(context.Background(), core.RunOptions{
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
