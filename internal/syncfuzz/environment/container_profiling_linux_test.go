//go:build linux

package environment

import "testing"

func TestParseContainerInspectScope(t *testing.T) {
	containerID, hostPID, err := parseContainerInspectScope("9a8b7c 1234\n")
	if err != nil {
		t.Fatalf("parseContainerInspectScope returned error: %v", err)
	}
	if containerID != "9a8b7c" || hostPID != 1234 {
		t.Fatalf("unexpected container scope: id=%q pid=%d", containerID, hostPID)
	}
	if _, _, err := parseContainerInspectScope("9a8b7c 0"); err == nil {
		t.Fatal("expected zero pid to fail")
	}
}
